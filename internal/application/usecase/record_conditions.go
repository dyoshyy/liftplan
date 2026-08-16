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
	// 空は成功として扱う。クライアントは同期のたびに送ってくるので、
	// 送るものが無い回に I/O を起こす理由がない。「空を送ってきた」ことを
	// エラーにすると、正常な同期がエラーログを埋める。
	if len(items) == 0 {
		return nil
	}
	// 切断済みのクライアントに 204 を返しつつ書き込むのを避ける。
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("保存が中断された: %w", err)
	}

	if err := u.repo.Save(ctx, items); err != nil {
		return fmt.Errorf("コンディションの保存に失敗: %w", err)
	}
	return nil
}
