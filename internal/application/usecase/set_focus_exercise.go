package usecase

import (
	"context"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetFocusExercise は重点種目だけを差し替える。
//
// ConfigureProgram と分けているのは、クライアントに全置換をさせないため。
// 全置換だと重点種目を変えるだけで週目標21区分と選択種目30件が往復し、
// DTO がずれれば 400、ずれなければ週目標を黙って上書きする経路ができる
// （D-120 が消したのはその経路）。
//
// exercise.Reader を持たないのは、種目の実在を確かめる必要が無いため。
// focus ⊂ declared ⊂ selected で、selected は保存時に verifySelection を
// 通っている。マスタに無い種目は declared にも入れない。
type SetFocusExercise struct {
	reader program.Reader
	writer program.Writer
}

func NewSetFocusExercise(reader program.Reader, writer program.Writer) *SetFocusExercise {
	return &SetFocusExercise{reader: reader, writer: writer}
}

// Execute は重点種目を差し替える。空の ID は「指定なし」に戻す。
//
// ConfigureProgram と違って I/O より先に検証できない。重点種目が妥当かは
// 宣言種目を見ないと決まらず、宣言種目は保存済みのプログラムの中にある。
func (u *SetFocusExercise) Execute(ctx context.Context, user account.UserID, focus exercise.ExerciseID) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	next, err := prog.WithFocus(focus)
	if err != nil {
		return fmt.Errorf("%w: 重点種目: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("重点種目の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, user, next)
}
