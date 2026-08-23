package training

import (
	"fmt"
	"sort"
	"strings"
)

const (
	// defaultStallSessions は停滞と見なす連続セッション数。
	//
	// 推定1RMが動く最小単位は、85kg なら1レップぶんで約2.83kg（2.5%）。
	// 中級者の現実的な伸び（週+0.5kg）だと、1レップ増えるまで約5.7週かかる。
	// メイン種目はバリエーションの日に実施しないので週2回として、
	// これは約11セッション。
	//
	// 窓を3セッションにすると、順調に伸びている人の半数のセッションで
	// 「停滞」と判定される。判定に使う指標の分解能より短い窓は意味を持たない。
	// 8セッション（週2回なら約1ヶ月）に置く。
	defaultStallSessions    = 8
	defaultIntensityDropPct = 0.10

	// cuttingThresholdKgPerWeek はこれを下回れば減量中と判定する。
	cuttingThresholdKgPerWeek = -0.1

	// maxStallSessions は停滞判定に使う連続セッション数の上限。
	// これを超えると、判定に必要な履歴が現実的に集まらない。
	maxStallSessions = 20
)

// DeloadPolicy はデロードの要否を判断するドメインサービス。無状態。
type DeloadPolicy struct {
	analyzer      ConditionAnalyzer
	stallSessions int
	intensityDrop Ratio
}

func NewDeloadPolicy(analyzer ConditionAnalyzer, stallSessions int, intensityDropPct float64) (DeloadPolicy, error) {
	if analyzer.IsZero() {
		return DeloadPolicy{}, fmt.Errorf("コンディション分析器が未設定である")
	}
	if stallSessions < 1 || stallSessions > maxStallSessions {
		return DeloadPolicy{}, fmt.Errorf(
			"停滞セッション数は1〜%dの範囲である必要がある: %d", maxStallSessions, stallSessions)
	}
	// 低下率は「強度に掛ける比率」なので Ratio の制約に従う。
	drop, err := NewRatio(intensityDropPct)
	if err != nil {
		return DeloadPolicy{}, fmt.Errorf("強度低下率: %w", err)
	}
	if drop.Float() >= 1 {
		return DeloadPolicy{}, fmt.Errorf("強度低下率は1未満である必要がある: %v", intensityDropPct)
	}
	return DeloadPolicy{analyzer: analyzer, stallSessions: stallSessions, intensityDrop: drop}, nil
}

func DefaultDeloadPolicy() DeloadPolicy {
	drop, err := NewRatio(defaultIntensityDropPct)
	if err != nil {
		panic(fmt.Sprintf("既定の強度低下率が不正: %v", err))
	}
	return DeloadPolicy{
		analyzer:      DefaultConditionAnalyzer(),
		stallSessions: defaultStallSessions,
		intensityDrop: drop,
	}
}

// Analyzer は判定に使うコンディション分析器。
// セッション生成器が RIR 補正に同じ分析器を使えるようにする。
func (p DeloadPolicy) Analyzer() ConditionAnalyzer { return p.analyzer }

func (p DeloadPolicy) StallSessions() int        { return p.stallSessions }
func (p DeloadPolicy) IntensityDropPct() float64 { return p.intensityDrop.Float() }
func (p DeloadPolicy) IsZero() bool              { return p == DeloadPolicy{} }

// Propose はデロードを提案する。適用はしない。承認するのはユーザー。
//
// 発火条件は「体重トレンドが横ばい以上」かつ「推定1RMが stallSessions 回
// 連続で更新されない」。
//
// 減量中の停滞は正常なので発火させない。体重が分からないときも発火させない。
// 減量による停滞とオーバーリーチによる停滞は、トレーニング記録だけ見ると
// 同じ形をしているため、体重が無いと誤診になる。
func (p DeloadPolicy) Propose(
	h History,
	mainIDs []ExerciseID,
	log ConditionLog,
	date Date,
) (DeloadProposal, bool) {
	if p.IsZero() || date.IsZero() || len(mainIDs) == 0 {
		return DeloadProposal{}, false
	}

	trend, ok := p.analyzer.BodyWeightTrendKgPerWeek(log, date)
	if !ok {
		return DeloadProposal{}, false
	}
	if trend < cuttingThresholdKgPerWeek {
		return DeloadProposal{}, false
	}

	// 履歴の絞り込みは1度だけ。種目ごとに全履歴を複製すると、
	// 3年ぶんの記録で1回の計画あたり数MBのゴミが出る。
	known := h.OnOrBefore(date)

	seen := make(map[ExerciseID]bool, len(mainIDs))
	stalled := make([]ExerciseID, 0, len(mainIDs))
	for _, id := range mainIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		if p.isStalled(known, id) {
			stalled = append(stalled, id)
		}
	}
	if len(stalled) == 0 {
		return DeloadProposal{}, false
	}
	sort.Slice(stalled, func(i, j int) bool { return stalled[i] < stalled[j] })

	names := make([]string, 0, len(stalled))
	for _, id := range stalled {
		names = append(names, string(id))
	}
	reason := fmt.Sprintf(
		"推定1RMが%dセッション更新されていない（%s）、体重トレンド %+.2fkg/週",
		p.stallSessions, strings.Join(names, ", "), trend)

	return DeloadProposal{
		reason:           reason,
		intensityDropPct: p.intensityDrop.Float(),
		stalled:          stalled,
	}, true
}

// isStalled は停滞しているか。次の2つをともに満たすときに真。
//
//  1. 窓の先頭を、それ以降のどのセッションも上回っていない
//  2. 直近のセッションが、窓の途中のどれも上回っていない
//
// 「更新」に許容幅を設けない。推定1RMが動く最小単位は、レップ1本ぶん
// （2.4〜2.9%）か実施重量のグリッド1段（1.25〜4.2%）で、それ未満の変化は
// そもそも起こりえない。0.5% のような閾値を置いても 0 と同じ意味にしかならず、
// 「わずかに伸びている」を停滞と誤判定する余地を作るだけになる。
//
// 条件1だけだと、病み上がりやデロード直後のように「窓の入口が高くて
// そこから回復している」状態を停滞と呼んでしまう。毎回更新しているのに
// 過去の最高値に届いていないだけ、という状況で重量を下げるのは誤り。
// 条件2でそれを除く。
//
// 条件2で「1つ前」ではなく「窓の途中の最大値」と比べるのは、
// 上下に振れているだけの状態を回復と誤認しないため。
// 85 → 82.5 → 85 → 82.5 → 85 は、最後だけ見れば上がっているが
// 一度も過去を超えていない。
func (p DeloadPolicy) isStalled(h History, id ExerciseID) bool {
	values := p.sessionValues(h, id)
	if len(values) < p.stallSessions+1 {
		return false
	}

	window := values[len(values)-(p.stallSessions+1):]

	improvedOnReference := false
	for _, v := range window[1:] {
		if v > window[0] {
			improvedOnReference = true
			break
		}
	}
	if improvedOnReference {
		return false
	}

	// 直近が窓の途中のどれかを上回っていれば、回復・上昇の途中とみなす。
	last := len(window) - 1
	peak := window[1]
	for _, v := range window[1:last] {
		if v > peak {
			peak = v
		}
	}
	if window[last] > peak {
		return false
	}

	// すでに窓の最高値から低下率ぶん以上落ちているなら、デロードの効果が
	// まだ出ていない段階か、体調を崩している段階のどちらか。ここでさらに
	// 下げると、下げる → 推定1RMが下がる → また停滞判定、の閉ループになり、
	// 承認するたびに永久に軽くなり続ける。
	return window[last] >= peak*(1-p.intensityDrop.Float())
}

// sessionValues は推定できたセッション代表値を古い順に返す。
//
// 推定できないセッション（全セット自重）は除く。0 として混ぜると、
// その回が窓の基準になったとき「以降すべて更新した」ことになり、
// 本当は停滞しているのに提案されなくなる。
func (p DeloadPolicy) sessionValues(h History, id ExerciseID) []float64 {
	sessions := h.ForExercise(id).Sessions()
	out := make([]float64, 0, len(sessions))
	for _, s := range sessions {
		if v, ok := s.MedianOneRepMax(); ok {
			out = append(out, v.Kg())
		}
	}
	return out
}
