package seed

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// 週目標の値を固定する。
//
// 週目標の大きさ（k）は種目カタログから切り離された定数で、実測に合わせて
// 較正してある（TestSimulation_StimulusPerSetMatchesTheTargetScale）。
// 値は意図せず動かさないために書き写したもの。k を較正し直したときは、
// 動いた理由を書いたうえで、ここを書き換える。
func TestDefaultWeeklyTarget_PinnedValues(t *testing.T) {
	cases := []struct {
		name      string
		freq      int
		exercises int
		sets      int
		want      map[training.MuscleRegion]float64
	}{
		{
			// 3 × 4 × 3 = 36セット。36 × 2.20 = 79.2
			name: "週3回・4種目・3セット", freq: 3, exercises: 4, sets: 3,
			want: map[training.MuscleRegion]float64{
				training.Abs: 2.880952, training.Adductor: 2.095238, training.Biceps: 3.142857,
				training.Calf: 1.676190, training.ChestLower: 2.357143, training.ChestMid: 5.238095,
				training.ChestUpper: 2.619048, training.Erector: 6.547619, training.Forearm: 2.619048,
				training.FrontDelt: 4.452381, training.Glute: 7.333333, training.Hamstring: 4.714286,
				training.Lat: 4.452381, training.Oblique: 2.357143, training.Quad: 6.547619,
				training.RearDelt: 2.095238, training.SideDelt: 2.095238, training.TrapMid: 4.190476,
				training.TrapUpper: 3.142857, training.TricepsLateral: 6.547619, training.TricepsLong: 2.095238,
			},
		},
		{
			// 7 × 6 × 6 = 252セット。頻度と量が最大のとき。
			name: "週7回・6種目・6セット", freq: 7, exercises: 6, sets: 6,
			want: map[training.MuscleRegion]float64{
				training.Abs: 20.166667, training.Adductor: 14.666667, training.Biceps: 22.000000,
				training.Calf: 11.733333, training.ChestLower: 16.500000, training.ChestMid: 36.666667,
				training.ChestUpper: 18.333333, training.Erector: 45.833333, training.Forearm: 18.333333,
				training.FrontDelt: 31.166667, training.Glute: 51.333333, training.Hamstring: 33.000000,
				training.Lat: 31.166667, training.Oblique: 16.500000, training.Quad: 45.833333,
				training.RearDelt: 14.666667, training.SideDelt: 14.666667, training.TrapMid: 29.333333,
				training.TrapUpper: 22.000000, training.TricepsLateral: 45.833333,
				training.TricepsLong: 14.666667,
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := program.NewFrequency(c.freq)
			if err != nil {
				t.Fatalf("頻度が不正: %v", err)
			}
			v, err := program.NewSessionVolume(c.exercises, c.sets)
			if err != nil {
				t.Fatalf("1回の量が不正: %v", err)
			}
			got, err := DefaultWeeklyTarget(f, v)
			if err != nil {
				t.Fatalf("週目標が不正: %v", err)
			}
			for r, want := range c.want {
				// 値は小数6桁まで書き写してある。量子化の幅（1e-6）の倍まで許す。
				if d := math.Abs(got.Sets(r) - want); d > 2e-6 {
					t.Errorf("%s が %.6f。%.6f のはず", r, got.Sets(r), want)
				}
			}
		})
	}
}
