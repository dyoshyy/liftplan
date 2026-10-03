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
			// 3 × 4 × 3 = 36セット。36 × 2.28 = 82.08
			name: "週3回・4種目・3セット", freq: 3, exercises: 4, sets: 3,
			want: map[training.MuscleRegion]float64{
				training.Abs: 3.025737, training.Adductor: 2.200536, training.Biceps: 3.300804,
				training.Calf: 1.760429, training.ChestLower: 2.475603, training.ChestMid: 5.501340,
				training.ChestUpper: 2.750670, training.Erector: 6.876676, training.Forearm: 2.750670,
				training.FrontDelt: 4.676139, training.Glute: 7.701877, training.Hamstring: 4.951206,
				training.Lat: 4.676139, training.Oblique: 2.475603, training.Quad: 6.876676,
				training.RearDelt: 2.200536, training.SideDelt: 2.200536, training.TrapMid: 4.401072,
				training.TrapUpper: 2.200536, training.TricepsLateral: 6.876676, training.TricepsLong: 2.200536,
			},
		},
		{
			// 7 × 6 × 6 = 252セット。頻度と量が最大のとき。
			name: "週7回・6種目・6セット", freq: 7, exercises: 6, sets: 6,
			want: map[training.MuscleRegion]float64{
				training.Abs: 21.180161, training.Adductor: 15.403753, training.Biceps: 23.105630,
				training.Calf: 12.323003, training.ChestLower: 17.329223, training.ChestMid: 38.509383,
				training.ChestUpper: 19.254692, training.Erector: 48.136729, training.Forearm: 19.254692,
				training.FrontDelt: 32.732976, training.Glute: 53.913137, training.Hamstring: 34.658445,
				training.Lat: 32.732976, training.Oblique: 17.329223, training.Quad: 48.136729,
				training.RearDelt: 15.403753, training.SideDelt: 15.403753, training.TrapMid: 30.807507,
				training.TrapUpper: 15.403753, training.TricepsLateral: 48.136729,
				training.TricepsLong: 15.403753,
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
