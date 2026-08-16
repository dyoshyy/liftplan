package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

func TestExercises_AreValid(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("シードが空である")
	}
}

func TestExercises_ContainsBigThree(t *testing.T) {
	all, _ := seed.Exercises()
	found := map[training.MainLift]bool{}
	for _, e := range all {
		if e.Kind() != training.KindMain {
			continue
		}
		if lift, ok := e.MainLift(); ok {
			found[lift] = true
		}
	}
	for _, want := range []training.MainLift{
		training.LiftSquat, training.LiftBench, training.LiftDeadlift,
	} {
		if !found[want] {
			t.Errorf("%s がメイン種目に無い", want)
		}
	}
}

func TestExercises_IDsAreUnique(t *testing.T) {
	all, _ := seed.Exercises()
	seen := map[training.ExerciseID]bool{}
	for _, e := range all {
		if seen[e.ID()] {
			t.Errorf("種目IDが重複している: %s", e.ID())
		}
		seen[e.ID()] = true
	}
}

func TestExercises_VariationsBelongToAMainLift(t *testing.T) {
	all, _ := seed.Exercises()
	count := 0
	for _, e := range all {
		if e.Kind() != training.KindVariation {
			continue
		}
		count++
		if _, ok := e.MainLift(); !ok {
			t.Errorf("%s に所属メインが無い", e.ID())
		}
		if _, ok := e.DefaultRatioToMain(); !ok {
			t.Errorf("%s に対メイン係数が無い", e.ID())
		}
	}
	if count == 0 {
		t.Error("バリエーションが1つも無い")
	}
}

func TestExercises_EveryMainLiftHasAVariation(t *testing.T) {
	all, _ := seed.Exercises()
	covered := map[training.MainLift]bool{}
	for _, e := range all {
		if e.Kind() != training.KindVariation {
			continue
		}
		if lift, ok := e.MainLift(); ok {
			covered[lift] = true
		}
	}
	for _, want := range []training.MainLift{
		training.LiftSquat, training.LiftBench, training.LiftDeadlift,
	} {
		if !covered[want] {
			t.Errorf("%s のバリエーションが無い", want)
		}
	}
}

func TestExercises_AccessoriesCoverEveryRegion(t *testing.T) {
	all, _ := seed.Exercises()
	covered := map[training.MuscleRegion]bool{}
	for _, e := range all {
		if e.Kind() != training.KindAccessory {
			continue
		}
		for _, r := range e.Stimulus().Regions() {
			covered[r] = true
		}
	}
	for _, r := range training.AllMuscleRegions() {
		if !covered[r] {
			t.Errorf("補助種目でカバーできない筋区分がある: %s", r)
		}
	}
}

func TestDefaultWeeklyTarget_CoversEveryRegion(t *testing.T) {
	target, err := seed.DefaultWeeklyTarget()
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	for _, r := range training.AllMuscleRegions() {
		if target.Sets(r) <= 0 {
			t.Errorf("筋区分 %s の目標が設定されていない", r)
		}
	}
}

// 対メイン係数が現実的な帯に入っていること。
// 帯を外れた値は VariationRatioResolver が実測で上書きするまで
// そのまま重量に効くので、初期値の時点で危険な数字を置かない。
func TestExercises_VariationRatiosAreRealistic(t *testing.T) {
	all, _ := seed.Exercises()
	for _, e := range all {
		if e.Kind() != training.KindVariation {
			continue
		}
		ratio, ok := e.DefaultRatioToMain()
		if !ok {
			continue
		}
		if r := ratio.Float(); r < 0.5 || r > 1.0 {
			t.Errorf("%s の対メイン係数が現実的でない: %v", e.ID(), r)
		}
	}
}

// 名前が空でなく重複しないこと。ユーザーが種目を選ぶ画面で
// 同じ名前が並ぶと選べない。
func TestExercises_NamesAreUniqueAndNotEmpty(t *testing.T) {
	all, _ := seed.Exercises()
	seen := map[string]bool{}
	for _, e := range all {
		if e.Name() == "" {
			t.Errorf("%s の名前が空である", e.ID())
		}
		if seen[e.Name()] {
			t.Errorf("種目名が重複している: %s", e.Name())
		}
		seen[e.Name()] = true
	}
}

// 呼ぶたびに別のインスタンスを返すこと。
// 使い回すと、1人のユーザーの操作が全員に伝播しうる。
func TestExercises_ReturnsFreshInstances(t *testing.T) {
	a, _ := seed.Exercises()
	b, _ := seed.Exercises()
	if len(a) != len(b) {
		t.Fatalf("件数が一致しない: %d, %d", len(a), len(b))
	}
	for i := range a {
		if a[i] == b[i] {
			t.Fatalf("同じインスタンスを共有している: %s", a[i].ID())
		}
	}
}

// 週目標のどの区分も、メイン以外の手段で埋められること。
// メイン種目でしか刺激できない区分があると、バリエーション日に
// 目標へ届かせる方法が無くなる。
func TestSeed_EveryTargetRegionHasANonMainExercise(t *testing.T) {
	all, _ := seed.Exercises()
	target, err := seed.DefaultWeeklyTarget()
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	covered := map[training.MuscleRegion]bool{}
	for _, e := range all {
		if e.Kind() == training.KindMain {
			continue
		}
		for _, r := range e.Stimulus().Regions() {
			covered[r] = true
		}
	}
	for _, r := range training.AllMuscleRegions() {
		if target.Sets(r) > 0 && !covered[r] {
			t.Errorf("%s はメイン種目でしか刺激できない", r)
		}
	}
}
