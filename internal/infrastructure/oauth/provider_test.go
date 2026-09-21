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

// fakeProvider は偽のプロバイダを立て、そこを指すプロバイダを返す。
//
// tokenStatus / userInfoStatus が 200 以外なら、その本文は空で返す。
func fakeProvider(t *testing.T, name string, tokenStatus int, userInfoStatus int, userInfoBody string) *oauth.Provider {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if tokenStatus != http.StatusOK {
			w.WriteHeader(tokenStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		// Bearer で渡していないと、本番では 401 になって識別子が取れない。
		if got := r.Header.Get("Authorization"); got != "Bearer at-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if userInfoStatus != http.StatusOK {
			w.WriteHeader(userInfoStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(userInfoBody))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	switch name {
	case "github":
		return oauth.NewGitHubAt("cid", "secret", "https://api.test/auth/github/callback",
			srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/userinfo")
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
			// GitHub は識別子（数値 id）だけ要るのでスコープを要求しない。
			name:     "GitHub は state を載せ、スコープを要求しない",
			provider: "github", wantScope: "",
		},
		{
			// Google の sub は openid だけで取れる。email / profile は要らない。
			name:     "Google は state を載せ、openid だけを要求する",
			provider: "google", wantScope: "openid",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fakeProvider(t, c.provider, http.StatusOK, http.StatusOK, `{}`)

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
		wantSubject    string
		wantErr        error
	}{
		{
			// GitHub は数値の id が不変。login（ユーザー名）は改名できるので使わない。
			name:     "GitHub は数値の id を識別子にする",
			provider: "github", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `{"id":12345,"login":"dyoshyy"}`,
			wantSubject:  "12345",
		},
		{
			// Google は OIDC の sub が不変。
			name:     "Google は sub を識別子にする",
			provider: "google", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `{"sub":"1078","email":"x@example.com"}`,
			wantSubject:  "1078",
		},
		{
			// 使用済み・期限切れの code はここで 400 になる。
			name:     "トークン交換が400ならプロバイダ側の失敗",
			provider: "github", tokenStatus: http.StatusBadRequest, userInfoStatus: http.StatusOK,
			userInfoBody: `{"id":12345}`,
			wantErr:      oauth.ErrProvider,
		},
		{
			name:     "userinfo が500ならプロバイダ側の失敗",
			provider: "google", tokenStatus: http.StatusOK, userInfoStatus: http.StatusInternalServerError,
			userInfoBody: `{"sub":"1078"}`,
			wantErr:      oauth.ErrProvider,
		},
		{
			// id が無ければ 0 になる。GitHub の id は1から振られるので 0 は本物ではない。
			name:     "GitHub の id が無ければ識別子が無い",
			provider: "github", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `{"login":"dyoshyy"}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			name:     "GitHub の id が0なら識別子が無い",
			provider: "github", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `{"id":0}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			// 文字列の id は想定外。strconv で拾って通すと、本物の id と別の
			// 表記（"0012345"）が別人になる。
			name:     "GitHub の id が文字列なら識別子が無い",
			provider: "github", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `{"id":"12345"}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			name:     "Google の sub が空なら識別子が無い",
			provider: "google", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `{"sub":""}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			name:     "Google の sub が無ければ識別子が無い",
			provider: "google", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `{"email":"x@example.com"}`,
			wantErr:      oauth.ErrNoIdentity,
		},
		{
			// 200 だが JSON ですらない（プロキシのエラーページなど）。
			name:     "応答がオブジェクトでなければ識別子が無い",
			provider: "google", tokenStatus: http.StatusOK, userInfoStatus: http.StatusOK,
			userInfoBody: `["sub"]`,
			wantErr:      oauth.ErrNoIdentity,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fakeProvider(t, c.provider, c.tokenStatus, c.userInfoStatus, c.userInfoBody)

			got, err := p.Identity(context.Background(), "code-1")

			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("エラーが %v。%v のはず", err, c.wantErr)
				}
				if got.Subject != "" {
					t.Errorf("失敗したのに識別子 %q を返している", got.Subject)
				}
				return
			}
			if err != nil {
				t.Fatalf("エラーになった: %v", err)
			}
			if got.Subject != c.wantSubject {
				t.Errorf("識別子が %q。%q のはず", got.Subject, c.wantSubject)
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
