package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// EditExerciseInput は種目を直す入力。Add と同じ形（global-constraints
// 「直せるのは名前・効き方・刻み」）。
type EditExerciseInput struct {
	Name        string
	Stimulus    map[training.MuscleRegion]float64
	IncrementKg float64
}

// EditExercise は利用者が種目の名前・効き方・刻みを直す。
//
// プリセット由来かどうかで扱いを変えない。ID・自重係数・派生元は
// Exercise.Edit がそのまま引き継ぐので、ここでは触らない。
type EditExercise struct {
	exercises exerciseStore
}

func NewEditExercise(exercises exerciseStore) *EditExercise {
	return &EditExercise{exercises: exercises}
}

func (u *EditExercise) Execute(ctx context.Context, user account.UserID, id exercise.ExerciseID, in EditExerciseInput) (_ *exercise.Exercise, err error) {
	defer func() { err = apperror.Classify(err) }()

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	var target *exercise.Exercise
	for _, e := range pool {
		if e != nil && e.ID() == id && !e.IsDeleted() {
			target = e
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%w: %s", apperror.ErrExerciseNotFound, id)
	}

	edited, err := target.Edit(exercise.ExerciseEdit{
		Name: in.Name, Stimulus: in.Stimulus, IncrementKg: in.IncrementKg,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", apperror.ErrInvalidInput, err)
	}

	// 重複チェックは自分自身を除く。消していない他の種目とだけ比べる
	// （global-constraints「直すときは自分自身を除く」）。
	for _, other := range pool {
		if other != nil && other.ID() != id && !other.IsDeleted() && other.Name() == edited.Name() {
			return nil, fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, edited.Name())
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("種目の変更が中断された: %w", err)
	}
	if err := u.exercises.Save(ctx, user, edited); err != nil {
		return nil, err
	}
	return edited, nil
}
