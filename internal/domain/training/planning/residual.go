package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
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
//
// active はその区分がこれからの1週ぶんで何回狙われるか。nil なら天井なし。
//
// **分割があるときだけ天井を掛ける。**分割が無いときに掛けると、全区分の
// 取り分が一律「週目標 ÷ 頻度」に揃い、補助が枯れた区分に集中せず散る
// （軸がベンチの日の上体ボリュームが 33.6 → 18.0 まで落ちた）。分割が
// 無いときの「その日らしさ」は残差の偏りだけが作っているので、均すと消える。
//
// 分割があるときは話が逆になる。その日らしさは分割が構造として決めるので、
// 天井は「1週ぶんの量をその区分が出る日数で分ける」だけの働きをする。
// 掛けないと、上下2分割の最初の下半身の日が週の下半身目標を丸ごと使い切り
// （実測38.1）、次の下半身の日が6セットまで落ちる。
func SessionResidual(
	target program.WeeklyVolumeTarget,
	window, thisSession StimulusCoverage,
	active ActiveCount,
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

		share := gap
		if active != nil {
			n := active(region)
			if n <= 0 {
				continue
			}
			// 今日すでに積んだ分は天井からも引く。引かないと、軸が
			// 埋めた区分に補助が1回ぶんを上乗せする。
			room := target.Sets(region)/float64(n) - thisSession.Sets(region)
			share = min(share, room)
		}

		if q := training.Quantize(share); q > 0 {
			out[region] = q
		}
	}
	return out
}

// ActiveCount はその筋区分が、これからの1週ぶんのセッションのうち
// 何回狙われるかを返す。
type ActiveCount func(training.MuscleRegion) int
