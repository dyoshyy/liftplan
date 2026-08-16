package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// RecordConditions は日次コンディションを保存するユースケース。
type RecordConditions struct {
	repo training.ConditionRepository
}

func NewRecordConditions(repo training.ConditionRepository) *RecordConditions {
	return &RecordConditions{repo: repo}
}

func (u *RecordConditions) Execute(ctx context.Context, items []training.DailyCondition) error {
	if len(items) == 0 {
		return nil
	}
	if err := u.repo.Save(ctx, items); err != nil {
		return fmt.Errorf("コンディションの保存に失敗: %w", err)
	}
	return nil
}
