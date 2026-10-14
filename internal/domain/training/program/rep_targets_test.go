package program_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// レップ数は 1〜15。
//
// 15 は限界までの総レップ（レップ + RIR1 = 16）が推定に使える上限
// （maxRepsToFailure = 20）の内側に収まる値。
func TestNewRepTargets(t *testing.T) {
	cases := []struct {
		name  string
		heavy int
		ok    bool
	}{
		{"既定", 3, true},
		{"下限", 1, true},
		{"上限", 15, true},
		{"0", 0, false},
		{"16", 16, false},
		{"負", -1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := program.NewRepTargets(c.heavy)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v。ok=%v のはず", err, c.ok)
			}
			if c.ok && got.Heavy() != c.heavy {
				t.Errorf("%d が %d になった", c.heavy, got.Heavy())
			}
		})
	}
}

// 既定は今の処方（重い番3）と同じ。ここが動くと全員の重量が動く。
func TestDefaultRepTargets(t *testing.T) {
	d := program.DefaultRepTargets()
	if d.Heavy() != 3 {
		t.Errorf("既定が %d。3 のはず", d.Heavy())
	}
	if d.IsZero() {
		t.Error("既定がゼロ値と区別できない")
	}
}
