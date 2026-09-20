package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// StimulusCoverage は各筋区分がすでに何セット分埋まっているか。不変。
//
// 内部のマップを公開しないのは、外から NaN や無限大を書き込めるようにすると
// 残差が壊れるため。+Inf の残差は補助種目の選択で永久に最優先され、
// しかも有限値を引いても減らないので、その区分がスロットを食い尽くす。
type StimulusCoverage struct {
	m map[training.MuscleRegion]float64
}

// Sets はその筋区分が何セット分埋まっているか。未知の区分は0。
func (c StimulusCoverage) Sets(r training.MuscleRegion) float64 { return c.m[r] }

func (c StimulusCoverage) IsEmpty() bool { return len(c.m) == 0 }

// Plus は種目を実施したぶんの刺激を加えた、新しいカバレッジを返す。
//
// 破壊的に書き換えないのは、ゼロ値に対する Add が nil マップへの代入で
// panic するのを避けるため。ゼロ値が有効な「まだ何も埋まっていない状態」
// として使える方が、呼び出し側の初期化忘れで落ちるより安全。
func (c StimulusCoverage) Plus(p exercise.StimulusProfile, sets training.SetCount) StimulusCoverage {
	out := make(map[training.MuscleRegion]float64, len(c.m)+len(p.Regions()))
	for r, v := range c.m {
		out[r] = v
	}
	for _, region := range p.Regions() {
		contribution, ok := p.Contribution(region)
		if !ok {
			continue
		}
		out[region] = training.Quantize(out[region] + contribution.TimesSets(sets))
	}
	return StimulusCoverage{m: out}
}

// SessionResidual はこのセッションで狙うべき、筋区分ごとの不足セット数。
//
// 直近1週の実績と、今日すでに積んだ分を、週目標から引いた残り。
//
// **割らない。**暦週のころは残りセッション数で割っていたが、ローリング窓には
// 「今週の残り」という区切りが存在しない（窓が毎日ずれる）。
//
// 1回ぶんの天井を掛ける案も測ったが、逆効果だった。全区分の share が
// 一律「週目標 ÷ 頻度」に揃うので、補助が枯れた区分に集中せず散る。
// 軸がベンチの日の上体ボリュームが 33.6 → 18.0 まで落ちた。
//
// 天井を提案した理由は「1週間休んだ翌日に1つの区分がスロットを食い尽くす」
// だったが、実測では起きない（最大2種目）。補助の選択は区分の古さで回すので、
// 残差が大きいだけでは同じ区分に積み上がらない。
//
// thisSession は今日すでに積んだ分（軸とバリエーション）。引かないと、
// 軸が胸を3セット埋めた日でも補助が同じだけ上乗せする。
func SessionResidual(
	target program.WeeklyVolumeTarget,
	window, thisSession StimulusCoverage,
) map[training.MuscleRegion]float64 {
	out := map[training.MuscleRegion]float64{}
	if target.IsEmpty() {
		return out
	}

	for _, region := range target.Regions() {
		gap := target.Sets(region) - window.Sets(region) - thisSession.Sets(region)
		if gap <= 0 {
			continue
		}
		if share := training.Quantize(gap); share > 0 {
			out[region] = share
		}
	}
	return out
}
