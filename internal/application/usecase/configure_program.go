package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// ConfigureProgramInput はプログラム設定の入力。
//
// 週目標を渡せるようにしているのは、シードのプリセットが出発点でしかないため。
// 不満が出た区分だけ後から調整できる。
type ConfigureProgramInput struct {
	PerWeek  int
	Target   map[training.MuscleRegion]float64
	Selected []training.ExerciseID
}

// ConfigureProgram はユーザーのプログラム設定を保存するユースケース。
//
// 選択された種目IDが種目マスタに実在するかを突合するのはここ。
// Program は種目マスタを知らないので自分では検証できず、実在しないIDは
// SessionPlanner が黙って落とす。突合しないと、シードから種目を削除・
// リネームした瞬間に、ユーザーが選んだ種目が理由の説明なく消える。
//
// 突合は「判断」ではなく入力検証なので、ドメインではなくここに置く。
// 種目マスタの取得が I/O である以上、ドメインには置けない。
type ConfigureProgram struct {
	exercises training.ExerciseRepository
	programs  training.ProgramRepository
}

func NewConfigureProgram(
	exercises training.ExerciseRepository,
	programs training.ProgramRepository,
) *ConfigureProgram {
	return &ConfigureProgram{exercises: exercises, programs: programs}
}

func (u *ConfigureProgram) Execute(ctx context.Context, in ConfigureProgramInput) error {
	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}

	known := make(map[training.ExerciseID]bool, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		known[e.ID()] = true
	}
	for _, id := range in.Selected {
		if !known[id] {
			return fmt.Errorf("%w: %s", training.ErrExerciseNotFound, id)
		}
	}

	frequency, err := training.NewFrequency(in.PerWeek)
	if err != nil {
		return fmt.Errorf("頻度: %w", err)
	}
	target, err := training.NewWeeklyVolumeTarget(in.Target)
	if err != nil {
		return fmt.Errorf("週目標: %w", err)
	}
	program, err := training.NewProgram(frequency, target, in.Selected)
	if err != nil {
		return fmt.Errorf("プログラム: %w", err)
	}

	if err := u.programs.Save(ctx, program); err != nil {
		return fmt.Errorf("プログラムの保存に失敗: %w", err)
	}
	return nil
}
