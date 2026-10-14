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
			// 3 × 4 × 3 = 36セット。36 × 2.23 = 80.28
			name: "週3回・4種目・3セット", freq: 3, exercises: 4, sets: 3,
			want: map[training.MuscleRegion]float64{
				training.Abs: 2.959383, training.Adductor: 2.152279, training.Biceps: 3.228418,
				training.Calf: 1.721823, training.ChestLower: 2.421314, training.ChestMid: 5.380697,
				training.ChestUpper: 2.690349, training.Erector: 6.725871, training.Forearm: 2.690349,
				training.FrontDelt: 4.573592, training.Glute: 7.532976, training.Hamstring: 4.842627,
				training.Lat: 4.573592, training.Oblique: 2.421314, training.Quad: 6.725871,
				training.RearDelt: 2.152279, training.SideDelt: 2.152279, training.TrapMid: 4.304558,
				training.TrapUpper: 2.152279, training.TricepsLateral: 6.725871, training.TricepsLong: 2.152279,
			},
		},
		{
			// 7 × 6 × 6 = 252セット。頻度と量が最大のとき。
			name: "週7回・6種目・6セット", freq: 7, exercises: 6, sets: 6,
			want: map[training.MuscleRegion]float64{
				training.Abs: 20.715684, training.Adductor: 15.065952, training.Biceps: 22.598928,
				training.Calf: 12.052761, training.ChestLower: 16.949196, training.ChestMid: 37.664879,
				training.ChestUpper: 18.832440, training.Erector: 47.081099, training.Forearm: 18.832440,
				training.FrontDelt: 32.015147, training.Glute: 52.730831, training.Hamstring: 33.898391,
				training.Lat: 32.015147, training.Oblique: 16.949196, training.Quad: 47.081099,
				training.RearDelt: 15.065952, training.SideDelt: 15.065952, training.TrapMid: 30.131903,
				training.TrapUpper: 15.065952, training.TricepsLateral: 47.081099,
				training.TricepsLong: 15.065952,
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
