package usecase

import (
	"context"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetSelectedExercises は使う種目だけを差し替える。
//
// 他の狭い口と違って種目マスタを読む。focus と declared は選択の部分集合で、
// 選択はプログラムを保存した時点で検証済みだったので突合が要らなかった。
// 選択そのものを動かすと、実在しない種目が入る経路と、週目標のどの区分も
// 刺激しない組み合わせになる経路が開く。どちらも verifySelection が見る。
type SetSelectedExercises struct {
	exercises exercise.Reader
	reader    program.Reader
	writer    program.Writer
}

func NewSetSelectedExercises(
	exercises exercise.Reader,
	reader program.Reader,
	writer program.Writer,
) *SetSelectedExercises {
	return &SetSelectedExercises{exercises: exercises, reader: reader, writer: writer}
}

func (u *SetSelectedExercises) Execute(ctx context.Context, user account.UserID, ids []exercise.ExerciseID) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = classify(err) }()

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	next, err := prog.WithSelected(ids)
	if err != nil {
		return fmt.Errorf("%w: 使う種目: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("使う種目の保存が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	if err := verifySelection(pool, next); err != nil {
		return err
	}

	return u.writer.Save(ctx, user, next)
}
