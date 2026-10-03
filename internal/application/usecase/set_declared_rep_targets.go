package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetDeclaredRepTargets は1つの宣言のレップ数（重い番・軽い番）だけを差し替える。
//
// SetFocusExercise と同じ形。受け取るのはその宣言の値だけで、プログラムの
// 残りは保存済みのものを使う。
//
// exercise.Reader を持たないのは、種目の実在を確かめる必要が無いため。
// 宣言していない種目は WithRepTargets が弾き、宣言は保存時に種目マスタを
// 通っている。
type SetDeclaredRepTargets struct {
	reader program.Reader
	writer program.Writer
}

func NewSetDeclaredRepTargets(reader program.Reader, writer program.Writer) *SetDeclaredRepTargets {
	return &SetDeclaredRepTargets{reader: reader, writer: writer}
}

// Execute は id のレップ数を差し替える。範囲外・宣言していない種目は入力の誤り。
func (u *SetDeclaredRepTargets) Execute(
	ctx context.Context, user account.UserID, id exercise.ExerciseID, heavy, light int,
) (err error) {
	// 出口で1度だけ翻訳する（SetFocusExercise と同じ理由）。
	defer func() { err = apperror.Classify(err) }()

	reps, err := program.NewRepTargets(heavy, light)
	if err != nil {
		return fmt.Errorf("%w: レップ数: %w", apperror.ErrInvalidInput, err)
	}

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}
	next, err := prog.WithRepTargets(id, reps)
	if err != nil {
		return fmt.Errorf("%w: レップ数: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("レップ数の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, user, next)
}
