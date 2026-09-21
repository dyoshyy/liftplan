package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetSplitCycle は分割の周期を差し替える。
//
// 種目マスタを読むのは、宣言種目がどの分割にも属さない状態を弾くため。
// 属さない種目は毎日「今日の候補ではない」と判定され、**二度と軸に
// 出ない**。他の宣言が毎日1つは該当するのでフォールバックも発火せず、
// エラーも立たないまま消える。
type SetSplitCycle struct {
	exercises exercise.Reader
	reader    program.Reader
	writer    program.Writer
}

func NewSetSplitCycle(
	exercises exercise.Reader,
	reader program.Reader,
	writer program.Writer,
) *SetSplitCycle {
	return &SetSplitCycle{exercises: exercises, reader: reader, writer: writer}
}

func (u *SetSplitCycle) Execute(ctx context.Context, user account.UserID, cycle []program.Split) (err error) {
	defer func() { err = apperror.Classify(err) }()

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	next, err := prog.WithCycle(cycle)
	if err != nil {
		return fmt.Errorf("%w: 分割: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("分割の保存が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	if err := verifyDeclaredHaveADay(pool, next); err != nil {
		return err
	}

	return u.writer.Save(ctx, user, next)
}
