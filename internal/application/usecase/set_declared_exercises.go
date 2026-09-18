package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetDeclaredExercises は伸ばしたい種目だけを差し替える。
//
// SetFocusExercise と同じ形。クライアントに全置換をさせないために口を
// 分けている。週目標と選択種目はここを通らない。
type SetDeclaredExercises struct {
	reader program.Reader
	writer program.Writer
}

func NewSetDeclaredExercises(reader program.Reader, writer program.Writer) *SetDeclaredExercises {
	return &SetDeclaredExercises{reader: reader, writer: writer}
}

// Execute は伸ばしたい種目を差し替える。
//
// exercise.Reader を持たないのは SetFocusExercise と同じ理由。
// declared ⊂ selected を NewProgram が確かめ、selected はプログラムを
// 保存した時点で verifySelection を通っている。マスタに無い種目は
// selected に入らないので、declared にも入りようがない。
func (u *SetDeclaredExercises) Execute(ctx context.Context, ids []exercise.ExerciseID) error {
	prog, err := u.reader.Get(ctx)
	if err != nil {
		return err
	}

	next, err := prog.WithDeclared(ids)
	if err != nil {
		return fmt.Errorf("%w: 伸ばしたい種目: %w", ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("伸ばしたい種目の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, next)
}
