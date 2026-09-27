package exercise

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// maxStimulusRegions は1種目が寄与できる筋区分の数の上限。
// これを超える種目は、寄与度の設定を誤っているとみなす。
const maxStimulusRegions = 8

// maxNameRunes は種目名の上限（rune 数）。エラー文や画面に出るので置く。
const maxNameRunes = 40

// CustomExerciseIDPrefix は利用者が足した種目の ID の接頭辞。
//
// シードの ID は英小文字と "_" だけなので、この接頭辞とは衝突しない
// （seed の TestExercises_NoIDUsesTheCustomPrefix が守る）。
const CustomExerciseIDPrefix = "u-"

// primaryContribution は寄与1.0を表す値。hasFullContribution が、この値の
// 区分が1つ以上あるかを見る。
const primaryContribution = 1.0

// minStimulusContribution は1区分あたりの寄与度の下限
// （設計書「各区分 0.1〜1.0」）。training.NewContribution は SmallestPositive
// まで通してしまうので、ここでエクササイズ側の規則として下限を課す。
// シードの最小は 0.2（barbell_row の RearDelt）なので、この下限はシードを壊さない。
const minStimulusContribution = 0.1

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
	m map[training.MuscleRegion]training.Contribution
}

func NewStimulusProfile(m map[training.MuscleRegion]float64) (StimulusProfile, error) {
	if len(m) == 0 {
		return StimulusProfile{}, errors.New("種目は少なくとも1つの筋区分に寄与する必要がある")
	}
	if len(m) > maxStimulusRegions {
		return StimulusProfile{}, fmt.Errorf(
			"1種目が寄与する筋区分は%d個までである必要がある: %d個", maxStimulusRegions, len(m))
	}

	out := make(map[training.MuscleRegion]training.Contribution, len(m))
	for region, v := range m {
		if !region.Valid() {
			return StimulusProfile{}, fmt.Errorf("未知の筋区分: %q", region)
		}
		c, err := training.NewContribution(v)
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
func (p StimulusProfile) Regions() []training.MuscleRegion {
	out := make([]training.MuscleRegion, 0, len(p.m))
	for r := range p.m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Contribution はその筋区分への寄与度。寄与しない区分では false を返す。
func (p StimulusProfile) Contribution(r training.MuscleRegion) (training.Contribution, bool) {
	c, ok := p.m[r]
	return c, ok
}

func (p StimulusProfile) IsEmpty() bool { return len(p.m) == 0 }

// hasFullContribution は寄与1.0の区分が1つ以上あるかを返す。
//
// 分割のどの日にも入らない種目（planning の isPrimaryIn）を防ぐため、
// 全種目に課す。プリセットもユーザーが足す種目も同じ規則で通す。
func (p StimulusProfile) hasFullContribution() bool {
	for _, c := range p.m {
		if c.Float() == primaryContribution {
			return true
		}
	}
	return false
}

// belowFloor は minStimulusContribution を下回る区分があれば、その区分と
// 値を返す。Regions() の順（ソート済み）で見るのは、複数の区分が下限を
// 下回ったときにエラー文が実行のたびに変わらないようにするため。
func (p StimulusProfile) belowFloor(min float64) (training.MuscleRegion, training.Contribution, bool) {
	for _, r := range p.Regions() {
		if c := p.m[r]; c.Float() < min {
			return r, c, true
		}
	}
	return "", training.Contribution{}, false
}

// ExerciseParams は Exercise の生成入力。
type ExerciseParams struct {
	ID               string
	Name             string
	Stimulus         map[training.MuscleRegion]float64
	IncrementKg      float64
	BodyweightFactor float64
	DerivedFrom      string
}

// Exercise は種目エンティティ。同一性は ID で決まる。
//
//ddd:aggregate
type Exercise struct {
	id               ExerciseID
	name             string
	stimulus         StimulusProfile
	increment        training.Increment
	bodyweightFactor training.BodyweightFactor
	derivedFrom      ExerciseID
	hasDerivedFrom   bool
	// deleted は消したか。消した種目も記録と履歴の名前のために残る。
	deleted bool
}

func NewExercise(p ExerciseParams) (*Exercise, error) {
	name := strings.TrimSpace(p.Name)

	id, err := NewExerciseID(p.ID)
	if err != nil {
		// ID が不正だと種目を特定する手がかりが消える。シードのうち
		// どれが壊れているのか分からないと直せないので、名前で補う。
		if name != "" {
			return nil, fmt.Errorf("種目 %q: %w", name, err)
		}
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("種目 %s の名前が空である", id)
	}
	if n := utf8.RuneCountInString(name); n > maxNameRunes {
		return nil, fmt.Errorf("種目 %s: 名前が長すぎる: %d文字（上限 %d）", id, n, maxNameRunes)
	}
	stimulus, err := NewStimulusProfile(p.Stimulus)
	if err != nil {
		return nil, fmt.Errorf("種目 %s: %w", id, err)
	}
	if !stimulus.hasFullContribution() {
		return nil, fmt.Errorf("種目 %s: 寄与1.0の区分が1つも無い", id)
	}
	if r, c, ok := stimulus.belowFloor(minStimulusContribution); ok {
		return nil, fmt.Errorf(
			"種目 %s: 筋区分 %s の寄与度が下限を下回る: %v（下限 %v）",
			id, r, c.Float(), minStimulusContribution)
	}
	increment, err := training.NewIncrement(p.IncrementKg)
	if err != nil {
		return nil, fmt.Errorf("種目 %s: %w", id, err)
	}
	bodyweightFactor, err := training.NewBodyweightFactor(p.BodyweightFactor)
	if err != nil {
		return nil, fmt.Errorf("種目 %s: %w", id, err)
	}
	var derivedFrom ExerciseID
	hasDerivedFrom := p.DerivedFrom != ""
	if hasDerivedFrom {
		derivedFrom, err = NewExerciseID(p.DerivedFrom)
		if err != nil {
			return nil, fmt.Errorf("種目 %s: 派生元の種目IDが不正: %w", id, err)
		}

		if derivedFrom == id {
			return nil, fmt.Errorf("種目 %s: 派生元の種目IDが自分自身である", id)
		}
	}

	e := &Exercise{
		id:               id,
		name:             name,
		stimulus:         stimulus,
		increment:        increment,
		bodyweightFactor: bodyweightFactor,
		derivedFrom:      derivedFrom,
		hasDerivedFrom:   hasDerivedFrom,
	}

	return e, nil
}

func (e *Exercise) ID() ExerciseID                              { return e.id }
func (e *Exercise) Name() string                                { return e.name }
func (e *Exercise) Stimulus() StimulusProfile                   { return e.stimulus }
func (e *Exercise) Increment() training.Increment               { return e.increment }
func (e *Exercise) BodyweightFactor() training.BodyweightFactor { return e.bodyweightFactor }
func (e *Exercise) DerivedFrom() (ExerciseID, bool)             { return e.derivedFrom, e.hasDerivedFrom }

// SameIdentity はエンティティの同一性判定。値ではなく ID で比べる。
func (e *Exercise) SameIdentity(o *Exercise) bool {
	if e == nil || o == nil {
		return false
	}
	return e.id == o.id
}

// IsDeleted は消した種目かを返す。
func (e *Exercise) IsDeleted() bool { return e.deleted }

// Delete は消した状態の新しい種目を返す。元は変えない。
func (e *Exercise) Delete() *Exercise {
	c := *e
	c.deleted = true
	return &c
}

// ExerciseEdit は種目を直す入力。直せるのは名前・効き方・刻みだけで、
// ID・自重係数・派生元は元から引き継ぐ（プリセット由来かどうかで扱いを
// 変えないため。docs/specs/2026-09-26-custom-exercises-design.md
// 「直した値を返すメソッドを1つにまとめ」）。
type ExerciseEdit struct {
	Name        string
	Stimulus    map[training.MuscleRegion]float64
	IncrementKg float64
}

// Edit は名前・効き方・刻みを差し替えた新しい値を返す。元は変えない。
// ID・自重係数・派生元・deleted は引き継ぐ。検証は NewExercise と同じ
// （プリセット由来もユーザーが足した種目も同じ規則で通す）。
func (e *Exercise) Edit(p ExerciseEdit) (*Exercise, error) {
	params := ExerciseParams{
		ID:               string(e.id),
		Name:             p.Name,
		Stimulus:         p.Stimulus,
		IncrementKg:      p.IncrementKg,
		BodyweightFactor: e.bodyweightFactor.Float(),
	}
	if e.hasDerivedFrom {
		params.DerivedFrom = string(e.derivedFrom)
	}
	edited, err := NewExercise(params)
	if err != nil {
		return nil, err
	}
	edited.deleted = e.deleted
	return edited, nil
}

// NewRandomExerciseID は種目の ID を採番する。
//
// サーバーが採番するのは、種目を足すのが設定画面で、圏外で足す必要が
// 無いから。二度押しは名前の重複で止まる。
func NewRandomExerciseID() (ExerciseID, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("乱数を取得できない: %w", err)
	}
	return ExerciseID(CustomExerciseIDPrefix + hex.EncodeToString(b[:])), nil
}
