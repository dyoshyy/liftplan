package seed

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// baseProfile は週3回を基準にした筋区分ごとの目標セット数。
//
// パワーリフティング寄りに、BIG3 が直接使う区分を厚くしている。
// ただし「厚くしたい区分」ではなく「その頻度で実際に供給できる量」を
// 置いている。届かない目標は毎週すべての区分が赤字の画面を出し続けるだけで、
// 何も導かない。数字は通し検証（simulation_test.go）で実測して決めた。
//
// 副次刺激の総和が大きい区分（上腕三頭筋外側・前部三角筋・前腕・内転筋）は、
// 狙わなくても BIG3 と各種プレスで埋まる。ここを小さく置くと、埋まって
// いるのに残差が常に0になり、その区分を主働筋とする種目が永久に選ばれない。
var baseProfile = map[training.MuscleRegion]float64{
	training.ChestUpper: 5,
	training.ChestMid:   10,
	training.ChestLower: 4.5,

	training.Lat:       8.5,
	training.TrapMid:   8,
	training.TrapUpper: 4,
	training.Erector:   12.5,

	training.FrontDelt: 8.5,
	training.SideDelt:  4,
	training.RearDelt:  4,

	training.TricepsLong:    4,
	training.TricepsLateral: 12.5,

	training.Biceps:  6,
	training.Forearm: 5,

	training.Quad:      12.5,
	training.Hamstring: 10.5,
	training.Glute:     14,
	training.Adductor:  5,
	training.Calf:      4,

	training.Abs:     5.5,
	training.Oblique: 4.5,
}

// baseFrequency は baseProfile の基準となる週あたりの回数。
const baseFrequency = 3

// DefaultWeeklyTarget は筋区分ごとの週目標セット数のプリセット。
//
// 頻度を受け取るのは、1週間に供給できるセット数が頻度に比例するため。
// 固定値にすると、週1回のユーザーは全区分が3割の達成率で埋まらず、
// 週4回のユーザーは狙っていない区分まで2倍に膨らむ。どちらの場合も
// 目標が実際の挙動を説明しなくなり、数字を見る意味が消える。
//
// 不満が出た区分だけ後から調整すればよく、最初から自分で全部決める
// 必要はない。
func DefaultWeeklyTarget(f program.Frequency) (program.WeeklyVolumeTarget, error) {
	scale := float64(f.PerWeek()) / baseFrequency
	scaled := make(map[training.MuscleRegion]float64, len(baseProfile))
	for r, sets := range baseProfile {
		scaled[r] = sets * scale
	}
	return program.NewWeeklyVolumeTarget(scaled)
}
