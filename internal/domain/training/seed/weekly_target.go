package seed

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// regionShare は筋区分ごとの配分。**比だけが意味を持つ。**
//
// パワーリフティング寄りに、BIG3 が直接使う区分を厚くしている。
// ただし「厚くしたい区分」ではなく「その頻度で実際に供給できる量」を
// 置いている。届かない目標は毎週すべての区分が赤字の画面を出し続けるだけで、
// 何も導かない。数字は通し検証（simulation_test.go）で実測して決めた。
//
// 副次刺激の総和が大きい区分（上腕三頭筋外側・前部三角筋・前腕・内転筋）は、
// 狙わなくても BIG3 と各種プレスで埋まる。ここを小さく置くと、埋まって
// いるのに残差が常に0になり、その区分を主働筋とする種目が永久に選ばれない。
var regionShare = map[training.MuscleRegion]float64{
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

// baseWeeklyStimulus は週3回で供給される刺激の総量。
//
// **配分と別に持つ。**同じ表で両方を表すと、1区分の重みを動かしただけで
// 週全体の量が動き、逆に量を絞ると21個すべてを引き直すことになる。実際
// baseProfile はそうなっていて、供給量（補助8スロット）に合わせて全区分を
// 引き直した跡が残っている。
//
// この値は供給の実測から来ている。週3回は1セッション27セット×3回＝81セット、
// 1セットあたりの区分への寄与が平均1.77なので、81×1.77≒143。表の合計152.5は
// それに合わせてある。
//
// **1日の種目数を可変にするときは、ここが差し替え点になる。**配分表は触らない。
const baseWeeklyStimulus = 152.5

// baseFrequency は baseWeeklyStimulus の基準となる週あたりの回数。
const baseFrequency = 3

// DefaultWeeklyTarget は筋区分ごとの週目標セット数のプリセット。
//
// 総量（頻度から決まる）を配分（regionShare）で割り振る。頻度を受け取るのは、
// 1週間に供給できるセット数が頻度に比例するため。固定値にすると、週1回の
// ユーザーは全区分が3割の達成率で埋まらず、週4回のユーザーは狙っていない
// 区分まで2倍に膨らむ。どちらの場合も目標が実際の挙動を説明しなくなり、
// 数字を見る意味が消える。
//
// この数字は利用者の設定ではない。補助セレクタが「その区分はもう足りて
// いるか」を判定する閾値で、種目マスタの刺激プロファイルと対になっている。
// 何セットが適切かは本人に答えられる問いではないので、編集させない（D-139）。
//
// 量を決めているのはここではない。補助の本数は AccessorySelector の
// maxSlots で打ち切られており、残差が尽きて止まることは低頻度では起きない
// （通し検証で週1〜3回は全セッションが27セットで固定）。この表が効くのは
// 「どの区分を狙うか」のゲートと、同点のときの順序付けまで。
func DefaultWeeklyTarget(f program.Frequency) (program.WeeklyVolumeTarget, error) {
	total := baseWeeklyStimulus * float64(f.PerWeek()) / baseFrequency

	// 合計は実行時に取る。定数に書くと、配分を1つ動かしたときに合計だけが
	// 古いまま残り、割り振りが静かにずれる。
	sum := 0.0
	for _, w := range regionShare {
		sum += w
	}

	scaled := make(map[training.MuscleRegion]float64, len(regionShare))
	for r, w := range regionShare {
		scaled[r] = w / sum * total
	}
	return program.NewWeeklyVolumeTarget(scaled)
}
