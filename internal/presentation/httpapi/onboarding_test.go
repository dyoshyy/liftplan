package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func getOnboarding(t *testing.T, h http.Handler) bool {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/onboarding", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Completed *bool `json:"completed"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答を解釈できない: %v", err)
	}
	if got.Completed == nil {
		t.Fatalf("completed が無い: %s", rec.Body.String())
	}
	return *got.Completed
}

// 初回の案内は、済ませるまで completed=false を返し、済ませたら true になる。
//
// PUT は冪等。画面は送信に失敗すると最初から送り直すので、2回目を拒むと
// 済んだのに「済ませられない」と出る。
func TestOnboarding_StartsPendingAndCompletes(t *testing.T) {
	h := newServer(t, true)

	if getOnboarding(t, h) {
		t.Fatal("始めは completed=false であること")
	}
	for i := range 2 {
		rec := do(t, h, http.MethodPut, "/api/onboarding/completed", "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%d回目の PUT のステータスが誤り: %d body=%s", i+1, rec.Code, rec.Body.String())
		}
	}
	if !getOnboarding(t, h) {
		t.Fatal("PUT のあとは completed=true であること")
	}
}
