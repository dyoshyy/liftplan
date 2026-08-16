# liftplan サーバー ドメイン層 第3部 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** スロット配分・残差計算・補助種目選択・コンディション補正・デロード判定を実装し、それらを束ねる `SessionPlanner` を完成させる。

**前提:** `01-domain.md`（Task 1〜5）と `02-domain-model.md`（Task 6〜10）が完了していること。

**Global Constraints:** `01-domain.md` の Global Constraints をすべて引き継ぐ。

---

### Task 11: Frequency と SlotCatalog

**Files:**
- Create: `internal/domain/training/slot.go`
- Test: `internal/domain/training/slot_test.go`

**Interfaces:**
- Consumes: Task 4 の `IntensityPct` / `SetCount` / `RIR`
- Produces:
  - `type Frequency struct{...}` / `func NewFrequency(perWeek int) (Frequency, error)` / `func (f Frequency) PerWeek() int`
  - `type SlotRole string`（`RoleVariation` / `RoleStandard` / `RoleHeavy`）と `Valid()`
  - `type SlotTemplate struct{...}` / `func (s SlotTemplate) Role() SlotRole` / `Intensity() IntensityPct` / `Sets() SetCount` / `TargetRIR() RIR`
  - `type SlotCatalog struct{}` / `func NewSlotCatalog() SlotCatalog` / `func (c SlotCatalog) For(f Frequency) []SlotTemplate` / `func (c SlotCatalog) Select(f Frequency, sessionIndex int) SlotTemplate`

**なぜこの数値か:** 現行のベンチ（1RM 約105kg想定で 80 / 85 / 90〜95kg）は概ね 76% / 81% / 88% に対応する。**RIR は調整ダイヤルではなくガードレール**なので2で固定し、高強度スロットのみ1とする。動かすのはスロットごとの強度帯とバリエーションの有無。

`Select` は `sessionIndex` を要素数で剰余するため、週の頻度を超えて回しても破綻しない。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/slot_test.go`:

```go
package training_test

import (
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mustFrequency(t *testing.T, n int) training.Frequency {
	t.Helper()
	f, err := training.NewFrequency(n)
	if err != nil {
		t.Fatalf("NewFrequency(%d): %v", n, err)
	}
	return f
}

func TestNewFrequency_Range(t *testing.T) {
	if _, err := training.NewFrequency(0); err == nil {
		t.Error("週0回が通ってしまう")
	}
	if _, err := training.NewFrequency(8); err == nil {
		t.Error("週8回が通ってしまう")
	}
	if f := mustFrequency(t, 3); f.PerWeek() != 3 {
		t.Errorf("PerWeek が誤り: %d", f.PerWeek())
	}
}

func TestSlotCatalog_ThreeTimesPerWeek(t *testing.T) {
	c := training.NewSlotCatalog()
	got := c.For(mustFrequency(t, 3))
	if len(got) != 3 {
		t.Fatalf("スロット数が誤り: %d", len(got))
	}
	want := []training.SlotRole{
		training.RoleVariation, training.RoleStandard, training.RoleHeavy,
	}
	for i, w := range want {
		if got[i].Role() != w {
			t.Errorf("%d番目の役割が誤り: got %s, want %s", i, got[i].Role(), w)
		}
	}
}

func TestSlotCatalog_IntensityAscends(t *testing.T) {
	c := training.NewSlotCatalog()
	for freq := 1; freq <= 7; freq++ {
		slots := c.For(mustFrequency(t, freq))
		for i := 1; i < len(slots); i++ {
			if slots[i-1].Intensity().Float() > slots[i].Intensity().Float() {
				t.Errorf("週%d回: 強度が昇順でない", freq)
			}
		}
	}
}

func TestSlotCatalog_OnlyHeavyHasRirOne(t *testing.T) {
	c := training.NewSlotCatalog()
	for _, s := range c.For(mustFrequency(t, 3)) {
		want := 2
		if s.Role() == training.RoleHeavy {
			want = 1
		}
		if s.TargetRIR().Int() != want {
			t.Errorf("%s の目標RIRが誤り: got %d, want %d", s.Role(), s.TargetRIR().Int(), want)
		}
	}
}

func TestSlotCatalog_SingleSessionIsStandard(t *testing.T) {
	c := training.NewSlotCatalog()
	slots := c.For(mustFrequency(t, 1))
	if len(slots) != 1 || slots[0].Role() != training.RoleStandard {
		t.Errorf("週1回の構成が誤り: %v", slots)
	}
}

func TestSlotCatalog_HighFrequencyReusesFourSlots(t *testing.T) {
	c := training.NewSlotCatalog()
	for _, freq := range []int{4, 5, 6, 7} {
		if got := len(c.For(mustFrequency(t, freq))); got != 4 {
			t.Errorf("週%d回のスロット数が誤り: %d", freq, got)
		}
	}
}

func TestSlotCatalog_SelectWrapsAround(t *testing.T) {
	c := training.NewSlotCatalog()
	f := mustFrequency(t, 3)
	first := c.Select(f, 0)
	if got := c.Select(f, 3); got.Role() != first.Role() {
		t.Errorf("剰余で巡回していない: got %s, want %s", got.Role(), first.Role())
	}
	if got := c.Select(f, -1); got.Role() != first.Role() {
		t.Errorf("負のインデックスが先頭に丸められていない: %s", got.Role())
	}
}

func TestSlotCatalog_ValuesAreRealistic(t *testing.T) {
	c := training.NewSlotCatalog()
	for freq := 1; freq <= 7; freq++ {
		for _, s := range c.For(mustFrequency(t, freq)) {
			if v := s.Intensity().Float(); v < 0.70 || v > 0.92 {
				t.Errorf("強度が範囲外: %v", v)
			}
			if n := s.Sets().Int(); n < 1 || n > 6 {
				t.Errorf("セット数が範囲外: %d", n)
			}
			if !s.Role().Valid() {
				t.Errorf("役割が不正: %s", s.Role())
			}
		}
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'Frequency|SlotCatalog'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/slot.go`:

```go
package training

import "fmt"

const maxFrequencyPerWeek = 7

// Frequency は週あたりのトレーニング回数。
type Frequency struct {
	perWeek int
}

func NewFrequency(perWeek int) (Frequency, error) {
	if perWeek < 1 || perWeek > maxFrequencyPerWeek {
		return Frequency{}, fmt.Errorf("週の頻度は1〜%d回である必要がある: %d", maxFrequencyPerWeek, perWeek)
	}
	return Frequency{perWeek: perWeek}, nil
}

func (f Frequency) PerWeek() int { return f.perWeek }

// SlotRole は週内スロットの役割。
//
//	RoleVariation … バリエーション種目で技術と弱点を突く日
//	RoleStandard  … 通常フォームでボリュームを積む日
//	RoleHeavy     … 高強度で神経系に効かせる日
type SlotRole string

const (
	RoleVariation SlotRole = "VARIATION"
	RoleStandard  SlotRole = "STANDARD"
	RoleHeavy     SlotRole = "HEAVY"
)

func (r SlotRole) Valid() bool {
	switch r {
	case RoleVariation, RoleStandard, RoleHeavy:
		return true
	}
	return false
}

// SlotTemplate は1スロットの設計。不変。
//
// TargetRIR は止め時の指示であり、レップ数は指示しない。
// レップ数はその日の状態が決める。
type SlotTemplate struct {
	role      SlotRole
	intensity IntensityPct
	sets      SetCount
	targetRIR RIR
}

func (s SlotTemplate) Role() SlotRole          { return s.role }
func (s SlotTemplate) Intensity() IntensityPct { return s.intensity }
func (s SlotTemplate) Sets() SetCount          { return s.sets }
func (s SlotTemplate) TargetRIR() RIR          { return s.targetRIR }

// mustSlot はカタログ定義用。定数のみを渡すため、失敗はプログラムの誤りとして panic する。
func mustSlot(role SlotRole, intensity float64, sets, rir int) SlotTemplate {
	i, err := NewIntensityPct(intensity)
	if err != nil {
		panic(fmt.Sprintf("スロット定義が不正: %v", err))
	}
	s, err := NewSetCount(sets)
	if err != nil {
		panic(fmt.Sprintf("スロット定義が不正: %v", err))
	}
	r, err := NewRIR(rir)
	if err != nil {
		panic(fmt.Sprintf("スロット定義が不正: %v", err))
	}
	return SlotTemplate{role: role, intensity: i, sets: s, targetRIR: r}
}

// SlotCatalog は週の頻度に対する強度配分を持つドメインサービス。無状態。
type SlotCatalog struct{}

func NewSlotCatalog() SlotCatalog { return SlotCatalog{} }

// For は週の頻度に応じたスロット構成を返す。
//
// 現行のベンチ（1RM 105kg想定で 80 / 85 / 90〜95kg）が概ね 76% / 81% / 88% に
// 対応する。週4回以上は4スロット構成を巡回させる。
func (c SlotCatalog) For(f Frequency) []SlotTemplate {
	switch f.PerWeek() {
	case 1:
		return []SlotTemplate{
			mustSlot(RoleStandard, 0.81, 4, 2),
		}
	case 2:
		return []SlotTemplate{
			mustSlot(RoleStandard, 0.81, 4, 2),
			mustSlot(RoleHeavy, 0.88, 3, 1),
		}
	case 3:
		return []SlotTemplate{
			mustSlot(RoleVariation, 0.76, 4, 2),
			mustSlot(RoleStandard, 0.81, 4, 2),
			mustSlot(RoleHeavy, 0.88, 3, 1),
		}
	default:
		return []SlotTemplate{
			mustSlot(RoleVariation, 0.76, 4, 2),
			mustSlot(RoleVariation, 0.78, 4, 2),
			mustSlot(RoleStandard, 0.81, 4, 2),
			mustSlot(RoleHeavy, 0.88, 3, 1),
		}
	}
}

// Select は週内 sessionIndex 本目のスロットを返す。
// 要素数で剰余を取るため、頻度を超えて回っても破綻しない。
func (c SlotCatalog) Select(f Frequency, sessionIndex int) SlotTemplate {
	slots := c.For(f)
	if sessionIndex < 0 {
		sessionIndex = 0
	}
	return slots[sessionIndex%len(slots)]
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'Frequency|SlotCatalog'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/slot.go internal/domain/training/slot_test.go
git commit -m "feat(domain): スロットカタログと週頻度を追加する"
```

---

### Task 12: Program 集約と残差計算

**Files:**
- Create: `internal/domain/training/program.go`
- Create: `internal/domain/training/residual.go`
- Test: `internal/domain/training/program_test.go`
- Test: `internal/domain/training/residual_test.go`

**Interfaces:**
- Consumes: Task 3 の `MuscleRegion`、Task 6 の `ExerciseID`、Task 11 の `Frequency`
- Produces:
  - `type WeeklyVolumeTarget struct{...}` / `func NewWeeklyVolumeTarget(m map[MuscleRegion]float64) (WeeklyVolumeTarget, error)` / `func (t WeeklyVolumeTarget) Sets(r MuscleRegion) float64` / `func (t WeeklyVolumeTarget) Regions() []MuscleRegion` / `func (t WeeklyVolumeTarget) PerSession(f Frequency) WeeklyVolumeTarget`
  - `type Program struct{...}` / `func NewProgram(freq Frequency, target WeeklyVolumeTarget, selected []ExerciseID) (*Program, error)` / `func (p *Program) Frequency() Frequency` / `func (p *Program) WeeklyTarget() WeeklyVolumeTarget` / `func (p *Program) SelectedExercises() []ExerciseID` / `func (p *Program) Includes(id ExerciseID) bool`
  - `type PlannedSet` は Task 14 で定義するため、ここでは残差の入力を `map[MuscleRegion]float64` として受ける薄い関数にする:
    - `type StimulusCoverage map[MuscleRegion]float64`
    - `func (c StimulusCoverage) Add(p StimulusProfile, sets int)`
    - `func Residual(target WeeklyVolumeTarget, coverage StimulusCoverage) map[MuscleRegion]float64`

`Program` は集約ルート。ユーザーの設定（頻度・週目標・使う種目）を一貫した単位で保持する。

`Residual` はメインが埋めた分を差し引いた不足量。**補助種目の選択を「自分で設計する」から「メインの残差を解く」に変える**ための土台。0以下の区分は落とす。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/program_test.go`:

```go
package training_test

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func simpleTarget(t *testing.T) training.WeeklyVolumeTarget {
	t.Helper()
	target, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid:   12,
		training.ChestUpper: 9,
	})
	if err != nil {
		t.Fatalf("週目標の生成に失敗: %v", err)
	}
	return target
}

func TestNewWeeklyVolumeTarget_RejectsEmpty(t *testing.T) {
	if _, err := training.NewWeeklyVolumeTarget(nil); err == nil {
		t.Error("空の週目標が通ってしまう")
	}
}

func TestNewWeeklyVolumeTarget_RejectsInvalid(t *testing.T) {
	if _, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid: 0,
	}); err == nil {
		t.Error("0セットの目標が通ってしまう")
	}
	if _, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.MuscleRegion("NOPE"): 10,
	}); err == nil {
		t.Error("未知の筋区分が通ってしまう")
	}
}

func TestWeeklyVolumeTarget_RegionsAreSorted(t *testing.T) {
	regions := simpleTarget(t).Regions()
	for i := 1; i < len(regions); i++ {
		if regions[i-1] >= regions[i] {
			t.Fatalf("ソートされていない: %v", regions)
		}
	}
}

func TestWeeklyVolumeTarget_PerSession(t *testing.T) {
	per := simpleTarget(t).PerSession(mustFrequency(t, 3))
	if got := per.Sets(training.ChestMid); math.Abs(got-4) > 1e-9 {
		t.Errorf("1セッションあたりの目標が誤り: %v", got)
	}
}

func TestWeeklyVolumeTarget_UnknownRegionIsZero(t *testing.T) {
	if got := simpleTarget(t).Sets(training.Calf); got != 0 {
		t.Errorf("未設定の区分が0でない: %v", got)
	}
}

func TestNewProgram(t *testing.T) {
	p, err := training.NewProgram(mustFrequency(t, 3), simpleTarget(t), []training.ExerciseID{"bench", "squat"})
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	if !p.Includes("bench") {
		t.Error("選択した種目が含まれていない")
	}
	if p.Includes("deadlift") {
		t.Error("選択していない種目が含まれている")
	}
	if p.Frequency().PerWeek() != 3 {
		t.Errorf("頻度が誤り: %d", p.Frequency().PerWeek())
	}
}

func TestNewProgram_RejectsEmptySelection(t *testing.T) {
	if _, err := training.NewProgram(mustFrequency(t, 3), simpleTarget(t), nil); err == nil {
		t.Error("種目を1つも選んでいないプログラムが通ってしまう")
	}
}

func TestNewProgram_RejectsDuplicateSelection(t *testing.T) {
	_, err := training.NewProgram(mustFrequency(t, 3), simpleTarget(t),
		[]training.ExerciseID{"bench", "bench"})
	if err == nil {
		t.Error("重複した種目選択が通ってしまう")
	}
}

func TestProgram_SelectedExercisesIsACopy(t *testing.T) {
	p, _ := training.NewProgram(mustFrequency(t, 3), simpleTarget(t), []training.ExerciseID{"bench"})
	got := p.SelectedExercises()
	got[0] = "tampered"
	if !p.Includes("bench") {
		t.Error("返り値の書き換えが集約に波及している")
	}
}
```

`internal/domain/training/residual_test.go`:

```go
package training_test

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestStimulusCoverage_Add(t *testing.T) {
	bench, err := training.NewExercise(benchParams())
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	coverage := training.StimulusCoverage{}
	coverage.Add(bench.Stimulus(), 4)

	if got := coverage[training.ChestMid]; math.Abs(got-4) > 1e-9 {
		t.Errorf("大胸筋中部のカバレッジが誤り: %v", got)
	}
	if got := coverage[training.TricepsLateral]; math.Abs(got-2) > 1e-9 {
		t.Errorf("三頭のカバレッジが誤り: %v", got)
	}
}

func TestResidual_SubtractsCoverage(t *testing.T) {
	target, _ := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid:   12,
		training.ChestUpper: 8,
	})
	coverage := training.StimulusCoverage{training.ChestMid: 4}

	got := training.Residual(target, coverage)
	if math.Abs(got[training.ChestMid]-8) > 1e-9 {
		t.Errorf("残差が誤り: %v", got[training.ChestMid])
	}
	if math.Abs(got[training.ChestUpper]-8) > 1e-9 {
		t.Errorf("カバーされていない区分の残差が誤り: %v", got[training.ChestUpper])
	}
}

func TestResidual_DropsSatisfiedRegions(t *testing.T) {
	target, _ := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid: 4,
	})
	coverage := training.StimulusCoverage{training.ChestMid: 6}

	if got := training.Residual(target, coverage); len(got) != 0 {
		t.Errorf("目標を超えた区分が残差に含まれている: %v", got)
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'Program|WeeklyVolumeTarget|Residual|StimulusCoverage'`
Expected: コンパイルエラー

- [ ] **Step 3: Program 集約を実装する**

`internal/domain/training/program.go`:

```go
package training

import (
	"errors"
	"fmt"
	"math"
	"sort"
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
			return WeeklyVolumeTarget{}, fmt.Errorf("未知の筋区分: %s", region)
		}
		if math.IsNaN(v) || v <= 0 {
			return WeeklyVolumeTarget{}, fmt.Errorf("筋区分 %s の目標セット数が不正: %v", region, v)
		}
		out[region] = v
	}
	return WeeklyVolumeTarget{m: out}, nil
}

// Sets は設定されていない区分に対して0を返す。
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

// PerSession は週目標を頻度で割った、1セッションあたりの目標。
func (t WeeklyVolumeTarget) PerSession(f Frequency) WeeklyVolumeTarget {
	out := make(map[MuscleRegion]float64, len(t.m))
	for r, v := range t.m {
		out[r] = v / float64(f.PerWeek())
	}
	return WeeklyVolumeTarget{m: out}
}

// Program はユーザーの設定を保持する集約ルート。
// 頻度・週目標・使う種目を一貫した単位で扱う。
type Program struct {
	frequency Frequency
	target    WeeklyVolumeTarget
	selected  []ExerciseID
}

func NewProgram(freq Frequency, target WeeklyVolumeTarget, selected []ExerciseID) (*Program, error) {
	if len(selected) == 0 {
		return nil, errors.New("使用する種目が1つも選ばれていない")
	}
	if len(target.m) == 0 {
		return nil, errors.New("週目標が設定されていない")
	}

	seen := map[ExerciseID]bool{}
	copied := make([]ExerciseID, 0, len(selected))
	for _, id := range selected {
		if id == "" {
			return nil, errors.New("空の種目IDが含まれている")
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
```

- [ ] **Step 4: 残差計算を実装する**

`internal/domain/training/residual.go`:

```go
package training

// StimulusCoverage は各筋区分がすでに何セット分埋まっているか。
type StimulusCoverage map[MuscleRegion]float64

// Add は種目を sets セット実施したときの刺激を積み上げる。
func (c StimulusCoverage) Add(p StimulusProfile, sets int) {
	for _, region := range p.Regions() {
		contribution, ok := p.Contribution(region)
		if !ok {
			continue
		}
		c[region] += contribution.Float() * float64(sets)
	}
}

// Residual は目標に対して埋まっていない分。0以下の区分は落とす。
//
// 補助種目の選択を「自分で設計する」から「メインの残差を解く」に変えるための土台。
func Residual(target WeeklyVolumeTarget, coverage StimulusCoverage) map[MuscleRegion]float64 {
	out := map[MuscleRegion]float64{}
	for _, region := range target.Regions() {
		gap := target.Sets(region) - coverage[region]
		if gap > 0 {
			out[region] = gap
		}
	}
	return out
}
```

- [ ] **Step 5: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'Program|WeeklyVolumeTarget|Residual|StimulusCoverage'`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/domain/training/program.go internal/domain/training/residual.go internal/domain/training/program_test.go internal/domain/training/residual_test.go
git commit -m "feat(domain): Program 集約と残差計算を追加する"
```

---

### Task 13: AccessorySelector（補助種目の割り当て）

**Files:**
- Create: `internal/domain/training/accessory_selector.go`
- Test: `internal/domain/training/accessory_selector_test.go`

**Interfaces:**
- Consumes: Task 6 の `Exercise`、Task 8 の `History`、Task 12 の残差
- Produces:
  - `type AccessorySelector struct{...}`
  - `func NewAccessorySelector(recoveryDays, setsPerAccessory int) (AccessorySelector, error)`
  - `func DefaultAccessorySelector() AccessorySelector` — recoveryDays 2、setsPerAccessory 3
  - `func (s AccessorySelector) Select(residual map[MuscleRegion]float64, pool []*Exercise, h History, date Date) []ExerciseID` — スロット数は残差から導く

**補助スロット数は残差から決める（重要）:**

固定の3スロットでは、週目標を構造的に達成できない。シードの週目標は21区分合計で198セット相当だが、3スロット×3セット×寄与合計では1セッションあたり15セット相当が上限で、頻度4でも週合計85%程度にしか届かない。

`slots` を固定値ではなく、次のように残差から導くこと。

```
必要スロット数 = ceil(残差の合計 / setsPerAccessory)
実際のスロット数 = min(必要スロット数, maxAccessorySlots)
```

`maxAccessorySlots` はセッションの長さの上限（8程度）。これで、残差が小さい日は短く、大きい日は長くなる。

**アルゴリズム:**

1. `recoveryDays` 日以内に刺激された筋区分を候補から外す（48時間ルール）
2. 残差の大きい区分から貪欲に選ぶ。同値なら筋区分名の昇順で決める（再現性のため）
3. 同じ区分を狙う種目が複数あれば、最後に使ってから最も間隔が空いているものを選ぶ。これでバリエーションが自動で回る
4. 選んだ種目の寄与分を残差から差し引き、0以下になった区分は落とす

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/accessory_selector_test.go`:

```go
package training_test

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mkAccessory(t *testing.T, id string, stimulus map[training.MuscleRegion]float64) *training.Exercise {
	t.Helper()
	e, err := training.NewExercise(training.ExerciseParams{
		ID:          id,
		Name:        id,
		Kind:        training.KindAccessory,
		Stimulus:    stimulus,
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("補助種目 %s の生成に失敗: %v", id, err)
	}
	return e
}

func accessoryPool(t *testing.T) []*training.Exercise {
	t.Helper()
	return []*training.Exercise{
		mkAccessory(t, "incline", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "incline_db", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "dip", map[training.MuscleRegion]float64{training.ChestLower: 1.0}),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
	}
}

func today() training.Date { return training.NewDate(2026, time.August, 16) }

func TestAccessorySelector_PicksLargestResidualFirst(t *testing.T) {
	s := training.DefaultAccessorySelector()
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 8, training.Biceps: 2},
		accessoryPool(t), training.NewHistory(nil), today(), 1,
	)
	if len(got) != 1 || got[0] != training.ExerciseID("incline") {
		t.Errorf("残差最大の区分が選ばれていない: %v", got)
	}
}

func TestAccessorySelector_SkipsRecentlyStimulatedRegion(t *testing.T) {
	s := training.DefaultAccessorySelector()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "recent", 15, "incline", 30, 10, 2),
	})
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 8, training.Biceps: 2},
		accessoryPool(t), h, today(), 1,
	)
	if len(got) != 1 || got[0] != training.ExerciseID("curl") {
		t.Errorf("48時間以内に刺激済みの区分が選ばれている: %v", got)
	}
}

func TestAccessorySelector_PrefersLeastRecentlyUsed(t *testing.T) {
	s := training.DefaultAccessorySelector()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "incline", 30, 10, 2),
		mkLog(t, "b", 1, "incline_db", 30, 10, 2),
	})
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 8},
		accessoryPool(t), h, today(), 1,
	)
	if len(got) != 1 || got[0] != training.ExerciseID("incline_db") {
		t.Errorf("間隔が空いている種目が選ばれていない: %v", got)
	}
}

func TestAccessorySelector_NoDuplicatesInOneSession(t *testing.T) {
	s := training.DefaultAccessorySelector()
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 30},
		accessoryPool(t), training.NewHistory(nil), today(), 3,
	)
	seen := map[training.ExerciseID]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("同じ種目が二度選ばれた: %v", got)
		}
		seen[id] = true
	}
}

func TestAccessorySelector_EmptyResidual(t *testing.T) {
	s := training.DefaultAccessorySelector()
	if got := s.Select(nil, accessoryPool(t), training.NewHistory(nil), today(), 3); len(got) != 0 {
		t.Errorf("残差が無いのに選ばれた: %v", got)
	}
}

func TestAccessorySelector_IgnoresNonAccessory(t *testing.T) {
	s := training.DefaultAccessorySelector()
	bench, err := training.NewExercise(benchParams())
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	pool := append(accessoryPool(t), bench)

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestMid: 10},
		pool, training.NewHistory(nil), today(), 2,
	)
	for _, id := range got {
		if id == training.ExerciseID("bench") {
			t.Error("メイン種目が補助として選ばれた")
		}
	}
}

func TestAccessorySelector_IsDeterministic(t *testing.T) {
	s := training.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 5, training.ChestLower: 5, training.Biceps: 5,
	}
	first := s.Select(residual, accessoryPool(t), training.NewHistory(nil), today(), 3)
	for i := 0; i < 20; i++ {
		got := s.Select(residual, accessoryPool(t), training.NewHistory(nil), today(), 3)
		if len(got) != len(first) {
			t.Fatalf("実行のたびに結果が変わる: %v vs %v", first, got)
		}
		for j := range got {
			if got[j] != first[j] {
				t.Fatalf("実行のたびに結果が変わる: %v vs %v", first, got)
			}
		}
	}
}

func TestNewAccessorySelector_RejectsBadParams(t *testing.T) {
	if _, err := training.NewAccessorySelector(-1, 3); err == nil {
		t.Error("負の回復日数が通ってしまう")
	}
	if _, err := training.NewAccessorySelector(2, 0); err == nil {
		t.Error("0セットが通ってしまう")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'AccessorySelector'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/accessory_selector.go`:

```go
package training

import (
	"fmt"
	"sort"
)

const (
	defaultRecoveryDays     = 2
	defaultSetsPerAccessory = 3
)

// AccessorySelector は残差を埋める補助種目を選ぶドメインサービス。無状態。
type AccessorySelector struct {
	recoveryDays     int
	setsPerAccessory int
}

func NewAccessorySelector(recoveryDays, setsPerAccessory int) (AccessorySelector, error) {
	if recoveryDays < 0 {
		return AccessorySelector{}, fmt.Errorf("回復日数は0以上である必要がある: %d", recoveryDays)
	}
	if setsPerAccessory < 1 {
		return AccessorySelector{}, fmt.Errorf("補助のセット数は1以上である必要がある: %d", setsPerAccessory)
	}
	return AccessorySelector{recoveryDays: recoveryDays, setsPerAccessory: setsPerAccessory}, nil
}

func DefaultAccessorySelector() AccessorySelector {
	return AccessorySelector{
		recoveryDays:     defaultRecoveryDays,
		setsPerAccessory: defaultSetsPerAccessory,
	}
}

func (s AccessorySelector) SetsPerAccessory() int { return s.setsPerAccessory }

// Select は残差の大きい筋区分から貪欲に補助種目を埋める。
//
//  1. recoveryDays 日以内に刺激された筋区分は候補から外す
//  2. 残差の大きい区分から選ぶ。同値なら筋区分名の昇順（再現性のため）
//  3. 同じ区分を狙う種目が複数あれば、最後に使ってから最も間隔が空いているものを選ぶ
func (s AccessorySelector) Select(
	residual map[MuscleRegion]float64,
	pool []*Exercise,
	h History,
	date Date,
	slots int,
) []ExerciseID {
	if slots <= 0 || len(residual) == 0 {
		return nil
	}

	byID := map[ExerciseID]*Exercise{}
	for _, e := range pool {
		if e != nil {
			byID[e.ID()] = e
		}
	}

	blocked := s.recentlyStimulated(h, byID, date)

	remaining := map[MuscleRegion]float64{}
	for region, gap := range residual {
		if gap > 0 && !blocked[region] {
			remaining[region] = gap
		}
	}

	accessories := make([]*Exercise, 0, len(pool))
	for _, e := range pool {
		if e != nil && e.Kind() == KindAccessory {
			accessories = append(accessories, e)
		}
	}
	sort.Slice(accessories, func(i, j int) bool { return accessories[i].ID() < accessories[j].ID() })

	chosen := make([]ExerciseID, 0, slots)
	taken := map[ExerciseID]bool{}

	for len(chosen) < slots {
		region, ok := topRegion(remaining)
		if !ok {
			break
		}

		candidate := s.pickForRegion(accessories, taken, region, h, date)
		if candidate == nil {
			// その区分を埋められる未使用の種目が無い。区分ごと諦める
			delete(remaining, region)
			continue
		}

		chosen = append(chosen, candidate.ID())
		taken[candidate.ID()] = true

		profile := candidate.Stimulus()
		for _, r := range profile.Regions() {
			c, ok := profile.Contribution(r)
			if !ok {
				continue
			}
			if _, tracked := remaining[r]; !tracked {
				continue
			}
			remaining[r] -= c.Float() * float64(s.setsPerAccessory)
			if remaining[r] <= 0 {
				delete(remaining, r)
			}
		}
	}

	return chosen
}

// recentlyStimulated は回復期間内に刺激された筋区分。
func (s AccessorySelector) recentlyStimulated(h History, byID map[ExerciseID]*Exercise, date Date) map[MuscleRegion]bool {
	// 半開区間 [cutoff, date) で見る。上限を閉じないと、セッション中に
	// 記録してから計画を開き直したとき、たった今やった種目の筋区分が
	// 「最近刺激した」と判定され、そのセッションの補助枠から自分自身が消える。
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

// pickForRegion はその区分を埋められる種目のうち、最後に使ってから最も間隔が空いているもの。
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

// neverUsedDaysAgo は一度も使っていない種目を最優先にするための番兵。
const neverUsedDaysAgo = 1 << 30

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
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'AccessorySelector'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/accessory_selector.go internal/domain/training/accessory_selector_test.go
git commit -m "feat(domain): 残差から補助種目を選ぶサービスを追加する"
```

---

### Task 14: ConditionAnalyzer（睡眠と体重トレンド）

**Files:**
- Create: `internal/domain/training/condition.go`
- Test: `internal/domain/training/condition_test.go`

**Interfaces:**
- Consumes: Task 2 の `Date`
- Produces:
  - `type DailyCondition struct{...}` / `func NewDailyCondition(date Date) DailyCondition` / `func (c DailyCondition) WithBodyWeight(kg float64) DailyCondition` / `func (c DailyCondition) WithSleepHours(h float64) DailyCondition` / アクセサ `Date()` / `BodyWeightKg() (float64, bool)` / `SleepHours() (float64, bool)`
  - `type ConditionLog struct{...}` / `func NewConditionLog(items []DailyCondition) ConditionLog`
  - `type ConditionAnalyzer struct{...}` / `func NewConditionAnalyzer(baselineDays int, sleepDeficitHours float64, trendWindowDays int) (ConditionAnalyzer, error)（窓は最低サンプル数以上・365日以下）` / `func DefaultConditionAnalyzer() ConditionAnalyzer`
  - `func (a ConditionAnalyzer) RIRAdjustment(log ConditionLog, date Date) int`
  - `func (a ConditionAnalyzer) BodyWeightTrendKgPerWeek(log ConditionLog, date Date) (float64, bool)`

**なぜ必要か:** 睡眠が短い日は自動で軽くする。体重トレンドは Task 15 の停滞判定で使う。**減量中の停滞とオーバーリーチによる停滞は記録だけ見ると同じ形をしている。体重と睡眠がないとこの2つを絶対に見分けられない。**

データが足りないときは補正しない。推測で軽くしない。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/condition_test.go`:

```go
package training_test

import (
	"math"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func condDate(daysAgo int) training.Date {
	return training.NewDate(2026, time.August, 30).AddDays(-daysAgo)
}

func sleepLog(days int, hours func(daysAgo int) float64) training.ConditionLog {
	items := make([]training.DailyCondition, 0, days)
	for i := 0; i < days; i++ {
		items = append(items, training.NewDailyCondition(condDate(i)).WithSleepHours(hours(i)))
	}
	return training.NewConditionLog(items)
}

func weightLog(days int, kg func(daysAgo int) float64) training.ConditionLog {
	items := make([]training.DailyCondition, 0, days)
	for i := 0; i < days; i++ {
		items = append(items, training.NewDailyCondition(condDate(i)).WithBodyWeight(kg(i)))
	}
	return training.NewConditionLog(items)
}

func TestDailyCondition_OptionalFields(t *testing.T) {
	c := training.NewDailyCondition(condDate(0))
	if _, ok := c.BodyWeightKg(); ok {
		t.Error("未設定の体重が取れてしまう")
	}
	if _, ok := c.SleepHours(); ok {
		t.Error("未設定の睡眠が取れてしまう")
	}

	c2 := c.WithBodyWeight(75).WithSleepHours(7)
	if v, ok := c2.BodyWeightKg(); !ok || v != 75 {
		t.Errorf("体重が誤り: %v %v", v, ok)
	}
	if v, ok := c2.SleepHours(); !ok || v != 7 {
		t.Errorf("睡眠が誤り: %v %v", v, ok)
	}
	// 元の値オブジェクトは変わらない
	if _, ok := c.BodyWeightKg(); ok {
		t.Error("With系が元の値を書き換えている")
	}
}

func TestConditionAnalyzer_NoAdjustmentWhenNormal(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := sleepLog(15, func(int) float64 { return 7 })
	if got := a.RIRAdjustment(log, condDate(0)); got != 0 {
		t.Errorf("平常時に補正が入っている: %d", got)
	}
}

func TestConditionAnalyzer_AdjustsWhenSleepDeprived(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := sleepLog(15, func(daysAgo int) float64 {
		if daysAgo == 0 {
			return 4.5
		}
		return 7
	})
	if got := a.RIRAdjustment(log, condDate(0)); got != 1 {
		t.Errorf("睡眠不足で補正されていない: %d", got)
	}
}

func TestConditionAnalyzer_NoAdjustmentWithoutTodayData(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	// 当日分を含まない
	items := make([]training.DailyCondition, 0, 14)
	for i := 1; i <= 14; i++ {
		items = append(items, training.NewDailyCondition(condDate(i)).WithSleepHours(7))
	}
	if got := a.RIRAdjustment(training.NewConditionLog(items), condDate(0)); got != 0 {
		t.Errorf("当日データが無いのに補正された: %d", got)
	}
}

func TestConditionAnalyzer_NoAdjustmentWithoutBaseline(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := sleepLog(1, func(int) float64 { return 4 })
	if got := a.RIRAdjustment(log, condDate(0)); got != 0 {
		t.Errorf("基準が無いのに補正された: %d", got)
	}
}

func TestConditionAnalyzer_TrendDetectsCutting(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	// 過去ほど重い＝減量中
	log := weightLog(21, func(daysAgo int) float64 { return 75 + float64(daysAgo)*0.05 })
	got, ok := a.BodyWeightTrendKgPerWeek(log, condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if got >= -0.1 {
		t.Errorf("減量中と判定されていない: %v", got)
	}
}

func TestConditionAnalyzer_TrendFlat(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := weightLog(21, func(int) float64 { return 75 })
	got, ok := a.BodyWeightTrendKgPerWeek(log, condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if math.Abs(got) > 0.01 {
		t.Errorf("横ばいと判定されていない: %v", got)
	}
}

func TestConditionAnalyzer_TrendNeedsEnoughSamples(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := weightLog(2, func(int) float64 { return 75 })
	if _, ok := a.BodyWeightTrendKgPerWeek(log, condDate(0)); ok {
		t.Error("サンプル不足なのにトレンドが返る")
	}
}

func TestConditionAnalyzer_TrendIgnoresMissingWeight(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := sleepLog(21, func(int) float64 { return 7 }) // 体重が一切ない
	if _, ok := a.BodyWeightTrendKgPerWeek(log, condDate(0)); ok {
		t.Error("体重が無いのにトレンドが返る")
	}
}

func TestNewConditionAnalyzer_RejectsBadParams(t *testing.T) {
	if _, err := training.NewConditionAnalyzer(0, 1.5, 21); err == nil {
		t.Error("基準日数0が通ってしまう")
	}
	if _, err := training.NewConditionAnalyzer(14, 0, 21); err == nil {
		t.Error("睡眠不足の閾値0が通ってしまう")
	}
	if _, err := training.NewConditionAnalyzer(14, 1.5, 0); err == nil {
		t.Error("トレンド窓0が通ってしまう")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'Condition'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/condition.go`:

```go
package training

import (
	"fmt"
	"math"
	"sort"
)

const (
	defaultBaselineDays      = 14
	defaultSleepDeficitHours = 1.5
	defaultTrendWindowDays   = 21

	minBaselineSamples = 5
	minTrendSamples    = 5
)

// DailyCondition は Health Connect から取り込んだ日次スナップショット。
// 体重と睡眠は欠損しうる。不変であり、With系は新しい値を返す。
type DailyCondition struct {
	date           Date
	bodyWeightKg   float64
	hasBodyWeight  bool
	sleepHours     float64
	hasSleepHours  bool
}

func NewDailyCondition(date Date) DailyCondition {
	return DailyCondition{date: date}
}

func (c DailyCondition) WithBodyWeight(kg float64) DailyCondition {
	if math.IsNaN(kg) || kg <= 0 {
		return c
	}
	c.bodyWeightKg = kg
	c.hasBodyWeight = true
	return c
}

func (c DailyCondition) WithSleepHours(h float64) DailyCondition {
	if math.IsNaN(h) || h < 0 {
		return c
	}
	c.sleepHours = h
	c.hasSleepHours = true
	return c
}

func (c DailyCondition) Date() Date { return c.date }

func (c DailyCondition) BodyWeightKg() (float64, bool) { return c.bodyWeightKg, c.hasBodyWeight }
func (c DailyCondition) SleepHours() (float64, bool)   { return c.sleepHours, c.hasSleepHours }

// ConditionLog は日次スナップショットの集まり。
type ConditionLog struct {
	items []DailyCondition
}

func NewConditionLog(items []DailyCondition) ConditionLog {
	copied := make([]DailyCondition, len(items))
	copy(copied, items)
	sort.Slice(copied, func(i, j int) bool { return copied[i].Date().Before(copied[j].Date()) })
	return ConditionLog{items: copied}
}

func (l ConditionLog) on(date Date) (DailyCondition, bool) {
	for _, c := range l.items {
		if c.Date().Equal(date) {
			return c, true
		}
	}
	return DailyCondition{}, false
}

// ConditionAnalyzer はコンディションから補正を導くドメインサービス。無状態。
type ConditionAnalyzer struct {
	baselineDays      int
	sleepDeficitHours float64
	trendWindowDays   int
}

func NewConditionAnalyzer(baselineDays int, sleepDeficitHours float64, trendWindowDays int) (ConditionAnalyzer, error)（窓は最低サンプル数以上・365日以下） {
	if baselineDays < 1 {
		return ConditionAnalyzer{}, fmt.Errorf("基準日数は1以上である必要がある: %d", baselineDays)
	}
	if math.IsNaN(sleepDeficitHours) || sleepDeficitHours <= 0 {
		return ConditionAnalyzer{}, fmt.Errorf("睡眠不足の閾値は正の数である必要がある: %v", sleepDeficitHours)
	}
	if trendWindowDays < 1 {
		return ConditionAnalyzer{}, fmt.Errorf("トレンド窓は1以上である必要がある: %d", trendWindowDays)
	}
	return ConditionAnalyzer{
		baselineDays:      baselineDays,
		sleepDeficitHours: sleepDeficitHours,
		trendWindowDays:   trendWindowDays,
	}, nil
}

func DefaultConditionAnalyzer() ConditionAnalyzer {
	return ConditionAnalyzer{
		baselineDays:      defaultBaselineDays,
		sleepDeficitHours: defaultSleepDeficitHours,
		trendWindowDays:   defaultTrendWindowDays,
	}
}

// RIRAdjustment は睡眠不足の日に目標RIRへ加える補正。
//
// 基準は直近 baselineDays 日の睡眠の中央値。そこから閾値以上短ければ +1。
// データが無い場合は補正しない。推測で軽くしない。
func (a ConditionAnalyzer) RIRAdjustment(log ConditionLog, date Date) int {
	todayCond, ok := log.on(date)
	if !ok {
		return 0
	}
	todaySleep, ok := todayCond.SleepHours()
	if !ok {
		return 0
	}

	from := date.AddDays(-a.baselineDays)
	samples := make([]float64, 0, a.baselineDays)
	for _, c := range log.items {
		if c.Date().Before(from) || !c.Date().Before(date) {
			continue
		}
		if h, ok := c.SleepHours(); ok {
			samples = append(samples, h)
		}
	}
	if len(samples) < minBaselineSamples {
		return 0
	}

	if todaySleep <= median(samples)-a.sleepDeficitHours {
		return 1
	}
	return 0
}

// BodyWeightTrendKgPerWeek は体重トレンド（kg/週）。最小二乗法の傾きを週換算する。
//
// 減量中かどうかの判定に使う。データが足りなければ false を返し、
// 呼び出し側は「判定できない」として扱う。
func (a ConditionAnalyzer) BodyWeightTrendKgPerWeek(log ConditionLog, date Date) (float64, bool) {
	from := date.AddDays(-a.trendWindowDays)

	type point struct{ x, y float64 }
	points := make([]point, 0, a.trendWindowDays)
	for _, c := range log.items {
		if c.Date().Before(from) || c.Date().After(date) {
			continue
		}
		if kg, ok := c.BodyWeightKg(); ok {
			points = append(points, point{x: float64(c.Date().DaysSince(from)), y: kg})
		}
	}
	if len(points) < minTrendSamples {
		return 0, false
	}

	n := float64(len(points))
	var sumX, sumY float64
	for _, p := range points {
		sumX += p.x
		sumY += p.y
	}
	meanX, meanY := sumX/n, sumY/n

	var numerator, denominator float64
	for _, p := range points {
		numerator += (p.x - meanX) * (p.y - meanY)
		denominator += (p.x - meanX) * (p.x - meanX)
	}
	if denominator == 0 {
		return 0, true
	}
	return numerator / denominator * 7, true
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'Condition'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/condition.go internal/domain/training/condition_test.go
git commit -m "feat(domain): コンディション補正を追加する"
```

---

### Task 15: DeloadPolicy

**Files:**
- Create: `internal/domain/training/deload.go`
- Test: `internal/domain/training/deload_test.go`

**Interfaces:**
- Consumes: Task 8 の `History`、Task 14 の `ConditionAnalyzer` / `ConditionLog`
- Produces:
  - `type DeloadProposal struct{...}` / `func (p DeloadProposal) Reason() string` / `func (p DeloadProposal) IntensityDropPct() float64`
  - `type DeloadPolicy struct{...}`
  - `func NewDeloadPolicy(analyzer ConditionAnalyzer, stallSessions int, intensityDropPct float64) (DeloadPolicy, error)`
  - `func DefaultDeloadPolicy() DeloadPolicy` — stallSessions 3、低下率 0.10
  - `func (p DeloadPolicy) Propose(h History, mainIDs []ExerciseID, log ConditionLog, date Date) (DeloadProposal, bool)`

**重要:** これは提案であって自動適用ではない。**重量が黙って下がるとアプリへの信頼が壊れる**ため、ユーザーが承認する。提案には必ず根拠を載せる。

**減量中は発火させない。** 減量中の停滞は正常であり、そこでデロードを出すのは誤診。体重が分からないときも発火させない（2つの停滞を見分けられないため）。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/deload_test.go`:

```go
package training_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func deloadDate() training.Date { return training.NewDate(2026, time.August, 30) }

// benchLog は daysAgo 日前に weight を挙げた記録。
func benchLog(t *testing.T, daysAgo int, weight float64) *training.SetLog {
	t.Helper()
	s, err := training.NewSetLog(training.SetLogParams{
		ID:          "bench-" + string(rune('a'+daysAgo%26)) + strings.Repeat("x", daysAgo%3+1),
		PerformedOn: deloadDate().AddDays(-daysAgo),
		ExerciseID:  "bench",
		WeightKg:    weight,
		Reps:        8,
		RIR:         2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	return s
}

func stalledHistory(t *testing.T) training.History {
	return training.NewHistory([]*training.SetLog{
		benchLog(t, 21, 85), benchLog(t, 14, 85), benchLog(t, 7, 85), benchLog(t, 0, 85),
	})
}

func improvingHistory(t *testing.T) training.History {
	return training.NewHistory([]*training.SetLog{
		benchLog(t, 21, 80), benchLog(t, 14, 82.5), benchLog(t, 7, 85), benchLog(t, 0, 87.5),
	})
}

func steadyConditions(kg func(daysAgo int) float64) training.ConditionLog {
	items := make([]training.DailyCondition, 0, 28)
	for i := 0; i < 28; i++ {
		items = append(items, training.NewDailyCondition(deloadDate().AddDays(-i)).
			WithBodyWeight(kg(i)).WithSleepHours(7))
	}
	return training.NewConditionLog(items)
}

func TestDeloadPolicy_ProposesWhenStalledAtStableWeight(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	got, ok := p.Propose(stalledHistory(t), []training.ExerciseID{"bench"},
		steadyConditions(func(int) float64 { return 75 }), deloadDate())
	if !ok {
		t.Fatal("停滞しているのに提案されない")
	}
	if got.IntensityDropPct() <= 0 {
		t.Errorf("低下率が正でない: %v", got.IntensityDropPct())
	}
	if strings.TrimSpace(got.Reason()) == "" {
		t.Error("根拠が空である")
	}
}

func TestDeloadPolicy_DoesNotProposeWhileCutting(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	// 過去ほど重い＝減量中
	cutting := steadyConditions(func(daysAgo int) float64 { return 75 + float64(daysAgo)*0.05 })
	if _, ok := p.Propose(stalledHistory(t), []training.ExerciseID{"bench"}, cutting, deloadDate()); ok {
		t.Error("減量中に提案されている")
	}
}

func TestDeloadPolicy_DoesNotProposeWhenImproving(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	if _, ok := p.Propose(improvingHistory(t), []training.ExerciseID{"bench"},
		steadyConditions(func(int) float64 { return 75 }), deloadDate()); ok {
		t.Error("伸びているのに提案されている")
	}
}

func TestDeloadPolicy_DoesNotProposeWithoutBodyWeight(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	items := make([]training.DailyCondition, 0, 28)
	for i := 0; i < 28; i++ {
		items = append(items, training.NewDailyCondition(deloadDate().AddDays(-i)).WithSleepHours(7))
	}
	if _, ok := p.Propose(stalledHistory(t), []training.ExerciseID{"bench"},
		training.NewConditionLog(items), deloadDate()); ok {
		t.Error("体重が無いのに提案されている")
	}
}

func TestDeloadPolicy_DoesNotProposeWithTooFewSessions(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory([]*training.SetLog{benchLog(t, 7, 85), benchLog(t, 0, 85)})
	if _, ok := p.Propose(h, []training.ExerciseID{"bench"},
		steadyConditions(func(int) float64 { return 75 }), deloadDate()); ok {
		t.Error("セッション数が足りないのに提案されている")
	}
}

func TestNewDeloadPolicy_RejectsBadParams(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	if _, err := training.NewDeloadPolicy(a, 0, 0.1); err == nil {
		t.Error("停滞セッション数0が通ってしまう")
	}
	if _, err := training.NewDeloadPolicy(a, 3, 0); err == nil {
		t.Error("低下率0が通ってしまう")
	}
	if _, err := training.NewDeloadPolicy(a, 3, 1); err == nil {
		t.Error("低下率100%が通ってしまう")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'Deload'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/deload.go`:

```go
package training

import (
	"fmt"
	"math"
	"strings"
)

const (
	defaultStallSessions    = 3
	defaultIntensityDropPct = 0.10

	// cuttingThresholdKgPerWeek はこれを下回れば減量中と判定する。
	cuttingThresholdKgPerWeek = -0.1
	// stallTolerance はこの割合を超えて伸びていなければ停滞と見なす。
	stallTolerance = 0.005
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

// DeloadPolicy はデロードの要否を判断するドメインサービス。無状態。
type DeloadPolicy struct {
	analyzer         ConditionAnalyzer
	stallSessions    int
	intensityDropPct float64
}

func NewDeloadPolicy(analyzer ConditionAnalyzer, stallSessions int, intensityDropPct float64) (DeloadPolicy, error) {
	if stallSessions < 1 {
		return DeloadPolicy{}, fmt.Errorf("停滞セッション数は1以上である必要がある: %d", stallSessions)
	}
	if math.IsNaN(intensityDropPct) || intensityDropPct <= 0 || intensityDropPct >= 1 {
		return DeloadPolicy{}, fmt.Errorf("強度低下率は0より大きく1未満である必要がある: %v", intensityDropPct)
	}
	return DeloadPolicy{
		analyzer:         analyzer,
		stallSessions:    stallSessions,
		intensityDropPct: intensityDropPct,
	}, nil
}

func DefaultDeloadPolicy() DeloadPolicy {
	return DeloadPolicy{
		analyzer:         DefaultConditionAnalyzer(),
		stallSessions:    defaultStallSessions,
		intensityDropPct: defaultIntensityDropPct,
	}
}

// Propose はデロードを提案する。適用はしない。承認するのはユーザー。
//
// 発火条件は「体重トレンドが横ばい以上」かつ「推定1RMが stallSessions 回連続で更新されない」。
// 減量中の停滞は正常なので発火させない。体重が分からないときも発火させない
// （減量による停滞とオーバーリーチによる停滞を見分けられないため）。
func (p DeloadPolicy) Propose(h History, mainIDs []ExerciseID, log ConditionLog, date Date) (DeloadProposal, bool) {
	trend, ok := p.analyzer.BodyWeightTrendKgPerWeek(log, date)
	if !ok {
		return DeloadProposal{}, false
	}
	if trend < cuttingThresholdKgPerWeek {
		return DeloadProposal{}, false
	}

	stalled := make([]ExerciseID, 0, len(mainIDs))
	for _, id := range mainIDs {
		if p.isStalled(h, id) {
			stalled = append(stalled, id)
		}
	}
	if len(stalled) == 0 {
		return DeloadProposal{}, false
	}

	names := make([]string, 0, len(stalled))
	for _, id := range stalled {
		names = append(names, string(id))
	}

	reason := fmt.Sprintf(
		"推定1RMが%dセッション停滞（%s）、体重トレンド %+.2fkg/週",
		p.stallSessions, strings.Join(names, ", "), trend,
	)

	return DeloadProposal{reason: reason, intensityDropPct: p.intensityDropPct}, true
}

// isStalled は直近 stallSessions 回で推定1RMが実質的に伸びていないか。
func (p DeloadPolicy) isStalled(h History, id ExerciseID) bool {
	sessions := h.ForExercise(id).Sessions()
	if len(sessions) < p.stallSessions+1 {
		return false
	}

	window := sessions[len(sessions)-(p.stallSessions+1):]
	reference, ok := window[0].MedianOneRepMax()
	if !ok || reference.Kg() <= 0 {
		return false
	}

	for _, s := range window[1:] {
		v, ok := s.MedianOneRepMax()
		if !ok {
			continue
		}
		if v.Kg() > reference.Kg()*(1+stallTolerance) {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'Deload'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/deload.go internal/domain/training/deload_test.go
git commit -m "feat(domain): デロード提案のポリシーを追加する"
```

---

### Task 16: SessionPlanner

**Files:**
- Create: `internal/domain/training/session_planner.go`
- Test: `internal/domain/training/session_planner_test.go`

**Interfaces:**
- Consumes: Task 6〜15 のすべて
- Produces:
  - `type PlannedSet struct{...}` / `func (s PlannedSet) ExerciseID() ExerciseID` / `func (s PlannedSet) Weight() (Weight, bool)` / `func (s PlannedSet) Sets() SetCount` / `func (s PlannedSet) TargetRIR() RIR` / `func (s PlannedSet) Role() (SlotRole, bool)`
  - `type PlannedSession struct{...}` / `func (s PlannedSession) Date() Date` / `func (s PlannedSession) Main() []PlannedSet` / `func (s PlannedSession) Accessories() []PlannedSet` / `func (s PlannedSession) DeloadProposal() (DeloadProposal, bool)`
  - `type PlanRequest struct{ Program *Program; Pool []*Exercise; History History; Conditions ConditionLog; Date Date; DeloadAccepted []ExerciseID; AccessorySlots int }`
  - `type SessionPlanner struct{...}` / `func NewSessionPlanner(...) SessionPlanner` / `func DefaultSessionPlanner() SessionPlanner`
  - `func (p SessionPlanner) Plan(req PlanRequest) (PlannedSession, error)`

**メイン種目が1つも選ばれていないプログラムはエラーにすること。** `Plan` が空のメインを返すと、ユーザーには空のワークアウトが返り、どこにもエラーが立たない。集約は種目マスタを知らないので検証できず、ここが唯一の検出点になる。

**週内カバレッジは履歴から求める。** `coveredThisWeek` は週初からその日までの実績を走査し、種目の刺激分布とセット数から `StimulusCoverage` を組み立てる。これが無いと残差の繰り越しが成立しない。

**この関数がドメインの入口。** 未来のセッションは保存せず、今日のメニューも来週のメニューもこの関数を対象日で呼んだ結果でしかない。だから予定と実績が食い違う状態が発生しない。

`PlannedSet.Weight()` が `(Weight, bool)` なのは、**履歴の無い種目では重量を推定できない**ため。数字を捏造せず「未確定」を返し、初回だけユーザーが決める。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/session_planner_test.go`:

```go
package training_test

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

var planMonday = training.NewDate(2026, time.August, 17) // 月曜

func mainExercise(t *testing.T, id string, lift training.MainLift, stimulus map[training.MuscleRegion]float64) *training.Exercise {
	t.Helper()
	e, err := training.NewExercise(training.ExerciseParams{
		ID: id, Name: id, Kind: training.KindMain,
		Stimulus: stimulus, IncrementKg: 2.5, MainLift: lift,
	})
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	return e
}

func planPool(t *testing.T) []*training.Exercise {
	t.Helper()
	return []*training.Exercise{
		mainExercise(t, "bench", training.LiftBench, map[training.MuscleRegion]float64{
			training.ChestMid: 1.0, training.TricepsLateral: 0.5,
		}),
		mainExercise(t, "squat", training.LiftSquat, map[training.MuscleRegion]float64{
			training.Quad: 1.0, training.Glute: 0.5,
		}),
		mainExercise(t, "deadlift", training.LiftDeadlift, map[training.MuscleRegion]float64{
			training.Hamstring: 1.0, training.Erector: 1.0,
		}),
		mkAccessory(t, "incline", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
	}
}

func planProgram(t *testing.T) *training.Program {
	t.Helper()
	target, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 10, training.Quad: 12,
	})
	if err != nil {
		t.Fatalf("週目標の生成に失敗: %v", err)
	}
	p, err := training.NewProgram(mustFrequency(t, 3), target,
		[]training.ExerciseID{"bench", "squat", "deadlift", "incline"})
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
}

// planHistory は3週分の履歴（推定1RMが立つ量）。
func planHistory(t *testing.T) training.History {
	t.Helper()
	logs := []*training.SetLog{}
	for _, daysAgo := range []int{7, 14, 21} {
		for _, spec := range []struct {
			id string
			kg float64
		}{{"bench", 85}, {"squat", 110}, {"deadlift", 140}} {
			s, err := training.NewSetLog(training.SetLogParams{
				ID:          spec.id + "-" + string(rune('a'+daysAgo)),
				PerformedOn: planMonday.AddDays(-daysAgo),
				ExerciseID:  spec.id,
				WeightKg:    spec.kg, Reps: 8, RIR: 2,
			})
			if err != nil {
				t.Fatalf("ログ生成に失敗: %v", err)
			}
			logs = append(logs, s)
		}
	}
	return training.NewHistory(logs)
}

func planRequest(t *testing.T) training.PlanRequest {
	t.Helper()
	return training.PlanRequest{
		Program:        planProgram(t),
		Pool:           planPool(t),
		History:        planHistory(t),
		Conditions:     training.NewConditionLog(nil),
		Date:           planMonday,
		AccessorySlots: 3,
	}
}

func mustPlan(t *testing.T, req training.PlanRequest) training.PlannedSession {
	t.Helper()
	s, err := training.DefaultSessionPlanner().Plan(req)
	if err != nil {
		t.Fatalf("Plan が失敗: %v", err)
	}
	return s
}

func TestSessionPlanner_AllMainLiftsGetASlot(t *testing.T) {
	s := mustPlan(t, planRequest(t))
	got := map[training.ExerciseID]bool{}
	for _, set := range s.Main() {
		got[set.ExerciseID()] = true
	}
	for _, want := range []training.ExerciseID{"bench", "squat", "deadlift"} {
		if !got[want] {
			t.Errorf("%s にスロットが割り当てられていない", want)
		}
	}
}

func TestSessionPlanner_FirstSessionOfWeekIsVariationRole(t *testing.T) {
	s := mustPlan(t, planRequest(t))
	for _, set := range s.Main() {
		role, ok := set.Role()
		if !ok || role != training.RoleVariation {
			t.Errorf("週1本目の役割が誤り: %v", role)
		}
	}
}

func TestSessionPlanner_SecondSessionOfWeekIsStandardRole(t *testing.T) {
	req := planRequest(t)
	extra, err := training.NewSetLog(training.SetLogParams{
		ID: "monday-done", PerformedOn: planMonday, ExerciseID: "bench",
		WeightKg: 80, Reps: 8, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	req.History = training.NewHistory(append(planHistory(t).Logs(), extra))
	req.Date = planMonday.AddDays(1)

	s := mustPlan(t, req)
	for _, set := range s.Main() {
		role, _ := set.Role()
		if role != training.RoleStandard {
			t.Errorf("週2本目の役割が誤り: %v", role)
		}
	}
}

func TestSessionPlanner_NoHistoryMeansNoWeight(t *testing.T) {
	req := planRequest(t)
	req.History = training.NewHistory(nil)

	s := mustPlan(t, req)
	for _, set := range s.Main() {
		if _, ok := set.Weight(); ok {
			t.Errorf("履歴が無いのに重量が出ている: %v", set.ExerciseID())
		}
	}
}

func TestSessionPlanner_FillsResidualWithAccessory(t *testing.T) {
	s := mustPlan(t, planRequest(t))
	found := false
	for _, set := range s.Accessories() {
		if set.ExerciseID() == training.ExerciseID("incline") {
			found = true
		}
	}
	if !found {
		t.Errorf("残差を埋める補助種目が付いていない: %v", s.Accessories())
	}
}

func TestSessionPlanner_SleepDeprivationRaisesTargetRIR(t *testing.T) {
	base := mustPlan(t, planRequest(t))

	req := planRequest(t)
	items := make([]training.DailyCondition, 0, 15)
	for i := 0; i <= 14; i++ {
		h := 7.5
		if i == 0 {
			h = 4.0
		}
		items = append(items, training.NewDailyCondition(planMonday.AddDays(-i)).WithSleepHours(h))
	}
	req.Conditions = training.NewConditionLog(items)
	tired := mustPlan(t, req)

	if tired.Main()[0].TargetRIR().Int() != base.Main()[0].TargetRIR().Int()+1 {
		t.Errorf("睡眠不足で目標RIRが上がっていない: %d → %d",
			base.Main()[0].TargetRIR().Int(), tired.Main()[0].TargetRIR().Int())
	}
}

func TestSessionPlanner_AcceptedDeloadLowersWeight(t *testing.T) {
	normal := mustPlan(t, planRequest(t))

	req := planRequest(t)
	req.DeloadAccepted = proposal.StalledExercises()
	deloaded := mustPlan(t, req)

	nw, ok1 := normal.Main()[0].Weight()
	dw, ok2 := deloaded.Main()[0].Weight()
	if !ok1 || !ok2 {
		t.Fatal("重量が確定していない")
	}
	if dw.Kg() >= nw.Kg() {
		t.Errorf("デロードで重量が下がっていない: %v → %v", nw.Kg(), dw.Kg())
	}
}

func TestSessionPlanner_DeloadKeepsSetCount(t *testing.T) {
	normal := mustPlan(t, planRequest(t))
	req := planRequest(t)
	req.DeloadAccepted = []training.ExerciseID{"bench"}
	deloaded := mustPlan(t, req)

	if normal.Main()[0].Sets().Int() != deloaded.Main()[0].Sets().Int() {
		t.Error("デロードでセット数が変わっている")
	}
}

func TestSessionPlanner_NoDeloadProposalWhenProgressing(t *testing.T) {
	s := mustPlan(t, planRequest(t))
	if _, ok := s.DeloadProposal(); ok {
		t.Error("停滞していないのに提案が付いている")
	}
}

func TestSessionPlanner_RejectsNilProgram(t *testing.T) {
	req := planRequest(t)
	req.Program = nil
	if _, err := training.DefaultSessionPlanner().Plan(req); err == nil {
		t.Error("プログラム無しが通ってしまう")
	}
}

func TestSessionPlanner_RejectsZeroDate(t *testing.T) {
	req := planRequest(t)
	req.Date = training.Date{}
	if _, err := training.DefaultSessionPlanner().Plan(req); err == nil {
		t.Error("日付無しが通ってしまう")
	}
}

func TestSessionPlanner_IsDeterministic(t *testing.T) {
	first := mustPlan(t, planRequest(t))
	for i := 0; i < 20; i++ {
		got := mustPlan(t, planRequest(t))
		if len(got.Main()) != len(first.Main()) || len(got.Accessories()) != len(first.Accessories()) {
			t.Fatal("実行のたびに構成が変わる")
		}
		for j := range got.Accessories() {
			if got.Accessories()[j].ExerciseID() != first.Accessories()[j].ExerciseID() {
				t.Fatalf("補助種目の順序が安定しない: %v vs %v",
					first.Accessories(), got.Accessories())
			}
		}
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'SessionPlanner'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/session_planner.go`:

```go
package training

import (
	"errors"
	"sort"
)

const (
	// accessoryIntensityPct は補助種目の強度。RIR2で10レップ前後を狙う位置。
	accessoryIntensityPct = 0.71
	accessoryTargetRIR    = 2
	defaultAccessorySlots = 3
)

// PlannedSet はその日にやることの1単位。
//
// Weight が (Weight, false) を返すのは、履歴が足りず推定できない場合。
// 数字を捏造せず「未確定」を返し、初回だけユーザーが決める。
type PlannedSet struct {
	exerciseID ExerciseID
	weight     Weight
	hasWeight  bool
	sets       SetCount
	targetRIR  RIR
	role       SlotRole
	hasRole    bool
}

func (s PlannedSet) ExerciseID() ExerciseID  { return s.exerciseID }
func (s PlannedSet) Weight() (Weight, bool)  { return s.weight, s.hasWeight }
func (s PlannedSet) Sets() SetCount          { return s.sets }
func (s PlannedSet) TargetRIR() RIR          { return s.targetRIR }
func (s PlannedSet) Role() (SlotRole, bool)  { return s.role, s.hasRole }

// PlannedSession は導出されたセッション。保存はしない。
type PlannedSession struct {
	date        Date
	main        []PlannedSet
	accessories []PlannedSet
	proposal    DeloadProposal
	hasProposal bool
}

func (s PlannedSession) Date() Date { return s.date }

func (s PlannedSession) Main() []PlannedSet {
	out := make([]PlannedSet, len(s.main))
	copy(out, s.main)
	return out
}

func (s PlannedSession) Accessories() []PlannedSet {
	out := make([]PlannedSet, len(s.accessories))
	copy(out, s.accessories)
	return out
}

func (s PlannedSession) DeloadProposal() (DeloadProposal, bool) {
	return s.proposal, s.hasProposal
}

// PlanRequest は導出の入力すべて。ドメインは自分でデータを取りに行かない。
type PlanRequest struct {
	Program        *Program
	Pool           []*Exercise
	History        History
	Conditions     ConditionLog
	Date           Date
	DeloadAccepted []ExerciseID
	AccessorySlots int
}

// SessionPlanner はドメインの入口となるドメインサービス。無状態。
type SessionPlanner struct {
	slots     SlotCatalog
	estimator OneRepMaxEstimator
	ratios    VariationRatioResolver
	accessory AccessorySelector
	deload    DeloadPolicy
}

func NewSessionPlanner(
	slots SlotCatalog,
	estimator OneRepMaxEstimator,
	ratios VariationRatioResolver,
	accessory AccessorySelector,
	deload DeloadPolicy,
) SessionPlanner {
	return SessionPlanner{
		slots: slots, estimator: estimator, ratios: ratios,
		accessory: accessory, deload: deload,
	}
}

func DefaultSessionPlanner() SessionPlanner {
	return SessionPlanner{
		slots:     NewSlotCatalog(),
		estimator: DefaultOneRepMaxEstimator(),
		ratios:    DefaultVariationRatioResolver(),
		accessory: DefaultAccessorySelector(),
		deload:    DefaultDeloadPolicy(),
	}
}

// Plan はその日のセッションを導出する。
//
// 未来のセッションはどこにも保存しない。今日のメニューも来週のメニューも
// この関数を対象日で呼んだ結果でしかない。だから予定と実績が食い違う状態が
// 原理的に発生しない。
func (p SessionPlanner) Plan(req PlanRequest) (PlannedSession, error) {
	if req.Program == nil {
		return PlannedSession{}, errors.New("プログラムが指定されていない")
	}
	if req.Date.IsZero() {
		return PlannedSession{}, errors.New("対象日が指定されていない")
	}

	selectedPool := p.selectedPool(req)
	template, ok := p.slots.Select(req.Program.Frequency(), sessionIndexInWeek(req.History, req.Date))
	if !ok {
		return PlannedSession{}, errors.New("週の頻度に対応するスロット構成が無い")
	}

	mainIDs := make([]ExerciseID, 0, 3)
	for _, e := range selectedPool {
		if e.Kind() == KindMain {
			mainIDs = append(mainIDs, e.ID())
		}
	}

	proposal, hasProposal := p.deload.Propose(req.History, mainIDs, req.Conditions, req.Date)

	// デロードは停滞した種目にだけ適用する。伸びている種目まで
	// 一律に下げると、本人の実感と噛み合わない。
	deloadTargets := map[ExerciseID]bool{}
	if hasProposal {
		for _, id := range proposal.StalledExercises() {
			deloadTargets[id] = true
		}
	}
	// 承認された種目にだけ適用する（D-022）。提案の有無とは独立。
	deloadTargets := make(map[ExerciseID]bool, len(req.DeloadAccepted))
	for _, id := range req.DeloadAccepted {
		deloadTargets[id] = true
	}

	rirBump := DefaultConditionAnalyzer().RIRAdjustment(req.Conditions, req.Date)

	main := make([]PlannedSet, 0, len(mainIDs))
	coverage := StimulusCoverage{}
	for _, e := range selectedPool {
		if e.Kind() != KindMain {
			continue
		}
		set, target := p.planMain(req, selectedPool, e, template, intensityScale, rirBump)
		main = append(main, set)
		coverage.Add(target.Stimulus(), set.Sets().Int())
	}

	perSession := req.Program.WeeklyTarget().PerSession(req.Program.Frequency())
	gaps := Residual(perSession, coverage)

	slots := req.AccessorySlots
	if slots <= 0 {
		slots = defaultAccessorySlots
	}
	chosen := p.accessory.Select(gaps, selectedPool, req.History, req.Date, slots)

	accessories := make([]PlannedSet, 0, len(chosen))
	for _, id := range chosen {
		accessories = append(accessories, p.planAccessory(req, selectedPool, id, intensityScale, rirBump))
	}

	return PlannedSession{
		date:        req.Date,
		main:        main,
		accessories: accessories,
		proposal:    proposal,
		hasProposal: hasProposal,
	}, nil
}

// selectedPool はプログラムで選択された種目に加え、それらの派生バリエーションを含む。
// バリエーションはユーザーが個別に選ぶものではなく、メインに付随して自動で回るため。
func (p SessionPlanner) selectedPool(req PlanRequest) []*Exercise {
	out := make([]*Exercise, 0, len(req.Pool))
	for _, e := range req.Pool {
		if e == nil {
			continue
		}
		if req.Program.Includes(e.ID()) || e.Kind() == KindVariation {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// planMain は1つのメインリフトのスロットを埋める。
// 返り値の *Exercise は実際に行う種目（バリエーションに差し替わることがある）。
func (p SessionPlanner) planMain(
	req PlanRequest,
	pool []*Exercise,
	main *Exercise,
	template SlotTemplate,
	intensityScale float64,
	rirBump int,
) (PlannedSet, *Exercise) {
	target := main
	ratio := unitRatio

	if template.Role() == RoleVariation {
		if v := p.pickVariation(req, pool, main); v != nil {
			target = v
			ratio = p.ratios.Resolve(req.History, v, main.ID())
		}
	}

	set := PlannedSet{
		exerciseID: target.ID(),
		sets:       template.Sets(),
		targetRIR:  template.TargetRIR().Plus(rirBump),
		role:       template.Role(),
		hasRole:    true,
	}

	// 重量はメインの推定1RMを基準にし、バリエーションには係数を掛ける。
	// バリエーション自身の1RMを使うと、履歴の少ない種目で数字が暴れる。
	if orm, ok := p.estimator.Estimate(req.History, main.ID(), req.Date); ok {
		set.weight = orm.WorkWeight(template.Intensity().Scale(intensityScale), ratio, target.Increment())
		set.hasWeight = true
	}
	return set, target
}

// pickVariation は同じメインリフトの派生のうち、最後に使ってから最も間隔が空いているもの。
func (p SessionPlanner) pickVariation(req PlanRequest, pool []*Exercise, main *Exercise) *Exercise {
	lift, ok := main.MainLift()
	if !ok {
		return nil
	}

	var best *Exercise
	bestDaysAgo := -1
	for _, e := range pool {
		if e.Kind() != KindVariation {
			continue
		}
		if l, ok := e.MainLift(); !ok || l != lift {
			continue
		}
		daysAgo := neverUsedDaysAgo
		if last, ok := req.History.LastPerformed(e.ID()); ok {
			daysAgo = req.Date.DaysSince(last)
		}
		if daysAgo > bestDaysAgo {
			best = e
			bestDaysAgo = daysAgo
		}
	}
	return best
}

func (p SessionPlanner) planAccessory(
	req PlanRequest,
	pool []*Exercise,
	id ExerciseID,
	intensityScale float64,
	rirBump int,
) PlannedSet {
	var exercise *Exercise
	for _, e := range pool {
		if e.ID() == id {
			exercise = e
			break
		}
	}

	sets, err := NewSetCount(p.accessory.SetsPerAccessory())
	if err != nil {
		sets = SetCount{v: defaultSetsPerAccessory}
	}
	baseRIR, err := NewRIR(accessoryTargetRIR)
	if err != nil {
		baseRIR = RIR{v: accessoryTargetRIR}
	}

	set := PlannedSet{
		exerciseID: id,
		sets:       sets,
		targetRIR:  baseRIR.Plus(rirBump),
	}
	if exercise == nil {
		return set
	}

	if orm, ok := p.estimator.Estimate(req.History, id, req.Date); ok {
		intensity, err := NewIntensityPct(accessoryIntensityPct)
		if err == nil {
			set.weight = orm.WorkWeight(intensity.Scale(intensityScale), unitRatio, exercise.Increment())
			set.hasWeight = true
		}
	}
	return set
}

// sessionIndexInWeek はその週で対象日が何本目のセッションか（0始まり）。
// 曜日の割り当てはドメインの責務ではないため、実績から導出する。
func sessionIndexInWeek(h History, date Date) int {
	start := date.WeekStart()
	return h.OnOrAfter(start).Before(date).SessionCount()
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'SessionPlanner'`
Expected: PASS

- [ ] **Step 5: 全テストを実行する**

Run: `go test ./...`
Expected: ok

- [ ] **Step 6: コミット**

```bash
git add internal/domain/training/session_planner.go internal/domain/training/session_planner_test.go
git commit -m "feat(domain): SessionPlanner でドメインを統合する"
```

---

### Task 17: シードデータ

**Files:**
- Create: `internal/domain/training/seed/exercises.go`
- Create: `internal/domain/training/seed/weekly_target.go`
- Test: `internal/domain/training/seed/seed_test.go`

**Interfaces:**
- Consumes: Task 6 の `Exercise`、Task 12 の `WeeklyVolumeTarget`
- Produces:
  - `func Exercises() ([]*training.Exercise, error)`
  - `func DefaultWeeklyTarget() (training.WeeklyVolumeTarget, error)`

`internal/domain/training/seed` は domain 配下なので、Task 1 の依存方向テストの制約（`/internal/domain/` を含むパスのみ許可）を満たす。

**なぜ必要か:** 「メニュー設定が面倒」から始まったのに、自動化を強くするほど初期登録という別の面倒が生まれる。それを潰すのがこのタスク。**ユーザーがやるのは「使う種目にチェックを入れる」だけにする。**

**注意:** ここに書いた種目リストは出発点であり網羅ではない。対メイン係数も仮の値でよい（Task 10 が実績で上書きする）。足りない種目は使いながら足す。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/seed/seed_test.go`:

```go
package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

func TestExercises_AreValid(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("シードが空である")
	}
}

func TestExercises_ContainsBigThree(t *testing.T) {
	all, _ := seed.Exercises()
	found := map[training.MainLift]bool{}
	for _, e := range all {
		if e.Kind() != training.KindMain {
			continue
		}
		if lift, ok := e.MainLift(); ok {
			found[lift] = true
		}
	}
	for _, want := range []training.MainLift{
		training.LiftSquat, training.LiftBench, training.LiftDeadlift,
	} {
		if !found[want] {
			t.Errorf("%s がメイン種目に無い", want)
		}
	}
}

func TestExercises_IDsAreUnique(t *testing.T) {
	all, _ := seed.Exercises()
	seen := map[training.ExerciseID]bool{}
	for _, e := range all {
		if seen[e.ID()] {
			t.Errorf("種目IDが重複している: %s", e.ID())
		}
		seen[e.ID()] = true
	}
}

func TestExercises_VariationsBelongToAMainLift(t *testing.T) {
	all, _ := seed.Exercises()
	count := 0
	for _, e := range all {
		if e.Kind() != training.KindVariation {
			continue
		}
		count++
		if _, ok := e.MainLift(); !ok {
			t.Errorf("%s に所属メインが無い", e.ID())
		}
		if _, ok := e.DefaultRatioToMain(); !ok {
			t.Errorf("%s に対メイン係数が無い", e.ID())
		}
	}
	if count == 0 {
		t.Error("バリエーションが1つも無い")
	}
}

func TestExercises_EveryMainLiftHasAVariation(t *testing.T) {
	all, _ := seed.Exercises()
	covered := map[training.MainLift]bool{}
	for _, e := range all {
		if e.Kind() != training.KindVariation {
			continue
		}
		if lift, ok := e.MainLift(); ok {
			covered[lift] = true
		}
	}
	for _, want := range []training.MainLift{
		training.LiftSquat, training.LiftBench, training.LiftDeadlift,
	} {
		if !covered[want] {
			t.Errorf("%s のバリエーションが無い", want)
		}
	}
}

func TestExercises_AccessoriesCoverEveryRegion(t *testing.T) {
	all, _ := seed.Exercises()
	covered := map[training.MuscleRegion]bool{}
	for _, e := range all {
		if e.Kind() != training.KindAccessory {
			continue
		}
		for _, r := range e.Stimulus().Regions() {
			covered[r] = true
		}
	}
	for _, r := range training.AllMuscleRegions() {
		if !covered[r] {
			t.Errorf("補助種目でカバーできない筋区分がある: %s", r)
		}
	}
}

func TestDefaultWeeklyTarget_CoversEveryRegion(t *testing.T) {
	target, err := seed.DefaultWeeklyTarget()
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	for _, r := range training.AllMuscleRegions() {
		if target.Sets(r) <= 0 {
			t.Errorf("筋区分 %s の目標が設定されていない", r)
		}
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/seed/...`
Expected: コンパイルエラー

- [ ] **Step 3: 種目マスタを実装する**

`internal/domain/training/seed/exercises.go`:

```go
// Package seed はアプリ同梱の初期データを提供する。
//
// 「メニュー設定が面倒」から始まったのに、自動化を強くするほど初期登録という
// 別の面倒が生まれる。それを潰すのがこのパッケージの役割で、ユーザーがやるのは
// 「使う種目にチェックを入れる」だけにする。
//
// ここに書いた種目リストは出発点であり網羅ではない。対メイン係数も仮の値でよく、
// 実績が溜まれば VariationRatioResolver が実測値で上書きする。
package seed

import "github.com/dyoshyy/liftplan-server/internal/domain/training"

type stimulus = map[training.MuscleRegion]float64

func mainLift(id, name string, lift training.MainLift, inc float64, s stimulus) training.ExerciseParams {
	return training.ExerciseParams{
		ID: id, Name: name, Kind: training.KindMain,
		Stimulus: s, IncrementKg: inc, MainLift: lift,
	}
}

func variation(id, name string, lift training.MainLift, ratio, inc float64, s stimulus) training.ExerciseParams {
	return training.ExerciseParams{
		ID: id, Name: name, Kind: training.KindVariation,
		Stimulus: s, IncrementKg: inc, MainLift: lift, DefaultRatioToMain: ratio,
	}
}

func accessory(id, name string, inc float64, s stimulus) training.ExerciseParams {
	return training.ExerciseParams{
		ID: id, Name: name, Kind: training.KindAccessory,
		Stimulus: s, IncrementKg: inc,
	}
}

func specs() []training.ExerciseParams {
	r := struct {
		ChestUpper, ChestMid, ChestLower                        training.MuscleRegion
		Lat, TrapMid, TrapUpper, Erector                        training.MuscleRegion
		FrontDelt, SideDelt, RearDelt                           training.MuscleRegion
		TricepsLong, TricepsLateral, Biceps, Forearm            training.MuscleRegion
		Quad, Hamstring, Glute, Adductor, Calf, Abs, Oblique    training.MuscleRegion
	}{
		training.ChestUpper, training.ChestMid, training.ChestLower,
		training.Lat, training.TrapMid, training.TrapUpper, training.Erector,
		training.FrontDelt, training.SideDelt, training.RearDelt,
		training.TricepsLong, training.TricepsLateral, training.Biceps, training.Forearm,
		training.Quad, training.Hamstring, training.Glute, training.Adductor,
		training.Calf, training.Abs, training.Oblique,
	}

	return []training.ExerciseParams{
		// --- メイン ---
		mainLift("squat", "スクワット", training.LiftSquat, 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		mainLift("bench", "ベンチプレス", training.LiftBench, 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.5}),
		mainLift("deadlift", "デッドリフト", training.LiftDeadlift, 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.TrapMid: 0.4, r.Forearm: 0.4}),

		// --- バリエーション ---
		variation("larsen_press", "ラーセンプレス", training.LiftBench, 0.90, 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4}),
		variation("tempo_bench", "テンポベンチ", training.LiftBench, 0.85, 2.5,
			stimulus{r.ChestMid: 1.0, r.TricepsLateral: 0.5, r.FrontDelt: 0.4}),
		variation("close_grip_bench", "ナローベンチ", training.LiftBench, 0.88, 2.5,
			stimulus{r.ChestMid: 0.7, r.TricepsLateral: 1.0, r.TricepsLong: 0.6}),
		variation("pause_squat", "ポーズスクワット", training.LiftSquat, 0.88, 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.7, r.Adductor: 0.4, r.Erector: 0.4}),
		variation("front_squat", "フロントスクワット", training.LiftSquat, 0.80, 2.5,
			stimulus{r.Quad: 1.0, r.Glute: 0.4, r.Erector: 0.5, r.Abs: 0.4}),
		variation("deficit_deadlift", "デフィシットデッドリフト", training.LiftDeadlift, 0.90, 5.0,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.8, r.Erector: 1.0, r.Quad: 0.4}),
		variation("romanian_deadlift", "ルーマニアンデッドリフト", training.LiftDeadlift, 0.70, 2.5,
			stimulus{r.Hamstring: 1.0, r.Glute: 0.7, r.Erector: 0.7}),

		// --- 胸 ---
		accessory("incline_db_press", "インクラインダンベルプレス", 2.0,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		accessory("incline_barbell_press", "インクラインベンチプレス", 2.5,
			stimulus{r.ChestUpper: 1.0, r.FrontDelt: 0.5, r.TricepsLateral: 0.3}),
		accessory("dip", "ディップス", 2.5,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.6, r.TricepsLong: 0.4}),
		accessory("decline_press", "デクラインプレス", 2.5,
			stimulus{r.ChestLower: 1.0, r.TricepsLateral: 0.4}),
		accessory("pec_fly", "ペックフライ", 2.5,
			stimulus{r.ChestMid: 1.0, r.ChestUpper: 0.3}),

		// --- 背中 ---
		accessory("lat_pulldown", "ラットプルダウン", 2.5,
			stimulus{r.Lat: 1.0, r.Biceps: 0.4, r.RearDelt: 0.2}),
		accessory("pull_up", "チンニング", 2.5,
			stimulus{r.Lat: 1.0, r.Biceps: 0.5, r.Forearm: 0.3}),
		accessory("barbell_row", "バーベルロウ", 2.5,
			stimulus{r.Lat: 0.7, r.TrapMid: 1.0, r.RearDelt: 0.4, r.Biceps: 0.3}),
		accessory("seated_row", "シーテッドロウ", 2.5,
			stimulus{r.TrapMid: 1.0, r.Lat: 0.6, r.Biceps: 0.3}),
		accessory("back_extension", "バックエクステンション", 2.5,
			stimulus{r.Erector: 1.0, r.Glute: 0.5, r.Hamstring: 0.4}),
		accessory("shrug", "シュラッグ", 2.5,
			stimulus{r.TrapUpper: 1.0, r.Forearm: 0.3}),

		// --- 肩 ---
		accessory("overhead_press", "オーバーヘッドプレス", 2.5,
			stimulus{r.FrontDelt: 1.0, r.SideDelt: 0.5, r.TricepsLateral: 0.4}),
		accessory("side_raise", "サイドレイズ", 1.0,
			stimulus{r.SideDelt: 1.0}),
		accessory("rear_delt_fly", "リアデルトフライ", 1.0,
			stimulus{r.RearDelt: 1.0, r.TrapMid: 0.3}),

		// --- 腕 ---
		accessory("triceps_pushdown", "トライセプスプレスダウン", 2.5,
			stimulus{r.TricepsLateral: 1.0, r.TricepsLong: 0.4}),
		accessory("overhead_extension", "オーバーヘッドエクステンション", 2.5,
			stimulus{r.TricepsLong: 1.0, r.TricepsLateral: 0.4}),
		accessory("barbell_curl", "バーベルカール", 2.5,
			stimulus{r.Biceps: 1.0, r.Forearm: 0.4}),
		accessory("hammer_curl", "ハンマーカール", 2.0,
			stimulus{r.Biceps: 0.8, r.Forearm: 1.0}),

		// --- 脚 ---
		accessory("leg_press", "レッグプレス", 5.0,
			stimulus{r.Quad: 1.0, r.Glute: 0.5, r.Adductor: 0.3}),
		accessory("leg_extension", "レッグエクステンション", 2.5,
			stimulus{r.Quad: 1.0}),
		accessory("leg_curl", "レッグカール", 2.5,
			stimulus{r.Hamstring: 1.0}),
		accessory("hip_thrust", "ヒップスラスト", 5.0,
			stimulus{r.Glute: 1.0, r.Hamstring: 0.4}),
		accessory("adductor_machine", "アダクション", 2.5,
			stimulus{r.Adductor: 1.0}),
		accessory("calf_raise", "カーフレイズ", 2.5,
			stimulus{r.Calf: 1.0}),

		// --- 体幹 ---
		accessory("cable_crunch", "ケーブルクランチ", 2.5,
			stimulus{r.Abs: 1.0, r.Oblique: 0.3}),
		accessory("side_bend", "サイドベンド", 2.5,
			stimulus{r.Oblique: 1.0, r.Abs: 0.3}),
	}
}

// Exercises はアプリ同梱の種目マスタ。
func Exercises() ([]*training.Exercise, error) {
	all := specs()
	out := make([]*training.Exercise, 0, len(all))
	for _, p := range all {
		e, err := training.NewExercise(p)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
```

- [ ] **Step 4: 週目標プリセットを実装する**

`internal/domain/training/seed/weekly_target.go`:

```go
package seed

import "github.com/dyoshyy/liftplan-server/internal/domain/training"

// DefaultWeeklyTarget は筋区分ごとの週目標セット数のプリセット。
//
// パワーリフティング寄りに、BIG3 が直接使う区分（大腿四頭筋・ハム・臀筋・
// 脊柱起立筋・大胸筋中部）を厚くし、装飾的な区分は薄くしている。
// 不満が出た区分だけ後から調整すればよく、最初から自分で全部決める必要はない。
func DefaultWeeklyTarget() (training.WeeklyVolumeTarget, error) {
	return training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestUpper: 8,
		training.ChestMid:   14,
		training.ChestLower: 6,

		training.Lat:       12,
		training.TrapMid:   12,
		training.TrapUpper: 6,
		training.Erector:   12,

		training.FrontDelt: 8,
		training.SideDelt:  10,
		training.RearDelt:  8,

		training.TricepsLong:    8,
		training.TricepsLateral: 10,

		training.Biceps:  10,
		training.Forearm: 6,

		training.Quad:      16,
		training.Hamstring: 12,
		training.Glute:     12,
		training.Adductor:  6,
		training.Calf:      8,

		training.Abs:     8,
		training.Oblique: 6,
	})
}
```

- [ ] **Step 5: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/seed/...`
Expected: PASS

- [ ] **Step 6: 全テストを実行する**

Run: `go test ./...`
Expected: ok

- [ ] **Step 7: コミット**

```bash
git add internal/domain/training/seed/
git commit -m "feat(domain): 種目マスタと週目標のシードを追加する"
```

---

### Task 18: シードを使った通し検証

**Files:**
- Test: `internal/domain/training/seed/end_to_end_test.go`

**Interfaces:**
- Consumes: Task 16 の `SessionPlanner`、Task 17 のシード
- Produces: なし（検証のみ）

**なぜ必要か:** 各部品が個別に通っても、シードを実際に食わせると噛み合わせで壊れることがある。「補助スロットが3つあるのに残差を埋められる種目が見つからない」といった不具合はここでしか出ない。

- [ ] **Step 1: 通しテストを書く**

`internal/domain/training/seed/end_to_end_test.go`:

```go
package seed_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

var start = training.NewDate(2026, time.August, 17) // 月曜

func fullProgram(t *testing.T) (*training.Program, []*training.Exercise) {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget()
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	// メインと補助をすべて選択する（バリエーションは自動で付随する）
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}

	freq, err := training.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	program, err := training.NewProgram(freq, target, selected)
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return program, pool
}

func TestSeed_FirstSessionIsPlannable(t *testing.T) {
	program, pool := fullProgram(t)

	session, err := training.DefaultSessionPlanner().Plan(training.PlanRequest{
		Program: program, Pool: pool,
		History: training.NewHistory(nil), Conditions: training.NewConditionLog(nil),
		Date: start, AccessorySlots: 3,
	})
	if err != nil {
		t.Fatalf("Plan が失敗: %v", err)
	}

	if len(session.Main()) != 3 {
		t.Errorf("メインが3種目でない: %d", len(session.Main()))
	}
	if len(session.Accessories()) == 0 {
		t.Error("補助種目が1つも選ばれていない")
	}
	for _, s := range session.Main() {
		if _, ok := s.Weight(); ok {
			t.Errorf("履歴が無いのに重量が出ている: %s", s.ExerciseID())
		}
	}
}

func TestSeed_FourWeeksStayCoherent(t *testing.T) {
	program, pool := fullProgram(t)
	planner := training.DefaultSessionPlanner()

	logs := []*training.SetLog{}
	counter := 0

	// 月・水・金 を4週間
	for week := 0; week < 4; week++ {
		for _, offset := range []int{0, 2, 4} {
			date := start.AddDays(week*7 + offset)

			session, err := planner.Plan(training.PlanRequest{
				Program: program, Pool: pool,
				History:    training.NewHistory(logs),
				Conditions: training.NewConditionLog(nil),
				Date:       date, AccessorySlots: 3,
			})
			if err != nil {
				t.Fatalf("%v の Plan が失敗: %v", date, err)
			}
			if len(session.Main()) != 3 {
				t.Fatalf("%v でメインが3種目でない: %d", date, len(session.Main()))
			}
			if len(session.Accessories()) == 0 {
				t.Fatalf("%v で補助が選ばれていない", date)
			}

			// 指示どおり実行したことにして記録する。重量未確定なら初回の仮値を置く
			for _, planned := range append(session.Main(), session.Accessories()...) {
				kg := 60.0
				if w, ok := planned.Weight(); ok {
					kg = w.Kg()
				}
				log, err := training.NewSetLog(training.SetLogParams{
					ID:          fmt.Sprintf("log-%04d", counter),
					PerformedOn: date,
					ExerciseID:  string(planned.ExerciseID()),
					WeightKg:    kg, Reps: 8, RIR: planned.TargetRIR().Int(),
				})
				if err != nil {
					t.Fatalf("ログ生成に失敗: %v", err)
				}
				logs = append(logs, log)
				counter++
			}
		}
	}

	final, err := planner.Plan(training.PlanRequest{
		Program: program, Pool: pool,
		History:    training.NewHistory(logs),
		Conditions: training.NewConditionLog(nil),
		Date:       start.AddDays(28), AccessorySlots: 3,
	})
	if err != nil {
		t.Fatalf("最終 Plan が失敗: %v", err)
	}
	for _, s := range final.Main() {
		w, ok := s.Weight()
		if !ok {
			t.Errorf("4週間分の履歴があるのに %s の重量が出ていない", s.ExerciseID())
			continue
		}
		if w.Kg() <= 0 {
			t.Errorf("%s の重量が0以下: %v", s.ExerciseID(), w.Kg())
		}
	}
}

func TestSeed_RolesRotateWithinAWeek(t *testing.T) {
	program, pool := fullProgram(t)
	planner := training.DefaultSessionPlanner()

	logs := []*training.SetLog{}
	roles := []training.SlotRole{}
	counter := 0

	for _, offset := range []int{0, 2, 4} {
		date := start.AddDays(offset)
		session, err := planner.Plan(training.PlanRequest{
			Program: program, Pool: pool,
			History:    training.NewHistory(logs),
			Conditions: training.NewConditionLog(nil),
			Date:       date, AccessorySlots: 3,
		})
		if err != nil {
			t.Fatalf("Plan が失敗: %v", err)
		}

		role, ok := session.Main()[0].Role()
		if !ok {
			t.Fatal("メインに役割が付いていない")
		}
		roles = append(roles, role)

		for _, planned := range session.Main() {
			kg := 60.0
			if w, ok := planned.Weight(); ok {
				kg = w.Kg()
			}
			log, _ := training.NewSetLog(training.SetLogParams{
				ID:          fmt.Sprintf("r-%04d", counter),
				PerformedOn: date, ExerciseID: string(planned.ExerciseID()),
				WeightKg: kg, Reps: 8, RIR: planned.TargetRIR().Int(),
			})
			logs = append(logs, log)
			counter++
		}
	}

	want := []training.SlotRole{
		training.RoleVariation, training.RoleStandard, training.RoleHeavy,
	}
	for i, w := range want {
		if roles[i] != w {
			t.Errorf("%d本目の役割が誤り: got %s, want %s", i, roles[i], w)
		}
	}
}
```

- [ ] **Step 2: テストを実行する**

Run: `go test ./internal/domain/training/seed/... -run 'TestSeed'`

Expected: 初回は失敗する可能性がある。失敗しても**テストを緩めず実装側の噛み合わせを直す**。よくある原因は次の3つ。

1. **補助種目が選ばれない** — `Plan` が週目標を頻度で割っているため、1セッションあたりの残差が小さくなりすぎている。`PerSession` の割り方か、48時間ルールの範囲を見直す
2. **バリエーションスロットで重量が出ない** — `planMain` がメイン種目の推定1RMを参照しているか確認する。バリエーション自身の履歴ではなく、メインの1RMに係数を掛けるのが正しい
3. **役割が巡回しない** — `sessionIndexInWeek` が対象日より前のセッション数を数えているか確認する。当日を含めると常に1本目にならない

- [ ] **Step 3: 全テストを実行する**

Run: `go test ./...`
Expected: ok

- [ ] **Step 4: コミット**

```bash
git add internal/domain/training/seed/end_to_end_test.go
git commit -m "test(domain): シードを使った通し検証を追加する"
```

---

## 完了の定義（第3部の範囲）

- `go test ./...` が全件パスする
- `TestDomain_DependsOnNothingOutside` が引き続き通る
- `SessionPlanner.Plan` を呼べば、履歴ゼロの状態からでも4週間分のセッションが破綻せず組める
- 同じ入力に対して常に同じセッションが返る（マップ反復順に依存していない）

続きは `04-application-and-api.md`。
