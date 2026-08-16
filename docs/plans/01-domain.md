# liftplan サーバー ドメイン層 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 実績ログ・種目プール・週目標・コンディションから、その日のセッションを導出する進行エンジンを、Go のドメイン層として実装する。

**Architecture:** Onion Architecture。この計画は**最も内側の Domain 層だけ**を作る。Domain は Postgres も HTTP も知らず、標準ライブラリ以外に依存しない。Application / Infrastructure / Presentation は後続の計画で外側に巻き付ける。

**Tech Stack:** Go 1.23 / 標準ライブラリのみ（テストも `testing` のみ）

**設計ドキュメント:** `docs/specs/2026-08-16-workout-app-design.md`

## Global Constraints

- 実装先はこのリポジトリ（`~/repos/liftplan-server`）
- モジュール名は `github.com/dyoshyy/liftplan-server`
- **この計画で作るコードは `internal/domain/training` パッケージのみ**。`cmd/`、`internal/application`、`internal/infrastructure`、`internal/presentation` は後続計画で作る
- **Domain 層は外部ライブラリを import してはいけない。** 標準ライブラリのみ。`database/sql`、`net/http`、ORM、Webフレームワークが現れたら設計違反
- **値オブジェクトは不変かつ自己検証。** フィールドは非公開にし、コンストラクタ関数（`NewXxx`）で不変条件を検証してエラーを返す。生成後に状態を変える手段を持たせない
- **エンティティは ID で同一性を持つ。** 値の比較で等価判定しない
- **ドメインサービスは状態を持たない。** 設定値（alpha、閾値など）はコンストラクタで受け取り、以後不変
- **リポジトリのインターフェースは Domain 層に置く。** 実装は Infrastructure（後続計画）
- **ユビキタス言語を型名に反映する。** 仕様書の語（スロット、残差、デロード、対メイン係数）と型名を一致させる
- 現在時刻を関数内で取得しない（`time.Now()` 禁止）。日付は必ず引数で受け取る
- 各タスクは「テストを書く → 失敗を確認 → 実装 → 通ることを確認 → コミット」の順で進める
- テストは日本語の関数名を使わず、`Test<対象>_<条件>` の形式にする。`t.Run` のサブテスト名に日本語を使うのは可

## 前提としている仕様上の判断

1. **全身法なので、毎セッションで SQUAT / BENCH / DEADLIFT の3種目すべてにスロットを割り当てる**
2. **履歴のない種目は重量を算出できない。** 重量は「未確定」を表現できる型で返し、初回だけユーザーが決める。推定に足る履歴が無いのに数字を捏造しない
3. **補助種目は1セッション3種目、各3セット固定**（設定値として注入し、後から変更可能にする）
4. **セッションが週の何本目かは、その週の既存ログから導出する。** 曜日の割り当てはドメインの責務ではない

---

### Task 1: モジュール骨格と依存方向の番人

**Files:**
- Create: `~/repos/liftplan-server/go.mod`
- Create: `~/repos/liftplan-server/.gitignore`
- Create: `~/repos/liftplan-server/internal/domain/training/doc.go`
- Test: `~/repos/liftplan-server/internal/domain/training/architecture_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `internal/domain/training` パッケージ。以降のすべてのタスクがここにコードを置く

このタスクの主眼は、**Domain 層が外側へ依存しないことを機械的に守らせる**こと。人間の規律ではなくテストで守る。

- [ ] **Step 1: リポジトリとモジュールを作る**

```bash
mkdir -p ~/repos/liftplan-server/internal/domain/training
cd ~/repos/liftplan-server
git init
go mod init github.com/dyoshyy/liftplan-server
```

`.gitignore`:

```
/bin/
*.test
.env
```

- [ ] **Step 2: パッケージのドキュメントを書く**

`internal/domain/training/doc.go`:

```go
// Package training はトレーニングの進行を司るドメイン層である。
//
// この層は Onion Architecture の最内周にあたり、外側（Application、
// Infrastructure、Presentation）を一切知らない。標準ライブラリ以外への
// 依存を持たないため、DB も HTTP も立てずに全機能をテストできる。
package training
```

- [ ] **Step 3: 依存方向を検査するテストを書く**

`internal/domain/training/architecture_test.go`:

```go
package training_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 標準ライブラリのうち、ドメイン層で使ってよいもの。
var allowedStdlib = map[string]bool{
	"errors":  true,
	"fmt":     true,
	"math":    true,
	"sort":    true,
	"strings": true,
	"time":    true,
	"testing": true,
	"go/ast":  true, // このテスト自身のため
	"go/parser": true,
	"go/token":  true,
	"os":        true,
	"path/filepath": true,
}

func TestDomain_DependsOnNothingOutside(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ディレクトリを読めない: %v", err)
	}

	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		path := filepath.Join(".", e.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s をパースできない: %v", path, err)
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)

			if strings.HasPrefix(p, "github.com/dyoshyy/liftplan-server/") {
				// 自リポジトリ内は domain 配下のみ許可
				if !strings.Contains(p, "/internal/domain/") {
					t.Errorf("%s: ドメイン層が外側に依存している: %s", path, p)
				}
				continue
			}
			if strings.Contains(p, ".") {
				t.Errorf("%s: ドメイン層が外部ライブラリに依存している: %s", path, p)
				continue
			}
			if !allowedStdlib[p] {
				t.Errorf("%s: 許可されていない標準ライブラリ: %s（意図的なら allowedStdlib に追加すること）", path, p)
			}
		}
	}
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `cd ~/repos/liftplan-server && go test ./internal/domain/training/...`
Expected: PASS（まだ検査対象のファイルが少ないが、以降のタスクで効く）

- [ ] **Step 5: コミット**

```bash
git add -A
git commit -m "chore: ドメイン層の骨格と依存方向の検査を用意する"
```

---

### Task 2: Date 値オブジェクト

**Files:**
- Create: `internal/domain/training/date.go`
- Test: `internal/domain/training/date_test.go`

**Interfaces:**
- Consumes: Task 1 のパッケージ
- Produces:
  - `type Date struct{ ... }`（不変）
  - `func NewDate(year int, month time.Month, day int) (Date, error)`
  - `func MustDate(year int, month time.Month, day int) Date` — テスト専用。実装では `NewDate` を使う

> **契約の修正:** `NewDate` はエラーを返す（2026-08-50 のような日付を黙って正規化しないため）。この文書の以降のサンプルは `NewDate` が値だけを返す前提で書かれているので、テストを書くときは `MustDate` に読み替えること。
  - `func ParseDate(s string) (Date, error)` — `2006-01-02` 形式
  - `func (d Date) AddDays(n int) Date`
  - `func (d Date) Before(o Date) bool` / `After` / `Equal`
  - `func (d Date) DaysSince(o Date) int`
  - `func (d Date) WeekStart() Date` — 月曜
  - `func (d Date) String() string`
  - `func (d Date) IsZero() bool`

`time.Time` をそのまま持ち回ると時刻とタイムゾーンが紛れ込む。トレーニングの記録に必要なのは日付だけなので、専用の値オブジェクトで閉じ込める。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/date_test.go`:

```go
package training_test

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestDate_AddDays(t *testing.T) {
	d := training.NewDate(2026, time.August, 31)
	got := d.AddDays(1)
	want := training.NewDate(2026, time.September, 1)
	if !got.Equal(want) {
		t.Errorf("月をまたぐ加算が誤り: got %v, want %v", got, want)
	}
}

func TestDate_DaysSince(t *testing.T) {
	a := training.NewDate(2026, time.August, 10)
	b := training.NewDate(2026, time.August, 17)
	if got := b.DaysSince(a); got != 7 {
		t.Errorf("日数差が誤り: got %d, want 7", got)
	}
	if got := a.DaysSince(b); got != -7 {
		t.Errorf("逆向きの日数差が誤り: got %d, want -7", got)
	}
}

func TestDate_WeekStartIsMonday(t *testing.T) {
	monday := training.NewDate(2026, time.August, 17)
	for i := 0; i < 7; i++ {
		d := monday.AddDays(i)
		if got := d.WeekStart(); !got.Equal(monday) {
			t.Errorf("%v の週初が誤り: got %v, want %v", d, got, monday)
		}
	}
	next := monday.AddDays(7)
	if got := next.WeekStart(); !got.Equal(next) {
		t.Errorf("翌週の週初が誤り: got %v, want %v", got, next)
	}
}

func TestDate_Ordering(t *testing.T) {
	a := training.NewDate(2026, time.August, 16)
	b := training.NewDate(2026, time.August, 17)
	if !a.Before(b) {
		t.Error("Before が誤り")
	}
	if !b.After(a) {
		t.Error("After が誤り")
	}
	if a.Equal(b) {
		t.Error("Equal が誤り")
	}
}

func TestParseDate(t *testing.T) {
	got, err := training.ParseDate("2026-08-16")
	if err != nil {
		t.Fatalf("パースに失敗: %v", err)
	}
	if want := training.NewDate(2026, time.August, 16); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, err := training.ParseDate("2026/08/16"); err == nil {
		t.Error("不正な形式がエラーにならない")
	}
}

func TestDate_String(t *testing.T) {
	d := training.NewDate(2026, time.August, 6)
	if got := d.String(); got != "2026-08-06" {
		t.Errorf("got %q, want %q", got, "2026-08-06")
	}
}

func TestDate_IsZero(t *testing.T) {
	var zero training.Date
	if !zero.IsZero() {
		t.Error("ゼロ値が IsZero で判定できない")
	}
	if training.NewDate(2026, time.August, 16).IsZero() {
		t.Error("有効な日付が IsZero になっている")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run TestDate`
Expected: コンパイルエラー（`training.NewDate` が未定義）

- [ ] **Step 3: 実装する**

`internal/domain/training/date.go`:

```go
package training

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date は時刻を持たない日付。不変。
// トレーニングの記録に必要なのは日付だけなので、時刻とタイムゾーンを閉じ込める。
type Date struct {
	t time.Time
}

// NewDate は UTC の午前0時に正規化した日付を返す。
func NewDate(year int, month time.Month, day int) Date {
	return Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate は "2006-01-02" 形式の文字列を解釈する。
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("日付として解釈できない %q: %w", s, err)
	}
	return Date{t: t.UTC()}, nil
}

func (d Date) AddDays(n int) Date { return Date{t: d.t.AddDate(0, 0, n)} }

func (d Date) Before(o Date) bool { return d.t.Before(o.t) }
func (d Date) After(o Date) bool  { return d.t.After(o.t) }
func (d Date) Equal(o Date) bool  { return d.t.Equal(o.t) }

// DaysSince は o から見た経過日数。o より前なら負になる。
func (d Date) DaysSince(o Date) int {
	return int(d.t.Sub(o.t).Hours() / 24)
}

// WeekStart はその日が属する週の月曜日を返す。
func (d Date) WeekStart() Date {
	offset := (int(d.t.Weekday()) + 6) % 7 // 月曜を0にする
	return d.AddDays(-offset)
}

func (d Date) String() string { return d.t.Format(dateLayout) }

func (d Date) IsZero() bool { return d.t.IsZero() }
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run TestDate`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/date.go internal/domain/training/date_test.go
git commit -m "feat(domain): Date 値オブジェクトを追加する"
```

---

### Task 3: 分類を表す型（MuscleRegion / ExerciseKind / MainLift）

**Files:**
- Create: `internal/domain/training/taxonomy.go`
- Test: `internal/domain/training/taxonomy_test.go`

**Interfaces:**
- Consumes: Task 1 のパッケージ
- Produces:
  - `type MuscleRegion string` と21個の定数、`func AllMuscleRegions() []MuscleRegion`、`func (r MuscleRegion) Valid() bool`
  - `type ExerciseKind string`（`KindMain` / `KindVariation` / `KindAccessory`）と `Valid()`
  - `type MainLift string`（`LiftSquat` / `LiftBench` / `LiftDeadlift`）と `Valid()`

粗い「部位」ではなくこの粒度で週ボリュームを管理する。胸だけ細かくて他が雑という歪みを避けるため、全身を同じ解像度で持つ。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/taxonomy_test.go`:

```go
package training_test

import (
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestMuscleRegion_Resolution(t *testing.T) {
	all := training.AllMuscleRegions()
	if len(all) < 20 || len(all) > 25 {
		t.Errorf("筋区分は20〜25個であるべきだが %d 個だった", len(all))
	}

	seen := map[training.MuscleRegion]bool{}
	for _, r := range all {
		if seen[r] {
			t.Errorf("筋区分が重複している: %s", r)
		}
		seen[r] = true
		if !r.Valid() {
			t.Errorf("一覧に含まれる筋区分が Valid でない: %s", r)
		}
	}
}

func TestMuscleRegion_ChestIsSplitIntoThree(t *testing.T) {
	// 「ベンチが埋めない大胸筋上部を補助で埋める」という判断を表現できる必要がある
	for _, r := range []training.MuscleRegion{
		training.ChestUpper, training.ChestMid, training.ChestLower,
	} {
		if !r.Valid() {
			t.Errorf("%s が定義されていない", r)
		}
	}
}

func TestMuscleRegion_InvalidValue(t *testing.T) {
	if training.MuscleRegion("NOT_A_REGION").Valid() {
		t.Error("未知の値が Valid になっている")
	}
}

func TestExerciseKind_Valid(t *testing.T) {
	for _, k := range []training.ExerciseKind{
		training.KindMain, training.KindVariation, training.KindAccessory,
	} {
		if !k.Valid() {
			t.Errorf("%s が Valid でない", k)
		}
	}
	if training.ExerciseKind("OTHER").Valid() {
		t.Error("未知の種別が Valid になっている")
	}
}

func TestMainLift_Valid(t *testing.T) {
	for _, l := range []training.MainLift{
		training.LiftSquat, training.LiftBench, training.LiftDeadlift,
	} {
		if !l.Valid() {
			t.Errorf("%s が Valid でない", l)
		}
	}
	if training.MainLift("PRESS").Valid() {
		t.Error("未知のリフトが Valid になっている")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'TestMuscleRegion|TestExerciseKind|TestMainLift'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/taxonomy.go`:

```go
package training

// MuscleRegion は筋区分。粗い「部位」ではなくこの粒度で週ボリュームを管理する。
type MuscleRegion string

const (
	ChestUpper MuscleRegion = "CHEST_UPPER"
	ChestMid   MuscleRegion = "CHEST_MID"
	ChestLower MuscleRegion = "CHEST_LOWER"

	Lat       MuscleRegion = "LAT"
	TrapMid   MuscleRegion = "TRAP_MID"
	TrapUpper MuscleRegion = "TRAP_UPPER"
	Erector   MuscleRegion = "ERECTOR"

	FrontDelt MuscleRegion = "FRONT_DELT"
	SideDelt  MuscleRegion = "SIDE_DELT"
	RearDelt  MuscleRegion = "REAR_DELT"

	TricepsLong    MuscleRegion = "TRICEPS_LONG"
	TricepsLateral MuscleRegion = "TRICEPS_LATERAL"

	Biceps  MuscleRegion = "BICEPS"
	Forearm MuscleRegion = "FOREARM"

	Quad      MuscleRegion = "QUAD"
	Hamstring MuscleRegion = "HAMSTRING"
	Glute     MuscleRegion = "GLUTE"
	Adductor  MuscleRegion = "ADDUCTOR"
	Calf      MuscleRegion = "CALF"

	Abs     MuscleRegion = "ABS"
	Oblique MuscleRegion = "OBLIQUE"
)

var allMuscleRegions = []MuscleRegion{
	ChestUpper, ChestMid, ChestLower,
	Lat, TrapMid, TrapUpper, Erector,
	FrontDelt, SideDelt, RearDelt,
	TricepsLong, TricepsLateral,
	Biceps, Forearm,
	Quad, Hamstring, Glute, Adductor, Calf,
	Abs, Oblique,
}

// AllMuscleRegions は全筋区分のコピーを返す。
func AllMuscleRegions() []MuscleRegion {
	out := make([]MuscleRegion, len(allMuscleRegions))
	copy(out, allMuscleRegions)
	return out
}

func (r MuscleRegion) Valid() bool {
	for _, v := range allMuscleRegions {
		if v == r {
			return true
		}
	}
	return false
}

// ExerciseKind は種目の役割。
//
//	KindMain      … 通常フォームのメイン種目
//	KindVariation … メインの派生（ラーセン、テンポなど）。対メイン係数を持つ
//	KindAccessory … 補助種目。残差を埋めるために選ばれる
type ExerciseKind string

const (
	KindMain      ExerciseKind = "MAIN"
	KindVariation ExerciseKind = "VARIATION"
	KindAccessory ExerciseKind = "ACCESSORY"
)

func (k ExerciseKind) Valid() bool {
	switch k {
	case KindMain, KindVariation, KindAccessory:
		return true
	}
	return false
}

// MainLift は週内スロットで強度帯を振り分ける対象。
type MainLift string

const (
	LiftSquat    MainLift = "SQUAT"
	LiftBench    MainLift = "BENCH"
	LiftDeadlift MainLift = "DEADLIFT"
)

func (l MainLift) Valid() bool {
	switch l {
	case LiftSquat, LiftBench, LiftDeadlift:
		return true
	}
	return false
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'TestMuscleRegion|TestExerciseKind|TestMainLift'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/taxonomy.go internal/domain/training/taxonomy_test.go
git commit -m "feat(domain): 筋区分と種目分類を追加する"
```

---

### Task 4: 計測系の値オブジェクト

**Files:**
- Create: `internal/domain/training/measures.go`
- Test: `internal/domain/training/measures_test.go`

**Interfaces:**
- Consumes: Task 1 のパッケージ
- Produces（すべて不変・自己検証。ゼロ値は無効とし、必ずコンストラクタ経由で作る）:
  - `type Weight struct{...}` / `func NewWeight(kg float64) (Weight, error)` / `func (w Weight) Kg() float64` / `func (w Weight) RoundTo(inc Increment) Weight` / `func (w Weight) Scale(f float64) Weight`
  - `type Reps struct{...}` / `func NewReps(v int) (Reps, error)` / `func (r Reps) Int() int`
  - `type RIR struct{...}` / `func NewRIR(v int) (RIR, error)` / `func (r RIR) Int() int` / `func (r RIR) Plus(n int) RIR`
  - `type Increment struct{...}` / `func NewIncrement(kg float64) (Increment, error)` / `func (i Increment) Kg() float64`
  - `type IntensityPct struct{...}` / `func NewIntensityPct(v float64) (IntensityPct, error)` / `func (i IntensityPct) Float() float64` / `func (i IntensityPct) Scale(f float64) IntensityPct`
  - `type Ratio struct{...}` / `func NewRatio(v float64) (Ratio, error)` / `func (r Ratio) Float() float64`
  - `type SetCount struct{...}` / `func NewSetCount(v int) (SetCount, error)` / `func (s SetCount) Int() int`
  - `type Contribution struct{...}` / `func NewContribution(v float64) (Contribution, error)` / `func (c Contribution) Float() float64`

`RIR.Plus` が上限を超えないよう内部で丸めるのは、睡眠不足による +1 補正を安全に適用するため。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/measures_test.go`:

```go
package training_test

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mustWeight(t *testing.T, kg float64) training.Weight {
	t.Helper()
	w, err := training.NewWeight(kg)
	if err != nil {
		t.Fatalf("NewWeight(%v) が失敗: %v", kg, err)
	}
	return w
}

func mustIncrement(t *testing.T, kg float64) training.Increment {
	t.Helper()
	i, err := training.NewIncrement(kg)
	if err != nil {
		t.Fatalf("NewIncrement(%v) が失敗: %v", kg, err)
	}
	return i
}

func TestWeight_RejectsNegative(t *testing.T) {
	if _, err := training.NewWeight(-1); err == nil {
		t.Error("負の重量が通ってしまう")
	}
	if _, err := training.NewWeight(math.NaN()); err == nil {
		t.Error("NaN が通ってしまう")
	}
}

func TestWeight_RoundTo(t *testing.T) {
	inc := mustIncrement(t, 2.5)
	cases := []struct {
		in   float64
		want float64
	}{
		{83.1, 82.5},
		{84.0, 85.0},
		{85.0, 85.0},
	}
	for _, c := range cases {
		got := mustWeight(t, c.in).RoundTo(inc).Kg()
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("RoundTo(%v) = %v, want %v", c.in, got, c.want)
		}
	}

	if got := mustWeight(t, 83.0).RoundTo(mustIncrement(t, 5.0)).Kg(); math.Abs(got-85.0) > 1e-9 {
		t.Errorf("5kg刻みの丸めが誤り: got %v, want 85", got)
	}
}

func TestWeight_Scale(t *testing.T) {
	got := mustWeight(t, 100).Scale(0.9).Kg()
	if math.Abs(got-90) > 1e-9 {
		t.Errorf("got %v, want 90", got)
	}
}

func TestReps_MustBePositive(t *testing.T) {
	if _, err := training.NewReps(0); err == nil {
		t.Error("0レップが通ってしまう")
	}
	r, err := training.NewReps(8)
	if err != nil {
		t.Fatalf("正常値が失敗: %v", err)
	}
	if r.Int() != 8 {
		t.Errorf("got %d, want 8", r.Int())
	}
}

func TestRIR_RejectsNegative(t *testing.T) {
	if _, err := training.NewRIR(-1); err == nil {
		t.Error("負のRIRが通ってしまう")
	}
}

func TestRIR_PlusStaysInRange(t *testing.T) {
	r, _ := training.NewRIR(2)
	if got := r.Plus(1).Int(); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
	// 下限を割らない
	if got := r.Plus(-10).Int(); got != 0 {
		t.Errorf("下限で丸められていない: got %d, want 0", got)
	}
}

func TestIntensityPct_Range(t *testing.T) {
	if _, err := training.NewIntensityPct(0); err == nil {
		t.Error("0%が通ってしまう")
	}
	if _, err := training.NewIntensityPct(1.5); err == nil {
		t.Error("150%が通ってしまう")
	}
	i, err := training.NewIntensityPct(0.81)
	if err != nil {
		t.Fatalf("正常値が失敗: %v", err)
	}
	if math.Abs(i.Scale(0.9).Float()-0.729) > 1e-9 {
		t.Errorf("Scale が誤り: got %v", i.Scale(0.9).Float())
	}
}

func TestRatio_Range(t *testing.T) {
	if _, err := training.NewRatio(0); err == nil {
		t.Error("0の係数が通ってしまう")
	}
	if _, err := training.NewRatio(3); err == nil {
		t.Error("非現実的な係数が通ってしまう")
	}
	if _, err := training.NewRatio(0.85); err != nil {
		t.Errorf("正常値が失敗: %v", err)
	}
}

func TestSetCount_MustBePositive(t *testing.T) {
	if _, err := training.NewSetCount(0); err == nil {
		t.Error("0セットが通ってしまう")
	}
}

func TestContribution_Range(t *testing.T) {
	if _, err := training.NewContribution(0); err == nil {
		t.Error("寄与度0が通ってしまう")
	}
	if _, err := training.NewContribution(1.1); err == nil {
		t.Error("寄与度が1を超えて通ってしまう")
	}
	if _, err := training.NewContribution(1.0); err != nil {
		t.Errorf("寄与度1.0が失敗: %v", err)
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'TestWeight|TestReps|TestRIR|TestIntensity|TestRatio|TestSetCount|TestContribution'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/measures.go`:

```go
package training

import (
	"errors"
	"fmt"
	"math"
)

// 係数の現実的な上限。これを超える値は入力ミスとして弾く。
const maxRatio = 1.2

// Increment はジムのプレート構成に対応する重量の刻み。
type Increment struct {
	kg float64
}

func NewIncrement(kg float64) (Increment, error) {
	if math.IsNaN(kg) || kg <= 0 {
		return Increment{}, fmt.Errorf("増加単位は正の数である必要がある: %v", kg)
	}
	return Increment{kg: kg}, nil
}

func (i Increment) Kg() float64 { return i.kg }

// Weight は重量（kg）。不変。
type Weight struct {
	kg float64
}

func NewWeight(kg float64) (Weight, error) {
	if math.IsNaN(kg) || math.IsInf(kg, 0) {
		return Weight{}, errors.New("重量が数値ではない")
	}
	if kg < 0 {
		return Weight{}, fmt.Errorf("重量は0以上である必要がある: %v", kg)
	}
	return Weight{kg: kg}, nil
}

func (w Weight) Kg() float64 { return w.kg }

// RoundTo は増加単位へ丸める。丸めの結果は必ず有効な重量になるためエラーを返さない。
func (w Weight) RoundTo(inc Increment) Weight {
	return Weight{kg: math.Round(w.kg/inc.kg) * inc.kg}
}

// Scale は倍率を掛ける。負の倍率は0に丸める。
func (w Weight) Scale(f float64) Weight {
	v := w.kg * f
	if math.IsNaN(v) || v < 0 {
		v = 0
	}
	return Weight{kg: v}
}

// Reps は実際に挙げた回数。1以上。
type Reps struct {
	v int
}

func NewReps(v int) (Reps, error) {
	if v < 1 {
		return Reps{}, fmt.Errorf("レップ数は1以上である必要がある: %d", v)
	}
	return Reps{v: v}, nil
}

func (r Reps) Int() int { return r.v }

// RIR は限界までの残りレップ数（Reps In Reserve）。
// 調整ダイヤルではなく「止め時」を表すガードレールとして使う。
type RIR struct {
	v int
}

func NewRIR(v int) (RIR, error) {
	if v < 0 {
		return RIR{}, fmt.Errorf("RIRは0以上である必要がある: %d", v)
	}
	return RIR{v: v}, nil
}

func (r RIR) Int() int { return r.v }

// Plus は補正を加える。下限0で丸めるため、常に有効な値を返す。
func (r RIR) Plus(n int) RIR {
	v := r.v + n
	if v < 0 {
		v = 0
	}
	return RIR{v: v}
}

// IntensityPct は推定1RMに対する割合。
type IntensityPct struct {
	v float64
}

func NewIntensityPct(v float64) (IntensityPct, error) {
	if math.IsNaN(v) || v <= 0 || v > 1 {
		return IntensityPct{}, fmt.Errorf("強度は0より大きく1以下である必要がある: %v", v)
	}
	return IntensityPct{v: v}, nil
}

func (i IntensityPct) Float() float64 { return i.v }

// Scale はデロード等で強度を下げるために使う。0以下にはならない。
func (i IntensityPct) Scale(f float64) IntensityPct {
	v := i.v * f
	if math.IsNaN(v) || v <= 0 {
		v = i.v
	}
	if v > 1 {
		v = 1
	}
	return IntensityPct{v: v}
}

// Ratio はバリエーション種目の対メイン係数。
type Ratio struct {
	v float64
}

func NewRatio(v float64) (Ratio, error) {
	if math.IsNaN(v) || v <= 0 || v > maxRatio {
		return Ratio{}, fmt.Errorf("対メイン係数は0より大きく%v以下である必要がある: %v", maxRatio, v)
	}
	return Ratio{v: v}, nil
}

func (r Ratio) Float() float64 { return r.v }

// SetCount はセット数。
type SetCount struct {
	v int
}

func NewSetCount(v int) (SetCount, error) {
	if v < 1 {
		return SetCount{}, fmt.Errorf("セット数は1以上である必要がある: %d", v)
	}
	return SetCount{v: v}, nil
}

func (s SetCount) Int() int { return s.v }

// Contribution は「1セット実施したとき、その筋区分に何セット分の刺激が入るか」。
type Contribution struct {
	v float64
}

func NewContribution(v float64) (Contribution, error) {
	if math.IsNaN(v) || v <= 0 || v > 1 {
		return Contribution{}, fmt.Errorf("寄与度は0より大きく1以下である必要がある: %v", v)
	}
	return Contribution{v: v}, nil
}

func (c Contribution) Float() float64 { return c.v }
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'TestWeight|TestReps|TestRIR|TestIntensity|TestRatio|TestSetCount|TestContribution'`
Expected: PASS

- [ ] **Step 5: 全テストを実行する**

Run: `go test ./...`
Expected: ok

- [ ] **Step 6: コミット**

```bash
git add internal/domain/training/measures.go internal/domain/training/measures_test.go
git commit -m "feat(domain): 計測系の値オブジェクトを追加する"
```

---

### Task 5: OneRepMax 値オブジェクト（Epley）

**Files:**
- Create: `internal/domain/training/one_rep_max.go`
- Test: `internal/domain/training/one_rep_max_test.go`

**Interfaces:**
- Consumes: Task 4 の `Weight` / `Reps` / `RIR` / `IntensityPct` / `Ratio` / `Increment`
- Produces:
  - `type OneRepMax struct{...}`
  - `func NewOneRepMax(kg float64) (OneRepMax, error)`
  - `func EstimateOneRepMax(w Weight, r Reps, rir RIR) OneRepMax` — Epley
  - `func (o OneRepMax) Kg() float64`
  - `func (o OneRepMax) WorkWeight(i IntensityPct, ratio Ratio, inc Increment) Weight`

RIR を「限界までの残りレップ数」として実績レップに足すことで、追い込みきっていないセットからも強度を推定できる。全セットに RIR を入力する設計は、**毎セットを1RM測定に変えるため**にある。

- [ ] **Step 1: 失敗するテストを書く**

`internal/domain/training/one_rep_max_test.go`:

```go
package training_test

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func estimate(t *testing.T, kg float64, reps, rir int) training.OneRepMax {
	t.Helper()
	w, err := training.NewWeight(kg)
	if err != nil {
		t.Fatalf("NewWeight: %v", err)
	}
	r, err := training.NewReps(reps)
	if err != nil {
		t.Fatalf("NewReps: %v", err)
	}
	ri, err := training.NewRIR(rir)
	if err != nil {
		t.Fatalf("NewRIR: %v", err)
	}
	return training.EstimateOneRepMax(w, r, ri)
}

func TestEstimateOneRepMax_SingleAtFailure(t *testing.T) {
	got := estimate(t, 100, 1, 0).Kg()
	want := 100.0 * (1 + 1.0/30.0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEstimateOneRepMax_RIRCountsTowardFailure(t *testing.T) {
	// 85kg x 9reps RIR2 → 限界まで11レップ
	got := estimate(t, 85, 9, 2).Kg()
	want := 85.0 * (1 + 11.0/30.0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEstimateOneRepMax_SameTotalRepsAreEquivalent(t *testing.T) {
	a := estimate(t, 80, 8, 3).Kg()
	b := estimate(t, 80, 11, 0).Kg()
	if math.Abs(a-b) > 1e-9 {
		t.Errorf("同じ総レップで値が違う: %v vs %v", a, b)
	}
}

func TestOneRepMax_WorkWeight(t *testing.T) {
	orm, err := training.NewOneRepMax(105)
	if err != nil {
		t.Fatalf("NewOneRepMax: %v", err)
	}
	intensity, _ := training.NewIntensityPct(0.81)
	ratio, _ := training.NewRatio(1.0)
	inc, _ := training.NewIncrement(2.5)

	// 105 * 0.81 = 85.05 → 85.0
	if got := orm.WorkWeight(intensity, ratio, inc).Kg(); math.Abs(got-85.0) > 1e-9 {
		t.Errorf("got %v, want 85", got)
	}
}

func TestOneRepMax_WorkWeightAppliesRatio(t *testing.T) {
	orm, _ := training.NewOneRepMax(105)
	intensity, _ := training.NewIntensityPct(0.76)
	ratio, _ := training.NewRatio(0.85)
	inc, _ := training.NewIncrement(2.5)

	// 105 * 0.76 * 0.85 = 67.83 → 67.5
	if got := orm.WorkWeight(intensity, ratio, inc).Kg(); math.Abs(got-67.5) > 1e-9 {
		t.Errorf("got %v, want 67.5", got)
	}
}

func TestNewOneRepMax_RejectsNonPositive(t *testing.T) {
	if _, err := training.NewOneRepMax(0); err == nil {
		t.Error("0の1RMが通ってしまう")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'OneRepMax'`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/domain/training/one_rep_max.go`:

```go
package training

import (
	"fmt"
	"math"
)

// epleyDivisor は Epley 式の定数。
const epleyDivisor = 30.0

// OneRepMax は推定1RM。不変。
type OneRepMax struct {
	kg float64
}

func NewOneRepMax(kg float64) (OneRepMax, error) {
	if math.IsNaN(kg) || math.IsInf(kg, 0) || kg <= 0 {
		return OneRepMax{}, fmt.Errorf("推定1RMは正の数である必要がある: %v", kg)
	}
	return OneRepMax{kg: kg}, nil
}

// EstimateOneRepMax は Epley 式で1セットから1RMを推定する。
//
// RIR を「限界までの残りレップ数」として実績レップに足すため、
// 追い込みきっていないセットからも強度が測れる。
// 全セットに RIR を入力する設計は、毎セットを1RM測定に変えるためにある。
func EstimateOneRepMax(w Weight, r Reps, rir RIR) OneRepMax {
	repsToFailure := float64(r.Int() + rir.Int())
	return OneRepMax{kg: w.Kg() * (1 + repsToFailure/epleyDivisor)}
}

func (o OneRepMax) Kg() float64 { return o.kg }

// WorkWeight は実際に使う重量。
// 推定1RM × 強度帯 × 対メイン係数 を増加単位へ丸める。
// 1RMが上がれば全スロットの重量が自動的に追随する。
func (o OneRepMax) WorkWeight(i IntensityPct, ratio Ratio, inc Increment) Weight {
	raw := o.kg * i.Float() * ratio.Float()
	return Weight{kg: raw}.RoundTo(inc)
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'OneRepMax'`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/domain/training/one_rep_max.go internal/domain/training/one_rep_max_test.go
git commit -m "feat(domain): Epley式による推定1RMを追加する"
```

---

## 続きの計画

1タスクあたりのコードを省略せずに書き切るため、計画を分割している。1ファイルに詰め込むと後半がプレースホルダに劣化する。

- `02-domain-model.md` — Task 6〜10（Exercise / SetLog / History / 推定器 / 対メイン係数）
- `03-domain-planning.md` — Task 11〜16（スロット / 残差 / 補助選択 / コンディション / デロード / SessionPlanner / シード）
- `04-application-and-api.md` — Application / Infrastructure（インメモリ）/ Presentation。ここで初めてサーバーが起動する

## 完了の定義（この計画の範囲）

- `go test ./...` が全件パスする
- `TestDomain_DependsOnNothingOutside` が通り、ドメイン層が外部に依存していない
- すべての値オブジェクトが不変かつ自己検証で、ゼロ値の混入をコンストラクタで防いでいる
