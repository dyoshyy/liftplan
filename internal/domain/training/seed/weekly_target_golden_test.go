package seed

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// 週目標の値は動かさない。
//
// 週目標の大きさ（k）が種目カタログの平均から切り離され、定数になった
// （docs/specs/2026-10-02-weekly-target-as-ratio-design.md の案A）。切り離す
// 前後で数字が1つも動かないことが、この変更の検収。値は切り離す前の
// DefaultWeeklyTarget が返した数字を、そのまま書き写したもの。
//
// k を較正し直す（実測に合わせる）PR では、この数字が動くのが正しい。
// そのときは、動かした理由を書いたうえでここを書き換える。
func TestDefaultWeeklyTarget_PinnedValues(t *testing.T) {
	cases := []struct {
		name      string
		freq      int
		exercises int
		sets      int
		want      map[training.MuscleRegion]float64
	}{
		{
			// 3 × 4 × 3 = 36セット。36 × 1.755263 = 63.19
			name: "週3回・4種目・3セット", freq: 3, exercises: 4, sets: 3,
			want: map[training.MuscleRegion]float64{
				training.Abs: 2.278965, training.Adductor: 2.071786, training.Biceps: 2.486143,
				training.Calf: 1.657429, training.ChestLower: 1.864607, training.ChestMid: 4.143572,
				training.ChestUpper: 2.071786, training.Erector: 5.179465, training.Forearm: 2.071786,
				training.FrontDelt: 3.522036, training.Glute: 5.801001, training.Hamstring: 4.350751,
				training.Lat: 3.522036, training.Oblique: 1.864607, training.Quad: 5.179465,
				training.RearDelt: 1.657429, training.SideDelt: 1.657429, training.TrapMid: 3.314858,
				training.TrapUpper: 1.657429, training.TricepsLateral: 5.179465, training.TricepsLong: 1.657429,
			},
		},
		{
			// 7 × 6 × 6 = 252セット。頻度と量が最大のとき。
			name: "週7回・6種目・6セット", freq: 7, exercises: 6, sets: 6,
			want: map[training.MuscleRegion]float64{
				training.Abs: 15.952752, training.Adductor: 14.502502, training.Biceps: 17.403003,
				training.Calf: 11.602002, training.ChestLower: 13.052252, training.ChestMid: 29.005004,
				training.ChestUpper: 14.502502, training.Erector: 36.256255, training.Forearm: 14.502502,
				training.FrontDelt: 24.654254, training.Glute: 40.607006, training.Hamstring: 30.455255,
				training.Lat: 24.654254, training.Oblique: 13.052252, training.Quad: 36.256255,
				training.RearDelt: 11.602002, training.SideDelt: 11.602002, training.TrapMid: 23.204003,
				training.TrapUpper: 11.602002, training.TricepsLateral: 36.256255, training.TricepsLong: 11.602002,
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
