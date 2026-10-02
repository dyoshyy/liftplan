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

// stimulusPerSet は1セットが筋区分に与える寄与の合計の平均（k）。
//
// 週の総セット数から「週に供給される刺激の総量」に直す係数。1セットが
// 複数の区分に寄与する（スクワットは大腿四頭筋1.0＋臀筋0.5）ので、
// セット数と刺激量は1対1ではない。
//
// **種目カタログから数えない。**以前は seed.Exercises() の全種目の平均を
// 実行時に数えていた。すると、プリセットを1つ足した瞬間に、その種目を
// 持たない既存の利用者も含めた全員の週目標が動き、補助の選ばれ方まで
// 変わる（アイソラテラル9種目で 1.755→1.802、同じ履歴から今日の補助が
// 変わる回が全構成で961回中338回）。これはプリセットの部位ごとの種目数が
// 釣り合っていることを暗に前提にしてしまう。目標の大きさは、プリセットの
// 構成ではなく、利用者の設定（頻度・1回の量）だけで決める。
//
// 値は切り離した時点のカタログの平均（寄与の合計 66.7 ÷ 38種目）をそのまま
// 置いた。数字は1つも動いていない（TestDefaultWeeklyTarget_PinnedValues）。
// **この値が実態と合っているかは別の話**：処方どおりにこなしたときの
// 1セットあたりの供給は、全身法・upper_lower・ppl・five_way の週2〜7回で
// 1.9〜2.0（24構成の平均 1.947）あり、この値より約1割多い。実測に合わせる
// のは動作の変更なので、別に行う。
const stimulusPerSet = 66.7 / 38

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
// 量を決めているのはここではない。補助の本数は、1回の種目数（利用者の設定）
// から軸・バリエーションを引いた枠と、AccessoryAllocator が損失（ΔL）の
// 改善が無くなった時点で打ち切る貪欲法（docs/specs/2026-09-26-accessory-
// allocation-design.md）で決まる。この表が効くのは「どの区分を狙うか」の
// ゲートと、同点のときの順序付けまで。
func DefaultWeeklyTarget(f program.Frequency, v program.SessionVolume) (program.WeeklyVolumeTarget, error) {
	return distribute(regionShare, f, v)
}

// distribute は配分表を受け取って週目標を組む。
//
// 配分表を引数にしているのは、テストが共有の regionShare を書き換えずに
// 「配分を動かしても総量が動かない」を検査できるようにするため。パッケージ
// 変数を書き換えるテストは、並行に走らせた瞬間に他のテストと競合する。
func distribute(share map[training.MuscleRegion]float64, f program.Frequency, v program.SessionVolume) (program.WeeklyVolumeTarget, error) {
	total := float64(f.PerWeek()*v.TotalSets()) * stimulusPerSet

	// 合計は実行時に取る。定数に書くと、配分を1つ動かしたときに合計だけが
	// 古いまま残り、割り振りが静かにずれる。
	sum := 0.0
	for _, w := range share {
		sum += w
	}

	scaled := make(map[training.MuscleRegion]float64, len(share))
	for r, w := range share {
		scaled[r] = w / sum * total
	}
	return program.NewWeeklyVolumeTarget(scaled)
}
