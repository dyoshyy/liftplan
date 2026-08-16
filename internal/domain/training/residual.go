package training

// StimulusCoverage は各筋区分がすでに何セット分埋まっているか。不変。
//
// 内部のマップを公開しないのは、外から NaN や無限大を書き込めるようにすると
// 残差が壊れるため。+Inf の残差は補助種目の選択で永久に最優先され、
// しかも有限値を引いても減らないので、その区分がスロットを食い尽くす。
type StimulusCoverage struct {
	m map[MuscleRegion]float64
}

// Sets はその筋区分が何セット分埋まっているか。未知の区分は0。
func (c StimulusCoverage) Sets(r MuscleRegion) float64 { return c.m[r] }

func (c StimulusCoverage) IsEmpty() bool { return len(c.m) == 0 }

// Plus は種目を実施したぶんの刺激を加えた、新しいカバレッジを返す。
//
// 破壊的に書き換えないのは、ゼロ値に対する Add が nil マップへの代入で
// panic するのを避けるため。ゼロ値が有効な「まだ何も埋まっていない状態」
// として使える方が、呼び出し側の初期化忘れで落ちるより安全。
func (c StimulusCoverage) Plus(p StimulusProfile, sets SetCount) StimulusCoverage {
	out := make(map[MuscleRegion]float64, len(c.m)+len(p.Regions()))
	for r, v := range c.m {
		out[r] = v
	}
	for _, region := range p.Regions() {
		contribution, ok := p.Contribution(region)
		if !ok {
			continue
		}
		out[region] = quantize(out[region] + contribution.TimesSets(sets))
	}
	return StimulusCoverage{m: out}
}

// SessionResidual はこのセッションで狙うべき、筋区分ごとの不足セット数。
//
// 週目標から「その週にすでに埋めた分」を引き、残りのセッション数で割る。
// 割る前に引くのが要点で、これによって過不足が翌セッションへ繰り越される。
//
// 週目標を頻度で割った固定値を毎回使うと、繰り越しが起きない。
// 補助種目は3セット固定なので、目標の小さい区分は毎回3倍超過し、
// 目標の大きい区分は毎回埋まらない。どちらの誤差も次に伝わらないため、
// 週を通した目標は構造的に達成できなくなる。
//
// sessionsRemaining はこのセッションを含む、その週の残りセッション数。
// 0以下なら「もう今週は埋める余地が無い」とみなして空を返す。
func SessionResidual(
	target WeeklyVolumeTarget,
	coveredThisWeek StimulusCoverage,
	sessionsRemaining int,
) map[MuscleRegion]float64 {
	out := map[MuscleRegion]float64{}
	if target.IsEmpty() || sessionsRemaining <= 0 {
		return out
	}

	for _, region := range target.Regions() {
		gap := target.Sets(region) - coveredThisWeek.Sets(region)
		if gap <= 0 {
			continue
		}
		share := quantize(gap / float64(sessionsRemaining))
		if share > 0 {
			out[region] = share
		}
	}
	return out
}
