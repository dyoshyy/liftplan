package usecase

import (
	"context"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"

	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// DeleteSetLog は打ち間違いを取り消すユースケース。
//
// 実績は「確定した1セット」で生成後は変更しないが、削除は意味が違う。
// 値を書き換えるのではなく「これは起きなかった」と言っている。
// 訂正の手段が無いほうが害が大きく、間違った記録が推定1RMを汚したまま残る。
type DeleteSetLog struct {
	repo setlog.Writer
}

func NewDeleteSetLog(repo setlog.Writer) *DeleteSetLog {
	return &DeleteSetLog{repo: repo}
}

func (u *DeleteSetLog) Execute(ctx context.Context, id setlog.SetLogID) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = classify(err) }()

	if id == "" {
		return fmt.Errorf("%w: 取り消す記録のIDが無い", apperror.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("取り消しが中断された: %w", err)
	}
	if err := u.repo.Delete(ctx, currentUser(), id); err != nil {
		return fmt.Errorf("実績の取り消しに失敗: %w", err)
	}
	return nil
}
