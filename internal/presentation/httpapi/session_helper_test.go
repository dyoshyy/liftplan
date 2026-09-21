package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

// requireTestSession は既定ユーザーのセッションを1件だけ持つ認証。
//
// 既定ユーザーにするのは、テストが用意するプログラムの持ち主と
// 揃える必要があるため。揃っていないと、設定したはずのプログラムが
// 読めず「未設定」になる。
func requireTestSession(t *testing.T) func(http.Handler) http.Handler {
	t.Helper()
	return httpapi.RequireSession(
		storeWith(t, sampleToken, testUser),
		func() time.Time { return authNow },
	)
}

// guarded は認証を通した先で反応するハンドラを返す。
func guarded(t *testing.T) (http.Handler, *bool) {
	t.Helper()
	var reached bool
	h := requireTestSession(t)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			reached = true
			w.WriteHeader(http.StatusNoContent)
		}))
	return h, &reached
}

func request(t *testing.T, h http.Handler, path, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if authorization != "" {
		r.Header.Set("Authorization", authorization)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}
