package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
)

// RecordConditions は日次コンディションを保存するユースケース。
type RecordConditions struct {
	repo condition.Writer
}

func NewRecordConditions(repo condition.Writer) *RecordConditions {
	return &RecordConditions{repo: repo}
}

func (u *RecordConditions) Execute(ctx context.Context, user account.UserID, items []condition.DailyCondition) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

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

	if err := u.repo.Save(ctx, user, items); err != nil {
		return fmt.Errorf("コンディションの保存に失敗: %w", err)
	}
	return nil
}
