package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

const testToken = "0123456789abcdef0123456789abcdef"

// guarded は認証を通した先で反応するハンドラを返す。
func guarded(t *testing.T) (http.Handler, *bool) {
	t.Helper()
	var reached bool
	h := httpapi.RequireBearerToken(testToken)(
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

func TestRequireBearerToken_AcceptsTheToken(t *testing.T) {
	h, reached := guarded(t)

	rec := request(t, h, "/api/sessions", "Bearer "+testToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("正しいトークンが弾かれた: %d body=%s", rec.Code, rec.Body.String())
	}
	if !*reached {
		t.Error("ハンドラに到達していない")
	}
}

// 方式名の大文字小文字を無視すること（RFC 7235）。
// 弾くと、原因の分からない 401 になる。
func TestRequireBearerToken_IgnoresSchemeCase(t *testing.T) {
	for _, scheme := range []string{"Bearer", "bearer", "BEARER", "BeArEr"} {
		h, reached := guarded(t)
		rec := request(t, h, "/api/sessions", scheme+" "+testToken)
		if rec.Code != http.StatusNoContent || !*reached {
			t.Errorf("%q が弾かれた: %d", scheme, rec.Code)
		}
	}
}

func TestRequireBearerToken_RejectsEverythingElse(t *testing.T) {
	cases := map[string]string{
		"ヘッダが無い":      "",
		"方式が違う":       "Basic " + testToken,
		"トークンが違う":     "Bearer ちがうトークン",
		"トークンが空":      "Bearer ",
		"方式だけ":        "Bearer",
		"接頭辞が一致する別物":  "Bearer " + testToken + "x",
		"接頭辞だけ一致":     "Bearer " + testToken[:8],
		"生のトークンだけ":    testToken,
		"前後に余計なものがある": "Bearer " + testToken + " extra",
	}
	for name, authorization := range cases {
		t.Run(name, func(t *testing.T) {
			h, reached := guarded(t)
			rec := request(t, h, "/api/sessions", authorization)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("401 でない: %d body=%s", rec.Code, rec.Body.String())
			}
			if *reached {
				t.Error("認証に失敗したのにハンドラへ到達した")
			}
		})
	}
}

// クライアントが「認証が要る」と「壊れている」を区別できること。
func TestRequireBearerToken_AdvertisesTheScheme(t *testing.T) {
	h, _ := guarded(t)
	rec := request(t, h, "/api/sessions", "")

	if got := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(got, "Bearer") {
		t.Errorf("WWW-Authenticate が付いていない: %q", got)
	}
}

// ヘルスチェックは認証しないこと。
// 認証すると、起動プローブが 401 を受けてトラフィックが来なくなる。
func TestRequireBearerToken_LetsHealthChecksThrough(t *testing.T) {
	h, reached := guarded(t)

	rec := request(t, h, httpapi.HealthPath, "")
	if rec.Code != http.StatusNoContent {
		t.Errorf("ヘルスチェックが弾かれた: %d", rec.Code)
	}
	if !*reached {
		t.Error("ヘルスチェックがハンドラへ到達していない")
	}
}

// 素通しするのは決めた経路の完全一致だけであること。
// 前方一致で判定していると /health-secret のような経路が開く。
//
// /index.html と /app.js.map は、画面を同居させていた頃の名残。
// いまは配っていないので経路として存在しないが、素通しの一覧に
// 紛れ込んでいないことを見る意味は残る（D-119）。
func TestRequireBearerToken_OnlyExemptsExactPaths(t *testing.T) {
	for _, path := range []string{
		"/health/../api/sessions", "/healthz", "/health-secret",
		"/api/sessions", "/api/program", "/api/program/focus", "/api/program/declared", "/api/program/frequency", "/api/program/selected", "/api/program/target", "/api/program/split",
		"/api/split-presets",
		"/app.js.map", "/index.html",
	} {
		h, reached := guarded(t)
		rec := request(t, h, path, "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s が認証なしで通った: %d", path, rec.Code)
		}
		if *reached {
			t.Errorf("%s がハンドラへ到達した", path)
		}
	}
}

// エラー応答からトークンが漏れないこと。
func TestRequireBearerToken_DoesNotEchoTheToken(t *testing.T) {
	h, _ := guarded(t)
	rec := request(t, h, "/api/sessions", "Bearer 秘密にしたい値")

	if strings.Contains(rec.Body.String(), "秘密にしたい値") {
		t.Errorf("送られたトークンが応答に載っている: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), testToken) {
		t.Errorf("正しいトークンが応答に載っている: %s", rec.Body.String())
	}
}

// ヘルスチェックの経路が /healthz でないこと。
//
// Cloud Run のフロントエンドは /healthz を完全一致で横取りし、
// Google の 404 を返す。アプリまで届かないので、DB の疎通を含めた
// ヘルスチェック（D-061）が本番で機能しなくなる。
func TestHealthPath_IsNotReservedByCloudRun(t *testing.T) {
	if httpapi.HealthPath == "/healthz" {
		t.Error("/healthz は Cloud Run が横取りするので使えない")
	}
	if httpapi.HealthPath == "" {
		t.Error("ヘルスチェックの経路が空である")
	}
}
