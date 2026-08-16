package training_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestMuscleRegion_Resolution(t *testing.T) {
	all := training.AllMuscleRegions()
	if len(all) < 20 || len(all) > 25 {
		t.Errorf("筋区分は20〜25個であるべきだが %d 個だった", len(all))
	}
}

func TestMuscleRegion_NoDuplicates(t *testing.T) {
	seen := map[training.MuscleRegion]bool{}
	for _, r := range training.AllMuscleRegions() {
		if seen[r] {
			t.Errorf("筋区分が重複している: %s", r)
		}
		seen[r] = true
	}
}

func TestMuscleRegion_AllAreValid(t *testing.T) {
	for _, r := range training.AllMuscleRegions() {
		if !r.Valid() {
			t.Errorf("一覧に含まれる筋区分が Valid でない: %s", r)
		}
	}
}

func TestMuscleRegion_ListIsSorted(t *testing.T) {
	// この一覧を走査するコードが常に同じ結果を返すよう、順序を一意に固定する。
	all := training.AllMuscleRegions()
	if !sort.SliceIsSorted(all, func(i, j int) bool { return all[i] < all[j] }) {
		t.Errorf("筋区分の一覧がソートされていない: %v", all)
	}
}

func TestMuscleRegion_ListIsStableAcrossCalls(t *testing.T) {
	first := training.AllMuscleRegions()
	for range 50 {
		got := training.AllMuscleRegions()
		if len(got) != len(first) {
			t.Fatalf("呼び出しごとに件数が変わる: %d vs %d", len(first), len(got))
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("呼び出しごとに順序が変わる: %v vs %v", first, got)
			}
		}
	}
}

func TestMuscleRegion_ListIsDefensivelyCopied(t *testing.T) {
	// 返り値を書き換えても内部状態が壊れないこと。
	got := training.AllMuscleRegions()
	if len(got) == 0 {
		t.Fatal("筋区分が空である")
	}
	got[0] = training.MuscleRegion("TAMPERED")

	for _, r := range training.AllMuscleRegions() {
		if r == training.MuscleRegion("TAMPERED") {
			t.Fatal("返り値の書き換えが内部状態に波及している")
		}
	}
	if training.MuscleRegion("TAMPERED").Valid() {
		t.Fatal("書き換えた値が Valid になっている")
	}
}

func TestMuscleRegion_ChestIsSplitIntoThree(t *testing.T) {
	// 「ベンチが埋めない大胸筋上部を補助で埋める」という判断を表現するために必要。
	for _, r := range []training.MuscleRegion{
		training.ChestUpper, training.ChestMid, training.ChestLower,
	} {
		if !r.Valid() {
			t.Errorf("%s が定義されていない", r)
		}
	}
}

func TestMuscleRegion_CoversEveryBodyPart(t *testing.T) {
	// 全身を同じ解像度で持つという方針の最低限の担保。
	// 主要な部位がそれぞれ1つ以上の筋区分でカバーされていること。
	prefixes := map[string]string{
		"胸":   "CHEST_",
		"三頭":  "TRICEPS_",
		"僧帽筋": "TRAP_",
	}
	all := training.AllMuscleRegions()
	for name, prefix := range prefixes {
		count := 0
		for _, r := range all {
			if strings.HasPrefix(string(r), prefix) {
				count++
			}
		}
		if count < 2 {
			t.Errorf("%s が %d 区分しかない。細分化されていない", name, count)
		}
	}

	required := []training.MuscleRegion{
		training.Lat, training.Erector,
		training.FrontDelt, training.SideDelt, training.RearDelt,
		training.Biceps, training.Forearm,
		training.Quad, training.Hamstring, training.Glute, training.Adductor, training.Calf,
		training.Abs, training.Oblique,
	}
	for _, r := range required {
		if !r.Valid() {
			t.Errorf("必須の筋区分が定義されていない: %s", r)
		}
	}
}

func TestMuscleRegion_InvalidValues(t *testing.T) {
	for _, r := range []training.MuscleRegion{
		"", "NOT_A_REGION", "chest_mid", "CHEST", "CHEST_MID ", " CHEST_MID",
	} {
		if r.Valid() {
			t.Errorf("不正な値が Valid になっている: %q", r)
		}
	}
}

func TestExerciseKind_Valid(t *testing.T) {
	all := training.AllExerciseKinds()
	if len(all) != 3 {
		t.Errorf("種別は3つであるべき: %d", len(all))
	}
	for _, k := range all {
		if !k.Valid() {
			t.Errorf("%s が Valid でない", k)
		}
	}
	for _, k := range []training.ExerciseKind{"", "OTHER", "main", "MAIN "} {
		if k.Valid() {
			t.Errorf("不正な種別が Valid になっている: %q", k)
		}
	}
}

func TestExerciseKind_ListCoversAllConstants(t *testing.T) {
	seen := map[training.ExerciseKind]bool{}
	for _, k := range training.AllExerciseKinds() {
		seen[k] = true
	}
	for _, k := range []training.ExerciseKind{
		training.KindMain, training.KindVariation, training.KindAccessory,
	} {
		if !seen[k] {
			t.Errorf("一覧に %s が含まれていない", k)
		}
	}
}

func TestMainLift_Valid(t *testing.T) {
	all := training.AllMainLifts()
	if len(all) != 3 {
		t.Errorf("メインリフトは3つであるべき: %d", len(all))
	}
	for _, l := range all {
		if !l.Valid() {
			t.Errorf("%s が Valid でない", l)
		}
	}
	for _, l := range []training.MainLift{"", "PRESS", "squat", "SQUAT "} {
		if l.Valid() {
			t.Errorf("不正なリフトが Valid になっている: %q", l)
		}
	}
}

func TestMainLift_ListCoversAllConstants(t *testing.T) {
	seen := map[training.MainLift]bool{}
	for _, l := range training.AllMainLifts() {
		seen[l] = true
	}
	for _, l := range []training.MainLift{
		training.LiftSquat, training.LiftBench, training.LiftDeadlift,
	} {
		if !seen[l] {
			t.Errorf("一覧に %s が含まれていない", l)
		}
	}
}

func TestTaxonomy_ValuesAreScreamingSnakeCase(t *testing.T) {
	// 永続化と JSON の表現をこの文字列に依存させるため、表記を固定しておく。
	check := func(t *testing.T, kind, value string) {
		t.Helper()
		if value == "" {
			t.Errorf("%s に空の値がある", kind)
			return
		}
		if value != strings.ToUpper(value) {
			t.Errorf("%s の %q が大文字でない", kind, value)
		}
		if strings.TrimSpace(value) != value {
			t.Errorf("%s の %q に空白が含まれる", kind, value)
		}
	}
	for _, r := range training.AllMuscleRegions() {
		check(t, "MuscleRegion", string(r))
	}
	for _, k := range training.AllExerciseKinds() {
		check(t, "ExerciseKind", string(k))
	}
	for _, l := range training.AllMainLifts() {
		check(t, "MainLift", string(l))
	}
}
