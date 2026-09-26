package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"time"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// newServer は認証を通したうえでルータを叩ける形にして返す。
//
// 認証ミドルウェアを噛ませるのは、そこが利用者を決める場所だから。
// 外すと、全てのハンドラテストが「利用者が届いていなくても緑」に
// なってしまう。ここを通しておけば、届かなくなった瞬間に全部が赤くなる。
func newServer(t *testing.T, configured bool) http.Handler {
	t.Helper()
	return authed(t, buildRoutes(t, configured))
}

// authed は各テストに Authorization を書かせずに認証を通す。
//
// 認証そのものの検査は session_auth_test.go が見ている。ここで個々の
// テストにヘッダを書かせると、検査したいこと（ハンドラの応答）から遠くなる。
//
// **本物のミドルウェアを通す。**素通しのラッパにすると、利用者が
// context に載らない経路をテストが作ってしまい、ハンドラが
// 受け取る利用者を誰も確かめていない状態になる。
func authed(t *testing.T, routes http.Handler) http.Handler {
	t.Helper()

	guarded := requireTestSession(t)(routes)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+sampleToken)
		guarded.ServeHTTP(w, r)
	})
}

// buildRoutes はミドルウェアを被せる前の生のルータを返す。
func buildRoutes(t *testing.T, configured bool) http.Handler {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	exercises := memory.NewExerciseRepository(pool)
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()

	programs := memory.NewProgramRepository()
	if configured {
		freq, _ := program.NewFrequency(3)
		selected := make([]exercise.ExerciseID, 0, len(pool))
		for _, e := range pool {
			selected = append(selected, e.ID())
		}
		program, err := program.NewProgram(freq, mustVolume(t, 6, 3), selected, []exercise.ExerciseID{"bench", "squat", "deadlift"}, "")
		if err != nil {
			t.Fatalf("プログラムが不正: %v", err)
		}
		// 保存先は既定ユーザー。ユースケースがいま使っているのと同じ
		// 利用者でないと、設定したはずのプログラムが読めない。
		if err := programs.Save(context.Background(), testUser, program); err != nil {
			t.Fatalf("プログラムの保存に失敗: %v", err)
		}
	}

	return routesFrom(t, dependencies(exercises, logs, conditions, programs))
}

// dependencies はリポジトリ一式から Dependencies を組む。
//
// 種目の読み口だけインターフェースで受けるのは、障害のテストが
// そこだけを壊れた実装に差し替えるため。差し替える1つが配線の8箇所に
// 現れるので、ここに寄せておかないと差し替えるたびに全部を書き写すことになる。
func dependencies(
	exercises exercise.Reader,
	logs *memory.SetLogRepository,
	conditions *memory.ConditionRepository,
	programs *memory.ProgramRepository,
) httpapi.Dependencies {
	return httpapi.Dependencies{
		GetSession:       usecase.NewGetSession(exercises, logs, conditions, programs, planning.DefaultSessionPlanner()),
		RecordSets:       usecase.NewRecordSets(logs, exercises),
		RecordConditions: usecase.NewRecordConditions(conditions),
		SetFocus:         usecase.NewSetFocusExercise(programs, programs),
		SetDeclared:      usecase.NewSetDeclaredExercises(exercises, programs, programs),
		SetFrequency:     usecase.NewSetFrequency(programs, programs),
		SetVolume:        usecase.NewSetSessionVolume(programs, programs),
		SetSelected:      usecase.NewSetSelectedExercises(exercises, programs, programs),
		SetSplit:         usecase.NewSetSplitCycle(exercises, programs, programs),
		GetProgram:       usecase.NewGetProgram(programs),
		DeleteSetLog:     usecase.NewDeleteSetLog(logs),
		Exercises:        query.NewExercises(exercises),
		History:          query.NewHistory(logs, exercises),
		Stats:            query.NewStats(logs, exercises, programs, planning.DefaultOneRepMaxEstimator()),
	}
}

// routesFrom は Dependencies からミドルウェアを被せる前の生のルータを組む。
func routesFrom(t *testing.T, d httpapi.Dependencies) http.Handler {
	t.Helper()

	handler, err := httpapi.NewHandler(d)
	if err != nil {
		t.Fatalf("ハンドラが組めない: %v", err)
	}
	return handler.Routes()
}

// 依存が1つでも欠けていたら、組み立ての時点で止める。
//
// 位置引数で受けていたときは、渡し忘れはコンパイルエラーだった。
// 構造体はゼロ値で通るので、検査が無いと、欠けた口が初めて叩かれた
// ときに nil 参照で落ちる。起動は成功しているので、気づくのは利用者になる。
//
// フィールドをリフレクションで回すのは、Dependencies に口を足したときに
// この検査が自動で付いてくるようにするため。手で並べると、足した人が
// NewHandler の検査とこの表の両方を忘れても緑のままになる。
//
// 名前がエラーに載ることまで見るのは、「何かが欠けている」だけでは
// 14個のどれを直せばよいか分からないから。
func TestNewHandler_RejectsMissingDependency(t *testing.T) {
	complete := dependencies(
		memory.NewExerciseRepository(nil),
		memory.NewSetLogRepository(),
		memory.NewConditionRepository(),
		memory.NewProgramRepository(),
	)
	// 揃っていれば通ること。これが無いと、常にエラーを返す実装でも
	// 下の検査が全部緑になる。
	if _, err := httpapi.NewHandler(complete); err != nil {
		t.Fatalf("全部揃っているのに組めない: %v", err)
	}

	fields := reflect.TypeOf(complete)
	for i := range fields.NumField() {
		field := fields.Field(i)
		t.Run(field.Name+" が欠けている", func(t *testing.T) {
			d := complete
			reflect.ValueOf(&d).Elem().Field(i).Set(reflect.Zero(field.Type))

			_, err := httpapi.NewHandler(d)
			if err == nil {
				t.Fatalf("%s が nil なのにエラーにならない", field.Name)
			}
			if !strings.Contains(err.Error(), field.Name) {
				t.Errorf("エラーに欠けた名前 %s が載っていない: %v", field.Name, err)
			}
		})
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
		} `json:"main"`
		Accessories []struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"accessories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}

	if body.Date != "2026-08-17" {
		t.Errorf("日付が誤り: %s", body.Date)
	}
	if len(body.Main) != 1 {
		t.Errorf("ヘビー枠が1つでない: %d", len(body.Main))
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

// 3レーンが応答の3つのキーに対応すること。
//
// PWA は session.variation を読む（Today.tsx）。ここが落ちるとバリエーション
// レーンが画面から消えるが、Go 側は何も壊れないので気づけない。
//
// 出ない日も null ではなく空配列であることを一緒に見る。null だと
// TypeScript 側の `?? []` を通っても .map で落ちる形になりやすい。
func TestGetSession_HasThreeLanes(t *testing.T) {
	mux := newServer(t, true)

	// 重点種目をベンチにする。指定しないとバリエーションレーンは出ない。
	//
	// 元は全置換の PUT /api/program で1度に組んでいた（#123 で消した）。
	// 狭い口で同じ状態を組む。頻度と宣言は newServer の既定（週3・BIG3）が
	// 元の前提と同じなので送らない。
	for _, step := range []struct{ path, body string }{
		{"/api/program/selected", `{"selected_exercises":["bench","squat","deadlift","larsen_press","tempo_bench"]}`},
		{"/api/program/focus", `{"focus_exercise":"bench"}`},
	} {
		if rec := do(t, mux, http.MethodPut, step.path, step.body); rec.Code != http.StatusNoContent {
			t.Fatalf("%s の保存に失敗: %d body=%s", step.path, rec.Code, rec.Body.String())
		}
	}

	// ベンチを3日前にやって軸を他へ移す。当日と前日は「中1日」の門に
	// 掛かるので、3日前にする。
	logs := `{"logs":[{"id":"b1","date":"2026-08-14","exercise_id":"bench",` +
		`"weight_kg":85,"reps":8,"rir":2}]}`
	if rec := do(t, mux, http.MethodPost, "/api/set-logs", logs); rec.Code != http.StatusNoContent {
		t.Fatalf("記録の保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/sessions?date=2026-08-17", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	// ポインタで受けて、キーの欠落と空配列を区別する。
	var got struct {
		Main *[]struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"main"`
		Variation *[]struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"variation"`
		Accessory *[]struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"accessories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}

	for name, lane := range map[string]*[]struct {
		ExerciseID string `json:"exercise_id"`
	}{"main": got.Main, "variation": got.Variation, "accessories": got.Accessory} {
		if lane == nil {
			t.Errorf("%s のキーが無いか null。空でも配列で返すこと", name)
		}
	}
	if got.Variation == nil {
		t.FailNow()
	}
	if len(*got.Variation) != 1 {
		t.Fatalf("バリエーションが1件でない: %v（軸 %v）", *got.Variation, *got.Main)
	}
	if id := (*got.Variation)[0].ExerciseID; id != "larsen_press" && id != "tempo_bench" {
		t.Errorf("バリエーションがベンチの派生でない: %s", id)
	}

	// 重点種目を指定しなければバリエーションは出ない。そのときも
	// null ではなく空配列で返すこと。ここが null だと、Today.tsx の
	// `?? []` は通るが、キーを消したときと区別が付かなくなる。
	empty := do(t, newServer(t, true), http.MethodGet, "/api/sessions?date=2026-08-17", "")
	if empty.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d", empty.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(empty.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	v, ok := raw["variation"]
	if !ok {
		t.Fatal("重点種目なしで variation のキーが消えている")
	}
	if string(v) != "[]" {
		t.Errorf("重点種目なしの variation が %s。空配列のはず", v)
	}
}

// putUpperLowerSplit は上下2分割を保存する。
//
// 「ほかを動かさない」を検査するテストは、これを呼んでから始める。
// シードのプログラムは分割なしなので、呼ばないと splits が前後とも []
// で一致し、分割が消えても緑のまま通る（#122 はそれで見逃した）。
func putUpperLowerSplit(t *testing.T, mux http.Handler) {
	t.Helper()
	body := `{"splits":[` +
		`{"name":"上半身","regions":["CHEST_MID","LAT","FRONT_DELT","TRICEPS_LATERAL","BICEPS"]},` +
		`{"name":"下半身","regions":["QUAD","HAMSTRING","GLUTE","ERECTOR"]}` +
		`]}`
	if rec := do(t, mux, http.MethodPut, "/api/program/split", body); rec.Code != http.StatusNoContent {
		t.Fatalf("分割の保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
}

// 重点種目だけの口は、本当に重点種目だけを動かすこと。
//
// この口を足した理由そのもの（D-127）。全置換の PUT /api/program を
// クライアントに使わせないのは、週目標や選択種目が往復する経路を作らない
// ためなので、ここが守られていないと分けた意味が消える。
func TestPutProgramFocus_TouchesNothingElse(t *testing.T) {
	mux := newServer(t, true)
	putUpperLowerSplit(t, mux)

	before := do(t, mux, http.MethodGet, "/api/program", "")
	if before.Code != http.StatusOK {
		t.Fatalf("取得に失敗: %d", before.Code)
	}

	if rec := do(t, mux, http.MethodPut, "/api/program/focus",
		`{"focus_exercise":"bench"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	after := do(t, mux, http.MethodGet, "/api/program", "")
	if after.Code != http.StatusOK {
		t.Fatalf("取得に失敗: %d", after.Code)
	}

	// 生の JSON で比べる。構造体に写すと、写し忘れたフィールドが
	// 変わっていても気づけない。
	var b, a map[string]json.RawMessage
	if err := json.Unmarshal(before.Body.Bytes(), &b); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if err := json.Unmarshal(after.Body.Bytes(), &a); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}

	if len(a) != len(b) {
		t.Errorf("フィールドの数が変わった: %d → %d", len(b), len(a))
	}
	for k, want := range b {
		if k == "focus_exercise" {
			continue
		}
		if string(a[k]) != string(want) {
			t.Errorf("%s が変わった: %s → %s", k, want, a[k])
		}
	}
	if string(a["focus_exercise"]) != `"bench"` {
		t.Errorf("重点種目が %s。\"bench\" のはず", a["focus_exercise"])
	}
}

// null で指定を解除できること。解除できないと、一度指定したら
// バリエーションレーンを止める手段がアプリの中に無くなる。
func TestPutProgramFocus_NullClearsIt(t *testing.T) {
	mux := newServer(t, true)

	if rec := do(t, mux, http.MethodPut, "/api/program/focus",
		`{"focus_exercise":"bench"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d", rec.Code)
	}
	if rec := do(t, mux, http.MethodPut, "/api/program/focus",
		`{"focus_exercise":null}`); rec.Code != http.StatusNoContent {
		t.Fatalf("解除に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/program", "")
	var got struct {
		Focus *string `json:"focus_exercise"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if got.Focus != nil {
		t.Errorf("解除できていない: %v", *got.Focus)
	}
}

func TestPutProgramFocus_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		configured bool
		body       string
		want       int
	}{
		{
			// 宣言していない種目を重点にできると、「伸ばしたい種目の中で
			// さらに重点」という意味が崩れる。
			name: "宣言していない種目", configured: true,
			body: `{"focus_exercise":"leg_press"}`, want: http.StatusBadRequest,
		},
		{
			name: "存在しない種目", configured: true,
			body: `{"focus_exercise":"nonexistent"}`, want: http.StatusBadRequest,
		},
		{
			// 未設定は「前提が満たされていない」なので 409。
			// GET /api/program の 404 とは意味が違う（D-042）。
			name: "プログラムが未設定", configured: false,
			body: `{"focus_exercise":"bench"}`, want: http.StatusConflict,
		},
		{
			// ほかの設定を一緒に送ってきたものを黙って受けない。受けて
			// 捨てると、送った側は頻度も変わったと思い込む。
			name: "余計なフィールド", configured: true,
			body: `{"focus_exercise":"bench","per_week":4}`, want: http.StatusBadRequest,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, c.configured), http.MethodPut,
				"/api/program/focus", c.body)
			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// 伸ばしたい種目だけの口も、それだけを動かすこと。
func TestPutProgramDeclared_TouchesNothingElse(t *testing.T) {
	cases := []struct {
		name string
		// setFocus が空でなければ、先に重点種目を立てておく。
		setFocus string
		body     string
		want     string
	}{
		{
			name: "重点種目はそのまま残る",
			// bench は宣言に残すので、重点種目を触らずに済む。
			setFocus: "bench",
			body:     `{"declared_exercises":["bench","squat"]}`,
			want:     `["bench","squat"]`,
		},
		{
			name: "重点種目なしでも通る",
			body: `{"declared_exercises":["squat"]}`,
			want: `["squat"]`,
		},
		{
			// 順序は集約が昇順に正規化する。送った順は残らない。
			name: "並び替えて送っても昇順に戻る",
			body: `{"declared_exercises":["squat","bench","deadlift"]}`,
			want: `["bench","deadlift","squat"]`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mux := newServer(t, true)
			// 分割が先、重点が後。逆だと、分割の口が重点を落としたときに
			// before が「重点なし」になり、前後が一致して通る（#132）。
			// この順で逆に分割が落ちる場合は、重点の口のテストが見る。
			putUpperLowerSplit(t, mux)
			if c.setFocus != "" {
				if rec := do(t, mux, http.MethodPut, "/api/program/focus",
					`{"focus_exercise":"`+c.setFocus+`"}`); rec.Code != http.StatusNoContent {
					t.Fatalf("重点種目の保存に失敗: %d", rec.Code)
				}
			}

			before := do(t, mux, http.MethodGet, "/api/program", "")
			if rec := do(t, mux, http.MethodPut, "/api/program/declared",
				c.body); rec.Code != http.StatusNoContent {
				t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
			}
			after := do(t, mux, http.MethodGet, "/api/program", "")

			// 生の JSON で比べる。構造体に写すと、写し忘れたフィールドが
			// 変わっていても気づけない。
			var b, a map[string]json.RawMessage
			if err := json.Unmarshal(before.Body.Bytes(), &b); err != nil {
				t.Fatalf("JSONが壊れている: %v", err)
			}
			if err := json.Unmarshal(after.Body.Bytes(), &a); err != nil {
				t.Fatalf("JSONが壊れている: %v", err)
			}
			if len(a) != len(b) {
				t.Errorf("フィールドの数が変わった: %d → %d", len(b), len(a))
			}
			for k, want := range b {
				if k == "declared_exercises" {
					continue
				}
				if string(a[k]) != string(want) {
					t.Errorf("%s が変わった: %s → %s", k, want, a[k])
				}
			}
			if string(a["declared_exercises"]) != c.want {
				t.Errorf("伸ばしたい種目が %s。%s のはず", a["declared_exercises"], c.want)
			}
		})
	}
}

func TestPutProgramDeclared_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		configured bool
		setFocus   string
		body       string
		want       int
	}{
		{
			// 黙って重点を解除しない。宣言を変えた副作用で重点が消えると、
			// 次に画面を開くまで気づけない。
			name: "重点種目が宣言から外れる", configured: true, setFocus: "bench",
			body: `{"declared_exercises":["squat"]}`, want: http.StatusBadRequest,
		},
		{
			name: "宣言が空", configured: true,
			body: `{"declared_exercises":[]}`, want: http.StatusBadRequest,
		},
		{
			name: "選択に無い種目", configured: true,
			body: `{"declared_exercises":["nonexistent"]}`, want: http.StatusBadRequest,
		},
		{
			name: "重複", configured: true,
			body: `{"declared_exercises":["bench","bench"]}`, want: http.StatusBadRequest,
		},
		{
			name: "プログラムが未設定", configured: false,
			body: `{"declared_exercises":["bench"]}`, want: http.StatusConflict,
		},
		{
			name: "余計なフィールド", configured: true,
			body: `{"declared_exercises":["bench"],"per_week":4}`, want: http.StatusBadRequest,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mux := newServer(t, c.configured)
			if c.setFocus != "" {
				do(t, mux, http.MethodPut, "/api/program/focus",
					`{"focus_exercise":"`+c.setFocus+`"}`)
			}
			rec := do(t, mux, http.MethodPut, "/api/program/declared", c.body)
			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// 分割を設定したあとで、どの日にも出られない種目を宣言に足せないこと。
//
// 分割の口（PUT /api/program/split）は同じ状態を弾いている。宣言の口が
// 見ていないと、順序を入れ替えるだけで同じ状態を作れる（#140）。出られない
// 宣言種目は毎日「今日の候補ではない」と判定され、エラーも立たないまま
// 二度と軸に出ない。
//
// シードとプリセットだけで踏める。ケーブルクランチの主働は腹直筋（1.0）で、
// 腹直筋と腹斜筋はどのプリセットのどの日にも入っていない。プリセットは
// 手で書き写さず GET /api/split-presets から取る。書き写すと、テストが
// 自前の周期だけで完結し、プリセットを変えても緑のままになる。取って
// くれば、プリセットに腹を足した日にここが赤くなり、種目を選び直せる。
func TestPutProgramDeclared_RejectsExerciseWithoutADay(t *testing.T) {
	mux := newServer(t, true)

	rec := do(t, mux, http.MethodGet, "/api/split-presets", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("プリセットの取得に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	var presets struct {
		Presets []struct {
			Key    string          `json:"key"`
			Splits json.RawMessage `json:"splits"`
		} `json:"presets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &presets); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}

	for _, p := range presets.Presets {
		t.Run(p.Key, func(t *testing.T) {
			mux := newServer(t, true)
			if rec := do(t, mux, http.MethodPut, "/api/program/split",
				`{"splits":`+string(p.Splits)+`}`); rec.Code != http.StatusNoContent {
				t.Fatalf("分割の保存に失敗: %d body=%s", rec.Code, rec.Body.String())
			}

			rec := do(t, mux, http.MethodPut, "/api/program/declared",
				`{"declared_exercises":["bench","squat","deadlift","cable_crunch"]}`)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("ステータスが %d。400 のはず: %s", rec.Code, rec.Body.String())
			}

			// 弾いたなら保存もされていないこと。400 を返しつつ保存していると、
			// 画面はエラーを出すのに設定は壊れた状態になる。
			var got struct {
				Declared []string `json:"declared_exercises"`
			}
			after := do(t, mux, http.MethodGet, "/api/program", "")
			if err := json.Unmarshal(after.Body.Bytes(), &got); err != nil {
				t.Fatalf("JSONが壊れている: %v", err)
			}
			if slices.Contains(got.Declared, "cable_crunch") {
				t.Errorf("出られない種目が宣言に入った: %v", got.Declared)
			}
		})
	}

	// 分割なしなら通る。分割で候補を絞らないので、出られない種目が無い。
	// ここが 400 になるなら、分割と無関係にこの種目を弾いている。
	if rec := do(t, mux, http.MethodPut, "/api/program/declared",
		`{"declared_exercises":["bench","squat","deadlift","cable_crunch"]}`); rec.Code != http.StatusNoContent {
		t.Errorf("分割なしで通らない: %d %s", rec.Code, rec.Body.String())
	}
}

// 頻度の口は、頻度だけを動かすこと。
//
// 以前は「頻度と週目標だけを動かす」ことを見ていた。週目標は
// 応答（GET /api/program）に含めていたので、頻度に追従して動くことを
// レスポンス越しに確認できた。週目標は応答から消えた（#176）。計画と
// 画面の充足は Frequency() と SessionVolume() から都度組み直すので、
// 追従は保存された値でなくその都度の導出で保証される。導出そのものは
// seed.DefaultWeeklyTarget の直接のテストと TestSimulation が見る。
func TestPutProgramFrequency_ChangesOnlyFrequency(t *testing.T) {
	mux := newServer(t, true)
	putUpperLowerSplit(t, mux)

	before := do(t, mux, http.MethodGet, "/api/program", "")
	if rec := do(t, mux, http.MethodPut, "/api/program/frequency",
		`{"per_week":4}`); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	after := do(t, mux, http.MethodGet, "/api/program", "")

	var b, a map[string]json.RawMessage
	if err := json.Unmarshal(before.Body.Bytes(), &b); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if err := json.Unmarshal(after.Body.Bytes(), &a); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(a) != len(b) {
		t.Errorf("フィールドの数が変わった: %d → %d", len(b), len(a))
	}
	for k, want := range b {
		if k == "per_week" {
			continue
		}
		if string(a[k]) != string(want) {
			t.Errorf("%s が変わった: %s → %s", k, want, a[k])
		}
	}
	if string(a["per_week"]) != "4" {
		t.Errorf("頻度が %s。4 のはず", a["per_week"])
	}
}

// 1回の量の口は、1回の量だけを動かすこと。理由は頻度の口と同じ。
func TestPutProgramVolume_ChangesOnlyVolume(t *testing.T) {
	mux := newServer(t, true)

	before := do(t, mux, http.MethodGet, "/api/program", "")
	if rec := do(t, mux, http.MethodPut, "/api/program/volume",
		`{"exercises_per_session":5,"sets_per_exercise":4}`); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	after := do(t, mux, http.MethodGet, "/api/program", "")

	var b, a map[string]json.RawMessage
	if err := json.Unmarshal(before.Body.Bytes(), &b); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if err := json.Unmarshal(after.Body.Bytes(), &a); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	moved := map[string]bool{"exercises_per_session": true, "sets_per_exercise": true}
	for k, want := range b {
		if moved[k] {
			continue
		}
		if string(a[k]) != string(want) {
			t.Errorf("%s が変わった: %s → %s", k, want, a[k])
		}
	}
	if string(a["exercises_per_session"]) != "5" || string(a["sets_per_exercise"]) != "4" {
		t.Errorf("1回の量が %s種目×%sセット。5×4 のはず",
			a["exercises_per_session"], a["sets_per_exercise"])
	}
}

func TestPutProgramVolume_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		configured bool
		body       string
		want       int
	}{
		{"種目数が下限未満", true, `{"exercises_per_session":1,"sets_per_exercise":3}`, http.StatusBadRequest},
		{"種目数が上限超え", true, `{"exercises_per_session":7,"sets_per_exercise":3}`, http.StatusBadRequest},
		{"セット数が下限未満", true, `{"exercises_per_session":4,"sets_per_exercise":1}`, http.StatusBadRequest},
		{"セット数が上限超え", true, `{"exercises_per_session":4,"sets_per_exercise":7}`, http.StatusBadRequest},
		{"片方が欠けている", true, `{"exercises_per_session":4}`, http.StatusBadRequest},
		{"プログラムが未設定", false, `{"exercises_per_session":4,"sets_per_exercise":3}`, http.StatusConflict},
		{"余計なフィールド", true, `{"exercises_per_session":4,"sets_per_exercise":3,"per_week":3}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, c.configured), http.MethodPut,
				"/api/program/volume", c.body)
			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func TestPutProgramFrequency_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		configured bool
		body       string
		want       int
	}{
		{"0回", true, `{"per_week":0}`, http.StatusBadRequest},
		{"負", true, `{"per_week":-1}`, http.StatusBadRequest},
		{"上限超え", true, `{"per_week":8}`, http.StatusBadRequest},
		{"プログラムが未設定", false, `{"per_week":3}`, http.StatusConflict},
		{"余計なフィールド", true, `{"per_week":3,"focus_exercise":"bench"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, c.configured), http.MethodPut,
				"/api/program/frequency", c.body)
			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// 使う種目の口は、それだけを動かすこと。
func TestPutProgramSelected_TouchesNothingElse(t *testing.T) {
	mux := newServer(t, true)
	putUpperLowerSplit(t, mux)

	before := do(t, mux, http.MethodGet, "/api/program", "")
	// 宣言の3種目は残したまま、それ以外を絞る。脚のプレスを1つ残すのは
	// 週目標のどの区分も刺激しない選択を作らないため。
	body := `{"selected_exercises":["bench","squat","deadlift","leg_press"]}`
	if rec := do(t, mux, http.MethodPut, "/api/program/selected",
		body); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	after := do(t, mux, http.MethodGet, "/api/program", "")

	var b, a map[string]json.RawMessage
	if err := json.Unmarshal(before.Body.Bytes(), &b); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if err := json.Unmarshal(after.Body.Bytes(), &a); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(a) != len(b) {
		t.Errorf("フィールドの数が変わった: %d → %d", len(b), len(a))
	}
	for k, want := range b {
		if k == "selected_exercises" {
			continue
		}
		if string(a[k]) != string(want) {
			t.Errorf("%s が変わった: %s → %s", k, want, a[k])
		}
	}
	// 昇順に正規化される。
	if want := `["bench","deadlift","leg_press","squat"]`; string(a["selected_exercises"]) != want {
		t.Errorf("使う種目が %s。%s のはず", a["selected_exercises"], want)
	}
}

func TestPutProgramSelected_Rejects(t *testing.T) {
	cases := []struct {
		name       string
		configured bool
		body       string
		want       int
	}{
		{
			// 黙って宣言を削らない。軸の顔ぶれが変わったことに
			// 次のセッションまで気づけない。
			name: "伸ばしたい種目が外れる", configured: true,
			body: `{"selected_exercises":["bench","squat"]}`, want: http.StatusBadRequest,
		},
		{
			name: "存在しない種目", configured: true,
			body: `{"selected_exercises":["bench","squat","deadlift","nonexistent"]}`,
			want: http.StatusBadRequest,
		},
		{"空", true, `{"selected_exercises":[]}`, http.StatusBadRequest},
		{
			name: "重複", configured: true,
			body: `{"selected_exercises":["bench","bench","squat","deadlift"]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "プログラムが未設定", configured: false,
			body: `{"selected_exercises":["bench"]}`, want: http.StatusConflict,
		},
		{
			name: "余計なフィールド", configured: true,
			body: `{"selected_exercises":["bench"],"per_week":4}`, want: http.StatusBadRequest,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, c.configured), http.MethodPut,
				"/api/program/selected", c.body)
			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// 分割の口は、分割だけを動かすこと。
func TestPutProgramSplit_TouchesNothingElse(t *testing.T) {
	mux := newServer(t, true)

	before := do(t, mux, http.MethodGet, "/api/program", "")
	body := `{"splits":[` +
		`{"name":"上半身","regions":["CHEST_MID","LAT","FRONT_DELT","TRICEPS_LATERAL","BICEPS"]},` +
		`{"name":"下半身","regions":["QUAD","HAMSTRING","GLUTE","ERECTOR"]}` +
		`]}`
	if rec := do(t, mux, http.MethodPut, "/api/program/split", body); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	after := do(t, mux, http.MethodGet, "/api/program", "")

	var b, a map[string]json.RawMessage
	if err := json.Unmarshal(before.Body.Bytes(), &b); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if err := json.Unmarshal(after.Body.Bytes(), &a); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(a) != len(b) {
		t.Errorf("フィールドの数が変わった: %d → %d", len(b), len(a))
	}
	for k, want := range b {
		if k == "splits" {
			continue
		}
		if string(a[k]) != string(want) {
			t.Errorf("%s が変わった: %s → %s", k, want, a[k])
		}
	}

	var got []struct {
		Name    string   `json:"name"`
		Regions []string `json:"regions"`
	}
	if err := json.Unmarshal(a["splits"], &got); err != nil {
		t.Fatalf("分割が壊れている: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("分割が %d 件。2件のはず: %v", len(got), got)
	}
	// 順序が周期そのもの。並びが保たれること。
	if got[0].Name != "上半身" || got[1].Name != "下半身" {
		t.Errorf("周期の並びが変わっている: %v", got)
	}
	// 区分は昇順に正規化される。
	if want := []string{"BICEPS", "CHEST_MID", "FRONT_DELT", "LAT", "TRICEPS_LATERAL"}; !slices.Equal(got[0].Regions, want) {
		t.Errorf("区分が %v。%v のはず", got[0].Regions, want)
	}
}

// 空を送れば分割なしに戻せること。
func TestPutProgramSplit_EmptyClearsIt(t *testing.T) {
	mux := newServer(t, true)

	full := `{"splits":[{"name":"上半身","regions":["CHEST_MID","LAT"]},` +
		`{"name":"下半身","regions":["QUAD","HAMSTRING","GLUTE","ERECTOR"]}]}`
	if rec := do(t, mux, http.MethodPut, "/api/program/split", full); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	if rec := do(t, mux, http.MethodPut, "/api/program/split", `{"splits":[]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("解除に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/program", "")
	var got struct {
		Splits []json.RawMessage `json:"splits"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(got.Splits) != 0 {
		t.Errorf("解除できていない: %v", got.Splits)
	}
}

// 「主働」は寄与 1.0 以上。副次的にかすっているだけでは、その日に
// 出られるとみなさない。
//
// デッドリフトは臀筋 0.8・ハムストリング 1.0・脊柱起立筋 1.0。臀筋だけの
// 分割では出られない。閾値を下げると 0.8 が主働に化け、実際には軸に
// 選ばれないのに設定だけ通る。
func TestPutProgramSplit_PrimaryMeansFullContribution(t *testing.T) {
	mux := newServer(t, true)

	// 宣言をスクワットとデッドリフトに絞る。スクワットは大腿四頭筋 1.0
	// なので、どちらの周期でも出られる。
	if rec := do(t, mux, http.MethodPut, "/api/program/declared",
		`{"declared_exercises":["squat","deadlift"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("宣言の保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	// 臀筋と大腿四頭筋だけの周期。デッドリフトの主働（ハム・脊柱起立筋）は
	// どちらにも入っていない。臀筋 0.8 は主働ではないので弾かれる。
	rec := do(t, mux, http.MethodPut, "/api/program/split",
		`{"splits":[{"name":"脚","regions":["QUAD","GLUTE"]}]}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("ステータスが %d。400 のはず: %s", rec.Code, rec.Body.String())
	}

	// ハムストリングを足せば通る。
	rec = do(t, mux, http.MethodPut, "/api/program/split",
		`{"splits":[{"name":"脚","regions":["QUAD","GLUTE","HAMSTRING"]}]}`)
	if rec.Code != http.StatusNoContent {
		t.Errorf("主働を含めても通らない: %d %s", rec.Code, rec.Body.String())
	}
}

func TestPutProgramSplit_Rejects(t *testing.T) {
	// 宣言は bench/squat/deadlift。胸と脚しか無い周期を送ると、
	// デッドリフト（ハム・脊柱起立筋）が出られる日を失う。
	cases := []struct {
		name string
		body string
		want int
	}{
		{
			name: "宣言種目が出られる日が無い",
			body: `{"splits":[{"name":"胸","regions":["CHEST_MID"]},` +
				`{"name":"脚","regions":["QUAD"]}]}`,
			want: http.StatusBadRequest,
		},
		{"名前が空", `{"splits":[{"name":"","regions":["QUAD"]}]}`, http.StatusBadRequest},
		{"存在しない区分", `{"splits":[{"name":"謎","regions":["NOSUCH"]}]}`, http.StatusBadRequest},
		{
			name: "同じ区分が重複",
			body: `{"splits":[{"name":"脚","regions":["QUAD","QUAD"]}]}`,
			want: http.StatusBadRequest,
		},
		{"余計なフィールド", `{"splits":[],"per_week":4}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, true), http.MethodPut, "/api/program/split", c.body)
			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

// プリセットが一覧で取れること。
func TestGetSplitPresets(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/split-presets", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d", rec.Code)
	}

	var got struct {
		Presets []struct {
			Key    string `json:"key"`
			Name   string `json:"name"`
			Splits []struct {
				Name    string   `json:"name"`
				Regions []string `json:"regions"`
			} `json:"splits"`
		} `json:"presets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(got.Presets) < 3 {
		t.Fatalf("プリセットが %d 件。3件以上のはず", len(got.Presets))
	}

	keys := map[string]int{}
	for _, p := range got.Presets {
		if p.Key == "" || p.Name == "" {
			t.Errorf("キーか名前が空: %+v", p)
		}
		keys[p.Key] = len(p.Splits)
	}
	for key, want := range map[string]int{
		"upper_lower": 2, "ppl": 3, "five_way": 5,
	} {
		if got := keys[key]; got != want {
			t.Errorf("%s の日数が %d。%d のはず", key, got, want)
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

	// 宣言した3種目すべてに記録する。ヘビー枠は「最後にやったのが最も
	// 古い種目」で、一度もやっていない種目が最優先になるので、ベンチだけ
	// 記録すると未実施のスクワットやデッドリフトが軸に来て、重量が
	// 確定しないまま返る。
	payload := `{"logs":[` +
		`{"id":"01J-A","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":9,"rir":2},` +
		`{"id":"01J-B","date":"2026-08-17","exercise_id":"squat","weight_kg":110,"reps":9,"rir":2},` +
		`{"id":"01J-C","date":"2026-08-17","exercise_id":"deadlift","weight_kg":140,"reps":9,"rir":2}]}`
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
	if len(body.Main) != 1 {
		t.Fatalf("ヘビー枠が1つでない: %d", len(body.Main))
	}
	if body.Main[0].WeightKg == nil {
		t.Errorf("記録したログが重量算出に反映されていない: %s",
			body.Main[0].ExerciseID)
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

// 入力の不正は、どの口から返っても同じ形（code が INVALID_INPUT）である。
//
// 元は date の検査と set-logs / conditions の中身の検査だけが別の経路で
// 400 を返していて、code が空文字だった。画面が code で分岐し始めると、
// 同じ「入力が不正」なのに口によって拾えたり拾えなかったりする。
// 1行が handler.go の拒否1箇所に対応する。
func TestBadInput_IsCodedAsInvalidInput(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "sessions に date が無い", method: http.MethodGet, path: "/api/sessions"},
		{name: "sessions の date が日付でない", method: http.MethodGet, path: "/api/sessions?date=2026/08/17"},
		{
			name: "set-logs の date が日付でない", method: http.MethodPost, path: "/api/set-logs",
			body: `{"logs":[{"id":"01J-B","date":"2026/08/17","exercise_id":"bench","weight_kg":85,"reps":9,"rir":2}]}`,
		},
		{
			// rir を落とす。0 と「送られていない」を区別するための必須検査。
			name: "set-logs の必須項目が欠けている", method: http.MethodPost, path: "/api/set-logs",
			body: `{"logs":[{"id":"01J-B","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":9}]}`,
		},
		{
			// 0レップはドメインが拒む。
			name: "set-logs の値をドメインが拒む", method: http.MethodPost, path: "/api/set-logs",
			body: `{"logs":[{"id":"01J-B","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":0,"rir":2}]}`,
		},
		{
			name: "conditions の date が日付でない", method: http.MethodPost, path: "/api/conditions",
			body: `{"conditions":[{"date":"2026/08/17","body_weight_kg":75}]}`,
		},
		{
			// 体重の上限は 300kg。
			name: "conditions の体重が範囲外", method: http.MethodPost, path: "/api/conditions",
			body: `{"conditions":[{"date":"2026-08-17","body_weight_kg":301}]}`,
		},
		{
			// 睡眠の上限は 24時間。
			name: "conditions の睡眠時間が範囲外", method: http.MethodPost, path: "/api/conditions",
			body: `{"conditions":[{"date":"2026-08-17","sleep_hours":25}]}`,
		},
		{
			name: "conditions に体重も睡眠も無い", method: http.MethodPost, path: "/api/conditions",
			body: `{"conditions":[{"date":"2026-08-17"}]}`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, true), c.method, c.path, c.body)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("JSONが壊れている: %v", err)
			}
			if body.Code != "INVALID_INPUT" {
				t.Errorf("code が %q。INVALID_INPUT のはず: %s", body.Code, rec.Body.String())
			}
		})
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

// 保存先に到達できないときは 503。500 と混ぜない。
//
// 後で送り直せば通るものを 500 で返すと、待ち行列が「送り直しても無駄」と
// 読んで記録を捨てる。分類を消しても 500 で緑になるので、独立に見る。
type unavailableExercises struct{}

func (unavailableExercises) FindAll(context.Context) ([]*exercise.Exercise, error) {
	return nil, fmt.Errorf("種目の取得: %w: dial tcp 10.0.0.1:5432: connect: refused",
		training.ErrRepositoryUnavailable)
}

func TestGetSession_UnavailableIsNot500(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, _ := program.NewFrequency(3)
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	prog, err := program.NewProgram(freq, mustVolume(t, 6, 3), selected,
		[]exercise.ExerciseID{"bench", "squat", "deadlift"}, "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	programs := memory.NewProgramRepository()
	if err := programs.Save(context.Background(), testUser, prog); err != nil {
		t.Fatalf("プログラムの保存に失敗: %v", err)
	}

	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	mux := routesFrom(t, dependencies(unavailableExercises{}, logs, conditions, programs))
	// 認証を通す。利用者を決めるのはミドルウェアなので、
	// 生のルータを叩くと「利用者が無い」で 500 になり、
	// ここで見たい分類が見えない。
	mux = authed(t, mux)

	rec := do(t, mux, http.MethodGet, "/api/sessions?date=2026-08-17", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("ステータスが %d。503 のはず: %s", rec.Code, rec.Body.String())
	}
	// 接続文字列は外に出さない。
	if strings.Contains(rec.Body.String(), "10.0.0.1") {
		t.Errorf("接続先が漏れている: %s", rec.Body.String())
	}
}

// 500 のときに内部のエラー文を返さないこと。
// ドメインのエラーには種目IDや閾値が載っており、外に出す理由がない。
func TestErrors_DoNotLeakInternals(t *testing.T) {
	mux := newServer(t, true)
	rec := do(t, mux, http.MethodPut, "/api/program/selected",
		`{"selected_exercises":["bench","squat","deadlift","無い種目"]}`)
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

	// 元は全置換の PUT /api/program で1度に設定していた（#123 で消した）。
	// 未設定から設定する口はもう無い（初期プログラムはサインイン時に入る）
	// ので、設定済みのサーバーに狭い口を順に当てて同じ状態を組む。
	mux = newServer(t, true)
	for _, step := range []struct{ path, body string }{
		{"/api/program/frequency", `{"per_week":2}`},
		{"/api/program/volume", `{"exercises_per_session":5,"sets_per_exercise":4}`},
		{"/api/program/selected", `{"selected_exercises":["bench","squat","deadlift","incline_db_press"]}`},
		{"/api/program/focus", `{"focus_exercise":"bench"}`},
	} {
		if rec := do(t, mux, http.MethodPut, step.path, step.body); rec.Code != http.StatusNoContent {
			t.Fatalf("%s の設定に失敗: %d body=%s", step.path, rec.Code, rec.Body.String())
		}
	}

	rec := do(t, mux, http.MethodGet, "/api/program", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("取得に失敗: %d", rec.Code)
	}
	var got struct {
		PerWeek   int      `json:"per_week"`
		Exercises int      `json:"exercises_per_session"`
		Sets      int      `json:"sets_per_exercise"`
		Selected  []string `json:"selected_exercises"`
		Declared  []string `json:"declared_exercises"`
		Focus     *string  `json:"focus_exercise"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if got.PerWeek != 2 {
		t.Errorf("頻度が往復していない: %d", got.PerWeek)
	}
	if got.Exercises != 5 || got.Sets != 4 {
		t.Errorf("1回の量が往復していない: %d種目×%dセット", got.Exercises, got.Sets)
	}
	if len(got.Selected) != 4 {
		t.Errorf("選択種目が往復していない: %v", got.Selected)
	}
	if len(got.Declared) != 3 {
		t.Errorf("宣言が往復していない: %v", got.Declared)
	}
	if got.Focus == nil || *got.Focus != "bench" {
		t.Errorf("重点種目が往復していない: %v", got.Focus)
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

// 週目標を手で変える口が無いこと。
//
// 週目標は補助セレクタの充足閾値であって利用者の設定ではない（D-139）。
// 口が残っていると、頻度変更が上書きする値を一時的に書き換えられ、
// 「設定できるように見えて保持されない」状態に戻る。
func TestRoutes_HasNoWeeklyTargetEndpoint(t *testing.T) {
	mux := newServer(t, true)
	rec := do(t, mux, http.MethodPut, "/api/program/target", `{"weekly_target":{"QUAD":12}}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("週目標の口がまだ生きている: %d", rec.Code)
	}
}

// メソッドが違えばルーティングされないこと。
func TestRoutes_RejectWrongMethod(t *testing.T) {
	mux := newServer(t, true)
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/sessions"},
		{http.MethodPut, "/api/set-logs"},
		{http.MethodGet, "/api/conditions"},
		{http.MethodPost, "/api/program"},
		// 全置換の口は消した（#123）。splits を黙って落とすので、
		// GET の応答をそのまま投げ返すと分割が消えていた。
		{http.MethodPut, "/api/program"},
		{http.MethodPost, "/api/program/focus"},
		{http.MethodPost, "/api/program/declared"},
		{http.MethodPost, "/api/program/frequency"},
		{http.MethodPost, "/api/program/volume"},
		{http.MethodPost, "/api/program/selected"},
		{http.MethodPost, "/api/program/split"},
		{http.MethodPost, "/api/split-presets"},
		{http.MethodPost, "/api/exercises"},
		{http.MethodPost, "/api/stats"},
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

func (brokenExercises) FindAll(context.Context) ([]*exercise.Exercise, error) {
	return nil, errors.New("種目テーブル exercises_v2 の接続文字列が不正: user=admin")
}

// 500 のときに内部のエラー文を返さないこと。
// ドメインやインフラのエラーにはテーブル名・接続情報・閾値が載っている。
func TestGetSession_InternalErrorDoesNotLeak(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, _ := program.NewFrequency(3)
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	program, err := program.NewProgram(freq, mustVolume(t, 6, 3), selected, []exercise.ExerciseID{"bench", "squat", "deadlift"}, "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	programs := memory.NewProgramRepository()
	if err := programs.Save(context.Background(), testUser, program); err != nil {
		t.Fatalf("プログラムの保存に失敗: %v", err)
	}
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()

	mux := routesFrom(t, dependencies(brokenExercises{}, logs, conditions, programs))
	// 認証を通す。利用者を決めるのはミドルウェアなので、
	// 生のルータを叩くと「利用者が無い」で 500 になり、
	// ここで見たい分類が見えない。
	mux = authed(t, mux)

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

	// 宣言した3種目すべてに記録する。ヘビー枠は「最後にやったのが最も
	// 古い種目」で、一度もやっていない種目が最優先になる。ベンチだけ
	// 積むと、未実施のスクワットが軸に来てベンチがメインに現れない。
	//
	// ベンチだけ重量を据え置き、他は伸ばす。停滞判定に載るのはベンチだけ。
	var logs []string
	for i := range 10 {
		date := training.MustDate(2026, time.June, 1).AddDays(i * 7)
		for s := range 3 {
			for _, spec := range []struct {
				id string
				kg float64
			}{
				{"bench", 85},
				{"squat", 110 + float64(i)*2.5},
				{"deadlift", 140 + float64(i)*2.5},
			} {
				logs = append(logs, fmt.Sprintf(
					`{"id":"%s%d-%d","date":"%s","exercise_id":%q,"weight_kg":%g,"reps":8,"rir":2}`,
					spec.id, i, s, date.String(), spec.id, spec.kg))
			}
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
			Sets      int `json:"sets"`
			TargetRIR int `json:"target_rir"`
		} `json:"main"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if len(got.Main) == 0 {
		t.Fatal("メイン種目が無い")
	}
	if got.Main[0].TargetRIR != 2 {
		t.Errorf("睡眠不足の補正が届いていない: target_rir=%d（期待 2）", got.Main[0].TargetRIR)
	}
	if got.Main[0].Sets != 3 {
		t.Errorf("セット数が誤り: %d（期待 3）", got.Main[0].Sets)
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
	for name, c := range map[string]struct{ method, path, body string }{
		"set-logs":   {http.MethodPost, "/api/set-logs", `{"logs":[{"id":"d","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":8,"rir":2}]}`},
		"conditions": {http.MethodPost, "/api/conditions", `{"conditions":[{"date":"2026-08-17","body_weight_kg":75}]}`},
		"focus":      {http.MethodPut, "/api/program/focus", `{"focus_exercise":"bench"}`},
		"declared":   {http.MethodPut, "/api/program/declared", `{"declared_exercises":["bench"]}`},
		"frequency":  {http.MethodPut, "/api/program/frequency", `{"per_week":4}`},
		"volume":     {http.MethodPut, "/api/program/volume", `{"exercises_per_session":5,"sets_per_exercise":4}`},
		"selected":   {http.MethodPut, "/api/program/selected", `{"selected_exercises":["bench","squat","deadlift"]}`},
		"split":      {http.MethodPut, "/api/program/split", `{"splits":[{"name":"全身","regions":[]}]}`},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)).WithContext(ctx)
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

// --- 画面はもう配らない ---

// 元は「画面の殻は認証なしで開ける」ことを検査していた。
// やめた理由: 画面を Cloudflare Workers に移し、別オリジンから CORS で
// この API を叩く形にした。バイナリに同居させていた頃の判断を覆した経緯は
// docs/decisions.md の D-119 と
// docs/specs/2026-09-07-pwa-client-design.md にある。
//
// 「配らなくなった」を検査に残すのは、embed を戻したときに気づくため。
func TestStatic_ShellIsNoLongerServed(t *testing.T) {
	mux := newServer(t, true)

	for _, path := range []string{
		"/", "/app.js", "/app.webmanifest", "/sw.js", "/icon.svg",
		"/index.html", "/web/index.html", "/web/app.js", "/app.js.map",
	} {
		if rec := do(t, mux, http.MethodGet, path, ""); rec.Code == http.StatusOK {
			t.Errorf("%s が配信された。画面はサーバーから配らない", path)
		}
	}
}

// API が認証なしで通らないこと。
//
// 認証は cmd が被せるので、ルータ単体では判定できない。
// 認証ミドルウェアを通した状態で確かめる。
func TestAPI_RequiresAuth(t *testing.T) {
	h, reached := guarded(t)
	for _, path := range []string{
		"/api/sessions?date=2026-08-17", "/api/program", "/api/program/focus",
		"/api/program/declared", "/api/program/frequency", "/api/program/selected",
		"/api/set-logs",
	} {
		rec := request(t, h, path, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s が認証なしで通った: %d", path, rec.Code)
		}
	}
	if *reached {
		t.Error("認証なしでハンドラへ到達した")
	}
}

// mustVolume はテスト用の1回の量。
func mustVolume(t *testing.T, exercises, sets int) program.SessionVolume {
	t.Helper()
	v, err := program.NewSessionVolume(exercises, sets)
	if err != nil {
		t.Fatalf("NewSessionVolume(%d, %d): %v", exercises, sets, err)
	}
	return v
}
