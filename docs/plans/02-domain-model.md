# liftplan サーバー ドメイン層 第2部 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 種目・実績ログ・履歴のモデルと、そこから推定1RMと対メイン係数を導く仕組みを作る。

**前提:** `01-domain.md` の Task 1〜5 が完了していること（`Date`、分類型、計測系の値オブジェクト、`OneRepMax` が存在する）。

**Global Constraints:** `01-domain.md` の Global Constraints をすべて引き継ぐ。特に「Domain 層は標準ライブラリのみ」「値オブジェクトは不変・自己検証」「ドメインサービスは無状態」。

---

### Task 6: StimulusProfile と Exercise エンティティ

**Files:**
- Create: `internal/domain/training/exercise.go`
- Test: `internal/domain/training/exercise_test.go`

**Interfaces:**
- Consumes: Task 3 の `MuscleRegion` / `ExerciseKind` / `MainLift`、Task 4 の `Contribution` / `Increment` / `Ratio`
- Produces:
  - `type ExerciseID string` / `func NewExerciseID(s string) (ExerciseID, error)`
  - `type StimulusProfile struct{...}` / `func NewStimulusProfile(m map[MuscleRegion]float64) (StimulusProfile, error)` / `func (p StimulusProfile) Regions() []MuscleRegion` / `func (p StimulusProfile) Contribution(r MuscleRegion) (Contribution, bool)` / `func (p StimulusProfile) IsEmpty() bool`
  - `type ExerciseParams struct{...}` / `func NewExercise(p ExerciseParams) (*Exercise, error)`
  - `type Exercise struct{...}` とアクセサ `ID()` / `Name()` / `Kind()` / `MainLift() (MainLift, bool)` / `DefaultRatioToMain() (Ratio, bool)` / `Stimulus() StimulusProfile` / `Increment() Increment`
  - `func (e *Exercise) SameIdentity(o *Exercise) bool`

`Regions()` はソート済みを返す。マップの反復順に依存すると、同じ入力で違うセッションが生成されてしまい再現性が壊れる。

不変条件：`KindMain` はメインリフトを必須とする。`KindVariation` はメインリフトと対メイン係数を必須とする。`KindAccessory` はメインリフトを持ってはいけない。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/exercise_test.go`:

```go
package training_test

import (
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func benchParams() training.ExerciseParams {
	return training.ExerciseParams{
		ID:   "bench",
		Name: "ベンチプレス",
		Kind: training.KindMain,
		Stimulus: map[training.MuscleRegion]float64{
			training.ChestMid:       1.0,
			training.TricepsLateral: 0.5,
		},
		IncrementKg: 2.5,
		MainLift:    training.LiftBench,
	}
}

func TestNewExercise_Main(t *testing.T) {
	e, err := training.NewExercise(benchParams())
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	if e.ID() != training.ExerciseID("bench") {
		t.Errorf("ID が誤り: %v", e.ID())
	}
	lift, ok := e.MainLift()
	if !ok || lift != training.LiftBench {
		t.Errorf("メインリフトが誤り: %v %v", lift, ok)
	}
	if _, ok := e.DefaultRatioToMain(); ok {
		t.Error("メイン種目に対メイン係数が付いている")
	}
}

func TestNewExercise_MainRequiresLift(t *testing.T) {
	p := benchParams()
	p.MainLift = ""
	if _, err := training.NewExercise(p); err == nil {
		t.Error("メインリフトの無いメイン種目が通ってしまう")
	}
}

func TestNewExercise_VariationRequiresLiftAndRatio(t *testing.T) {
	p := benchParams()
	p.ID = "larsen"
	p.Kind = training.KindVariation

	if _, err := training.NewExercise(p); err == nil {
		t.Error("対メイン係数の無いバリエーションが通ってしまう")
	}

	p.DefaultRatioToMain = 0.9
	if _, err := training.NewExercise(p); err != nil {
		t.Errorf("正常なバリエーションが失敗: %v", err)
	}

	p.MainLift = ""
	if _, err := training.NewExercise(p); err == nil {
		t.Error("所属メインの無いバリエーションが通ってしまう")
	}
}

func TestNewExercise_AccessoryMustNotHaveLift(t *testing.T) {
	p := benchParams()
	p.ID = "pec_fly"
	p.Kind = training.KindAccessory
	if _, err := training.NewExercise(p); err == nil {
		t.Error("メインリフトを持つ補助種目が通ってしまう")
	}

	p.MainLift = ""
	if _, err := training.NewExercise(p); err != nil {
		t.Errorf("正常な補助種目が失敗: %v", err)
	}
}

func TestNewExercise_RejectsEmptyStimulus(t *testing.T) {
	p := benchParams()
	p.Stimulus = nil
	if _, err := training.NewExercise(p); err == nil {
		t.Error("どの筋区分にも寄与しない種目が通ってしまう")
	}
}

func TestNewExercise_RejectsUnknownRegion(t *testing.T) {
	p := benchParams()
	p.Stimulus = map[training.MuscleRegion]float64{training.MuscleRegion("NOPE"): 1.0}
	if _, err := training.NewExercise(p); err == nil {
		t.Error("未知の筋区分が通ってしまう")
	}
}

func TestNewExercise_RejectsEmptyID(t *testing.T) {
	p := benchParams()
	p.ID = "  "
	if _, err := training.NewExercise(p); err == nil {
		t.Error("空のIDが通ってしまう")
	}
}

func TestStimulusProfile_RegionsAreSorted(t *testing.T) {
	e, err := training.NewExercise(benchParams())
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	regions := e.Stimulus().Regions()
	for i := 1; i < len(regions); i++ {
		if regions[i-1] >= regions[i] {
			t.Fatalf("Regions がソートされていない: %v", regions)
		}
	}
}

func TestStimulusProfile_Contribution(t *testing.T) {
	e, _ := training.NewExercise(benchParams())
	c, ok := e.Stimulus().Contribution(training.ChestMid)
	if !ok || c.Float() != 1.0 {
		t.Errorf("寄与度が誤り: %v %v", c, ok)
	}
	if _, ok := e.Stimulus().Contribution(training.Calf); ok {
		t.Error("寄与しない区分が取れてしまう")
	}
}

func TestExercise_SameIdentity(t *testing.T) {
	a, _ := training.NewExercise(benchParams())

	p := benchParams()
	p.Name = "別名だが同じID"
	b, _ := training.NewExercise(p)

	if !a.SameIdentity(b) {
		t.Error("同じIDのエンティティが別物と判定された")
	}

	p2 := benchParams()
	p2.ID = "squat"
	p2.MainLift = training.LiftSquat
	c, _ := training.NewExercise(p2)
	if a.SameIdentity(c) {
		t.Error("違うIDのエンティティが同一と判定された")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'Exercise|StimulusProfile'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/exercise.go`:

```go
package training

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ExerciseID は種目の同一性。
type ExerciseID string

func NewExerciseID(s string) (ExerciseID, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", errors.New("種目IDが空である")
	}
	return ExerciseID(trimmed), nil
}

// StimulusProfile は種目が各筋区分へ与える刺激の分布。不変。
type StimulusProfile struct {
	m map[MuscleRegion]Contribution
}

func NewStimulusProfile(m map[MuscleRegion]float64) (StimulusProfile, error) {
	if len(m) == 0 {
		return StimulusProfile{}, errors.New("種目は少なくとも1つの筋区分に寄与する必要がある")
	}
	out := make(map[MuscleRegion]Contribution, len(m))
	for region, v := range m {
		if !region.Valid() {
			return StimulusProfile{}, fmt.Errorf("未知の筋区分: %s", region)
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
// マップの反復順に依存すると同じ入力から違うセッションが生成され、再現性が壊れる。
func (p StimulusProfile) Regions() []MuscleRegion {
	out := make([]MuscleRegion, 0, len(p.m))
	for r := range p.m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (p StimulusProfile) Contribution(r MuscleRegion) (Contribution, bool) {
	c, ok := p.m[r]
	return c, ok
}

func (p StimulusProfile) IsEmpty() bool { return len(p.m) == 0 }

// ExerciseParams は Exercise の生成入力。
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
	id, err := NewExerciseID(p.ID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, fmt.Errorf("種目 %s の名前が空である", id)
	}
	if !p.Kind.Valid() {
		return nil, fmt.Errorf("種目 %s の種別が不正: %s", id, p.Kind)
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
		name:      strings.TrimSpace(p.Name),
		kind:      p.Kind,
		stimulus:  stimulus,
		increment: increment,
	}

	if p.MainLift != "" {
		if !p.MainLift.Valid() {
			return nil, fmt.Errorf("種目 %s のメインリフトが不正: %s", id, p.MainLift)
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
	}
	return nil
}

func (e *Exercise) ID() ExerciseID          { return e.id }
func (e *Exercise) Name() string            { return e.name }
func (e *Exercise) Kind() ExerciseKind      { return e.kind }
func (e *Exercise) Stimulus() StimulusProfile { return e.stimulus }
func (e *Exercise) Increment() Increment    { return e.increment }

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
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'Exercise|StimulusProfile'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/exercise.go internal/domain/training/exercise_test.go
git commit -m "feat(domain): Exercise エンティティと StimulusProfile を追加する"
```

---

### Task 7: SetLog エンティティ

**Files:**
- Create: `internal/domain/training/set_log.go`
- Test: `internal/domain/training/set_log_test.go`

**Interfaces:**
- Consumes: Task 2 の `Date`、Task 4 の計測系、Task 5 の `EstimateOneRepMax`、Task 6 の `ExerciseID`
- Produces:
  - `type SetLogID string` / `func NewSetLogID(s string) (SetLogID, error)`
  - `type SetLogParams struct{ ID string; PerformedOn Date; ExerciseID string; WeightKg float64; Reps int; RIR int }`
  - `func NewSetLog(p SetLogParams) (*SetLog, error)`
  - アクセサ `ID()` / `PerformedOn()` / `ExerciseID()` / `Weight()` / `Reps()` / `RIR()`
  - `func (s *SetLog) EstimatedOneRepMax() (OneRepMax, bool)` — 推定できない場合は false

ID はクライアントが採番した ULID を受け取る。サーバーが振り直さないのは、同じログを二度送っても壊れない冪等性のため。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/set_log_test.go`:

```go
package training_test

import (
	"math"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func setLogParams() training.SetLogParams {
	return training.SetLogParams{
		ID:          "01J0000000000000000000BENCH",
		PerformedOn: training.NewDate(2026, time.August, 16),
		ExerciseID:  "bench",
		WeightKg:    85,
		Reps:        9,
		RIR:         2,
	}
}

func TestNewSetLog(t *testing.T) {
	s, err := training.NewSetLog(setLogParams())
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	if s.Weight().Kg() != 85 {
		t.Errorf("重量が誤り: %v", s.Weight().Kg())
	}
	if s.Reps().Int() != 9 || s.RIR().Int() != 2 {
		t.Errorf("レップ/RIRが誤り: %d %d", s.Reps().Int(), s.RIR().Int())
	}
	if s.ExerciseID() != training.ExerciseID("bench") {
		t.Errorf("種目IDが誤り: %v", s.ExerciseID())
	}
}

func TestNewSetLog_RejectsEmptyID(t *testing.T) {
	p := setLogParams()
	p.ID = ""
	if _, err := training.NewSetLog(p); err == nil {
		t.Error("空のIDが通ってしまう")
	}
}

func TestNewSetLog_RejectsZeroDate(t *testing.T) {
	p := setLogParams()
	p.PerformedOn = training.Date{}
	if _, err := training.NewSetLog(p); err == nil {
		t.Error("日付なしのログが通ってしまう")
	}
}

func TestNewSetLog_RejectsInvalidMeasures(t *testing.T) {
	p := setLogParams()
	p.Reps = 0
	if _, err := training.NewSetLog(p); err == nil {
		t.Error("0レップが通ってしまう")
	}

	p = setLogParams()
	p.RIR = -1
	if _, err := training.NewSetLog(p); err == nil {
		t.Error("負のRIRが通ってしまう")
	}

	p = setLogParams()
	p.WeightKg = -5
	if _, err := training.NewSetLog(p); err == nil {
		t.Error("負の重量が通ってしまう")
	}
}

func TestSetLog_EstimatedOneRepMax(t *testing.T) {
	s, _ := training.NewSetLog(setLogParams())
	want := 85.0 * (1 + 11.0/30.0)
	if got := s.EstimatedOneRepMax().Kg(); math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'SetLog'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/set_log.go`:

```go
package training

import (
	"errors"
	"fmt"
	"strings"
)

// SetLogID は実績1セットの同一性。
// クライアントが採番した ULID をそのまま使う。サーバーが振り直さないのは、
// 同じログを二度送っても壊れない冪等性を保つため。
type SetLogID string

func NewSetLogID(s string) (SetLogID, error) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", errors.New("セットログIDが空である")
	}
	return SetLogID(trimmed), nil
}

type SetLogParams struct {
	ID          string
	PerformedOn Date
	ExerciseID  string
	WeightKg    float64
	Reps        int
	RIR         int
}

// SetLog は確定した実績。エンジンにとって唯一の真実であり、生成後は変更しない。
type SetLog struct {
	id          SetLogID
	performedOn Date
	exerciseID  ExerciseID
	weight      Weight
	reps        Reps
	rir         RIR
}

func NewSetLog(p SetLogParams) (*SetLog, error) {
	id, err := NewSetLogID(p.ID)
	if err != nil {
		return nil, err
	}
	if p.PerformedOn.IsZero() {
		return nil, fmt.Errorf("セットログ %s: 実施日が無い", id)
	}
	exerciseID, err := NewExerciseID(p.ExerciseID)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}
	weight, err := NewWeight(p.WeightKg)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}
	reps, err := NewReps(p.Reps)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}
	rir, err := NewRIR(p.RIR)
	if err != nil {
		return nil, fmt.Errorf("セットログ %s: %w", id, err)
	}

	return &SetLog{
		id:          id,
		performedOn: p.PerformedOn,
		exerciseID:  exerciseID,
		weight:      weight,
		reps:        reps,
		rir:         rir,
	}, nil
}

func (s *SetLog) ID() SetLogID          { return s.id }
func (s *SetLog) PerformedOn() Date     { return s.performedOn }
func (s *SetLog) ExerciseID() ExerciseID { return s.exerciseID }
func (s *SetLog) Weight() Weight        { return s.weight }
func (s *SetLog) Reps() Reps            { return s.reps }
func (s *SetLog) RIR() RIR              { return s.rir }

// EstimatedOneRepMax はこの1セットから推定される1RM。
func (s *SetLog) EstimatedOneRepMax() OneRepMax {
	return EstimateOneRepMax(s.weight, s.reps, s.rir)
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'SetLog'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/set_log.go internal/domain/training/set_log_test.go
git commit -m "feat(domain): SetLog エンティティを追加する"
```

---

### Task 8: History と TrainingSession

**Files:**
- Create: `internal/domain/training/history.go`
- Test: `internal/domain/training/history_test.go`

**Interfaces:**
- Consumes: Task 7 の `SetLog`、Task 2 の `Date`
- Produces:
  - `type TrainingSession struct{...}` / `func (s TrainingSession) Date() Date` / `func (s TrainingSession) Logs() []*SetLog` / `func (s TrainingSession) MedianOneRepMax() (OneRepMax, bool)`
  - `type History struct{...}` / `func NewHistory(logs []*SetLog) History`
  - `func (h History) IsEmpty() bool` / `func (h History) Logs() []*SetLog`
  - `func (h History) ForExercise(id ExerciseID) History`
  - `func (h History) OnOrAfter(d Date) History`
  - `func (h History) Before(d Date) History`
  - `func (h History) Sessions() []TrainingSession` — 日付昇順
  - `func (h History) SessionCount() int`
  - `func (h History) LastPerformed(id ExerciseID) (Date, bool)`

`MedianOneRepMax` に中央値を使うのは、セッション内に1セットだけ異常な記録が混ざっても引きずられないため。仕様の「明らかな外れ値は除外する」をこの形で満たす。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/history_test.go`:

```go
package training_test

import (
	"math"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mkLog(t *testing.T, id string, day int, exercise string, kg float64, reps, rir int) *training.SetLog {
	t.Helper()
	s, err := training.NewSetLog(training.SetLogParams{
		ID:          id,
		PerformedOn: training.NewDate(2026, time.August, day),
		ExerciseID:  exercise,
		WeightKg:    kg,
		Reps:        reps,
		RIR:         rir,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	return s
}

func TestHistory_Empty(t *testing.T) {
	h := training.NewHistory(nil)
	if !h.IsEmpty() {
		t.Error("空の履歴が IsEmpty でない")
	}
	if len(h.Sessions()) != 0 {
		t.Error("空の履歴からセッションが出てくる")
	}
	if _, ok := h.LastPerformed("bench"); ok {
		t.Error("空の履歴で最終実施日が取れてしまう")
	}
}

func TestHistory_SessionsAreGroupedAndSorted(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "c", 20, "bench", 85, 8, 2),
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "b", 10, "bench", 80, 8, 2),
	})

	sessions := h.Sessions()
	if len(sessions) != 2 {
		t.Fatalf("セッション数が誤り: %d", len(sessions))
	}
	if !sessions[0].Date().Before(sessions[1].Date()) {
		t.Error("セッションが日付昇順になっていない")
	}
	if len(sessions[0].Logs()) != 2 {
		t.Errorf("同日ログがまとまっていない: %d", len(sessions[0].Logs()))
	}
	if h.SessionCount() != 2 {
		t.Errorf("SessionCount が誤り: %d", h.SessionCount())
	}
}

func TestHistory_ForExercise(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "b", 10, "squat", 100, 8, 2),
	})
	only := h.ForExercise("bench")
	if len(only.Logs()) != 1 {
		t.Fatalf("絞り込みが誤り: %d", len(only.Logs()))
	}
	if only.Logs()[0].ExerciseID() != training.ExerciseID("bench") {
		t.Error("違う種目が混ざっている")
	}
}

func TestHistory_DateFilters(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "b", 15, "bench", 82.5, 8, 2),
		mkLog(t, "c", 20, "bench", 85, 8, 2),
	})
	cut := training.NewDate(2026, time.August, 15)

	if got := len(h.OnOrAfter(cut).Logs()); got != 2 {
		t.Errorf("OnOrAfter が誤り: %d", got)
	}
	if got := len(h.Before(cut).Logs()); got != 1 {
		t.Errorf("Before が誤り: %d", got)
	}
}

func TestHistory_LastPerformed(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "b", 20, "bench", 85, 8, 2),
	})
	got, ok := h.LastPerformed("bench")
	if !ok {
		t.Fatal("最終実施日が取れない")
	}
	if want := training.NewDate(2026, time.August, 20); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTrainingSession_MedianResistsOutlier(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 85, 9, 2),
		mkLog(t, "b", 10, "bench", 85, 9, 2),
		mkLog(t, "c", 10, "bench", 85, 20, 5), // 外れ値
	})
	session := h.Sessions()[0]
	got, ok := session.MedianOneRepMax()
	if !ok {
		t.Fatal("中央値が取れない")
	}
	want := 85.0 * (1 + 11.0/30.0)
	if math.Abs(got.Kg()-want) > 1e-9 {
		t.Errorf("外れ値に引きずられている: got %v, want %v", got.Kg(), want)
	}
}

func TestTrainingSession_MedianOfEvenCount(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 10, 0),
		mkLog(t, "b", 10, "bench", 90, 10, 0),
	})
	got, _ := h.Sessions()[0].MedianOneRepMax()
	want := (80.0*(1+10.0/30.0) + 90.0*(1+10.0/30.0)) / 2
	if math.Abs(got.Kg()-want) > 1e-9 {
		t.Errorf("偶数個の中央値が誤り: got %v, want %v", got.Kg(), want)
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'History|TrainingSession'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/history.go`:

```go
package training

import "sort"

// TrainingSession は同一日に実施されたセットのまとまり。
type TrainingSession struct {
	date Date
	logs []*SetLog
}

func (s TrainingSession) Date() Date { return s.date }

func (s TrainingSession) Logs() []*SetLog {
	out := make([]*SetLog, len(s.logs))
	copy(out, s.logs)
	return out
}

// MedianOneRepMax はこのセッションを代表する推定1RM。
//
// 中央値を使うのは、セッション内に1セットだけ異常な記録が混ざっても
// 引きずられないため。仕様の「明らかな外れ値は除外する」をこの形で満たす。
// 推定できないセット（自重種目、Epley 式の適用範囲外）は必ず除外する。
// 0 として混ぜると、加重ディップス2セット + 自重3セットのような
// 日常的なセッションで代表値が 0 に崩壊する。
//
// 生成は必ず NewOneRepMax を通す。構造体リテラルで組み立てると、
// コンストラクタが拒否する値（0 など）が ok=true で流通してしまう。
func (s TrainingSession) MedianOneRepMax() (OneRepMax, bool) {
	values := make([]float64, 0, len(s.logs))
	for _, l := range s.logs {
		if v, ok := l.EstimatedOneRepMax(); ok {
			values = append(values, v.Kg())
		}
	}
	if len(values) == 0 {
		return OneRepMax{}, false
	}

	orm, err := NewOneRepMax(median(values))
	if err != nil {
		return OneRepMax{}, false
	}
	return orm, true
}

// median は昇順ソートした上での中央値。呼び出し側が非空を保証すること。
func median(values []float64) float64 {
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// History は確定した実績の集まり。読み取り専用に扱う。
type History struct {
	logs []*SetLog
}

func NewHistory(logs []*SetLog) History {
	out := make([]*SetLog, 0, len(logs))
	for _, l := range logs {
		if l != nil {
			out = append(out, l)
		}
	}
	return History{logs: out}
}

func (h History) IsEmpty() bool { return len(h.logs) == 0 }

func (h History) Logs() []*SetLog {
	out := make([]*SetLog, len(h.logs))
	copy(out, h.logs)
	return out
}

func (h History) filter(keep func(*SetLog) bool) History {
	out := make([]*SetLog, 0, len(h.logs))
	for _, l := range h.logs {
		if keep(l) {
			out = append(out, l)
		}
	}
	return History{logs: out}
}

func (h History) ForExercise(id ExerciseID) History {
	return h.filter(func(l *SetLog) bool { return l.ExerciseID() == id })
}

// OnOrAfter は d を含むそれ以降。
func (h History) OnOrAfter(d Date) History {
	return h.filter(func(l *SetLog) bool { return !l.PerformedOn().Before(d) })
}

// Before は d を含まないそれ以前。
func (h History) Before(d Date) History {
	return h.filter(func(l *SetLog) bool { return l.PerformedOn().Before(d) })
}

// Sessions は同一日ごとにまとめ、日付昇順で返す。
func (h History) Sessions() []TrainingSession {
	grouped := map[string][]*SetLog{}
	dates := map[string]Date{}
	for _, l := range h.logs {
		key := l.PerformedOn().String()
		grouped[key] = append(grouped[key], l)
		dates[key] = l.PerformedOn()
	}

	keys := make([]string, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Strings(keys) // ISO形式なので辞書順＝日付順

	out := make([]TrainingSession, 0, len(keys))
	for _, k := range keys {
		out = append(out, TrainingSession{date: dates[k], logs: grouped[k]})
	}
	return out
}

func (h History) SessionCount() int { return len(h.Sessions()) }

// LastPerformed はその種目を最後に実施した日。履歴が無ければ false。
func (h History) LastPerformed(id ExerciseID) (Date, bool) {
	var last Date
	found := false
	for _, l := range h.logs {
		if l.ExerciseID() != id {
			continue
		}
		if !found || last.Before(l.PerformedOn()) {
			last = l.PerformedOn()
			found = true
		}
	}
	return last, found
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'History|TrainingSession'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/history.go internal/domain/training/history_test.go
git commit -m "feat(domain): History と TrainingSession を追加する"
```

---

### Task 9: OneRepMaxEstimator（EWMA とヒステリシス）

**Files:**
- Create: `internal/domain/training/one_rep_max_estimator.go`
- Test: `internal/domain/training/one_rep_max_estimator_test.go`

**Interfaces:**
- Consumes: Task 8 の `History`、Task 5 の `OneRepMax`
- Produces:
  - `type OneRepMaxEstimator struct{...}`（無状態のドメインサービス）
  - `func NewOneRepMaxEstimator(alpha float64, maxStaleDays int) (OneRepMaxEstimator, error)`
  - `func DefaultOneRepMaxEstimator() OneRepMaxEstimator` — alpha 0.3、鮮度の上限 42日
  - `func (e OneRepMaxEstimator) Estimate(h History, id ExerciseID, asOf Date) (OneRepMax, bool)`

単発の記録で全スロットの重量が動くと不安定になる。調子が良かった日の1セットで重量が跳ね上がり、翌週それを引きずって潰れるのを防ぐ。手順は「セッションごとの中央値 → 古い順に EWMA → ヒステリシス」。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/one_rep_max_estimator_test.go`:

```go
package training_test

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestOneRepMaxEstimator_NoHistory(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	if _, ok := est.Estimate(training.NewHistory(nil), "bench", nil); ok {
		t.Error("履歴が無いのに推定値が返る")
	}
}

func TestOneRepMaxEstimator_SingleSession(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 85, 9, 2),
	})
	got, ok := est.Estimate(h, "bench", nil)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	want := 85.0 * (1 + 11.0/30.0)
	if math.Abs(got.Kg()-want) > 1e-9 {
		t.Errorf("got %v, want %v", got.Kg(), want)
	}
}

func TestOneRepMaxEstimator_SmoothsAcrossSessions(t *testing.T) {
	est, err := training.NewOneRepMaxEstimator(0.5, 0.02)
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "bench", 80, 8, 2),
		mkLog(t, "b", 8, "bench", 90, 8, 2),
	})
	got, _ := est.Estimate(h, "bench", nil)

	old := 80.0 * (1 + 10.0/30.0)
	recent := 90.0 * (1 + 10.0/30.0)
	if got.Kg() <= old {
		t.Errorf("新しい記録が反映されていない: %v", got.Kg())
	}
	if got.Kg() >= recent {
		t.Errorf("平滑化されず直近値そのものになっている: %v", got.Kg())
	}
}

func TestOneRepMaxEstimator_HysteresisHoldsSmallChange(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 85, 9, 2),
	})
	candidate := 85.0 * (1 + 11.0/30.0)

	// 1%だけ低い前回値を渡すと、閾値2%未満なので前回値が維持される
	prev, err := training.NewOneRepMax(candidate * 0.99)
	if err != nil {
		t.Fatalf("NewOneRepMax: %v", err)
	}
	got, _ := est.Estimate(h, "bench", &prev)
	if math.Abs(got.Kg()-prev.Kg()) > 1e-9 {
		t.Errorf("微小変化でヒステリシスが効いていない: got %v, want %v", got.Kg(), prev.Kg())
	}
}

func TestOneRepMaxEstimator_HysteresisPassesLargeChange(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 85, 9, 2),
	})
	candidate := 85.0 * (1 + 11.0/30.0)

	prev, _ := training.NewOneRepMax(candidate * 0.8) // 20%低い
	got, _ := est.Estimate(h, "bench", &prev)
	if math.Abs(got.Kg()-candidate) > 1e-9 {
		t.Errorf("大きな変化が通っていない: got %v, want %v", got.Kg(), candidate)
	}
}

func TestNewOneRepMaxEstimator_RejectsBadParams(t *testing.T) {
	if _, err := training.NewOneRepMaxEstimator(0, 0.02); err == nil {
		t.Error("alpha 0 が通ってしまう")
	}
	if _, err := training.NewOneRepMaxEstimator(1.5, 0.02); err == nil {
		t.Error("alpha 1.5 が通ってしまう")
	}
	if _, err := training.NewOneRepMaxEstimator(0.3, -0.1); err == nil {
		t.Error("負のヒステリシスが通ってしまう")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'OneRepMaxEstimator'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/one_rep_max_estimator.go`:

```go
package training

import (
	"fmt"
	"math"
)

const (
	defaultEstimatorAlpha      = 0.3
	defaultEstimatorHysteresis = 0.02
)

// OneRepMaxEstimator は履歴から推定1RMを導くドメインサービス。無状態。
//
// 単発の記録で全スロットの重量が動くと不安定になる。調子が良かった日の
// 1セットで重量が跳ね上がり、翌週それを引きずって潰れるのを防ぐため、
// セッション中央値 → EWMA → ヒステリシス の順に均す。
type OneRepMaxEstimator struct {
	alpha      float64
	hysteresis float64
}

func NewOneRepMaxEstimator(alpha float64, maxStaleDays int) (OneRepMaxEstimator, error) {
	if math.IsNaN(alpha) || alpha <= 0 || alpha > 1 {
		return OneRepMaxEstimator{}, fmt.Errorf("alpha は0より大きく1以下である必要がある: %v", alpha)
	}
	if math.IsNaN(hysteresis) || hysteresis < 0 || hysteresis >= 1 {
		return OneRepMaxEstimator{}, fmt.Errorf("ヒステリシスは0以上1未満である必要がある: %v", hysteresis)
	}
	return OneRepMaxEstimator{alpha: alpha, hysteresis: hysteresis}, nil
}

func DefaultOneRepMaxEstimator() OneRepMaxEstimator {
	return OneRepMaxEstimator{alpha: defaultEstimatorAlpha, hysteresis: defaultEstimatorHysteresis}
}

// Estimate は指定種目の平滑化された推定1RM。履歴が無ければ false。
//
// previous に前回公表した値を渡すと、変化がヒステリシス閾値未満のとき
// 前回値を維持する。重量が毎回ちらつくのを防ぐ。
func (e OneRepMaxEstimator) Estimate(h History, id ExerciseID, previous *OneRepMax) (OneRepMax, bool) {
	sessions := h.ForExercise(id).Sessions()
	if len(sessions) == 0 {
		return OneRepMax{}, false
	}

	var acc float64
	for i, s := range sessions {
		m, ok := s.MedianOneRepMax()
		if !ok {
			continue
		}
		if i == 0 {
			acc = m.Kg()
			continue
		}
		acc = e.alpha*m.Kg() + (1-e.alpha)*acc
	}
	if acc <= 0 {
		return OneRepMax{}, false
	}

	candidate := OneRepMax{kg: acc}
	if previous == nil || previous.Kg() <= 0 {
		return candidate, true
	}
	change := math.Abs(candidate.Kg()-previous.Kg()) / previous.Kg()
	if change < e.hysteresis {
		return *previous, true
	}
	return candidate, true
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'OneRepMaxEstimator'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/one_rep_max_estimator.go internal/domain/training/one_rep_max_estimator_test.go
git commit -m "feat(domain): 推定1RMの平滑化サービスを追加する"
```

---

### Task 10: VariationRatioResolver（対メイン係数の学習）

**Files:**
- Create: `internal/domain/training/variation_ratio.go`
- Test: `internal/domain/training/variation_ratio_test.go`

**Interfaces:**
- Consumes: Task 6 の `Exercise`、Task 8 の `History`、Task 9 の `OneRepMaxEstimator`
- Produces:
  - `type VariationRatioResolver struct{...}`
  - `func NewVariationRatioResolver(est OneRepMaxEstimator, minSessions int) (VariationRatioResolver, error)`（`Resolve` は `asOf Date` を受け取る）
  - `func DefaultVariationRatioResolver() VariationRatioResolver` — minSessions 3
  - `func (r VariationRatioResolver) Resolve(h History, variation *Exercise, mainID ExerciseID) Ratio`

ラーセンプレスやテンポは通常フォームより挙がらない。同じ推定1RMの物差しに乗せるため比率で換算する。**シードが持つ初期値は「最初の一歩を踏み出すための仮の値」であり正確である必要はない。** minSessions 回こなせば実測値に置き換わる。

実測比が現実的な範囲（`NewRatio` の制約）を外れた場合は初期値へフォールバックする。記録ミスで係数が壊れると、以後の全セッションの重量が狂うため。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/variation_ratio_test.go`:

```go
package training_test

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func larsen(t *testing.T) *training.Exercise {
	t.Helper()
	e, err := training.NewExercise(training.ExerciseParams{
		ID:                 "larsen",
		Name:               "ラーセンプレス",
		Kind:               training.KindVariation,
		Stimulus:           map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg:        2.5,
		MainLift:           training.LiftBench,
		DefaultRatioToMain: 0.85,
	})
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	return e
}

func TestVariationRatioResolver_FallsBackWhenTooFewSessions(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "m1", 1, "bench", 100, 8, 2),
		mkLog(t, "v1", 2, "larsen", 80, 8, 2),
	})
	if got := r.Resolve(h, larsen(t), "bench").Float(); math.Abs(got-0.85) > 1e-9 {
		t.Errorf("初期値にフォールバックしていない: %v", got)
	}
}

func TestVariationRatioResolver_UsesMeasuredRatio(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "m1", 1, "bench", 100, 8, 2),
		mkLog(t, "m2", 8, "bench", 100, 8, 2),
		mkLog(t, "m3", 15, "bench", 100, 8, 2),
		mkLog(t, "v1", 2, "larsen", 80, 8, 2),
		mkLog(t, "v2", 9, "larsen", 80, 8, 2),
		mkLog(t, "v3", 16, "larsen", 80, 8, 2),
	})
	// 同じレップ・RIRなので推定1RMの比は重量の比に一致する
	if got := r.Resolve(h, larsen(t), "bench").Float(); math.Abs(got-0.80) > 1e-9 {
		t.Errorf("実測値になっていない: %v", got)
	}
}

func TestVariationRatioResolver_FallsBackWhenMainMissing(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "v1", 2, "larsen", 80, 8, 2),
		mkLog(t, "v2", 9, "larsen", 80, 8, 2),
		mkLog(t, "v3", 16, "larsen", 80, 8, 2),
	})
	if got := r.Resolve(h, larsen(t), "bench").Float(); math.Abs(got-0.85) > 1e-9 {
		t.Errorf("メイン履歴が無いのに実測を使っている: %v", got)
	}
}

func TestVariationRatioResolver_FallsBackWhenRatioUnrealistic(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	// バリエーションがメインの2倍という記録ミスを想定する
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "m1", 1, "bench", 50, 8, 2),
		mkLog(t, "m2", 8, "bench", 50, 8, 2),
		mkLog(t, "m3", 15, "bench", 50, 8, 2),
		mkLog(t, "v1", 2, "larsen", 200, 8, 2),
		mkLog(t, "v2", 9, "larsen", 200, 8, 2),
		mkLog(t, "v3", 16, "larsen", 200, 8, 2),
	})
	if got := r.Resolve(h, larsen(t), "bench").Float(); math.Abs(got-0.85) > 1e-9 {
		t.Errorf("非現実的な実測値が採用された: %v", got)
	}
}

func TestVariationRatioResolver_NonVariationReturnsOne(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	bench, err := training.NewExercise(benchParams())
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	if got := r.Resolve(training.NewHistory(nil), bench, "bench").Float(); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("メイン種目の係数が1でない: %v", got)
	}
}

func TestNewVariationRatioResolver_RejectsBadMinSessions(t *testing.T) {
	if _, err := training.NewVariationRatioResolver(training.DefaultOneRepMaxEstimator(), 0); err == nil {
		t.Error("minSessions 0 が通ってしまう")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'VariationRatio'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/variation_ratio.go`:

```go
package training

import "fmt"

const defaultMinRatioSessions = 3

// unitRatio は「換算しない」を表す係数。
var unitRatio = Ratio{v: 1.0}

// VariationRatioResolver はバリエーション種目の対メイン係数を決めるドメインサービス。無状態。
//
// シードが持つ初期値は「最初の一歩を踏み出すための仮の値」であり正確である必要はない。
// minSessions 回こなせば実測値に置き換わる。
type VariationRatioResolver struct {
	estimator   OneRepMaxEstimator
	minSessions int
}

func NewVariationRatioResolver(est OneRepMaxEstimator, minSessions int) (VariationRatioResolver, error) {
	if minSessions < 1 {
		return VariationRatioResolver{}, fmt.Errorf("minSessions は1以上である必要がある: %d", minSessions)
	}
	return VariationRatioResolver{estimator: est, minSessions: minSessions}, nil
}

func DefaultVariationRatioResolver() VariationRatioResolver {
	return VariationRatioResolver{
		estimator:   DefaultOneRepMaxEstimator(),
		minSessions: defaultMinRatioSessions,
	}
}

// Resolve は換算係数を返す。バリエーション以外は常に 1.0。
//
// 実測比が現実的な範囲を外れた場合は初期値へ戻す。記録ミスで係数が壊れると
// 以後の全セッションの重量が狂うため。
func (r VariationRatioResolver) Resolve(h History, variation *Exercise, mainID ExerciseID) Ratio {
	if variation == nil || variation.Kind() != KindVariation {
		return unitRatio
	}

	fallback, ok := variation.DefaultRatioToMain()
	if !ok {
		fallback = unitRatio
	}

	if h.ForExercise(variation.ID()).SessionCount() < r.minSessions {
		return fallback
	}

	variationOneRM, ok := r.estimator.Estimate(h, variation.ID(), asOf)
	if !ok {
		return fallback
	}
	mainOneRM, ok := r.estimator.Estimate(h, mainID, asOf)
	if !ok || mainOneRM.Kg() <= 0 {
		return fallback
	}

	measured, err := NewRatio(variationOneRM.Kg() / mainOneRM.Kg())
	if err != nil {
		return fallback
	}
	return measured
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'VariationRatio'`
Expected: PASS

- [ ] **Step 5: 全テストを実行する**

Run: `go test ./...`
Expected: ok

- [ ] **Step 6: コミット**

```bash
git add internal/domain/training/variation_ratio.go internal/domain/training/variation_ratio_test.go
git commit -m "feat(domain): 対メイン係数を実績から学習する"
```

---

## 完了の定義（第2部の範囲）

- `go test ./...` が全件パスする
- `TestDomain_DependsOnNothingOutside` が引き続き通る
- 種目の不変条件（メインはリフト必須、バリエーションは係数必須、補助はリフト禁止）がコンストラクタで守られている
- `StimulusProfile.Regions()` が常にソート済みで、セッション生成の再現性が保たれている

続きは `03-domain-planning.md`。
