package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

type runDTO struct {
	ID              string  `json:"id"`
	Date            string  `json:"date"`
	DistanceKm      float64 `json:"distance_km"`
	DurationSeconds int     `json:"duration_seconds"`
}

func getRuns(t *testing.T, mux http.Handler, query string) []runDTO {
	t.Helper()
	rec := do(t, mux, http.MethodGet, "/api/run-logs"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET のステータスが %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Runs []runDTO `json:"runs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	return body.Runs
}

func postRun(t *testing.T, mux http.Handler, body string) int {
	t.Helper()
	return do(t, mux, http.MethodPost, "/api/run-logs", body).Code
}

// 記録したランが、送ったとおりの値で一覧に出る。
func TestRunLogs_RecordAndList(t *testing.T) {
	mux := newServer(t, true)

	if code := postRun(t, mux,
		`{"id":"r1","date":"2026-08-18","distance_km":5.2,"duration_seconds":1680}`); code != http.StatusNoContent {
		t.Fatalf("POST のステータスが %d。204 のはず", code)
	}

	got := getRuns(t, mux, "?from=2026-08-01&to=2026-08-31")
	want := runDTO{ID: "r1", Date: "2026-08-18", DistanceKm: 5.2, DurationSeconds: 1680}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("一覧が %+v。%+v の1件のはず", got, want)
	}
}

// 期間の外は返さない。境界の日は含む（セットの記録の from/to と同じ）。
func TestRunLogs_ListIsLimitedToThePeriod(t *testing.T) {
	mux := newServer(t, true)
	for _, b := range []string{
		`{"id":"before","date":"2026-07-31","distance_km":3,"duration_seconds":900}`,
		`{"id":"first","date":"2026-08-01","distance_km":3,"duration_seconds":900}`,
		`{"id":"last","date":"2026-08-31","distance_km":3,"duration_seconds":900}`,
		`{"id":"after","date":"2026-09-01","distance_km":3,"duration_seconds":900}`,
	} {
		if code := postRun(t, mux, b); code != http.StatusNoContent {
			t.Fatalf("POST のステータスが %d", code)
		}
	}

	got := getRuns(t, mux, "?from=2026-08-01&to=2026-08-31")
	if len(got) != 2 || got[0].ID != "first" || got[1].ID != "last" {
		t.Fatalf("一覧が %+v。first と last の2件のはず", got)
	}
}

// 何も無いときも runs は null ではなく空の配列。
func TestRunLogs_EmptyListIsNotNull(t *testing.T) {
	mux := newServer(t, true)
	rec := do(t, mux, http.MethodGet, "/api/run-logs?from=2026-08-01&to=2026-08-31", "")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if got := string(raw["runs"]); got != "[]" {
		t.Errorf("runs が %s。[] のはず", got)
	}
}

// 再送は成功、同じIDで内容が違えば 409。
//
// 画面はオフラインで記録して後から送るので、同じ記録が二度届くのは日常。
// 内容が違うのは採番の誤りで、黙って上書きするとどちらが正しいか分からない。
func TestRunLogs_ResendIsIdempotentAndConflictIs409(t *testing.T) {
	mux := newServer(t, true)
	body := `{"id":"r1","date":"2026-08-18","distance_km":5.2,"duration_seconds":1680}`
	for i := range 2 {
		if code := postRun(t, mux, body); code != http.StatusNoContent {
			t.Fatalf("%d回目の POST が %d。204 のはず", i+1, code)
		}
	}
	if code := postRun(t, mux,
		`{"id":"r1","date":"2026-08-18","distance_km":10,"duration_seconds":3600}`); code != http.StatusConflict {
		t.Errorf("内容の違う再送が %d。409 のはず", code)
	}
	if got := getRuns(t, mux, "?from=2026-08-01&to=2026-08-31"); len(got) != 1 || got[0].DistanceKm != 5.2 {
		t.Errorf("一覧が %+v。最初の 5.2km の1件のままのはず", got)
	}
}

func TestRunLogs_RejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		// 欠けた数値を 0 と読むと、0km のランとしてではなくコンストラクタで
		// 弾かれるが、エラーの本文が「距離が範囲外」になり原因が分からない。
		{"距離が無い", `{"id":"r1","date":"2026-08-18","duration_seconds":1680}`},
		{"時間が無い", `{"id":"r1","date":"2026-08-18","distance_km":5}`},
		{"日付が読めない", `{"id":"r1","date":"2026/08/18","distance_km":5,"duration_seconds":1680}`},
		{"IDが空", `{"id":"","date":"2026-08-18","distance_km":5,"duration_seconds":1680}`},
		{"距離が0", `{"id":"r1","date":"2026-08-18","distance_km":0,"duration_seconds":1680}`},
		{"時間が0", `{"id":"r1","date":"2026-08-18","distance_km":5,"duration_seconds":0}`},
		{"JSONが壊れている", `{"id":`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code := postRun(t, newServer(t, true), c.body); code != http.StatusBadRequest {
				t.Errorf("ステータスが %d。400 のはず", code)
			}
		})
	}
}

// 消した記録は一覧から消え、二度消しても 204（再送で「消せない」にしない）。
func TestRunLogs_Delete(t *testing.T) {
	mux := newServer(t, true)
	if code := postRun(t, mux,
		`{"id":"r1","date":"2026-08-18","distance_km":5.2,"duration_seconds":1680}`); code != http.StatusNoContent {
		t.Fatalf("POST のステータスが %d", code)
	}
	for i := range 2 {
		if rec := do(t, mux, http.MethodDelete, "/api/run-logs/r1", ""); rec.Code != http.StatusNoContent {
			t.Fatalf("%d回目の DELETE が %d。204 のはず", i+1, rec.Code)
		}
	}
	if got := getRuns(t, mux, "?from=2026-08-01&to=2026-08-31"); len(got) != 0 {
		t.Errorf("消したのに一覧に %+v がある", got)
	}
}
