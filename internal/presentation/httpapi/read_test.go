package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
)

// 種目が日本語で引けること。
// ID のままだと、ジムで一瞬見て何の種目か分からない。
func TestGetExercises_ReturnsJapaneseNames(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/exercises", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		Exercises []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"exercises"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	// 件数を数字で書かない。シードに種目を足すたびに、応答と関係ない
	// 理由でここが落ちる。見たいのは「同梱の種目が全部返る」こと。
	all, err := seed.Exercises()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Exercises) != len(all) {
		t.Errorf("種目の数が誤り: %d（期待 %d）", len(got.Exercises), len(all))
	}

	for _, e := range got.Exercises {
		if e.Name == "" {
			t.Errorf("%s の名前が空である", e.ID)
		}
		if e.Name == e.ID {
			t.Errorf("%s の名前がIDのままである", e.ID)
		}
	}
}

// 記録して、履歴として引けること。
func TestGetSetLogs_ReturnsWhatWasRecorded(t *testing.T) {
	mux := newServer(t, true)

	logs := make([]string, 0, 3)
	for i := range 3 {
		logs = append(logs, fmt.Sprintf(
			`{"id":"h%d","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":%d,"rir":2}`,
			i, 8-i))
	}
	if rec := do(t, mux, http.MethodPost, "/api/set-logs",
		`{"logs":[`+strings.Join(logs, ",")+`]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("記録に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-31", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("取得に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	var got setLogsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if len(got.Days) != 1 {
		t.Fatalf("日の数が誤り: %d", len(got.Days))
	}
	day := got.Days[0]
	if day.Date != "2026-08-17" || day.TotalSets != 3 {
		t.Errorf("日の内容が誤り: %+v", day)
	}
	if len(day.Exercises) != 1 || day.Exercises[0].Name != "ベンチプレス" {
		t.Errorf("種目名が引けていない: %+v", day.Exercises)
	}
	if reps := day.Exercises[0].Sets; len(reps) != 3 || reps[0].Reps != 8 {
		t.Errorf("セットの中身が誤り: %+v", reps)
	}
}

// 前回の実績が引けること。今日の重量を信じる根拠になる。
func TestGetSetLogs_ReturnsLastPerformance(t *testing.T) {
	mux := newServer(t, true)

	// 別々の日に2回やる。前回として返るのは新しいほう。
	for _, spec := range []struct {
		date, id string
		kg       int
	}{
		{"2026-08-10", "old", 80},
		{"2026-08-17", "new", 85},
	} {
		body := fmt.Sprintf(
			`{"logs":[{"id":"%s","date":"%s","exercise_id":"bench","weight_kg":%d,"reps":8,"rir":2}]}`,
			spec.id, spec.date, spec.kg)
		if rec := do(t, mux, http.MethodPost, "/api/set-logs", body); rec.Code != http.StatusNoContent {
			t.Fatalf("記録に失敗: %d", rec.Code)
		}
	}

	rec := do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-24", "")
	var got setLogsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}

	last, ok := got.Last["bench"]
	if !ok {
		t.Fatal("前回の実績が返っていない")
	}
	if last.Date != "2026-08-17" || last.WeightKg != 85 {
		t.Errorf("前回が新しいほうになっていない: %+v", last)
	}
	if last.DaysAgo != 7 {
		t.Errorf("経過日数が誤り: %d", last.DaysAgo)
	}
}

// 当日ぶんを「前回」に含めないこと。
// 含めると、いま記録した1セットが前回として出て比較の意味が消える。
func TestGetSetLogs_ExcludesTodayFromLastPerformance(t *testing.T) {
	mux := newServer(t, true)

	for _, spec := range []struct {
		date, id string
		kg       int
	}{
		{"2026-08-10", "prev", 80},
		{"2026-08-17", "today", 85},
	} {
		body := fmt.Sprintf(
			`{"logs":[{"id":"%s","date":"%s","exercise_id":"bench","weight_kg":%d,"reps":8,"rir":2}]}`,
			spec.id, spec.date, spec.kg)
		do(t, mux, http.MethodPost, "/api/set-logs", body)
	}

	rec := do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-17", "")
	var got setLogsBody
	_ = json.Unmarshal(rec.Body.Bytes(), &got)

	if last := got.Last["bench"]; last.Date != "2026-08-10" {
		t.Errorf("当日ぶんが前回として返った: %+v", last)
	}
}

// 打ち間違いを取り消せること。
func TestDeleteSetLog_RemovesTheRecord(t *testing.T) {
	mux := newServer(t, true)

	body := `{"logs":[{"id":"typo","date":"2026-08-17","exercise_id":"bench","weight_kg":850,"reps":8,"rir":2}]}`
	if rec := do(t, mux, http.MethodPost, "/api/set-logs", body); rec.Code != http.StatusNoContent {
		t.Fatalf("記録に失敗: %d", rec.Code)
	}

	if rec := do(t, mux, http.MethodDelete, "/api/set-logs/typo", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("取り消しに失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-31", "")
	var got setLogsBody
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Days) != 0 {
		t.Errorf("取り消したのに残っている: %+v", got.Days)
	}

	// 二度目も成功すること。再送で来たときにエラーにすると、
	// 消えているのに消せないという状態になる。
	if rec := do(t, mux, http.MethodDelete, "/api/set-logs/typo", ""); rec.Code != http.StatusNoContent {
		t.Errorf("二度目の取り消しが失敗した: %d", rec.Code)
	}
}

// 週目標の充足が引けること。
// これはアプリの中心概念なのに、これまでどこにも表示されていなかった。
func TestGetStats_ReturnsWeeklyVolume(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/stats?to=2026-08-17", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var got struct {
		WeeklyVolume []struct {
			Region     string  `json:"region"`
			TargetSets float64 `json:"target_sets"`
			DoneSets   float64 `json:"done_sets"`
		} `json:"weekly_volume"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if len(got.WeeklyVolume) == 0 {
		t.Fatal("週目標が返っていない")
	}
	for _, v := range got.WeeklyVolume {
		if v.TargetSets <= 0 {
			t.Errorf("%s の目標が0以下: %v", v.Region, v.TargetSets)
		}
	}
	// 埋まっていない順に並ぶこと。目を向けるべきものが上に来る。
	for i := 1; i < len(got.WeeklyVolume); i++ {
		a := got.WeeklyVolume[i-1]
		b := got.WeeklyVolume[i]
		if a.DoneSets/a.TargetSets > b.DoneSets/b.TargetSets {
			t.Errorf("充足の低い順になっていない: %s の後に %s", a.Region, b.Region)
			break
		}
	}
}

// 推定1RMの推移が引けること。
func TestGetStats_ReturnsTrends(t *testing.T) {
	mux := newServer(t, true)

	for i := range 4 {
		body := fmt.Sprintf(
			`{"logs":[{"id":"t%d","date":"2026-07-%02d","exercise_id":"bench",`+
				`"weight_kg":%d,"reps":8,"rir":2}]}`, i, 6+i*7, 80+i*5)
		if rec := do(t, mux, http.MethodPost, "/api/set-logs", body); rec.Code != http.StatusNoContent {
			t.Fatalf("記録に失敗: %d", rec.Code)
		}
	}

	rec := do(t, mux, http.MethodGet, "/api/stats?from=2026-07-01&to=2026-08-17", "")
	var got struct {
		Trends []struct {
			ExerciseID string `json:"exercise_id"`
			Name       string `json:"name"`
			Points     []struct {
				Date string  `json:"date"`
				Kg   float64 `json:"kg"`
			} `json:"points"`
			CurrentKg float64 `json:"current_kg"`
			ChangeKg  float64 `json:"change_kg"`
		} `json:"trends"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}

	for _, tr := range got.Trends {
		if tr.ExerciseID != "bench" {
			continue
		}
		if len(tr.Points) != 4 {
			t.Errorf("点の数が誤り: %d", len(tr.Points))
		}
		if tr.Name != "ベンチプレス" {
			t.Errorf("名前が引けていない: %q", tr.Name)
		}
		if tr.ChangeKg <= 0 {
			t.Errorf("伸びているのに変化が正でない: %v", tr.ChangeKg)
		}
		if tr.CurrentKg <= 0 {
			t.Errorf("現在値が不正: %v", tr.CurrentKg)
		}
		return
	}
	t.Fatal("bench の推移が返っていない")
}

// 期間の指定が不正なら 400。
func TestReadEndpoints_RejectBadPeriods(t *testing.T) {
	mux := newServer(t, true)
	for name, path := range map[string]string{
		"from が日付でない":   "/api/set-logs?from=きのう&to=2026-08-17",
		"to が日付でない":     "/api/set-logs?to=いつか",
		"from が to より後": "/api/set-logs?from=2026-08-20&to=2026-08-17",
		"期間が長すぎる":       "/api/set-logs?from=2000-01-01&to=2026-08-17",
	} {
		t.Run(name, func(t *testing.T) {
			if rec := do(t, mux, http.MethodGet, path, ""); rec.Code != http.StatusBadRequest {
				t.Errorf("400 でない: %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

// 期間を省略しても既定で引けること。
// 画面が毎回日付を組み立てなくて済む。
func TestReadEndpoints_DefaultThePeriod(t *testing.T) {
	mux := newServer(t, true)
	for _, path := range []string{"/api/set-logs", "/api/stats"} {
		if rec := do(t, mux, http.MethodGet, path, ""); rec.Code != http.StatusOK {
			t.Errorf("%s が既定の期間で引けない: %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

// 読み取りの口も、失敗を分類して返すこと。
//
// 翻訳（apperror.Classify）は usecase の出口にしか無く、query を通る
// 読み取りは一時障害も未設定も 500 で返していた（#129）。500 は
// 「調べるべき障害」として鳴るので、DB が一瞬応えなかっただけで警報が汚れる。
//
// TestGetSession_UnavailableIsNot500 は usecase 経由の1本しか見ていない。
// 分類を消しても 500 で緑になるので、読み取りは読み取りで独立に見る。
func TestReadEndpoints_ClassifyFailures(t *testing.T) {
	// 種目マスタに届かないサーバー。読み取りの口はどれも種目を引くので、
	// 差し替えるのはここ1つで足りる。
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	programs := memory.NewProgramRepository()
	unavailable := authed(t, routesFrom(t, dependencies(unavailableExercises{}, logs, conditions, programs)))

	cases := []struct {
		name     string
		mux      http.Handler
		path     string
		want     int
		wantCode string
	}{
		{
			name: "種目一覧は保存先に届かなければ 503",
			mux:  unavailable, path: "/api/exercises",
			want: http.StatusServiceUnavailable, wantCode: "UNAVAILABLE",
		},
		{
			name: "履歴は保存先に届かなければ 503",
			mux:  unavailable, path: "/api/set-logs?from=2026-08-01&to=2026-08-17",
			want: http.StatusServiceUnavailable, wantCode: "UNAVAILABLE",
		},
		{
			name: "推移は保存先に届かなければ 503",
			mux:  unavailable, path: "/api/stats?from=2026-08-01&to=2026-08-17",
			want: http.StatusServiceUnavailable, wantCode: "UNAVAILABLE",
		},
		{
			// 404 ではなく 409。推移と週の達成度はプログラム（選択と週目標）
			// から導くもので、無いのは「前提が満たされていない」。
			// セッション導出と同じ側で、404 にするのは取得の対象が
			// プログラムそのものである GET /api/program だけ（D-042）。
			name: "推移はプログラムが未設定なら 409",
			mux:  newServer(t, false), path: "/api/stats?from=2026-08-01&to=2026-08-17",
			want: http.StatusConflict, wantCode: "NOT_CONFIGURED",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, c.mux, http.MethodGet, c.path, "")
			if rec.Code != c.want {
				t.Errorf("ステータスが %d。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("応答を解釈できない: %v", err)
			}
			if body.Code != c.wantCode {
				t.Errorf("コードが %q。%q のはず", body.Code, c.wantCode)
			}
			// 接続文字列は外に出さない。
			if strings.Contains(rec.Body.String(), "10.0.0.1") {
				t.Errorf("接続先が漏れている: %s", rec.Body.String())
			}
		})
	}
}

type setLogsBody struct {
	Days []struct {
		Date      string `json:"date"`
		TotalSets int    `json:"total_sets"`
		Exercises []struct {
			ExerciseID string `json:"exercise_id"`
			Name       string `json:"name"`
			Sets       []struct {
				ID       string  `json:"id"`
				WeightKg float64 `json:"weight_kg"`
				Reps     int     `json:"reps"`
			} `json:"sets"`
		} `json:"exercises"`
	} `json:"days"`
	Last map[string]struct {
		Date     string    `json:"date"`
		WeightKg float64   `json:"weight_kg"`
		Weights  []float64 `json:"weights"`
		Reps     []int     `json:"reps"`
		DaysAgo  int       `json:"days_ago"`
	} `json:"last_performances"`
}

// 1日に複数の種目をやった場合を、種目ごとに分けて返すこと。
//
// TrainingSession は「同じ日」でまとめたもので、1日に複数種目が入る。
// セッション単位で扱うと、その日の最初の種目しか出ない。
// 実際にそう書いて、実データで初めて気づいた。
func TestGetSetLogs_SeparatesExercisesWithinADay(t *testing.T) {
	mux := newServer(t, true)

	body := `{"logs":[
		{"id":"b1","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":8,"rir":2},
		{"id":"b2","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":7,"rir":2},
		{"id":"s1","date":"2026-08-17","exercise_id":"squat","weight_kg":120,"reps":6,"rir":2},
		{"id":"d1","date":"2026-08-17","exercise_id":"deadlift","weight_kg":150,"reps":5,"rir":2}
	]}`
	if rec := do(t, mux, http.MethodPost, "/api/set-logs", body); rec.Code != http.StatusNoContent {
		t.Fatalf("記録に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-31", "")
	var got setLogsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}

	if len(got.Days) != 1 {
		t.Fatalf("日の数が誤り: %d", len(got.Days))
	}
	day := got.Days[0]
	if len(day.Exercises) != 3 {
		t.Fatalf("種目の数が誤り: %d（期待 3）: %+v", len(day.Exercises), day.Exercises)
	}
	if day.TotalSets != 4 {
		t.Errorf("総セット数が誤り: %d（期待 4）", day.TotalSets)
	}

	sets := map[string]int{}
	for _, e := range day.Exercises {
		sets[e.ExerciseID] = len(e.Sets)
	}
	for id, want := range map[string]int{"bench": 2, "squat": 1, "deadlift": 1} {
		if sets[id] != want {
			t.Errorf("%s のセット数が誤り: %d（期待 %d）", id, sets[id], want)
		}
	}

	// 前回の実績も種目ごとに出ること。
	rec = do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-24", "")
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	for _, id := range []string{"bench", "squat", "deadlift"} {
		if _, ok := got.Last[id]; !ok {
			t.Errorf("%s の前回が返っていない: %v", id, got.Last)
		}
	}
	if last := got.Last["bench"]; len(last.Reps) != 2 {
		t.Errorf("前回のレップが種目ごとになっていない: %+v", last)
	}
}

// 種目ごとに違う日が最新のとき、それぞれの最新を返すこと。
func TestGetSetLogs_LastPerformanceIsPerExercise(t *testing.T) {
	mux := newServer(t, true)

	body := `{"logs":[
		{"id":"b-old","date":"2026-08-10","exercise_id":"bench","weight_kg":80,"reps":8,"rir":2},
		{"id":"b-new","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":8,"rir":2},
		{"id":"s-old","date":"2026-08-12","exercise_id":"squat","weight_kg":110,"reps":8,"rir":2}
	]}`
	do(t, mux, http.MethodPost, "/api/set-logs", body)

	rec := do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-24", "")
	var got setLogsBody
	_ = json.Unmarshal(rec.Body.Bytes(), &got)

	if b := got.Last["bench"]; b.Date != "2026-08-17" || b.WeightKg != 85 {
		t.Errorf("bench の前回が誤り: %+v", b)
	}
	if s := got.Last["squat"]; s.Date != "2026-08-12" || s.WeightKg != 110 {
		t.Errorf("squat の前回が誤り: %+v", s)
	}
}

// 「前回」がセットごとの重量を持つこと。
//
// 1つに畳むと、途中で落とした重量も上げた重量も画面から消える。
// 実際に 105kg と 101kg のセットが「101kg × 5, 5」と出ていた。
func TestGetSetLogs_LastPerformanceKeepsEachSetsWeight(t *testing.T) {
	mux := newServer(t, true)

	body := `{"logs":[` +
		`{"id":"w1","date":"2026-08-17","exercise_id":"bench","weight_kg":100,"reps":5,"rir":1},` +
		`{"id":"w2","date":"2026-08-17","exercise_id":"bench","weight_kg":90,"reps":8,"rir":1},` +
		`{"id":"w3","date":"2026-08-17","exercise_id":"bench","weight_kg":80,"reps":12,"rir":0}]}`
	if rec := do(t, mux, http.MethodPost, "/api/set-logs", body); rec.Code != http.StatusNoContent {
		t.Fatalf("記録に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/set-logs?from=2026-08-01&to=2026-08-20", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("取得に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	var got setLogsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	last, ok := got.Last["bench"]
	if !ok {
		t.Fatal("前回が返らない")
	}
	if len(last.Weights) != len(last.Reps) {
		t.Fatalf("重量とレップの数が揃わない: %d と %d", len(last.Weights), len(last.Reps))
	}
	want := []float64{100, 90, 80}
	if len(last.Weights) != len(want) {
		t.Fatalf("重量が畳まれている: %v", last.Weights)
	}
	for i, w := range want {
		if last.Weights[i] != w {
			t.Fatalf("%d番目の重量が %v、期待は %v", i, last.Weights[i], w)
		}
	}
	if last.WeightKg != 100 {
		t.Fatalf("代表の重量が最重量でない: %v", last.WeightKg)
	}
}

// from を省いたら、当日だけでなく直近をまとめて返すこと。
//
// 画面は毎回日付を組み立てずに済むよう省略できる。ここが当日だけに
// なると、履歴を開いても今日しか出ない。
func TestGetSetLogs_DefaultPeriodLooksBack(t *testing.T) {
	mux := newServer(t, true)

	// 30日前。既定の窓（56日）には入り、当日だけの窓には入らない。
	old := time.Now().UTC().AddDate(0, 0, -30).Format("2006-01-02")
	body := fmt.Sprintf(
		`{"logs":[{"id":"old-1","date":%q,"exercise_id":"bench","weight_kg":85,"reps":5,"rir":2}]}`,
		old)
	if rec := do(t, mux, http.MethodPost, "/api/set-logs", body); rec.Code != http.StatusNoContent {
		t.Fatalf("記録に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/set-logs", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("取得に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	var got setLogsBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if len(got.Days) != 1 || got.Days[0].Date != old {
		t.Fatalf("既定の期間が遡っていない: %+v", got.Days)
	}
}

// 種目の一覧が刺激の分布まで返すこと。
//
// 画面が部位ごとにまとめるのに使う。応答にフィールドを足すのは古い
// クライアントを壊さない（DisallowUnknownFields はリクエストにしか効かない）。
func TestGetExercises_CarriesStimulus(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/exercises", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが %d", rec.Code)
	}

	var got struct {
		Exercises []struct {
			ID       string             `json:"id"`
			Stimulus map[string]float64 `json:"stimulus"`
		} `json:"exercises"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を読めない: %v", err)
	}
	if len(got.Exercises) == 0 {
		t.Fatal("種目が1件も返っていない")
	}

	for _, e := range got.Exercises {
		if len(e.Stimulus) == 0 {
			t.Errorf("%s の刺激分布が空である", e.ID)
		}
	}
}
