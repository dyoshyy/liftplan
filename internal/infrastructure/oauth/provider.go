// Package oauth は OAuth プロバイダ（GitHub / Google）との通信。
//
// このパッケージが答えるのは2つだけ。認可画面へ送るURLを組むことと、
// コールバックで受け取った code を交換して**そのプロバイダにおける本人の
// 識別子**を取ること。
//
// 呼び手（`cmd` と `httpapi`）に OAuth の知識は漏らさない。`oauth2.Config` を
// 公開して組ませると、エンドポイント・スコープ・どの応答のどの項目が識別子か
// までが外に出る。だからコンストラクタが全部持つ。
//
// プロバイダを増やすための仕組み（レジストリ・設定ファイル）は作らない。
// いま要るのは2つで、3つ目が来てから足せばそのときの実態に合う。
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

// エラーを2つに分けるのは、呼び手が「送り直せば通りうるか」を判断できる
// ようにするため。区別できないと、次のPRのハンドラは全部 502 で返すか
// 全部 400 で返すかのどちらかになる。
//
// センチネルにしたのは、呼び手が分類できないと困るのがこの2つだけで、
// 原因の詳細（どのURLがどう失敗したか）はログに出れば足りるため。型を
// 起こすと `errors.As` の対象が増えるが、分岐の数は変わらない。
var (
	// ErrProvider はプロバイダ側またはネットワークで失敗した。
	// code の期限切れ・プロバイダの障害・到達不能がここに入る。送り直せば通りうる。
	ErrProvider = errors.New("プロバイダとの通信に失敗した")

	// ErrNoIdentity は応答は受け取ったが識別子が取れなかった。
	// 空の識別子をそのまま返すと「識別子が空の全員が同一人物」になるので、
	// ここは必ず失敗にする。送り直しても同じ結果になる。
	ErrNoIdentity = errors.New("識別子が取れなかった")
)

// Identity はプロバイダにおける本人の識別子。
//
// Subject はそのプロバイダの中で不変なもの。GitHub なら数値の id、Google なら
// OIDC の sub。GitHub の login（ユーザー名）は改名できるので使わない。
type Identity struct {
	Provider string
	Subject  string
}

// Provider は1つの OAuth プロバイダ。
type Provider struct {
	name        string
	cfg         oauth2.Config
	userInfoURL string
	// subject は userinfo の応答から識別子を取り出す。
	// 2つのプロバイダの違いはここ（どの項目か・どの型か）だけに収まる。
	subject func([]byte) (string, error)
}

// NewGitHub は GitHub のプロバイダを作る。
//
// スコープは要求しない。識別子だけ要るので、メールアドレスを取ると
// 要らない同意画面が出て、保持する義務が増える。
func NewGitHub(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		name: "github",
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     endpoints.GitHub,
		},
		userInfoURL: "https://api.github.com/user",
		subject:     gitHubSubject,
	}
}

// NewGoogle は Google のプロバイダを作る。
//
// id_token の JWT を自前で検証せず、access token で userinfo を叩く。
// JWKS の取得・鍵の回転・aud/iss/exp を自前で持つと、間違えたときに
// 「通ってはいけないトークンが通る」形で出る。userinfo なら検証は
// プロバイダ側にある。
//
// スコープは openid だけ。sub はこれで取れる。profile も email も要らない。
func NewGoogle(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		name: "google",
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid"},
			Endpoint:     endpoints.Google,
		},
		userInfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
		subject:     googleSubject,
	}
}

// Name はプロバイダ名を返す。accounts 表の provider 列に入る値。
func (p *Provider) Name() string { return p.name }

// AuthCodeURL は認可画面へ送るURLを組む。
//
// state は呼び手が作って Cookie に置き、コールバックで突き合わせる。
// ここは受け取って載せるだけ。
func (p *Provider) AuthCodeURL(state string) string {
	return p.cfg.AuthCodeURL(state)
}

// Identity は code を交換し、そのプロバイダにおける本人の識別子を返す。
func (p *Provider) Identity(ctx context.Context, code string) (Identity, error) {
	token, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("%s: code を交換できない: %w: %w", p.name, ErrProvider, err)
	}

	body, err := p.fetchUserInfo(ctx, token)
	if err != nil {
		return Identity{}, err
	}

	subject, err := p.subject(body)
	if err != nil {
		return Identity{}, fmt.Errorf("%s: %w", p.name, err)
	}
	return Identity{Provider: p.name, Subject: subject}, nil
}

// fetchUserInfo はアクセストークンで本人の情報を引く。
func (p *Provider) fetchUserInfo(ctx context.Context, token *oauth2.Token) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: 問い合わせを組み立てられない: %w", p.name, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.cfg.Client(ctx, token).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: 本人の情報を引けない: %w: %w", p.name, ErrProvider, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: 本人の情報が %d で返った: %w", p.name, resp.StatusCode, ErrProvider)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: 本人の情報を読めない: %w: %w", p.name, ErrProvider, err)
	}
	return body, nil
}

// gitHubSubject は GET /user の応答から数値の id を取る。
//
// 文字列の id（`{"id":"123"}`）は想定外として弾く。受け入れると、同じ人が
// 表記違い（"0012345"）で別人になりうる。
func gitHubSubject(body []byte) (string, error) {
	var got struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return "", fmt.Errorf("応答を解釈できない: %w: %w", ErrNoIdentity, err)
	}
	// GitHub の id は1から振られる。0 は「項目が無かった」の意味にしかならない。
	if got.ID == 0 {
		return "", fmt.Errorf("id が無い: %w", ErrNoIdentity)
	}
	return strconv.FormatInt(got.ID, 10), nil
}

// googleSubject は userinfo の応答から sub を取る。
func googleSubject(body []byte) (string, error) {
	var got struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return "", fmt.Errorf("応答を解釈できない: %w: %w", ErrNoIdentity, err)
	}
	if got.Sub == "" {
		return "", fmt.Errorf("sub が無い: %w", ErrNoIdentity)
	}
	return got.Sub, nil
}
