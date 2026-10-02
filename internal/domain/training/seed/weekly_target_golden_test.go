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
			// 3 × 4 × 3 = 36セット。36 × 2.04 = 73.44
			name: "週3回・4種目・3セット", freq: 3, exercises: 4, sets: 3,
			want: map[training.MuscleRegion]float64{
				training.Abs: 2.648656, training.Adductor: 2.407869, training.Biceps: 2.889443,
				training.Calf: 1.926295, training.ChestLower: 2.167082, training.ChestMid: 4.815738,
				training.ChestUpper: 2.407869, training.Erector: 6.019672, training.Forearm: 2.407869,
				training.FrontDelt: 4.093377, training.Glute: 6.742033, training.Hamstring: 5.056525,
				training.Lat: 4.093377, training.Oblique: 2.167082, training.Quad: 6.019672,
				training.RearDelt: 1.926295, training.SideDelt: 1.926295, training.TrapMid: 3.852590,
				training.TrapUpper: 1.926295, training.TricepsLateral: 6.019672, training.TricepsLong: 1.926295,
			},
		},
		{
			// 7 × 6 × 6 = 252セット。頻度と量が最大のとき。
			name: "週7回・6種目・6セット", freq: 7, exercises: 6, sets: 6,
			want: map[training.MuscleRegion]float64{
				training.Abs: 18.540590, training.Adductor: 16.855082, training.Biceps: 20.226098,
				training.Calf: 13.484066, training.ChestLower: 15.169574, training.ChestMid: 33.710164,
				training.ChestUpper: 16.855082, training.Erector: 42.137705, training.Forearm: 16.855082,
				training.FrontDelt: 28.653639, training.Glute: 47.194230, training.Hamstring: 35.395672,
				training.Lat: 28.653639, training.Oblique: 15.169574, training.Quad: 42.137705,
				training.RearDelt: 13.484066, training.SideDelt: 13.484066, training.TrapMid: 26.968131,
				training.TrapUpper: 13.484066, training.TricepsLateral: 42.137705,
				training.TricepsLong: 13.484066,
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
