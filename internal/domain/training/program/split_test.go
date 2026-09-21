package program_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

func mustSplit(t *testing.T, name string, regions ...training.MuscleRegion) program.Split {
	t.Helper()
	s, err := program.NewSplit(name, regions)
	if err != nil {
		t.Fatalf("NewSplit(%q): %v", name, err)
	}
	return s
}

func mustProgram(t *testing.T) *program.Program {
	t.Helper()
	p, err := program.NewProgram(mustFrequency(t, 3), simpleTarget(t),
		[]exercise.ExerciseID{"bench", "squat"}, []exercise.ExerciseID{"bench"}, "")
	if err != nil {
		t.Fatalf("NewProgram: %v", err)
	}
	return p
}

func TestNewSplit_Rejects(t *testing.T) {
	cases := []struct {
		name    string
		label   string
		regions []training.MuscleRegion
		want    error
	}{
		{"名前が空", "", []training.MuscleRegion{training.Quad}, program.ErrEmptySplitName},
		{
			"同じ区分の重複", "脚",
			[]training.MuscleRegion{training.Quad, training.Quad},
			program.ErrDuplicateRegion,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := program.NewSplit(c.label, c.regions); !errors.Is(err, c.want) {
				t.Errorf("エラーが %v。%v のはず", err, c.want)
			}
		})
	}

	if _, err := program.NewSplit("謎", []training.MuscleRegion{"NOSUCH"}); err == nil {
		t.Error("存在しない区分が通る")
	}
	if _, err := program.NewSplit("あ", nil); err != nil {
		t.Errorf("区分なしが弾かれた: %v", err)
	}
}

// 区分は昇順に正規化される。同じ設定から同じ計画が出るようにするため。
func TestNewSplit_SortsRegions(t *testing.T) {
	s := mustSplit(t, "上半身", training.Lat, training.Biceps, training.ChestMid)
	want := []training.MuscleRegion{training.Biceps, training.ChestMid, training.Lat}
	if got := s.Regions(); !slices.Equal(got, want) {
		t.Errorf("区分が %v。%v のはず", got, want)
	}
}

// 区分を1つも持たない分割は全区分を狙う。
func TestSplit_WithNoRegionsCoversEverything(t *testing.T) {
	s := mustSplit(t, "全身")
	for _, r := range training.AllMuscleRegions() {
		if !s.Includes(r) {
			t.Errorf("%s を狙わない", r)
		}
	}
}

func TestSplit_Includes(t *testing.T) {
	s := mustSplit(t, "脚", training.Quad, training.Hamstring)
	if !s.Includes(training.Quad) {
		t.Error("含むはずの区分を含まない")
	}
	if s.Includes(training.ChestMid) {
		t.Error("含まないはずの区分を含む")
	}
}

// 返した区分を書き換えても中身が変わらないこと。
func TestSplit_RegionsAreDefensivelyCopied(t *testing.T) {
	s := mustSplit(t, "脚", training.Quad, training.Hamstring)
	got := s.Regions()
	got[0] = training.ChestMid
	if s.Regions()[0] == training.ChestMid {
		t.Error("返した区分を書き換えると中身が変わる")
	}
}

func TestProgram_WithCycle(t *testing.T) {
	base := mustProgram(t)

	if got := base.Cycle(); len(got) != 0 {
		t.Errorf("既定が分割なしでない: %v", got)
	}
	if _, ok := base.SplitOn(0); ok {
		t.Error("分割なしなのに分割が返る")
	}

	upper := mustSplit(t, "上半身", training.ChestMid, training.Lat)
	lower := mustSplit(t, "下半身", training.Quad, training.Hamstring)

	// 同じ分割の繰り返しを許す。上・下・上 のような週が組めなくなる。
	next, err := base.WithCycle([]program.Split{upper, lower, upper})
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}
	if got := len(next.Cycle()); got != 3 {
		t.Fatalf("周期が %d 日。3日のはず", got)
	}

	// 元は変わらない。
	if got := len(base.Cycle()); got != 0 {
		t.Errorf("元のプログラムが変わっている: %v", got)
	}

	// 出席回数で進み、折り返す。
	for _, c := range []struct {
		sessionsBefore int
		want           string
	}{
		{0, "上半身"}, {1, "下半身"}, {2, "上半身"},
		{3, "上半身"}, {4, "下半身"}, {5, "上半身"},
		// 負の値でも落ちない。記録が壊れていても計画は出す。
		{-1, "上半身"},
	} {
		s, ok := next.SplitOn(c.sessionsBefore)
		if !ok {
			t.Fatalf("%d 回目で分割が返らない", c.sessionsBefore)
		}
		if s.Name() != c.want {
			t.Errorf("%d 回目が %q。%q のはず", c.sessionsBefore, s.Name(), c.want)
		}
	}

	// 空を渡せば分割なしに戻る。
	cleared, err := next.WithCycle(nil)
	if err != nil {
		t.Fatalf("WithCycle(nil): %v", err)
	}
	if got := len(cleared.Cycle()); got != 0 {
		t.Errorf("分割が残っている: %v", got)
	}
}

func TestProgram_WithCycleRejects(t *testing.T) {
	base := mustProgram(t)

	long := make([]program.Split, 15)
	for i := range long {
		long[i] = mustSplit(t, "脚", training.Quad)
	}
	if _, err := base.WithCycle(long); err == nil {
		t.Error("15日の周期が通る")
	}

	if _, err := base.WithCycle([]program.Split{{}}); err == nil {
		t.Error("ゼロ値の分割が通る")
	}
}

// 分割以外の With* は、分割の周期を引き継ぐこと。
//
// 引き継がないと、分割を設定した人が重点種目や頻度を変えただけで分割が
// 黙って消え、全身法に戻る（#122）。cycle は NewProgram の形が固まった
// あとに足されたフィールドで、NewProgram に委譲するだけだと落ちる。
// D-127（全置換の口が、あとから足された declared と focus を落とす）と
// 同じ形の事故。
//
// 長さではなく中身を比べる。1日ぶんだけ残る、並びが変わる、といった
// 壊れ方でも周期は狂う。
func TestProgram_WithKeepsCycle(t *testing.T) {
	cases := []struct {
		name  string
		apply func(*program.Program) (*program.Program, error)
	}{
		{
			name: "重点種目を変えても残る",
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithFocus("bench")
			},
		},
		{
			name: "伸ばしたい種目を変えても残る",
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithDeclared([]exercise.ExerciseID{"bench", "squat"})
			},
		},
		{
			// 頻度は週目標を道連れにするが、分割は道連れにしない。
			name: "頻度と週目標を変えても残る",
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithFrequency(mustFrequency(t, 4),
					mustTarget(t, map[training.MuscleRegion]float64{training.Quad: 10}))
			},
		},
		{
			name: "使う種目を変えても残る",
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithSelected([]exercise.ExerciseID{"bench", "squat", "deadlift"})
			},
		},
		{
			name: "週目標を変えても残る",
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithTarget(mustTarget(t,
					map[training.MuscleRegion]float64{training.Quad: 10}))
			},
		},
	}

	upper := mustSplit(t, "上半身", training.ChestMid, training.Lat)
	lower := mustSplit(t, "下半身", training.Quad, training.Hamstring)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base, err := mustProgram(t).WithCycle([]program.Split{upper, lower})
			if err != nil {
				t.Fatalf("WithCycle: %v", err)
			}

			next, err := c.apply(base)
			if err != nil {
				t.Fatalf("差し替えに失敗: %v", err)
			}

			got := next.Cycle()
			if len(got) != 2 {
				t.Fatalf("周期が %d 日。2日のはず", len(got))
			}
			for i, want := range []program.Split{upper, lower} {
				if got[i].Name() != want.Name() || !slices.Equal(got[i].Regions(), want.Regions()) {
					t.Errorf("%d 日目が %s %v。%s %v のはず", i,
						got[i].Name(), got[i].Regions(), want.Name(), want.Regions())
				}
			}
		})
	}
}
