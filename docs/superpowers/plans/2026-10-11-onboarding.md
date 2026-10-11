# 初回の案内（オンボーディング） Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 初回ログインの人に、通い方を決める4段階の画面と「今日」のコーチマークを1回だけ出す。

**Architecture:** サーバーは `account` ドメインに「案内を済ませたか」の口を足し、`onboarding` 表（行が無い＝まだ）に持つ。画面は `loadAll` でその値を取り、未完了なら `App.tsx` が「今日」の代わりに設定画面を描く。保存は既存の PUT を純粋関数が決めた順で送り、最後に完了を記録する。続けてメモリだけの状態でコーチマークを流す。

**Tech Stack:** Go 1.22+（net/http, pgx）、Postgres、React 19 + TypeScript、vitest、playwright-core（検査スクリプト）

**Spec:** `docs/specs/2026-10-11-onboarding-design.md`

## Global Constraints

- 既存の利用者には出さない（0020 が `program` の全行を完了として入れる）
- 行が無い＝まだ。初回ログインで行を作らない
- `GET /api/onboarding` → `{"completed": bool}`、`PUT /api/onboarding/completed` → 204、冪等
- 取得に失敗したら済んだものとして扱う（アプリを止めない）
- 開発用利用者は済んだ状態で始まる。`DEV_ONBOARDING_PENDING=1` のときだけ未完了。インメモリ構成でのみ効く
- 設定は「はじめる」でまとめて送る。途中では送らない
- 回数と分割：新しい分割の下限 ≤ 今の回数 なら分割を先、そうでなければ回数を先
- 送る列の最後は必ず `PUT /api/onboarding/completed`
- コーチマークの状態はメモリだけ。暗い部分を押しても進まない
- 画面は判断（`.ts`）／手順（`use*.ts`）／描画（`.tsx`）に割る
- エラーは `%w` で包む。doc コメントは宣言名で始める

## Review Focus

1. **送信の途中で失敗して選び直す** — 回数5・5分割を保存したあと失敗し、週2・全身法に直して再送しても通ること（Task 4 の順序テスト「5分割→全身法・週2」）
2. **回数を下げて分割が選べなくなる** — 段階1に戻って週3にしたら5分割が全身法に戻ること（Task 4 `withFrequency` のテスト）
3. **案内の取得が失敗する（オフライン）** — 設定画面ではなく「今日」が出ること（Task 5 `onboardedFrom` のテスト）
4. **カードが無い日（全部済み・予定が空）** — コーチマークが無い要素を飛ばし、全部無ければ出ないこと（Task 6 のテスト）
5. **2つ目のプロバイダでログイン（同じ利用者に紐づく）** — 既存利用者なので出ないこと。行は利用者単位なので構造的に満たす（Task 1 の「別の利用者に影響しない」と Task 2 のマイグレーションテストで守る）

## レビューで足したこと（各タスクより優先）

- **PR は2本に割って積む。**下が Task 1〜3（サーバー＋spec＋この計画）、上が Task 4〜7（画面）。どちらを先に出しても壊れない：画面が先なら `GET /api/onboarding` が 404 → null → 済んだ扱いで何も出ない。サーバーが先なら口が使われないだけ。2本の間に登録した人だけは行が無く、画面が出たあとに1回出る（間を短くする）
- **Task 2：** `migrate_test.go` の `TestMigrate_CreatesEverySchemaObject` の表の一覧に `onboarding` を足す。`saveDefaultProgram` が `ProgramRepository.Save` で組めなければ、0018 の検査と同じく `INSERT INTO program` を生で書く
- **Task 5：** `<main>` の中身を丸ごと差し替えない。`UpdateBanner`（古いバンドルからの脱出口）と `SyncBanner` は残し、`key={route}` の `div` の中身だけを `<Onboarding>` にする
- **Task 6：** `CoachMarks` の最初の段階は**描画中に決めない**。`useState<number | null>(null)`（測定中）で始め、`useLayoutEffect` で `nextCoachIndex` を呼ぶ。`reload()` と `setCoaching(true)` が同じ描画にまとまると、描画中の `querySelector` は前の DOM を見て全部「無い」と判断し、黙って終わる。`onboarding-check.mjs` は吹き出しが**出たこと**を確かめる
- 分割と伸ばしたい種目も両方向で検査される（`verifyDeclaredHaveADay` を `SetSplitCycle` と `SetDeclaredExercises` が呼ぶ）。プリセットはどれも全区分を持つので、伸ばしたい種目がどの日にも出られない組み合わせは作れない。順序は分割→種目のままでよい

---

## File Structure

**Go**

| ファイル | 役割 |
| --- | --- |
| `internal/domain/account/onboarding.go`（新規） | `OnboardingReader` / `OnboardingWriter` |
| `internal/domain/account/accounttest/contract.go` | `Repos.Onboarding` と `RunOnboardingContract` |
| `internal/domain/account/repository_test.go` | 形と読み書き分離の固定に2つの口を足す |
| `internal/infrastructure/memory/account_repositories.go` | `OnboardingRepository` |
| `internal/infrastructure/postgres/migrations/0020_onboarding.sql`（新規） | 表と既存利用者の埋め |
| `internal/infrastructure/postgres/onboarding_repository.go`（新規） | Postgres 実装 |
| `internal/infrastructure/postgres/onboarding_migration_test.go`（新規） | 0020 の埋めの検査 |
| `internal/application/query/onboarding.go`（新規） | `query.Onboarding.Completed` |
| `internal/application/usecase/complete_onboarding.go`（新規） | `usecase.CompleteOnboarding.Execute` |
| `internal/presentation/httpapi/{handler.go,router.go,dto.go}` | 口2つ |
| `internal/presentation/httpapi/onboarding_test.go`（新規） | HTTP の検査 |
| `cmd/api/main.go` | 配線、開発用利用者の完了 |

**Web**

| ファイル | 役割 |
| --- | --- |
| `web/src/api/types.ts` | `OnboardingResponse` |
| `web/src/app/useLiftplan.ts` | `data.onboarded` |
| `web/src/app/onboarded.ts` / `.test.ts`（新規） | 取得結果から「済んだか」を決める |
| `web/src/features/onboarding/onboarding.ts` / `.test.ts`（新規） | 下書き、段階、送る列 |
| `web/src/features/onboarding/useOnboarding.ts`（新規） | 読み込みと送信 |
| `web/src/features/onboarding/Onboarding.tsx`（新規） | 4段階の描画 |
| `web/src/features/onboarding/coach.ts` / `.test.ts`（新規） | コーチマークの段階 |
| `web/src/features/onboarding/CoachMarks.tsx`（新規） | 切り抜きと吹き出し |
| `web/src/features/today/{ExerciseCard,Today}.tsx`、`web/src/ui/Card.tsx` | `data-coach` |
| `web/src/app/App.tsx` | 出し分けとコーチマーク |
| `web/scripts/onboarding-check.mjs`（新規） | 実画面の検査 |

---

### Task 1: ドメインの口とインメモリ実装

**Files:**
- Create: `internal/domain/account/onboarding.go`
- Modify: `internal/domain/account/accounttest/contract.go`, `internal/domain/account/repository_test.go`, `internal/infrastructure/memory/account_repositories.go`, `internal/infrastructure/memory/account_repositories_test.go`, `internal/infrastructure/postgres/account_repository_test.go`（`Repos` に足したフィールドを埋めるだけ。Postgres 実装は Task 2）

**Interfaces:**
- Produces:
  - `account.OnboardingReader{ OnboardingCompleted(ctx, UserID) (bool, error) }`
  - `account.OnboardingWriter{ CompleteOnboarding(ctx, UserID, time.Time) error }`
  - `accounttest.OnboardingRepo`, `accounttest.Repos.Onboarding`, `accounttest.RunOnboardingContract(t, newRepos)`
  - `memory.NewOnboardingRepository() *memory.OnboardingRepository`

- [ ] **Step 1: 契約テストを書く**（`accounttest/contract.go` に追記）

```go
// OnboardingRepo は読み書き両方を備えた案内の実装。
type OnboardingRepo interface {
	account.OnboardingReader
	account.OnboardingWriter
}

// RunOnboardingContract は案内の口が満たすべき契約を回す。
func RunOnboardingContract(t *testing.T, newRepos func(*testing.T) Repos) {
	t.Helper()
	ctx := context.Background()

	t.Run("行が無ければまだ", func(t *testing.T) {
		r := newRepos(t).Onboarding
		got, err := r.OnboardingCompleted(ctx, userA)
		if err != nil || got {
			t.Fatalf("OnboardingCompleted = %v, %v。false, nil であること", got, err)
		}
	})
	t.Run("済ませたら済み、別の利用者はまだ", func(t *testing.T) {
		r := newRepos(t).Onboarding
		if err := r.CompleteOnboarding(ctx, userA, at); err != nil {
			t.Fatalf("CompleteOnboarding: %v", err)
		}
		if got, err := r.OnboardingCompleted(ctx, userA); err != nil || !got {
			t.Fatalf("userA = %v, %v。true であること", got, err)
		}
		if got, err := r.OnboardingCompleted(ctx, userB); err != nil || got {
			t.Fatalf("userB = %v, %v。false のままであること", got, err)
		}
	})
	t.Run("2回済ませてもエラーにならない", func(t *testing.T) {
		r := newRepos(t).Onboarding
		for i := range 2 {
			if err := r.CompleteOnboarding(ctx, userA, at.Add(time.Duration(i)*time.Hour)); err != nil {
				t.Fatalf("%d回目: %v", i+1, err)
			}
		}
	})
}
```

`Repos` に `Onboarding OnboardingRepo` を足す。memory と postgres の `newRepos` / `newAccountRepos` に `Onboarding:` を足し、memory 側に `TestOnboardingRepository_Contract` を足す。

`repository_test.go` の形の固定に `stubOnboardingRepo` と2行、読み書き分離のケースに `OnboardingReader: [OnboardingCompleted]`、`OnboardingWriter: [CompleteOnboarding]` を足す。

- [ ] **Step 2: 走らせて、未定義で落ちることを確かめる**

Run: `go test ./internal/domain/account/... ./internal/infrastructure/memory/...`
Expected: コンパイルエラー（`account.OnboardingReader` 未定義）

- [ ] **Step 3: 口とインメモリ実装を書く**

`internal/domain/account/onboarding.go`：

```go
package account

import (
	"context"
	"time"
)

// OnboardingReader は初回の案内を済ませたかを読む。
//
// 行が無いことを「まだ」と読む。初回ログインで行を作らずに済み、既存の
// 利用者はマイグレーションで済んだ行を持つ。
type OnboardingReader interface {
	OnboardingCompleted(ctx context.Context, user UserID) (bool, error)
}

// OnboardingWriter は初回の案内を済ませたことを記録する。
//
// 何度呼んでも同じ結果になる。画面は送信に失敗すると最初から送り直すので、
// 2回目を拒むと、済んだのに「済ませられない」と出る。
type OnboardingWriter interface {
	CompleteOnboarding(ctx context.Context, user UserID, at time.Time) error
}
```

`memory/account_repositories.go`：

```go
// OnboardingRepository は案内を済ませた時刻を利用者ごとに持つ。
type OnboardingRepository struct {
	mu   sync.RWMutex
	done map[account.UserID]time.Time
}

func NewOnboardingRepository() *OnboardingRepository {
	return &OnboardingRepository{done: map[account.UserID]time.Time{}}
}

// OnboardingCompleted は済ませた記録があるかを返す。
func (r *OnboardingRepository) OnboardingCompleted(_ context.Context, user account.UserID) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.done[user]
	return ok, nil
}

// CompleteOnboarding は済ませた時刻を記録する。既にあれば最初の時刻を残す。
func (r *OnboardingRepository) CompleteOnboarding(_ context.Context, user account.UserID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.done[user]; !ok {
		r.done[user] = at
	}
	return nil
}
```

末尾の `var (...)` に2行足す。

- [ ] **Step 4: 緑を確かめる。** `go test ./internal/domain/account/... ./internal/infrastructure/memory/...` → PASS

- [ ] **Step 5: 変異。** memory の `OnboardingCompleted` を `return true, nil` にして赤、戻す

- [ ] **Step 6: Commit** `feat(account): 初回の案内を済ませたかを持つ口`

---

### Task 2: Postgres の表と実装

**Files:**
- Create: `internal/infrastructure/postgres/migrations/0020_onboarding.sql`, `internal/infrastructure/postgres/onboarding_repository.go`, `internal/infrastructure/postgres/onboarding_migration_test.go`
- Modify: `internal/infrastructure/postgres/account_repository_test.go`

**Interfaces:**
- Consumes: Task 1 の口
- Produces: `postgres.NewOnboardingRepository(pool) *postgres.OnboardingRepository`

- [ ] **Step 1: テストを書く。**`account_repository_test.go` の `newAccountRepos` に `Onboarding: postgres.NewOnboardingRepository(pool)`、`TestOnboardingRepository_Contract` を足す。`onboarding_migration_test.go`：

```go
// 既存の利用者を済んだものとして扱う（0020）。
//
// 全部当てたあとで program に行を入れ、0020 をもう一度流す（IF NOT EXISTS と
// ON CONFLICT で何度流しても同じ）。0018 の検査と同じ形。
func TestMigration0020_MarksExistingUsersOnboarded(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	sql, err := os.ReadFile("migrations/0020_onboarding.sql")
	if err != nil {
		t.Fatalf("0020 を読めない: %v", err)
	}

	existing := mustUser(t, "11111111-1111-4111-8111-111111111111")
	fresh := mustUser(t, "22222222-2222-4222-8222-222222222222")
	saveDefaultProgram(t, pool, existing)

	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("0020 が失敗: %v", err)
	}

	repo := postgres.NewOnboardingRepository(pool)
	for _, c := range []struct {
		name string
		user account.UserID
		want bool
	}{{"program がある利用者", existing, true}, {"program が無い利用者", fresh, false}} {
		got, err := repo.OnboardingCompleted(ctx, c.user)
		if err != nil || got != c.want {
			t.Errorf("%s: %v, %v。%v であること", c.name, got, err, c.want)
		}
	}
}
```

`saveDefaultProgram` は `seed.DefaultProgram(seed.Exercises())` を `postgres.NewProgramRepository(pool).Save` するヘルパー（同ファイルに置く。既存に同等があれば使う）。

- [ ] **Step 2: 赤を確かめる**（埋め込み Postgres を立てて `TEST_DATABASE_URL` を渡す）。`go test ./internal/infrastructure/postgres/ -run 'Onboarding|0020'` → コンパイルエラー

- [ ] **Step 3: 実装**

`0020_onboarding.sql`：

```sql
-- 初回の案内を済ませた利用者。行が無い＝まだ。
--
-- program の列にしないのは、Program が訓練の設定を持つ集約で、画面の
-- 進み具合は計画の導出に一切効かないため。
CREATE TABLE IF NOT EXISTS onboarding (
    user_id      uuid        PRIMARY KEY,
    completed_at timestamptz NOT NULL
);

-- 既存の利用者は済んだものとして扱う。program は初回ログインで必ず作られる
-- ので、program がある＝この版より前にログインした利用者。
INSERT INTO onboarding (user_id, completed_at)
SELECT user_id, now() FROM program
ON CONFLICT (user_id) DO NOTHING;
```

`onboarding_repository.go`：

```go
// OnboardingRepository は初回の案内の Postgres 実装。
type OnboardingRepository struct {
	pool *pgxpool.Pool
}

func NewOnboardingRepository(pool *pgxpool.Pool) *OnboardingRepository {
	return &OnboardingRepository{pool: pool}
}

// OnboardingCompleted は済ませた行があるかを返す。
func (r *OnboardingRepository) OnboardingCompleted(ctx context.Context, user account.UserID) (bool, error) {
	var done bool
	if err := r.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM onboarding WHERE user_id = $1)", user.String(),
	).Scan(&done); err != nil {
		return false, wrapUnavailable(err, "案内の状態を読めない")
	}
	return done, nil
}

// CompleteOnboarding は済ませた時刻を記録する。既にあれば最初の時刻を残す。
func (r *OnboardingRepository) CompleteOnboarding(ctx context.Context, user account.UserID, at time.Time) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO onboarding (user_id, completed_at) VALUES ($1, $2)
		ON CONFLICT (user_id) DO NOTHING`, user.String(), at); err != nil {
		return wrapUnavailable(err, "案内を済ませたことを保存できない")
	}
	return nil
}
```

- [ ] **Step 4: 緑。**`TEST_DATABASE_URL=... go test ./internal/infrastructure/postgres/...` → PASS（既存のマイグレーションテストも）

- [ ] **Step 5: 変異。**SQL の `SELECT user_id, now() FROM program` を消して 0020 テストが赤、戻す

- [ ] **Step 6: Commit** `feat(postgres): 初回の案内を済ませた利用者の表（0020）`

---

### Task 3: 経路（query / usecase / HTTP）と配線

**Files:**
- Create: `internal/application/query/onboarding.go`, `internal/application/usecase/complete_onboarding.go`, `internal/presentation/httpapi/onboarding_test.go`
- Modify: `internal/presentation/httpapi/{handler.go,router.go,dto.go,handler_test.go}`, `internal/presentation/httpapi/user_internal_test.go`（`Dependencies` を埋める）, `cmd/api/main.go`, `cmd/api/main_test.go`

**Interfaces:**
- Consumes: Task 1・2
- Produces:
  - `query.NewOnboarding(account.OnboardingReader) *query.Onboarding`、`(*Onboarding).Completed(ctx, UserID) (bool, error)`
  - `usecase.NewCompleteOnboarding(account.OnboardingWriter, func() time.Time) *usecase.CompleteOnboarding`、`Execute(ctx, UserID) error`
  - `httpapi.Dependencies.Onboarding`, `.CompleteOnboarding`
  - `GET /api/onboarding` → `{"completed": bool}`、`PUT /api/onboarding/completed` → 204

- [ ] **Step 1: HTTP テストを書く**（`onboarding_test.go`）

```go
func TestOnboarding_StartsPendingAndCompletes(t *testing.T) {
	h := newServer(t, true)

	if got := getOnboarding(t, h); got {
		t.Fatalf("始めは completed=false であること")
	}
	rec := do(t, h, http.MethodPut, "/api/onboarding/completed", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT のステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := getOnboarding(t, h); !got {
		t.Fatalf("PUT のあとは completed=true であること")
	}
	// 冪等。画面は失敗すると最初から送り直す
	if rec := do(t, h, http.MethodPut, "/api/onboarding/completed", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("2回目の PUT が %d", rec.Code)
	}
}
```

未認証の 401 は `session_auth_test.go` の既存の網（`/api/` 全体）が見ているかを確認し、経路の一覧を持つテストがあればそこへ足す。

`cmd/api/main_test.go` に：既定で `GET /api/onboarding` が `true`、`DEV_ONBOARDING_PENDING=1` で `false`。

- [ ] **Step 2: 赤。**`go test ./internal/presentation/... ./cmd/...` → 404 またはコンパイルエラー

- [ ] **Step 3: 実装**

`query/onboarding.go`：

```go
// Onboarding は初回の案内を済ませたかを読む経路。
type Onboarding struct {
	repo account.OnboardingReader
}

func NewOnboarding(repo account.OnboardingReader) *Onboarding {
	return &Onboarding{repo: repo}
}

// Completed はその利用者が案内を済ませたかを返す。
func (q *Onboarding) Completed(ctx context.Context, user account.UserID) (_ bool, err error) {
	defer func() { err = apperror.Classify(err) }()
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("読み取りが中断された: %w", err)
	}
	done, err := q.repo.OnboardingCompleted(ctx, user)
	if err != nil {
		return false, fmt.Errorf("案内の状態の取得に失敗: %w", err)
	}
	return done, nil
}
```

`usecase/complete_onboarding.go`：同じ形で `Execute(ctx, user)` が `w.CompleteOnboarding(ctx, user, u.now())`。既存の usecase がエラーをどう分類しているか（`apperror.Classify` を出口で呼ぶか）を `set_frequency.go` に合わせる。

ハンドラ：

```go
// handleGetOnboarding は初回の案内を済ませたかを返す。
func (h *Handler) handleGetOnboarding(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	done, err := h.onboarding.Completed(r.Context(), user)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, onboardingDTO{Completed: done})
}

// handlePutOnboardingCompleted は案内を済ませたことを記録する。何度呼んでも 204。
func (h *Handler) handlePutOnboardingCompleted(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if err := h.completeOnboarding.Execute(r.Context(), user); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`dto.go`: `type onboardingDTO struct { Completed bool \`json:"completed"\` }`。`Dependencies` / `Handler` / `NewHandler` の検査に2つ。テストの `dependencies()` は `memory.NewOnboardingRepository()` で埋める。

`main.go`：`onboardingStore` を `repositories` に足し、両構成で組む。インメモリ構成で：

```go
onboarding := memory.NewOnboardingRepository()
// 開発用の利用者は済んだ状態で始める。画面の検査スクリプトが設定画面に
// 阻まれないように。案内そのものを検査するときだけ未完了で始める。
if os.Getenv("DEV_ONBOARDING_PENDING") != "1" {
	if err := onboarding.CompleteOnboarding(ctx, devUser, time.Now()); err != nil {
		return repositories{}, fmt.Errorf("開発用の利用者の案内を済ませられない: %w", err)
	}
}
```

- [ ] **Step 4: 緑。**`go test ./...`

- [ ] **Step 5: 変異。**ハンドラの `Completed: done` を `Completed: true` にして赤、戻す

- [ ] **Step 6: Commit** `feat(api): 初回の案内の状態を読み書きする口`

---

### Task 4: 設定画面の判断（`onboarding.ts`）

**Files:**
- Create: `web/src/features/onboarding/onboarding.ts`, `web/src/features/onboarding/onboarding.test.ts`

**Interfaces:**
- Consumes: `isSplitSelectable`, `matchingPresetKey`, `splitBody`（`../settings/split`）、`toggleDeclared`（`../today/declared`）
- Produces:

```ts
export type Draft = {
  perWeek: number;
  split: SplitPreset | null; // null は全身法
  exercises: number;
  sets: number;
  declared: string[];
};
export const STEPS = ['frequency', 'split', 'volume', 'declared'] as const;
export type Step = (typeof STEPS)[number];
export type PutRequest = { path: string; body?: unknown };
export function draftFrom(program: Program, presets: readonly SplitPreset[]): Draft;
export function withFrequency(draft: Draft, perWeek: number): Draft;
export function planOnboardingRequests(current: Program, draft: Draft): PutRequest[];
```

- [ ] **Step 1: テストを書く**

```ts
const fiveWay: SplitPreset = { key: 'five_way', name: '5分割', splits: [...5件], min_frequency_per_week: 4 };
const upperLower: SplitPreset = { key: 'upper_lower', name: '上下2分割', splits: [...2件], min_frequency_per_week: 0 };
const presets = [upperLower, fiveWay];
const base: Program = { per_week: 3, exercises_per_session: 4, sets_per_exercise: 3, splits: [], declared_exercises: ['bench', 'deadlift', 'squat'], ... };

describe('draftFrom', () => {
  it('全身法は split=null、プリセットと一致すればそれ', ...);
});

describe('withFrequency', () => {
  it('下限に足りなくなった分割は全身法に戻す', () => {
    const d = { ...draftFrom(base, presets), perWeek: 5, split: fiveWay };
    expect(withFrequency(d, 3).split).toBeNull();
  });
  it('足りていれば分割を残す', () => {
    expect(withFrequency({ ...draftFrom(base, presets), split: upperLower }, 2).split).toBe(upperLower);
  });
});

describe('planOnboardingRequests', () => {
  const paths = (rs: PutRequest[]) => rs.map((r) => r.path);
  it('全身法・週3 → 5分割・週5 は回数を先に送る', () => {
    const d = { ...draftFrom(base, presets), perWeek: 5, split: fiveWay };
    expect(paths(planOnboardingRequests(base, d))).toEqual([
      '/api/program/frequency', '/api/program/split', '/api/program/volume',
      '/api/program/declared', '/api/onboarding/completed',
    ]);
  });
  it('5分割・週5 → 全身法・週2 は分割を先に送る（途中で失敗して選び直した場合）', () => {
    const cur = { ...base, per_week: 5, splits: fiveWay.splits };
    const d = { ...draftFrom(cur, presets), perWeek: 2, split: null };
    expect(paths(planOnboardingRequests(cur, d)).slice(0, 2)).toEqual([
      '/api/program/split', '/api/program/frequency',
    ]);
  });
  it('本文は既存の設定画面と同じ形', () => {
    const d = { ...draftFrom(base, presets), perWeek: 4, exercises: 5, sets: 2, declared: ['bench'] };
    const rs = planOnboardingRequests(base, d);
    expect(rs.find((r) => r.path === '/api/program/frequency')?.body).toEqual({ per_week: 4 });
    expect(rs.find((r) => r.path === '/api/program/volume')?.body).toEqual({ exercises_per_session: 5, sets_per_exercise: 2 });
    expect(rs.find((r) => r.path === '/api/program/split')?.body).toEqual({ splits: [] });
    expect(rs.find((r) => r.path === '/api/program/declared')?.body).toEqual({ declared_exercises: ['bench'] });
  });
  it('最後は必ず完了で、本文を持たない', () => {
    const rs = planOnboardingRequests(base, draftFrom(base, presets));
    expect(rs.at(-1)).toEqual({ path: '/api/onboarding/completed' });
  });
});
```

- [ ] **Step 2: 赤。**`cd web && pnpm vitest run src/features/onboarding` → モジュールが無い

- [ ] **Step 3: 実装**

```ts
export function draftFrom(program: Program, presets: readonly SplitPreset[]): Draft {
  const key = matchingPresetKey(program, presets);
  return {
    perWeek: program.per_week,
    split: presets.find((p) => p.key === key) ?? null,
    exercises: program.exercises_per_session,
    sets: program.sets_per_exercise,
    declared: [...program.declared_exercises],
  };
}

export function withFrequency(draft: Draft, perWeek: number): Draft {
  const split = draft.split && isSplitSelectable(draft.split, perWeek) ? draft.split : null;
  return { ...draft, perWeek, split };
}

export function planOnboardingRequests(current: Program, draft: Draft): PutRequest[] {
  const frequency = { path: '/api/program/frequency', body: { per_week: draft.perWeek } };
  const split = { path: '/api/program/split', body: splitBody(draft.split) };
  const splitFirst = (draft.split?.min_frequency_per_week ?? 0) <= current.per_week;
  return [
    ...(splitFirst ? [split, frequency] : [frequency, split]),
    { path: '/api/program/volume', body: { exercises_per_session: draft.exercises, sets_per_exercise: draft.sets } },
    { path: '/api/program/declared', body: { declared_exercises: [...draft.declared].sort() } },
    { path: '/api/onboarding/completed' },
  ];
}
```

規則は「新しい分割の下限 ≤ 今の回数」だけで決まり、今の分割の下限は使わない（spec の証明どおり、これが偽なら回数を先に送れば必ず通る）。

- [ ] **Step 4: 緑。**

- [ ] **Step 5: 変異。**`<=` を `>` に、完了の行を消す、`withFrequency` の判定を外す、をそれぞれ入れて赤

- [ ] **Step 6: Commit** `feat(web): 初回の設定で送る順と本文を決める`

---

### Task 5: 設定画面（取得・手順・描画・App の出し分け）

**Files:**
- Create: `web/src/app/onboarded.ts`, `web/src/app/onboarded.test.ts`, `web/src/features/onboarding/useOnboarding.ts`, `web/src/features/onboarding/Onboarding.tsx`
- Modify: `web/src/api/types.ts`, `web/src/app/useLiftplan.ts`, `web/src/app/App.tsx`

**Interfaces:**
- Consumes: Task 4、`ExercisePicker`（`../settings/ExercisePicker`）、`describePutFailure`、`lockedDeclared`、`send` / `getJSON`
- Produces:
  - `export type OnboardingResponse = { completed: boolean }`
  - `export function onboardedFrom(res: OnboardingResponse | null): boolean`（null＝取れなかった→true）
  - `Data.onboarded: boolean`（`empty` では true）
  - `useOnboarding(onDone: () => Promise<void>)` → `{ program, presets, draft, step, note, busy, setDraft, next, back, finish }`
  - `<Onboarding exercises={Exercise[]} onDone={() => Promise<void>} />`

- [ ] **Step 1: テストを書く**（`onboarded.test.ts`）

```ts
it('取れなかったら済んだものとして扱う', () => expect(onboardedFrom(null)).toBe(true));
it('サーバーの値をそのまま使う', () => {
  expect(onboardedFrom({ completed: false })).toBe(false);
  expect(onboardedFrom({ completed: true })).toBe(true);
});
```

- [ ] **Step 2: 赤 → 実装 → 緑**

`useLiftplan.ts` の `Promise.all` に足す：

```ts
getJSON<OnboardingResponse>('/api/onboarding').catch((e: unknown) => {
  if (e instanceof Unauthorized) throw e;
  return null;
}),
```

`setData` に `onboarded: onboardedFrom(onboarding)`。冒頭のコメント（ここで読むもの）を更新する。

`useOnboarding.ts`：マウント時に `/api/program` と `/api/split-presets` を読み `draftFrom`。`finish` は `planOnboardingRequests` の列を順に `send({ path, method: 'PUT', body })`。`navigator.onLine` が false、または例外なら「つながらないので変えられません」。`!res.ok` なら `describePutFailure` で止める。全部通れば `await onDone()`。

`Onboarding.tsx`：段階ごとの見出し・1〜2行の説明・入力。段階1 `select[aria-label="週に通う回数"]`（1〜7、`withFrequency`）、段階2 全身法＋プリセットのボタン（`isSplitSelectable` で無効化し `splitUnselectableReason` を出す）、段階3 2つの `select`（種目数・セット数）、段階4 `ExercisePicker`（候補は `program.selected_exercises`、`lockedDeclared(draft.declared, program.focus_exercise)`）。下に「戻る」「次へ」／最後は「はじめる」。`note` を出す。

`App.tsx`：

```tsx
const onboarding = hasToken && !firstLoad && !data.onboarded;
const [coaching, setCoaching] = useState(false);
```

`onboarding` のときは `<main>` の中身を `<Onboarding exercises={data.exercises} onDone={async () => { await reload(); setCoaching(true); }} />` だけにし、歯車・`RestTimerBar`・`BottomNav` を出さない。

- [ ] **Step 3: 型・lint・テスト。**`pnpm typecheck && pnpm lint && pnpm test`

- [ ] **Step 4: Commit** `feat(web): 初回は通い方を決める画面を出す`

---

### Task 6: コーチマーク

**Files:**
- Create: `web/src/features/onboarding/coach.ts`, `web/src/features/onboarding/coach.test.ts`, `web/src/features/onboarding/CoachMarks.tsx`
- Modify: `web/src/features/today/ExerciseCard.tsx`, `web/src/features/today/Today.tsx`, `web/src/ui/Card.tsx`, `web/src/app/App.tsx`

**Interfaces:**
- Produces:

```ts
export type CoachTarget = 'target' | 'slot' | 'lane' | 'rest' | 'settings';
export type CoachStep = { target: CoachTarget; text: string };
export const COACH_STEPS: readonly CoachStep[];
/** from 以降で最初に要素がある段階の添字。無ければ null（終わり）。 */
export function nextCoachIndex(steps: readonly CoachStep[], from: number, present: (t: CoachTarget) => boolean): number | null;
```

- [ ] **Step 1: テストを書く**

```ts
const all = () => true;
it('最初は0', () => expect(nextCoachIndex(COACH_STEPS, 0, all)).toBe(0));
it('要素が無い段階を飛ばす', () =>
  expect(nextCoachIndex(COACH_STEPS, 0, (t) => t !== 'target' && t !== 'slot')).toBe(2));
it('全部無ければ null', () => expect(nextCoachIndex(COACH_STEPS, 0, () => false)).toBeNull());
it('最後の次は null', () => expect(nextCoachIndex(COACH_STEPS, COACH_STEPS.length, all)).toBeNull());
it('5段階を仕様の順で持つ', () =>
  expect(COACH_STEPS.map((s) => s.target)).toEqual(['target', 'slot', 'lane', 'rest', 'settings']));
```

- [ ] **Step 2: 赤 → 実装 → 緑**

```ts
export function nextCoachIndex(steps, from, present) {
  for (let i = from; i < steps.length; i++) {
    const s = steps[i];
    if (s && present(s.target)) return i;
  }
  return null;
}
```

文言は spec の表どおり。

`data-coach` を付ける：`Target` の重量の行（予定のカードの分岐だけ）に `data-coach="target"`、`ExerciseCard` の `i === 0` のボタンに `data-coach="slot"`、`SectionTitle` に任意の `coach?: string` を足して `Today` の最初に出る見出しにだけ渡す（`main` → `variation` → `accessories` → `adhoc` → `done` の最初の空でない区分）、「休憩を始める」に `data-coach="rest"`、`App` の歯車に `data-coach="settings"`。`querySelector` は文書順で最初の1つを返すので、カードには全部付けてよい。

`CoachMarks.tsx`：`index` を state に持ち、`nextCoachIndex(COACH_STEPS, 0, present)` で始める（`present = (t) => document.querySelector(\`[data-coach="${t}"]\`) !== null`）。null なら `onDone()`。描画は：

- 画面全体の透明な `div`（`fixed inset-0 z-50`）でクリックを吸う（押しても進まない）
- 対象の `getBoundingClientRect()` に合わせた `fixed` の枠（`box-shadow: 0 0 0 9999px rgb(0 0 0 / 0.6)`、角丸、`pointer-events: none`）
- 吹き出し（`role="dialog"` `aria-modal="true"` `aria-label="使い方"`）。対象の下に余白があれば下、無ければ上。文言、「スキップ」「次へ」（最後は「はじめる」）。開いたら「次へ」にフォーカス
- 段階が変わったら `scrollIntoView({ block: 'center' })` してから測る。`resize` / `scroll` で測り直す

`App.tsx`：`coaching && route === 'today' && !firstLoad && <CoachMarks onDone={() => setCoaching(false)} />`

- [ ] **Step 3: 変異。**`nextCoachIndex` の `present` を無視させて「飛ばす」テストが赤

- [ ] **Step 4: 型・lint・テスト。**

- [ ] **Step 5: Commit** `feat(web): 初回の設定のあとに今日の画面の使い方を指して見せる`

---

### Task 7: 実画面の検査

**Files:**
- Create: `web/scripts/onboarding-check.mjs`
- Modify: `web/README.md`（検査スクリプトの一覧に1行）

- [ ] **Step 1: スクリプトを書く**（`settings-check.mjs` の形に合わせる）

1. `#token=` でログイン。`select[aria-label="週に通う回数"]` が見え、`header button[aria-label="設定"]` が無いこと
2. 週5 → 次へ → 5分割 → 次へ → 次へ → はじめる
3. `GET /api/program` が `per_week=5`・分割5件、`GET /api/onboarding` が `completed=true`
4. `role=dialog[name=使い方]` が出る。「次へ」を押して最後の「はじめる」まで進み、閉じる。各段階の文言をログに出す
5. 再読み込み → 設定画面もコーチマークも出ない
6. `pageerror` / console error が0件

- [ ] **Step 2: 走らせる**（API は `DEV_ONBOARDING_PENDING=1` で立てる。手順は memory の reference に従う）。期待どおりに出ることを確かめ、スクリーンショットを scratchpad に残して見る

- [ ] **Step 3: 既存の検査を回す。**API を `DEV_ONBOARDING_PENDING` 無しで立て直し、`ui-check` `settings-check` `nav-check` `a11y-check` を1本ずつ（間で API を立て直す）

- [ ] **Step 4: 全体の検収。**`gofmt -l . && go vet ./... && go test ./...`、`cd web && pnpm typecheck && pnpm lint && pnpm test && pnpm build`

- [ ] **Step 5: Commit** `test(web): 初回の案内を実画面で確かめる検査`
