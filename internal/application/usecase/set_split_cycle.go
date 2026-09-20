package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// primaryContribution は「主働」とみなす寄与の下限。
//
// AccessorySelector と同じ値。最大値を取る方式にしないのは、デッドリフトが
// ハムストリングと脊柱起立筋のどちらも 1.0 で、並びのアルファベット順に
// 落ちてしまうため。閾値なら「両方の日の候補」になり、どちらに出るかは
// 最終実施日が決める。
const primaryContribution = 1.0

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

func (u *SetSplitCycle) Execute(ctx context.Context, cycle []program.Split) (err error) {
	defer func() { err = classify(err) }()

	prog, err := u.reader.Get(ctx)
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

	return u.writer.Save(ctx, next)
}

// verifyDeclaredHaveADay は、どの分割にも出られない宣言種目が無いことを確かめる。
func verifyDeclaredHaveADay(pool []*exercise.Exercise, prog *program.Program) error {
	cycle := prog.Cycle()
	if len(cycle) == 0 {
		return nil
	}

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	for _, e := range pool {
		if e != nil {
			byID[e.ID()] = e
		}
	}

	for _, id := range prog.DeclaredExercises() {
		e, ok := byID[id]
		if !ok {
			// 選択との突合は ConfigureProgram が済ませている。ここで
			// 見つからないのは保存済みの不整合なので、分割の問題として
			// 扱わずそのまま通す。
			continue
		}
		if !hasADay(e, cycle) {
			return fmt.Errorf(
				"%w: 伸ばしたい種目 %q が出られる日が無い。主働の筋区分をどれかの分割に入れること",
				apperror.ErrInvalidInput, id)
		}
	}
	return nil
}

// hasADay はその種目の主働区分を含む分割が周期にあるか。
func hasADay(e *exercise.Exercise, cycle []program.Split) bool {
	for _, s := range cycle {
		for _, r := range e.Stimulus().Regions() {
			c, ok := e.Stimulus().Contribution(r)
			if ok && c.Float() >= primaryContribution && s.Includes(r) {
				return true
			}
		}
	}
	return false
}
