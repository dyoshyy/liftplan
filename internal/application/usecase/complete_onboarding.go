package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// CompleteOnboarding は初回の案内を済ませたことを記録する。
//
// 画面は通い方の設定を全部送ってから最後にこれを呼ぶ。設定の途中で失敗
// したら呼ばれないので、次に開いたときにもう一度案内が出る。
type CompleteOnboarding struct {
	writer account.OnboardingWriter
	now    func() time.Time
}

func NewCompleteOnboarding(writer account.OnboardingWriter, now func() time.Time) *CompleteOnboarding {
	return &CompleteOnboarding{writer: writer, now: now}
}

// Execute は何度呼んでも同じ結果になる。
func (u *CompleteOnboarding) Execute(ctx context.Context, user account.UserID) (err error) {
	defer func() { err = apperror.Classify(err) }()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("案内の記録が中断された: %w", err)
	}
	if err := u.writer.CompleteOnboarding(ctx, user, u.now()); err != nil {
		return fmt.Errorf("案内を済ませたことを保存できない: %w", err)
	}
	return nil
}
