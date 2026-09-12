package planning

import (
	"fmt"
	"math"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

const (
	defaultRecoveryDays      = 2
	defaultSetsPerAccessory  = 3
	defaultMaxAccessorySlots = 8
)

// primaryContribution はこの値以上の寄与を「主働筋として使う」とみなす境界。
//
// 回復期間中の筋区分に対しては、主働筋として使う種目を避ける。
// 補助的に軽く関与するぶん（三頭が 0.4 など）まで避けると、
// 多関節種目がほとんど選べなくなる。
const primaryContribution = 1.0

// neverStimulated は一度も刺激していない筋区分・種目を最優先にするための番兵。
const neverStimulated = 1 << 30

// AccessorySelector は残差を埋める補助種目を選ぶドメインサービス。無状態。
type AccessorySelector struct {
	recoveryDays     int
	setsPerAccessory training.SetCount
	maxSlots         int
}

func NewAccessorySelector(recoveryDays, setsPerAccessory, maxSlots int) (AccessorySelector, error) {
	if recoveryDays < 0 {
		return AccessorySelector{}, fmt.Errorf("回復日数は0以上である必要がある: %d", recoveryDays)
	}
	// セット数の妥当性は SetCount に委ねる。独自に判定すると、上限を超える値が
	// 通ったあと消費側で無言の no-op になり、残差が永久に減らなくなる。
	sets, err := training.NewSetCount(setsPerAccessory)
	if err != nil {
		return AccessorySelector{}, fmt.Errorf("補助のセット数: %w", err)
	}
	if maxSlots < 1 {
		return AccessorySelector{}, fmt.Errorf("補助スロットの上限は1以上である必要がある: %d", maxSlots)
	}
	return AccessorySelector{
		recoveryDays:     recoveryDays,
		setsPerAccessory: sets,
		maxSlots:         maxSlots,
	}, nil
}

func DefaultAccessorySelector() AccessorySelector {
	sets, err := training.NewSetCount(defaultSetsPerAccessory)
	if err != nil {
		panic(fmt.Sprintf("既定のセット数が不正: %v", err))
	}
	return AccessorySelector{
		recoveryDays:     defaultRecoveryDays,
		setsPerAccessory: sets,
		maxSlots:         defaultMaxAccessorySlots,
	}
}

func (s AccessorySelector) SetsPerAccessory() training.SetCount { return s.setsPerAccessory }
func (s AccessorySelector) RecoveryDays() int                   { return s.recoveryDays }
func (s AccessorySelector) MaxSlots() int                       { return s.maxSlots }

func (s AccessorySelector) IsZero() bool { return s == AccessorySelector{} }

// Select は残差を埋める補助種目を選ぶ。
//
// スロット数は固定しない。残差が無くなるか、埋められる種目が尽きるか、
// 上限に達するまで選び続ける。事前にスロット数を計算すると、
// 1種目が複数区分を埋める事実を無視した見積もりになり、
// 実際より多く積む日と、埋める余地を残して終わる日の両方が生まれる。
//
// 区分の選び方は「最も長く刺激していない区分から」。残差の大きい順にすると、
// 週目標の大きい区分（大腿四頭筋16セット）が常に勝ち、小さい区分
// （僧帽筋上部6セット）にスロットが一度も回らない。上限に張り付く低頻度では、
// その区分が永久に0セットのままになる。
//
// exclude には伸ばしたい種目をすべて渡す。補助レーンが軸レーンの仕事を
// 兼務しないため。
//
// 以前は今日のヘビー枠だけを除いていた（D-117）。「宣言は伸ばしたいという
// 目標であって、ヘビーでしかやらないではない。除きすぎると脚の日に
// スクワットがどこにも出なくなる」という理由だったが、これは的外れだった。
// スクワットが軸でない日に脚のボリュームを埋めるのはレッグプレスや
// レッグカールであって、スクワットである必要が無い。
//
// 宣言は軸レーンで扱うものと割り切ると、レーンの境界がはっきりする。
// 「今日その種目が出るかどうか」を決める場所が1つになるので、補助の
// 残差計算を変えても軸の頻度が動かない。
func (s AccessorySelector) Select(
	residual map[training.MuscleRegion]float64,
	pool []*exercise.Exercise,
	h setlog.History,
	date training.Date,
	exclude []exercise.ExerciseID,
) []exercise.ExerciseID {
	if s.IsZero() || len(residual) == 0 || date.IsZero() {
		return nil
	}

	excluded := make(map[exercise.ExerciseID]bool, len(exclude))
	for _, id := range exclude {
		excluded[id] = true
	}

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	accessories := make([]*exercise.Exercise, 0, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		// byID は候補リストではなく、履歴のIDから種目を引く辞書。
		// 除外した種目もここには入れる。抜くと recovering と
		// regionStaleness がその種目の過去の記録を読めなくなり、
		// 「昨日その区分を刺激した」が見えなくなる。
		byID[e.ID()] = e
		if excluded[e.ID()] {
			continue
		}
		accessories = append(accessories, e)
	}
	sort.Slice(accessories, func(i, j int) bool { return accessories[i].ID() < accessories[j].ID() })

	recovering := s.recovering(h, byID, date)
	remaining := s.trackable(residual, recovering)
	staleness := s.regionStaleness(h, byID, date)

	chosen := make([]exercise.ExerciseID, 0, s.maxSlots)
	taken := make(map[exercise.ExerciseID]bool, s.maxSlots)

	for len(chosen) < s.maxSlots && len(remaining) > 0 {
		region, ok := nextRegion(remaining, staleness)
		if !ok {
			break
		}

		candidate := s.pickForRegion(accessories, taken, recovering, region, h, date)
		if candidate == nil {
			// この区分を埋められる未使用の種目が無い。区分ごと諦める。
			delete(remaining, region)
			continue
		}

		chosen = append(chosen, candidate.ID())
		taken[candidate.ID()] = true
		s.consume(remaining, candidate.Stimulus())
	}
	return chosen
}

// trackable は残差のうち、実際に狙える区分だけを取り出す。
//
// 非有限値を落とすのは、+Inf の残差が常に最優先になったうえ、
// 有限値を引いても減らずスロットを食い尽くすため。
func (s AccessorySelector) trackable(
	residual map[training.MuscleRegion]float64,
	recovering map[training.MuscleRegion]bool,
) map[training.MuscleRegion]float64 {
	out := make(map[training.MuscleRegion]float64, len(residual))
	for region, gap := range residual {
		if gap <= 0 || math.IsNaN(gap) || math.IsInf(gap, 0) {
			continue
		}
		if recovering[region] {
			continue
		}
		out[region] = gap
	}
	return out
}

// consume は選んだ種目の刺激ぶんを残差から差し引く。
func (s AccessorySelector) consume(remaining map[training.MuscleRegion]float64, p exercise.StimulusProfile) {
	for _, r := range p.Regions() {
		c, ok := p.Contribution(r)
		if !ok {
			continue
		}
		if _, tracked := remaining[r]; !tracked {
			continue
		}
		remaining[r] = training.Quantize(remaining[r] - c.TimesSets(s.setsPerAccessory))
		if remaining[r] <= 0 {
			delete(remaining, r)
		}
	}
}

// recovering は回復期間中の筋区分。
//
// 区間は (date - recoveryDays, date) の開区間。
// 下限を開くのは、回復日数ぶん経過した記録は解禁されるべきだから。
// recoveryDays=2 なら、月曜の記録は火曜を塞ぐが水曜は解禁される。
// 閉じると実質72時間ルールになり、月水金の水曜がほぼ何も選べなくなる。
//
// 上限を開くのは、セッション中に記録してから計画を開き直したとき、
// たった今やった種目の筋区分で自分自身の枠が消えないようにするため。
func (s AccessorySelector) recovering(
	h setlog.History, byID map[exercise.ExerciseID]*exercise.Exercise, date training.Date) map[training.MuscleRegion]bool {
	cutoff := date.AddDays(-s.recoveryDays)
	out := map[training.MuscleRegion]bool{}

	for _, l := range h.After(cutoff).Before(date).Logs() {
		e, ok := byID[l.ExerciseID()]
		if !ok {
			continue
		}
		for _, r := range e.Stimulus().Regions() {
			out[r] = true
		}
	}
	return out
}

// regionStaleness は筋区分ごとの「最後に刺激してからの日数」。
// 一度も刺激していない区分は番兵で最優先にする。
func (s AccessorySelector) regionStaleness(
	h setlog.History, byID map[exercise.ExerciseID]*exercise.Exercise, date training.Date) map[training.MuscleRegion]int {
	out := map[training.MuscleRegion]int{}
	for _, l := range h.Logs() {
		e, ok := byID[l.ExerciseID()]
		if !ok {
			continue
		}
		days := date.DaysSince(l.PerformedOn())
		if days < 0 {
			days = 0
		}
		for _, r := range e.Stimulus().Regions() {
			if prev, seen := out[r]; !seen || days < prev {
				out[r] = days
			}
		}
	}
	return out
}

// pickForRegion はその区分を埋められる種目のうち、最後に使ってから
// 最も間隔が空いているものを選ぶ。これでバリエーションが自動で回る。
//
// 回復期間中の区分を主働筋として使う種目は避ける。
func (s AccessorySelector) pickForRegion(
	accessories []*exercise.Exercise, taken map[exercise.ExerciseID]bool,
	recovering map[training.MuscleRegion]bool,
	region training.MuscleRegion, h setlog.History, date training.Date) *exercise.Exercise {
	var best *exercise.Exercise
	bestDaysAgo := -1

	for _, e := range accessories {
		if taken[e.ID()] {
			continue
		}
		if _, ok := e.Stimulus().Contribution(region); !ok {
			continue
		}
		if s.hitsRecoveringPrimaryMover(e, recovering) {
			continue
		}

		daysAgo := neverStimulated
		if last, ok := h.LastPerformed(e.ID()); ok {
			daysAgo = date.DaysSince(last)
			if daysAgo < 0 {
				// 未来日のログ。時計のずれで入りうる。
				// 負のままだと、その種目が永久に選ばれなくなる。
				daysAgo = 0
			}
		}
		if daysAgo > bestDaysAgo {
			best = e
			bestDaysAgo = daysAgo
		}
	}
	return best
}

// hitsRecoveringPrimaryMover は、回復期間中の区分を主働筋として使う種目か。
func (s AccessorySelector) hitsRecoveringPrimaryMover(e *exercise.Exercise, recovering map[training.MuscleRegion]bool) bool {
	profile := e.Stimulus()
	for _, r := range profile.Regions() {
		if !recovering[r] {
			continue
		}
		if c, ok := profile.Contribution(r); ok && c.Float() >= primaryContribution {
			return true
		}
	}
	return false
}

// nextRegion は次に埋める筋区分。
//
// 最も長く刺激していない区分を優先し、同じなら残差の大きい方、
// それも同じなら名前の昇順（再現性のため）。
func nextRegion(remaining map[training.MuscleRegion]float64, staleness map[training.MuscleRegion]int) (training.MuscleRegion, bool) {
	regions := make([]training.MuscleRegion, 0, len(remaining))
	for r := range remaining {
		regions = append(regions, r)
	}
	if len(regions) == 0 {
		return "", false
	}

	daysAgo := func(r training.MuscleRegion) int {
		if d, ok := staleness[r]; ok {
			return d
		}
		return neverStimulated
	}

	sort.Slice(regions, func(i, j int) bool {
		a, b := regions[i], regions[j]
		if da, db := daysAgo(a), daysAgo(b); da != db {
			return da > db
		}
		if remaining[a] != remaining[b] {
			return remaining[a] > remaining[b]
		}
		return a < b
	})
	return regions[0], true
}
