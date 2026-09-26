package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// StimulusCoverage は各筋区分がすでに何セット分埋まっているか。不変。
//
// 内部のマップを公開しないのは、外から NaN や無限大を書き込めるようにすると
// 壊れるため。AccessoryAllocator の損失計算（regionLoss）はこの値を3乗して
// 候補どうしを比べるので、NaN や +Inf が混ざると全ての候補の優劣が
// 付かなくなる。
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

// CoverageBetween は期間内に埋めた刺激量を数える。両端を含む。
//
// 記録1件を1セットとして数える。SetLog は「確定した実績1セット」なので、
// 件数がそのままセット数になる。
//
// 公開しているのは、週目標の充足を見せる読み取り経路が同じ数え方を
// 必要とするため。別々に実装すると、画面に出る数字とエンジンが使う数字が
// ずれる。ずれた瞬間、どちらが正しいのか誰にも分からなくなる。
func CoverageBetween(h setlog.History, pool []*exercise.Exercise, from, to training.Date) StimulusCoverage {
	coverage := StimulusCoverage{}
	one, err := training.NewSetCount(1)
	if err != nil {
		return coverage
	}

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		byID[e.ID()] = e
	}

	for _, l := range h.OnOrAfter(from).OnOrBefore(to).Logs() {
		e, ok := byID[l.ExerciseID()]
		if !ok {
			continue
		}
		coverage = coverage.Plus(e.Stimulus(), one)
	}
	return coverage
}

// CoverageWindowWeeks は AccessoryAllocator が目標と比べる基準窓の長さ（週）。
//
// **4週（28日）。**以前は1週だった。1週の窓では、週目標が1種目ぶん（3セット）
// より小さい区分を抑えられない。週3回のカーフは目標1.7セット/週だが、1回
// 選ばれると3セット入り、7日経つとそれが窓から消えて「10割欠け」に戻り、
// すぐまた選ばれる。どう並べても週3セット未満にはならず、達成率は179%に
// 張り付いた。平均を取る期間の問題ではなく、エンジンの記憶の長さの問題で、
// 52週平均で測っても縮まない。
//
// 4週にすると、目標も4週ぶんで比べるので1回ぶんの刻みより十分大きくなる。
// 週2〜7回の全区分が 60〜145% に収まる（通し検証、1日4種目3セット）。
//
// 休んだあとの取り戻しは緩やかになる。2週休んでも、その前の2週ぶんが窓に
// 残るので、1日で全区分を取り返そうとしない。暦週をやめた理由（週の先頭で
// 全区分の不足が最大になり、一日で使い尽くす）はローリングのまま保たれる。
//
// 週1回は4週でも回りきらない（4週で48セットを21区分に配る）。週1回は
// 想定する利用者ではないので、この窓は週2回以上を基準に決めている。
const CoverageWindowWeeks = 4

// CoverageWindowDays は AccessoryAllocator が目標と比べる基準窓の長さ（日）。
const CoverageWindowDays = 7 * CoverageWindowWeeks
