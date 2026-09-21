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

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

var authNow = time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)

// sessionStore は SessionReader の最小実装。
type sessionStore struct {
	byHash map[string]*account.Session
	err    error
	// asked は Find に渡された現在時刻。期限の判定を店側に任せている
	// ことを確かめる。
	asked []time.Time
}

func (s *sessionStore) Find(
	_ context.Context, h account.TokenHash, now time.Time,
) (*account.Session, error) {
	s.asked = append(s.asked, now)
	if s.err != nil {
		return nil, s.err
	}
	sess, ok := s.byHash[h.String()]
	if !ok {
		return nil, account.ErrSessionNotFound
	}
	return sess, nil
}

func storeWith(t *testing.T, token string, user account.UserID) *sessionStore {
	t.Helper()

	parsed, err := account.ParseSessionToken(token)
	if err != nil {
		t.Fatalf("トークンが不正: %v", err)
	}
	sess, err := account.NewSession(parsed.Hash(), user, authNow.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("セッションを組めない: %v", err)
	}
	return &sessionStore{byHash: map[string]*account.Session{parsed.Hash().String(): sess}}
}

// 中身を見るためのハンドラ。context に載った利用者を書き出す。
func echoUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := httpapi.UserForTest(r.Context())
		if !ok {
			http.Error(w, "利用者が無い", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, user.String())
	})
}

func guardedBy(store *sessionStore) http.Handler {
	return httpapi.RequireSession(store, func() time.Time { return authNow })(echoUser())
}

// セッションのトークンが通り、そのセッションの利用者が context に載ること。
//
// **ここが「誰が」を決める唯一の場所。**トークンは本人かどうかしか
// 言えないが、セッションは誰かを言える。
func TestRequireSession_PutsTheSessionOwnerOnTheContext(t *testing.T) {
	user, err := account.NewUserID("3f0b9f62-6a7c-4b2e-8f2f-1d0a2b3c4d5e")
	if err != nil {
		t.Fatalf("UserID: %v", err)
	}
	store := storeWith(t, sampleToken, user)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+sampleToken)
	rec := httptest.NewRecorder()
	guardedBy(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状態が %d。200 のはず（本体 %s）", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != user.String() {
		t.Errorf("載った利用者が %q。%q のはず", got, user.String())
	}
	// 既定ユーザーに落ちていないこと。落ちると、誰がログインしても
	// 同じ記録を見る状態に静かに戻る。
	if rec.Body.String() == account.DefaultUserID().String() {
		t.Error("セッションの利用者ではなく既定ユーザーが載っている")
	}
}

// 期限の判定はセッションの店に委ねること（現在時刻を渡す）。
func TestRequireSession_AsksTheStoreWithTheCurrentTime(t *testing.T) {
	store := storeWith(t, sampleToken, account.DefaultUserID())

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+sampleToken)
	guardedBy(store).ServeHTTP(httptest.NewRecorder(), req)

	if len(store.asked) != 1 || !store.asked[0].Equal(authNow) {
		t.Errorf("店に渡した時刻が %v。%v のはず", store.asked, authNow)
	}
}

// 通らないものは 401。
func TestRequireSession_RejectsWhatItCannotResolve(t *testing.T) {
	cases := []struct {
		name   string
		header string
		store  func(*testing.T) *sessionStore
		want   int
	}{
		{
			name: "Authorization が無い", header: "",
			store: func(t *testing.T) *sessionStore { return storeWith(t, sampleToken, account.DefaultUserID()) },
			want:  http.StatusUnauthorized,
		},
		{
			// 形は正しいが、そのセッションは存在しない（消された・期限切れ）。
			name: "知らないトークン", header: "Bearer " + strings.Repeat("z", 43),
			store: func(t *testing.T) *sessionStore { return storeWith(t, sampleToken, account.DefaultUserID()) },
			want:  http.StatusUnauthorized,
		},
		{
			// セッショントークンの形ですらない。引く前に落とす。
			name: "形が違うトークン", header: "Bearer short",
			store: func(t *testing.T) *sessionStore { return storeWith(t, sampleToken, account.DefaultUserID()) },
			want:  http.StatusUnauthorized,
		},
		{
			// 保存先に届かない。401 にすると、クライアントは
			// 「ログインし直せ」と読んで、通るはずのトークンを捨てる。
			name: "保存先に届かない", header: "Bearer " + sampleToken,
			store: func(t *testing.T) *sessionStore {
				s := storeWith(t, sampleToken, account.DefaultUserID())
				s.err = fmt.Errorf("%w: 接続できない", training.ErrRepositoryUnavailable)
				return s
			},
			want: http.StatusServiceUnavailable,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rec := httptest.NewRecorder()
			guardedBy(c.store(t)).ServeHTTP(rec, req)

			if rec.Code != c.want {
				t.Errorf("状態が %d。%d のはず", rec.Code, c.want)
			}
			if c.want == http.StatusUnauthorized &&
				rec.Header().Get("WWW-Authenticate") == "" {
				t.Error("WWW-Authenticate が無い")
			}
		})
	}
}

// 認証を通さない経路は、ヘルスチェックとログインの入口・戻りだけ。
//
// **ログアウトを含めてはいけない。**誰でも叩けると、トークンの
// ハッシュを総当たりする口になる。経路をまとめて /auth で除外すると
// ここが静かに開く。
func TestRequireSession_ExemptsOnlyTheLoginEntrances(t *testing.T) {
	cases := []struct {
		path     string
		exempted bool
	}{
		{"/health", true},
		{"/auth/github/start", true},
		{"/auth/github/callback", true},
		{"/auth/google/start", true},
		{"/auth/google/callback", true},
		{"/auth/session", false},
		{"/api/sessions", false},
		// 前方一致で通してしまわないこと。
		{"/health/../api/sessions", false},
		{"/auth/github/start/extra", false},
		{"/auth/evil/start", false},
	}

	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			store := storeWith(t, sampleToken, account.DefaultUserID())
			req := httptest.NewRequest(http.MethodGet, c.path, nil)
			rec := httptest.NewRecorder()
			guardedBy(store).ServeHTTP(rec, req)

			// 認証を通さない経路なら、トークンが無くても先へ進む。
			// echoUser は利用者が無いと 500 を書くので、それが目印。
			switch {
			case c.exempted && rec.Code == http.StatusUnauthorized:
				t.Errorf("%s が 401。通す経路のはず", c.path)
			case !c.exempted && rec.Code != http.StatusUnauthorized:
				t.Errorf("%s が %d。401 のはず", c.path, rec.Code)
			}
		})
	}
}

// 401 の本体にトークンを書き出さないこと。ログと画面に残る。
func TestRequireSession_DoesNotEchoTheToken(t *testing.T) {
	store := storeWith(t, sampleToken, account.DefaultUserID())
	const secret = "zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"

	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+secret)
	rec := httptest.NewRecorder()
	guardedBy(store).ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), secret) {
		t.Errorf("応答にトークンが載っている: %s", rec.Body.String())
	}
}

var _ = errors.Is
