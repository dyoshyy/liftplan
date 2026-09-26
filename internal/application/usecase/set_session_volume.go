package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// SetSessionVolume は1回の量（種目数 × 1種目あたりのセット数）を差し替える。
// 週目標も1回の量に合わせて置き直す。
//
// 週目標を道連れにするのは SetFrequency と同じ理由。週に供給できる量は
// 「頻度 × 種目数 × セット数」で決まるので、量だけ動かすと目標が実際の
// 挙動を説明しなくなる。
type SetSessionVolume struct {
	reader program.Reader
	writer program.Writer
}

func NewSetSessionVolume(reader program.Reader, writer program.Writer) *SetSessionVolume {
	return &SetSessionVolume{reader: reader, writer: writer}
}

// Execute は1回の量を差し替える。
//
// 量の検証を I/O より先に済ませるのは SetFrequency と同じ。後回しにすると、
// 範囲外という自明な入力ミスが保存先の障害時に別の顔で返る。
func (u *SetSessionVolume) Execute(ctx context.Context, user account.UserID, exercises, sets int) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

	volume, err := program.NewSessionVolume(exercises, sets)
	if err != nil {
		return fmt.Errorf("%w: 1回の量: %w", apperror.ErrInvalidInput, err)
	}

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	target, err := seed.DefaultWeeklyTarget(prog.Frequency(), volume)
	if err != nil {
		return fmt.Errorf("%w: 週目標: %w", apperror.ErrInvalidInput, err)
	}
	next, err := prog.WithSessionVolume(volume, target)
	if err != nil {
		return fmt.Errorf("%w: 1回の量: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("1回の量の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, user, next)
}
