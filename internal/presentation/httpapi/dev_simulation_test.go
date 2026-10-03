package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/devsim"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

func devMux(t *testing.T) *http.ServeMux {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	sim, err := devsim.NewSimulator(pool)
	if err != nil {
		t.Fatalf("NewSimulator: %v", err)
	}

	mux := http.NewServeMux()
	httpapi.NewDevSimulation(sim).Mount(mux)
	return mux
}

func devGet(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	devMux(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestDevSimulation_ReturnsAPlan(t *testing.T) {
	rec := devGet(t, "/api/dev/simulate?declared=bench,squat,deadlift&focus=bench&split=upper_lower&frequency=4&weeks=2")

	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Days []struct {
			Date  string `json:"date"`
			Split string `json:"split"`
			Main  []struct {
				ExerciseID string   `json:"exercise_id"`
				PctOf1RM   *float64 `json:"pct_of_1rm"`
			} `json:"main"`
		} `json:"days"`
		Weeks []struct {
			Regions []struct {
				Target float64 `json:"target"`
			} `json:"regions"`
		} `json:"weeks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	if len(got.Days) != 8 {
		t.Errorf("セッションが %d 件。週4×2週で 8 件のはず", len(got.Days))
	}
	if len(got.Weeks) != 2 {
		t.Errorf("週が %d 件。2 件のはず", len(got.Weeks))
	}
	if len(got.Days) > 0 && got.Days[0].Split == "" {
		t.Error("分割を指定したのに、その日の分割が空")
	}
	if len(got.Weeks) > 0 && len(got.Weeks[0].Regions) == 0 {
		t.Error("週の充足が空")
	}
}

// 未確定の重量は null で返すこと。0 にすると画面が「0kg」と出す。
func TestDevSimulation_UndecidedWeightIsNull(t *testing.T) {
	rec := devGet(t, "/api/dev/simulate?declared=bench&frequency=1&weeks=1")

	var got struct {
		Days []struct {
			Main []struct {
				WeightKg *float64 `json:"weight_kg"`
			} `json:"main"`
		} `json:"days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if len(got.Days) == 0 || len(got.Days[0].Main) == 0 {
		t.Fatal("軸が出ていない")
	}
	if got.Days[0].Main[0].WeightKg != nil {
		t.Errorf("履歴が無い初日の重量が %v。null のはず", *got.Days[0].Main[0].WeightKg)
	}
}

// 成り立たない設定は 400。開発中の日常なので 500 にしない。
func TestDevSimulation_RejectsBadInput(t *testing.T) {
	for _, path := range []string{
		"/api/dev/simulate?declared=&frequency=4",
		"/api/dev/simulate?declared=bench&frequency=99",
		"/api/dev/simulate?declared=bench&weeks=zero",
		"/api/dev/simulate?declared=bench&split=nope",
	} {
		t.Run(path, func(t *testing.T) {
			rec := devGet(t, path)
			if code := rec.Code; code != http.StatusBadRequest {
				t.Errorf("%d が返った。400 のはず", code)
			}
			// 本体の口と同じ形で返す。400 の形が口によって違わないこと。
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

// 宣言ごとのレップ数をクエリで受け取り、設定に返し、軸の処方に効くこと。
func TestDevSimulation_TakesRepTargetsAndEchoesThem(t *testing.T) {
	rec := devGet(t, "/api/dev/simulate?declared=bench&focus=&split=&frequency=2&weeks=1&reps=bench:8:12")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Settings struct {
			Reps map[string]struct {
				Heavy int `json:"heavy"`
				Light int `json:"light"`
			} `json:"reps"`
		} `json:"settings"`
		Days []struct {
			Main []struct {
				ExerciseID string `json:"exercise_id"`
				TargetReps int    `json:"target_reps"`
			} `json:"main"`
		} `json:"days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if r := got.Settings.Reps["bench"]; r.Heavy != 8 || r.Light != 12 {
		t.Errorf("設定に返った bench が %+v。{8 12} のはず", r)
	}
	if len(got.Days) == 0 || len(got.Days[0].Main) == 0 || got.Days[0].Main[0].TargetReps != 8 {
		t.Errorf("軸の目標レップが 8 になっていない: %+v", got.Days)
	}
}

func TestDevSimulation_RejectsBadRepsQuery(t *testing.T) {
	for _, q := range []string{
		"reps=bench:8",     // 軽い番が無い
		"reps=bench:x:12",  // 数字でない
		"reps=bench:16:12", // 範囲外（NewRepTargets が弾く）
		"reps=squat:8:12",  // 宣言していない（devsim が弾く）
	} {
		t.Run(q, func(t *testing.T) {
			rec := devGet(t, "/api/dev/simulate?declared=bench&"+q)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
			// キーを知らないことによる 400 は、reps の中身を見て弾いたことにならない。
			if strings.Contains(rec.Body.String(), "知らないキー") {
				t.Errorf("reps を読まずに弾いている: %s", rec.Body.String())
			}
		})
	}
}

func TestDevSimulation_ServesOptions(t *testing.T) {
	rec := devGet(t, "/api/dev/options")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず", rec.Code)
	}

	var got struct {
		Exercises []struct {
			ID string `json:"id"`
		} `json:"exercises"`
		Presets []struct {
			Key  string   `json:"key"`
			Days []string `json:"days"`
		} `json:"presets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if len(got.Exercises) == 0 {
		t.Error("種目が空")
	}
	if len(got.Presets) == 0 {
		t.Error("分割プリセットが空")
	}
	for _, p := range got.Presets {
		if len(p.Days) == 0 {
			t.Errorf("%s の日が空。画面が中身を出せない", p.Key)
		}
	}
}

// 模擬ユーザーの設定はクエリで渡し、応答は実際に使った設定を返す。
//
// 応答だけ読めば、どの仮定から出た数字かが分かるようにする。画面を
// 通さずに結果を読む（Claude が curl で確かめる）ときに、仮定が既定値
// なのか上書きなのかを推測させない。
func TestDevSimulation_TakesAthleteSettingsAndEchoesThem(t *testing.T) {
	rec := devGet(t, "/api/dev/simulate?declared=bench&frequency=2&weeks=2"+
		"&growth=1.5&first_pct=60&body_weight=82&orm=bench:150,pull_up:0")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Settings struct {
			Declared []string `json:"declared"`
			Weeks    int      `json:"weeks"`
			Start    string   `json:"start"`
			Athlete  struct {
				GrowthPctPerWeek float64            `json:"growth_pct_per_week"`
				FirstSessionPct  float64            `json:"first_session_pct"`
				BodyWeightKg     float64            `json:"body_weight_kg"`
				OneRepMaxKg      map[string]float64 `json:"one_rep_max_kg"`
			} `json:"athlete"`
		} `json:"settings"`
		Days []struct {
			Main []struct {
				ExerciseID string   `json:"exercise_id"`
				Athlete1RM *float64 `json:"athlete_1rm_kg"`
				Performed  *struct {
					WeightKg float64 `json:"weight_kg"`
					Reps     int     `json:"reps"`
					RIR      int     `json:"rir"`
				} `json:"performed"`
			} `json:"main"`
		} `json:"days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	a := got.Settings.Athlete
	if a.GrowthPctPerWeek != 1.5 || a.FirstSessionPct != 60 || a.BodyWeightKg != 82 {
		t.Errorf("設定が返っていない: %+v", a)
	}
	if a.OneRepMaxKg["bench"] != 150 || a.OneRepMaxKg["pull_up"] != 0 {
		t.Errorf("上書きした1RMが返っていない: bench=%v pull_up=%v", a.OneRepMaxKg["bench"], a.OneRepMaxKg["pull_up"])
	}
	// 上書きしなかった種目も既定値で埋まる。どれが何kgの前提か、応答だけで分かる。
	if a.OneRepMaxKg["squat"] != devsim.DefaultOneRepMax("squat") {
		t.Errorf("上書きしていない squat が %v。既定値のはず", a.OneRepMaxKg["squat"])
	}
	if got.Settings.Weeks != 2 || got.Settings.Start == "" || len(got.Settings.Declared) != 1 {
		t.Errorf("計画の設定が返っていない: %+v", got.Settings)
	}

	if len(got.Days) == 0 || len(got.Days[0].Main) == 0 {
		t.Fatal("軸が出ていない")
	}
	first := got.Days[0].Main[0]
	if first.Performed == nil {
		t.Fatal("記録（performed）が無い")
	}
	// 履歴の無い初日は本人が選ぶ。150kg の60%で 90kg。
	if first.Performed.WeightKg != 90 || first.Performed.Reps < 1 {
		t.Errorf("初日の記録が %+v。90kg で1回以上のはず", *first.Performed)
	}
	if first.Athlete1RM == nil || *first.Athlete1RM != 150 {
		t.Errorf("初日の実力が %v。150 のはず", first.Athlete1RM)
	}
}

// 模擬ユーザーの設定の形が壊れていたら 400。
func TestDevSimulation_RejectsBadAthleteQuery(t *testing.T) {
	for _, q := range []string{
		"growth=fast",
		"growth=50",
		"first_pct=",
		"first_pct=0",
		"body_weight=heavy",
		"orm=bench",
		"orm=bench:heavy",
		// 自重種目は 0kg が正当なので、数値の失敗を 0 に倒すと素通りする。
		"orm=pull_up:heavy",
		"orm=nope:100",
	} {
		t.Run(q, func(t *testing.T) {
			rec := devGet(t, "/api/dev/simulate?declared=bench&"+q)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// 知らないキーと、同じキーの2回指定は 400。どのキーかを本文で名指しする。
//
// 黙って読み飛ばすと、書き間違えたキーは既定値のまま走る。growth の既定は
// 0.5%/週なので、`growt=0` と打つと「実力一定」のつもりで伸びる人の結果を
// 読むことになり、しかも応答は 200 で気づけない（開発用シミュレーションを
// 観点ごとに叩いたときに実際に踏んだ）。2回指定は q.Get が先頭だけを取り、
// 後ろが消える。
func TestDevSimulation_RejectsUnknownOrRepeatedKey(t *testing.T) {
	cases := []struct {
		name  string
		query string
		key   string
	}{
		{name: "書き間違えたキー", query: "declared=bench&growt=0", key: "growt"},
		{name: "知らないキー", query: "declared=bench&volume=5", key: "volume"},
		{name: "同じキーを2回", query: "declared=bench&declared=squat", key: "declared"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := devGet(t, "/api/dev/simulate?"+c.query)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("JSONが壊れている: %v", err)
			}
			if !strings.Contains(body.Error, c.key) {
				t.Errorf("本文が %q。キー %q を名指しするはず", body.Error, c.key)
			}
		})
	}
}

// 画面が入力欄の初期値に使う既定値を返す。画面が定数を二重に持たないため。
func TestDevSimulation_OptionsCarryAthleteDefaults(t *testing.T) {
	rec := devGet(t, "/api/dev/options")

	var got struct {
		Exercises []struct {
			ID               string  `json:"id"`
			DefaultOneRepMax float64 `json:"default_1rm_kg"`
			Bodyweight       bool    `json:"bodyweight"`
		} `json:"exercises"`
		Athlete struct {
			GrowthPctPerWeek float64 `json:"growth_pct_per_week"`
			FirstSessionPct  float64 `json:"first_session_pct"`
			BodyWeightKg     float64 `json:"body_weight_kg"`
		} `json:"athlete_defaults"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}

	want := devsim.DefaultAthlete()
	if got.Athlete.GrowthPctPerWeek != want.GrowthPctPerWeek ||
		got.Athlete.FirstSessionPct != want.FirstSessionPct ||
		got.Athlete.BodyWeightKg != want.BodyWeightKg {
		t.Errorf("既定値が %+v。%+v のはず", got.Athlete, want)
	}
	byID := map[string]int{}
	for i, e := range got.Exercises {
		byID[e.ID] = i
	}
	if e := got.Exercises[byID["bench"]]; e.DefaultOneRepMax != 100 || e.Bodyweight {
		t.Errorf("bench が %+v。既定100kg・自重でない、のはず", e)
	}
	if e := got.Exercises[byID["pull_up"]]; !e.Bodyweight {
		t.Errorf("pull_up が自重種目になっていない: %+v", e)
	}
}

// 空の split と focus は「指定なし」。画面は「分割なし」を URL で再現する
// ために空のまま送る（キーごと落とすと既定の分割に戻る）。
func TestDevSimulation_EmptySplitAndFocusMeanNone(t *testing.T) {
	rec := devGet(t, "/api/dev/simulate?declared=bench&split=&focus=&frequency=2&weeks=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Days []struct {
			Split string `json:"split"`
		} `json:"days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	for _, d := range got.Days {
		if d.Split != "" {
			t.Errorf("分割なしのはずが %q", d.Split)
		}
	}
}

// 1回の量・曜日・開始日もクエリで変えられ、応答は解決した値を返す。
func TestDevSimulation_TakesScheduleAndEchoesIt(t *testing.T) {
	// 曜日は順不同で受け、並べて返す。
	rec := devGet(t, "/api/dev/simulate?declared=bench&weeks=1&days=3,1&exercises=5&sets=4&start=2026-09-07")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Settings struct {
			Frequency int    `json:"frequency"`
			Start     string `json:"start"`
			Exercises int    `json:"exercises_per_session"`
			Sets      int    `json:"sets_per_exercise"`
			Weekdays  []int  `json:"weekdays"`
		} `json:"settings"`
		Days []struct {
			Date string `json:"date"`
		} `json:"days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	s := got.Settings
	// 曜日を指定したら、頻度はその数。
	if s.Frequency != 2 || s.Exercises != 5 || s.Sets != 4 || s.Start != "2026-09-07" {
		t.Errorf("設定が返っていない: %+v", s)
	}
	if len(s.Weekdays) != 2 || s.Weekdays[0] != 1 || s.Weekdays[1] != 3 {
		t.Errorf("曜日が %v。[1 3] のはず", s.Weekdays)
	}
	if len(got.Days) != 2 || got.Days[0].Date != "2026-09-08" {
		t.Errorf("日が %+v。2026-09-08 から2件のはず", got.Days)
	}
}

// 曜日を指定しなければ、頻度ごとの既定の曜日を返す。応答だけで分かるように。
func TestDevSimulation_EchoesDefaultWeekdays(t *testing.T) {
	rec := devGet(t, "/api/dev/simulate?declared=bench&weeks=1&frequency=3")
	var got struct {
		Settings struct {
			Weekdays  []int `json:"weekdays"`
			Exercises int   `json:"exercises_per_session"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	want, _ := devsim.DefaultWeekdays(3)
	if fmt.Sprint(got.Settings.Weekdays) != fmt.Sprint(want) {
		t.Errorf("曜日が %v。既定の %v のはず", got.Settings.Weekdays, want)
	}
	if got.Settings.Exercises != seed.DefaultExercisesPerSession {
		t.Errorf("種目数が %d。既定の %d のはず", got.Settings.Exercises, seed.DefaultExercisesPerSession)
	}
}

// 形の壊れた値は 400。黙って既定値に倒すと、書き間違えた URL が
// 別の設定の結果を返し、読む側（人も Claude も）が気づけない。
//
// 本文は、どの項目が悪いかを名指しする。「400」だけでは、URL を組んだ側が
// どこを直せばよいか分からない。
func TestDevSimulation_RejectsBadSchedule(t *testing.T) {
	for _, c := range []struct{ query, names string }{
		{"exercises=many", "exercises"},
		{"sets=", "sets"},
		{"days=mon", "days"},
		{"days=1,,3", "days"},
		{"days=1,3&frequency=4", "頻度"},
		{"start=someday", "start"},
	} {
		t.Run(c.query, func(t *testing.T) {
			rec := devGet(t, "/api/dev/simulate?declared=bench&"+c.query)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), c.names) {
				t.Errorf("本文が %q を名指ししていない: %s", c.names, rec.Body.String())
			}
		})
	}
}

// 画面が曜日と量の初期値に使う既定値を返す。
func TestDevSimulation_OptionsCarryScheduleDefaults(t *testing.T) {
	rec := devGet(t, "/api/dev/options")
	var got struct {
		Schedule struct {
			Exercises int              `json:"exercises_per_session"`
			Sets      int              `json:"sets_per_exercise"`
			Weekdays  map[string][]int `json:"weekdays_by_frequency"`
			Start     string           `json:"start"`
		} `json:"schedule_defaults"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	s := got.Schedule
	// 開始日の既定。画面が欄の初期値に出す（二重に持たない）。
	if s.Start != "2026-08-03" {
		t.Errorf("開始日の既定が %q。2026-08-03 のはず", s.Start)
	}
	if s.Exercises != seed.DefaultExercisesPerSession || s.Sets != seed.DefaultSetsPerExercise {
		t.Errorf("量の既定が %+v", s)
	}
	for f := 1; f <= 7; f++ {
		want, _ := devsim.DefaultWeekdays(f)
		if fmt.Sprint(s.Weekdays[fmt.Sprint(f)]) != fmt.Sprint(want) {
			t.Errorf("週%d の曜日が %v。%v のはず", f, s.Weekdays[fmt.Sprint(f)], want)
		}
	}
}

// 自分の種目を custom= で渡せること。設定の echo に ID つきで返り、
// 1RM の上書きもその ID で効くこと。
//
// 形は「名前|区分:寄与,区分:寄与|刻み」を ; で並べる。本番の POST
// /api/exercises と同じ、区分ごとの寄与度の生の値を1行で書ける形にした。
func TestDevSimulation_TakesCustomExercisesAndEchoesThem(t *testing.T) {
	custom := "アイソラテラル・ロー|TRAP_MID:1,LAT:0.5,BICEPS:0.5|2.5;アイソラテラル・フロント・プルダウン|LAT:1|2.5"
	rec := devGet(t, "/api/dev/simulate?declared=bench&weeks=2&orm=u-sim01:80&custom="+url.QueryEscape(custom))
	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず: %s", rec.Code, rec.Body.String())
	}

	var got struct {
		Settings struct {
			Custom []struct {
				ID          string             `json:"id"`
				Name        string             `json:"name"`
				Stimulus    map[string]float64 `json:"stimulus"`
				IncrementKg float64            `json:"increment_kg"`
			} `json:"custom"`
			Athlete struct {
				OneRepMaxKg map[string]float64 `json:"one_rep_max_kg"`
			} `json:"athlete"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	c := got.Settings.Custom
	if len(c) != 2 || c[0].ID != "u-sim01" || c[0].Name != "アイソラテラル・ロー" ||
		c[0].Stimulus["TRAP_MID"] != 1 || c[0].Stimulus["LAT"] != 0.5 || c[0].Stimulus["BICEPS"] != 0.5 ||
		c[0].IncrementKg != 2.5 || c[1].ID != "u-sim02" || len(c[1].Stimulus) != 1 {
		t.Errorf("自分の種目が返っていない: %+v", c)
	}
	orm := got.Settings.Athlete.OneRepMaxKg
	if orm["u-sim01"] != 80 || orm["u-sim02"] != devsim.DefaultOneRepMax("u-sim02") {
		t.Errorf("自分の種目の1RM: u-sim01=%v u-sim02=%v", orm["u-sim01"], orm["u-sim02"])
	}
}

// 自分の種目の形が壊れていたら 400。
func TestDevSimulation_RejectsBadCustomQuery(t *testing.T) {
	for _, custom := range []string{
		"名前だけ",
		"a|NECK:1|2.5",
		"a|LAT:heavy|2.5",
		"a|LAT:0.5|2.5",                 // 寄与1.0の区分が無い
		"a|LAT:1|heavy",                 // 刻みが数値でない
		"サイドレイズ|SIDE_DELT:1|1",          // 共通の種目と同名
		"a|TRAP_MID:1,TRAP_MID:0.5|2.5", // 同じ区分の2回指定
		"a||2.5",                        // 寄与が1つも無い
		"a|LAT|2.5",                     // コロンが無い
	} {
		t.Run(custom, func(t *testing.T) {
			rec := devGet(t, "/api/dev/simulate?declared=bench&custom="+url.QueryEscape(custom))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
			// custom を知らないキーとして弾いているだけなら、形を見ていない。
			if strings.Contains(rec.Body.String(), "知らないキー") {
				t.Errorf("形ではなくキーで弾いている: %s", rec.Body.String())
			}
		})
	}
}

// 効き方の書式が壊れている理由は、本文にそのまま出す。
//
// 「custom が不正である」とだけ返すと、区分の重複と桁の書き間違いが
// 見分けられず、読み返した本人が同じ間違いをもう一度探すことになる。
func TestDevSimulation_NamesTheReasonForBadCustomStimulus(t *testing.T) {
	cases := []struct {
		custom string
		want   string
	}{
		{"a|TRAP_MID:1,TRAP_MID:0.5|2.5", "TRAP_MID"},
		{"a|LAT|2.5", "寄与の形式が不正"},
	}
	for _, c := range cases {
		t.Run(c.custom, func(t *testing.T) {
			rec := devGet(t, "/api/dev/simulate?declared=bench&custom="+url.QueryEscape(c.custom))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), c.want) {
				t.Errorf("本文に %q が無い: %s", c.want, rec.Body.String())
			}
		})
	}
}
