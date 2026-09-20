package program

import (
	"errors"
	"fmt"
	"slices"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// maxSplitName は分割の名前の長さの上限。画面の見出しに出るだけなので短い。
const maxSplitName = 20

// maxCycleLength は周期の長さの上限。
//
// 14 は「2週ぶん」。それより長い周期は、1つの分割が2週に1度しか来ない
// ことを意味する。回復の間隔としては空きすぎで、設定ミスの可能性が高い。
const maxCycleLength = 14

var (
	// ErrEmptySplitName は分割に名前が無いことを表す。
	ErrEmptySplitName = errors.New("分割に名前が無い")

	// ErrDuplicateRegion は同じ筋区分が1つの分割に二度入っていることを表す。
	ErrDuplicateRegion = errors.New("同じ筋区分が重複している")
)

// Split はその日に狙う筋区分の集合。
//
// 区分が空なら「全区分」。
type Split struct {
	name    string
	regions []training.MuscleRegion
}

// NewSplit は分割を組み立てる。区分は昇順に正規化する。
//
// 並びを固定するのは、同じ設定から同じ計画が出るようにするため。
func NewSplit(name string, regions []training.MuscleRegion) (Split, error) {
	if name == "" {
		return Split{}, ErrEmptySplitName
	}
	if len([]rune(name)) > maxSplitName {
		return Split{}, fmt.Errorf("分割の名前が長すぎる: %d文字", len([]rune(name)))
	}

	seen := make(map[training.MuscleRegion]bool, len(regions))
	out := make([]training.MuscleRegion, 0, len(regions))
	for _, r := range regions {
		if !r.Valid() {
			return Split{}, fmt.Errorf("筋区分が不正: %q", r)
		}
		if seen[r] {
			return Split{}, fmt.Errorf("%w: %s", ErrDuplicateRegion, r)
		}
		seen[r] = true
		out = append(out, r)
	}
	slices.Sort(out)

	return Split{name: name, regions: out}, nil
}

func (s Split) Name() string { return s.name }

func (s Split) Regions() []training.MuscleRegion {
	out := make([]training.MuscleRegion, len(s.regions))
	copy(out, s.regions)
	return out
}

// Includes はその筋区分を今日狙うか。
//
// 区分を1つも持たない分割は全区分を狙う。
func (s Split) Includes(r training.MuscleRegion) bool {
	if len(s.regions) == 0 {
		return true
	}
	return slices.Contains(s.regions, r)
}

func (s Split) IsZero() bool { return s.name == "" && len(s.regions) == 0 }

// normalizeCycle は周期を検証する。空なら分割なし。
func normalizeCycle(cycle []Split) ([]Split, error) {
	if len(cycle) == 0 {
		return nil, nil
	}
	if len(cycle) > maxCycleLength {
		return nil, fmt.Errorf("周期が長すぎる: %d日", len(cycle))
	}
	for _, s := range cycle {
		if s.IsZero() {
			return nil, errors.New("周期にゼロ値の分割がある")
		}
	}

	// 同じ分割の繰り返しを許す。本人の週は 上・下・上・下・上 で、
	// 上が3日ある。重複を弾くとこれが表せない。
	out := make([]Split, len(cycle))
	copy(out, cycle)
	return out, nil
}
