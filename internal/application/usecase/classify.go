package usecase

import (
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// classify はドメインとインフラのエラーを、アプリケーションの分類に翻訳する。
//
// ここが翻訳の唯一の地点。presentation はドメインのパッケージを知らずに
// errors.As だけで応答を決められる。センチネルを足したときに直す場所も
// ここ1つになる。
//
// 元のエラーは連鎖に残す。ユースケースの利用者は errors.Is でドメインの
// センチネルを見ているので、切ると判断の根拠が消える。
//
// 分類できないものはそのまま返す。presentation の既定が 500 にして、
// 中身をログにだけ残す。知らないものを黙って 400 や 409 にするより、
// 500 で目立たせるほうがよい。
func classify(err error) error {
	if err == nil {
		return nil
	}

	var coded *apperror.Error
	if errors.As(err, &coded) {
		return err
	}

	switch {
	case errors.Is(err, program.ErrProgramNotConfigured):
		return fmt.Errorf("%w: %w", apperror.ErrNotConfigured, err)
	case errors.Is(err, setlog.ErrConflictingSetLog):
		return fmt.Errorf("%w: %w", apperror.ErrConflict, err)
	case errors.Is(err, training.ErrRepositoryUnavailable):
		return fmt.Errorf("%w: %w", apperror.ErrUnavailable, err)
	default:
		return err
	}
}
