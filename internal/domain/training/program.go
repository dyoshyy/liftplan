package training

import (
	"errors"
	"fmt"
	"sort"
)

// 週目標セット数の範囲。
//
// 上限は「1筋区分に週40セット」で、どんなプログラムでも過剰。
// 下限を正の数にするのは、0を設定するくらいなら区分ごと外すべきだから。
const (
	minWeeklySets = 0.5
	maxWeeklySets = 40
)

// WeeklyVolumeTarget は筋区分ごとの週あたり目標セット数。不変。
type WeeklyVolumeTarget struct {
	m map[MuscleRegion]float64
}

func NewWeeklyVolumeTarget(m map[MuscleRegion]float64) (WeeklyVolumeTarget, error) {
	if len(m) == 0 {
		return WeeklyVolumeTarget{}, errors.New("週目標が空である")
	}

	out := make(map[MuscleRegion]float64, len(m))
	for region, v := range m {
		if !region.Valid() {
			return WeeklyVolumeTarget{}, fmt.Errorf("未知の筋区分: %q", region)
		}
		q := quantize(v)
		name := fmt.Sprintf("筋区分 %s の目標セット数", region)
		if err := validateRange(name, q, minWeeklySets, maxWeeklySets); err != nil {
			return WeeklyVolumeTarget{}, err
		}
		out[region] = q
	}
	return WeeklyVolumeTarget{m: out}, nil
}

// Sets は設定されていない区分に対して0を返す。
//
// 0は「その区分を狙わない」という意味であり、エラーではない。
// 全21区分を必ず設定させると、シードを触りたくないユーザーが詰まる。
func (t WeeklyVolumeTarget) Sets(r MuscleRegion) float64 { return t.m[r] }

// Regions はソート済みの筋区分を返す。再現性のため順序を固定する。
func (t WeeklyVolumeTarget) Regions() []MuscleRegion {
	out := make([]MuscleRegion, 0, len(t.m))
	for r := range t.m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (t WeeklyVolumeTarget) IsEmpty() bool { return len(t.m) == 0 }

// Program はユーザーの設定を保持する集約ルート。
// 頻度・週目標・使う種目を一貫した単位で扱う。
type Program struct {
	frequency Frequency
	target    WeeklyVolumeTarget
	selected  []ExerciseID
}

func NewProgram(freq Frequency, target WeeklyVolumeTarget, selected []ExerciseID) (*Program, error) {
	if freq.IsZero() {
		return nil, errors.New("週の頻度が設定されていない")
	}
	if target.IsEmpty() {
		return nil, errors.New("週目標が設定されていない")
	}
	if len(selected) == 0 {
		return nil, errors.New("使用する種目が1つも選ばれていない")
	}

	seen := make(map[ExerciseID]bool, len(selected))
	copied := make([]ExerciseID, 0, len(selected))
	for _, raw := range selected {
		// 種目IDの検証は NewExerciseID に委ねる。ここで独自に判定すると、
		// 前後に空白のあるIDが通り、種目マスタのIDと永久に一致しなくなる。
		// 一致しないIDは黙って無視されるため、ユーザーが選んだ種目が
		// 理由の説明なく消える。
		id, err := NewExerciseID(string(raw))
		if err != nil {
			return nil, fmt.Errorf("選択された種目: %w", err)
		}
		if seen[id] {
			return nil, fmt.Errorf("種目が重複している: %s", id)
		}
		seen[id] = true
		copied = append(copied, id)
	}

	return &Program{frequency: freq, target: target, selected: copied}, nil
}

func (p *Program) Frequency() Frequency             { return p.frequency }
func (p *Program) WeeklyTarget() WeeklyVolumeTarget { return p.target }

func (p *Program) SelectedExercises() []ExerciseID {
	out := make([]ExerciseID, len(p.selected))
	copy(out, p.selected)
	return out
}

func (p *Program) Includes(id ExerciseID) bool {
	for _, v := range p.selected {
		if v == id {
			return true
		}
	}
	return false
}
