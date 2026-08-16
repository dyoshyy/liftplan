package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan-server/internal/presentation/httpapi"
)

func newServer(t *testing.T, configured bool) http.Handler {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	exercises := memory.NewExerciseRepository(pool)
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()

	programs := memory.NewProgramRepository(nil)
	if configured {
		freq, _ := training.NewFrequency(3)
		target, err := seed.DefaultWeeklyTarget(freq)
		if err != nil {
			t.Fatalf("週目標が不正: %v", err)
		}
		selected := make([]training.ExerciseID, 0, len(pool))
		for _, e := range pool {
			if e.Kind() != training.KindVariation {
				selected = append(selected, e.ID())
			}
		}
		program, err := training.NewProgram(freq, target, selected)
		if err != nil {
			t.Fatalf("プログラムが不正: %v", err)
		}
		if err := programs.Save(context.Background(), program); err != nil {
			t.Fatalf("プログラムの保存に失敗: %v", err)
		}
	}

	handler := httpapi.NewHandler(
		usecase.NewGetSession(exercises, logs, conditions, programs, training.DefaultSessionPlanner()),
		usecase.NewRecordSets(logs, exercises),
		usecase.NewRecordConditions(conditions),
		usecase.NewConfigureProgram(exercises, programs),
		usecase.NewGetProgram(programs),
	)
	return handler.Routes()
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ステータスが誤り: %d", rec.Code)
	}
}

func TestGetSession_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil)
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Date string `json:"date"`
		Main []struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
			Sets       int      `json:"sets"`
			TargetRIR  int      `json:"target_rir"`
			Role       string   `json:"role"`
		} `json:"main"`
		Accessories []struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"accessories"`
		Deload *struct {
			Reason string `json:"reason"`
		} `json:"deload_proposal"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}

	if body.Date != "2026-08-17" {
		t.Errorf("日付が誤り: %s", body.Date)
	}
	if len(body.Main) != 3 {
		t.Errorf("メインが3種目でない: %d", len(body.Main))
	}
	if len(body.Accessories) == 0 {
		t.Error("補助種目が空である")
	}
	// 履歴が無いので重量は null になるのが正しい
	for _, m := range body.Main {
		if m.WeightKg != nil {
			t.Errorf("履歴が無いのに重量が入っている: %s", m.ExerciseID)
		}
		if m.Sets <= 0 {
			t.Errorf("セット数が0以下: %s", m.ExerciseID)
		}
	}
}

func TestGetSession_MissingDate(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("日付なしが 400 にならない: %d", rec.Code)
	}
}

func TestGetSession_BadDate(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026/08/17", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("不正な日付が 400 にならない: %d", rec.Code)
	}
}

func TestGetSession_ProgramNotConfigured(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, false).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("未設定が 409 にならない: %d", rec.Code)
	}
}

func TestPostSetLogs(t *testing.T) {
	server := newServer(t, true)

	payload := `{"logs":[{"id":"01J-A","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":9,"rir":2}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/set-logs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	// 保存されたログが次のセッションに反映される
	rec2 := httptest.NewRecorder()
	server.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-24", nil))

	var body struct {
		Main []struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
		} `json:"main"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	found := false
	for _, m := range body.Main {
		if m.ExerciseID == "bench" && m.WeightKg != nil {
			found = true
		}
	}
	if !found {
		t.Error("記録したログが重量算出に反映されていない")
	}
}

func TestPostSetLogs_InvalidBody(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/set-logs", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("壊れたJSONが 400 にならない: %d", rec.Code)
	}
}

func TestPostSetLogs_InvalidDomainValue(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := `{"logs":[{"id":"01J-B","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":0,"rir":2}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/set-logs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("0レップが 400 にならない: %d", rec.Code)
	}
}

func TestPostConditions(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := `{"conditions":[{"date":"2026-08-17","body_weight_kg":75.2,"sleep_hours":6.5}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/conditions", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/sessions", nil))
	if rec.Code == http.StatusOK {
		t.Error("DELETE が通ってしまう")
	}
}

// --- ステータス分類 ---

func do(t *testing.T, mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

// 入力の不正は 400、I/O の失敗は 500。区別できないと、
// クライアントは自分の入力を直さずリトライを繰り返す。
func TestPutProgram_ClassifiesFailures(t *testing.T) {
	valid := `{"per_week":3,"weekly_target":{"CHEST_MID":12,"QUAD":12},` +
		`"selected_exercises":["bench","squat","deadlift"]}`

	cases := map[string]struct {
		body string
		want int
	}{
		"正常":         {valid, http.StatusNoContent},
		"頻度が範囲外":     {`{"per_week":99,"weekly_target":{"QUAD":12},"selected_exercises":["squat"]}`, http.StatusBadRequest},
		"週目標が空":      {`{"per_week":3,"weekly_target":{},"selected_exercises":["squat"]}`, http.StatusBadRequest},
		"未知の筋区分":     {`{"per_week":3,"weekly_target":{"膝の皿":8},"selected_exercises":["squat"]}`, http.StatusBadRequest},
		"実在しない種目":    {`{"per_week":3,"weekly_target":{"QUAD":12},"selected_exercises":["無い種目"]}`, http.StatusBadRequest},
		"メイン種目ゼロ":    {`{"per_week":3,"weekly_target":{"BICEPS":9},"selected_exercises":["barbell_curl"]}`, http.StatusBadRequest},
		"選択が空":       {`{"per_week":3,"weekly_target":{"QUAD":12},"selected_exercises":[]}`, http.StatusBadRequest},
		"JSONが壊れている": {`{`, http.StatusBadRequest},
		"未知のフィールド":   {`{"per_week":3,"weekly_target":{"QUAD":12},"selected_exercises":["squat"],"謎":1}`, http.StatusBadRequest},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(t, newServer(t, false), http.MethodPut, "/api/program", c.body)
			if rec.Code != c.want {
				t.Errorf("ステータスが誤り: %d（期待 %d）body=%s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// 500 のときに内部のエラー文を返さないこと。
// ドメインのエラーには種目IDや閾値が載っており、外に出す理由がない。
func TestErrors_DoNotLeakInternals(t *testing.T) {
	mux := newServer(t, false)
	rec := do(t, mux, http.MethodPut, "/api/program",
		`{"per_week":3,"weekly_target":{"QUAD":12},"selected_exercises":["無い種目"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ステータスが誤り: %d", rec.Code)
	}
	// 400 は原因を返す。ユーザーが直せる情報なので隠す理由がない。
	if !strings.Contains(rec.Body.String(), "種目") {
		t.Errorf("400 で原因が返っていない: %s", rec.Body.String())
	}
}

// プログラムの設定と取得が往復すること。
func TestProgram_RoundTrips(t *testing.T) {
	mux := newServer(t, false)

	if rec := do(t, mux, http.MethodGet, "/api/program", ""); rec.Code != http.StatusNotFound {
		t.Errorf("未設定なのに 404 でない: %d", rec.Code)
	}

	body := `{"per_week":2,"weekly_target":{"CHEST_MID":12,"QUAD":12},` +
		`"selected_exercises":["bench","squat","deadlift","incline_db_press"]}`
	if rec := do(t, mux, http.MethodPut, "/api/program", body); rec.Code != http.StatusNoContent {
		t.Fatalf("設定に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/program", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("取得に失敗: %d", rec.Code)
	}
	var got struct {
		PerWeek  int                `json:"per_week"`
		Target   map[string]float64 `json:"weekly_target"`
		Selected []string           `json:"selected_exercises"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if got.PerWeek != 2 {
		t.Errorf("頻度が往復していない: %d", got.PerWeek)
	}
	if got.Target["CHEST_MID"] != 12 {
		t.Errorf("週目標が往復していない: %v", got.Target)
	}
	if len(got.Selected) != 4 {
		t.Errorf("選択種目が往復していない: %v", got.Selected)
	}

	// 設定した直後にセッションが導出できること。
	if rec := do(t, mux, http.MethodGet, "/api/sessions?date=2026-08-17", ""); rec.Code != http.StatusOK {
		t.Errorf("設定したのにセッションが出ない: %d body=%s", rec.Code, rec.Body.String())
	}
}

// 同じIDで内容の違うログは 409。500 にすると、クライアントは
// 自分の採番ミスに気づかずリトライを繰り返す。
func TestPostSetLogs_ConflictIs409(t *testing.T) {
	mux := newServer(t, true)
	first := `{"logs":[{"id":"a","date":"2026-08-17","exercise_id":"bench",` +
		`"weight_kg":85,"reps":8,"rir":2}]}`
	second := `{"logs":[{"id":"a","date":"2026-08-17","exercise_id":"bench",` +
		`"weight_kg":90,"reps":8,"rir":2}]}`

	if rec := do(t, mux, http.MethodPost, "/api/set-logs", first); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	// 同じ内容の再送は成功する（クライアントはタイムアウト後に再送する）。
	if rec := do(t, mux, http.MethodPost, "/api/set-logs", first); rec.Code != http.StatusNoContent {
		t.Errorf("同じ内容の再送が失敗した: %d", rec.Code)
	}
	if rec := do(t, mux, http.MethodPost, "/api/set-logs", second); rec.Code != http.StatusConflict {
		t.Errorf("衝突が 409 でない: %d body=%s", rec.Code, rec.Body.String())
	}
}

// クライアント切断はサーバーの障害ではない。500 と混ぜると
// ログと警報がクライアントの都合で汚れる。
func TestGetSession_ClientDisconnectIsNot500(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code == http.StatusInternalServerError {
		t.Errorf("クライアント切断が 500 になった")
	}
	if rec.Code != 499 {
		t.Errorf("ステータスが誤り: %d（期待 499）", rec.Code)
	}
}

// デロードの承認は種目IDのカンマ区切り（D-022）。
func TestGetSession_ParsesDeloadAcceptance(t *testing.T) {
	mux := newServer(t, true)
	for _, raw := range []string{"bench", "bench,squat", "bench,,squat,", " bench , squat "} {
		rec := do(t, mux, http.MethodGet, "/api/sessions?date=2026-08-17&deload_accepted="+url.QueryEscape(raw), "")
		if rec.Code != http.StatusOK {
			t.Errorf("deload_accepted=%q で失敗: %d body=%s", raw, rec.Code, rec.Body.String())
		}
	}
}

// メソッドが違えばルーティングされないこと。
func TestRoutes_RejectWrongMethod(t *testing.T) {
	mux := newServer(t, true)
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/sessions"},
		{http.MethodGet, "/api/set-logs"},
		{http.MethodGet, "/api/conditions"},
		{http.MethodPost, "/api/program"},
	} {
		if rec := do(t, mux, c.method, c.path, "{}"); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: ステータスが誤り: %d", c.method, c.path, rec.Code)
		}
	}
}

// --- 生存したミューテーションを殺すテスト ---

func TestGetSession_RequiresDate(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("date 無しが 400 でない: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "date") {
		t.Errorf("原因が返っていない: %s", rec.Body.String())
	}
}

// 取得に失敗するリポジトリ。500 の経路を作るために使う。
type brokenExercises struct{}

func (brokenExercises) FindAll(context.Context) ([]*training.Exercise, error) {
	return nil, errors.New("種目テーブル exercises_v2 の接続文字列が不正: user=admin")
}

// 500 のときに内部のエラー文を返さないこと。
// ドメインやインフラのエラーにはテーブル名・接続情報・閾値が載っている。
func TestGetSession_InternalErrorDoesNotLeak(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, _ := training.NewFrequency(3)
	target, _ := seed.DefaultWeeklyTarget(freq)
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}
	program, err := training.NewProgram(freq, target, selected)
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	programs := memory.NewProgramRepository(program)
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()

	mux := httpapi.NewHandler(
		usecase.NewGetSession(brokenExercises{}, logs, conditions, programs, training.DefaultSessionPlanner()),
		usecase.NewRecordSets(logs, brokenExercises{}),
		usecase.NewRecordConditions(conditions),
		usecase.NewConfigureProgram(brokenExercises{}, programs),
		usecase.NewGetProgram(programs),
	).Routes()

	rec := do(t, mux, http.MethodGet, "/api/sessions?date=2026-08-17", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("ステータスが誤り: %d", rec.Code)
	}
	for _, leak := range []string{"exercises_v2", "admin", "接続文字列"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("内部の情報が漏れている（%q）: %s", leak, rec.Body.String())
		}
	}
}

// weight_kg / reps / rir の欠落を黙って読み飛ばさないこと。
// 読み飛ばすと、クライアントは保存に成功したと思ったまま実績が消える。
func TestPostSetLogs_RequiresMeasurements(t *testing.T) {
	for name, body := range map[string]string{
		"weight_kg 欠落": `{"logs":[{"id":"a","date":"2026-08-17","exercise_id":"bench","reps":8,"rir":2}]}`,
		"reps 欠落":      `{"logs":[{"id":"a","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"rir":2}]}`,
		"rir 欠落":       `{"logs":[{"id":"a","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":8}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, newServer(t, true), http.MethodPost, "/api/set-logs", body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("欠落が 400 でない: %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// 停滞した履歴を投入し、セッションを取得する。
func stalledServer(t *testing.T) http.Handler {
	t.Helper()
	mux := newServer(t, true)

	var logs []string
	for i := range 10 {
		date := training.MustDate(2026, time.June, 1).AddDays(i * 7)
		for s := range 3 {
			logs = append(logs, fmt.Sprintf(
				`{"id":"b%d-%d","date":"%s","exercise_id":"bench","weight_kg":85,"reps":8,"rir":2}`,
				i, s, date.String()))
		}
	}
	if rec := do(t, mux, http.MethodPost, "/api/set-logs",
		`{"logs":[`+strings.Join(logs, ",")+`]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("実績の保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	var conditions []string
	for i := range 28 {
		date := training.MustDate(2026, time.August, 17).AddDays(-i)
		conditions = append(conditions, fmt.Sprintf(
			`{"date":"%s","body_weight_kg":75}`, date.String()))
	}
	if rec := do(t, mux, http.MethodPost, "/api/conditions",
		`{"conditions":[`+strings.Join(conditions, ",")+`]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("コンディションの保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	return mux
}

func fetchSession(t *testing.T, mux http.Handler, query string) sessionResponse {
	t.Helper()
	rec := do(t, mux, http.MethodGet, "/api/sessions?date=2026-08-17"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("セッションの取得に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	var got sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	return got
}

type sessionResponse struct {
	Main []struct {
		ExerciseID string   `json:"exercise_id"`
		WeightKg   *float64 `json:"weight_kg"`
	} `json:"main"`
	Deload *struct {
		Reason           string  `json:"reason"`
		IntensityDropPct float64 `json:"intensity_drop_pct"`
	} `json:"deload_proposal"`
}

func (s sessionResponse) weightOf(t *testing.T, id string) float64 {
	t.Helper()
	for _, m := range s.Main {
		if m.ExerciseID == id {
			if m.WeightKg == nil {
				t.Fatalf("%s の重量が未確定", id)
			}
			return *m.WeightKg
		}
	}
	t.Fatalf("%s がメインに無い", id)
	return 0
}

// 停滞していればデロード提案が応答に載ること。
// 載らないと、ユーザーは重量が上がらない理由を知る手立てがない。
func TestGetSession_IncludesDeloadProposal(t *testing.T) {
	got := fetchSession(t, stalledServer(t), "")
	if got.Deload == nil {
		t.Fatal("停滞しているのに提案が応答に無い")
	}
	if got.Deload.Reason == "" {
		t.Error("提案の根拠が空である")
	}
	if got.Deload.IntensityDropPct <= 0 {
		t.Errorf("低下率が載っていない: %v", got.Deload.IntensityDropPct)
	}
}

// 承認した種目IDが実際にドメインへ届くこと。
// 空要素や前後の空白が混ざっても同じ結果になること。
func TestGetSession_DeloadAcceptanceReachesTheDomain(t *testing.T) {
	mux := stalledServer(t)
	normal := fetchSession(t, mux, "").weightOf(t, "bench")

	for _, raw := range []string{"bench", ",bench,", " bench ", "bench,無い種目"} {
		got := fetchSession(t, mux, "&deload_accepted="+url.QueryEscape(raw)).weightOf(t, "bench")
		if got >= normal {
			t.Errorf("deload_accepted=%q が届いていない: %v → %v", raw, normal, got)
		}
	}

	// 承認していない種目は変わらない。
	before := fetchSession(t, mux, "")
	after := fetchSession(t, mux, "&deload_accepted=bench")
	for i, m := range before.Main {
		if m.ExerciseID == "bench" {
			continue
		}
		got := after.Main[i]
		if got.ExerciseID != m.ExerciseID {
			t.Fatalf("メインの構成が変わった: %s → %s", m.ExerciseID, got.ExerciseID)
		}
		if (m.WeightKg == nil) != (got.WeightKg == nil) {
			t.Errorf("%s の重量の確定状態が変わった", m.ExerciseID)
			continue
		}
		if m.WeightKg != nil && *m.WeightKg != *got.WeightKg {
			t.Errorf("承認していない %s の重量が変わった: %v → %v",
				m.ExerciseID, *m.WeightKg, *got.WeightKg)
		}
	}
}

// --- HTTP 境界の入力検証 ---

// ボディに上限があること。無認証のエンドポイントに巨大なボディを
// 投げるだけでメモリを食い潰せてはいけない。
func TestPostSetLogs_RejectsHugeBody(t *testing.T) {
	huge := strings.Repeat("a", 2<<20)
	body := `{"logs":[{"id":"a","date":"2026-08-17","exercise_id":"` + huge +
		`","weight_kg":85,"reps":8,"rir":2}]}`

	rec := do(t, newServer(t, true), http.MethodPost, "/api/set-logs", body)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("巨大なボディが 413 でない: %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() > 1024 {
		t.Errorf("エラー応答が入力を反射している: %dバイト", rec.Body.Len())
	}
}

// 上限内でも、入力をそのまま反射するエラー応答を返さないこと。
func TestErrors_DoNotReflectTheInput(t *testing.T) {
	long := strings.Repeat("b", 100_000)
	body := `{"logs":[{"id":"a","date":"2026-08-17","exercise_id":"` + long +
		`","weight_kg":85,"reps":8,"rir":2}]}`

	rec := do(t, newServer(t, true), http.MethodPost, "/api/set-logs", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ステータスが誤り: %d", rec.Code)
	}
	if rec.Body.Len() > 1024 {
		t.Errorf("エラー応答が入力を反射している: %dバイト", rec.Body.Len())
	}
}

// JSON の後ろに続くドキュメントを黙って捨てないこと。
// 捨てると、クライアントは保存されたと受け取ったまま実績が消える。
func TestPostSetLogs_RejectsTrailingData(t *testing.T) {
	one := `{"logs":[{"id":"t1","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":8,"rir":2}]}`
	for name, body := range map[string]string{
		"2つのドキュメント": one + one,
		"末尾のゴミ":     `{"logs":[]} これは JSON ではない`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, newServer(t, true), http.MethodPost, "/api/set-logs", body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("余分なデータが 400 でない: %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// Content-Type を検証すること。検証しないと、認証を入れた時点で
// form-urlencoded がプリフライト無しで送れて CSRF が通る。
func TestRequests_RequireJSONContentType(t *testing.T) {
	body := `{"logs":[]}`
	for name, ct := range map[string]string{
		"無し":         "",
		"text/plain": "text/plain",
		"フォーム":       "application/x-www-form-urlencoded",
		"text/html":  "text/html",
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/set-logs", strings.NewReader(body))
			if ct != "" {
				r.Header.Set("Content-Type", ct)
			}
			rec := httptest.NewRecorder()
			newServer(t, true).ServeHTTP(rec, r)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("Content-Type %q が通った: %d", ct, rec.Code)
			}
		})
	}

	// charset 付きは通す。
	r := httptest.NewRequest(http.MethodPost, "/api/set-logs", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Errorf("charset 付きが弾かれた: %d body=%s", rec.Code, rec.Body.String())
	}
}

// 種目マスタに無い種目のログを受け取らないこと。
func TestPostSetLogs_RejectsUnknownExercise(t *testing.T) {
	body := `{"logs":[{"id":"x","date":"2026-08-17","exercise_id":"存在しない種目",` +
		`"weight_kg":85,"reps":8,"rir":2}]}`
	rec := do(t, newServer(t, true), http.MethodPost, "/api/set-logs", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("未知の種目が 400 でない: %d body=%s", rec.Code, rec.Body.String())
	}
}

// 範囲外のコンディションを黙って捨てないこと。
// 取得口が無いので、捨てられるとクライアントは検知できない。
func TestPostConditions_RejectsOutOfRangeValues(t *testing.T) {
	for name, body := range map[string]string{
		"体重が負":    `{"conditions":[{"date":"2026-08-16","body_weight_kg":-500}]}`,
		"体重が巨大":   `{"conditions":[{"date":"2026-08-16","body_weight_kg":1e308}]}`,
		"睡眠が負":    `{"conditions":[{"date":"2026-08-16","sleep_hours":-3}]}`,
		"睡眠が1日超":  `{"conditions":[{"date":"2026-08-16","sleep_hours":480}]}`,
		"値が1つも無い": `{"conditions":[{"date":"2026-08-16"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := do(t, newServer(t, true), http.MethodPost, "/api/conditions", body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("範囲外の値が 400 でない: %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// 睡眠時間が HTTP から計画まで届くこと。
func TestPostConditions_SleepReachesThePlan(t *testing.T) {
	mux := newServer(t, true)

	var items []string
	for i := 1; i <= 14; i++ {
		date := training.MustDate(2026, time.August, 17).AddDays(-i)
		items = append(items, fmt.Sprintf(`{"date":"%s","sleep_hours":8}`, date.String()))
	}
	items = append(items, `{"date":"2026-08-17","sleep_hours":3}`)
	if rec := do(t, mux, http.MethodPost, "/api/conditions",
		`{"conditions":[`+strings.Join(items, ",")+`]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/sessions?date=2026-08-17", "")
	var got struct {
		Main []struct {
			Sets      int    `json:"sets"`
			TargetRIR int    `json:"target_rir"`
			Role      string `json:"role"`
		} `json:"main"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if len(got.Main) == 0 {
		t.Fatal("メイン種目が無い")
	}
	if got.Main[0].TargetRIR != 3 {
		t.Errorf("睡眠不足の補正が届いていない: target_rir=%d（期待 3）", got.Main[0].TargetRIR)
	}
	if got.Main[0].Sets != 4 {
		t.Errorf("セット数が誤り: %d（期待 4）", got.Main[0].Sets)
	}
	if got.Main[0].Role != "STANDARD" {
		t.Errorf("役割が載っていない: %q", got.Main[0].Role)
	}
}

// 応答の Content-Type が JSON であること。
func TestResponses_AreJSON(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions?date=2026-08-17", "")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type が誤り: %q", ct)
	}
}

// タイムアウトは 504。500 にすると、遅いだけの処理が
// サーバー障害として警報を上げる。
func TestGetSession_TimeoutIs504(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("タイムアウトが 504 でない: %d", rec.Code)
	}
}

// 書き込みも切断済みなら実行しないこと。
func TestWrites_StopOnClientDisconnect(t *testing.T) {
	for name, c := range map[string]struct{ path, body string }{
		"set-logs":   {"/api/set-logs", `{"logs":[{"id":"d","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":8,"rir":2}]}`},
		"conditions": {"/api/conditions", `{"conditions":[{"date":"2026-08-17","body_weight_kg":75}]}`},
		"program":    {"/api/program", `{"per_week":3,"weekly_target":{"QUAD":12},"selected_exercises":["squat"]}`},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			method := http.MethodPost
			if name == "program" {
				method = http.MethodPut
			}
			r := httptest.NewRequest(method, c.path, strings.NewReader(c.body)).WithContext(ctx)
			r.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newServer(t, true).ServeHTTP(rec, r)

			if rec.Code != 499 {
				t.Errorf("切断済みなのに %d を返した", rec.Code)
			}
		})
	}
}

// 取得もキャンセルの扱いを揃えること。
func TestGetProgram_ClientDisconnectIsNot200(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/program", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != 499 {
		t.Errorf("切断済みなのに %d を返した", rec.Code)
	}
}

// 解釈エラーで Go の型名・フィールド名を返さないこと。
// ユーザーが直せる情報ではない。
func TestDecodeError_DoesNotLeakGoTypes(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodPost, "/api/set-logs", `{"logs":123}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("ステータスが誤り: %d", rec.Code)
	}
	for _, leak := range []string{"httpapi", "setLogsRequest", "setLogDTO", "Go value", "struct field"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Errorf("内部の型名が漏れている（%q）: %s", leak, rec.Body.String())
		}
	}
}
