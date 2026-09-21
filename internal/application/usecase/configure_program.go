package usecase

import (
	"context"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"sort"

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

// verifySelection は選択された種目を種目マスタと突合する。
func verifySelection(pool []*exercise.Exercise, prog *program.Program) error {
	known := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		known[e.ID()] = e
	}

	selected := make([]*exercise.Exercise, 0, len(pool))
	for _, id := range prog.SelectedExercises() {
		e, ok := known[id]
		if !ok {
			return fmt.Errorf("%w: %w: %s", apperror.ErrInvalidInput, exercise.ErrExerciseNotFound, id)
		}
		selected = append(selected, e)
	}

	// 週目標のどの区分も刺激しない選択は、補助種目が毎回ゼロになる。
	// エラーが立たないまま「設定した週目標が永久に埋まらない」状態になる。
	//
	// 区分ごとに種目を要求はしない。特定の区分を埋める種目を持っていない
	// のは普通のことで、その区分の達成率が低く出るのは情報として正しい。
	// 弾くのは、目標と選択がまったく噛み合っていない場合だけ。
	if !stimulatesAnyTarget(selected, prog) {
		return fmt.Errorf(
			"%w: 選択した種目が週目標のどの筋区分も刺激しない: %v",
			apperror.ErrInvalidInput, sortedRegions(prog.WeeklyTarget()))
	}
	return nil
}

// stimulatesAnyTarget は選択した種目が週目標の区分を1つでも刺激するか。
//
// 以前はここで「選択されたメインリフトの派生」も数えていた。バリエーションが
// 選択に含まれなくても自動で回る抜け道があったため。抜け道を塞いだので、
// 選択された種目だけを見ればよい。
func stimulatesAnyTarget(
	selected []*exercise.Exercise,
	prog *program.Program,
) bool {
	target := prog.WeeklyTarget()
	for _, e := range selected {
		for _, r := range e.Stimulus().Regions() {
			if target.Sets(r) > 0 {
				return true
			}
		}
	}
	return false
}

func sortedRegions(t program.WeeklyVolumeTarget) []training.MuscleRegion {
	out := t.Regions()
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
