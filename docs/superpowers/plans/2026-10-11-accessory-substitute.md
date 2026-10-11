# 補助の代わりを出す Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 器具が埋まっていた補助のカードで「埋まっている」を押すと、週の残差から選んだ代わりの種目が重量まで付いてその場に出る。

**Architecture:** 割り振り器に「既に置いてある補助（Pinned）」を渡せるようにし、`SessionPlanner.Substitute` が今日の割り振りから1つ抜いた残りを Pinned にして空いた1枠だけを同じ損失で埋める。`GET /api/sessions/substitute` がそれを返し、画面は差し替えの状態だけを持つ。

**Tech Stack:** Go 1.x（標準ライブラリ・`net/http`）、React + TypeScript + vitest、playwright-core（配線の検査）

**Spec:** `docs/specs/2026-10-11-accessory-substitute-design.md`

## Global Constraints

- 対象は補助のレーンだけ。軸・バリエーションには「埋まっている」を出さず、API も 400 で拒む
- 選び方は割り振り器と同じ損失。新しい選択の規則を足さない
- 今日のほかの補助と先の回の割り振りは動かさない。今日の計画は一日中変わらない（`TestSessionPlanner_PlanIsFixedForTheWholeDay` を壊さない）
- `AllocationRequest.Pinned` が nil なら、割り振りの結果は今と完全に同じ
- 損失を減らす候補が無ければ代わりなし（`{"substitute": null}`、画面は「代わりなし。飛ばしてよい」）
- 1セットでも記録したカードには「埋まっている」を出さない
- オフラインでは「オフラインでは出せません」。通信できたが失敗したときは「代わりを読み込めませんでした」
- 差し替えは localStorage（`liftplan.substitutes`）に計画の日付つきで残す。開き直しても代わりのカードが出る。別の日付・壊れた値は読まない
- `date` は表示している計画の日付（`Session.date`）を送る。`rejected` は繰り返しのクエリパラメータ
- エラーは `%w` で包む。doc コメントは宣言名で始める。値オブジェクトは既存のものを使う
- 動作の変更と機械的な移動を混ぜない。PR は2本（PR 1：Task 1〜3、PR 2：Task 4〜5）
- 各テストは「書く → 期待した理由で赤 → 実装 → 緑 → 変異を入れて赤 → 戻す」。変異は手で入れて走らせる（CLAUDE.md）
- 検収：`gofmt -l .`、`go vet ./...`、`go test ./...`、`cd web && pnpm typecheck && pnpm test && pnpm format:check`

## Review Focus

1. **日付をまたいだ押下**：23:50 に読んだ計画で 0:05 に押す → 表示中の計画の日付を送るので 400 にならない（Task 4 の `substituteQuery` のテストで固定）
2. **開き直したとき**：代わりのカードが重量つきで残り、別の日に開けば元の計画に戻る（Task 4 の `swapsFor`／`decodeSwaps` と Task 5 の配線の検査で固定）。今日のうちに記録が増えても代わりは同じ：ほかの補助を何セット記録しても、同じ `replace` に同じ代わりが出る（Task 2 のテストで固定）
3. **代わりが今日すでに「種目を選んで記録」でやった種目**：カードは記録済みの枠つきで出て、「埋まっている」は出ない。これは受け入れる（Task 4 の `canMarkBlocked` のテストで固定）
4. **代わりの代わり**：差し替わったカードでもう一度押すと、元の `replace` のまま、それまでに出した代わりを全部 `rejected` に積む。元の種目を `replace` から外さない（Task 4 の `withResult`／`rejectedOf` のテストで固定）
5. **連打**：取得中にもう一度押しても2回は飛ばない（Task 5 のフックで `pending` 中は無視し、配線の検査で固定）

---

## ファイル構成

| ファイル | 役割 | Task |
|---|---|---|
| `internal/domain/training/planning/accessory_allocator.go` | `Pinned` と、置く手続きの切り出し | 1 |
| `internal/domain/training/planning/accessory_allocator_test.go` | Pinned のテスト | 1 |
| `internal/domain/training/planning/session_planner.go` | `planWeek` の切り出し、`Substitute`、`ErrNotPlannedAccessory` | 2 |
| `internal/domain/training/planning/substitute_test.go` | `Substitute` のテスト（新規） | 2 |
| `internal/application/usecase/get_substitute.go` | ユースケース（新規） | 3 |
| `internal/application/usecase/get_substitute_test.go` | 同テスト（新規） | 3 |
| `internal/application/apperror/classify.go` | `ErrNotPlannedAccessory` → 400 | 3 |
| `internal/presentation/httpapi/{handler.go,router.go,dto.go}` | 口・依存・DTO | 3 |
| `internal/presentation/httpapi/substitute_handler_test.go` | 口のテスト（新規） | 3 |
| `internal/presentation/httpapi/handler_test.go` | `dependencies()` に足す | 3 |
| `cmd/api/main.go` | 配線 | 3 |
| `README.md` | API の表に1行 | 3 |
| `web/src/api/types.ts` | `SubstituteResponse` | 4 |
| `web/src/features/today/substitute.ts` / `.test.ts` | 判断（新規） | 4 |
| `web/src/features/today/useSubstitute.ts` | 手順（新規） | 5 |
| `web/src/features/today/ExerciseCard.tsx` / `Today.tsx` | 描画・配線 | 5 |
| `web/scripts/substitute-check.mjs` | 配線の検査（新規） | 5 |
| `docs/user-stories.md` | B6 の行を更新 | 5 |

---

### Task 1: 割り振り器に Pinned を足す

**Files:**
- Modify: `internal/domain/training/planning/accessory_allocator.go`（`AllocationRequest` と `Allocate` のループ）
- Test: `internal/domain/training/planning/accessory_allocator_test.go`（末尾に追記）

**Interfaces:**
- Produces: `AllocationRequest.Pinned [][]exercise.ExerciseID`。`Allocate` の戻り値 `out[k]` は **`Pinned[k]` をその順で先頭に含み**、そのあとに新しく置いた種目が続く。`Sessions[k].Slots` は「Pinned の数 + 空き枠」として解釈する

- [ ] **Step 1: 失敗するテストを書く**

`accessory_allocator_test.go` の末尾に追記する。

```go
// TestAccessoryAllocator_PinnedStayAndOnlyFreeSlotsAreFilled は
// 「置いてある補助はそのまま、空き枠だけが埋まる」を守る。
//
// 回0に curl、回1に ham_a を置き、回0にだけ空きを1つ残す。Ham が最も
// 遅れているので回0の空きには Ham の種目が入る。ham_a と ham_b は同じ
// 減り幅だが、ham_a は回1に置いた（最終実施日が進んだ）ので、一度も
// やっていない ham_b が多様性の同点処理で勝つ。
//
// 【変異1】Pinned を置く手続きで lastPerformed／hasLast を進めない
// → ham_a と ham_b がどちらも未実施に見え、ID 順で ham_a が回0に入り赤。
// 【変異2】Pinned を out の先頭に入れない → out[0] が [ham_b] になり赤。
// 【変異3】Pinned で slots を減らさない → 回1にも空きが1つ残り、回1に
// 2つ目が入って赤。
func TestAccessoryAllocator_PinnedStayAndOnlyFreeSlotsAreFilled(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Hamstring: 12, training.Biceps: 6,
	})
	hamA := regionOnly(t, "ham_a", training.Hamstring, 1.0)
	hamB := regionOnly(t, "ham_b", training.Hamstring, 1.0)
	curl := regionOnly(t, "curl", training.Biceps, 1.0)
	all := []*exercise.Exercise{hamA, hamB, curl}

	req := planning.AllocationRequest{
		Target: target,
		Sessions: []planning.HorizonSession{
			noSplitSession(allocatorDay, 2),
			noSplitSession(allocatorDay.AddDays(3), 1),
		},
		Pinned:           [][]exercise.ExerciseID{{"curl"}, {"ham_a"}},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             all,
		Master:           all,
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if want := []string{"curl", "ham_b"}; !slices.Equal(idsOf(got[0]), want) {
		t.Errorf("回0 = %v, want %v", idsOf(got[0]), want)
	}
	if want := []string{"ham_a"}; !slices.Equal(idsOf(got[1]), want) {
		t.Errorf("回1 = %v, want %v（空きが無いので置いたものだけ）", idsOf(got[1]), want)
	}
}

// TestAccessoryAllocator_PinnedIsNotChosenAgainInTheSameSession は
// 置いてある種目が同じ回に2回入らないことを守る。
//
// 【変異】Pinned を置く手続きで chosenSet に入れない → 回0が
// [ham_a ham_a] になり赤。
func TestAccessoryAllocator_PinnedIsNotChosenAgainInTheSameSession(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Hamstring: 30})
	hamA := regionOnly(t, "ham_a", training.Hamstring, 1.0)

	req := planning.AllocationRequest{
		Target:           target,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 2)},
		Pinned:           [][]exercise.ExerciseID{{"ham_a"}},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{hamA},
		Master:           []*exercise.Exercise{hamA},
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if want := []string{"ham_a"}; !slices.Equal(idsOf(got[0]), want) {
		t.Errorf("回0 = %v, want %v", idsOf(got[0]), want)
	}
}

// TestAccessoryAllocator_PinnedCountsTowardCoverage は置いてある種目の刺激が
// 残差に数えられることを守る。
//
// 目標 Quad 1.5・Ham 1（4週窓で Quad 6・Ham 4）。quad_a を3セット置くと
// Quad はほぼ埋まり、空きには ham_a が入る（ΔL: Quad −0.19、Ham −0.98）。
//
// 【変異】Pinned を置く手続きで current を進めない → Quad が0のまま見え、
// quad_b の ΔL（−1.31）が最良になって赤。
func TestAccessoryAllocator_PinnedCountsTowardCoverage(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Quad: 1.5, training.Hamstring: 1,
	})
	quadA := regionOnly(t, "quad_a", training.Quad, 1.0)
	quadB := regionOnly(t, "quad_b", training.Quad, 1.0)
	hamA := regionOnly(t, "ham_a", training.Hamstring, 1.0)
	all := []*exercise.Exercise{quadA, quadB, hamA}

	req := planning.AllocationRequest{
		Target:           target,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 2)},
		Pinned:           [][]exercise.ExerciseID{{"quad_a"}},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             all,
		Master:           all,
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if want := []string{"quad_a", "ham_a"}; !slices.Equal(idsOf(got[0]), want) {
		t.Errorf("回0 = %v, want %v", idsOf(got[0]), want)
	}
}

// TestAccessoryAllocator_PinnedBlocksRecovery は先の回に置いた種目が
// 回復の判定に効くことを守る。
//
// 翌日（回1）に ham_a を置く。回復日数2なので今日（回0）は Ham を主働筋に
// する種目を避け、空きには curl が入る。Biceps は基準で既に6セット
// 入っているので、回復の判定が無ければ遅れている Ham の ham_b が選ばれる。
//
// 【変異】Pinned を置く手続きで allocated に入れない → 回0に ham_b が入り赤。
func TestAccessoryAllocator_PinnedBlocksRecovery(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Hamstring: 12, training.Biceps: 6,
	})
	hamA := regionOnly(t, "ham_a", training.Hamstring, 1.0)
	hamB := regionOnly(t, "ham_b", training.Hamstring, 1.0)
	curl := regionOnly(t, "curl", training.Biceps, 1.0)
	all := []*exercise.Exercise{hamA, hamB, curl}

	req := planning.AllocationRequest{
		Target:   target,
		Baseline: coverage(t, regionOnly(t, "past_curl", training.Biceps, 1.0), 6),
		Sessions: []planning.HorizonSession{
			noSplitSession(allocatorDay, 1),
			noSplitSession(allocatorDay.AddDays(1), 1),
		},
		Pinned:           [][]exercise.ExerciseID{nil, {"ham_a"}},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             all,
		Master:           all,
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if want := []string{"curl"}; !slices.Equal(idsOf(got[0]), want) {
		t.Errorf("回0 = %v, want %v（翌日に Ham を置いたので Ham は回復中）", idsOf(got[0]), want)
	}
}

// TestAccessoryAllocator_PinnedUnknownExerciseIsAnError は種目一覧に無い
// Pinned を黙って飛ばさないことを守る（飛ばすと刺激の数え漏れになる）。
func TestAccessoryAllocator_PinnedUnknownExerciseIsAnError(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Hamstring: 12})
	hamA := regionOnly(t, "ham_a", training.Hamstring, 1.0)

	req := planning.AllocationRequest{
		Target:           target,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 2)},
		Pinned:           [][]exercise.ExerciseID{{"ghost"}},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{hamA},
		Master:           []*exercise.Exercise{hamA},
		History:          setlog.NewHistory(nil),
	}

	if _, err := planning.DefaultAccessoryAllocator().Allocate(req); err == nil {
		t.Error("種目一覧に無い Pinned でエラーにならない")
	}
}
```

- [ ] **Step 2: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/domain/training/planning/ -run 'TestAccessoryAllocator_Pinned' -v`
Expected: コンパイルエラー `unknown field Pinned in struct literal of type planning.AllocationRequest`

- [ ] **Step 3: 実装する**

`AllocationRequest` の `History` の下にフィールドを足す。

```go
	// Pinned は既に置いてある補助。回ごとに Sessions と同じ並び（短ければ
	// 足りない回は何も置かない）。nil なら何も置かない。
	//
	// 置いた種目は戻り値の各回の先頭にその順で入る。Sessions[k].Slots は
	// Pinned[k] の数を含めた枠の数として読む（置くたびに1つ減る）。
	Pinned [][]exercise.ExerciseID
```

`Allocate` の中で、ループ本体の末尾にある「選んだ種目を置く」処理（`out[chosen.k] = append(...)` から `lastPerformed` の更新まで）を closure `place` に切り出し、ループの前で Pinned に通す。`type candidate` の宣言の直前に置く。

```go
	// place は回 k に種目 e を置く。貪欲法が1手選んだときと Pinned を
	// 置くときで同じ手続きを通す（片方だけ直すと Pinned の回が別物になる）。
	place := func(k int, e *exercise.Exercise) {
		out[k] = append(out[k], e.ID())
		chosenSet[k][e.ID()] = true
		slots[k]--

		for _, r := range e.Stimulus().Regions() {
			t := req.Target.Sets(r)
			if t <= 0 {
				continue
			}
			c, ok := e.Stimulus().Contribution(r)
			if !ok {
				continue
			}
			current[r] = training.Quantize(current[r] + c.TimesSets(req.SetsPerAccessory))
		}

		d := req.Sessions[k].Date
		allocated = append(allocated, stimulusEvent{date: d, ex: e})
		if !hasLast[e.ID()] || d.After(lastPerformed[e.ID()]) {
			lastPerformed[e.ID()] = d
			hasLast[e.ID()] = true
		}
	}

	for k, ids := range req.Pinned {
		if k >= len(req.Sessions) {
			return nil, fmt.Errorf("置いてある補助の回 %d が先の回の数 %d を超える", k, len(req.Sessions))
		}
		for _, id := range ids {
			e, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("置いてある補助 %s が種目一覧に無い", id)
			}
			place(k, e)
		}
	}
```

ループ本体の末尾は `place(chosen.k, chosen.e)` の1行に置き換える（元のコメントは `place` の上か、呼ぶ行の上に移す）。`lastPerformed`／`hasLast` を Pool の外の種目（Pinned だけにある種目）にも書くことになるが、map なので問題ない。

- [ ] **Step 4: 走らせて緑を確かめる**

Run: `go test ./internal/domain/training/planning/ -v -run 'TestAccessoryAllocator'`
Expected: PASS（既存の割り振り器のテストも全部緑。Pinned が nil なら置く処理は1回も走らない）

- [ ] **Step 5: 変異を入れて赤くなることを確かめる**

各テストの doc コメントにある変異を1つずつ手で入れ、`go test ./internal/domain/training/planning/ -run 'TestAccessoryAllocator_Pinned' -v` を走らせて、そのテストが期待した理由で赤くなることを確かめてから戻す。

- `place` の `lastPerformed`／`hasLast` 更新を Pinned のときだけ飛ばす → `PinnedStayAndOnlyFreeSlotsAreFilled` が赤
- `place` の `chosenSet` 更新を Pinned のときだけ飛ばす → `PinnedIsNotChosenAgainInTheSameSession` が赤
- `place` の `current` 更新を Pinned のときだけ飛ばす → `PinnedCountsTowardCoverage` が赤
- `place` の `allocated` 追加を Pinned のときだけ飛ばす → `PinnedBlocksRecovery` が赤
- Pinned のときだけ `slots[k]--` を飛ばす → `PinnedStayAndOnlyFreeSlotsAreFilled` が赤

（「Pinned のときだけ」は、Pinned のループで `place` を呼ぶ代わりに中身を写して該当行を消す形で入れる。）

- [ ] **Step 6: 全テストと検収**

Run: `gofmt -l . && go vet ./... && go test ./internal/domain/training/...`
Expected: 出力なし／PASS

- [ ] **Step 7: Commit**

```bash
git add internal/domain/training/planning/accessory_allocator.go internal/domain/training/planning/accessory_allocator_test.go
git commit -m "feat(planning): 割り振り器に既に置いてある補助（Pinned）を渡せるようにする"
```

---

### Task 2: `SessionPlanner.Substitute`

**Files:**
- Modify: `internal/domain/training/planning/session_planner.go`（`Forecast` の前半を `planWeek` に切り出し、`Substitute` と `ErrNotPlannedAccessory` を足す）
- Create: `internal/domain/training/planning/substitute_test.go`

**順序：Step 3（移動）→ Step 1 → Step 2 → Step 4 以降。**

**Interfaces:**
- Consumes: `AllocationRequest.Pinned`（Task 1）
- Produces:
  - `var ErrNotPlannedAccessory = errors.New(...)`（`planning` パッケージ）
  - `func (p SessionPlanner) Substitute(req PlanRequest, replace exercise.ExerciseID, rejected []exercise.ExerciseID) (PlannedSet, bool, error)`

- [ ] **Step 1: 失敗するテストを書く**

`substitute_test.go` を新規に作る。

```go
package planning_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// substituteRequest は「今日の補助が Ham の種目1つ」になる計画。
//
// 1回2種目（軸＋補助1つ）。Biceps は直近の curl で4週窓の目標ちょうどまで
// 埋めてあるので、curl を足すと超過になり候補として損失を減らさない。
// 残差が残っているのは Ham だけで、Ham の候補は leg_curl と nordic の2つ。
// nordic には履歴があり重量が付く。
func substituteRequest(t *testing.T) planning.PlanRequest {
	t.Helper()
	pool := []*exercise.Exercise{
		mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mainExercise(t, "squat", map[training.MuscleRegion]float64{training.Quad: 1.0}),
		mainExercise(t, "deadlift", map[training.MuscleRegion]float64{training.Erector: 1.0}),
		mkAccessory(t, "leg_curl", map[training.MuscleRegion]float64{training.Hamstring: 1.0}),
		mkAccessory(t, "nordic", map[training.MuscleRegion]float64{training.Hamstring: 1.0}),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
	}
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Hamstring: 12, training.Biceps: 3,
	})
	prog, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 2, 3),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "leg_curl", "nordic", "curl"},
		big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	logs := planHistory(t)
	// Biceps を4週窓で12セット（目標3×4週）まで埋める。窓は先の回の最後
	// （今日+5日）から27日前までなので、22日より前の記録は数えられない。
	// 今日から2日以上離すのは、回復の判定で curl が弾かれて「損失を減らさない」
	// を確かめたことにならないようにするため。
	for i := range 3 {
		for s := range 4 {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("curl-%d-%d", i, s),
				planMonday.AddDays(-7*i-3), "curl", 15, 10, 2))
		}
	}
	// nordic は推定1RMが立つ量だけやってある。
	for i, daysAgo := range []int{30, 23, 16} {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("nordic-%d", i),
			planMonday.AddDays(-daysAgo), "nordic", 20, 8, 2))
	}

	return planning.PlanRequest{
		Program: prog, Target: target, Pool: pool,
		History:    setlog.NewHistory(logs),
		Conditions: condition.NewConditionLog(nil),
		Date:       planMonday,
	}
}

// todaysOnlyAccessory は前提として今日の補助がちょうど1つであることを確かめて返す。
func todaysOnlyAccessory(t *testing.T, req planning.PlanRequest) planning.PlannedSet {
	t.Helper()
	plan := mustPlan(t, req)
	if len(plan.Accessories()) != 1 {
		t.Fatalf("前提: 今日の補助が1つでない: %v", plan.Accessories())
	}
	return plan.Accessories()[0]
}

// 今日の補助が Ham の片方なら、代わりはもう片方の Ham。
//
// 【変異】Substitute で候補のプールから replace を外さない → replace 自身が
// 戻ってくる（同じ減り幅で、ID か最終実施日の同点処理しだい）か、
// 少なくとも「もう片方」と一致しなくなって赤。
func TestSessionPlanner_Substitute_FillsTheSameDeficit(t *testing.T) {
	req := substituteRequest(t)
	planned := todaysOnlyAccessory(t, req)
	other := map[exercise.ExerciseID]exercise.ExerciseID{"leg_curl": "nordic", "nordic": "leg_curl"}
	want, ok := other[planned.ExerciseID()]
	if !ok {
		t.Fatalf("前提: 今日の補助が Ham の種目でない: %s", planned.ExerciseID())
	}

	got, ok, err := planning.DefaultSessionPlanner().Substitute(req, planned.ExerciseID(), nil)
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	if !ok {
		t.Fatal("代わりが出ない")
	}
	if got.ExerciseID() != want {
		t.Errorf("代わり = %s, want %s", got.ExerciseID(), want)
	}
}

// 出した代わりも埋まっていれば、Ham の候補が尽き、Biceps は足しても
// 超過なので代わりは無い。
//
// 【変異】Substitute で rejected をプールから外さない → 1つ目の代わりが
// もう一度出て赤。
func TestSessionPlanner_Substitute_NoneWhenNothingReducesTheLoss(t *testing.T) {
	req := substituteRequest(t)
	planned := todaysOnlyAccessory(t, req)
	planner := planning.DefaultSessionPlanner()

	first, ok, err := planner.Substitute(req, planned.ExerciseID(), nil)
	if err != nil || !ok {
		t.Fatalf("前提: 1つ目の代わりが出ない: ok=%v err=%v", ok, err)
	}
	got, ok, err := planner.Substitute(req, planned.ExerciseID(), []exercise.ExerciseID{first.ExerciseID()})
	if err != nil {
		t.Fatalf("Substitute: %v", err)
	}
	if ok {
		t.Errorf("候補が尽きたのに %s が出た", got.ExerciseID())
	}
}

// 処方は今日の補助と同じ規則で付く。履歴のある種目には重量が付く。
//
// 【変異】Substitute で prescribe の役割を accessoryRole 以外（variationRole）
// にする → RIR かレップ数が今日の補助と食い違って赤。
func TestSessionPlanner_Substitute_PrescribedLikeTodaysAccessories(t *testing.T) {
	req := substituteRequest(t)
	planned := todaysOnlyAccessory(t, req)

	got, ok, err := planning.DefaultSessionPlanner().Substitute(req, planned.ExerciseID(), nil)
	if err != nil || !ok {
		t.Fatalf("代わりが出ない: ok=%v err=%v", ok, err)
	}
	if got.Sets() != planned.Sets() || got.TargetRIR() != planned.TargetRIR() || got.TargetReps() != planned.TargetReps() {
		t.Errorf("処方が今日の補助と違う: got sets=%v rir=%v reps=%v, planned sets=%v rir=%v reps=%v",
			got.Sets(), got.TargetRIR(), got.TargetReps(),
			planned.Sets(), planned.TargetRIR(), planned.TargetReps())
	}
	_, hasWeight := got.Weight()
	if want := got.ExerciseID() == "nordic"; hasWeight != want {
		t.Errorf("%s の重量の有無 = %v, want %v（履歴があるのは nordic だけ）",
			got.ExerciseID(), hasWeight, want)
	}
}

// 補助でない種目（軸）や、予定に無い種目は差し替えられない。
//
// 【変異】Substitute の「回0の補助に replace があるか」の判定を外す →
// 軸の bench でも代わりが返って赤。
func TestSessionPlanner_Substitute_RejectsWhatIsNotTodaysAccessory(t *testing.T) {
	req := substituteRequest(t)
	plan := mustPlan(t, req)
	if len(plan.Main()) == 0 {
		t.Fatal("前提: 今日の軸が無い")
	}
	for _, id := range []exercise.ExerciseID{plan.Main()[0].ExerciseID(), "curl", "no_such"} {
		_, _, err := planning.DefaultSessionPlanner().Substitute(req, id, nil)
		if !errors.Is(err, planning.ErrNotPlannedAccessory) {
			t.Errorf("%s: err = %v, want ErrNotPlannedAccessory", id, err)
		}
	}
}

// 今日のうちに何を記録しても、同じ replace には同じ代わりが出る
// （今日の計画は一日中変わらない。D-116）。
//
// 【変異】Substitute の中で req.History.Before(req.Date) を通さずに
// 当日の記録まで渡す → 今日 nordic を記録すると最終実施日が進み、
// 結果が変わって赤。
func TestSessionPlanner_Substitute_IsFixedForTheWholeDay(t *testing.T) {
	req := substituteRequest(t)
	planned := todaysOnlyAccessory(t, req)
	planner := planning.DefaultSessionPlanner()

	before, ok, err := planner.Substitute(req, planned.ExerciseID(), nil)
	if err != nil || !ok {
		t.Fatalf("前提: 代わりが出ない: ok=%v err=%v", ok, err)
	}

	logs := append([]*setlog.SetLog{}, req.History.Logs()...)
	for i, id := range []string{"bench", "squat", "nordic", "leg_curl", "curl"} {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("today-%d", i), req.Date, id, 40, 8, 2))
	}
	req.History = setlog.NewHistory(logs)

	after, ok, err := planner.Substitute(req, planned.ExerciseID(), nil)
	if err != nil || !ok {
		t.Fatalf("今日の記録のあとで代わりが出ない: ok=%v err=%v", ok, err)
	}
	if planDiffSet(before, after) != "" {
		t.Errorf("今日の記録で代わりが変わった: %s", planDiffSet(before, after))
	}
}

// planDiffSet は2つの処方の食い違いを1行で返す。同じなら空。
func planDiffSet(a, b planning.PlannedSet) string {
	aw, aok := a.Weight()
	bw, bok := b.Weight()
	if a.ExerciseID() != b.ExerciseID() || aok != bok || (aok && aw != bw) ||
		a.Sets() != b.Sets() || a.TargetRIR() != b.TargetRIR() || a.TargetReps() != b.TargetReps() {
		return fmt.Sprintf("%s(%v,%v) → %s(%v,%v)", a.ExerciseID(), aw, aok, b.ExerciseID(), bw, bok)
	}
	return ""
}
```

注：`Weight` の比較が `!=` で書けない型なら、`aw.Kg() != bw.Kg()` に直す。`planDiff`（`session_planner_test.go:873`）が既にあれば、そちらの比べ方に揃える。

- [ ] **Step 2: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/domain/training/planning/ -run 'TestSessionPlanner_Substitute' -v`
Expected: コンパイルエラー `planner.Substitute undefined` と `undefined: planning.ErrNotPlannedAccessory`

- [ ] **Step 3: `Forecast` の前半を `planWeek` に切り出す（動作を変えない移動）**

`session_planner.go` で、`Forecast` の検証から `Allocate` の呼び出しまでを `planWeek` に移す。`Forecast` は `planWeek` を呼んでから、これまでどおり lineup を組んで `prescribe` する。コメント（当日の記録を落とす理由など）は `planWeek` の該当行と一緒に移す。

```go
// weekPlan は Forecast と Substitute が共有する、週ぶんの割り振りまでの途中経過。
type weekPlan struct {
	pool        []*exercise.Exercise
	estimable   setlog.History
	sessions    []ProjectedSession
	alloc       AllocationRequest
	allocations [][]exercise.ExerciseID
}

// planWeek は今日を含めた先の回を予測し、補助を割り振るところまでを行う。
// 当日の記録を落とすのはここ1箇所（Forecast のコメントの理由）。
func (p SessionPlanner) planWeek(req PlanRequest) (weekPlan, error) {
	// ここに Forecast の 84〜184 行目の中身を移す（検証、history/estimable、
	// declared の確認、ProjectHorizon、horizon、exclude、baseline、
	// setsPerAccessory、target、Allocate）。Allocate に渡していた
	// AllocationRequest はローカル変数 alloc に入れてから渡す。
	return weekPlan{
		pool: pool, estimable: estimable, sessions: sessions,
		alloc: alloc, allocations: allocations,
	}, nil
}
```

`Forecast` の残り：

```go
func (p SessionPlanner) Forecast(req PlanRequest) ([]PlannedSession, error) {
	w, err := p.planWeek(req)
	if err != nil {
		return nil, err
	}
	out := make([]PlannedSession, len(w.sessions))
	for k, sess := range w.sessions {
		// 元の 187〜209 行目。pool → w.pool、allocations → w.allocations、
		// estimable → w.estimable に置き換えるだけ。
	}
	return out, nil
}
```

**このステップは Step 1 の前に行う。**テストを先に置くとパッケージがコンパイルできず、移動で動作が変わっていないことを確かめられない。移動だけで1コミットにする（動作の変更と機械的な移動を混ぜない）。

Run: `go test ./internal/domain/training/...`
Expected: PASS（移動で動作が変わっていない）

```bash
git add internal/domain/training/planning/session_planner.go
git commit -m "refactor(planning): Forecast の割り振りまでを planWeek に切り出す"
```

- [ ] **Step 4: `Substitute` と番兵を足す**

`session_planner.go` の `Plan` の下に足す（import に `slices` を足す）。

```go
// ErrNotPlannedAccessory は差し替えを頼まれた種目が今日の補助に無いこと。
// 軸・バリエーション・予定に無い種目は差し替えない（設計書「補助だけ」）。
var ErrNotPlannedAccessory = errors.New("今日の補助に無い種目は差し替えられない")

// Substitute は今日の補助 replace の代わりを1つ導出する。
//
// 今日のほかの補助と先の回の割り振りは置いたまま（Pinned）、replace を
// 抜いて空いた1枠だけを割り振り器の同じ損失で埋める。候補からは replace と
// rejected（前に出したが、それも埋まっていた種目）を外す。損失を減らす
// 候補が無ければ2番目の戻り値が false。
//
// Forecast と同じ planWeek を通るので、今日の計画と別の計算にならず、
// 当日の記録を見ない（一日中同じ代わりが出る）。
func (p SessionPlanner) Substitute(
	req PlanRequest, replace exercise.ExerciseID, rejected []exercise.ExerciseID,
) (PlannedSet, bool, error) {
	w, err := p.planWeek(req)
	if err != nil {
		return PlannedSet{}, false, err
	}
	if !slices.Contains(w.allocations[0], replace) {
		return PlannedSet{}, false, fmt.Errorf("%s: %w", replace, ErrNotPlannedAccessory)
	}

	pinned := make([][]exercise.ExerciseID, len(w.allocations))
	for k, ids := range w.allocations {
		pinned[k] = slices.Clone(ids)
	}
	pinned[0] = slices.DeleteFunc(pinned[0], func(id exercise.ExerciseID) bool { return id == replace })

	// 空き枠は回0の1つだけ。先の回に空きを残すと、候補が今日ではなく
	// 先の回に入り、回0に何も入らないことがある。
	sessions := slices.Clone(w.alloc.Sessions)
	for k := range sessions {
		sessions[k].Slots = len(pinned[k])
	}
	sessions[0].Slots++

	again := w.alloc
	again.Sessions = sessions
	again.Pinned = pinned
	again.Pool = candidateAccessories(w.alloc.Pool, append([]exercise.ExerciseID{replace}, rejected...))

	out, err := p.accessory.Allocate(again)
	if err != nil {
		return PlannedSet{}, false, fmt.Errorf("代わりの割り振りに失敗: %w", err)
	}
	added := out[0][len(pinned[0]):]
	if len(added) == 0 {
		return PlannedSet{}, false, nil
	}
	e := findExercise(w.pool, added[0])
	if e == nil {
		// 到達しない。Allocate は Pool の中の種目しか置かない。
		return PlannedSet{}, false, fmt.Errorf("代わり %s が種目一覧に無い", added[0])
	}

	split, hasSplit := w.sessions[0].Split()
	s := p.prescribe(
		[]lineupEntry{{exercise: e, role: accessoryRole, reps: program.DefaultRepTargets()}},
		w.estimable, req.Conditions, req.Date, req.Program.SessionVolume().Sets(), split, hasSplit)
	return s.Accessories()[0], true, nil
}
```

- [ ] **Step 5: 走らせて緑を確かめる**

Run: `go test ./internal/domain/training/planning/ -v -run 'TestSessionPlanner_Substitute|PlanIsFixedForTheWholeDay|Forecast'`
Expected: PASS

- [ ] **Step 6: 変異を入れて赤くなることを確かめる**

各テストの doc コメントにある変異を1つずつ手で入れて走らせ、そのテストが赤くなることを確かめてから戻す。

- `append([]exercise.ExerciseID{replace}, rejected...)` を `rejected` だけにする → `FillsTheSameDeficit` が赤
- 同じ箇所を `[]exercise.ExerciseID{replace}` だけにする → `NoneWhenNothingReducesTheLoss` が赤
- `role: accessoryRole` を `role: variationRole` にする → `PrescribedLikeTodaysAccessories` が赤
- `slices.Contains` の判定ブロックを消す → `RejectsWhatIsNotTodaysAccessory` が赤
- `planWeek` の `history := req.History.Before(req.Date)` を `history := req.History` にする → `Substitute_IsFixedForTheWholeDay`（と既存の `PlanIsFixedForTheWholeDay`）が赤
- `sessions[k].Slots = len(pinned[k])` を `sessions[k].Slots = len(pinned[k]) + 1` にする（先の回に空きを残す） → `FillsTheSameDeficit` か `NoneWhenNothingReducesTheLoss` が赤になるかを見る。**緑のままなら**、この fixture では先の回に Ham が入る余地が無いということなので、`substituteRequest` の1回の種目数を3（`mustVolume(t, 3, 3)`）にした別のテスト `TestSessionPlanner_Substitute_DoesNotLeakIntoLaterSessions`（「代わりが出る」ことだけを確かめる）を足し、この変異で赤くなることを確かめる

- [ ] **Step 7: 全テストと検収**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: 出力なし／PASS

Run: `go test ./internal/domain/training/seed/ -run TestSimulation -v 2>&1 | tail -40`
Expected: 数字が main と同じ（`git stash` は使わない。比べるなら `git worktree add ../substitute-base origin/main` で旧版を別に回す）

- [ ] **Step 8: Commit**

```bash
git add internal/domain/training/planning/session_planner.go internal/domain/training/planning/substitute_test.go
git commit -m "feat(planning): 今日の補助の代わりを1つ導出する Substitute を足す"
```

---

### Task 3: `GET /api/sessions/substitute`

**Files:**
- Create: `internal/application/usecase/get_substitute.go`
- Create: `internal/application/usecase/get_substitute_test.go`
- Modify: `internal/application/apperror/classify.go`（`ErrNotPlannedAccessory` → `ErrInvalidInput`）
- Modify: `internal/presentation/httpapi/handler.go`（フィールド・`Dependencies`・検証・ハンドラ）
- Modify: `internal/presentation/httpapi/router.go`
- Modify: `internal/presentation/httpapi/dto.go`
- Modify: `internal/presentation/httpapi/handler_test.go`（`dependencies()`）
- Create: `internal/presentation/httpapi/substitute_handler_test.go`
- Modify: `cmd/api/main.go`
- Modify: `README.md`（API の表）

**Interfaces:**
- Consumes: `planning.SessionPlanner.Substitute`、`planning.ErrNotPlannedAccessory`（Task 2）
- Produces:
  - `usecase.GetSubstituteInput{Date training.Date; Replace exercise.ExerciseID; Rejected []exercise.ExerciseID}`
  - `(*usecase.GetSubstitute).Execute(ctx, user, in) (planning.PlannedSet, bool, error)`
  - HTTP：`GET /api/sessions/substitute?date=&replace=&rejected=...` → `200 {"substitute": plannedSetDTO | null}`、不正は 400

- [ ] **Step 1: 失敗するテストを書く（口）**

`substitute_handler_test.go` を新規に作る。

```go
package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// todaysAccessory は今日の補助の先頭を返す。newServer(t, true) は1回6種目
// なので補助が必ずある。
func todaysAccessory(t *testing.T, h http.Handler, date string) string {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/api/sessions?date="+date, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("前提: 今日の計画が取れない: %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Accessories []struct {
			ExerciseID string `json:"exercise_id"`
		} `json:"accessories"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if len(body.Accessories) == 0 {
		t.Fatal("前提: 今日の補助が無い")
	}
	return body.Accessories[0].ExerciseID
}

func TestGetSubstitute_ReturnsAnotherAccessory(t *testing.T) {
	h := newServer(t, true)
	replace := todaysAccessory(t, h, "2026-08-17")

	rec := do(t, h, http.MethodGet,
		"/api/sessions/substitute?date=2026-08-17&replace="+url.QueryEscape(replace), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if _, ok := raw["substitute"]; !ok {
		t.Fatalf("substitute のキーが無い: %s", rec.Body.String())
	}
	var body struct {
		Substitute *struct {
			ExerciseID string   `json:"exercise_id"`
			WeightKg   *float64 `json:"weight_kg"`
			Sets       int      `json:"sets"`
		} `json:"substitute"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if body.Substitute == nil {
		t.Fatal("同梱の全種目から選べるのに代わりが null")
	}
	if body.Substitute.ExerciseID == replace {
		t.Errorf("代わりが元の種目 %s と同じ", replace)
	}
	if body.Substitute.Sets <= 0 {
		t.Errorf("セット数が %d", body.Substitute.Sets)
	}
	// 履歴が無いので重量は null（自分で決める）。
	if body.Substitute.WeightKg != nil {
		t.Errorf("履歴が無いのに重量 %v が付いている", *body.Substitute.WeightKg)
	}
}

// rejected は繰り返しのクエリで受ける。1つ目の代わりを rejected に積むと
// 別の種目が出る。
//
// 【変異】ハンドラで rejected を Query().Get（先頭1つだけ）で読む → 2つ目に
// 積んだ種目が素通りして、それが返ってきて赤。
func TestGetSubstitute_RejectedIsRepeatable(t *testing.T) {
	h := newServer(t, true)
	replace := todaysAccessory(t, h, "2026-08-17")
	base := "/api/sessions/substitute?date=2026-08-17&replace=" + url.QueryEscape(replace)

	var seen []string
	for range 2 {
		q := base
		for _, id := range seen {
			q += "&rejected=" + url.QueryEscape(id)
		}
		rec := do(t, h, http.MethodGet, q, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
		}
		var body struct {
			Substitute *struct {
				ExerciseID string `json:"exercise_id"`
			} `json:"substitute"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("JSONが壊れている: %v", err)
		}
		if body.Substitute == nil {
			t.Fatalf("%d回目で代わりが null", len(seen)+1)
		}
		for _, id := range seen {
			if body.Substitute.ExerciseID == id {
				t.Fatalf("rejected に積んだ %s が出た（rejected=%v）", id, seen)
			}
		}
		seen = append(seen, body.Substitute.ExerciseID)
	}
}

func TestGetSubstitute_BadRequests(t *testing.T) {
	h := newServer(t, true)
	cases := []struct {
		name string
		path string
	}{
		{"日付なし", "/api/sessions/substitute?replace=leg_curl"},
		{"不正な日付", "/api/sessions/substitute?date=2026/08/17&replace=leg_curl"},
		{"replace なし", "/api/sessions/substitute?date=2026-08-17"},
		{"空の rejected", "/api/sessions/substitute?date=2026-08-17&replace=leg_curl&rejected="},
		{"軸は差し替えない", "/api/sessions/substitute?date=2026-08-17&replace=bench"},
		{"予定に無い種目", "/api/sessions/substitute?date=2026-08-17&replace=no_such_exercise"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, c.path, "")
			if rec.Code != http.StatusBadRequest {
				t.Errorf("ステータス = %d, want 400 body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGetSubstitute_NotConfigured(t *testing.T) {
	rec := do(t, newServer(t, false), http.MethodGet,
		"/api/sessions/substitute?date=2026-08-17&replace=leg_curl", "")
	if rec.Code != http.StatusConflict {
		t.Errorf("未設定が 409 にならない: %d", rec.Code)
	}
}
```

注：「軸は差し替えない」は `bench` が 2026-08-17 の軸でない日もある。その場合でも bench は宣言種目で補助の候補から外れているので、補助には出ず 400 になる（どちらの理由でも 400）。

- [ ] **Step 2: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/presentation/httpapi/ -run 'TestGetSubstitute' -v`
Expected: FAIL。ルートが無いので 404 か 405（`ステータスが誤り: 404` など）

- [ ] **Step 3: ユースケースのテストを書く**

`get_substitute_test.go` を、`get_forecast_test.go` の組み立て（memory のリポジトリ・`testUser`）に揃えて書く。確かめるのは次の2つ。

```go
// 予定に無い種目は入力の誤り（400）に分類される。
func TestGetSubstitute_NotPlannedIsInvalidInput(t *testing.T) {
	u := newGetSubstituteForTest(t) // get_forecast_test.go の組み立てを写した補助関数
	_, _, err := u.Execute(context.Background(), testUser, usecase.GetSubstituteInput{
		Date: training.MustDate(2026, time.August, 17), Replace: "no_such_exercise",
	})
	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
	if !errors.Is(err, planning.ErrNotPlannedAccessory) {
		t.Errorf("err = %v, ドメインの番兵が辿れない（%%w で包んでいない）", err)
	}
}

// プログラム未設定は ErrNotConfigured（GetForecast と同じ）。
func TestGetSubstitute_NotConfigured(t *testing.T) { /* get_forecast_test.go の同名のテストを写す */ }
```

`newGetSubstituteForTest` と未設定のテストは、`get_forecast_test.go` を開いて同じ組み立てを写すこと（`NewGetForecast` → `NewGetSubstitute` に置き換える）。

Run: `go test ./internal/application/usecase/ -run TestGetSubstitute -v`
Expected: コンパイルエラー `undefined: usecase.GetSubstituteInput`

- [ ] **Step 4: ユースケースを実装する**

`get_substitute.go` を新規に作る。組み立ては `GetForecast` と同じ（`get_forecast.go` の `Execute` を写し、最後の呼び出しだけ変える）。3つ目の同じ組み立てになるが、共通化は機械的な移動なので別の PR にする（Step 10 で issue に積む）。

```go
package usecase

// （import は get_forecast.go と同じ。exercise を足す）

// GetSubstituteInput は差し替えの入力。Date は画面が表示している計画の日付。
type GetSubstituteInput struct {
	Date     training.Date
	Replace  exercise.ExerciseID
	Rejected []exercise.ExerciseID
}

// GetSubstitute は今日の補助の代わりを1つ導出するユースケース。
// 組み立ては GetForecast と同じ。
type GetSubstitute struct {
	exercises  exercise.Reader
	logs       setlog.Reader
	conditions condition.Reader
	programs   program.Reader
	planner    planning.SessionPlanner
}

func NewGetSubstitute(
	exercises exercise.Reader,
	logs setlog.Reader,
	conditions condition.Reader,
	programs program.Reader,
	planner planning.SessionPlanner,
) *GetSubstitute {
	return &GetSubstitute{
		exercises: exercises, logs: logs,
		conditions: conditions, programs: programs, planner: planner,
	}
}

func (u *GetSubstitute) Execute(ctx context.Context, user account.UserID, in GetSubstituteInput) (_ planning.PlannedSet, _ bool, err error) {
	defer func() { err = apperror.Classify(err) }()

	if in.Date.IsZero() {
		return planning.PlannedSet{}, false, errors.New("対象日が指定されていない")
	}
	// ここから get_forecast.go の Execute と同じ取得（prog・target・pool・
	// history・conditions、ctx.Err() の確認を含む）。戻り値の形だけ
	// (planning.PlannedSet{}, false, err) に変える。

	return u.planner.Substitute(planning.PlanRequest{
		Program: prog, Target: target, Pool: pool,
		History: history, Conditions: conditions, Date: in.Date,
	}, in.Replace, in.Rejected)
}
```

`classify.go` の switch に1行足す（import に `planning` を足す）。

```go
	case errors.Is(err, planning.ErrNotPlannedAccessory):
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
```

Run: `go test ./internal/application/... -run 'TestGetSubstitute|TestClassify' -v`
Expected: PASS

- [ ] **Step 5: 口を実装する**

`dto.go` に足す。

```go
// substituteResponse は差し替えの応答。代わりが無ければ substitute は null。
type substituteResponse struct {
	Substitute *plannedSetDTO `json:"substitute"`
}
```

`handler.go`：`getForecast` の並びに `getSubstitute *usecase.GetSubstitute` を、`Dependencies` に `GetSubstitute *usecase.GetSubstitute` を、検証の switch に `case d.GetSubstitute == nil: return nil, errMissingDependency("GetSubstitute")` を、組み立てに `getSubstitute: d.GetSubstitute,` を足す。ハンドラは `handleGetForecast` の下に置く。

```go
func (h *Handler) handleGetSubstitute(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	raw := q.Get("date")
	if raw == "" {
		respondError(w, invalidInput("date クエリパラメータが必要である"))
		return
	}
	date, err := training.ParseDate(raw)
	if err != nil {
		respondError(w, invalidInput(err.Error()))
		return
	}
	replace := q.Get("replace")
	if replace == "" {
		respondError(w, invalidInput("replace クエリパラメータが必要である"))
		return
	}
	// rejected は繰り返しで受ける。カンマで割らないのは、種目 ID の文字種に
	// 依存しないため。
	var rejected []exercise.ExerciseID
	for _, id := range q["rejected"] {
		if id == "" {
			respondError(w, invalidInput("rejected が空である"))
			return
		}
		rejected = append(rejected, exercise.ExerciseID(id))
	}

	sub, found, err := h.getSubstitute.Execute(r.Context(), user, usecase.GetSubstituteInput{
		Date: date, Replace: exercise.ExerciseID(replace), Rejected: rejected,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	res := substituteResponse{}
	if found {
		dto := toPlannedSetDTO(sub)
		res.Substitute = &dto
	}
	writeJSON(w, http.StatusOK, res)
}
```

（`handler.go` が `exercise` をまだ import していなければ足す。ID の組み立てに既存の `exercise.NewExerciseID` のような検証付きコンストラクタがあれば、そちらを使って不正な ID を 400 にする。）

`router.go` の forecast の下に足す。

```go
	mux.HandleFunc("GET /api/sessions/substitute", h.handleGetSubstitute)
```

`handler_test.go` の `dependencies()` と `cmd/api/main.go` の `httpapi.Dependencies{...}` に、forecast の行の下へ足す。

```go
		GetSubstitute:    usecase.NewGetSubstitute(exercises, logs, conditions, programs, planning.DefaultSessionPlanner()),
```

（`main.go` は `planning.DefaultSessionPlanner()` ではなく、そこで使っている `planner` を渡す。）

- [ ] **Step 6: 走らせて緑を確かめる**

Run: `go test ./internal/presentation/httpapi/ -run 'TestGetSubstitute' -v`
Expected: PASS

- [ ] **Step 7: 変異を入れて赤くなることを確かめる**

- `q["rejected"]` のループを `if id := q.Get("rejected"); id != "" { rejected = append(rejected, exercise.ExerciseID(id)) }` にする → `TestGetSubstitute_RejectedIsRepeatable` が赤
- `classify.go` に足した case を消す → `TestGetSubstitute_BadRequests` の「軸は差し替えない」「予定に無い種目」が 500 になって赤、`TestGetSubstitute_NotPlannedIsInvalidInput` も赤
- `if found` を外して常に `&dto` を入れる → このテスト群では捕まらない（null の経路は Task 2 のドメインのテストが守る）。捕まらないことを確かめたら戻す

- [ ] **Step 8: README の API の表に1行足す**

`GET /api/sessions/forecast` の行の下へ。

```markdown
| GET | `/api/sessions/substitute?date=&replace=&rejected=` | 今日の補助 `replace` の代わりを1つ導出する。`rejected` は前に出した代わり（繰り返しで渡す）。無ければ `{"substitute": null}`。補助でない種目は 400 |
```

- [ ] **Step 9: 全体の検収**

Run: `gofmt -l . && go vet ./... && go test ./...`
Expected: 出力なし／PASS

- [ ] **Step 10: Commit と、組み立ての重複を issue に積む**

```bash
git add internal/application internal/presentation cmd/api/main.go README.md
git commit -m "feat(api): 補助の代わりを返す GET /api/sessions/substitute を足す"
gh issue create --title "GetSession・GetForecast・GetSubstitute が同じ PlanRequest の組み立てを3回書いている" --body "$(cat <<'EOF'
いま壊れていないこと：3つとも同じ読み取り（プログラム・週目標・種目・実績・コンディション）を同じ順で行っており、結果は一致している。

壊れる条件：PlanRequest に入力を1つ足したとき（例：新しいリポジトリから読む値）、3箇所のうち1つに足し忘れると、その口だけ別の計画を出す。forecast の回0と今日の計画、または代わりの導出が食い違う。

直すなら：usecase に非公開の `loadPlanRequest(ctx, readers, user, date) (planning.PlanRequest, error)` を置き、3つから呼ぶ。動作を変えない機械的な移動として単独の PR にする。
EOF
)"
```

PR 1 を出す（Task 1〜3）。本文に「Closes」は付けない。auto-merge を付ける（`gh pr merge --auto --merge`）。

---

### Task 4: 画面の判断（`substitute.ts`）

**Files:**
- Modify: `web/src/api/types.ts`
- Modify: `web/src/features/today/ExerciseCard.tsx`（`CardPlan` の型だけ）
- Create: `web/src/features/today/substitute.ts`
- Create: `web/src/features/today/substitute.test.ts`

**Interfaces:**
- Consumes: `GET /api/sessions/substitute`（Task 3）
- Produces:
  - `type SubstituteResponse = { substitute: PlannedSet | null }`（`types.ts`）
  - `CardPlan` に `substitute_for?: string`（元の種目 ID）と `no_substitute?: boolean`
  - `type Swap = { replace: string; shown: string[]; current: PlannedSet | null }`
  - `type SwapState = { date: string; swaps: ReadonlyMap<string, Swap> }`
  - `swapsFor(state: SwapState, date: string): ReadonlyMap<string, Swap>`
  - `rejectedOf(swaps: ReadonlyMap<string, Swap>, replace: string): string[]`
  - `withResult(state: SwapState, date: string, replace: string, result: PlannedSet | null): SwapState`
  - `accessoryCards(accessories: readonly PlannedSet[], swaps: ReadonlyMap<string, Swap>): CardPlan[]`
  - `canMarkBlocked(plan: CardPlan, recordedCount: number): boolean`
  - `substituteQuery(date: string, replace: string, rejected: readonly string[]): string`
  - `substituteErrorMessage(e: unknown): string`
  - `type StoredSwaps = { date: string; swaps: Swap[] }`
  - `encodeSwaps(state: SwapState): StoredSwaps`
  - `decodeSwaps(raw: unknown): SwapState`（形が合わなければ `{ date: '', swaps: new Map() }`）

- [ ] **Step 1: 型を足す**

`types.ts` の `ForecastResponse` の下に足す。

```ts
/** 補助の代わり。dto.go の substituteResponse と対。null は「代わりなし」。 */
export type SubstituteResponse = { substitute: PlannedSet | null };
```

`ExerciseCard.tsx` の `CardPlan` に足す。

```ts
  /** substitute_for は埋まっていた元の補助の ID。代わりのカードにだけ付く。 */
  substitute_for?: string;
  /** no_substitute は代わりを探したが無かった元の補助。 */
  no_substitute?: boolean;
```

- [ ] **Step 2: 失敗するテストを書く**

`substitute.test.ts` を新規に作る。

```ts
import { describe, expect, it } from 'vitest';
import type { PlannedSet } from '../../api/types';
import {
  accessoryCards,
  canMarkBlocked,
  decodeSwaps,
  encodeSwaps,
  rejectedOf,
  substituteErrorMessage,
  substituteQuery,
  swapsFor,
  withResult,
  type SwapState,
} from './substitute';

const set = (id: string): PlannedSet => ({
  exercise_id: id,
  weight_kg: null,
  sets: 3,
  target_rir: 2,
  target_reps: 10,
});
const empty = (date: string): SwapState => ({ date, swaps: new Map() });

describe('withResult / rejectedOf', () => {
  it('代わりの代わりは、元の replace のまま出した代わりを全部 rejected に積む', () => {
    let s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    expect(rejectedOf(s.swaps, 'leg_curl')).toEqual(['nordic']);
    s = withResult(s, '2026-10-11', 'leg_curl', set('ghr'));
    expect(rejectedOf(s.swaps, 'leg_curl')).toEqual(['nordic', 'ghr']);
    expect(s.swaps.get('leg_curl')?.current?.exercise_id).toBe('ghr');
  });

  it('代わりなしでも、それまでに出した代わりは残す', () => {
    let s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    s = withResult(s, '2026-10-11', 'leg_curl', null);
    expect(s.swaps.get('leg_curl')).toEqual({ replace: 'leg_curl', shown: ['nordic'], current: null });
  });

  it('まだ押していない種目の rejected は空', () => {
    expect(rejectedOf(new Map(), 'leg_curl')).toEqual([]);
  });
});

describe('swapsFor', () => {
  it('日付が変わったら差し替えは持ち越さない', () => {
    const s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    expect(swapsFor(s, '2026-10-11').size).toBe(1);
    expect(swapsFor(s, '2026-10-12').size).toBe(0);
  });

  it('別の日付の状態に結果を書くと、前の日の差し替えは捨てる', () => {
    const s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    const next = withResult(s, '2026-10-12', 'curl', set('hammer'));
    expect([...next.swaps.keys()]).toEqual(['curl']);
  });
});

describe('accessoryCards', () => {
  it('差し替えた補助はその場所に代わりのカードが出る', () => {
    const s = withResult(empty('d'), 'd', 'leg_curl', set('nordic'));
    const cards = accessoryCards([set('curl'), set('leg_curl'), set('incline')], s.swaps);
    expect(cards.map((c) => c.exercise_id)).toEqual(['curl', 'nordic', 'incline']);
    expect(cards[1].substitute_for).toBe('leg_curl');
    expect(cards[0].substitute_for).toBeUndefined();
  });

  it('代わりなしなら元のカードに no_substitute が付く', () => {
    const s = withResult(empty('d'), 'd', 'leg_curl', null);
    const cards = accessoryCards([set('leg_curl')], s.swaps);
    expect(cards[0]).toMatchObject({ exercise_id: 'leg_curl', no_substitute: true });
  });
});

describe('canMarkBlocked', () => {
  it('記録の無い補助・代わりには出す', () => {
    expect(canMarkBlocked(set('leg_curl'), 0)).toBe(true);
    expect(canMarkBlocked({ ...set('nordic'), substitute_for: 'leg_curl' }, 0)).toBe(true);
  });

  it('1セットでも記録したら出さない（代わりが今日すでにやった種目でも同じ）', () => {
    expect(canMarkBlocked(set('leg_curl'), 1)).toBe(false);
    expect(canMarkBlocked({ ...set('nordic'), substitute_for: 'leg_curl' }, 2)).toBe(false);
  });

  it('代わりなしと分かったカードには出さない', () => {
    expect(canMarkBlocked({ ...set('leg_curl'), no_substitute: true }, 0)).toBe(false);
  });
});

describe('substituteQuery', () => {
  it('表示している計画の日付と、繰り返しの rejected で組む', () => {
    expect(substituteQuery('2026-10-11', 'leg_curl', ['nordic', 'ghr'])).toBe(
      '/api/sessions/substitute?date=2026-10-11&replace=leg_curl&rejected=nordic&rejected=ghr',
    );
  });

  it('rejected が無ければ付けない', () => {
    expect(substituteQuery('2026-10-11', 'leg_curl', [])).toBe(
      '/api/sessions/substitute?date=2026-10-11&replace=leg_curl',
    );
  });
});

describe('encodeSwaps / decodeSwaps', () => {
  it('残したものを読み戻すと同じ差し替えになる', () => {
    let s = withResult(empty('2026-10-11'), '2026-10-11', 'leg_curl', set('nordic'));
    s = withResult(s, '2026-10-11', 'curl', null);
    const back = decodeSwaps(JSON.parse(JSON.stringify(encodeSwaps(s))));
    expect(back.date).toBe('2026-10-11');
    expect([...back.swaps.entries()]).toEqual([...s.swaps.entries()]);
  });

  it('壊れた値・形の違う値は空から始める', () => {
    for (const raw of [null, 'x', 42, {}, { date: 1, swaps: [] }, { date: 'd', swaps: [{ replace: 1 }] },
      { date: 'd', swaps: [{ replace: 'a', shown: ['b'], current: { exercise_id: 'b' } }] }]) {
      const s = decodeSwaps(raw);
      expect(s.swaps.size).toBe(0);
    }
  });
});

describe('substituteErrorMessage', () => {
  it('応答が返らなかったときだけオフラインと言う', () => {
    expect(substituteErrorMessage(new TypeError('Failed to fetch'))).toBe('オフラインでは出せません');
    expect(substituteErrorMessage(new Error('500'))).toBe('代わりを読み込めませんでした');
  });
});
```

- [ ] **Step 3: 走らせて、期待した理由で落ちることを確かめる**

Run: `cd web && pnpm vitest run src/features/today/substitute.test.ts`
Expected: FAIL `Failed to resolve import "./substitute"`

- [ ] **Step 4: 実装する**

`substitute.ts` を新規に作る。

```ts
import type { PlannedSet } from '../../api/types';
import type { CardPlan } from './ExerciseCard';

const OFFLINE_MESSAGE = 'オフラインでは出せません';
const COMMUNICATION_ERROR_MESSAGE = '代わりを読み込めませんでした';

/**
 * Swap は予定の補助1つぶんの差し替え。
 *
 * replace は予定に出ていた補助で、何度押してもサーバーへはこれを送る
 * （代わりの代わりを頼むときに、代わりを replace にすると予定に無いので 400）。
 * shown はこれまでに出した代わりで、次に押したとき rejected として送る。
 * current はいま出している代わり。null は「代わりなし」。
 */
export type Swap = { replace: string; shown: string[]; current: PlannedSet | null };

/** SwapState は差し替えと、それがどの日の計画に対するものか。 */
export type SwapState = { date: string; swaps: ReadonlyMap<string, Swap> };

/** swapsFor は date の計画に対する差し替え。別の日のものは持ち越さない。 */
export function swapsFor(state: SwapState, date: string): ReadonlyMap<string, Swap> {
  return state.date === date ? state.swaps : new Map();
}

/** rejectedOf は replace の代わりを次に頼むときに外す種目。 */
export function rejectedOf(swaps: ReadonlyMap<string, Swap>, replace: string): string[] {
  return [...(swaps.get(replace)?.shown ?? [])];
}

/** withResult はサーバーの答えを差し替えに反映した新しい状態を返す。 */
export function withResult(
  state: SwapState,
  date: string,
  replace: string,
  result: PlannedSet | null,
): SwapState {
  const swaps = new Map(swapsFor(state, date));
  const shown = rejectedOf(swaps, replace);
  if (result) shown.push(result.exercise_id);
  swaps.set(replace, { replace, shown, current: result });
  return { date, swaps };
}

/**
 * accessoryCards は補助のレーンに出すカード。差し替えた補助はその場所に
 * 代わりが入り、代わりなしと分かった補助には印が付く。
 *
 * ここで作った並びを今日の予定として leftovers・plannedIds に渡すこと。
 * 渡さないと、代わりを記録した時点で「予定外の記録」にも同じ種目が出る。
 */
export function accessoryCards(
  accessories: readonly PlannedSet[],
  swaps: ReadonlyMap<string, Swap>,
): CardPlan[] {
  return accessories.map((a) => {
    const swap = swaps.get(a.exercise_id);
    if (!swap) return a;
    if (swap.current === null) return { ...a, no_substitute: true };
    return { ...swap.current, substitute_for: a.exercise_id };
  });
}

/**
 * canMarkBlocked は「埋まっている」を出すか。補助のカードにだけ使う。
 *
 * 1セットでも記録したら出さない。途中までやったならそこでやめればよい
 * （今日のリストはキュー）。差し替えると同じ枠で2種目を数えることになる。
 */
export function canMarkBlocked(plan: CardPlan, recordedCount: number): boolean {
  return !plan.no_substitute && recordedCount === 0;
}

/**
 * substituteQuery は代わりを頼む URL。date は表示している計画の日付を渡す
 * （端末の「いま」を渡すと、日付をまたいだとき翌日の計画を引いて 400 になる）。
 */
export function substituteQuery(date: string, replace: string, rejected: readonly string[]): string {
  const q = new URLSearchParams({ date, replace });
  for (const id of rejected) q.append('rejected', id);
  return `/api/sessions/substitute?${q.toString()}`;
}

/** StoredSwaps は localStorage に置く形。Map は JSON にできないので配列にする。 */
export type StoredSwaps = { date: string; swaps: Swap[] };

/** encodeSwaps は差し替えを localStorage に置ける形にする。 */
export function encodeSwaps(state: SwapState): StoredSwaps {
  return { date: state.date, swaps: [...state.swaps.values()] };
}

const isString = (v: unknown): v is string => typeof v === 'string';
const isNumber = (v: unknown): v is number => typeof v === 'number' && Number.isFinite(v);

function isPlannedSet(v: unknown): v is PlannedSet {
  if (typeof v !== 'object' || v === null) return false;
  const p = v as Record<string, unknown>;
  return (
    isString(p.exercise_id) &&
    (p.weight_kg === null || isNumber(p.weight_kg)) &&
    isNumber(p.sets) &&
    isNumber(p.target_rir) &&
    isNumber(p.target_reps)
  );
}

function isSwap(v: unknown): v is Swap {
  if (typeof v !== 'object' || v === null) return false;
  const s = v as Record<string, unknown>;
  return (
    isString(s.replace) &&
    Array.isArray(s.shown) &&
    s.shown.every(isString) &&
    (s.current === null || isPlannedSet(s.current))
  );
}

/**
 * decodeSwaps は localStorage から読んだ値を差し替えに戻す。
 *
 * 形が1か所でも合わなければ全部捨てて空から始める。古い版が残した値や
 * 手で壊れた値で画面を止めない。捨てても失うのは差し替えだけで、記録は
 * 残る（「予定外の記録」に出る）。
 */
export function decodeSwaps(raw: unknown): SwapState {
  const none: SwapState = { date: '', swaps: new Map() };
  if (typeof raw !== 'object' || raw === null) return none;
  const r = raw as Record<string, unknown>;
  if (!isString(r.date) || !Array.isArray(r.swaps) || !r.swaps.every(isSwap)) return none;
  return { date: r.date, swaps: new Map(r.swaps.map((s: Swap) => [s.replace, s])) };
}

/** substituteErrorMessage は取得に失敗したときの一言（forecastErrorMessage と同じ分け方）。 */
export function substituteErrorMessage(e: unknown): string {
  return e instanceof TypeError ? OFFLINE_MESSAGE : COMMUNICATION_ERROR_MESSAGE;
}
```

- [ ] **Step 5: 走らせて緑を確かめる**

Run: `cd web && pnpm vitest run src/features/today/substitute.test.ts`
Expected: PASS

- [ ] **Step 6: 変異を入れて赤くなることを確かめる**

- `withResult` の `const shown = rejectedOf(swaps, replace)` を `const shown: string[] = []` にする → 「代わりの代わり」「代わりなしでも残す」が赤
- `swapsFor` を `return state.swaps` にする → 「日付が変わったら持ち越さない」が赤
- `canMarkBlocked` の `recordedCount === 0` を `true` にする → 「1セットでも記録したら出さない」が赤
- `substituteQuery` で `q.append` を `q.set` にする → 繰り返しの rejected のテストが赤
- `accessoryCards` で `substitute_for` を付けない → 「その場所に代わり」が赤
- `isPlannedSet` の `isNumber(p.sets)` を外す → 「壊れた値は空から始める」の最後の値が通って赤

- [ ] **Step 7: 検収と Commit**

Run: `cd web && pnpm typecheck && pnpm test && pnpm format:check`
Expected: PASS

```bash
git add web/src/api/types.ts web/src/features/today/ExerciseCard.tsx web/src/features/today/substitute.ts web/src/features/today/substitute.test.ts
git commit -m "feat(web): 補助の差し替えの判断（substitute.ts）を足す"
```

---

### Task 5: 手順・描画・配線の検査

**Files:**
- Create: `web/src/features/today/useSubstitute.ts`
- Modify: `web/src/features/today/ExerciseCard.tsx`
- Modify: `web/src/features/today/Today.tsx`
- Create: `web/scripts/substitute-check.mjs`
- Modify: `docs/user-stories.md`

**Interfaces:**
- Consumes: Task 4 の全関数・型、`getJSON`（`web/src/api/client.ts`）
- Produces: `useSubstitute(date: string | undefined): { swaps: ReadonlyMap<string, Swap>; pending: string | null; errors: ReadonlyMap<string, string>; markBlocked: (replace: string) => Promise<void> }`

- [ ] **Step 1: フックを書く**

`useSubstitute.ts` を新規に作る。

```ts
import { useCallback, useEffect, useRef, useState } from 'react';
import { getJSON } from '../../api/client';
import type { SubstituteResponse } from '../../api/types';
import { KEY_SUBSTITUTES, readJSON, writeJSON } from '../../storage/local';
import {
  decodeSwaps,
  encodeSwaps,
  rejectedOf,
  substituteErrorMessage,
  substituteQuery,
  swapsFor,
  withResult,
  type Swap,
  type SwapState,
} from './substitute';

export type Substitute = {
  /** swaps は表示している計画に対する差し替え。 */
  swaps: ReadonlyMap<string, Swap>;
  /** pending は取得中の補助（replace）。取得中はほかの押下を受けない。 */
  pending: string | null;
  /** errors は補助ごとの失敗の一言。 */
  errors: ReadonlyMap<string, string>;
  markBlocked: (replace: string) => Promise<void>;
};

/**
 * useSubstitute は「埋まっている」を押したときの取得を持つ。
 *
 * 差し替えは localStorage に計画の日付つきで残す（設計書）。スマホは勝手に
 * 閉じるので、残さないと代わりの重量と目標セット数が開き直した時点で消える。
 * 別の日付のものは swapsFor が読まない。取得そのものは PWA のキャッシュにも
 * Outbox にも載せない。オフラインでは出さない（古い答えを見せない）。
 *
 * date は表示している計画の日付（Session.date）。端末の今日を使わない。
 */
export function useSubstitute(date: string | undefined): Substitute {
  const [state, setState] = useState<SwapState>(() => decodeSwaps(readJSON(KEY_SUBSTITUTES)));
  useEffect(() => writeJSON(KEY_SUBSTITUTES, encodeSwaps(state)), [state]);
  const [pending, setPending] = useState<string | null>(null);
  const [errors, setErrors] = useState<ReadonlyMap<string, string>>(new Map());
  // 連打の判定は ref で行う。state だと、押した直後の再描画より前の
  // 2回目の押下が古い pending（null）を読んで2回飛ぶ。
  const inflight = useRef(false);

  const swaps = date ? swapsFor(state, date) : new Map<string, Swap>();

  const markBlocked = useCallback(
    async (replace: string) => {
      if (!date || inflight.current) return;
      inflight.current = true;
      setPending(replace);
      setErrors((m) => {
        const next = new Map(m);
        next.delete(replace);
        return next;
      });
      try {
        const res = await getJSON<SubstituteResponse>(
          substituteQuery(date, replace, rejectedOf(swapsFor(state, date), replace)),
        );
        setState((s) => withResult(s, date, replace, res.substitute));
      } catch (e) {
        setErrors((m) => new Map(m).set(replace, substituteErrorMessage(e)));
      } finally {
        inflight.current = false;
        setPending(null);
      }
    },
    [date, state],
  );

  return { swaps, pending, errors, markBlocked };
}
```

`web/src/storage/local.ts` の末尾に足す。

```ts
// --- 補助の差し替え ---
//
// どの補助を何に差し替えたか。計画の日付つきで置き、別の日付のものは読まない
// （features/today/substitute.ts の swapsFor）。失っても記録は消えない。
export const KEY_SUBSTITUTES = 'liftplan.substitutes';
```

- [ ] **Step 2: カードに「埋まっている」と注記を足す**

`ExerciseCard.tsx` の `Props` に足す。

```ts
  /** replacingName は元の補助の名前。代わりのカードにだけ渡す。 */
  replacingName?: string;
  /** onBlocked を渡したカードにだけ「埋まっている」を出す。 */
  onBlocked?: () => void;
  /** blockedBusy は代わりを取得中。 */
  blockedBusy?: boolean;
  /** note はカードの下に出す一言（代わりなし・取得の失敗）。 */
  note?: string;
```

名前の行を次に置き換える（`Button` を `../../ui/Button` から import する）。

```tsx
      <div className="flex items-center gap-2.5">
        <div className="grid">
          <span className="text-[17px] font-bold">{name}</span>
          {replacingName && <span className="text-xs text-faint">{replacingName} の代わり</span>}
        </div>
        {onBlocked && (
          <Button
            variant="quiet"
            size="chip"
            className="ml-auto shrink-0"
            disabled={blockedBusy}
            onClick={onBlocked}
          >
            {blockedBusy ? '探しています' : '埋まっている'}
          </Button>
        )}
      </div>
```

枠のボタン列（`grid auto-cols-fr`）の直後に足す。

```tsx
      {note && <Note>{note}</Note>}
```

注：`adhoc-check.mjs` などはカード名を `span.font-bold.text-\[17px\]` で拾っている。名前の `span` のクラスは変えないこと。

- [ ] **Step 3: 今日の画面に配線する**

`Today.tsx`：

```tsx
import { accessoryCards, canMarkBlocked } from './substitute';
import { useSubstitute } from './useSubstitute';
```

`const accessories = ...` の行を次に置き換える。

```tsx
  // 補助は差し替えを反映した並びを予定として扱う（leftovers・plannedIds に
  // この並びを渡す。accessoryCards のコメント）。
  const substitute = useSubstitute(data.session?.date);
  const accessories = accessoryCards(data.session?.accessories ?? [], substitute.swaps);
```

補助のカードだけ別の描画にする。`card` の下に足す。

```tsx
  const accessoryCard = (plan: CardPlan) => {
    // 差し替えの鍵は常に元の補助（サーバーへ送る replace）。
    const replace = plan.substitute_for ?? plan.exercise_id;
    const recorded = doneOf(plan.exercise_id);
    return (
      <ExerciseCard
        key={`${plan.exercise_id}-plan`}
        plan={plan}
        name={nameOf(plan.exercise_id)}
        last={data.last[plan.exercise_id]}
        recorded={recorded}
        onOpen={(index, rec) => sheet.open({ plan, index, recorded: rec })}
        replacingName={plan.substitute_for ? nameOf(plan.substitute_for) : undefined}
        onBlocked={canMarkBlocked(plan, recorded.length) ? () => void substitute.markBlocked(replace) : undefined}
        blockedBusy={substitute.pending === replace}
        note={
          substitute.errors.get(replace) ?? (plan.no_substitute ? '代わりなし。飛ばしてよい' : undefined)
        }
      />
    );
  };
```

`{accessories.map(card)}` を `{accessories.map(accessoryCard)}` に変える。`lanes` は `[main, variation, accessories]` のまま（`accessories` が差し替え後の並びになっている）。

- [ ] **Step 4: 型と単体テストを通す**

Run: `cd web && pnpm typecheck && pnpm test && pnpm format:check`
Expected: PASS

- [ ] **Step 5: 配線の検査を書く**

`web/scripts/substitute-check.mjs` を新規に作る。冒頭（起動・トークン・`todaysSets` で今日の記録を消す・ブラウザ起動・`page.goto`）は `adhoc-check.mjs` の 1〜58 行目をそのまま写し、冒頭のコメントだけ次に変える。

```js
// 補助の「埋まっている」を実機で確かめる。
//
// 単体テストでは踏めないところを見る。押すとその場でカードが代わりに
// 差し替わること、もう一度押すと次の候補か「代わりなし」になること、
// 記録したカードからボタンが消えること、開き直しても代わりが残ること、
// 連打しても1回しか取りに行かないこと。判断（何を送るか・どのカードを
// 出すか）は features/today/substitute.ts の単体テストが守っているので、
// ここは配線を見る。
//
// 使い方は scripts/ui-check.mjs と同じ。ポートは APP= と API= で渡す。
```

その下に検査を書く。

```js
const body = () => page.innerText('body');
const cardNames = () => page.locator('span.font-bold.text-\\[17px\\]').allInnerTexts();
const blocked = () => page.getByRole('button', { name: '埋まっている' });

// 補助の見出しの下にあるカードだけに出る。
const nBlocked = await blocked().count();
check('補助のカードに「埋まっている」が出る', nBlocked > 0, `${nBlocked}件`);

// 連打しても1回しか取りに行かない。
const requests = [];
page.on('request', (r) => {
  if (r.url().includes('/api/sessions/substitute')) requests.push(r.url());
});

const before = await cardNames();
const firstBlocked = blocked().first();
await firstBlocked.click();
await firstBlocked.click({ force: true }).catch(() => {});
await page.waitForTimeout(1200);
check('連打しても取得は1回', requests.length === 1, requests.join(' | '));

const after = await cardNames();
const replaced = before.find((n) => !after.includes(n));
const substituteName = after.find((n) => !before.includes(n));
const text = await body();
if (substituteName) {
  check('カードが代わりに差し替わる', !!replaced, `${replaced} → ${substituteName}`);
  check('何の代わりかが出る', text.includes(`${replaced} の代わり`));
  check('送った日付は表示中の計画の日付', requests[0].includes(`date=${day()}`), requests[0]);

  // もう一度押す：rejected に1つ目が積まれ、別の候補か「代わりなし」になる。
  const card = page.locator('div.grid.gap-3', { has: page.locator(`span.font-bold:text-is("${substituteName}")`) });
  await card.getByRole('button', { name: '埋まっている' }).click();
  await page.waitForTimeout(1200);
  check('2回目は rejected を付けて頼む', (requests[1] ?? '').includes('rejected='), requests[1] ?? '');
  const third = await cardNames();
  check(
    '2回目は別の候補か「代わりなし」',
    !third.includes(substituteName) || (await body()).includes('代わりなし。飛ばしてよい'),
    third.join(','),
  );
} else {
  check('代わりが無ければ「代わりなし」と出る', text.includes('代わりなし。飛ばしてよい'));
}

// 記録したカードからはボタンが消える。
const target = (await cardNames()).find((n) => !before.slice(0, 1).includes(n) && after.includes(n)) ?? after[after.length - 1];
const slotsOf = (name) =>
  page.locator('div.grid.gap-3', { has: page.locator(`span.font-bold:text-is("${name}")`) }).locator('button.set');
const blockedOf = (name) =>
  page
    .locator('div.grid.gap-3', { has: page.locator(`span.font-bold:text-is("${name}")`) })
    .getByRole('button', { name: '埋まっている' });
if ((await blockedOf(target).count()) === 1) {
  await slotsOf(target).nth(0).click();
  await page.waitForTimeout(400);
  await page.fill('dialog input[inputmode=decimal]', '20');
  await page.fill('dialog input[inputmode=numeric] >> nth=0', '10');
  await page.click('dialog >> text=記録する');
  await page.waitForTimeout(900);
  check('記録したカードには「埋まっている」が出ない', (await blockedOf(target).count()) === 0, target);
}

// 開き直しても差し替えは残る（localStorage）。
const beforeReload = await cardNames();
await page.reload();
await page.waitForTimeout(2500);
const reloaded = await cardNames();
check('開き直しても代わりのカードが残る', JSON.stringify(reloaded) === JSON.stringify(beforeReload), reloaded.join(','));
check('開き直しても元の補助は出ない', !replaced || !reloaded.includes(replaced), reloaded.join(','));

// 片付け：次の実行に差し替えを持ち越さない。
await page.evaluate(() => localStorage.removeItem('liftplan.substitutes'));

check('画面のエラーが無い', errs.length === 0, errs.join(' | '));
await browser.close();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} 通過`);
process.exit(failed.length ? 1 : 0);
```

（末尾の集計と終了の書き方は `adhoc-check.mjs` の末尾に揃える。違っていればそちらに合わせる。）

- [ ] **Step 6: 配線の検査を回す**

`ui-check.mjs` 冒頭の手順でサーバー（インメモリ）と画面（preview）を立てる。playwright-core は scratchpad のもの（メモリ「docker 無しでの DB・画面検査」）を `PLAYWRIGHT=` で渡す。

Run: `cd web && PLAYWRIGHT=<scratchpad>/node_modules/playwright-core/index.mjs node scripts/substitute-check.mjs`
Expected: 全項目 ✓

続けて、画面を触ったので既存の検査も回す：`adhoc-check.mjs`、`ui-check.mjs`、`a11y-check.mjs`。
Expected: 全項目 ✓（#278 の nav-check・settings-check は既知の失敗なので対象外）

- [ ] **Step 7: 変異を入れて配線の検査が赤くなることを確かめる**

- `useSubstitute` の `inflight.current` の判定を外す → 「連打しても取得は1回」が赤
- `Today.tsx` で `useSubstitute(data.session?.date)` を `useSubstitute(today())` にして端末の日付をずらす（`TZ=Pacific/Kiritimati` でブラウザを起動する）→ 「送った日付は表示中の計画の日付」が赤。手間が大きければこの変異は省き、単体テスト（`substituteQuery`）が守っていることを PR に書く
- `accessoryCard` の `onBlocked` で `canMarkBlocked` を通さず常に渡す → 「記録したカードには出ない」が赤
- `useSubstitute` の `writeJSON` の `useEffect` を消す → 「開き直しても代わりのカードが残る」が赤

- [ ] **Step 8: ユーザーストーリーを更新する**

`docs/user-stories.md` の B6 の行を次に置き換える。

```markdown
| B6 | 器具が埋まっていたら順番を飛ばせる。補助なら代わりの種目が重量まで付いて出る | 画面（順序は表示だけ）・`GET /api/sessions/substitute` |
```

B8 の段落の末尾に1文足す。

```markdown
補助の器具が埋まっているだけなら、B8 ではなくカードの「埋まっている」を使う。代わりの種目と重量をアプリが決める（`docs/specs/2026-10-11-accessory-substitute-design.md`）。
```

- [ ] **Step 9: 検収と Commit**

Run: `cd web && pnpm typecheck && pnpm test && pnpm format:check`
Expected: PASS

```bash
git add web/src/features/today web/src/storage/local.ts web/scripts/substitute-check.mjs docs/user-stories.md
git commit -m "feat(web): 補助のカードに「埋まっている」を置き、代わりの種目に差し替える"
```

spec の `status` を `実装済み（PR 1 ドメイン・API・PR 2 画面）` に直して同じ PR に含める。PR 2 を出し、auto-merge を付ける。
