package httpapi_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

// ---- 差し込むもの ----

type stubProvider struct {
	name    account.Provider
	authURL string
	subject string
	// email は確認済みとして渡されるアドレス。空なら渡さない。
	email string
	err   error
	// calls はコード交換が呼ばれた回数。呼ばれてはいけない経路を押さえる。
	calls int
}

func (p *stubProvider) Name() account.Provider          { return p.name }
func (p *stubProvider) AuthCodeURL(state string) string { return p.authURL + "?state=" + state }
func (p *stubProvider) Identity(_ context.Context, code string) (account.Identity, error) {
	p.calls++
	if p.err != nil {
		return account.Identity{}, p.err
	}
	return account.NewIdentity(p.name, p.subject+":"+code, account.NewEmail(p.email))
}

type stubSignIn struct {
	token string
	err   error
	// seen は受け取った identity。誰として受け入れたかを確かめる。
	seen []string
}

func (s *stubSignIn) Execute(
	_ context.Context, id account.Identity, _ time.Time,
) (account.SessionToken, error) {
	s.seen = append(s.seen, id.Provider().String()+"/"+id.Subject()+"/"+id.Email().String())
	if s.err != nil {
		return account.SessionToken{}, s.err
	}
	return account.ParseSessionToken(s.token)
}

type stubSessions struct {
	deleted []string
	err     error
}

func (s *stubSessions) Create(context.Context, *account.Session) error { return nil }
func (s *stubSessions) Delete(_ context.Context, h account.TokenHash) error {
	s.deleted = append(s.deleted, h.String())
	return s.err
}

// 43文字の base64url。account.NewSessionToken が返す形に合わせる。
const sampleToken = "abcdefghijklmnopqrstuvwxyz0123456789-_ABCDE"

func newAuth(t *testing.T, provider *stubProvider, signIn *stubSignIn, sessions *stubSessions) (
	http.Handler, *stubProvider,
) {
	t.Helper()

	h, err := httpapi.NewAuthHandler(httpapi.AuthConfig{
		Providers: []httpapi.IdentityProvider{provider},
		SignIn:    signIn,
		Sessions:  sessions,
		WebOrigin: "https://liftplan.example",
		Now:       func() time.Time { return time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("認証ハンドラを組めない: %v", err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, provider
}

func githubProvider() *stubProvider {
	return &stubProvider{
		name:    account.GitHub(),
		authURL: "https://github.example/login/oauth/authorize",
		subject: "12345",
		email:   "me@example.com",
	}
}

// ---- 入口 ----

// 認可画面へ送るとき、state を作って Cookie に置くこと。
//
// Cookie に置かないと、コールバックで突き合わせる相手が無くなる。
// 突き合わせないと、攻撃者が用意したコードで本人のブラウザに
// セッションを作らせられる（CSRF）。
func TestAuthStart_PutsStateInACookieAndRedirects(t *testing.T) {
	mux, _ := newAuth(t, githubProvider(), &stubSignIn{token: sampleToken}, &stubSessions{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/github/start", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("状態が %d。302 のはず", rec.Code)
	}

	cookie := findCookie(t, rec.Result().Cookies(), "github")
	switch {
	case cookie.Value == "":
		t.Error("state の Cookie が空である")
	case !cookie.HttpOnly:
		t.Error("state の Cookie に HttpOnly が無い。JS から読めてしまう")
	case !cookie.Secure:
		t.Error("state の Cookie に Secure が無い。平文で流れうる")
	case cookie.SameSite != http.SameSiteLaxMode:
		t.Error("SameSite が Lax でない。Strict だと戻りの遷移で送られず、None だと守りが消える")
	case !strings.HasPrefix(cookie.Path, "/auth"):
		t.Errorf("Cookie の Path が %q。/auth に限らないと API の要求にも毎回付く", cookie.Path)
	case cookie.MaxAge <= 0:
		t.Error("state の Cookie に期限が無い。ブラウザを閉じるまで残る")
	}

	// 送り先に、その state がそのまま載っていること。
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, cookie.Value) {
		t.Errorf("送り先 %q に state が載っていない", loc)
	}
}

// 知らないプロバイダは 404。
func TestAuthStart_RejectsUnknownProvider(t *testing.T) {
	mux, _ := newAuth(t, githubProvider(), &stubSignIn{token: sampleToken}, &stubSessions{})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/twitter/start", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("状態が %d。404 のはず", rec.Code)
	}
}

// ---- 戻り ----

// 正常系。トークンは**フラグメント**に載せること。
//
// クエリに載せると Referer とアクセスログに残る。トークンは
// 90日効くので、ログに残った時点で誰でも本人になれる。
func TestAuthCallback_RedirectsWithTheTokenInTheFragment(t *testing.T) {
	signIn := &stubSignIn{token: sampleToken}
	mux, provider := newAuth(t, githubProvider(), signIn, &stubSessions{})

	rec := callback(t, mux, "/auth/github/callback", "state-1", "state-1", "code-9")

	if rec.Code != http.StatusFound {
		t.Fatalf("状態が %d。302 のはず（本体 %s）", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if want := "https://liftplan.example/#token=" + sampleToken; loc != want {
		t.Errorf("送り先が %q。%q のはず", loc, want)
	}
	if strings.Contains(loc, "?token=") {
		t.Error("トークンがクエリに載っている。Referer とアクセスログに残る")
	}
	if provider.calls != 1 {
		t.Errorf("コード交換が %d 回。1回のはず", provider.calls)
	}
	if len(signIn.seen) != 1 || signIn.seen[0] != "github/12345:code-9/me@example.com" {
		t.Errorf("受け入れた identity が %v", signIn.seen)
	}
}

// state が合わないなら、コードを交換しないこと。
//
// 交換してからセッションを作らないだけでは足りない。攻撃者のコードを
// 本人のブラウザ経由で使わせている時点で、その先の副作用は止められない。
func TestAuthCallback_RefusesMismatchedState(t *testing.T) {
	cases := []struct {
		name         string
		cookie       string
		query        string
		wantStatus   int
		wantExchange int
	}{
		{"state が食い違う", "state-1", "state-2", http.StatusBadRequest, 0},
		{"Cookie が無い", "", "state-2", http.StatusBadRequest, 0},
		{"クエリに state が無い", "state-1", "", http.StatusBadRequest, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mux, provider := newAuth(t, githubProvider(), &stubSignIn{token: sampleToken}, &stubSessions{})

			rec := callback(t, mux, "/auth/github/callback", c.cookie, c.query, "code-9")

			if rec.Code != c.wantStatus {
				t.Errorf("状態が %d。%d のはず", rec.Code, c.wantStatus)
			}
			if provider.calls != c.wantExchange {
				t.Errorf("コード交換が %d 回。%d 回のはず", provider.calls, c.wantExchange)
			}
		})
	}
}

// プロバイダ側の失敗を、送り直せるかどうかで writeup する。
func TestAuthCallback_MapsFailures(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		query string
		want  int
	}{
		{
			// プロバイダが落ちている・繋がらない。あとで押し直せば通る。
			name: "プロバイダに届かない", err: fmt.Errorf("%w: 503", apperror.ErrUnavailable),
			want: http.StatusServiceUnavailable,
		},
		{
			// 識別子が取れない。押し直しても同じなので、やり直しを促さない。
			name: "識別子が取れない", err: fmt.Errorf("%w: sub が無い", apperror.ErrInvalidInput),
			want: http.StatusBadRequest,
		},
		{
			// 本人が同意画面で断った。エラーではないが、受け入れるものも無い。
			name: "本人が断った", query: "error=access_denied", want: http.StatusBadRequest,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := githubProvider()
			p.err = c.err
			mux, _ := newAuth(t, p, &stubSignIn{token: sampleToken}, &stubSessions{})

			target := "/auth/github/callback?state=s&code=c"
			if c.query != "" {
				target += "&" + c.query
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			req.AddCookie(&http.Cookie{Name: stateCookieName("github"), Value: "s"})

			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != c.want {
				t.Errorf("状態が %d。%d のはず（本体 %s）", rec.Code, c.want, rec.Body.String())
			}
			if loc := rec.Header().Get("Location"); strings.Contains(loc, "token=") {
				t.Errorf("失敗したのにトークンを渡している: %q", loc)
			}
		})
	}
}

// 受け入れに失敗したら、トークンを渡さないこと。
func TestAuthCallback_PassesNoTokenWhenSignInFails(t *testing.T) {
	signIn := &stubSignIn{err: fmt.Errorf("%w: 保存できない", apperror.ErrUnavailable)}
	mux, _ := newAuth(t, githubProvider(), signIn, &stubSessions{})

	rec := callback(t, mux, "/auth/github/callback", "s", "s", "c")

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("状態が %d。503 のはず", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Errorf("失敗したのに送り先を返している: %q", loc)
	}
}

// 使い終わった state の Cookie を消すこと。
//
// 残すと、同じ state でもう一度コールバックを踏める。
func TestAuthCallback_ClearsTheStateCookie(t *testing.T) {
	mux, _ := newAuth(t, githubProvider(), &stubSignIn{token: sampleToken}, &stubSessions{})

	rec := callback(t, mux, "/auth/github/callback", "s", "s", "c")

	cookie := findCookie(t, rec.Result().Cookies(), "github")
	if cookie.MaxAge >= 0 && cookie.Value != "" {
		t.Errorf("state の Cookie が消えていない: %+v", cookie)
	}
}

// ---- ログアウト ----

// 端末のトークンに対応するセッションを消すこと。
func TestAuthLogout_DeletesTheSession(t *testing.T) {
	sessions := &stubSessions{}
	mux, _ := newAuth(t, githubProvider(), &stubSignIn{token: sampleToken}, sessions)

	req := httptest.NewRequest(http.MethodDelete, "/auth/session", nil)
	req.Header.Set("Authorization", "Bearer "+sampleToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("状態が %d。204 のはず（本体 %s）", rec.Code, rec.Body.String())
	}

	token, err := account.ParseSessionToken(sampleToken)
	if err != nil {
		t.Fatalf("トークンが不正: %v", err)
	}
	if len(sessions.deleted) != 1 || sessions.deleted[0] != token.Hash().String() {
		t.Errorf("消した先が %v。トークンのハッシュのはず", sessions.deleted)
	}
}

// 消えているものをもう一度消しても成功として扱う。再送で二度目が来る。
func TestAuthLogout_IsIdempotent(t *testing.T) {
	mux, _ := newAuth(t, githubProvider(), &stubSignIn{token: sampleToken}, &stubSessions{})

	for range 2 {
		req := httptest.NewRequest(http.MethodDelete, "/auth/session", nil)
		req.Header.Set("Authorization", "Bearer "+sampleToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("状態が %d。204 のはず", rec.Code)
		}
	}
}

// 認証の無いログアウトは 401。
//
// 誰でも叩けると、トークンのハッシュを総当たりする口になる。
// 認証を通す経路の一覧（8b で差し替えるミドルウェア）から /auth を
// まるごと外すと、ここが静かに開く。
func TestAuthLogout_RequiresAToken(t *testing.T) {
	sessions := &stubSessions{}
	mux, _ := newAuth(t, githubProvider(), &stubSignIn{token: sampleToken}, sessions)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/auth/session", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("状態が %d。401 のはず", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Error("WWW-Authenticate が無い。「認証が要る」と「壊れている」を区別できない")
	}
	if len(sessions.deleted) != 0 {
		t.Errorf("認証なしで %v を消している", sessions.deleted)
	}
}

// ---- 組み立て ----

// 設定が欠けていたら組めないこと。起動を止めるための口。
func TestNewAuthHandler_RefusesIncompleteConfig(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(*httpapi.AuthConfig)
	}{
		{"プロバイダが1つも無い", func(c *httpapi.AuthConfig) { c.Providers = nil }},
		{"画面のオリジンが無い", func(c *httpapi.AuthConfig) { c.WebOrigin = "" }},
		{"受け入れ口が無い", func(c *httpapi.AuthConfig) { c.SignIn = nil }},
		{"セッションの口が無い", func(c *httpapi.AuthConfig) { c.Sessions = nil }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := httpapi.AuthConfig{
				Providers: []httpapi.IdentityProvider{githubProvider()},
				SignIn:    &stubSignIn{token: sampleToken},
				Sessions:  &stubSessions{},
				WebOrigin: "https://liftplan.example",
			}
			c.break_(&cfg)

			if _, err := httpapi.NewAuthHandler(cfg); err == nil {
				t.Error("欠けた設定で組めてしまった")
			}
		})
	}
}

// ---- ヘルパ ----

func stateCookieName(provider string) string { return "liftplan_oauth_state_" + provider }

func findCookie(t *testing.T, cookies []*http.Cookie, provider string) *http.Cookie {
	t.Helper()
	for _, c := range cookies {
		if c.Name == stateCookieName(provider) {
			return c
		}
	}
	t.Fatalf("state の Cookie が無い: %v", cookies)
	return nil
}

func callback(
	t *testing.T, mux http.Handler, path, cookieState, queryState, code string,
) *httptest.ResponseRecorder {
	t.Helper()

	target := path + "?code=" + code
	if queryState != "" {
		target += "&state=" + queryState
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if cookieState != "" {
		req.AddCookie(&http.Cookie{
			Name:  stateCookieName(strings.Split(strings.TrimPrefix(path, "/auth/"), "/")[0]),
			Value: cookieState,
		})
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

var _ = errors.Is
