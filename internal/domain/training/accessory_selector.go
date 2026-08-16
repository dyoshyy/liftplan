package training

import (
	"fmt"
	"math"
	"sort"
)

const (
	defaultRecoveryDays      = 2
	defaultSetsPerAccessory  = 3
	defaultMaxAccessorySlots = 8
)

// neverUsedDaysAgo は一度も使っていない種目を最優先にするための番兵。
const neverUsedDaysAgo = 1 << 30

// AccessorySelector は残差を埋める補助種目を選ぶドメインサービス。無状態。
type AccessorySelector struct {
	recoveryDays     int
	setsPerAccessory int
	maxSlots         int
}

func NewAccessorySelector(recoveryDays, setsPerAccessory, maxSlots int) (AccessorySelector, error) {
	if recoveryDays < 0 {
		return AccessorySelector{}, fmt.Errorf("回復日数は0以上である必要がある: %d", recoveryDays)
	}
	if setsPerAccessory < 1 {
		return AccessorySelector{}, fmt.Errorf("補助のセット数は1以上である必要がある: %d", setsPerAccessory)
	}
	if maxSlots < 1 {
		return AccessorySelector{}, fmt.Errorf("補助スロットの上限は1以上である必要がある: %d", maxSlots)
	}
	return AccessorySelector{
		recoveryDays:     recoveryDays,
		setsPerAccessory: setsPerAccessory,
		maxSlots:         maxSlots,
	}, nil
}

func DefaultAccessorySelector() AccessorySelector {
	return AccessorySelector{
		recoveryDays:     defaultRecoveryDays,
		setsPerAccessory: defaultSetsPerAccessory,
		maxSlots:         defaultMaxAccessorySlots,
	}
}

func (s AccessorySelector) SetsPerAccessory() int { return s.setsPerAccessory }
func (s AccessorySelector) RecoveryDays() int     { return s.recoveryDays }
func (s AccessorySelector) MaxSlots() int         { return s.maxSlots }

func (s AccessorySelector) IsZero() bool { return s == AccessorySelector{} }

// Select は残差を埋める補助種目を選ぶ。
//
// スロット数は固定せず、残差の合計から導く。固定すると、残差が小さい日に
// 過剰なボリュームを積み、残差が大きい日には週目標に届かない。
//
//  1. recoveryDays 日以内に刺激された筋区分を候補から外す（48時間ルール）
//  2. 残差の大きい区分から貪欲に選ぶ。同値なら筋区分名の昇順（再現性のため）
//  3. 同じ区分を狙う種目が複数あれば、最後に使ってから最も間隔が空いているものを選ぶ
func (s AccessorySelector) Select(
	residual map[MuscleRegion]float64,
	pool []*Exercise,
	h History,
	date Date,
) []ExerciseID {
	if s.IsZero() || len(residual) == 0 || date.IsZero() {
		return nil
	}

	byID := make(map[ExerciseID]*Exercise, len(pool))
	accessories := make([]*Exercise, 0, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		byID[e.ID()] = e
		if e.Kind() == KindAccessory {
			accessories = append(accessories, e)
		}
	}
	sort.Slice(accessories, func(i, j int) bool { return accessories[i].ID() < accessories[j].ID() })

	blocked := s.recentlyStimulated(h, byID, date)

	remaining := make(map[MuscleRegion]float64, len(residual))
	for region, gap := range residual {
		if gap > 0 && !blocked[region] {
			remaining[region] = gap
		}
	}

	slots := s.slotsFor(remaining)
	chosen := make([]ExerciseID, 0, slots)
	taken := make(map[ExerciseID]bool, slots)

	for len(chosen) < slots {
		region, ok := topRegion(remaining)
		if !ok {
			break
		}

		candidate := s.pickForRegion(accessories, taken, region, h, date)
		if candidate == nil {
			// その区分を埋められる未使用の種目が無い。区分ごと諦める。
			delete(remaining, region)
			continue
		}

		chosen = append(chosen, candidate.ID())
		taken[candidate.ID()] = true
		s.consume(remaining, candidate.Stimulus())
	}
	return chosen
}

// slotsFor は残差の合計から必要なスロット数を導く。
//
// 残差の合計を1種目あたりのセット数で割った切り上げ。
// セッションが長くなりすぎないよう上限で頭打ちにする。
func (s AccessorySelector) slotsFor(remaining map[MuscleRegion]float64) int {
	total := 0.0
	for _, gap := range remaining {
		total += gap
	}
	if total <= 0 {
		return 0
	}

	needed := int(math.Ceil(total / float64(s.setsPerAccessory)))
	if needed > s.maxSlots {
		return s.maxSlots
	}
	return needed
}

// consume は選んだ種目の刺激ぶんを残差から差し引く。
func (s AccessorySelector) consume(remaining map[MuscleRegion]float64, p StimulusProfile) {
	sets, err := NewSetCount(s.setsPerAccessory)
	if err != nil {
		return
	}
	for _, r := range p.Regions() {
		c, ok := p.Contribution(r)
		if !ok {
			continue
		}
		if _, tracked := remaining[r]; !tracked {
			continue
		}
		remaining[r] = quantize(remaining[r] - c.TimesSets(sets))
		if remaining[r] <= 0 {
			delete(remaining, r)
		}
	}
}

// recentlyStimulated は回復期間内に刺激された筋区分。
//
// 半開区間 [cutoff, date) で見る。上限を閉じないと、セッション中に記録してから
// 計画を開き直したとき、たった今やった種目の筋区分が「最近刺激した」と
// 判定され、そのセッションの補助枠から自分自身が消える。
func (s AccessorySelector) recentlyStimulated(
	h History,
	byID map[ExerciseID]*Exercise,
	date Date,
) map[MuscleRegion]bool {
	cutoff := date.AddDays(-s.recoveryDays)
	blocked := map[MuscleRegion]bool{}

	for _, l := range h.OnOrAfter(cutoff).Before(date).Logs() {
		e, ok := byID[l.ExerciseID()]
		if !ok {
			continue
		}
		for _, r := range e.Stimulus().Regions() {
			blocked[r] = true
		}
	}
	return blocked
}

// pickForRegion はその区分を埋められる種目のうち、最後に使ってから
// 最も間隔が空いているものを選ぶ。これでバリエーションが自動で回る。
func (s AccessorySelector) pickForRegion(
	accessories []*Exercise,
	taken map[ExerciseID]bool,
	region MuscleRegion,
	h History,
	date Date,
) *Exercise {
	var best *Exercise
	bestDaysAgo := -1

	for _, e := range accessories {
		if taken[e.ID()] {
			continue
		}
		if _, ok := e.Stimulus().Contribution(region); !ok {
			continue
		}

		daysAgo := neverUsedDaysAgo
		if last, ok := h.LastPerformed(e.ID()); ok {
			daysAgo = date.DaysSince(last)
		}
		if daysAgo > bestDaysAgo {
			best = e
			bestDaysAgo = daysAgo
		}
	}
	return best
}

// topRegion は残差最大の筋区分。同値なら名前の昇順で決める（再現性のため）。
func topRegion(remaining map[MuscleRegion]float64) (MuscleRegion, bool) {
	regions := make([]MuscleRegion, 0, len(remaining))
	for r := range remaining {
		regions = append(regions, r)
	}
	if len(regions) == 0 {
		return "", false
	}

	sort.Slice(regions, func(i, j int) bool {
		if remaining[regions[i]] != remaining[regions[j]] {
			return remaining[regions[i]] > remaining[regions[j]]
		}
		return regions[i] < regions[j]
	})
	return regions[0], true
}
