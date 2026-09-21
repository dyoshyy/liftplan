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

// ConfigureProgramInput はプログラム設定の入力。
//
// 週目標を渡せるようにしているのは、シードのプリセットが出発点でしかないため。
// 不満が出た区分だけ後から調整できる。
type ConfigureProgramInput struct {
	PerWeek  int
	Target   map[training.MuscleRegion]float64
	Selected []exercise.ExerciseID
	Declared []exercise.ExerciseID
	Focus    exercise.ExerciseID // 空なら指定なし
}

// ConfigureProgram はユーザーのプログラム設定を保存するユースケース。
//
// 選択された種目が種目マスタと噛み合うかを突合するのはここ。Program は
// ExerciseID しか持たず、種目の実在も種類も知らないので自分では検証できない。
// 突合しないと、保存は成功するのに以後のセッション導出が壊れる。
//
// 突合は「判断」ではなく入力検証なので、ドメインではなくここに置く。
// 種目マスタの取得が I/O である以上、ドメインには置けない。
type ConfigureProgram struct {
	exercises exercise.Reader
	programs  program.Writer
}

func NewConfigureProgram(
	exercises exercise.Reader,
	programs program.Writer,
) *ConfigureProgram {
	return &ConfigureProgram{exercises: exercises, programs: programs}
}

func (u *ConfigureProgram) Execute(ctx context.Context, user account.UserID, in ConfigureProgramInput) (err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = classify(err) }()

	// I/O を必要としない検証を先に済ませる。後回しにすると、頻度が範囲外
	// という自明な入力ミスが、種目マスタの障害時に「種目の取得に失敗」として
	// 返る。クライアントは自分の入力を直さずリトライを繰り返す。
	frequency, err := program.NewFrequency(in.PerWeek)
	if err != nil {
		return fmt.Errorf("%w: 頻度: %w", apperror.ErrInvalidInput, err)
	}
	target, err := program.NewWeeklyVolumeTarget(in.Target)
	if err != nil {
		return fmt.Errorf("%w: 週目標: %w", apperror.ErrInvalidInput, err)
	}
	prog, err := program.NewProgram(frequency, target, in.Selected, in.Declared, in.Focus)
	if err != nil {
		return fmt.Errorf("%w: プログラム: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("設定の保存が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	if err := verifySelection(pool, prog); err != nil {
		return err
	}

	if err := u.programs.Save(ctx, user, prog); err != nil {
		return fmt.Errorf("プログラムの保存に失敗: %w", err)
	}
	return nil
}
