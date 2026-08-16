package seed

import "github.com/dyoshyy/liftplan-server/internal/domain/training"

// DefaultWeeklyTarget は筋区分ごとの週目標セット数のプリセット。
//
// パワーリフティング寄りに、BIG3 が直接使う区分（大腿四頭筋・ハム・臀筋・
// 脊柱起立筋・大胸筋中部）を厚くし、装飾的な区分は薄くしている。
// 不満が出た区分だけ後から調整すればよく、最初から自分で全部決める必要はない。
func DefaultWeeklyTarget() (training.WeeklyVolumeTarget, error) {
	return training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestUpper: 8,
		training.ChestMid:   14,
		training.ChestLower: 6,

		training.Lat:       12,
		training.TrapMid:   12,
		training.TrapUpper: 6,
		training.Erector:   12,

		training.FrontDelt: 8,
		training.SideDelt:  10,
		training.RearDelt:  8,

		training.TricepsLong:    8,
		training.TricepsLateral: 10,

		training.Biceps:  10,
		training.Forearm: 6,

		training.Quad:      16,
		training.Hamstring: 12,
		training.Glute:     12,
		training.Adductor:  6,
		training.Calf:      8,

		training.Abs:     8,
		training.Oblique: 6,
	})
}
