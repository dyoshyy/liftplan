package exercise_test

import (
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

func validCustom() exercise.CustomExerciseParams {
	return exercise.CustomExerciseParams{
		ID:          "u-0123456789abcdef",
		Name:        "アイソラテラル・ロー",
		Primary:     []training.MuscleRegion{training.TrapMid},
		Secondary:   []training.MuscleRegion{training.Lat, training.Biceps},
		IncrementKg: 2.5,
	}
}

// 主は1.0、少しは0.5になること。本人に数値を入れさせない代わりに、
// 対応はここで固定する。
func TestNewCustomExercise_MapsPrimaryAndSecondaryToFixedContributions(t *testing.T) {
	e, err := exercise.NewCustomExercise(validCustom())
	if err != nil {
		t.Fatal(err)
	}
	want := map[training.MuscleRegion]float64{
		training.TrapMid: 1.0, training.Lat: 0.5, training.Biceps: 0.5,
	}
	for r, w := range want {
		c, ok := e.Stimulus().Contribution(r)
		if !ok || c.Float() != w {
			t.Errorf("%s の寄与が %v（期待 %v）", r, c.Float(), w)
		}
	}
	if got := len(e.Stimulus().Regions()); got != len(want) {
		t.Errorf("寄与する区分が %d 個（期待 %d）", got, len(want))
	}
	if !e.IsCustom() || e.IsDeleted() {
		t.Errorf("custom=%v deleted=%v（期待 true, false）", e.IsCustom(), e.IsDeleted())
	}
	if _, ok := e.DerivedFrom(); ok || e.BodyweightFactor().Float() != 0 {
		t.Error("自分の種目に派生元か自重係数が入っている")
	}
}

func TestNewCustomExercise_Rejects(t *testing.T) {
	nine := training.AllMuscleRegions()[:9]
	cases := []struct {
		name   string
		modify func(*exercise.CustomExerciseParams)
	}{
		{"主が無い", func(p *exercise.CustomExerciseParams) { p.Primary = nil }},
		{"主と少しが重なる", func(p *exercise.CustomExerciseParams) {
			p.Secondary = []training.MuscleRegion{training.TrapMid}
		}},
		{"主の中で重なる", func(p *exercise.CustomExerciseParams) {
			p.Primary = []training.MuscleRegion{training.TrapMid, training.TrapMid}
		}},
		{"9区分", func(p *exercise.CustomExerciseParams) {
			p.Primary, p.Secondary = nine[:1], nine[1:]
		}},
		{"知らない区分", func(p *exercise.CustomExerciseParams) {
			p.Primary = []training.MuscleRegion{"NECK"}
		}},
		{"名前が空", func(p *exercise.CustomExerciseParams) { p.Name = "  " }},
		{"名前が41文字", func(p *exercise.CustomExerciseParams) { p.Name = strings.Repeat("あ", 41) }},
		{"刻みが0", func(p *exercise.CustomExerciseParams) { p.IncrementKg = 0 }},
		{"IDの接頭辞が違う", func(p *exercise.CustomExerciseParams) { p.ID = "bench" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validCustom()
			c.modify(&p)
			if _, err := exercise.NewCustomExercise(p); err == nil {
				t.Error("通ってしまった")
			}
		})
	}
}

// 40文字ちょうどは通ること（上の41文字と対で境界を固定する）。
func TestNewCustomExercise_AcceptsFortyRuneName(t *testing.T) {
	p := validCustom()
	p.Name = strings.Repeat("あ", 40)
	if _, err := exercise.NewCustomExercise(p); err != nil {
		t.Fatal(err)
	}
}

// Delete は元を変えず、消した状態の新しい値を返すこと。
func TestExercise_DeleteReturnsANewValue(t *testing.T) {
	e, _ := exercise.NewCustomExercise(validCustom())
	d := e.Delete()
	if e.IsDeleted() {
		t.Error("元が消えた状態になった")
	}
	if !d.IsDeleted() || !d.IsCustom() || d.ID() != e.ID() || d.Name() != e.Name() {
		t.Errorf("消した値が崩れている: %+v", d)
	}
}

// 保存用に主と少しを取り出せること。読み戻しで同じ種目になる。
func TestExercise_PrimaryAndSecondaryRoundTrip(t *testing.T) {
	p := validCustom()
	e, _ := exercise.NewCustomExercise(p)
	back, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: string(e.ID()), Name: e.Name(),
		Primary: e.PrimaryRegions(), Secondary: e.SecondaryRegions(),
		IncrementKg: e.Increment().Kg(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range e.Stimulus().Regions() {
		a, _ := e.Stimulus().Contribution(r)
		b, _ := back.Stimulus().Contribution(r)
		if a != b {
			t.Errorf("%s: %v と %v", r, a.Float(), b.Float())
		}
	}
}

func TestNewRandomCustomExerciseID_HasThePrefixAndIsValid(t *testing.T) {
	id, err := exercise.NewRandomCustomExerciseID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(id), exercise.CustomExerciseIDPrefix) || len(id) != 18 {
		t.Errorf("ID の形が違う: %q", id)
	}
	other, _ := exercise.NewRandomCustomExerciseID()
	if other == id {
		t.Error("2回採番して同じ ID になった")
	}
}
