package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/infrastructure/oauth"
	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"
)

// identityProvider は infrastructure の OAuth 実装を presentation の口に合わせる。
//
// **この翻訳が cmd にあるのは、両側を知ってよいのがここだけだから。**
// presentation と infrastructure は同じ深さの層で、互いに import できない
// （internal/architecture_test.go）。だから presentation は要る形だけを
// 宣言し（httpapi.IdentityProvider）、具象を渡すこの層が橋を架ける。
//
// 翻訳するのは2つ。プロバイダの名前をドメインの値にすること、そして
// エラーを apperror の語彙にすること。後者をやらないと、presentation が
// oauth のセンチネルを知ることになり、分類のために層を越える。
type identityProvider struct {
	name     account.Provider
	provider *oauth.Provider
}

func newIdentityProvider(name account.Provider, p *oauth.Provider) *identityProvider {
	return &identityProvider{name: name, provider: p}
}

func (a *identityProvider) Name() account.Provider { return a.name }

func (a *identityProvider) AuthCodeURL(state string) string {
	return a.provider.AuthCodeURL(state)
}

// Subject はコードを交換し、そのプロバイダにおける本人の識別子を返す。
//
// エラーの写し方が、そのまま「送り直す意味があるか」の答えになる。
//
//   - ErrProvider  … プロバイダ側かネットワーク。あとで押し直せば通る → 503
//   - ErrNoIdentity … 応答は来たが識別子が無い。押し直しても同じ → 400
//
// 混ぜると、直しようのない失敗にやり直しを促すか、直る失敗を諦めさせるか
// のどちらかになる。
func (a *identityProvider) Subject(ctx context.Context, code string) (string, error) {
	identity, err := a.provider.Identity(ctx, code)
	switch {
	case errors.Is(err, oauth.ErrProvider):
		return "", fmt.Errorf("%w: %w", apperror.ErrUnavailable, err)
	case errors.Is(err, oauth.ErrNoIdentity):
		return "", fmt.Errorf("%w: %w", apperror.ErrInvalidInput, err)
	case err != nil:
		return "", err
	}
	return identity.Subject, nil
}

var _ httpapi.IdentityProvider = (*identityProvider)(nil)
