package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
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

// verifyDeclaredHaveADay は、どの分割にも出られない宣言種目が無いことを確かめる。
//
// 出られるかどうかの判断は planning が持つ。ここに写しを置くと、計画の側と
// 閾値がずれたときに「保存は通るのに軸に出ない」が黙って起きる。
//
// pool に無い宣言種目は planning が返さないので、そのまま通る。選択との
// 突合は ConfigureProgram が済ませていて、ここで見つからないのは保存済みの
// 不整合であり、分割の問題ではない。
func verifyDeclaredHaveADay(pool []*exercise.Exercise, prog *program.Program) error {
	without := planning.DeclaredWithoutADay(pool, prog)
	if len(without) == 0 {
		return nil
	}
	// 1件ずつ直してもらう。宣言の順なので、同じ入力なら同じ種目を指す。
	return fmt.Errorf(
		"%w: 伸ばしたい種目 %q が出られる日が無い。主働の筋区分をどれかの分割に入れること",
		apperror.ErrInvalidInput, without[0])
}
