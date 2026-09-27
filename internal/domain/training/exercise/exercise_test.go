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

// 不正な入力は弾き、nil を返し、エラーで種目を特定できること。
//
// シードを読み込むとき、どの種目が不正なのか分からないと直せない。
// 通常はIDで特定するが、そのIDが壊れているケースでは名前で補う。
//
// 以前は「弾くこと」「nil を返すこと」「エラーが種目を特定できること」を
// 別々のテストに分けていた。後者2つは前者のケースを部分的に再掲していただけで、
// 片方に足してもう片方を忘れる形になっていた。1つの表に畳み、wantHint を
// 列にすることで、全ケースで手がかりを検査する。
func TestNewExercise_RejectsInvalidParams(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*exercise.ExerciseParams)
		// エラーに含まれるべき手がかり。IDが壊れているケースでは名前で特定する。
		wantHint string
	}{
		{"IDが空", func(p *exercise.ExerciseParams) { p.ID = "" }, "ベンチプレス"},
		{"IDが空白のみ", func(p *exercise.ExerciseParams) { p.ID = "   " }, "ベンチプレス"},
		{"IDの前後に空白", func(p *exercise.ExerciseParams) { p.ID = " bench " }, "ベンチプレス"},
		{"名前が空", func(p *exercise.ExerciseParams) { p.Name = "" }, "bench"},
		{"名前が空白のみ", func(p *exercise.ExerciseParams) { p.Name = "  " }, "bench"},
		{"増加単位が0", func(p *exercise.ExerciseParams) { p.IncrementKg = 0 }, "bench"},
		{"増加単位が負", func(p *exercise.ExerciseParams) { p.IncrementKg = -2.5 }, "bench"},
		{"刺激が空", func(p *exercise.ExerciseParams) { p.Stimulus = nil }, "bench"},
		{
			name: "未知の筋区分",
			mutate: func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{"NOPE": 1.0}
			},
			wantHint: "bench",
		},
		{
			name: "寄与度が範囲外",
			mutate: func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 1.5}
			},
			wantHint: "bench",
		},
		{
			// 0は「刺激しない」であって「寄与度0で刺激する」ではない。
			// 通すと、狙っていない区分が残差の計算に現れる。
			name: "寄与度が0",
			mutate: func(p *exercise.ExerciseParams) {
				p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 0}
			},
			wantHint: "bench",
		},
		{
			name: "自分自身を親にしている", mutate: func(p *exercise.ExerciseParams) { p.DerivedFrom = "bench" }, wantHint: "bench",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := benchParams()
			c.mutate(&p)

			got, err := exercise.NewExercise(p)
			if err == nil {
				t.Fatalf("不正な種目が通ってしまう: %+v", got)
			}
			if got != nil {
				t.Errorf("失敗時に nil でない値が返る: %+v", got)
			}
			if !strings.Contains(err.Error(), c.wantHint) {
				t.Errorf("エラーが種目を特定できない（%q が無い）: %v", c.wantHint, err)
			}
		})
	}
}

// 通常の NewExercise（シードが使う経路）は生成直後は deleted にならないこと。
func TestNewExercise_IsNotDeleted(t *testing.T) {
	e := mustExercise(t, benchParams())
	if e.IsDeleted() {
		t.Error("生成直後の種目が deleted=true になっている")
	}
}

// NewRandomExerciseID は "u-" で始まる ID を採番し、2回呼べば別の値になること。
//
// プリセット由来かどうかで扱いを変えない設計では、利用者が足す種目も
// NewExercise がそのまま受け取る。採番の性質（接頭辞・一意性）はここで
// 固定しておく。
func TestNewRandomExerciseID_HasThePrefixAndIsValid(t *testing.T) {
	id, err := exercise.NewRandomExerciseID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(id), exercise.CustomExerciseIDPrefix) || len(id) != 18 {
		t.Errorf("ID の形が違う: %q", id)
	}
	other, err := exercise.NewRandomExerciseID()
	if err != nil {
		t.Fatal(err)
	}
	if other == id {
		t.Error("2回採番して同じ ID になった")
	}
}

func TestNewExercise_TrimsName(t *testing.T) {
	p := benchParams()
	p.Name = "  ベンチプレス  "
	if got := mustExercise(t, p).Name(); got != "ベンチプレス" {
		t.Errorf("名前が整形されていない: %q", got)
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

func TestNewExercise_RequiresARegionAtFullContribution(t *testing.T) {
	_, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "x", Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 0.9},
	})
	if err == nil {
		t.Error("寄与1.0の区分が無い種目が通った")
	}
}

func TestNewExercise_NameUpToFortyRunes(t *testing.T) {
	p := exercise.ExerciseParams{ID: "x", IncrementKg: 2.5, Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}}
	p.Name = strings.Repeat("あ", 40)
	if _, err := exercise.NewExercise(p); err != nil {
		t.Fatal(err)
	}
	p.Name = strings.Repeat("あ", 41)
	if _, err := exercise.NewExercise(p); err == nil {
		t.Error("41文字が通った")
	}
}

// 名前だけ直しても、プリセットの細かい寄与（0.7・0.4）は変わらないこと。
// 画面は直さない項目も今の値のまま送るので、Edit は渡された値をそのまま使う。
func TestExercise_EditKeepsBodyweightAndDerivedFrom(t *testing.T) {
	squat, _ := exercise.NewExercise(exercise.ExerciseParams{
		ID: "pull_up_like", Name: "チンニング", IncrementKg: 2.5, BodyweightFactor: 0.95, DerivedFrom: "parent",
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1, training.Biceps: 0.5},
	})
	got, err := squat.Edit(exercise.ExerciseEdit{
		Name: "  懸垂  ", IncrementKg: 1.25,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1, training.Biceps: 0.7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != "pull_up_like" || got.Name() != "懸垂" || got.Increment().Kg() != 1.25 {
		t.Errorf("直した値が違う: %s %s %v", got.ID(), got.Name(), got.Increment().Kg())
	}
	if c, _ := got.Stimulus().Contribution(training.Biceps); c.Float() != 0.7 {
		t.Errorf("寄与が %v", c.Float())
	}
	if got.BodyweightFactor().Float() != 0.95 {
		t.Error("自重係数が引き継がれていない")
	}
	if from, ok := got.DerivedFrom(); !ok || from != "parent" {
		t.Error("派生元が引き継がれていない")
	}
	if c, _ := squat.Stimulus().Contribution(training.Biceps); c.Float() != 0.5 {
		t.Error("元の値が書き換わった")
	}
}

func TestExercise_EditValidatesLikeNewExercise(t *testing.T) {
	e, _ := exercise.NewExercise(exercise.ExerciseParams{ID: "x", Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}})
	if _, err := e.Edit(exercise.ExerciseEdit{Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 0.5}}); err == nil {
		t.Error("寄与1.0の区分が無くなる編集が通った")
	}
}

func TestExercise_EditKeepsDeleted(t *testing.T) {
	e, _ := exercise.NewExercise(exercise.ExerciseParams{ID: "x", Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}})
	got, _ := e.Delete().Edit(exercise.ExerciseEdit{Name: "y", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}})
	if !got.IsDeleted() {
		t.Error("消した印が落ちた")
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
		for i, r := range all[:n] {
			if i == 0 {
				p.Stimulus[r] = 1.0
				continue
			}
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
