package usecase

import (
	"context"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetFrequency は週の頻度を差し替える。
//
// 週目標はもう集約の持ち物ではなく、頻度と1回の量から都度導く値
// （D-139、#176）。頻度を差し替えれば、導いた先の値も自動でついてくる。
type SetFrequency struct {
	reader program.Reader
	writer program.Writer
}

func NewSetFrequency(reader program.Reader, writer program.Writer) *SetFrequency {
	return &SetFrequency{reader: reader, writer: writer}
}

// Execute は頻度を差し替える。
//
// 頻度の検証は I/O より先に済ませる。範囲内かどうかは入力だけで決まる
// ので、読む前に分かる。後回しにすると、範囲外という自明な入力ミスが
// 保存先の障害時に別の顔で返る。
func (u *SetFrequency) Execute(ctx context.Context, user account.UserID, perWeek int) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

	freq, err := program.NewFrequency(perWeek)
	if err != nil {
		return fmt.Errorf("%w: 頻度: %w", apperror.ErrInvalidInput, err)
	}
	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	next, err := prog.WithFrequency(freq)
	if err != nil {
		return fmt.Errorf("%w: 頻度: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("頻度の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, user, next)
}
