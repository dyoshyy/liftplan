package usecase

import (
	"context"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetDeclaredExercises は伸ばしたい種目だけを差し替える。
//
// SetFocusExercise と同じ形。受け取るのは宣言だけで、クライアントに設定を
// 丸ごと送らせない。週目標と選択種目はここを通らない。
type SetDeclaredExercises struct {
	exercises exercise.Reader
	reader    program.Reader
	writer    program.Writer
}

func NewSetDeclaredExercises(
	exercises exercise.Reader,
	reader program.Reader,
	writer program.Writer,
) *SetDeclaredExercises {
	return &SetDeclaredExercises{exercises: exercises, reader: reader, writer: writer}
}

// Execute は伸ばしたい種目を差し替える。
//
// 種目マスタを読むのは、分割のどの日にも出られない種目を宣言させない
// ため（#140）。SetSplitCycle が同じ状態を弾いているので、こちらが見て
// いないと「分割 → 宣言」の順に保存するだけで同じ状態を作れる。
//
// 実在の確認のためには読んでいない。そこは SetFocusExercise と同じで、
// declared ⊂ selected を NewProgram が確かめ、selected はプログラムを
// 保存した時点で verifySelection を通っている。マスタに無い種目は
// selected に入らないので、declared にも入りようがない。
func (u *SetDeclaredExercises) Execute(ctx context.Context, user account.UserID, ids []exercise.ExerciseID) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	next, err := prog.WithDeclared(ids)
	if err != nil {
		return fmt.Errorf("%w: 伸ばしたい種目: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("伸ばしたい種目の保存が中断された: %w", err)
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
