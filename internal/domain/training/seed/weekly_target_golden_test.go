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
			// 3 × 4 × 3 = 36セット。36 × 2.27 = 81.72
			name: "週3回・4種目・3セット", freq: 3, exercises: 4, sets: 3,
			want: map[training.MuscleRegion]float64{
				training.Abs: 2.972619, training.Adductor: 2.161905, training.Biceps: 3.242857,
				training.Calf: 1.729524, training.ChestLower: 2.432143, training.ChestMid: 5.404762,
				training.ChestUpper: 2.702381, training.Erector: 6.755952, training.Forearm: 2.702381,
				training.FrontDelt: 4.594048, training.Glute: 7.566667, training.Hamstring: 4.864286,
				training.Lat: 4.594048, training.Oblique: 2.432143, training.Quad: 6.755952,
				training.RearDelt: 2.161905, training.SideDelt: 2.161905, training.TrapMid: 4.323810,
				training.TrapUpper: 3.242857, training.TricepsLateral: 6.755952, training.TricepsLong: 2.161905,
			},
		},
		{
			// 7 × 6 × 6 = 252セット。頻度と量が最大のとき。
			name: "週7回・6種目・6セット", freq: 7, exercises: 6, sets: 6,
			want: map[training.MuscleRegion]float64{
				training.Abs: 20.808333, training.Adductor: 15.133333, training.Biceps: 22.700000,
				training.Calf: 12.106667, training.ChestLower: 17.025000, training.ChestMid: 37.833333,
				training.ChestUpper: 18.916667, training.Erector: 47.291667, training.Forearm: 18.916667,
				training.FrontDelt: 32.158333, training.Glute: 52.966667, training.Hamstring: 34.050000,
				training.Lat: 32.158333, training.Oblique: 17.025000, training.Quad: 47.291667,
				training.RearDelt: 15.133333, training.SideDelt: 15.133333, training.TrapMid: 30.266667,
				training.TrapUpper: 22.700000, training.TricepsLateral: 47.291667,
				training.TricepsLong: 15.133333,
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
