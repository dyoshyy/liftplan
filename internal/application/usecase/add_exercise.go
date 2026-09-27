package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// exerciseStore は種目の読み書き。足す・消すは両方要る。
type exerciseStore interface {
	exercise.Reader
	exercise.Writer
}

// AddExerciseInput は利用者が足す種目の入力。
//
// プリセット由来かどうかで扱いを変えない（global-constraints）。効き方は
// 区分ごとの寄与度の生の値で受け取り、検証は exercise.NewExercise に
// 一本化する。
type AddExerciseInput struct {
	Name        string
	Stimulus    map[training.MuscleRegion]float64
	IncrementKg float64
}

// AddExercise は利用者が種目を足す。
//
// 足した種目は使う種目にも入れる。足すのは使うためで、チェックを入れ直す
// 手間を残さない。種目の保存とプログラムの保存の間で落ちると、種目は
// あるが選ばれていない状態になる。設定画面でチェックを入れれば済むので、
// トランザクションは張らない（設計書「ユースケース」）。
type AddExercise struct {
	exercises exerciseStore
	reader    program.Reader
	writer    program.Writer
}

func NewAddExercise(exercises exerciseStore, reader program.Reader, writer program.Writer) *AddExercise {
	return &AddExercise{exercises: exercises, reader: reader, writer: writer}
}

func (u *AddExercise) Execute(ctx context.Context, user account.UserID, in AddExerciseInput) (_ *exercise.Exercise, err error) {
	defer func() { err = apperror.Classify(err) }()

	// プログラムが無ければ種目を作る前に止める。作ってから止まると、
	// 選ばれていない種目だけが残る。
	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return nil, err
	}

	id, err := exercise.NewRandomExerciseID()
	if err != nil {
		return nil, err
	}
	e, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: string(id), Name: in.Name,
		Stimulus: in.Stimulus, IncrementKg: in.IncrementKg,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", apperror.ErrInvalidInput, err)
	}

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	for _, other := range pool {
		if other != nil && !other.IsDeleted() && other.Name() == e.Name() {
			return nil, fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, e.Name())
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("種目の保存が中断された: %w", err)
	}
	if err := u.exercises.Save(ctx, user, e); err != nil {
		return nil, err
	}

	next, err := prog.WithSelected(append(prog.SelectedExercises(), e.ID()))
	if err != nil {
		return nil, err
	}
	if err := u.writer.Save(ctx, user, next); err != nil {
		return nil, err
	}
	return e, nil
}
