package oauth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/dyoshyy/liftplan/internal/infrastructure/oauth"
)

// fakeAPI は偽プロバイダが返すもの。
//
// ステータスの 0 は 200 と見なす。ケースの多くは「正常に返るが本文が違う」を
// 見ているので、そこで毎回 http.StatusOK と書かせない。
type fakeAPI struct {
	tokenStatus    int
	userInfoStatus int
	userInfoBody   string
	// emailsStatus / emailsBody は GitHub の GET /user/emails。Google では使わない。
	emailsStatus int
	emailsBody   string
}

// fakeProvider は偽のプロバイダを立て、そこを指すプロバイダを返す。
//
// ステータスが 200 以外なら、その本文は空で返す。
func fakeProvider(t *testing.T, name string, api fakeAPI) *oauth.Provider {
	t.Helper()

	ok := func(status int) int {
		if status == 0 {
			return http.StatusOK
		}
		return status
	}
	// 本番の GitHub / Google はアクセストークンの無い問い合わせを 401 で返す。
	// 偽プロバイダも同じにしておかないと、トークンを載せ忘れた実装が緑のまま
	// 通り、本番でだけ取れなくなる。
	serve := func(status int, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer at-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if status = ok(status); status != http.StatusOK {
				w.WriteHeader(status)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if s := ok(api.tokenStatus); s != http.StatusOK {
			w.WriteHeader(s)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/userinfo", serve(api.userInfoStatus, api.userInfoBody))
	mux.HandleFunc("/emails", serve(api.emailsStatus, api.emailsBody))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	switch name {
	case "github":
		return oauth.NewGitHubAt("cid", "secret", "https://api.test/auth/github/callback",
			srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/userinfo", srv.URL+"/emails")
	case "google":
		return oauth.NewGoogleAt("cid", "secret", "https://api.test/auth/google/callback",
			srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/userinfo")
	default:
		t.Fatalf("知らないプロバイダ %q", name)
		return nil
	}
}

// 認可画面のURLに state が載る。
//
// state を載せ忘れると、コールバックで突き合わせる相手が無くなり、
// 第三者が仕込んだ code を本人のセッションで交換できてしまう（CSRF）。
// 「state パラメータがある」ではなく「渡した値がそのまま載っている」を見る。
func TestProvider_AuthCodeURL(t *testing.T) {
	cases := []struct {
		name      string
		provider  string
		wantScope string
	}{
		{
			// 元は「識別子だけ要るのでスコープを要求しない」だった（#97）。
			// GitHub と Google を同じ人として結ぶ手掛かりが確認済みメール
			// アドレスしか無いので、user:email を足した。これが無いと
			// GET /user/emails が 403 になり、名寄せの材料が永久に取れない。
			name:     "GitHub は state を載せ、user:email を要求する",
			provider: "github", wantScope: "user:email",
		},
		{
			// 元は「sub は openid だけで取れる。email は要らない」だった（#97）。
			// 同上の理由で email を足した。順序は cfg.Scopes のまま。
			name:     "Google は state を載せ、openid と email を要求する",
			provider: "google", wantScope: "openid email",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fakeProvider(t, c.provider, fakeAPI{userInfoBody: `{}`})

			raw := p.AuthCodeURL("state-xyz")
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("認可URLを解釈できない: %v", err)
			}
			q := u.Query()

			if got := q.Get("state"); got != "state-xyz" {
				t.Errorf("state が %q。\"state-xyz\" のはず", got)
			}
			if got := q.Get("client_id"); got != "cid" {
				t.Errorf("client_id が %q。\"cid\" のはず", got)
			}
			if got := q.Get("redirect_uri"); got != "https://api.test/auth/"+c.provider+"/callback" {
				t.Errorf("redirect_uri が %q", got)
			}
			if got := q.Get("response_type"); got != "code" {
				t.Errorf("response_type が %q。\"code\" のはず", got)
			}
			if got := q.Get("scope"); got != c.wantScope {
				t.Errorf("scope が %q。%q のはず", got, c.wantScope)
			}
		})
	}
}

// code を交換して識別子を取る。
//
// 識別子が取れないときに空文字を黙って返すと、次のPRで「識別子が空の全員が
// 同一人物」になる。だから「エラーになること」ではなく「どちらのエラーか」を
// 見る。ErrProvider（プロバイダ側で失敗した＝送り直せば通りうる）と
// ErrNoIdentity（応答が想定外＝アカウントにしてはいけない）を取り違えると、
// 次のPRで返す状態と再試行の可否が入れ替わる。
func TestProvider_Identity(t *testing.T) {
	cases := []struct {
		name           string
		provider       string
		tokenStatus    int
		userInfoStatus int
		userInfoBody   string
		emailsStatus   int
		emailsBody     string
		wantSubject    string
		wantEmail      string
		wantErr        error
	}{
		{
			// GitHub は数値の id が不変。login（ユーザー名）は改名できるので使わない。
			name:     "GitHub は数値の id を識別子にする",
			provider: "github",
			// GET /user の email は使わない。公開設定次第で null になるうえ、
			// 確認済みかどうかが分からない。ここに罠を置いて、拾わないことを見る。
			userInfoBody: `{"id":12345,"login":"dyoshyy","email":"trap@example.com"}`,
			emailsBody:   `[{"email":"me@example.com","primary":true,"verified":true}]`,
			wantSubject:  "12345", wantEmail: "me@example.com",
		},
		{
			// primary かつ verified の1件だけを使う。「verified のどれか」や
			// 「primary のどれか」で拾う実装だと、ここで別のアドレスが出る。
			name:         "GitHub は primary かつ verified の1件を使う",
			provider:     "github",
			userInfoBody: `{"id":12345}`,
			emailsBody: `[{"email":"old@example.com","primary":false,"verified":true},` +
				`{"email":"me@example.com","primary":true,"verified":true},` +
				`{"email":"other@example.com","primary":false,"verified":true}]`,
			wantSubject: "12345", wantEmail: "me@example.com",
		},
		{
			// 確認の取れていないアドレスで名寄せすると、他人のアドレスを登録した
			// アカウントを作るだけでその人の記録を乗っ取れる。だから使わない。
			name:         "GitHub は verified でないアドレスを使わない",
			provider:     "github",
			userInfoBody: `{"id":12345}`,
			emailsBody:   `[{"email":"me@example.com","primary":true,"verified":false}]`,
			wantSubject:  "12345", wantEmail: "",
		},
		{
			// verified だが primary でない／primary だが verified でない、の2件。
			// 片方の条件しか見ていない実装はここで別人のアドレスを返す。
			name:         "GitHub は片方の条件しか満たさないアドレスを使わない",
			provider:     "github",
			userInfoBody: `{"id":12345}`,
			emailsBody: `[{"email":"unverified@example.com","primary":true,"verified":false},` +
				`{"email":"secondary@example.com","primary":false,"verified":true}]`,
			wantSubject: "12345", wantEmail: "",
		},
		{
			name:         "GitHub は配列が空ならメールアドレスを持たない",
			provider:     "github",
			userInfoBody: `{"id":12345,"email":"trap@example.com"}`,
			emailsBody:   `[]`,
			wantSubject:  "12345", wantEmail: "",
		},
		{
			// スコープが無い・取り消された場合がこれ。メールアドレスは補助情報
			// なので、取れなくてもログインは成立する（識別子は取れている）。
			// ここを失敗にすると、名寄せの都合でログインできない人が出る。
			name:         "GitHub は /user/emails が403でもログインは成立する",
			provider:     "github",
			userInfoBody: `{"id":12345}`,
			emailsStatus: http.StatusForbidden,
			wantSubject:  "12345", wantEmail: "",
		},
		{
			// 200 だが配列ですらない（プロキシのエラーページなど）。
			// 識別子は取れているので、ここも失敗にはしない。
			name:         "GitHub は /user/emails が配列でなくてもログインは成立する",
			provider:     "github",
			userInfoBody: `{"id":12345}`,
			emailsBody:   `{"message":"Bad credentials"}`,
			wantSubject:  "12345", wantEmail: "",
		},
		{
			// 大文字小文字だけが違うアドレスを別人にしないため、小文字に揃える。
			// 前後の空白も落とす。凝った正規化（Gmail のドット除去など）はしない。
			name:         "GitHub のアドレスは小文字に揃え、前後の空白を落とす",
			provider:     "github",
			userInfoBody: `{"id":12345}`,
			emailsBody:   `[{"email":"  Me@Example.COM ","primary":true,"verified":true}]`,
			wantSubject:  "12345", wantEmail: "me@example.com",
		},
		{
			// Google は OIDC の sub が不変。
			name:         "Google は sub を識別子にする",
			provider:     "google",
			userInfoBody: `{"sub":"1078","email":"  Me@Example.COM ","email_verified":true}`,
			wantSubject:  "1078", wantEmail: "me@example.com",
		},
		{
			name:         "Google は email_verified が false ならメールアドレスを使わない",
			provider:     "google",
			userInfoBody: `{"sub":"1078","email":"me@example.com","email_verified":false}`,
			wantSubject:  "1078", wantEmail: "",
		},
		{
			// 項目が無ければ false と同じ扱い。「email があれば使う」にすると、
			// 確認の取れていないアドレスが名寄せに入る。
			name:         "Google は email_verified が無ければメールアドレスを使わない",
			provider:     "google",
			userInfoBody: `{"sub":"1078","email":"x@example.com"}`,
			wantSubject:  "1078", wantEmail: "",
		},
		{
			// email_verified が想定外の型で返っても、識別子は取れているので
			// ログインは成立する。sub と同じ構造体に読む実装だと Unmarshal ごと
			// 失敗し、補助情報の不調が「識別子が取れない」に化ける。
			name:         "Google は email_verified が文字列でもログインは成立する",
			provider:     "google",
			userInfoBody: `{"sub":"1078","email":"x@example.com","email_verified":"true"}`,
			wantSubject:  "1078", wantEmail: "",
		},
		{
			// 使用済み・期限切れの code はここで 400 になる。
			name:     "トークン交換が400ならプロバイダ側の失敗",
			provider: "github", tokenStatus: http.StatusBadRequest,
			userInfoBody: `{"id":12345}`,
			wantErr:      oauth.ErrProvider,
		},
		{
			name:     "userinfo が500ならプロバイダ側の失敗",
			provider: "google", userInfoStatus: http.StatusInternalServerError,
			userInfoBody: `{"sub":"1078"}`,
			wantErr:      oauth.ErrProvider,
		},
		{
			// id が無ければ 0 になる。GitHub の id は1から振られるので 0 は本物ではない。
			name:         "GitHub の id が無ければ識別子が無い",
			provider:     "github",
			userInfoBody: `{"login":"dyoshyy"}`,
			emailsBody:   `[{"email":"me@example.com","primary":true,"verified":true}]`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			name:         "GitHub の id が0なら識別子が無い",
			provider:     "github",
			userInfoBody: `{"id":0}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			// 文字列の id は想定外。strconv で拾って通すと、本物の id と別の
			// 表記（"0012345"）が別人になる。
			name:         "GitHub の id が文字列なら識別子が無い",
			provider:     "github",
			userInfoBody: `{"id":"12345"}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			// 識別子が取れないなら、確認済みのメールアドレスがあってもアカウントに
			// してはいけない。メールアドレスは補助情報で、識別子の代わりにはならない。
			name:         "Google の sub が空なら識別子が無い",
			provider:     "google",
			userInfoBody: `{"sub":"","email":"me@example.com","email_verified":true}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			name:         "Google の sub が無ければ識別子が無い",
			provider:     "google",
			userInfoBody: `{"email":"x@example.com","email_verified":true}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			// 200 だが JSON ですらない（プロキシのエラーページなど）。
			name:         "応答がオブジェクトでなければ識別子が無い",
			provider:     "google",
			userInfoBody: `["sub"]`,
			wantErr:      oauth.ErrNoIdentity,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fakeProvider(t, c.provider, fakeAPI{
				tokenStatus:    c.tokenStatus,
				userInfoStatus: c.userInfoStatus,
				userInfoBody:   c.userInfoBody,
				emailsStatus:   c.emailsStatus,
				emailsBody:     c.emailsBody,
			})

			got, err := p.Identity(context.Background(), "code-1")

			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("エラーが %v。%v のはず", err, c.wantErr)
				}
				if got.Subject != "" {
					t.Errorf("失敗したのに識別子 %q を返している", got.Subject)
				}
				if got.Email != "" {
					t.Errorf("失敗したのにメールアドレス %q を返している", got.Email)
				}
				return
			}
			if err != nil {
				t.Fatalf("エラーになった: %v", err)
			}
			if got.Subject != c.wantSubject {
				t.Errorf("識別子が %q。%q のはず", got.Subject, c.wantSubject)
			}
			// 確認の取れていないアドレスが1つでも漏れると、それを登録した他人が
			// 本人の記録に入れる。「空でないこと」ではなく値そのものを見る。
			if got.Email != c.wantEmail {
				t.Errorf("メールアドレスが %q。%q のはず", got.Email, c.wantEmail)
			}
			// プロバイダ名は accounts 表の列になる。取り違えると、GitHub の
			// 12345 と Google の 12345 が同一人物になる。
			if got.Provider != c.provider {
				t.Errorf("プロバイダ名が %q。%q のはず", got.Provider, c.provider)
			}
		})
	}
}

// Name はプロバイダ名をそのまま返す。
//
// この文字列は accounts 表に入り、手で流す INSERT（設計メモ）とも一致する
// 必要がある。
func TestProvider_Name(t *testing.T) {
	if got := oauth.NewGitHub("cid", "secret", "https://api.test/cb").Name(); got != "github" {
		t.Errorf("GitHub のプロバイダ名が %q。\"github\" のはず", got)
	}
	if got := oauth.NewGoogle("cid", "secret", "https://api.test/cb").Name(); got != "google" {
		t.Errorf("Google のプロバイダ名が %q。\"google\" のはず", got)
	}
}
