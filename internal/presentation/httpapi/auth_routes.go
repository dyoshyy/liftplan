package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// IdentityProvider は認可先1つ分。
//
// 実体は infrastructure（internal/infrastructure/oauth）にあるが、
// presentation はそれを import できない（同じ深さの別の層）。だから
// **要る形だけをここで宣言し、cmd が翻訳して渡す。**
//
// Subject が返すエラーは apperror の語彙であること。プロバイダ固有の
// センチネルをここで並べると、分類のために presentation が infrastructure を
// 知ることになる。翻訳は cmd の仕事（apperror の doc と同じ理由）。
type IdentityProvider interface {
	// Name はこのプロバイダ。accounts 表の provider 列に入る値。
	Name() account.Provider

	// AuthCodeURL は認可画面へ送るURLを組む。state は呼び手が作って渡す。
	AuthCodeURL(state string) string

	// Identity はコードを交換し、そのプロバイダにおける本人を返す。
	//
	// 識別子のほかに、**確認済みの**メールアドレスを含むことがある。
	// それが GitHub と Google を同じ人として結ぶ唯一の手がかりになる。
	// 取れなければ空でよく、その場合は別の利用者になる。
	Identity(ctx context.Context, code string) (account.Identity, error)
}

// SignInUseCase は identity を受け入れてセッショントークンを返す口。
type SignInUseCase interface {
	Execute(
		ctx context.Context, identity account.Identity, now time.Time,
	) (account.SessionToken, error)
}

// AuthConfig は認証の経路を組むための材料。
type AuthConfig struct {
	Providers []IdentityProvider
	SignIn    SignInUseCase
	// Sessions はログアウトでセッションを消すために要る。読みは要らない。
	Sessions account.SessionWriter
	// WebOrigin は受け入れたあとに戻す先。画面のオリジン1つだけを持つ。
	//
	// リダイレクト先をパラメータで受け取らないのは、open redirect に
	// なるため。「どこへ戻すか」を要求が決められる時点で、認証の戻りは
	// 攻撃者のサイトへトークンを運ぶ経路になる。画面が1つしか無いうちは
	// 候補から選ばせる仕組みも要らない。
	WebOrigin string
	// Now は現在時刻。セッションの期限を決めるのに使う。
	//
	// 内部で time.Now() を呼ばないのは、期限のテストが書けなくなるため。
	Now func() time.Time
}

// AuthHandler は OAuth の入口・戻り・ログアウトを扱う。
type AuthHandler struct {
	providers map[account.Provider]IdentityProvider
	signIn    SignInUseCase
	sessions  account.SessionWriter
	webOrigin string
	now       func() time.Time
}

// stateCookieMaxAge は state を突き合わせられる時間。
//
// 認可画面で迷っても押し切れる程度に長く、放置された state が
// 使い回せるほどには長くしない。
const stateCookieMaxAge = 10 * time.Minute

// stateCookiePath は state の Cookie を送る経路。
//
// /auth に限るのは、API の要求すべてに付けないため。認証には使わない
// 値なので、送る必要が無い経路には送らない。
const stateCookiePath = "/auth"

// NewAuthHandler は認証の経路を組む。欠けている設定があれば失敗する。
//
// 起動を止めるための口。「設定が無ければその機能を黙って無効にする」に
// すると、ログインできないサーバーが健全なふりをして立ち上がる
// （AUTH_TOKEN と ALLOWED_ORIGINS が未設定なら起動しないのと同じ理由。D-069）。
func NewAuthHandler(cfg AuthConfig) (*AuthHandler, error) {
	switch {
	case len(cfg.Providers) == 0:
		return nil, fmt.Errorf("認可先が1つも設定されていない")
	case cfg.SignIn == nil:
		return nil, fmt.Errorf("ログインの受け入れ口が設定されていない")
	case cfg.Sessions == nil:
		return nil, fmt.Errorf("セッションの保存先が設定されていない")
	case cfg.WebOrigin == "":
		return nil, fmt.Errorf("WEB_ORIGIN が設定されていない。戻り先が決まらない")
	}

	byName := make(map[account.Provider]IdentityProvider, len(cfg.Providers))
	for _, p := range cfg.Providers {
		byName[p.Name()] = p
	}

	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &AuthHandler{
		providers: byName,
		signIn:    cfg.SignIn,
		sessions:  cfg.Sessions,
		webOrigin: cfg.WebOrigin,
		now:       now,
	}, nil
}

// Register は認証の経路をルータに載せる。
func (h *AuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/{provider}/start", h.handleStart)
	mux.HandleFunc("GET /auth/{provider}/callback", h.handleCallback)
	mux.HandleFunc("DELETE /auth/session", h.handleLogout)
}

// handleStart は認可画面へ送る。
//
// state を作って Cookie に置き、同じものを認可先へ渡す。戻ってきたときに
// 突き合わせないと、**攻撃者が用意したコードを本人のブラウザに使わせる**
// ことができる（CSRF）。本人は攻撃者のアカウントでログインした状態になり、
// そこへ記録を書き続ける。
func (h *AuthHandler) handleStart(w http.ResponseWriter, r *http.Request) {
	provider, ok := h.provider(w, r)
	if !ok {
		return
	}

	state, err := newState()
	if err != nil {
		respondCoded(w, apperror.ErrInternal, fmt.Errorf("state を作れない: %w", err))
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:  stateCookieName(provider.Name()),
		Value: state,
		Path:  stateCookiePath,
		// JS から読ませない。読めると、画面に入り込んだスクリプトが
		// state を知り、突き合わせが素通りする。
		HttpOnly: true,
		Secure:   true,
		// Lax にするのは、認可先からの戻り（トップレベルの GET）で
		// 送られる必要があるため。Strict だと送られず必ず失敗し、
		// None だと守りが消える。
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(stateCookieMaxAge.Seconds()),
	})

	http.Redirect(w, r, provider.AuthCodeURL(state), http.StatusFound)
}

// handleCallback は認可先からの戻りを受ける。
func (h *AuthHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	provider, ok := h.provider(w, r)
	if !ok {
		return
	}
	// 使ったかどうかに関わらず state を捨てる。残すと、同じ state で
	// もう一度この経路を踏める。
	//
	// **応答を書く前に消す。**defer にすると、リダイレクトや失敗応答が
	// ヘッダを送り出したあとに Set-Cookie を足すことになり、
	// net/http は「ヘッダは書き終わった」として黙って捨てる。
	// state が消えないまま成功応答が返り、テストが無ければ気づけない。
	// 要求側の Cookie はこのあとも読めるので、先に消して困らない。
	h.clearState(w, provider.Name())

	// 本人が同意画面で断った場合。エラーではないが、受け入れるものも無い。
	if reason := r.URL.Query().Get("error"); reason != "" {
		respondCoded(w, apperror.ErrInvalidInput,
			fmt.Errorf("認可されなかった: %s", reason))
		return
	}

	if !h.stateMatches(r, provider.Name()) {
		// **コードを交換する前に止める。**交換してからセッションを作らない
		// だけでは足りない。攻撃者のコードを本人のブラウザ経由で使わせて
		// いる時点で、その先の副作用は止められない。
		respondCoded(w, apperror.ErrInvalidInput, fmt.Errorf("state が一致しない"))
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		respondCoded(w, apperror.ErrInvalidInput, fmt.Errorf("code が無い"))
		return
	}

	identity, err := provider.Identity(r.Context(), code)
	if err != nil {
		respondError(w, err)
		return
	}

	token, err := h.signIn.Execute(r.Context(), identity, h.now())
	if err != nil {
		respondError(w, err)
		return
	}

	// **トークンはフラグメントに載せる。**クエリに載せると Referer と
	// アクセスログに残る。90日効くトークンなので、残った時点で
	// それを読める全員が本人になれる。フラグメントはサーバーへ送られない。
	http.Redirect(w, r, h.webOrigin+"/#token="+token.String(), http.StatusFound)
}

// handleLogout は端末のセッションを失効させる。
//
// 端末側でトークンを捨てるだけでは、盗まれたトークンが90日使える。
// 消せる口を置く。
//
// 存在しないセッションの削除も成功として扱う。再送で二度目が来ることが
// あり、そこでエラーにすると「消えているのに消せない」状態になる
// （実績ログの Delete と同じ扱い）。
func (h *AuthHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	raw, ok := bearerToken(r)
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="liftplan"`)
		writeError(w, http.StatusUnauthorized, "認証が必要である")
		return
	}

	token, err := account.ParseSessionToken(raw)
	if err != nil {
		// 形が違うトークンに対応するセッションは存在しない。
		// 消す相手が無いだけなので、成功として扱う。
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := h.sessions.Delete(r.Context(), token.Hash()); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// provider は経路のプロバイダ名を解決する。知らない名前なら 404 を書く。
func (h *AuthHandler) provider(w http.ResponseWriter, r *http.Request) (IdentityProvider, bool) {
	name, err := account.NewProvider(r.PathValue("provider"))
	if err != nil {
		writeError(w, http.StatusNotFound, "知らない認可先である")
		return nil, false
	}
	p, ok := h.providers[name]
	if !ok {
		// 名前としては正しいが、このサーバーでは設定されていない。
		writeError(w, http.StatusNotFound, "その認可先は使えない")
		return nil, false
	}
	return p, true
}

// stateMatches は Cookie の state とクエリの state が一致するかを返す。
func (h *AuthHandler) stateMatches(r *http.Request, provider account.Provider) bool {
	cookie, err := r.Cookie(stateCookieName(provider))
	if err != nil || cookie.Value == "" {
		return false
	}
	got := r.URL.Query().Get("state")
	if got == "" {
		return false
	}
	// 定数時間で比べる。素朴な == は、一致する接頭辞が長いほど応答が
	// 遅くなるので、state を1バイトずつ推測できる。
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(got)) == 1
}

func (h *AuthHandler) clearState(w http.ResponseWriter, provider account.Provider) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName(provider),
		Value:    "",
		Path:     stateCookiePath,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// stateCookieName はプロバイダごとに別の名前にする。
//
// 1つの名前を共有すると、2つの認可を同時に始めたときに後から始めた
// ほうが前の state を上書きし、前の戻りが必ず失敗する。
func stateCookieName(provider account.Provider) string {
	return "liftplan_oauth_state_" + provider.String()
}

// stateBytes は state の長さ。推測できない程度にあればよい。
const stateBytes = 32

func newState() (string, error) {
	buf := make([]byte, stateBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("乱数を読めない: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
