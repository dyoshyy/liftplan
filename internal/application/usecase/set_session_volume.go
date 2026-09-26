package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetSessionVolume は1回の量（種目数 × 1種目あたりのセット数）を差し替える。
//
// 週目標はもう集約の持ち物ではなく、頻度と1回の量から都度導く値
// （D-139、#176）。量を差し替えれば、導いた先の値も自動でついてくる。
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

	next, err := prog.WithSessionVolume(volume)
	if err != nil {
		return fmt.Errorf("%w: 1回の量: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("1回の量の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, user, next)
}
