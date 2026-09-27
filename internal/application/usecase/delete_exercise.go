package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// DeleteExercise は利用者が種目を消す（論理削除）。
//
// プリセット由来かどうかで扱いを変えない
// （docs/specs/2026-09-26-custom-exercises-design.md「プリセット由来も消せる・直せる」）。
// 共通の種目もその利用者の一覧に写っているので、消せば以後その利用者からだけ
// 見えなくなる。
//
// **プログラムを先に、種目を後に書く。**逆順だと、途中で落ちたときに
// 消した種目が使う種目に残り、verifySelection がそれを弾くので、以後の
// 選択の保存が失敗し続ける。この順なら途中で落ちても「チェックが外れた
// だけ」で、もう一度消せば終わる。
type DeleteExercise struct {
	exercises exerciseStore
	reader    program.Reader
	writer    program.Writer
}

func NewDeleteExercise(exercises exerciseStore, reader program.Reader, writer program.Writer) *DeleteExercise {
	return &DeleteExercise{exercises: exercises, reader: reader, writer: writer}
}

func (u *DeleteExercise) Execute(ctx context.Context, user account.UserID, id exercise.ExerciseID) (err error) {
	defer func() { err = apperror.Classify(err) }()

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	var target *exercise.Exercise
	for _, e := range pool {
		if e != nil && e.ID() == id {
			target = e
			break
		}
	}
	if target == nil {
		return fmt.Errorf("%w: %s", apperror.ErrExerciseNotFound, id)
	}
	// 消した種目をもう一度消しても、同じ手順をなぞって成功する（冪等）。
	// 「消してあれば何もしない」で抜けると、前回プログラムの保存だけが
	// 落ちていた場合に、選択に残った ID を外す機会が無くなる。

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	if prog.Declares(id) {
		return fmt.Errorf("%w: %s", apperror.ErrStillDeclared, target.Name())
	}

	rest := make([]exercise.ExerciseID, 0, len(prog.SelectedExercises()))
	for _, s := range prog.SelectedExercises() {
		if s != id {
			rest = append(rest, s)
		}
	}
	next, err := prog.WithSelected(rest)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("種目の削除が中断された: %w", err)
	}
	if err := u.writer.Save(ctx, user, next); err != nil {
		return fmt.Errorf("使う種目の保存に失敗: %w", err)
	}
	if err := u.exercises.Save(ctx, user, target.Delete()); err != nil {
		return fmt.Errorf("種目の削除の保存に失敗: %w", err)
	}
	return nil
}
