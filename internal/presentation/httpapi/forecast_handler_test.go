package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// 応答の形：index・split はあるが date が無いこと
// （設計書「日付は返さない」）。重量が付かない種目は weight_kg が
// null のまま通ること（履歴が無いので全種目が null のはず）。
func TestGetForecast_Success(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var raw struct {
		Sessions []map[string]json.RawMessage `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(raw.Sessions) != 3 { // newServer(t, true) は週3
		t.Fatalf("回数が %d。週3のはず", len(raw.Sessions))
	}
	for i, s := range raw.Sessions {
		if _, ok := s["date"]; ok {
			t.Errorf("回%d: date が応答に含まれている", i)
		}
		if _, ok := s["split"]; !ok {
			t.Errorf("回%d: split のキーが無い", i)
		}
	}

	var body struct {
		Sessions []struct {
			Index int     `json:"index"`
			Split *string `json:"split"`
			Main  []struct {
				ExerciseID string   `json:"exercise_id"`
				WeightKg   *float64 `json:"weight_kg"`
			} `json:"main"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	for i, s := range body.Sessions {
		if s.Index != i {
			t.Errorf("index が %d 番目の要素で %d", i, s.Index)
		}
	}
	// 履歴が無いので全回で重量は null のはず。
	for i, s := range body.Sessions {
		for _, m := range s.Main {
			if m.WeightKg != nil {
				t.Errorf("回%d: 履歴が無いのに %s の重量が入っている", i, m.ExerciseID)
			}
		}
	}
	// newServer の既定プログラムは分割なし。
	if body.Sessions[0].Split != nil {
		t.Errorf("分割なしなのに split が %v", *body.Sessions[0].Split)
	}
}

func TestGetForecast_MissingDate(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions/forecast", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("日付なしが 400 にならない: %d", rec.Code)
	}
}

func TestGetForecast_BadDate(t *testing.T) {
	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions/forecast?date=2026/08/17", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("不正な日付が 400 にならない: %d", rec.Code)
	}
}

func TestGetForecast_ProgramNotConfigured(t *testing.T) {
	rec := do(t, newServer(t, false), http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
	if rec.Code != http.StatusConflict {
		t.Errorf("未設定が 409 にならない: %d", rec.Code)
	}
}

// 分割ありプログラムでは split に日の名前が入ること。
func TestGetForecast_SplitNameAppears(t *testing.T) {
	mux := newServer(t, true)
	putUpperLowerSplit(t, mux)

	rec := do(t, mux, http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Sessions []struct {
			Split *string `json:"split"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if body.Sessions[0].Split == nil || *body.Sessions[0].Split != "上半身" {
		t.Errorf("回0の split が %v。上半身のはず", body.Sessions[0].Split)
	}
}

// 有酸素の日は「何回目の後の、休みの何日目か」で返り、日付を持たない
// （予定の回と同じく、仮置きの日付は返さない）。
//
// 上下2分割の週3回。練習は 上(0)・下(2)・上(5) と仮置きされ、休みは
// 1・3（下の翌日）・4・6。下の翌日を避けて 1 と 4 を取る。
func TestGetForecast_Cardio(t *testing.T) {
	mux := newServer(t, true)
	putUpperLowerSplit(t, mux)

	rec := do(t, mux, http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var raw struct {
		Cardio []map[string]json.RawMessage `json:"cardio"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	for i, c := range raw.Cardio {
		if _, ok := c["date"]; ok {
			t.Errorf("有酸素%d: date が応答に含まれている", i)
		}
	}

	var body struct {
		Cardio []struct {
			AfterIndex int `json:"after_index"`
			RestDay    int `json:"rest_day"`
		} `json:"cardio"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	want := []struct{ afterIndex, restDay int }{{0, 1}, {1, 2}}
	if len(body.Cardio) != len(want) {
		t.Fatalf("有酸素の日が %d 日。%d 日のはず: %s", len(body.Cardio), len(want), rec.Body.String())
	}
	for i, w := range want {
		if g := body.Cardio[i]; g.AfterIndex != w.afterIndex || g.RestDay != w.restDay {
			t.Errorf("有酸素%d が %d回目の後の%d日目。%d回目の後の%d日目のはず",
				i, g.AfterIndex, g.RestDay, w.afterIndex, w.restDay)
		}
	}
}

// 休みが無い（毎日練習する）ときも cardio は null ではなく空の配列。
// 画面が null を配列として回すと落ちる。
func TestGetForecast_CardioIsNeverNull(t *testing.T) {
	mux := newServer(t, true)
	rec := do(t, mux, http.MethodPut, "/api/program/frequency", `{"per_week":7}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("頻度の更新に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec = do(t, mux, http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if got := string(raw["cardio"]); got != "[]" {
		t.Errorf("cardio が %s。[] のはず", got)
	}
}
