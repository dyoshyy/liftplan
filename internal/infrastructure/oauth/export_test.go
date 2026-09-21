package oauth

import "golang.org/x/oauth2"

// テストからエンドポイントを差し替えるための口。
//
// 本番のコンストラクタに URL を渡させる形にすると、呼び手＝次のPRの `cmd` に
// 「GitHub のトークン URL はどれか」という OAuth の知識が漏れる。
// export_test.go はテストのときだけコンパイルされるので、本番の口は
// NewGitHub / NewGoogle の3引数のままで、偽プロバイダも指せる。

// NewGitHubAt は GitHub のプロバイダを、指定したエンドポイントで作る。
func NewGitHubAt(clientID, clientSecret, redirectURL, authURL, tokenURL, userInfoURL, emailsURL string) *Provider {
	p := NewGitHub(clientID, clientSecret, redirectURL)
	overrideEndpoints(p, authURL, tokenURL, userInfoURL)
	p.emailsURL = emailsURL
	return p
}

// NewGoogleAt は Google のプロバイダを、指定したエンドポイントで作る。
func NewGoogleAt(clientID, clientSecret, redirectURL, authURL, tokenURL, userInfoURL string) *Provider {
	p := NewGoogle(clientID, clientSecret, redirectURL)
	overrideEndpoints(p, authURL, tokenURL, userInfoURL)
	return p
}

func overrideEndpoints(p *Provider, authURL, tokenURL, userInfoURL string) {
	p.cfg.Endpoint = oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL}
	p.userInfoURL = userInfoURL
}
