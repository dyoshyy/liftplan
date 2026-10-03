package program_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// レップ数は 1〜15。重い番と軽い番のどちらにも同じ範囲が掛かる。
//
// 15 は限界までの総レップ（レップ + RIR1 = 16）が推定に使える上限
// （maxRepsToFailure = 20）の内側に収まる値。N ≤ M は強制しない。
func TestNewRepTargets(t *testing.T) {
	cases := []struct {
		name         string
		heavy, light int
		ok           bool
	}{
		{"既定", 3, 6, true},
		{"下限", 1, 1, true},
		{"上限", 15, 15, true},
		{"重い番のほうが多くてもよい", 8, 5, true},
		{"重い番が0", 0, 6, false},
		{"軽い番が0", 3, 0, false},
		{"重い番が16", 16, 6, false},
		{"軽い番が16", 3, 16, false},
		{"負", -1, 6, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := program.NewRepTargets(c.heavy, c.light)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v。ok=%v のはず", err, c.ok)
			}
			if c.ok && (got.Heavy() != c.heavy || got.Light() != c.light) {
				t.Errorf("(%d, %d) が (%d, %d) になった", c.heavy, c.light, got.Heavy(), got.Light())
			}
		})
	}
}

// 既定は今の処方（重い番3・軽い番6）と同じ。ここが動くと全員の重量が動く。
func TestDefaultRepTargets(t *testing.T) {
	d := program.DefaultRepTargets()
	if d.Heavy() != 3 || d.Light() != 6 {
		t.Errorf("既定が (%d, %d)。(3, 6) のはず", d.Heavy(), d.Light())
	}
	if d.IsZero() {
		t.Error("既定がゼロ値と区別できない")
	}
}
