package usecase

import (
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

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
