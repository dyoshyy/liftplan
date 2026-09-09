package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/program"
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

// BIG3 がシードに入っていること。既定の軸がここから引かれる。
func TestExercises_ContainsBigThree(t *testing.T) {
	all, _ := seed.Exercises()
	found := map[exercise.ExerciseID]bool{}
	for _, e := range all {
		found[e.ID()] = true
	}
	for _, want := range []exercise.ExerciseID{"squat", "bench", "deadlift"} {
		if !found[want] {
			t.Errorf("%s がシードに無い", want)
		}
	}
}

func TestExercises_IDsAreUnique(t *testing.T) {
	all, _ := seed.Exercises()
	seen := map[exercise.ExerciseID]bool{}
	for _, e := range all {
		if seen[e.ID()] {
			t.Errorf("種目IDが重複している: %s", e.ID())
		}
		seen[e.ID()] = true
	}
}

// 既定の宣言（BIG3）を外しても、残りの種目で全区分を狙えること。
//
// 元は「Kind == ACCESSORY の種目が全区分をカバーする」を検査していた。
// メイン/補助はマスタの属性ではなくなったので（D-117）、既定の宣言を
// 除いた集合で見る。宣言を入れ替える人がいても、区分が浮かないこと。
// lookupIDs は種目IDの集合を引きやすい形にする。
func lookupIDs(ids []exercise.ExerciseID) map[exercise.ExerciseID]bool {
	out := make(map[exercise.ExerciseID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func TestExercises_NonDeclaredCoverEveryRegion(t *testing.T) {
	all, _ := seed.Exercises()
	declared := lookupIDs(seed.DefaultDeclared())
	covered := map[training.MuscleRegion]bool{}
	for _, e := range all {
		if declared[e.ID()] {
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
	for f := 1; f <= 4; f++ {
		freq, err := program.NewFrequency(f)
		if err != nil {
			t.Fatalf("頻度が不正: %v", err)
		}
		target, err := seed.DefaultWeeklyTarget(freq)
		if err != nil {
			t.Fatalf("週目標が不正: %v", err)
		}
		for _, r := range training.AllMuscleRegions() {
			if target.Sets(r) <= 0 {
				t.Errorf("週%d回: 筋区分 %s の目標が設定されていない", f, r)
			}
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
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	covered := map[training.MuscleRegion]bool{}
	declared := lookupIDs(seed.DefaultDeclared())
	for _, e := range all {
		if declared[e.ID()] {
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

// 各筋区分に、それを主働筋とする補助種目が最低1つあること。
//
// 副次刺激（0.3〜0.5）だけで週目標を埋めることになる区分があると、
// その区分は他の種目のついでにしか動かず、狙って埋められない。
func TestExercises_EveryRegionHasAPrimaryAccessory(t *testing.T) {
	all, _ := seed.Exercises()

	primary := map[training.MuscleRegion]bool{}
	declared := lookupIDs(seed.DefaultDeclared())
	for _, e := range all {
		if declared[e.ID()] {
			continue
		}
		for _, r := range e.Stimulus().Regions() {
			if c, ok := e.Stimulus().Contribution(r); ok && c.Float() >= 1.0 {
				primary[r] = true
			}
		}
	}
	for _, r := range training.AllMuscleRegions() {
		if !primary[r] {
			t.Errorf("%s を主働筋とする補助種目が無い", r)
		}
	}
}

// メインリフトの主働筋が取り違えられていないこと。
func TestExercises_MainLiftsHaveTheRightPrimaryMover(t *testing.T) {
	want := map[exercise.ExerciseID]training.MuscleRegion{
		"squat":    training.Quad,
		"bench":    training.ChestMid,
		"deadlift": training.Hamstring,
	}

	all, _ := seed.Exercises()
	for _, e := range all {
		region, ok := want[e.ID()]
		if !ok {
			continue
		}
		c, has := e.Stimulus().Contribution(region)
		if !has || c.Float() < 1.0 {
			t.Errorf("%s の主働筋が %s になっていない", e.ID(), region)
		}
	}
}

// 増加単位が実在の器具で刻める範囲にあること。
//
// 小さすぎると推定1RMのわずかな揺れがそのまま重量の変化として出て、
// 大きすぎると丸めた結果が実力から離れる。
func TestExercises_IncrementsArePracticable(t *testing.T) {
	all, _ := seed.Exercises()
	for _, e := range all {
		if inc := e.Increment().Kg(); inc < 0.5 || inc > 5.0 {
			t.Errorf("%s の増加単位が実用的でない: %vkg", e.ID(), inc)
		}
	}
}

// 自重が乗る種目に係数が入っていること。
//
// 係数が0だと実効負荷への変換が素通りし、自重でこなした記録は0kgのまま
// 推定に入る。ドメイン側に仕組みがあっても、ここが埋まっていなければ
// 誰にも効かない。
func TestExercises_BodyweightExercisesHaveAFactor(t *testing.T) {
	want := map[exercise.ExerciseID]float64{
		"pull_up":        0.95,
		"dip":            0.93,
		"back_extension": 0.55,
	}

	all, _ := seed.Exercises()
	for _, e := range all {
		f := e.BodyweightFactor().Float()
		if w, ok := want[e.ID()]; ok {
			if f != w {
				t.Errorf("%s の自重係数が %v。%v のはず", e.ID(), f, w)
			}
			continue
		}
		if f != 0 {
			t.Errorf("%s に自重係数 %v が入っている。自重は乗らないはず", e.ID(), f)
		}
	}
}
