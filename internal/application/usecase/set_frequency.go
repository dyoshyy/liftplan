package usecase

import (
	"context"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// SetFrequency は週の頻度を差し替える。週目標も頻度に合わせて置き直す。
//
// 週目標を道連れにするのは、1週間に供給できるセット数が頻度に比例する
// ため（seed.DefaultWeeklyTarget のコメント）。頻度だけ動かすと、目標が
// 実際の挙動を説明しなくなる。
//
// 手で調整した週目標があれば上書きされる。「不満が出た区分だけ後から
// 調整すればよく、最初から自分で全部決める必要はない」という既定値の
// 設計意図（seed）に沿った扱いで、頻度を変えたら調整もやり直しになる。
type SetFrequency struct {
	reader program.Reader
	writer program.Writer
}

func NewSetFrequency(reader program.Reader, writer program.Writer) *SetFrequency {
	return &SetFrequency{reader: reader, writer: writer}
}

// Execute は頻度を差し替える。
//
// 頻度の検証を I/O より先に済ませるのは ConfigureProgram と同じ。後回しに
// すると、範囲外という自明な入力ミスが保存先の障害時に別の顔で返る。
func (u *SetFrequency) Execute(ctx context.Context, user account.UserID, perWeek int) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

	freq, err := program.NewFrequency(perWeek)
	if err != nil {
		return fmt.Errorf("%w: 頻度: %w", apperror.ErrInvalidInput, err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		return fmt.Errorf("%w: 週目標: %w", apperror.ErrInvalidInput, err)
	}

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	next, err := prog.WithFrequency(freq, target)
	if err != nil {
		return fmt.Errorf("%w: 頻度: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("頻度の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, user, next)
}
