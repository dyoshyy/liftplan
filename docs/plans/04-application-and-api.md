# liftplan サーバー Application / API 第4部 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ドメイン層の外側に Application / Infrastructure / Presentation を巻き付け、`go run ./cmd/api` で実際に動くサーバーにする。

**Architecture:** Onion Architecture。依存は常に内向き。この計画では Infrastructure を**インメモリ実装**で作る。Neon Postgres への差し替えは次の計画（`05-postgres.md`）で行う。

**なぜインメモリを先にやるか:** Onion の要は「インフラが差し替え可能であること」。ドメインが正しいことを DB 抜きで証明してから Postgres に差し替えれば、ドメインのバグと SQL のバグが混ざらない。逆順にすると切り分けが不可能になる。

**前提:** `01-domain.md`〜`03-domain-planning.md`（Task 1〜18）が完了していること。

## Global Constraints

- モジュール名は `github.com/dyoshyy/liftplan-server`
- **依存方向**（外→内のみ）:
  - `internal/domain` … 何にも依存しない（標準ライブラリのみ）
  - `internal/application` … domain のみに依存
  - `internal/infrastructure` … domain と application に依存
  - `internal/presentation` … domain と application に依存
  - `cmd/api` … すべてを組み立てる。ここだけが具象を知る
- **リポジトリのインターフェースは domain 層に置く。** application はそれを使うだけで、自分では定義しない
- **HTTP の DTO をドメインモデルとして使わない。** presentation で必ず変換する
- 外部ライブラリは追加しない。ルーティングは Go 1.22 以降の `net/http` のパターンマッチで足りる
- 時刻は `cmd/api` の組み立て時に注入する。domain / application 内で `time.Now()` を呼ばない
- 単一ユーザー前提。認証は次の計画で扱う
- 各タスクは「テストを書く → 失敗を確認 → 実装 → 通ることを確認 → コミット」の順で進める

---

### Task 19: リポジトリインターフェース（domain 層）

**Files:**
- Create: `internal/domain/training/repository.go`
- Test: `internal/domain/training/repository_test.go`

**Interfaces:**
- Consumes: Task 6〜17 のドメインモデル
- Produces:
  - `type ExerciseRepository interface { FindAll(ctx context.Context) ([]*Exercise, error) }`
  - `type SetLogRepository interface { FindAll(ctx context.Context) (History, error); Save(ctx context.Context, logs []*SetLog) error }`
  - `type ConditionRepository interface { FindAll(ctx context.Context) (ConditionLog, error); Save(ctx context.Context, items []DailyCondition) error }`
  - `type ProgramRepository interface { Get(ctx context.Context) (*Program, error) }`
  - `var ErrProgramNotConfigured = errors.New("プログラムが未設定である")`

`Save` が単数ではなくスライスを取るのは、クライアントがまとめて同期してくるため。**同じ ID を二度送っても壊れない冪等な実装**を各リポジトリに要求する（インターフェースのドキュメントに明記する）。

`context` を使うため、`architecture_test.go` の `allowedStdlib` に `"context"` を追加する必要がある。

- [ ] **Step 1: 依存許可リストに context を追加する**

`internal/domain/training/architecture_test.go` の `allowedStdlib` に `"context": true,` を追加する。ドメイン層がキャンセル伝播を受け取るために必要で、外部への依存ではない。

- [ ] **Step 2: 失敗するテストを書く**

`internal/domain/training/repository_test.go`:

```go
package training_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// 各リポジトリインターフェースを満たす最小実装。
// インターフェースの形が壊れたらコンパイルで気づける。
type stubRepos struct{}

func (stubRepos) FindAll(context.Context) ([]*training.Exercise, error) { return nil, nil }

type stubSetLogRepo struct{}

func (stubSetLogRepo) FindAll(context.Context) (training.History, error) {
	return training.NewHistory(nil), nil
}
func (stubSetLogRepo) Save(context.Context, []*training.SetLog) error { return nil }

type stubConditionRepo struct{}

func (stubConditionRepo) FindAll(context.Context) (training.ConditionLog, error) {
	return training.NewConditionLog(nil), nil
}
func (stubConditionRepo) Save(context.Context, []training.DailyCondition) error { return nil }

type stubProgramRepo struct{}

func (stubProgramRepo) Get(context.Context) (*training.Program, error) {
	return nil, training.ErrProgramNotConfigured
}

func TestRepositoryInterfaces_AreSatisfiable(t *testing.T) {
	var _ training.ExerciseRepository = stubRepos{}
	var _ training.SetLogRepository = stubSetLogRepo{}
	var _ training.ConditionRepository = stubConditionRepo{}
	var _ training.ProgramRepository = stubProgramRepo{}
}

func TestErrProgramNotConfigured_Exists(t *testing.T) {
	if training.ErrProgramNotConfigured == nil {
		t.Error("未設定エラーが定義されていない")
	}
}
```

- [ ] **Step 3: テストを実行して失敗することを確認する**

Run: `go test ./internal/domain/training/... -run 'Repository|ErrProgram'`
Expected: コンパイルエラー

- [ ] **Step 4: 実装する**

`internal/domain/training/repository.go`:

```go
package training

import (
	"context"
	"errors"
)

// ErrProgramNotConfigured はユーザーがまだプログラムを設定していないことを表す。
var ErrProgramNotConfigured = errors.New("プログラムが未設定である")

// ExerciseRepository は種目マスタの取得口。
type ExerciseRepository interface {
	FindAll(ctx context.Context) ([]*Exercise, error)
}

// SetLogRepository は実績ログの永続化口。
//
// Save は冪等でなければならない。クライアントが採番した ID を使い、
// 同じログを二度送っても重複が生まれない実装にすること。
type SetLogRepository interface {
	FindAll(ctx context.Context) (History, error)
	Save(ctx context.Context, logs []*SetLog) error
}

// ConditionRepository は日次コンディションの永続化口。
//
// Save は冪等でなければならない。同じ日付を二度送ったら上書きになる実装にすること。
type ConditionRepository interface {
	FindAll(ctx context.Context) (ConditionLog, error)
	Save(ctx context.Context, items []DailyCondition) error
}

// ProgramRepository はユーザー設定の取得口。
// 未設定の場合は ErrProgramNotConfigured を返す。
type ProgramRepository interface {
	Get(ctx context.Context) (*Program, error)
}
```

- [ ] **Step 5: テストを実行して通ることを確認する**

Run: `go test ./internal/domain/training/... -run 'Repository|ErrProgram'`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/domain/training/repository.go internal/domain/training/repository_test.go internal/domain/training/architecture_test.go
git commit -m "feat(domain): リポジトリインターフェースを定義する"
```

---

### Task 20: Application ユースケース

**Files:**
- Create: `internal/application/usecase/get_session.go`
- Create: `internal/application/usecase/record_sets.go`
- Create: `internal/application/usecase/record_conditions.go`
- Test: `internal/application/usecase/get_session_test.go`
- Test: `internal/application/usecase/record_test.go`
- Create: `internal/application/usecase/configure_program.go`（D-031）
- Test: `internal/application/usecase/configure_program_test.go`

> **契約の修正:** `AccessorySlots` は廃止した。補助種目の枠数は残差から導出するので外から渡さない（D-018）。`training.NewDate` は `(Date, error)` を返すので、テストでは `MustDate` を使う。

**Interfaces:**
- Consumes: Task 19 のリポジトリインターフェース、Task 16 の `SessionPlanner`
- Produces:
  - `type GetSession struct{...}` / `func NewGetSession(exercises training.ExerciseRepository, logs training.SetLogRepository, conditions training.ConditionRepository, programs training.ProgramRepository, planner training.SessionPlanner) *GetSession`
  - `type GetSessionInput struct{ Date training.Date; DeloadAccepted []training.ExerciseID }`
  - `func (u *GetSession) Execute(ctx context.Context, in GetSessionInput) (training.PlannedSession, error)`
  - `type RecordSets struct{...}` / `func NewRecordSets(repo training.SetLogRepository) *RecordSets` / `func (u *RecordSets) Execute(ctx context.Context, logs []*training.SetLog) error`
  - `type RecordConditions struct{...}` / `func NewRecordConditions(repo training.ConditionRepository) *RecordConditions` / `func (u *RecordConditions) Execute(ctx context.Context, items []training.DailyCondition) error`

ユースケースは**データを集めてドメインに渡すだけ**。判断は一切しない。判断がここに漏れ出したら、それはドメイン層に置くべきもの。

- [ ] **Step 1: 失敗するテストを書く**

`internal/application/usecase/get_session_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

var testDate = training.MustDate(2026, time.August, 17)

type fakeExercises struct {
	all []*training.Exercise
	err error
}

func (f fakeExercises) FindAll(context.Context) ([]*training.Exercise, error) {
	return f.all, f.err
}

type fakeLogs struct {
	history training.History
	saved   []*training.SetLog
	err     error
}

func (f *fakeLogs) FindAll(context.Context) (training.History, error) {
	return f.history, f.err
}
func (f *fakeLogs) Save(_ context.Context, logs []*training.SetLog) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, logs...)
	return nil
}

type fakeConditions struct {
	log   training.ConditionLog
	saved []training.DailyCondition
	err   error
}

func (f *fakeConditions) FindAll(context.Context) (training.ConditionLog, error) {
	return f.log, f.err
}
func (f *fakeConditions) Save(_ context.Context, items []training.DailyCondition) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, items...)
	return nil
}

type fakeProgram struct {
	program *training.Program
	err     error
}

func (f fakeProgram) Get(context.Context) (*training.Program, error) {
	return f.program, f.err
}

func buildProgram(t *testing.T, pool []*training.Exercise) *training.Program {
	t.Helper()
	freq, err := training.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}
	p, err := training.NewProgram(freq, target, selected)
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return p
}

func newGetSession(t *testing.T, logs *fakeLogs, conditions *fakeConditions, program fakeProgram) *usecase.GetSession {
	t.Helper()
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	return usecase.NewGetSession(
		fakeExercises{all: pool}, logs, conditions, program,
		training.DefaultSessionPlanner(),
	)
}

func TestGetSession_ReturnsPlannedSession(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(nil)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		fakeProgram{program: buildProgram(t, pool)},
	)

	got, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(got.Main()) != 3 {
		t.Errorf("メインが3種目でない: %d", len(got.Main()))
	}
	if !got.Date().Equal(testDate) {
		t.Errorf("日付が誤り: %v", got.Date())
	}
}

func TestGetSession_PropagatesProgramNotConfigured(t *testing.T) {
	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(nil)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		fakeProgram{err: training.ErrProgramNotConfigured},
	)

	_, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if !errors.Is(err, training.ErrProgramNotConfigured) {
		t.Errorf("未設定エラーが伝播していない: %v", err)
	}
}

func TestGetSession_PropagatesRepositoryError(t *testing.T) {
	boom := errors.New("DBが落ちている")
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{err: boom},
		&fakeConditions{log: training.NewConditionLog(nil)},
		fakeProgram{program: buildProgram(t, pool)},
	)

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate}); !errors.Is(err, boom) {
		t.Errorf("リポジトリのエラーが伝播していない: %v", err)
	}
}

func TestGetSession_RejectsZeroDate(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(nil)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		fakeProgram{program: buildProgram(t, pool)},
	)

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{}); err == nil {
		t.Error("日付無しが通ってしまう")
	}
}
```

`internal/application/usecase/record_test.go`:

```go
package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestRecordSets_SavesLogs(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	uc := usecase.NewRecordSets(repo)

	log, err := training.NewSetLog(training.SetLogParams{
		ID: "01J-A", PerformedOn: testDate, ExerciseID: "bench",
		WeightKg: 85, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}

	if err := uc.Execute(context.Background(), []*training.SetLog{log}); err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Errorf("保存されていない: %d", len(repo.saved))
	}
}

func TestRecordSets_EmptyIsNoop(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	if err := usecase.NewRecordSets(repo).Execute(context.Background(), nil); err != nil {
		t.Errorf("空の保存でエラーになった: %v", err)
	}
	if len(repo.saved) != 0 {
		t.Error("空なのに保存された")
	}
}

func TestRecordSets_PropagatesError(t *testing.T) {
	boom := errors.New("書けない")
	repo := &fakeLogs{err: boom}

	log, _ := training.NewSetLog(training.SetLogParams{
		ID: "01J-B", PerformedOn: testDate, ExerciseID: "bench",
		WeightKg: 85, Reps: 9, RIR: 2,
	})
	if err := usecase.NewRecordSets(repo).Execute(context.Background(), []*training.SetLog{log}); !errors.Is(err, boom) {
		t.Errorf("エラーが伝播していない: %v", err)
	}
}

func TestRecordConditions_SavesItems(t *testing.T) {
	repo := &fakeConditions{log: training.NewConditionLog(nil)}
	uc := usecase.NewRecordConditions(repo)

	item := training.NewDailyCondition(testDate).WithBodyWeight(75).WithSleepHours(7)
	if err := uc.Execute(context.Background(), []training.DailyCondition{item}); err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Errorf("保存されていない: %d", len(repo.saved))
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/application/...`
Expected: コンパイルエラー（パッケージが存在しない）

- [ ] **Step 3: GetSession を実装する**

`internal/application/usecase/get_session.go`:

```go
// Package usecase はアプリケーション層。
//
// ユースケースはデータを集めてドメインに渡すだけで、判断は一切しない。
// 判断がここに漏れ出したら、それはドメイン層に置くべきもの。
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

type GetSessionInput struct {
	Date           training.Date
	DeloadAccepted []training.ExerciseID
}

// GetSession は指定日のセッションを導出するユースケース。
type GetSession struct {
	exercises  training.ExerciseRepository
	logs       training.SetLogRepository
	conditions training.ConditionRepository
	programs   training.ProgramRepository
	planner    training.SessionPlanner
}

func NewGetSession(
	exercises training.ExerciseRepository,
	logs training.SetLogRepository,
	conditions training.ConditionRepository,
	programs training.ProgramRepository,
	planner training.SessionPlanner,
) *GetSession {
	return &GetSession{
		exercises: exercises, logs: logs,
		conditions: conditions, programs: programs, planner: planner,
	}
}

func (u *GetSession) Execute(ctx context.Context, in GetSessionInput) (training.PlannedSession, error) {
	if in.Date.IsZero() {
		return training.PlannedSession{}, errors.New("対象日が指定されていない")
	}

	program, err := u.programs.Get(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	history, err := u.logs.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	conditions, err := u.conditions.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("コンディションの取得に失敗: %w", err)
	}

	return u.planner.Plan(training.PlanRequest{
		Program:        program,
		Pool:           pool,
		History:        history,
		Conditions:     conditions,
		Date:           in.Date,
		DeloadAccepted: in.DeloadAccepted,
	})
}
```

- [ ] **Step 4: RecordSets と RecordConditions を実装する**

`internal/application/usecase/record_sets.go`:

```go
package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// RecordSets は実績ログを保存するユースケース。
// リポジトリ側が冪等なので、同じログを二度受け取っても壊れない。
type RecordSets struct {
	repo training.SetLogRepository
}

func NewRecordSets(repo training.SetLogRepository) *RecordSets {
	return &RecordSets{repo: repo}
}

func (u *RecordSets) Execute(ctx context.Context, logs []*training.SetLog) error {
	if len(logs) == 0 {
		return nil
	}
	if err := u.repo.Save(ctx, logs); err != nil {
		return fmt.Errorf("実績の保存に失敗: %w", err)
	}
	return nil
}
```

`internal/application/usecase/record_conditions.go`:

```go
package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// RecordConditions は日次コンディションを保存するユースケース。
type RecordConditions struct {
	repo training.ConditionRepository
}

func NewRecordConditions(repo training.ConditionRepository) *RecordConditions {
	return &RecordConditions{repo: repo}
}

func (u *RecordConditions) Execute(ctx context.Context, items []training.DailyCondition) error {
	if len(items) == 0 {
		return nil
	}
	if err := u.repo.Save(ctx, items); err != nil {
		return fmt.Errorf("コンディションの保存に失敗: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: テストを実行して通ることを確認する**

Run: `go test ./internal/application/...`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/application/
git commit -m "feat(application): セッション取得と記録のユースケースを追加する"
```

---

> **追加（D-031）:** プログラムを設定する経路が Task 19〜24 のどこにも無く、`ErrProgramNotConfigured` → 409 の分岐が本番で到達不能なデッドコードになっていた。`ProgramRepository.Save` をドメイン層のインターフェースに足し、ここに `ConfigureProgram` ユースケースを追加する。
>
> ```go
> // ConfigureProgram はユーザーのプログラム設定を保存する。
> //
> // 選択された種目IDが種目マスタに実在するかを突合するのはここ。
> // Program は種目マスタを知らないので自分では検証できず、
> // 実在しないIDは SessionPlanner が黙って落とす。
> type ConfigureProgram struct {
> 	exercises training.ExerciseRepository
> 	programs  training.ProgramRepository
> }
>
> type ConfigureProgramInput struct {
> 	PerWeek  int
> 	Target   map[training.MuscleRegion]float64
> 	Selected []training.ExerciseID
> }
>
> func (u *ConfigureProgram) Execute(ctx context.Context, in ConfigureProgramInput) error
> ```
>
> 実在しないIDが混ざっていたら `training.ErrExerciseNotFound` を包んで返し、プレゼンテーション層は 400 にする。黙って落とすと、ユーザーが選んだ種目が理由の説明なく消える。

> **契約の修正（D-031 / D-033）:** `ProgramRepository.Set(p)` は廃止し、インターフェースの `Save(ctx, p) error` にする。`SetLogRepository.Save` は同じIDで内容が違えば `ErrConflictingSetLog` を返し、全か無かで書く（黙って上書きしない）。`ConditionRepository.Save` は日付ごと置き換えず、`DailyCondition.Merge` と同じ規則で項目ごとに上書きする（体重だけ送ると睡眠時間が消えるため）。

### Task 21: インメモリ Infrastructure

**Files:**
- Create: `internal/infrastructure/memory/repositories.go`
- Test: `internal/infrastructure/memory/repositories_test.go`

**Interfaces:**
- Consumes: Task 19 のリポジトリインターフェース、Task 17 のシード
- Produces:
  - `type ExerciseRepository struct{...}` / `func NewExerciseRepository(all []*training.Exercise) *ExerciseRepository`
  - `type SetLogRepository struct{...}` / `func NewSetLogRepository() *SetLogRepository`
  - `type ConditionRepository struct{...}` / `func NewConditionRepository() *ConditionRepository`
  - `type ProgramRepository struct{...}` / `func NewProgramRepository(p *training.Program) *ProgramRepository` / `func (r *ProgramRepository) Set(p *training.Program)`

**冪等性がここで最初に問われる。** `SetLogRepository.Save` は ID をキーにした map で保持し、同じ ID を二度受けても件数が増えないこと。`ConditionRepository.Save` は日付をキーにして上書きすること。

並行アクセスがあるため `sync.RWMutex` で保護する。

- [ ] **Step 1: 失敗するテストを書く**

`internal/infrastructure/memory/repositories_test.go`:

```go
package memory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
)

var day = training.NewDate(2026, time.August, 17)

func mkSetLog(t *testing.T, id string, kg float64) *training.SetLog {
	t.Helper()
	l, err := training.NewSetLog(training.SetLogParams{
		ID: id, PerformedOn: day, ExerciseID: "bench",
		WeightKg: kg, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	return l
}

func TestExerciseRepository_ReturnsSeed(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(all)

	got, err := repo.FindAll(context.Background())
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(got) != len(all) {
		t.Errorf("件数が誤り: got %d, want %d", len(got), len(all))
	}
}

func TestSetLogRepository_SaveIsIdempotent(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	log := mkSetLog(t, "01J-SAME", 85)
	if err := repo.Save(ctx, []*training.SetLog{log}); err != nil {
		t.Fatalf("1回目の保存に失敗: %v", err)
	}
	if err := repo.Save(ctx, []*training.SetLog{log}); err != nil {
		t.Fatalf("2回目の保存に失敗: %v", err)
	}

	history, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if got := len(history.Logs()); got != 1 {
		t.Errorf("同じIDが重複して保存された: %d件", got)
	}
}

func TestSetLogRepository_SaveOverwritesSameID(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	_ = repo.Save(ctx, []*training.SetLog{mkSetLog(t, "01J-X", 85)})
	_ = repo.Save(ctx, []*training.SetLog{mkSetLog(t, "01J-X", 90)})

	history, _ := repo.FindAll(ctx)
	logs := history.Logs()
	if len(logs) != 1 {
		t.Fatalf("件数が誤り: %d", len(logs))
	}
	if logs[0].Weight().Kg() != 90 {
		t.Errorf("上書きされていない: %v", logs[0].Weight().Kg())
	}
}

func TestConditionRepository_SaveOverwritesSameDate(t *testing.T) {
	repo := memory.NewConditionRepository()
	ctx := context.Background()

	_ = repo.Save(ctx, []training.DailyCondition{
		training.NewDailyCondition(day).WithBodyWeight(75),
	})
	_ = repo.Save(ctx, []training.DailyCondition{
		training.NewDailyCondition(day).WithBodyWeight(74),
	})

	log, err := repo.FindAll(ctx)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	trend, ok := training.DefaultConditionAnalyzer().BodyWeightTrendKgPerWeek(log, day)
	_ = trend
	if ok {
		t.Log("サンプル数が足りればトレンドが出る（ここでは1件なので false が正しい）")
	}
	// 件数の確認は ConditionLog に件数APIが無いため、
	// 同日2回保存で 1 件になっていることを Save の戻り値ではなく Size で確かめる
	if got := repo.Size(); got != 1 {
		t.Errorf("同じ日付が重複して保存された: %d件", got)
	}
}

func TestProgramRepository_NotConfigured(t *testing.T) {
	repo := memory.NewProgramRepository(nil)
	if _, err := repo.Get(context.Background()); err != training.ErrProgramNotConfigured {
		t.Errorf("未設定エラーが返らない: %v", err)
	}
}

func TestProgramRepository_Set(t *testing.T) {
	pool, _ := seed.Exercises()
	freq, _ := training.NewFrequency(3)
	target, _ := seed.DefaultWeeklyTarget(freq)
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}
	program, err := training.NewProgram(freq, target, selected)
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}

	repo := memory.NewProgramRepository(nil)
	repo.Set(program)

	got, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if got.Frequency().PerWeek() != 3 {
		t.Errorf("頻度が誤り: %d", got.Frequency().PerWeek())
	}
}

func TestSetLogRepository_ConcurrentSaveIsSafe(t *testing.T) {
	repo := memory.NewSetLogRepository()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "01J-" + string(rune('A'+n%26)) + string(rune('a'+n/26))
			_ = repo.Save(ctx, []*training.SetLog{mkSetLog(t, id, 85)})
		}(i)
	}
	wg.Wait()

	if _, err := repo.FindAll(ctx); err != nil {
		t.Fatalf("並行保存後の取得に失敗: %v", err)
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/infrastructure/...`
Expected: コンパイルエラー

- [ ] **Step 3: 実装する**

`internal/infrastructure/memory/repositories.go`:

```go
// Package memory はリポジトリのインメモリ実装。
//
// Onion の要はインフラが差し替え可能であること。ドメインが正しいことを
// DB 抜きで証明するために、まずこの実装でサーバーを動かす。
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// ExerciseRepository は種目マスタを保持する。起動時にシードを流し込む。
type ExerciseRepository struct {
	mu  sync.RWMutex
	all []*training.Exercise
}

func NewExerciseRepository(all []*training.Exercise) *ExerciseRepository {
	copied := make([]*training.Exercise, len(all))
	copy(copied, all)
	return &ExerciseRepository{all: copied}
}

func (r *ExerciseRepository) FindAll(context.Context) ([]*training.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*training.Exercise, len(r.all))
	copy(out, r.all)
	return out, nil
}

// SetLogRepository は実績ログを ID キーで保持する。
// 同じ ID を二度受けても重複しないため、Save は冪等になる。
type SetLogRepository struct {
	mu   sync.RWMutex
	byID map[training.SetLogID]*training.SetLog
}

func NewSetLogRepository() *SetLogRepository {
	return &SetLogRepository{byID: map[training.SetLogID]*training.SetLog{}}
}

func (r *SetLogRepository) FindAll(context.Context) (training.History, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.byID))
	for id := range r.byID {
		ids = append(ids, string(id))
	}
	sort.Strings(ids) // 取得順を安定させる

	out := make([]*training.SetLog, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.byID[training.SetLogID(id)])
	}
	return training.NewHistory(out), nil
}

func (r *SetLogRepository) Save(_ context.Context, logs []*training.SetLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, l := range logs {
		if l == nil {
			continue
		}
		r.byID[l.ID()] = l
	}
	return nil
}

func (r *SetLogRepository) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

// ConditionRepository は日次コンディションを日付キーで保持する。
// 同じ日付を二度受けたら上書きになるため、Save は冪等になる。
type ConditionRepository struct {
	mu     sync.RWMutex
	byDate map[string]training.DailyCondition
}

func NewConditionRepository() *ConditionRepository {
	return &ConditionRepository{byDate: map[string]training.DailyCondition{}}
}

func (r *ConditionRepository) FindAll(context.Context) (training.ConditionLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]string, 0, len(r.byDate))
	for k := range r.byDate {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]training.DailyCondition, 0, len(keys))
	for _, k := range keys {
		out = append(out, r.byDate[k])
	}
	return training.NewConditionLog(out), nil
}

func (r *ConditionRepository) Save(_ context.Context, items []training.DailyCondition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, c := range items {
		if c.Date().IsZero() {
			continue
		}
		r.byDate[c.Date().String()] = c
	}
	return nil
}

func (r *ConditionRepository) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byDate)
}

// ProgramRepository はユーザー設定を1つだけ保持する（単一ユーザー前提）。
type ProgramRepository struct {
	mu      sync.RWMutex
	program *training.Program
}

func NewProgramRepository(p *training.Program) *ProgramRepository {
	return &ProgramRepository{program: p}
}

func (r *ProgramRepository) Get(context.Context) (*training.Program, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.program == nil {
		return nil, training.ErrProgramNotConfigured
	}
	return r.program, nil
}

func (r *ProgramRepository) Set(p *training.Program) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.program = p
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `go test ./internal/infrastructure/...`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add internal/infrastructure/
git commit -m "feat(infrastructure): インメモリのリポジトリ実装を追加する"
```

---

> **契約の追加（D-035 / D-036 / D-037 / D-039）:** ハンドラのステータス分類は次のとおり。
>
> | 条件 | ステータス |
> |---|---|
> | `errors.Is(err, usecase.ErrInvalidInput)` | 400 |
> | `errors.Is(err, training.ErrProgramNotConfigured)` | 409 |
> | `errors.Is(err, training.ErrConflictingSetLog)` | 409 |
> | `errors.Is(err, context.Canceled)` | 499（クライアント切断。ログに残すが警報にしない） |
> | それ以外 | 500 |
>
> `ErrInvalidInput` はアプリケーション層のセンチネルで、ドメインのコンストラクタが返す匿名エラーを包み直したもの。これが無いと「頻度が範囲外」と「データベースが落ちている」が同じ形になる。

### Task 22: HTTP Presentation

**Files:**
- Create: `internal/presentation/httpapi/dto.go`
- Create: `internal/presentation/httpapi/handler.go`
- Create: `internal/presentation/httpapi/router.go`
- Test: `internal/presentation/httpapi/handler_test.go`

**Interfaces:**
- Consumes: Task 20 のユースケース
- Produces:
  - `type Handler struct{...}` / `func NewHandler(get *usecase.GetSession, sets *usecase.RecordSets, conditions *usecase.RecordConditions) *Handler`
  - `func (h *Handler) Routes() *http.ServeMux`
  - エンドポイント:
    - `GET /api/sessions?date=2026-08-17&deload_accepted=bench,squat`（承認した種目IDをカンマ区切り。D-022）
    - `POST /api/set-logs`
    - `POST /api/conditions`
    - `GET /healthz`

**DTO をドメインモデルとして使わない。** JSON の形が変わってもドメインが揺れないよう、presentation で必ず変換する。

`weight_kg` は履歴が無いとき `null` になる。**これはバグではなく仕様**で、クライアントは「初回だけ自分で決める」UI を出す。

- [ ] **Step 1: 失敗するテストを書く**

`internal/presentation/httpapi/handler_test.go`:

```go
package httpapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan-server/internal/presentation/httpapi"
)

func newServer(t *testing.T, configured bool) http.Handler {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	exercises := memory.NewExerciseRepository(pool)
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()

	programs := memory.NewProgramRepository(nil)
	if configured {
		freq, _ := training.NewFrequency(3)
		target, err := seed.DefaultWeeklyTarget(freq)
		if err != nil {
			t.Fatalf("週目標が不正: %v", err)
		}
		selected := make([]training.ExerciseID, 0, len(pool))
		for _, e := range pool {
			if e.Kind() != training.KindVariation {
				selected = append(selected, e.ID())
			}
		}
		program, err := training.NewProgram(freq, target, selected)
		if err != nil {
			t.Fatalf("プログラムが不正: %v", err)
		}
		programs.Set(program)
	}

	handler := httpapi.NewHandler(
		usecase.NewGetSession(exercises, logs, conditions, programs, training.DefaultSessionPlanner()),
		usecase.NewRecordSets(logs),
		usecase.NewRecordConditions(conditions),
	)
	return handler.Routes()
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ステータスが誤り: %d", rec.Code)
	}
}

func TestGetSession_Success(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil)
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	var body struct {
		Date string `json:"date"`
		Main []struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
			Sets       int      `json:"sets"`
			TargetRIR  int      `json:"target_rir"`
			Role       string   `json:"role"`
		} `json:"main"`
		Accessories []struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"accessories"`
		Deload *struct {
			Reason string `json:"reason"`
		} `json:"deload_proposal"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}

	if body.Date != "2026-08-17" {
		t.Errorf("日付が誤り: %s", body.Date)
	}
	if len(body.Main) != 3 {
		t.Errorf("メインが3種目でない: %d", len(body.Main))
	}
	if len(body.Accessories) == 0 {
		t.Error("補助種目が空である")
	}
	// 履歴が無いので重量は null になるのが正しい
	for _, m := range body.Main {
		if m.WeightKg != nil {
			t.Errorf("履歴が無いのに重量が入っている: %s", m.ExerciseID)
		}
		if m.Sets <= 0 {
			t.Errorf("セット数が0以下: %s", m.ExerciseID)
		}
	}
}

func TestGetSession_MissingDate(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("日付なしが 400 にならない: %d", rec.Code)
	}
}

func TestGetSession_BadDate(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026/08/17", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("不正な日付が 400 にならない: %d", rec.Code)
	}
}

func TestGetSession_ProgramNotConfigured(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, false).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("未設定が 409 にならない: %d", rec.Code)
	}
}

func TestPostSetLogs(t *testing.T) {
	server := newServer(t, true)

	payload := `{"logs":[{"id":"01J-A","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":9,"rir":2}]}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/set-logs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}

	// 保存されたログが次のセッションに反映される
	rec2 := httptest.NewRecorder()
	server.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-24", nil))

	var body struct {
		Main []struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
		} `json:"main"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	found := false
	for _, m := range body.Main {
		if m.ExerciseID == "bench" && m.WeightKg != nil {
			found = true
		}
	}
	if !found {
		t.Error("記録したログが重量算出に反映されていない")
	}
}

func TestPostSetLogs_InvalidBody(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/set-logs", bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("壊れたJSONが 400 にならない: %d", rec.Code)
	}
}

func TestPostSetLogs_InvalidDomainValue(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := `{"logs":[{"id":"01J-B","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":0,"rir":2}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/set-logs", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("0レップが 400 にならない: %d", rec.Code)
	}
}

func TestPostConditions(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := `{"conditions":[{"date":"2026-08-17","body_weight_kg":75.2,"sleep_hours":6.5}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/conditions", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	newServer(t, true).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	rec := httptest.NewRecorder()
	newServer(t, true).ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/sessions", nil))
	if rec.Code == http.StatusOK {
		t.Error("DELETE が通ってしまう")
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./internal/presentation/...`
Expected: コンパイルエラー

- [ ] **Step 3: DTO を実装する**

`internal/presentation/httpapi/dto.go`:

```go
// Package httpapi は HTTP のプレゼンテーション層。
//
// DTO をドメインモデルとして使わない。JSON の形が変わってもドメインが
// 揺れないよう、この層で必ず変換する。
package httpapi

import "github.com/dyoshyy/liftplan-server/internal/domain/training"

// plannedSetDTO の WeightKg が null になるのはバグではなく仕様。
// 履歴が足りず重量を推定できない場合で、クライアントは
// 「初回だけ自分で決める」UI を出す。
type plannedSetDTO struct {
	ExerciseID string   `json:"exercise_id"`
	WeightKg   *float64 `json:"weight_kg"`
	Sets       int      `json:"sets"`
	TargetRIR  int      `json:"target_rir"`
	Role       string   `json:"role,omitempty"`
}

type deloadProposalDTO struct {
	Reason           string  `json:"reason"`
	IntensityDropPct float64 `json:"intensity_drop_pct"`
}

type sessionDTO struct {
	Date        string             `json:"date"`
	Main        []plannedSetDTO    `json:"main"`
	Accessories []plannedSetDTO    `json:"accessories"`
	Deload      *deloadProposalDTO `json:"deload_proposal"`
}

func toPlannedSetDTO(s training.PlannedSet) plannedSetDTO {
	dto := plannedSetDTO{
		ExerciseID: string(s.ExerciseID()),
		Sets:       s.Sets().Int(),
		TargetRIR:  s.TargetRIR().Int(),
	}
	if w, ok := s.Weight(); ok {
		kg := w.Kg()
		dto.WeightKg = &kg
	}
	if role, ok := s.Role(); ok {
		dto.Role = string(role)
	}
	return dto
}

func toSessionDTO(s training.PlannedSession) sessionDTO {
	main := make([]plannedSetDTO, 0, len(s.Main()))
	for _, v := range s.Main() {
		main = append(main, toPlannedSetDTO(v))
	}
	accessories := make([]plannedSetDTO, 0, len(s.Accessories()))
	for _, v := range s.Accessories() {
		accessories = append(accessories, toPlannedSetDTO(v))
	}

	out := sessionDTO{
		Date:        s.Date().String(),
		Main:        main,
		Accessories: accessories,
	}
	if p, ok := s.DeloadProposal(); ok {
		out.Deload = &deloadProposalDTO{
			Reason:           p.Reason(),
			IntensityDropPct: p.IntensityDropPct(),
		}
	}
	return out
}

// ポインタなのは、フィールドの欠落を検出するため。
//
// 非ポインタだと weight_kg の欠落が 0kg（正当な自重セット）になり、
// rir の欠落が RIR 0（限界まで追い込んだ）になる。どちらも有意味な値なので、
// 「送られなかった」と区別できない。
// RIR は毎セットを1RM測定に変えるための必須情報であり、欠落を黙って
// 0 と解釈すると推定1RMが実態より低くなる。
type setLogDTO struct {
	ID         string   `json:"id"`
	Date       string   `json:"date"`
	ExerciseID string   `json:"exercise_id"`
	WeightKg   *float64 `json:"weight_kg"`
	Reps       *int     `json:"reps"`
	RIR        *int     `json:"rir"`
}

type setLogsRequest struct {
	Logs []setLogDTO `json:"logs"`
}

type conditionDTO struct {
	Date         string   `json:"date"`
	BodyWeightKg *float64 `json:"body_weight_kg"`
	SleepHours   *float64 `json:"sleep_hours"`
}

type conditionsRequest struct {
	Conditions []conditionDTO `json:"conditions"`
}

type errorResponse struct {
	Error string `json:"error"`
}
```

- [ ] **Step 4: ハンドラとルータを実装する**

`internal/presentation/httpapi/handler.go`:

```go
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

type Handler struct {
	getSession       *usecase.GetSession
	recordSets       *usecase.RecordSets
	recordConditions *usecase.RecordConditions
}

func NewHandler(
	getSession *usecase.GetSession,
	recordSets *usecase.RecordSets,
	recordConditions *usecase.RecordConditions,
) *Handler {
	return &Handler{
		getSession:       getSession,
		recordSets:       recordSets,
		recordConditions: recordConditions,
	}
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("date")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "date クエリパラメータが必要である")
		return
	}
	date, err := training.ParseDate(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	session, err := h.getSession.Execute(r.Context(), usecase.GetSessionInput{
		Date:           date,
		DeloadAccepted: parseExerciseIDs(r.URL.Query().Get("deload_accepted")),
	})
	if err != nil {
		if errors.Is(err, training.ErrProgramNotConfigured) {
			writeError(w, http.StatusConflict, "プログラムが未設定である")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, toSessionDTO(session))
}

func (h *Handler) handlePostSetLogs(w http.ResponseWriter, r *http.Request) {
	var req setLogsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	logs := make([]*training.SetLog, 0, len(req.Logs))
	for i, dto := range req.Logs {
		date, err := training.ParseDate(dto.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("logs[%d]: %v", i, err))
			return
		}
		if dto.WeightKg == nil || dto.Reps == nil || dto.RIR == nil {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("logs[%d]: weight_kg / reps / rir は必須である", i))
			return
		}
		log, err := training.NewSetLog(training.SetLogParams{
			ID:          dto.ID,
			PerformedOn: date,
			ExerciseID:  dto.ExerciseID,
			WeightKg:    *dto.WeightKg,
			Reps:        *dto.Reps,
			RIR:         *dto.RIR,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("logs[%d]: %v", i, err))
			return
		}
		logs = append(logs, log)
	}

	if err := h.recordSets.Execute(r.Context(), logs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handlePostConditions(w http.ResponseWriter, r *http.Request) {
	var req conditionsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	items := make([]training.DailyCondition, 0, len(req.Conditions))
	for i, dto := range req.Conditions {
		date, err := training.ParseDate(dto.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("conditions[%d]: %v", i, err))
			return
		}
		c := training.NewDailyCondition(date)
		if dto.BodyWeightKg != nil {
			c = c.WithBodyWeight(*dto.BodyWeightKg)
		}
		if dto.SleepHours != nil {
			c = c.WithSleepHours(*dto.SleepHours)
		}
		items = append(items, c)
	}

	if err := h.recordConditions.Execute(r.Context(), items); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("リクエストボディを解釈できない: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
```

`internal/presentation/httpapi/router.go`:

```go
package httpapi

import "net/http"

// Routes は Go 1.22 以降のメソッド付きパターンでルーティングする。
// 外部のルータライブラリは不要。
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.HandleFunc("GET /api/sessions", h.handleGetSession)
	mux.HandleFunc("POST /api/set-logs", h.handlePostSetLogs)
	mux.HandleFunc("POST /api/conditions", h.handlePostConditions)
	mux.HandleFunc("GET /api/program", h.handleGetProgram)
	mux.HandleFunc("PUT /api/program", h.handlePutProgram)
	return mux
}
```

- [ ] **Step 5: テストを実行して通ることを確認する**

Run: `go test ./internal/presentation/...`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/presentation/
git commit -m "feat(presentation): HTTP API を追加する"
```

---

### Task 23: 組み立てと起動

**Files:**
- Create: `cmd/api/main.go`
- Create: `README.md`
- Test: `cmd/api/main_test.go`

**Interfaces:**
- Consumes: Task 20〜22 のすべて
- Produces: `go run ./cmd/api` で起動するサーバー

**cmd だけが具象を知る。** ここで初めてインメモリ実装が選ばれる。Postgres に差し替えるときも、変更はこのファイルだけで済むはず——それが Onion が守れているかの試金石になる。

- [ ] **Step 1: 組み立てのテストを書く**

`cmd/api/main_test.go`:

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildHandler_ServesSession(t *testing.T) {
	handler, err := buildHandler()
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/sessions?date=2026-08-17", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBuildHandler_Healthz(t *testing.T) {
	handler, err := buildHandler()
	if err != nil {
		t.Fatalf("組み立てに失敗: %v", err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ヘルスチェックが失敗: %d", rec.Code)
	}
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `go test ./cmd/...`
Expected: コンパイルエラー（`buildHandler` が未定義）

- [ ] **Step 3: 実装する**

`cmd/api/main.go`:

```go
// Command api は liftplan のサーバーを起動する。
//
// Onion Architecture において、具象を知ってよいのはこの層だけ。
// リポジトリ実装を Postgres に差し替えるときも、変更はこのファイルに閉じるはずで、
// それが依存方向を守れているかの試金石になる。
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan-server/internal/presentation/httpapi"
)

const defaultFrequencyPerWeek = 3

func main() {
	handler, err := buildHandler()
	if err != nil {
		log.Fatalf("起動に失敗: %v", err)
	}

	addr := ":" + port()
	log.Printf("liftplan-server を %s で起動する", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("サーバーが停止した: %v", err)
	}
}

func port() string {
	if v := os.Getenv("PORT"); v != "" {
		return v
	}
	return "8080"
}

// buildHandler は依存を組み立てる。テストからも呼べるよう main と分けている。
func buildHandler() (http.Handler, error) {
	pool, err := seed.Exercises()
	if err != nil {
		return nil, fmt.Errorf("種目シードが不正: %w", err)
	}
	program, err := defaultProgram(pool)
	if err != nil {
		return nil, err
	}

	exercises := memory.NewExerciseRepository(pool)
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	programs := memory.NewProgramRepository(program)

	planner := training.DefaultSessionPlanner()

	handler := httpapi.NewHandler(
		usecase.NewGetSession(exercises, logs, conditions, programs, planner),
		usecase.NewRecordSets(logs),
		usecase.NewRecordConditions(conditions),
	)
	return handler.Routes(), nil
}

// defaultProgram はシードから初期プログラムを組む。
// バリエーションはメインに付随して自動で回るため、選択には含めない。
func defaultProgram(pool []*training.Exercise) (*training.Program, error) {
	freq, err := training.NewFrequency(defaultFrequencyPerWeek)
	if err != nil {
		return nil, fmt.Errorf("既定の頻度が不正: %w", err)
	}

	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		return nil, fmt.Errorf("週目標シードが不正: %w", err)
	}

	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}

	return training.NewProgram(freq, target, selected)
}
```

- [ ] **Step 4: README を書く**

`README.md`:

````markdown
# liftplan-server

筋トレの進行を自動化するサーバー。実績ログ・週目標・コンディションから、その日のセッション（種目・重量・目標RIR・セット数）を導出する。

設計は `docs/specs/`、実装計画は `docs/plans/` にある。

## 設計の要点

- **未来のセッションは保存しない。** 今日のメニューも来週のメニューも、確定した実績から毎回導出した結果でしかない。だから予定と実績が食い違う状態が原理的に発生しない
- **RIR は止め時の指示であり、レップ数は指示しない。** レップはその日の状態が決めるため、強度が自動でコンディションに追従する
- **減量中の停滞とオーバーリーチによる停滞は、記録だけ見ると同じ形をしている。** 体重と睡眠を取り込むのは、この2つを見分けるため
- **Onion Architecture。** 依存は常に内向き。ドメイン層は標準ライブラリ以外に依存せず、DB も HTTP も立てずにテストできる

## 起動

```bash
go run ./cmd/api
```

`PORT` 環境変数でポートを変更できる（既定 8080）。現時点のリポジトリ実装はインメモリなので、再起動すると記録は消える。

## API

| メソッド | パス | 説明 |
|---|---|---|
| GET | `/healthz` | ヘルスチェック |
| GET | `/api/sessions?date=YYYY-MM-DD&deload_accepted=bench,squat` | その日のセッションを導出する |
| POST | `/api/set-logs` | 実績ログを保存する（冪等） |
| POST | `/api/conditions` | 日次コンディションを保存する（冪等） |
| GET | `/api/program` | プログラム（頻度・週目標・選択種目）を取得する。未設定なら 404 |
| PUT | `/api/program` | プログラムを設定する（冪等） |

### セッション取得の例

```bash
curl 'http://localhost:8080/api/sessions?date=2026-08-17'
```

`weight_kg` が `null` になるのはバグではない。履歴が足りず重量を推定できない状態で、初回だけ自分で決めて記録する。

### 実績の保存

```bash
curl -X POST http://localhost:8080/api/set-logs \
  -H 'Content-Type: application/json' \
  -d '{"logs":[{"id":"01J-A","date":"2026-08-17","exercise_id":"bench","weight_kg":85,"reps":9,"rir":2}]}'
```

`id` はクライアントが採番する（ULID を想定）。同じ ID を二度送っても重複しない。

## テスト

```bash
go test ./...
```

ドメイン層のテストは DB も HTTP も必要としない。
````

- [ ] **Step 5: テストを実行して通ることを確認する**

Run: `go test ./...`
Expected: ok（全パッケージ）

- [ ] **Step 6: 実際に起動して確認する**

```bash
go run ./cmd/api &
sleep 1
curl -s 'http://localhost:8080/api/sessions?date=2026-08-17' | head -c 500
kill %1
```

Expected: `{"date":"2026-08-17","main":[...],"accessories":[...],"deload_proposal":null}` が返る

- [ ] **Step 7: コミット**

```bash
git add cmd/ README.md
git commit -m "feat: サーバーを組み立てて起動できるようにする"
```

---

### Task 24: 依存方向の最終検査

**Files:**
- Test: `internal/architecture_test.go`

**Interfaces:**
- Consumes: すべてのパッケージ
- Produces: なし（検査のみ）

Task 1 の検査は domain 層の内側だけを見ていた。ここで**プロジェクト全体の依存方向**を検査する。Onion が崩れたらテストが落ちる状態にしてから、次の計画（Postgres 差し替え）へ進む。

- [ ] **Step 1: 検査テストを書く**

`internal/architecture_test.go`:

```go
package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const modulePrefix = "github.com/dyoshyy/liftplan-server/"

// layerOf はパスから層を判定する。数値が小さいほど内側。
func layerOf(pkgPath string) (string, int, bool) {
	switch {
	case strings.Contains(pkgPath, "internal/domain"):
		return "domain", 0, true
	case strings.Contains(pkgPath, "internal/application"):
		return "application", 1, true
	case strings.Contains(pkgPath, "internal/infrastructure"):
		return "infrastructure", 2, true
	case strings.Contains(pkgPath, "internal/presentation"):
		return "presentation", 2, true
	case strings.Contains(pkgPath, "cmd/"):
		return "cmd", 3, true
	}
	return "", 0, false
}

func TestOnion_DependenciesPointInward(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("ルートを解決できない: %v", err)
	}

	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fromLayer, fromDepth, ok := layerOf(filepath.ToSlash(rel))
		if !ok {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(p, modulePrefix) {
				continue
			}
			toLayer, toDepth, ok := layerOf(strings.TrimPrefix(p, modulePrefix))
			if !ok {
				continue
			}
			if toDepth > fromDepth {
				t.Errorf("%s: %s 層が %s 層に依存している（%s）", rel, fromLayer, toLayer, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
}

func TestOnion_DomainHasNoExternalDependency(t *testing.T) {
	root, err := filepath.Abs("./domain")
	if err != nil {
		t.Fatalf("ドメインを解決できない: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("ドメインディレクトリが無い: %v", err)
	}

	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imp := range file.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(p, modulePrefix) {
				continue
			}
			// ドット付きはホスト名を含む＝外部モジュール
			if strings.Contains(strings.Split(p, "/")[0], ".") {
				t.Errorf("%s: ドメイン層が外部ライブラリに依存している: %s", path, p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("走査に失敗: %v", err)
	}
}
```

- [ ] **Step 2: テストを実行して通ることを確認する**

Run: `go test ./internal/... -run 'TestOnion'`
Expected: PASS

落ちた場合は**テストを緩めず実装側の依存を直す**。よくある原因は、infrastructure が presentation の型を使っている、application が具象リポジトリを import している、など。

- [ ] **Step 3: 全テストを実行する**

Run: `go test ./...`
Expected: ok

- [ ] **Step 4: コミット**

```bash
git add internal/architecture_test.go
git commit -m "test: Onion の依存方向を全体で検査する"
```

---

## 完了の定義（第4部の範囲）

- `go test ./...` が全件パスする
- `go run ./cmd/api` が起動し、`GET /api/sessions?date=YYYY-MM-DD` が実際のセッションを返す
- `POST /api/set-logs` で記録したログが、次のセッションの重量算出に反映される
- 同じ ID を二度送っても重複しない（冪等）
- `TestOnion_DependenciesPointInward` が通り、依存が常に内向きである
- リポジトリ実装を差し替える際の変更が `cmd/api/main.go` に閉じている

## 次の計画

- `05-postgres.md` — Neon Postgres 実装への差し替え、マイグレーション、接続設定
- `06-auth.md` — 単一ユーザー向けの最小認証
- クライアント（Kotlin + Jetpack Compose）は別リポジトリで、このAPIを叩く側として作る
