package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// todaysAccessory は今日の補助の先頭を返す。newServer(t, true) は1回6種目
// なので補助が必ずある。
func todaysAccessory(t *testing.T, h http.Handler, date string) string {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/sessions?date="+date, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("前提: 今日の計画が取れない: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Accessories []struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"accessories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(body.Accessories) == 0 {
		t.Fatal("前提: 今日の補助が無い")
	}
	return body.Accessories[0].ExerciseID
}

func TestGetSubstitute_ReturnsAnotherAccessory(t *testing.T) {
	h := newServer(t, true)
	replace := todaysAccessory(t, h, "2026-08-17")

	rec := do(t, h, http.MethodGet,
		"/api/sessions/substitute?date=2026-08-17&replace="+url.QueryEscape(replace), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if _, ok := raw["substitute"]; !ok {
		t.Fatalf("substitute のキーが無い: %s", rec.Body.String())
	}
	var body struct {
		Substitute *struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
			Sets       int      `json:"sets"`
		} `json:"substitute"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if body.Substitute == nil {
		t.Fatal("同梱の全種目から選べるのに代わりが null")
	}
	if body.Substitute.ExerciseID == replace {
		t.Errorf("代わりが元の種目 %s と同じ", replace)
	}
	if body.Substitute.Sets <= 0 {
		t.Errorf("セット数が %d", body.Substitute.Sets)
	}
	// 履歴が無いので重量は null（自分で決める）。
	if body.Substitute.WeightKg != nil {
		t.Errorf("履歴が無いのに重量 %v が付いている", *body.Substitute.WeightKg)
	}
}

// rejected は繰り返しのクエリで受ける。出た代わりを rejected に積んでいくと
// 毎回別の種目が出る。3回回すのは、2つ以上積んだときを見るため（1つだけなら
// 先頭だけを読む実装でも通る）。
//
// 【変異】ハンドラで rejected を Query().Get（先頭1つだけ）で読む → 2つ目に
// 積んだ種目が素通りして、それが返ってきて赤。
func TestGetSubstitute_RejectedIsRepeatable(t *testing.T) {
	h := newServer(t, true)
	replace := todaysAccessory(t, h, "2026-08-17")
	base := "/api/sessions/substitute?date=2026-08-17&replace=" + url.QueryEscape(replace)

	var seen []string
	for range 3 {
		q := base
		for _, id := range seen {
			q += "&rejected=" + url.QueryEscape(id)
		}
		rec := do(t, h, http.MethodGet, q, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Substitute *struct {
				ExerciseID string `json:"exercise_id"`
			} `json:"substitute"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSONが壊れている: %v", err)
		}
		if body.Substitute == nil {
			t.Fatalf("%d回目で代わりが null", len(seen)+1)
		}
		for _, id := range seen {
			if body.Substitute.ExerciseID == id {
				t.Fatalf("rejected に積んだ %s が出た（rejected=%v）", id, seen)
			}
		}
		seen = append(seen, body.Substitute.ExerciseID)
	}
}

func TestGetSubstitute_BadRequests(t *testing.T) {
	h := newServer(t, true)
	cases := []struct {
		name string
		path string
	}{
		{"日付なし", "/api/sessions/substitute?replace=leg_curl"},
		{"不正な日付", "/api/sessions/substitute?date=2026/08/17&replace=leg_curl"},
		{"replace なし", "/api/sessions/substitute?date=2026-08-17"},
		{"空の rejected", "/api/sessions/substitute?date=2026-08-17&replace=leg_curl&rejected="},
		{"軸は差し替えない", "/api/sessions/substitute?date=2026-08-17&replace=bench"},
		{"予定に無い種目", "/api/sessions/substitute?date=2026-08-17&replace=no_such_exercise"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, c.path, "")
			if rec.Code != http.StatusBadRequest {
				t.Errorf("ステータス = %d, want 400 body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGetSubstitute_NotConfigured(t *testing.T) {
	rec := do(t, newServer(t, false), http.MethodGet,
		"/api/sessions/substitute?date=2026-08-17&replace=leg_curl", "")
	if rec.Code != http.StatusConflict {
		t.Errorf("未設定が 409 にならない: %d", rec.Code)
	}
}

// 候補が尽きれば substitute は null で返る（200）。ほかの種目を全部 rejected に
// 積んで尽きさせる。
//
// 【変異】ハンドラの `if found` を `if found || true` にする → 空の種目の
// 処方が返り、null にならず赤。
func TestGetSubstitute_NullWhenNothingIsLeft(t *testing.T) {
	h := newServer(t, true)
	replace := todaysAccessory(t, h, "2026-08-17")

	rec := do(t, h, http.MethodGet, "/api/exercises", "")
	var catalog struct {
		Exercises []struct {
			ID string `json:"id"`
		} `json:"exercises"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	q := "/api/sessions/substitute?date=2026-08-17&replace=" + url.QueryEscape(replace)
	for _, e := range catalog.Exercises {
		if e.ID != replace {
			q += "&rejected=" + url.QueryEscape(e.ID)
		}
	}

	rec = do(t, h, http.MethodGet, q, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "{\"substitute\":null}\n" {
		t.Errorf("応答 = %q, want {\"substitute\":null}", got)
	}
}
