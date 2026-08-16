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
type ExerciseID string

func NewExerciseID(s string) (ExerciseID, error) {
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
//
// MainLift は空文字、DefaultRatioToMain は0を「未設定」として扱う。
type ExerciseParams struct {
	ID                 string
	Name               string
	Kind               ExerciseKind
	Stimulus           map[MuscleRegion]float64
	IncrementKg        float64
	MainLift           MainLift
	DefaultRatioToMain float64
}

// Exercise は種目エンティティ。同一性は ID で決まる。
//
//ddd:aggregate
type Exercise struct {
	id                 ExerciseID
	name               string
	kind               ExerciseKind
	stimulus           StimulusProfile
	increment          Increment
	mainLift           MainLift
	hasMainLift        bool
	defaultRatioToMain Ratio
	hasDefaultRatio    bool
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

	e := &Exercise{
		id:        id,
		name:      name,
		kind:      p.Kind,
		stimulus:  stimulus,
		increment: increment,
	}

	if p.MainLift != "" {
		if !p.MainLift.Valid() {
			return nil, fmt.Errorf("種目 %s のメインリフトが不正: %q", id, p.MainLift)
		}
		e.mainLift = p.MainLift
		e.hasMainLift = true
	}
	if p.DefaultRatioToMain != 0 {
		ratio, err := NewRatio(p.DefaultRatioToMain)
		if err != nil {
			return nil, fmt.Errorf("種目 %s: %w", id, err)
		}
		e.defaultRatioToMain = ratio
		e.hasDefaultRatio = true
	}

	if err := e.validateKindInvariants(); err != nil {
		return nil, err
	}
	return e, nil
}

// validateKindInvariants は種別ごとの不変条件を検査する。
//
// メインリフトを持たないメイン種目や、対メイン係数を持たないバリエーションが
// 生まれると、スロット割り当てのときに黙って無視される。
func (e *Exercise) validateKindInvariants() error {
	switch e.kind {
	case KindMain:
		if !e.hasMainLift {
			return fmt.Errorf("種目 %s: メイン種目はメインリフトを持つ必要がある", e.id)
		}
		if e.hasDefaultRatio {
			return fmt.Errorf("種目 %s: メイン種目は対メイン係数を持たない", e.id)
		}
	case KindVariation:
		if !e.hasMainLift {
			return fmt.Errorf("種目 %s: バリエーションは所属メインリフトを持つ必要がある", e.id)
		}
		if !e.hasDefaultRatio {
			return fmt.Errorf("種目 %s: バリエーションは対メイン係数の初期値を持つ必要がある", e.id)
		}
	case KindAccessory:
		if e.hasMainLift {
			return fmt.Errorf("種目 %s: 補助種目はメインリフトを持たない", e.id)
		}
		if e.hasDefaultRatio {
			return fmt.Errorf("種目 %s: 補助種目は対メイン係数を持たない", e.id)
		}
	}
	return nil
}

func (e *Exercise) ID() ExerciseID            { return e.id }
func (e *Exercise) Name() string              { return e.name }
func (e *Exercise) Kind() ExerciseKind        { return e.kind }
func (e *Exercise) Stimulus() StimulusProfile { return e.stimulus }
func (e *Exercise) Increment() Increment      { return e.increment }

func (e *Exercise) MainLift() (MainLift, bool) { return e.mainLift, e.hasMainLift }

func (e *Exercise) DefaultRatioToMain() (Ratio, bool) {
	return e.defaultRatioToMain, e.hasDefaultRatio
}

// SameIdentity はエンティティの同一性判定。値ではなく ID で比べる。
func (e *Exercise) SameIdentity(o *Exercise) bool {
	if e == nil || o == nil {
		return false
	}
	return e.id == o.id
}

// IsVariationOf はこの種目が指定のメインリフトのバリエーションか。
//
// メイン種目自身は false を返す。バリエーションのスロットにメイン自身が
// 候補として混ざると、差し替えたつもりで同じ種目が選ばれる。
func (e *Exercise) IsVariationOf(lift MainLift) bool {
	return e.kind == KindVariation && e.mainLift == lift
}
