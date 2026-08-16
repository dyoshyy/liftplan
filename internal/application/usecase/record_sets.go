package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// RecordSets は実績ログを保存するユースケース。
// リポジトリ側が冪等なので、同じログを二度受け取っても壊れない。
type RecordSets struct {
	repo training.SetLogRepository
}

func NewRecordSets(repo training.SetLogRepository) *RecordSets {
	return &RecordSets{repo: repo}
}

func (u *RecordSets) Execute(ctx context.Context, logs []*training.SetLog) error {
	if len(logs) == 0 {
		return nil
	}
	if err := u.repo.Save(ctx, logs); err != nil {
		return fmt.Errorf("実績の保存に失敗: %w", err)
	}
	return nil
}
