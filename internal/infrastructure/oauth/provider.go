// Package oauth は OAuth プロバイダ（GitHub / Google）との通信。
//
// このパッケージが答えるのは2つだけ。認可画面へ送るURLを組むことと、
// コールバックで受け取った code を交換して**そのプロバイダにおける本人の
// 識別子**と、取れるなら**確認済みのメールアドレス**を取ること。
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
	"strings"

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

	// Email は**プロバイダが確認を取った**メールアドレス。取れなければ空文字。
	//
	// #97 では「識別子だけ要る」としてメールアドレスを取らなかった。それを
	// ひっくり返したのは、GitHub と Google の両方でログインすると別アカウントに
	// なるため。accounts の主キーは (provider, subject) で、GitHub の数値 id と
	// Google の sub は別の名前空間なので、同じ人だと言える情報がどこにも無い。
	// 両者に共通して存在し、本人しか持てないものが確認済みメールアドレスだけ
	// だった。必要になったので取る。
	//
	// **確認の取れたものしか入れない。**確認していないアドレスで名寄せすると、
	// 他人のアドレスを登録したアカウントを作るだけでその人の記録を乗っ取れる。
	// 名寄せの仕組みがそのまま乗っ取りの経路になるので、ここは緩めない。
	//
	// 識別子と違って、これは「あれば使う」補助情報。取れなくてもログインは
	// 成立する（別アカウントになるだけで、それは #97 時点の挙動と同じ）。
	Email string
}

// Provider は1つの OAuth プロバイダ。
type Provider struct {
	name        string
	cfg         oauth2.Config
	userInfoURL string
	// emailsURL は GitHub の GET /user/emails。Google では空。
	emailsURL string
	// subject は userinfo の応答から識別子を取り出す。
	// 2つのプロバイダの違いはここ（どの項目か・どの型か）だけに収まる。
	subject func([]byte) (string, error)
	// email は確認済みのメールアドレスを取り出す。取れなければ空文字。
	//
	// Google は userinfo の応答だけで足りるが、GitHub は別の問い合わせが要る
	// （GET /user には verified が無い）。だから client と emailsURL を渡す。
	// エラーを返さないのは、ここで失敗してもログインは成立させるため。
	email func(ctx context.Context, client *http.Client, emailsURL string, userInfo []byte) string
}

// NewGitHub は GitHub のプロバイダを作る。
//
// #97 ではスコープを要求しなかった（識別子だけ要るのに、メールアドレスを
// 取ると要らない同意画面が出て保持する義務が増える、という理由）。
// user:email を足したのは、GitHub と Google を同じ人として結ぶ材料が
// 確認済みメールアドレスしか無いため。これが無いと GET /user/emails が
// 403 になり、名寄せの材料が永久に取れない。
func NewGitHub(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		name: "github",
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"user:email"},
			Endpoint:     endpoints.GitHub,
		},
		userInfoURL: "https://api.github.com/user",
		emailsURL:   "https://api.github.com/user/emails",
		subject:     gitHubSubject,
		email:       gitHubVerifiedEmail,
	}
}

// NewGoogle は Google のプロバイダを作る。
//
// id_token の JWT を自前で検証せず、access token で userinfo を叩く。
// JWKS の取得・鍵の回転・aud/iss/exp を自前で持つと、間違えたときに
// 「通ってはいけないトークンが通る」形で出る。userinfo なら検証は
// プロバイダ側にある。
//
// スコープは openid と email。#97 では openid だけで「profile も email も
// 要らない」としていたが、GitHub と Google を結ぶために確認済みメール
// アドレスが要るようになったので email を足した。profile は今も要らない。
func NewGoogle(clientID, clientSecret, redirectURL string) *Provider {
	return &Provider{
		name: "google",
		cfg: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email"},
			Endpoint:     endpoints.Google,
		},
		userInfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
		subject:     googleSubject,
		email:       googleVerifiedEmail,
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
//
// 識別子が取れなければ失敗する。メールアドレスが取れないのは失敗にしない
// （取れなければ別アカウントになるだけで、それは #97 時点の挙動と同じ）。
func (p *Provider) Identity(ctx context.Context, code string) (Identity, error) {
	token, err := p.cfg.Exchange(ctx, code)
	if err != nil {
		return Identity{}, fmt.Errorf("%s: code を交換できない: %w: %w", p.name, ErrProvider, err)
	}
	client := p.cfg.Client(ctx, token)

	body, err := p.fetchUserInfo(ctx, client)
	if err != nil {
		return Identity{}, err
	}

	subject, err := p.subject(body)
	if err != nil {
		return Identity{}, fmt.Errorf("%s: %w", p.name, err)
	}
	return Identity{
		Provider: p.name,
		Subject:  subject,
		Email:    p.email(ctx, client, p.emailsURL, body),
	}, nil
}

// fetchUserInfo はアクセストークンで本人の情報を引く。
func (p *Provider) fetchUserInfo(ctx context.Context, client *http.Client) ([]byte, error) {
	body, err := get(ctx, client, p.userInfoURL)
	if err != nil {
		return nil, fmt.Errorf("%s: 本人の情報を引けない: %w: %w", p.name, ErrProvider, err)
	}
	return body, nil
}

// get は client で1件引き、200 のときだけ本文を返す。
func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("問い合わせを組み立てられない: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%d で返った", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
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

// gitHubVerifiedEmail は GET /user/emails から primary かつ verified の1件を取る。
//
// GET /user が返す email は使わない。公開設定次第で null になるうえ、確認済み
// かどうかが応答に出ない。確認の取れていないアドレスで名寄せすると、他人の
// アドレスを登録しただけでその人の記録を乗っ取れる。
//
// primary も要るのは、verified なアドレスは複数ありうるため。どれを選ぶかを
// 実装の気分（配列の順）で決めると、同じ人が日によって別のアドレスで名寄せ
// される。本人が「主」と決めた1件だけを使う。
//
// 引けなかった・解釈できなかったときは空文字を返す。403（スコープが無い・
// 取り消された）はここに入るが、識別子は取れているのでログインは成立させる。
func gitHubVerifiedEmail(ctx context.Context, client *http.Client, emailsURL string, _ []byte) string {
	body, err := get(ctx, client, emailsURL)
	if err != nil {
		return ""
	}

	var got []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		return ""
	}

	for _, e := range got {
		if e.Primary && e.Verified {
			return normalizeEmail(e.Email)
		}
	}
	return ""
}

// googleVerifiedEmail は userinfo の email を、email_verified が true のときだけ取る。
//
// 識別子（sub）とは別に解釈する。1つの構造体にまとめると、email_verified が
// 想定外の型（文字列の "true" など）で返ったときに Unmarshal ごと失敗し、
// 補助情報の不調が「識別子が取れない」に化ける。
func googleVerifiedEmail(_ context.Context, _ *http.Client, _ string, userInfo []byte) string {
	var got struct {
		Email    string `json:"email"`
		Verified bool   `json:"email_verified"`
	}
	if err := json.Unmarshal(userInfo, &got); err != nil {
		return ""
	}
	// 項目が無ければ false。「email があれば使う」にすると、確認の取れて
	// いないアドレスが名寄せに入る。
	if !got.Verified {
		return ""
	}
	return normalizeEmail(got.Email)
}

// normalizeEmail は前後の空白を落として小文字に揃える。
//
// 大文字小文字だけが違うアドレスを別人にしないため。Gmail のドット除去のような
// 凝った正規化はしない（必要になってから足す。今は破綻しない）。
//
// 採否の判断より先に掛ける。後にすると、空白だけのアドレスが「値がある」と
// 見なされて通る。
func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
