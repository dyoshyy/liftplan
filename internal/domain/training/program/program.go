package program

import (
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
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
	m map[training.MuscleRegion]float64
}

func NewWeeklyVolumeTarget(m map[training.MuscleRegion]float64) (WeeklyVolumeTarget, error) {
	if len(m) == 0 {
		return WeeklyVolumeTarget{}, errors.New("週目標が空である")
	}

	out := make(map[training.MuscleRegion]float64, len(m))
	for region, v := range m {
		if !region.Valid() {
			return WeeklyVolumeTarget{}, fmt.Errorf("未知の筋区分: %q", region)
		}
		q := training.Quantize(v)
		name := fmt.Sprintf("筋区分 %s の目標セット数", region)
		if err := training.ValidateRange(name, q, minWeeklySets, maxWeeklySets); err != nil {
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
func (t WeeklyVolumeTarget) Sets(r training.MuscleRegion) float64 { return t.m[r] }

// Regions はソート済みの筋区分を返す。再現性のため順序を固定する。
func (t WeeklyVolumeTarget) Regions() []training.MuscleRegion {
	out := make([]training.MuscleRegion, 0, len(t.m))
	for r := range t.m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (t WeeklyVolumeTarget) IsEmpty() bool { return len(t.m) == 0 }

// Program はユーザーの設定を保持する集約ルート。
//
//ddd:aggregate
type Program struct {
	frequency Frequency
	target    WeeklyVolumeTarget
	selected  []exercise.ExerciseID // 実施可能な種目
	declared  []exercise.ExerciseID // 重量を伸ばしたい種目
	focus     exercise.ExerciseID   // 重点的に伸ばしたい種目。空なら指定なし
}

func NewProgram(freq Frequency, target WeeklyVolumeTarget, selected, declared []exercise.ExerciseID, focus exercise.ExerciseID) (*Program, error) {
	if freq.IsZero() {
		return nil, errors.New("週の頻度が設定されていない")
	}
	if target.IsEmpty() {
		return nil, errors.New("週目標が設定されていない")
	}
	if len(selected) == 0 {
		return nil, errors.New("使用する種目が1つも選ばれていない")
	}
	if len(declared) == 0 {
		return nil, ErrNoDeclaredExercise
	}

	selected, err := normalizeExerciseIDs(selected)
	if err != nil {
		return nil, fmt.Errorf("選択種目が不正: %w", err)
	}
	declared, err = normalizeExerciseIDs(declared)
	if err != nil {
		return nil, fmt.Errorf("伸ばしたい種目が不正: %w", err)
	}

	// declared ⊂ selected であることを確認する。
	for _, id := range declared {
		if !slices.Contains(selected, id) {
			return nil, fmt.Errorf("伸ばしたい種目 %q が選択種目に含まれていない", id)
		}
	}
	// 重点種目が伸ばしたい種目に含まれていることを確認する。
	//
	// declared ⊂ selected なので、これが通れば選択にも含まれる。宣言して
	// いない種目を重点にできると、「伸ばしたい種目の中でさらに重点」という
	// 意味が崩れる。
	//
	// 空は素通しする。指定なしが正当な既定値で、ここで NewExerciseID に
	// 渡すと全プログラムが「種目IDが空である」で落ちる。
	if focus != "" {
		id, err := exercise.NewExerciseID(string(focus))
		if err != nil {
			return nil, fmt.Errorf("重点種目が不正: %w", err)
		}
		if !slices.Contains(declared, id) {
			return nil, fmt.Errorf("重点種目 %q が伸ばしたい種目に含まれていない", id)
		}
		focus = id
	}

	return &Program{frequency: freq, target: target, selected: selected, declared: declared, focus: focus}, nil
}

// WithFocus は重点種目だけを差し替えた新しいプログラムを返す。元は変えない。
//
// 空の ID を渡すと「指定なし」に戻る。NewProgram が空を素通しするので、
// 解除のための分岐は要らない。
//
// NewProgram に委譲するのは、focus ⊂ declared の検証を2箇所に書かない
// ため。ここで自前に検査すると、片方だけ直したときに黙ってずれる。
func (p *Program) WithFocus(id exercise.ExerciseID) (*Program, error) {
	return NewProgram(p.frequency, p.target, p.SelectedExercises(), p.DeclaredExercises(), id)
}

// WithDeclared は伸ばしたい種目だけを差し替えた新しいプログラムを返す。
//
// 重点種目が新しい宣言に含まれなくなる場合はエラーになる。黙って解除は
// しない。解除するかどうかは本人が決めることで、宣言を変えた副作用として
// 重点が消えると、次に画面を開くまで気づけない。
func (p *Program) WithDeclared(ids []exercise.ExerciseID) (*Program, error) {
	return NewProgram(p.frequency, p.target, p.SelectedExercises(), ids, p.focus)
}

func (p *Program) Frequency() Frequency             { return p.frequency }
func (p *Program) WeeklyTarget() WeeklyVolumeTarget { return p.target }

// normalizeExerciseIDs は種目IDの正規化を行う。重複と存在しない種目はエラーになる。昇順にソートする。
func normalizeExerciseIDs(ids []exercise.ExerciseID) ([]exercise.ExerciseID, error) {
	seen := make(map[exercise.ExerciseID]bool, len(ids))
	out := make([]exercise.ExerciseID, 0, len(ids))
	for _, id := range ids {
		validID, err := exercise.NewExerciseID(string(id))
		if err != nil {
			return nil, err
		}
		if seen[validID] {
			return nil, fmt.Errorf("種目 %q が重複している", validID)
		}
		seen[validID] = true
		out = append(out, validID)
	}

	slices.Sort(out)
	return out, nil
}

func (p *Program) SelectedExercises() []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, len(p.selected))
	copy(out, p.selected)
	return out
}

func (p *Program) DeclaredExercises() []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, len(p.declared))
	copy(out, p.declared)
	return out
}

// FocusExercise は重点種目。指定が無ければ false。
//
// ポインタではなく (値, bool) を返すのは、Weight() と同じ
// 「任意項目」の規約に揃えるため。ポインタだと呼び出し側が nil 判定と
// 逆参照の2手を踏むうえ、集約の内部フィールドのアドレスが外へ出る。
func (p *Program) FocusExercise() (exercise.ExerciseID, bool) {
	return p.focus, p.focus != ""
}

func (p *Program) Includes(id exercise.ExerciseID) bool {
	return slices.Contains(p.selected, id)
}

func (p *Program) Declares(id exercise.ExerciseID) bool {
	return slices.Contains(p.declared, id)
}
