package seed

import (
	"errors"
	"fmt"

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

// averageStimulusPerSet は1セットが筋区分に与える寄与の合計の平均。
//
// 週の総セット数から「週に供給される刺激の総量」に直す係数。1セットが
// 複数の区分に寄与する（スクワットは大腿四頭筋1.0＋臀筋0.5）ので、
// セット数と刺激量は1対1ではない。
//
// **定数に書かずに数える。**この値は種目カタログの性質であって、独立した
// 調整つまみではない。書き写すと、種目を足したときに黙ってずれる。
//
// 選択された種目ではなくカタログ全体で取る。利用者が種目を外すたびに週目標が
// 動くと、目標が「何を選んだか」に依存して意味が薄れる。カタログの性質として
// 固定しておくほうが、達成率の読み方が安定する。
func averageStimulusPerSet() (float64, error) {
	all, err := Exercises()
	if err != nil {
		return 0, fmt.Errorf("種目カタログが読めない: %w", err)
	}
	if len(all) == 0 {
		return 0, errors.New("種目カタログが空である")
	}

	total := 0.0
	for _, e := range all {
		for _, r := range e.Stimulus().Regions() {
			c, ok := e.Stimulus().Contribution(r)
			if !ok {
				continue
			}
			total += c.Float()
		}
	}
	return total / float64(len(all)), nil
}

// DefaultWeeklyTarget は筋区分ごとの週目標セット数のプリセット。
//
// **週に供給できる量を、配分で割り振る。**供給量は利用者の設定だけで決まる。
//
//	週の総セット数 = 頻度 × 1回の種目数 × 1種目あたりのセット数
//	週の刺激総量   = 週の総セット数 × 1セットあたりの平均寄与
//
// 供給から導くのは、届かない目標を置かないため。目標を供給と無関係に置くと、
// 毎週すべての区分が赤字の画面を出し続けるだけで何も導かない。逆に供給より
// 小さく置くと、残差が常に0になってその区分を主働筋とする種目が選ばれない。
//
// この数字は利用者の設定ではない。補助セレクタが「その区分はもう足りて
// いるか」を判定する閾値で、種目マスタの刺激プロファイルと対になっている。
// 何セットが適切かは本人に答えられる問いではないので、編集させない（D-139）。
//
// 量を決めているのはここではない。補助の本数は AccessorySelector の
// maxSlots で打ち切られており、残差が尽きて止まることは低頻度では起きない
// （通し検証で週1〜3回は全セッションが27セットで固定）。この表が効くのは
// 「どの区分を狙うか」のゲートと、同点のときの順序付けまで。
func DefaultWeeklyTarget(f program.Frequency, v program.SessionVolume) (program.WeeklyVolumeTarget, error) {
	k, err := averageStimulusPerSet()
	if err != nil {
		return program.WeeklyVolumeTarget{}, err
	}
	total := float64(f.PerWeek()*v.TotalSets()) * k

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
