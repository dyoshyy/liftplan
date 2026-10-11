package query

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// Onboarding は初回の案内を済ませたかを読む経路。
type Onboarding struct {
	repo account.OnboardingReader
}

func NewOnboarding(repo account.OnboardingReader) *Onboarding {
	return &Onboarding{repo: repo}
}

// Completed はその利用者が初回の案内を済ませたかを返す。
func (q *Onboarding) Completed(ctx context.Context, user account.UserID) (_ bool, err error) {
	// 出口で1度だけ翻訳する。Accounts と同じ形。
	defer func() { err = apperror.Classify(err) }()
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("読み取りが中断された: %w", err)
	}

	done, err := q.repo.OnboardingCompleted(ctx, user)
	if err != nil {
		return false, fmt.Errorf("案内の状態の取得に失敗: %w", err)
	}
	return done, nil
}
