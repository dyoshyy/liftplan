package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
