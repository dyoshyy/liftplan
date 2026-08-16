package training

import (
	"fmt"
	"sort"
	"strings"
)

const (
	defaultStallSessions    = 3
	defaultIntensityDropPct = 0.10

	// cuttingThresholdKgPerWeek はこれを下回れば減量中と判定する。
	cuttingThresholdKgPerWeek = -0.1

	// maxStallSessions は停滞判定に使う連続セッション数の上限。
	// これを超えると、判定に必要な履歴が現実的に集まらない。
	maxStallSessions = 20
)

// DeloadProposal はデロードの提案。適用はしない。
//
// Reason はユーザーへ表示する根拠。黙って重量を下げないための情報。
type DeloadProposal struct {
	reason           string
	intensityDropPct float64
}

func (p DeloadProposal) Reason() string            { return p.reason }
func (p DeloadProposal) IntensityDropPct() float64 { return p.intensityDropPct }

func (p DeloadProposal) IsZero() bool { return p == DeloadProposal{} }

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

	stalled := make([]string, 0, len(mainIDs))
	for _, id := range mainIDs {
		if p.isStalled(h.OnOrBefore(date), id) {
			stalled = append(stalled, string(id))
		}
	}
	if len(stalled) == 0 {
		return DeloadProposal{}, false
	}
	sort.Strings(stalled)

	reason := fmt.Sprintf(
		"推定1RMが%dセッション更新されていない（%s）、体重トレンド %+.2fkg/週",
		p.stallSessions, strings.Join(stalled, ", "), trend)

	return DeloadProposal{reason: reason, intensityDropPct: p.intensityDrop.Float()}, true
}

// isStalled は直近 stallSessions 回で推定1RMが一度も更新されていないか。
//
// 「更新」に許容幅を設けない。推定1RMが動く最小単位は、レップ1本ぶん
// （2.4〜2.9%）か実施重量のグリッド1段（1.25〜4.2%）で、それ未満の変化は
// そもそも起こりえない。0.5% のような閾値を置いても 0 と同じ意味にしかならず、
// 「わずかに伸びている」を停滞と誤判定する余地を作るだけになる。
//
// 基準は直前の窓の最大値。単に1つ前と比べると、上下に振れているだけの
// 状態を「更新した」と誤判定する。
func (p DeloadPolicy) isStalled(h History, id ExerciseID) bool {
	sessions := h.ForExercise(id).Sessions()

	values := make([]float64, 0, len(sessions))
	for _, s := range sessions {
		if v, ok := s.MedianOneRepMax(); ok {
			values = append(values, v.Kg())
		}
	}
	if len(values) < p.stallSessions+1 {
		return false
	}

	window := values[len(values)-(p.stallSessions+1):]
	reference := window[0]
	for _, v := range window[1:] {
		if v > reference {
			return false
		}
	}
	return true
}
