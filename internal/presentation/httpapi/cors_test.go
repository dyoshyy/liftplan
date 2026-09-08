package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/presentation/httpapi"
)

const allowed = "https://liftplan-web.example.workers.dev"

// corsHandler は「認証を必ず要求するハンドラ」を CORS で包んだもの。
//
// 実物と同じ重ね順にしてある。preflight が認証の内側に落ちると 401 になり、
// ブラウザからは原因の分からない失敗になる。
func corsHandler(t *testing.T) http.Handler {
	t.Helper()
	inner := httpapi.RequireBearerToken("0123456789abcdef0123456789abcdef")(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)
	return httpapi.AllowOrigins([]string{allowed})(inner)
}

func preflight(t *testing.T, origin, method string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodOptions, "/api/set-logs", nil)
	r.Header.Set("Origin", origin)
	r.Header.Set("Access-Control-Request-Method", method)
	r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	rec := httptest.NewRecorder()
	corsHandler(t).ServeHTTP(rec, r)
	return rec
}

// preflight には Authorization が付かない。ブラウザが付けないので、
// 認証の内側で処理すると必ず 401 になる。
func TestAllowOrigins_PreflightSucceedsWithoutCredentials(t *testing.T) {
	rec := preflight(t, allowed, http.MethodPost)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight のステータスが %d（期待 204）", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Errorf("Allow-Origin が %q（期待 %q）", got, allowed)
	}
	if rec.Header().Get("Access-Control-Max-Age") == "" {
		t.Error("Max-Age が無い。preflight が毎回飛ぶ")
	}
}

// ルータは "POST /api/set-logs" のようなメソッド付きパターンで登録している。
// OPTIONS をミドルウェアで完結させないと、ルータまで届いて 405 で落ちる。
func TestAllowOrigins_PreflightDoesNotReachTheRouter(t *testing.T) {
	reached := false
	h := httpapi.AllowOrigins([]string{allowed})(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }),
	)

	r := httptest.NewRequest(http.MethodOptions, "/api/set-logs", nil)
	r.Header.Set("Origin", allowed)
	r.Header.Set("Access-Control-Request-Method", http.MethodPost)
	h.ServeHTTP(httptest.NewRecorder(), r)

	if reached {
		t.Error("preflight が内側のハンドラまで届いている")
	}
}

// 許可していないオリジンに応答ヘッダを付けると、そのサイトから
// Bearer トークン付きの API を読めるようになる。
func TestAllowOrigins_RejectsUnknownOrigin(t *testing.T) {
	rec := preflight(t, "https://evil.example.com", http.MethodPost)

	if rec.Code != http.StatusForbidden {
		t.Errorf("ステータスが %d（期待 403）", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin が付いている: %q", got)
	}
}

// Vary が無いと、途中のキャッシュが別オリジン向けの応答を使い回す。
func TestAllowOrigins_AlwaysVariesOnOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	corsHandler(t).ServeHTTP(rec, r)

	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary が %q（期待 Origin）", got)
	}
}

// 本番の GET/POST はオリジンを許可したうえで認証を通す。
func TestAllowOrigins_AllowsTheRealRequest(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	r.Header.Set("Origin", allowed)
	r.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	rec := httptest.NewRecorder()
	corsHandler(t).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが %d（期待 200）", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowed {
		t.Errorf("Allow-Origin が %q（期待 %q）", got, allowed)
	}
}

// オリジンが無い要求（curl やヘルスチェック）はそのまま通す。
func TestAllowOrigins_PassesRequestsWithoutOrigin(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	r.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	rec := httptest.NewRecorder()
	corsHandler(t).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが %d（期待 200）", rec.Code)
	}
}
