package exercise_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

func benchParams() exercise.ExerciseParams {
	return exercise.ExerciseParams{
		ID:   "bench",
		Name: "ベンチプレス",
		Stimulus: map[training.MuscleRegion]float64{
			training.ChestMid:       1.0,
			training.TricepsLateral: 0.5,
		},
		IncrementKg: 2.5,
	}
}

func mustExercise(t *testing.T, p exercise.ExerciseParams) *exercise.Exercise {
	t.Helper()
	e, err := exercise.NewExercise(p)
	if err != nil {
		t.Fatalf("NewExercise(%s): %v", p.ID, err)
	}
	return e
}

func TestNewExercise_Main(t *testing.T) {
	e := mustExercise(t, benchParams())

	if e.ID() != exercise.ExerciseID("bench") {
		t.Errorf("ID が誤り: %v", e.ID())
	}
	if e.Name() != "ベンチプレス" {
		t.Errorf("名前が誤り: %v", e.Name())
	}
	if e.Increment().Kg() != 2.5 {
		t.Errorf("増加単位が誤り: %v", e.Increment().Kg())
	}
}

func TestNewExercise_RejectsInvalidFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*exercise.ExerciseParams)
	}{
		{"IDが空", func(p *exercise.ExerciseParams) { p.ID = "" }},
		{"IDが空白のみ", func(p *exercise.ExerciseParams) { p.ID = "   " }},
		{"IDの前後に空白", func(p *exercise.ExerciseParams) { p.ID = " bench " }},
		{"名前が空", func(p *exercise.ExerciseParams) { p.Name = "" }},
		{"名前が空白のみ", func(p *exercise.ExerciseParams) { p.Name = "  " }},
		{"刺激が空", func(p *exercise.ExerciseParams) { p.Stimulus = nil }},
		{"増加単位が0", func(p *exercise.ExerciseParams) { p.IncrementKg = 0 }},
		{"増加単位が負", func(p *exercise.ExerciseParams) { p.IncrementKg = -2.5 }},
		{
			"未知の筋区分",
			func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{"NOPE": 1.0}
			},
		},
		{
			"寄与度が範囲外",
			func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 1.5}
			},
		},
		{
			"寄与度が0",
			func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 0}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := benchParams()
			c.mutate(&p)
			if got, err := exercise.NewExercise(p); err == nil {
				t.Errorf("不正な種目が通ってしまう: %+v", got)
			}
		})
	}
}

func TestNewExercise_RejectsTooManyRegions(t *testing.T) {
	// 全身に寄与する種目は寄与度の設定ミス。残差計算が意味を失う。
	p := benchParams()
	p.Stimulus = map[training.MuscleRegion]float64{}
	for _, r := range training.AllMuscleRegions() {
		p.Stimulus[r] = 0.5
	}
	if _, err := exercise.NewExercise(p); err == nil {
		t.Error("全筋区分に寄与する種目が通ってしまう")
	}
}

func TestNewExercise_FailureReturnsNil(t *testing.T) {
	p := benchParams()
	p.ID = ""
	got, err := exercise.NewExercise(p)
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if got != nil {
		t.Errorf("失敗時に nil でない値が返る: %+v", got)
	}
}

func TestNewExercise_TrimsName(t *testing.T) {
	p := benchParams()
	p.Name = "  ベンチプレス  "
	if got := mustExercise(t, p).Name(); got != "ベンチプレス" {
		t.Errorf("名前が整形されていない: %q", got)
	}
}

func TestNewExercise_ErrorMessagesIdentifyTheExercise(t *testing.T) {
	// 36種目のシードを読み込むとき、どの種目が不正なのか分からないと直せない。
	p := benchParams()
	p.IncrementKg = 0
	_, err := exercise.NewExercise(p)
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !strings.Contains(err.Error(), "bench") {
		t.Errorf("エラーメッセージに種目IDが含まれない: %v", err)
	}
}

func TestStimulusProfile_RegionsAreSorted(t *testing.T) {
	p := benchParams()
	p.Stimulus = map[training.MuscleRegion]float64{
		training.TricepsLateral: 0.5,
		training.ChestMid:       1.0,
		training.FrontDelt:      0.5,
		training.ChestUpper:     0.3,
	}
	regions := mustExercise(t, p).Stimulus().Regions()

	if !sort.SliceIsSorted(regions, func(i, j int) bool { return regions[i] < regions[j] }) {
		t.Errorf("Regions がソートされていない: %v", regions)
	}
	if len(regions) != 4 {
		t.Errorf("件数が誤り: %d", len(regions))
	}
}

func TestStimulusProfile_RegionsAreStable(t *testing.T) {
	// マップの反復順に依存すると、同じ入力から違うセッションが生成される。
	e := mustExercise(t, benchParams())
	first := e.Stimulus().Regions()

	for range 100 {
		got := e.Stimulus().Regions()
		if len(got) != len(first) {
			t.Fatalf("件数が変わる: %d vs %d", len(first), len(got))
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("順序が変わる: %v vs %v", first, got)
			}
		}
	}
}

func TestStimulusProfile_RegionsIsDefensivelyCopied(t *testing.T) {
	e := mustExercise(t, benchParams())
	got := e.Stimulus().Regions()
	if len(got) == 0 {
		t.Fatal("筋区分が空")
	}
	got[0] = training.MuscleRegion("TAMPERED")

	for _, r := range e.Stimulus().Regions() {
		if r == training.MuscleRegion("TAMPERED") {
			t.Fatal("返り値の書き換えが内部状態に波及している")
		}
	}
}

func TestStimulusProfile_IsImmutableAgainstInputMutation(t *testing.T) {
	// 生成に使ったマップを呼び出し側が書き換えても影響しないこと。
	p := benchParams()
	input := map[training.MuscleRegion]float64{training.ChestMid: 1.0}
	p.Stimulus = input

	e := mustExercise(t, p)
	input[training.ChestMid] = 0.1
	input[training.Calf] = 1.0

	c, ok := e.Stimulus().Contribution(training.ChestMid)
	if !ok || c.Float() != 1.0 {
		t.Errorf("入力マップの書き換えが波及している: %v %v", c.Float(), ok)
	}
	if _, ok := e.Stimulus().Contribution(training.Calf); ok {
		t.Error("入力マップへの追加が波及している")
	}
}

func TestStimulusProfile_Contribution(t *testing.T) {
	e := mustExercise(t, benchParams())

	c, ok := e.Stimulus().Contribution(training.ChestMid)
	if !ok || c.Float() != 1.0 {
		t.Errorf("寄与度が誤り: %v %v", c.Float(), ok)
	}
	if _, ok := e.Stimulus().Contribution(training.Calf); ok {
		t.Error("寄与しない区分が取れてしまう")
	}
}

func TestStimulusProfile_ZeroValueIsEmpty(t *testing.T) {
	var p exercise.StimulusProfile
	if !p.IsEmpty() {
		t.Error("ゼロ値が空でない")
	}
	if len(p.Regions()) != 0 {
		t.Error("ゼロ値から筋区分が出てくる")
	}
	if _, ok := p.Contribution(training.ChestMid); ok {
		t.Error("ゼロ値から寄与度が取れる")
	}
}

func TestExercise_SameIdentity(t *testing.T) {
	a := mustExercise(t, benchParams())

	p := benchParams()
	p.Name = "別名だが同じID"
	p.IncrementKg = 5.0
	b := mustExercise(t, p)

	if !a.SameIdentity(b) {
		t.Error("同じIDのエンティティが別物と判定された")
	}

	p2 := benchParams()
	p2.ID = "squat"
	if a.SameIdentity(mustExercise(t, p2)) {
		t.Error("違うIDのエンティティが同一と判定された")
	}

	if a.SameIdentity(nil) {
		t.Error("nil と同一と判定された")
	}
	var nilExercise *exercise.Exercise
	if nilExercise.SameIdentity(a) {
		t.Error("nil レシーバが同一と判定された")
	}
}

func TestNewExercise_StimulusRegionLimitBoundary(t *testing.T) {
	const max = 8
	all := training.AllMuscleRegions()
	if len(all) < max+1 {
		t.Fatalf("検査に必要な筋区分が足りない: %d", len(all))
	}

	build := func(n int) exercise.ExerciseParams {
		p := benchParams()
		p.Stimulus = map[training.MuscleRegion]float64{}
		for _, r := range all[:n] {
			p.Stimulus[r] = 0.5
		}
		return p
	}

	if _, err := exercise.NewExercise(build(max)); err != nil {
		t.Errorf("上限ちょうど %d 区分が弾かれた: %v", max, err)
	}
	if got, err := exercise.NewExercise(build(max + 1)); err == nil {
		t.Errorf("上限を超える %d 区分が通ってしまう: %+v", max+1, got)
	} else if !strings.Contains(err.Error(), "筋区分") {
		t.Errorf("区分数以外の理由でエラーになっている: %v", err)
	}
}

// IDが不正なとき、種目名で特定できること。
// 36種目のシードで1つのIDをタイプミスで消したとき、
// 「種目IDが空である」だけではどれか分からない。
func TestNewExercise_IdentifiesExerciseWhenIDIsInvalid(t *testing.T) {
	p := benchParams()
	p.ID = ""
	_, err := exercise.NewExercise(p)
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !strings.Contains(err.Error(), "ベンチプレス") {
		t.Errorf("エラーメッセージに種目名が含まれない: %v", err)
	}
}

// 各検証経路のエラーが種目を特定できること。
func TestNewExercise_AllErrorsIdentifyTheExercise(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*exercise.ExerciseParams)
	}{
		{"増加単位", func(p *exercise.ExerciseParams) { p.IncrementKg = 0 }},
		{"刺激が空", func(p *exercise.ExerciseParams) { p.Stimulus = nil }},
		{
			"未知の筋区分",
			func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{"NOPE": 1.0}
			},
		},
		{
			"寄与度",
			func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 5}
			},
		},
		{"名前が空", func(p *exercise.ExerciseParams) { p.Name = "" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := benchParams()
			c.mutate(&p)
			_, err := exercise.NewExercise(p)
			if err == nil {
				t.Fatal("エラーにならない")
			}
			if !strings.Contains(err.Error(), "bench") {
				t.Errorf("エラーメッセージに種目IDが含まれない: %v", err)
			}
		})
	}
}
