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
			// 3 × 4 × 3 = 36セット。36 × 1.97 = 70.92
			name: "週3回・4種目・3セット", freq: 3, exercises: 4, sets: 3,
			want: map[training.MuscleRegion]float64{
				training.Abs: 2.557770, training.Adductor: 2.325246, training.Biceps: 2.790295,
				training.Calf: 1.860197, training.ChestLower: 2.092721, training.ChestMid: 4.650492,
				training.ChestUpper: 2.325246, training.Erector: 5.813115, training.Forearm: 2.325246,
				training.FrontDelt: 3.952918, training.Glute: 6.510689, training.Hamstring: 4.883016,
				training.Lat: 3.952918, training.Oblique: 2.092721, training.Quad: 5.813115,
				training.RearDelt: 1.860197, training.SideDelt: 1.860197, training.TrapMid: 3.720393,
				training.TrapUpper: 1.860197, training.TricepsLateral: 5.813115, training.TricepsLong: 1.860197,
			},
		},
		{
			// 7 × 6 × 6 = 252セット。頻度と量が最大のとき。
			name: "週7回・6種目・6セット", freq: 7, exercises: 6, sets: 6,
			want: map[training.MuscleRegion]float64{
				training.Abs: 17.904393, training.Adductor: 16.276721, training.Biceps: 19.532066,
				training.Calf: 13.021377, training.ChestLower: 14.649049, training.ChestMid: 32.553443,
				training.ChestUpper: 16.276721, training.Erector: 40.691803, training.Forearm: 16.276721,
				training.FrontDelt: 27.670426, training.Glute: 45.574820, training.Hamstring: 34.181115,
				training.Lat: 27.670426, training.Oblique: 14.649049, training.Quad: 40.691803,
				training.RearDelt: 13.021377, training.SideDelt: 13.021377, training.TrapMid: 26.042754,
				training.TrapUpper: 13.021377, training.TricepsLateral: 40.691803,
				training.TricepsLong: 13.021377,
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
