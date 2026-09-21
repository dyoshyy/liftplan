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

// SetWeeklyTarget は週目標だけを差し替える。
//
// 種目マスタを読むのは SetSelectedExercises と同じ理由。週目標のどの区分も
// 刺激しない組み合わせになると、補助種目が毎回ゼロになったままエラーが
// 立たない。選択を動かしても目標を動かしても同じ穴に落ちるので、両方で見る。
type SetWeeklyTarget struct {
	exercises exercise.Reader
	reader    program.Reader
	writer    program.Writer
}

func NewSetWeeklyTarget(
	exercises exercise.Reader,
	reader program.Reader,
	writer program.Writer,
) *SetWeeklyTarget {
	return &SetWeeklyTarget{exercises: exercises, reader: reader, writer: writer}
}

func (u *SetWeeklyTarget) Execute(ctx context.Context, user account.UserID, sets map[training.MuscleRegion]float64) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = classify(err) }()

	target, err := program.NewWeeklyVolumeTarget(sets)
	if err != nil {
		return fmt.Errorf("%w: 週目標: %w", apperror.ErrInvalidInput, err)
	}

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	next, err := prog.WithTarget(target)
	if err != nil {
		return fmt.Errorf("%w: 週目標: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("週目標の保存が中断された: %w", err)
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
