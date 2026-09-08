package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
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

func (u *DeleteSetLog) Execute(ctx context.Context, id setlog.SetLogID) error {
	if id == "" {
		return fmt.Errorf("%w: 取り消す記録のIDが無い", ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("取り消しが中断された: %w", err)
	}
	if err := u.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("実績の取り消しに失敗: %w", err)
	}
	return nil
}
