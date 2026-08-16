package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// ErrInvalidInput は入力そのものが不正であることを表す。
//
// プレゼンテーション層が 400 と 500 を区別するための分類。ドメインの
// コンストラクタが返すのは匿名のエラーなので、包み直さないと
// 「ユーザーの入力が悪い」と「データベースが落ちている」が同じ形になる。
// 区別できないと、入力ミスが全部 500 になるか I/O 障害が全部 400 になるかの
// 二択になり、前者はクライアントに無駄なリトライをさせる。
var ErrInvalidInput = errors.New("入力が不正である")

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
// 選択された種目が種目マスタと噛み合うかを突合するのはここ。Program は
// ExerciseID しか持たず、種目の実在も種類も知らないので自分では検証できない。
// 突合しないと、保存は成功するのに以後のセッション導出が壊れる。
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
	// I/O を必要としない検証を先に済ませる。後回しにすると、頻度が範囲外
	// という自明な入力ミスが、種目マスタの障害時に「種目の取得に失敗」として
	// 返る。クライアントは自分の入力を直さずリトライを繰り返す。
	frequency, err := training.NewFrequency(in.PerWeek)
	if err != nil {
		return fmt.Errorf("%w: 頻度: %w", ErrInvalidInput, err)
	}
	target, err := training.NewWeeklyVolumeTarget(in.Target)
	if err != nil {
		return fmt.Errorf("%w: 週目標: %w", ErrInvalidInput, err)
	}
	program, err := training.NewProgram(frequency, target, in.Selected)
	if err != nil {
		return fmt.Errorf("%w: プログラム: %w", ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("設定の保存が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	if err := verifySelection(pool, program); err != nil {
		return err
	}

	if err := u.programs.Save(ctx, program); err != nil {
		return fmt.Errorf("プログラムの保存に失敗: %w", err)
	}
	return nil
}

// verifySelection は選択された種目を種目マスタと突合する。
func verifySelection(pool []*training.Exercise, program *training.Program) error {
	known := make(map[training.ExerciseID]*training.Exercise, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		known[e.ID()] = e
	}

	selected := make([]*training.Exercise, 0, len(pool))
	for _, id := range program.SelectedExercises() {
		e, ok := known[id]
		if !ok {
			return fmt.Errorf("%w: %w: %s", ErrInvalidInput, training.ErrExerciseNotFound, id)
		}
		selected = append(selected, e)
	}

	// メイン種目が1つも無いと SessionPlanner が致命エラーを返す。
	// 保存を通すと、以後すべてのセッション導出が失敗し続ける。
	hasMain := false
	for _, e := range selected {
		if e.Kind() == training.KindMain {
			hasMain = true
			break
		}
	}
	if !hasMain {
		return fmt.Errorf("%w: %w", ErrInvalidInput, training.ErrNoMainExercise)
	}

	// 週目標のどの区分も刺激しない選択は、補助種目が毎回ゼロになる。
	// エラーが立たないまま「設定した週目標が永久に埋まらない」状態になる。
	//
	// 区分ごとに種目を要求はしない。特定の区分を埋める種目を持っていない
	// のは普通のことで、その区分の達成率が低く出るのは情報として正しい。
	// 弾くのは、目標と選択がまったく噛み合っていない場合だけ。
	if !stimulatesAnyTarget(selected, known, program) {
		return fmt.Errorf(
			"%w: 選択した種目が週目標のどの筋区分も刺激しない: %v",
			ErrInvalidInput, sortedRegions(program.WeeklyTarget()))
	}
	return nil
}

// stimulatesAnyTarget は選択した種目が週目標の区分を1つでも刺激するか。
//
// バリエーションは選択に含まれないがメインに付随して自動で回るので、
// 選択されたメインリフトの派生も数える。
func stimulatesAnyTarget(
	selected []*training.Exercise,
	known map[training.ExerciseID]*training.Exercise,
	program *training.Program,
) bool {
	lifts := map[training.MainLift]bool{}
	for _, e := range selected {
		if lift, ok := e.MainLift(); ok && e.Kind() == training.KindMain {
			lifts[lift] = true
		}
	}

	effective := append([]*training.Exercise{}, selected...)
	for _, e := range known {
		if e.Kind() != training.KindVariation {
			continue
		}
		if lift, ok := e.MainLift(); ok && lifts[lift] {
			effective = append(effective, e)
		}
	}

	target := program.WeeklyTarget()
	for _, e := range effective {
		for _, r := range e.Stimulus().Regions() {
			if target.Sets(r) > 0 {
				return true
			}
		}
	}
	return false
}

func sortedRegions(t training.WeeklyVolumeTarget) []training.MuscleRegion {
	out := t.Regions()
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
