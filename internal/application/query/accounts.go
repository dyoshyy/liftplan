package query

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// Login は利用者が入れるログイン方法の1つ。
type Login struct {
	// Provider はプロバイダの名前（github / google）。
	Provider string
	// Email は確認済みのメールアドレス。取っていなければ空。
	Email string
}

// Accounts は利用者のログイン方法を読む経路。
type Accounts struct {
	repo account.AccountReader
}

func NewAccounts(repo account.AccountReader) *Accounts {
	return &Accounts{repo: repo}
}

// Of はその利用者のログイン方法を返す。
//
// 設定画面の「アカウント」に、どのアドレスで入っているかを出すのに使う。
// 同じアドレスの重複を畳むか、空をどう見せるかは表示の判断なので、ここでは
// ログイン方法ごとの事実をそのまま渡す。
func (q *Accounts) Of(ctx context.Context, user account.UserID) (_ []Login, err error) {
	// 出口で1度だけ翻訳する。Exercises と同じ形。
	defer func() { err = apperror.Classify(err) }()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("読み取りが中断された: %w", err)
	}

	accounts, err := q.repo.FindByUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("アカウントの取得に失敗: %w", err)
	}

	out := make([]Login, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, Login{Provider: a.Provider().String(), Email: a.Email().String()})
	}
	return out, nil
}
