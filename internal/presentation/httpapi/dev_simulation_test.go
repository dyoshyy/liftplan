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
			if code := devGet(t, path).Code; code != http.StatusBadRequest {
				t.Errorf("%d が返った。400 のはず", code)
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
