package training

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// maxStimulusRegions は1種目が寄与できる筋区分の数の上限。
// これを超える種目は、寄与度の設定を誤っているとみなす。
const maxStimulusRegions = 8

// ExerciseID は種目の同一性。
// maxExerciseIDLen は種目IDの長さの上限。
//
// シードの最長は "incline_barbell_press" の21文字。ユーザーが自分で
// 追加する余地を見て64に置く。SetLogID と同じ上限。
const maxExerciseIDLen = 64

type ExerciseID string

func NewExerciseID(s string) (ExerciseID, error) {
	// 長さを先に見る。エラー文に入力を埋め込むので、上限が無いと
	// 20MB の ID が 20MB のエラー応答になって返る。
	if len(s) > maxExerciseIDLen {
		return "", fmt.Errorf("種目IDが長すぎる: %d文字（上限 %d）", len(s), maxExerciseIDLen)
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", errors.New("種目IDが空である")
	}
	if trimmed != s {
		return "", fmt.Errorf("種目IDの前後に空白がある: %q", s)
	}
	return ExerciseID(trimmed), nil
}

// StimulusProfile は種目が各筋区分へ与える刺激の分布。
//
// パッケージ外からは不変。ただし構造体の値コピーは内部マップを共有するため、
// パッケージ内で m に書き込むとエンティティの状態が壊れる。
// 消費者（残差計算・補助種目選択・セッション生成）は全て同じパッケージにいるので、
// この禁止は TestDomain_StimulusProfileIsNotMutated が機械的に検査する。
type StimulusProfile struct {
	m map[MuscleRegion]Contribution
}

func NewStimulusProfile(m map[MuscleRegion]float64) (StimulusProfile, error) {
	if len(m) == 0 {
		return StimulusProfile{}, errors.New("種目は少なくとも1つの筋区分に寄与する必要がある")
	}
	if len(m) > maxStimulusRegions {
		return StimulusProfile{}, fmt.Errorf(
			"1種目が寄与する筋区分は%d個までである必要がある: %d個", maxStimulusRegions, len(m))
	}

	out := make(map[MuscleRegion]Contribution, len(m))
	for region, v := range m {
		if !region.Valid() {
			return StimulusProfile{}, fmt.Errorf("未知の筋区分: %q", region)
		}
		c, err := NewContribution(v)
		if err != nil {
			return StimulusProfile{}, fmt.Errorf("筋区分 %s の寄与度が不正: %w", region, err)
		}
		out[region] = c
	}
	return StimulusProfile{m: out}, nil
}

// Regions はソート済みの筋区分を返す。
//
// マップの反復順に依存すると、同じ入力から違うセッションが生成され再現性が壊れる。
func (p StimulusProfile) Regions() []MuscleRegion {
	out := make([]MuscleRegion, 0, len(p.m))
	for r := range p.m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Contribution はその筋区分への寄与度。寄与しない区分では false を返す。
func (p StimulusProfile) Contribution(r MuscleRegion) (Contribution, bool) {
	c, ok := p.m[r]
	return c, ok
}

func (p StimulusProfile) IsEmpty() bool { return len(p.m) == 0 }

// ExerciseParams は Exercise の生成入力。
type ExerciseParams struct {
	ID               string
	Name             string
	Kind             ExerciseKind
	Stimulus         map[MuscleRegion]float64
	IncrementKg      float64
	BodyweightFactor float64
}

// Exercise は種目エンティティ。同一性は ID で決まる。
//
//ddd:aggregate
type Exercise struct {
	id               ExerciseID
	name             string
	kind             ExerciseKind
	stimulus         StimulusProfile
	increment        Increment
	bodyweightFactor BodyweightFactor
}

func NewExercise(p ExerciseParams) (*Exercise, error) {
	name := strings.TrimSpace(p.Name)

	id, err := NewExerciseID(p.ID)
	if err != nil {
		// ID が不正だと種目を特定する手がかりが消える。36種目のシードのうち
		// どれが壊れているのか分からないと直せないので、名前で補う。
		if name != "" {
			return nil, fmt.Errorf("種目 %q: %w", name, err)
		}
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("種目 %s の名前が空である", id)
	}
	if !p.Kind.Valid() {
		return nil, fmt.Errorf("種目 %s の種別が不正: %q", id, p.Kind)
	}
	stimulus, err := NewStimulusProfile(p.Stimulus)
	if err != nil {
		return nil, fmt.Errorf("種目 %s: %w", id, err)
	}
	increment, err := NewIncrement(p.IncrementKg)
	if err != nil {
		return nil, fmt.Errorf("種目 %s: %w", id, err)
	}
	bodyweightFactor, err := NewBodyweightFactor(p.BodyweightFactor)
	if err != nil {
		return nil, fmt.Errorf("種目 %s: %w", id, err)
	}

	e := &Exercise{
		id:               id,
		name:             name,
		kind:             p.Kind,
		stimulus:         stimulus,
		increment:        increment,
		bodyweightFactor: bodyweightFactor,
	}

	return e, nil
}

func (e *Exercise) ID() ExerciseID                     { return e.id }
func (e *Exercise) Name() string                       { return e.name }
func (e *Exercise) Kind() ExerciseKind                 { return e.kind }
func (e *Exercise) Stimulus() StimulusProfile          { return e.stimulus }
func (e *Exercise) Increment() Increment               { return e.increment }
func (e *Exercise) BodyweightFactor() BodyweightFactor { return e.bodyweightFactor }

// SameIdentity はエンティティの同一性判定。値ではなく ID で比べる。
func (e *Exercise) SameIdentity(o *Exercise) bool {
	if e == nil || o == nil {
		return false
	}
	return e.id == o.id
}
