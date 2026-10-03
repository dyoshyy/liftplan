# 伸ばしたい種目ごとのレップ数 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 宣言種目（伸ばしたい種目）ごとに、軸で出すときの「重い番」「軽い番」のレップ数を本人が決められるようにする。あわせて、重点種目の一巡で派生が軸に立つ日を「軽い番・上乗せなし」にする。

**Architecture:** 値は `program` 集約が宣言ごとに `RepTargets{heavy, light}` として持つ（既定 3・6）。計画の導出（`planning`）は役割（`laneRole`）までを決め、軸の役割の強度だけを Epley の逆算 `round2(1/(1+(レップ+1)/30))` で出す。派生が軸に立つ日には新しい役割 `focusVariationRole` を与え、上乗せ（overload）の対象から外す。保存は `program.declared_reps jsonb`、API は `PUT /api/program/declared/{id}/reps`、画面は設定の「種目」節。

**Tech Stack:** Go 1.2x（標準 `net/http`、pgx、embedded-postgres でのテスト）、React + TypeScript + vitest、playwright-core（`web/scripts/*-check.mjs`）

**Spec:** `docs/specs/2026-10-03-declared-rep-targets-design.md`

## Global Constraints

- レップ数の範囲は **1〜15**。既定は **重い番 3・軽い番 6**
- 軸の RIR は **1**。強度は `math.Round(100/(1+float64(reps+1)/30))/100`（3 → 0.88、6 → 0.81 で今の表と一致すること）
- バリエーション（0.80・RIR2・6レップ）と補助（0.71・RIR2・10レップ）の定数は変えない
- N ≤ M は強制しない
- 宣言から外した種目の値は捨てる。入れ直したら既定に戻る
- 派生が軸に立つ日は、重点種目の M で出して上乗せを掛けない
- ドメイン層は外側に依存しない。値オブジェクトは `struct` で包む。エラーは `%w` で包む。doc コメントは宣言名で始める（CLAUDE.md）
- PR は3本。**1本に1つの判断**。動作の変更と機械的な移動を混ぜない
  - PR 1（Task 1）：派生の日を軽い版にする
  - PR 2（Task 2〜5）：宣言ごとのレップ数（ドメイン・保存・API）。既定では計画の数字が1つも動かない
  - PR 3（Task 6〜7）：設定画面と devsim
- テストは CLAUDE.md の5段階で書く。**変異は手で入れて赤くなることを確かめる**（`.claude/skills/writing-tests/` を先に読む）
- 検収コマンド：`gofmt -l .`、`go vet ./...`、`go test ./...`。web は `cd web && npm test` と型検査（`npm run build` か `npx tsc -b`）
- コミット末尾：`Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`。PR 本文末尾：`🤖 Generated with [Claude Code](https://claude.com/claude-code)`
- 積んだ PR のマージ順に注意（メモリ「積み重ねた PR のマージ順」）。PR 2 は PR 1 の上に、PR 3 は PR 2 の上に積む

## Review Focus

1. **別の設定を変えたらレップ数が消える** — `params()` への載せ忘れ。`WithFrequency` などで値が残ることを Task 2 の `TestProgram_WithKeepsOtherFields` に `reps` を足して守る
2. **既存の DB（列が無い版・NULL の行）が読めない** — Task 3 で旧版のスキーマに行を作ってから新版を当てる手順を踏み、NULL を既定として読むテストを置く
3. **重点を切り替えると派生の日の M が別の種目のものになる** — 派生の日は「重点種目の M」。Task 4 のテストで、派生の M が派生自身ではなく重点種目の値であることを見る
4. **宣言を外して入れ直したら古い値が戻る** — 仕様は「既定に戻る」。Task 2 で `WithDeclared` が落とすこと、Task 6 で画面が取り直した値を出すことを見る
5. **今日の計画が一日の途中で動く** — レップ数は `Program` から引くだけで履歴を見ないので動かないはずだが、`TestSessionPlanner_PlanIsFixedForTheWholeDay` を各 PR の検収に入れる

---

## File Structure

| ファイル | 責務 | Task |
|---|---|---|
| `internal/domain/training/planning/lane_prescription.go` | 役割の定義、役割 → 処方、上乗せ | 1, 4 |
| `internal/domain/training/planning/axis_rotation.go` | 軸と役割を選ぶ（派生の日の役割） | 1 |
| `internal/domain/training/planning/session_planner.go` | lineup に「その軸のレップ数」を載せる | 4 |
| `internal/domain/training/planning/horizon_projector.go` | 先の回のセット数（レップ数は関係しない） | 4 |
| `internal/domain/training/program/rep_targets.go`（新規） | 値オブジェクト `RepTargets` | 2 |
| `internal/domain/training/program/program.go` | 宣言ごとの `RepTargets` を持つ | 2 |
| `internal/infrastructure/postgres/migrations/0016_add_declared_reps_on_program.sql`（新規） | 列を足す | 3 |
| `internal/infrastructure/postgres/program_repository.go` | 列の読み書き | 3 |
| `internal/application/usecase/set_declared_rep_targets.go`（新規） | 1宣言のレップ数だけを差し替える | 5 |
| `internal/presentation/httpapi/{handler.go,dto.go,router.go}` | 口と DTO | 5 |
| `cmd/api/main.go` | 配線 | 5 |
| `web/src/features/settings/reps.ts`（新規） | 判断（選択肢・本文） | 6 |
| `web/src/features/settings/useProgramSettings.ts` | 手順（送る・取り直す） | 6 |
| `web/src/features/settings/ProgramSettings.tsx` | 描画 | 6 |
| `web/scripts/settings-check.mjs` | 配線の検査 | 6 |
| `internal/application/devsim/simulator.go`、`internal/presentation/httpapi/dev_simulation.go`、`web/src/dev/{simulate.ts,DevSimulation.tsx}` | devsim の設定 | 7 |

---

## PR 1：派生の日を軽い版にする

ブランチ：`feat/derived-axis-light`（main から）

### Task 1: 派生が軸に立つ日を `focusVariationRole`（0.81・6レップ・RIR1・上乗せなし）にする

**Files:**
- Modify: `internal/domain/training/planning/lane_prescription.go:11-27`（役割）, `:86-100`（`prescriptionFor`）, `:136-147`（`prescribe` の振り分け）, `:209-266`（`overload` のコメント）
- Modify: `internal/domain/training/planning/axis_rotation.go:10-60`（`case 2` の戻り値とコメント）
- Modify: `internal/domain/training/planning/horizon_projector.go:40-52`（コメントの役割の列挙）
- Test: `internal/domain/training/planning/axis_rotation_test.go`, `lane_prescription_test.go`, `lane_prescription_internal_test.go`

**Interfaces:**
- Produces: `laneRole` に `focusVariationRole` を追加。`prescriptionFor(focusVariationRole, sets)` は `{intensityPct: 0.81, sets, targetRIR: 1, targetReps: 6}`。`axis()` の一巡3番目は `(派生, focusVariationRole)` を返す

- [ ] **Step 1: 失敗するテストを書く（一巡の3番目の強度とレップ数）**

`axis_rotation_test.go` の `TestSessionPlanner_FocusAxisRotates` のケースを直し、目標レップ数も見るようにする。

```go
	cases := []struct {
		name string
		// sessions は系統が軸に来た回数（履歴に積むセッション数）。
		sessions  int
		want      exercise.ExerciseID
		intensity float64
		reps      int
	}{
		{name: "1周目は3レップ相当", sessions: 3, want: "bench", intensity: 0.88, reps: 3},
		{name: "2周目は6レップ相当", sessions: 4, want: "bench", intensity: 0.81, reps: 6},
		// 派生の目的はフォームの向上で、重くする必要が無い。軽い番で出す。
		{name: "3周目は派生を軽い番で", sessions: 5, want: "tempo", intensity: 0.81, reps: 6},
		{name: "4周目で本体に戻る", sessions: 6, want: "bench", intensity: 0.88, reps: 3},
	}
```

ループ本体の `assertIntensity(t, req, set, c.intensity)` の後に足す。

```go
			if got := set.TargetReps().Int(); got != c.reps {
				t.Errorf("目標レップが %d。%d のはず", got, c.reps)
			}
```

- [ ] **Step 2: 失敗するテストを書く（派生の日には上乗せしない）**

`lane_prescription_test.go` の末尾に足す。

```go
// 派生が一巡の3番目で軸に立つ日は、上乗せの条件を満たしても刻みを乗せないこと。
//
// 派生の目的はフォームの向上で、重くする必要が無い（設計書
// 2026-10-03-declared-rep-targets「決めたこと」）。
//
// tempo の起点は 75kg×8 RIR2（Epley 100kg）。0.81 で 81 → 80kg。
// 以後 80kg×6 RIR1（Epley 98.67kg）を3回。EWMA で 100 → 99.6 → 99.32 →
// 99.12 と下がるが、どの日の始まりの推定 × 0.81 も 80kg に丸まる。窓の
// 3セッションが平坦で目標 RIR を割っていないので、上乗せが掛かれば 82.5kg。
//
// 系統（bench・tempo）のセッションは5回で、一巡の位置は 5 % 3 = 2（派生の番）。
func TestSessionPlanner_DerivativeAxisIsNotOverloaded(t *testing.T) {
	req := planRequest(t)
	req.Program, req.Target = rotationProgramWithout(t, "larsen")
	req.History = setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "bench", planMonday.AddDays(-40), "bench", 85, 8, 0),
		mkLogOn(t, "tempo-base", planMonday.AddDays(-35), "tempo", 75, 8, 2),
		mkLogOn(t, "tempo-1", planMonday.AddDays(-21), "tempo", 80, 6, 1),
		mkLogOn(t, "tempo-2", planMonday.AddDays(-14), "tempo", 80, 6, 1),
		mkLogOn(t, "tempo-3", planMonday.AddDays(-7), "tempo", 80, 6, 1),
	})

	s := mustPlan(t, req)
	if len(s.Main()) != 1 || s.Main()[0].ExerciseID() != "tempo" {
		t.Fatalf("前提: 今日は tempo が派生の番で軸に立つこと: %v", s.Main())
	}
	got, ok := plannedWeight(t, s, "tempo")
	if !ok {
		t.Fatal("前提: tempo の重量が出ること")
	}
	if got != 80 {
		t.Errorf("tempo が %vkg。80kg のはず（派生の日に上乗せが掛かっている）", got)
	}
}
```

- [ ] **Step 3: `TestSessionPlanner_AxisOverload_ExcludesVariationDays` を「宣言した派生」の形に書き換える**

PR 1 のあと、派生の日は上乗せされないので、今のテスト（tempo が派生の番で軸に立つ日の上乗せ）は前提ごと消える。ただし `performedAtLeast` で軽い日を窓から外す守りは、**派生を宣言にも入れた場合**に残る。tempo を宣言すると、tempo は「宣言の軸（0.88・上乗せあり）」の日と「重点ベンチの派生の番（0.81・上乗せなし）」の日を行き来する。後者を窓の証拠に使ってはいけない。

`mixedWindowProgram` を次に置き換える。

```go
// declaredDerivativeProgram は宣言がベンチと tempo、重点がベンチのプログラム。
//
// tempo はベンチの派生で、宣言にも入っている。宣言の軸（heavyRole・0.88・
// 上乗せあり）に立つ日と、重点ベンチの一巡の3番目（focusVariationRole・
// 0.81・上乗せなし）に立つ日を行き来する。履歴は役割を持たないので、
// どちらの日も tempo の記録として同じに並ぶ。
func declaredDerivativeProgram(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "incline", "curl", "tempo"},
		[]exercise.ExerciseID{"bench", "tempo"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p, target
}
```

テストを次に置き換える（名前も変える）。

```go
// 宣言した派生（tempo）が軸の重い日に立つとき、上乗せの判定が軽い日
// （重点ベンチの派生の番で 0.81 で出た日）を証拠に使わないこと。
//
// 軽い日の記録 RIR（1）は軸の目標 RIR（1）を割っておらず、「推定が平坦」の
// 判定もその日の始まりの推定を今日の役割の強度で引き直すだけなので、
// 軽い重量でやったことと無関係に成立する。窓の3セッションのうち重い処方で
// やったのは1日（9日前）だけなのに、守りが無いと上乗せが発火して 90kg になる。
//
// 数値（実測で確かめること）：起点 75×8 RIR2 で Epley 100kg。
//   -13日 軽い日 80×6 RIR1（Epley 98.67）→ 推定 99.6
//   -9日  重い日 87.5×3 RIR1（Epley 99.17）→ 推定 99.47
//   -6日・-3日 軽い日 80×6 RIR1 → 推定 99.23 → 99.06
// 今日の base は 0.88 × 99.06 = 87.17 → 87.5kg。
//
// 今日 tempo が宣言の軸に立つのは、ベンチを2日前にやって tempo（最後が3日前）を
// 最も古い宣言にしているため。2日前は回復の門（開区間 (date-2, date)）の外。
func TestSessionPlanner_AxisOverload_ExcludesLighterDays(t *testing.T) {
	req := planRequest(t)
	req.Program, req.Target = declaredDerivativeProgram(t)

	logs := []*setlog.SetLog{
		mkLogOn(t, "bench-recent", planMonday.AddDays(-2), "bench", 85, 8, 2),
	}
	for _, s := range []struct {
		name      string
		daysAgo   int
		kg        float64
		reps, rir int
	}{
		{"base", 20, 75, 8, 2},
		{"light13", 13, 80, 6, 1},
		{"heavy9", 9, 87.5, 3, 1},
		{"light6", 6, 80, 6, 1},
		{"light3", 3, 80, 6, 1},
	} {
		for k := range 3 {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("%s-%d", s.name, k),
				planMonday.AddDays(-s.daysAgo), "tempo", s.kg, s.reps, s.rir))
		}
	}
	req.History = setlog.NewHistory(logs)

	s := mustPlan(t, req)
	if len(s.Main()) != 1 || s.Main()[0].ExerciseID() != "tempo" {
		t.Fatalf("前提: 今日は tempo が宣言の軸に立つこと: %v", s.Main())
	}
	if reps := s.Main()[0].TargetReps().Int(); reps != 3 {
		t.Fatalf("前提: 今日の tempo は重い番（3レップ）であること: %d", reps)
	}

	got, ok := plannedWeight(t, s, "tempo")
	if !ok {
		t.Fatal("前提: tempo の重量が出ること")
	}
	const want = 87.5 // 軽い日を除けば、重い処方でやったのは1日だけで窓が満たない
	if got != want {
		t.Errorf("tempo が %vkg。%vkg のはず（軽い日を上乗せの証拠に使っている）", got, want)
	}
}
```

**注意：**「前提」の `t.Fatalf` で落ちたら、数値ではなく履歴の組み方（一巡の位置・回復の門）が想定とずれている。`axis_rotation.go` の `focusRested`・`heavyLift` を読んで日付を合わせ、コメントの数値も実測に合わせて直す。

- [ ] **Step 4: `TestLanePrescriptions_RoundTripIsCurrentlyContractive` に役割を足す**

`lane_prescription_internal_test.go` の `lanes` に1行足す。

```go
		{"重点種目の派生の番", focusVariationRole},
```

- [ ] **Step 5: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/domain/training/planning/ -run 'TestSessionPlanner_FocusAxisRotates|TestSessionPlanner_DerivativeAxisIsNotOverloaded|TestSessionPlanner_AxisOverload_ExcludesLighterDays|TestLanePrescriptions' -v`

Expected:
- `FocusAxisRotates/3周目は派生を軽い番で` が FAIL（重量が 0.88 相当、目標レップ 3）
- `DerivativeAxisIsNotOverloaded` が FAIL（82.5kg ではなく 0.88 相当の重量。役割がまだ heavyRole）
- `Lanes` はコンパイルエラー（`focusVariationRole` が未定義）。これは期待どおり
- `ExcludesLighterDays` は、今の実装だと派生の番の日が 0.88 で処方されるため、前提の組み方次第で通ることもある。通っても構わない（実装後に緑のままであることと、Step 9 の変異で赤くなることを見る）

- [ ] **Step 6: 役割を足す（`lane_prescription.go`）**

```go
const (
	// heavyRole は軸。3レップ相当で、3レーンで最も重い。
	heavyRole laneRole = iota
	// focusVolumeRole は重点種目の一巡の2番目。同じ軸を6レップ相当で出す。
	focusVolumeRole
	// focusVariationRole は重点種目の一巡の3番目で、派生が軸に立つ日。
	// 派生の目的はフォームの向上なので、軽い番で出し、上乗せは掛けない。
	focusVariationRole
	// variationRole はバリエーションレーン。軸より軽く、補助より重い。
	variationRole
	// accessoryRole は補助。RIR2 で10レップ前後を狙う位置。
	accessoryRole
)
```

`prescriptionFor` の switch に足す。

```go
	case focusVolumeRole, focusVariationRole:
		return lanePrescription{intensityPct: 0.81, sets: sets, targetRIR: 1, targetReps: 6}
```

（既存の `case focusVolumeRole:` の行をこの形に置き換える。）

`prescribe` の振り分けを直す。

```go
		switch entry.role {
		case heavyRole, focusVolumeRole, focusVariationRole:
			session.main = append(session.main, set)
```

`prescribeSet` の上乗せの条件（`if role == heavyRole || role == focusVolumeRole`）は**変えない**。`focusVariationRole` はここに入らないので上乗せされない。条件の直前にコメントを足す。

```go
			// 派生が軸に立つ日（focusVariationRole）には掛けない。派生の目的は
			// フォームの向上で、重くする必要が無い。
```

- [ ] **Step 7: 一巡の3番目の役割を変える（`axis_rotation.go`）**

```go
		if d := stalest(history, candidates); d != nil {
			return d, focusVariationRole
		}
```

`axis` の doc コメント1行目を `// axis は今日の軸と、その役割（heavyRole・focusVolumeRole・focusVariationRole）を返す。` に直す。`horizon_projector.go:40` と `:47-51` のコメントの役割の列挙にも `focusVariationRole` を足す。

- [ ] **Step 8: `overload` のコメントを書き直す（`lane_prescription.go:240-266`）**

「対象は heavyRole と focusVolumeRole」の段落を次に置き換える。

```go
// 対象は heavyRole と focusVolumeRole（prescribeSet の呼び分け）。派生が
// 一巡の3番目で軸に立つ日（focusVariationRole）は対象外。派生の目的は
// フォームの向上で、重くする必要が無い。
//
// 以前はここに「focusVolumeRole（0.81）に派生が立つ経路ができると、
// variationRole の 0.80 と同じ刻みに丸まってすり抜ける」という警告があった。
// 派生の軸の日は上乗せの判定に入らなくなったので、その経路は無い。
// 宣言種目そのものはバリエーションレーンに出ない（D-125）。
```

「バリエーションの日は窓の証拠に使わない」の段落の主語を「軽い日」に直し、具体例を「宣言した派生が、重点種目の派生の番（focusVariationRole・0.81）でも出る日」に差し替える（`TestSessionPlanner_AxisOverload_ExcludesLighterDays` の状況）。

- [ ] **Step 9: 走らせて緑を確かめ、変異で赤くなることを確かめる**

Run: `go test ./internal/domain/training/planning/ -v -run 'TestSessionPlanner_FocusAxisRotates|TestSessionPlanner_DerivativeAxisIsNotOverloaded|TestSessionPlanner_AxisOverload|TestLanePrescriptions'`
Expected: PASS

変異（1つずつ入れて、走らせ、戻す）：
1. `prescribeSet` の上乗せの条件に `|| role == focusVariationRole` を足す → `DerivativeAxisIsNotOverloaded` が FAIL（82.5kg）
2. `axis` の `case 2` を `return d, heavyRole` に戻す → `FocusAxisRotates/3周目` が FAIL
3. `overload` の `if !performedAtLeast(s, w) { continue }` を消す → `ExcludesLighterDays` が FAIL（90kg）。**赤くならなければ、テストの履歴が守りを通っていない。** Step 3 の前提を見直す

- [ ] **Step 10: 全体の検収とシミュレーションの数字の確認**

Run:
```bash
gofmt -l . && go vet ./... && go test ./...
go test ./internal/domain/training/planning/ -run TestSessionPlanner_PlanIsFixedForTheWholeDay -v
SIM=<scratchpad>/sim; mkdir -p $SIM
go test ./internal/domain/training/seed/ -run TestSimulation -v > $SIM/after.txt 2>&1
git worktree add $SIM/base main
(cd $SIM/base && go test ./internal/domain/training/seed/ -run TestSimulation -v > $SIM/before.txt 2>&1)
git worktree remove $SIM/base
diff $SIM/before.txt $SIM/after.txt
```
（比べる相手は積む先のブランチ。`git stash` は未追跡ファイルを退避しないので使わない。CLAUDE.md の注意。）

Expected: 全部緑。シミュレーションの数字は**重点種目のある構成で、派生の日の重量と、それに続く推定**だけが動く。重点の無い構成や派生の無い種目の数字が動いたら、想定外の依存なので原因を調べる。動いた理由を PR 本文に書く。

- [ ] **Step 11: コミットと PR**

```bash
git add internal/domain/training/planning/
git commit -m "feat: 重点種目の一巡で派生が軸に立つ日を軽い番・上乗せなしにする

派生の目的はフォームの向上で、重くする必要が無い。0.88・3レップ・上乗せ
ありから 0.81・6レップ・上乗せなしにする。

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git push -u origin feat/derived-axis-light
gh pr create --title "派生が軸に立つ日を軽い番・上乗せなしにする" --body "..."
```

PR 本文には、設計書へのリンク、動いたシミュレーションの数字とその理由、`overload` のコメントの前提を書き直したことを書く。

---

## PR 2：宣言ごとのレップ数（ドメイン・保存・API）

ブランチ：`feat/declared-rep-targets`（`feat/derived-axis-light` の上に積む）

### Task 2: 値オブジェクト `RepTargets` と、`Program` が宣言ごとに持つこと

**Files:**
- Create: `internal/domain/training/program/rep_targets.go`
- Create: `internal/domain/training/program/rep_targets_test.go`
- Modify: `internal/domain/training/program/program.go:67-198`（`Program`・`programParams`・`newProgram`・`params`）, `:245-252`（`WithDeclared`）、アクセサを追加
- Test: `internal/domain/training/program/program_test.go`

**Interfaces:**
- Produces:
  - `program.NewRepTargets(heavy, light int) (program.RepTargets, error)`
  - `program.DefaultRepTargets() program.RepTargets`（3, 6）
  - `(program.RepTargets).Heavy() int`, `.Light() int`, `.IsZero() bool`
  - `(*program.Program).RepTargetsFor(id exercise.ExerciseID) program.RepTargets`（持っていなければ既定）
  - `(*program.Program).WithRepTargets(id exercise.ExerciseID, t program.RepTargets) (*program.Program, error)`（宣言していない ID はエラー）
  - `(*program.Program).DeclaredRepTargets() map[exercise.ExerciseID]program.RepTargets`（保存用。既定と同じ値も含めて、設定されたものをそのまま返す写し）

- [ ] **Step 1: 失敗するテストを書く（`rep_targets_test.go`）**

```go
package program_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// レップ数は 1〜15。重い番と軽い番のどちらにも同じ範囲が掛かる。
//
// 15 は限界までの総レップ（レップ + RIR1 = 16）が推定に使える上限
// （maxRepsToFailure = 20）の内側に収まる値。N ≤ M は強制しない。
func TestNewRepTargets(t *testing.T) {
	cases := []struct {
		name         string
		heavy, light int
		ok           bool
	}{
		{"既定", 3, 6, true},
		{"下限", 1, 1, true},
		{"上限", 15, 15, true},
		{"重い番のほうが多くてもよい", 8, 5, true},
		{"重い番が0", 0, 6, false},
		{"軽い番が0", 3, 0, false},
		{"重い番が16", 16, 6, false},
		{"軽い番が16", 3, 16, false},
		{"負", -1, 6, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := program.NewRepTargets(c.heavy, c.light)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v。ok=%v のはず", err, c.ok)
			}
			if c.ok && (got.Heavy() != c.heavy || got.Light() != c.light) {
				t.Errorf("(%d, %d) が (%d, %d) になった", c.heavy, c.light, got.Heavy(), got.Light())
			}
		})
	}
}

// 既定は今の処方（重い番3・軽い番6）と同じ。ここが動くと全員の重量が動く。
func TestDefaultRepTargets(t *testing.T) {
	d := program.DefaultRepTargets()
	if d.Heavy() != 3 || d.Light() != 6 {
		t.Errorf("既定が (%d, %d)。(3, 6) のはず", d.Heavy(), d.Light())
	}
	if d.IsZero() {
		t.Error("既定がゼロ値と区別できない")
	}
}
```

- [ ] **Step 2: 失敗するテストを書く（`program_test.go`）**

ヘルパーを足す。

```go
func mustRepTargets(t *testing.T, heavy, light int) program.RepTargets {
	t.Helper()
	r, err := program.NewRepTargets(heavy, light)
	if err != nil {
		t.Fatalf("NewRepTargets(%d, %d): %v", heavy, light, err)
	}
	return r
}
```

`fieldsOf` に1行足す（並びを固定して文字列にする）。

```go
		"reps":      repsString(p),
```

```go
// repsString は宣言ごとのレップ数を、宣言の順に "id:重い/軽い" で並べる。
func repsString(p *program.Program) string {
	s := ""
	for _, id := range p.DeclaredExercises() {
		r := p.RepTargetsFor(id)
		s += fmt.Sprintf("%s:%d/%d ", id, r.Heavy(), r.Light())
	}
	return s
}
```

`TestProgram_WithKeepsOtherFields` を直す。出発点で bench に (8, 12) を立てる（既定のままだと、落ちても前後とも既定で一致して空振りする）。

```go
			base, err = base.WithRepTargets("bench", mustRepTargets(t, 8, 12))
			if err != nil {
				t.Fatalf("WithRepTargets: %v", err)
			}
```

（`base.WithCycle(...)` の直後に置く。）ケースを1つ足す。

```go
		{
			name: "WithRepTargets", changed: []string{"reps"},
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithRepTargets("squat", mustRepTargets(t, 6, 10))
			},
		},
```

`WithDeclared` のケースは `{"bench", "deadlift"}` で squat が外れる。squat は既定のままなので `reps` の文字列は「squat:3/6」が「deadlift:3/6」に変わる。`changed` を `[]string{"declared", "reps"}` にする（宣言の顔ぶれが変われば、宣言ごとの一覧も変わる）。

新しいテストを足す。

```go
// 持っていない宣言には既定を返す。宣言していない種目にも既定を返す
// （計画の側は軸に立った種目の値を引くだけで、宣言かどうかを知らない）。
func TestProgram_RepTargetsForFallsBackToTheDefault(t *testing.T) {
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 4, 3),
		big3(), []exercise.ExerciseID{"bench", "squat"}, "")
	if err != nil {
		t.Fatalf("NewProgram: %v", err)
	}
	p, err = p.WithRepTargets("bench", mustRepTargets(t, 8, 12))
	if err != nil {
		t.Fatalf("WithRepTargets: %v", err)
	}

	for _, c := range []struct {
		id           exercise.ExerciseID
		heavy, light int
	}{
		{"bench", 8, 12},
		{"squat", 3, 6},
		{"deadlift", 3, 6},
	} {
		got := p.RepTargetsFor(c.id)
		if got.Heavy() != c.heavy || got.Light() != c.light {
			t.Errorf("%s が (%d, %d)。(%d, %d) のはず", c.id, got.Heavy(), got.Light(), c.heavy, c.light)
		}
	}
}

// 宣言していない種目にはレップ数を立てられない。黙って受けると、宣言に
// 入れた瞬間に昔の値が出てくる。
func TestProgram_WithRepTargetsRejectsUndeclared(t *testing.T) {
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 4, 3),
		big3(), []exercise.ExerciseID{"bench"}, "")
	if err != nil {
		t.Fatalf("NewProgram: %v", err)
	}
	if _, err := p.WithRepTargets("squat", mustRepTargets(t, 8, 12)); err == nil {
		t.Error("宣言していない squat に立てられた")
	}
	if _, err := p.WithRepTargets("bench", program.RepTargets{}); err == nil {
		t.Error("ゼロ値を受け取った")
	}
}

// 宣言から外した種目の値は捨てる。入れ直したら既定に戻る（設計書）。
func TestProgram_WithDeclaredDropsTheRemovedTargets(t *testing.T) {
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 4, 3),
		big3(), []exercise.ExerciseID{"bench", "squat"}, "")
	if err != nil {
		t.Fatalf("NewProgram: %v", err)
	}
	p, err = p.WithRepTargets("squat", mustRepTargets(t, 6, 10))
	if err != nil {
		t.Fatalf("WithRepTargets: %v", err)
	}

	dropped, err := p.WithDeclared([]exercise.ExerciseID{"bench"})
	if err != nil {
		t.Fatalf("外すのに失敗: %v", err)
	}
	if _, ok := dropped.DeclaredRepTargets()["squat"]; ok {
		t.Error("外した squat の値が残っている")
	}

	back, err := dropped.WithDeclared([]exercise.ExerciseID{"bench", "squat"})
	if err != nil {
		t.Fatalf("入れ直すのに失敗: %v", err)
	}
	if got := back.RepTargetsFor("squat"); got != program.DefaultRepTargets() {
		t.Errorf("入れ直した squat が (%d, %d)。既定のはず", got.Heavy(), got.Light())
	}
}

// 返す対応表は写し。書き換えても集約は変わらない。
func TestProgram_DeclaredRepTargetsIsACopy(t *testing.T) {
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 4, 3),
		big3(), []exercise.ExerciseID{"bench"}, "")
	if err != nil {
		t.Fatalf("NewProgram: %v", err)
	}
	p, err = p.WithRepTargets("bench", mustRepTargets(t, 8, 12))
	if err != nil {
		t.Fatalf("WithRepTargets: %v", err)
	}
	m := p.DeclaredRepTargets()
	m["bench"] = mustRepTargets(t, 1, 1)
	if got := p.RepTargetsFor("bench"); got.Heavy() != 8 {
		t.Errorf("外から書き換えられた: %d", got.Heavy())
	}
}
```

- [ ] **Step 3: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/domain/training/program/`
Expected: コンパイルエラー（`NewRepTargets`・`WithRepTargets`・`RepTargetsFor`・`DeclaredRepTargets` が未定義）

- [ ] **Step 4: `rep_targets.go` を書く**

```go
package program

import "fmt"

const (
	// minAxisReps と maxAxisReps は軸で狙うレップ数の範囲。
	//
	// 上限 15 は、軸の RIR1 を足した限界までの総レップ（16）が、推定に
	// 使える上限（training.maxRepsToFailure = 20）の内側に収まるため。
	// Epley は直線近似で、高レップほど粗くなる。
	minAxisReps = 1
	maxAxisReps = 15

	// defaultHeavyReps と defaultLightReps は今の処方と同じ値（0.88・0.81）。
	// 変えると、レップ数を設定していない全員の重量が動く。
	defaultHeavyReps = 3
	defaultLightReps = 6
)

// RepTargets は宣言種目を軸で出すときに狙うレップ数。不変。
//
// heavy は重い番（宣言の軸、重点種目の一巡の1番目）、light は軽い番
// （重点種目の一巡の2番目と、派生が軸に立つ3番目）。どちらを重くするかは
// 本人の判断なので、heavy ≤ light は強制しない。
//
// 種目マスタではなく宣言に持つ。「この種目を何レップで伸ばしたいか」は
// 種目の性質ではなく本人の目標で、メイン/補助を Program へ移したのと
// 同じ理屈（D-117）。
type RepTargets struct {
	heavy, light int
}

// NewRepTargets は範囲（1〜15）を検証して RepTargets を組み立てる。
func NewRepTargets(heavy, light int) (RepTargets, error) {
	for _, f := range []struct {
		name string
		v    int
	}{{"重い番", heavy}, {"軽い番", light}} {
		if f.v < minAxisReps || f.v > maxAxisReps {
			return RepTargets{}, fmt.Errorf("%sのレップ数は%d〜%dである必要がある: %d",
				f.name, minAxisReps, maxAxisReps, f.v)
		}
	}
	return RepTargets{heavy: heavy, light: light}, nil
}

// DefaultRepTargets は設定していない宣言に使う値（重い番3・軽い番6）。
func DefaultRepTargets() RepTargets {
	return RepTargets{heavy: defaultHeavyReps, light: defaultLightReps}
}

func (r RepTargets) Heavy() int   { return r.heavy }
func (r RepTargets) Light() int   { return r.light }
func (r RepTargets) IsZero() bool { return r == RepTargets{} }
```

- [ ] **Step 5: `Program` に持たせる（`program.go`）**

`Program` と `programParams` にフィールドを足す。

```go
	reps      map[exercise.ExerciseID]RepTargets // 宣言ごとのレップ数。無い宣言は既定
```

`programParams` のコメント「Program と同じ6つを持つ」を「Program と同じ7つを持つ」に直す。

`newProgram` の `cycle, err := normalizeCycle(x.cycle)` の後に足す。

```go
	// 宣言ごとのレップ数は、宣言に含まれる種目だけが持てる。宣言から外す
	// ときの掃除は WithDeclared がする。ここで黙って落とすと、宣言して
	// いない種目に立てた呼び出し（WithRepTargets）が成功に見える。
	reps := make(map[exercise.ExerciseID]RepTargets, len(x.reps))
	for id, r := range x.reps {
		if !slices.Contains(declared, id) {
			return nil, fmt.Errorf("レップ数を持つ %q が伸ばしたい種目に含まれていない", id)
		}
		if r.IsZero() {
			return nil, fmt.Errorf("%q のレップ数が設定されていない", id)
		}
		reps[id] = r
	}
```

`return &Program{...}` に `reps: reps,` を足し、`params()` に `reps: p.reps,` を足す（`newProgram` が必ず写すので共有されない。`params` のコメント「スライスの中身は写さない」を「スライスと対応表の中身は写さない」に直す）。

`WithDeclared` を直す。

```go
// WithDeclared は伸ばしたい種目だけを差し替えた新しいプログラムを返す。
//
// 重点種目が新しい宣言に含まれなくなる場合はエラーになる。黙って解除は
// しない。解除するかどうかは本人が決めることで、宣言を変えた副作用として
// 重点が消えると、次に画面を開くまで気づけない。
//
// 外した種目のレップ数は捨てる。入れ直したら既定に戻る（設計書
// 2026-10-03-declared-rep-targets）。重点と違って黙って捨ててよいのは、
// 外した種目は計画に出ないので、値が残っていても使われないため。
func (p *Program) WithDeclared(ids []exercise.ExerciseID) (*Program, error) {
	return p.with(func(x *programParams) {
		x.declared = ids
		kept := make(map[exercise.ExerciseID]RepTargets, len(x.reps))
		for id, r := range x.reps {
			if slices.Contains(ids, id) {
				kept[id] = r
			}
		}
		x.reps = kept
	})
}
```

アクセサと `WithRepTargets` を足す（`Declares` の後）。

```go
// WithRepTargets は1つの宣言のレップ数だけを差し替えた新しいプログラムを返す。
// 宣言していない種目はエラー。
func (p *Program) WithRepTargets(id exercise.ExerciseID, t RepTargets) (*Program, error) {
	return p.with(func(x *programParams) {
		next := make(map[exercise.ExerciseID]RepTargets, len(x.reps)+1)
		for k, v := range x.reps {
			next[k] = v
		}
		next[id] = t
		x.reps = next
	})
}

// RepTargetsFor はその種目を軸で出すときのレップ数。設定していなければ既定。
//
// 宣言していない種目にも既定を返す。計画の側は軸に立った種目の値を引くだけで、
// 宣言かどうかを確かめさせない。
func (p *Program) RepTargetsFor(id exercise.ExerciseID) RepTargets {
	if r, ok := p.reps[id]; ok {
		return r
	}
	return DefaultRepTargets()
}

// DeclaredRepTargets は設定されたレップ数の写し。保存に使う。設定していない
// 宣言は含まない。
func (p *Program) DeclaredRepTargets() map[exercise.ExerciseID]RepTargets {
	out := make(map[exercise.ExerciseID]RepTargets, len(p.reps))
	for k, v := range p.reps {
		out[k] = v
	}
	return out
}
```

- [ ] **Step 6: 走らせて緑を確かめ、変異で赤くなることを確かめる**

Run: `go test ./internal/domain/training/program/ -v`
Expected: PASS

変異（1つずつ）：
1. `params()` から `reps: p.reps,` を消す → `TestProgram_WithKeepsOtherFields` の `WithFrequency` などが FAIL
2. `WithDeclared` の掃除を消して `x.declared = ids` だけにする → `WithDeclaredDropsTheRemovedTargets` が FAIL（`newProgram` が「伸ばしたい種目に含まれていない」で弾く）
3. `newProgram` の `!slices.Contains(declared, id)` の検査を消す → `WithRepTargetsRejectsUndeclared` が FAIL
4. `NewRepTargets` の上限を 16 にする → `TestNewRepTargets/重い番が16` が FAIL

- [ ] **Step 7: コミット**

```bash
gofmt -l . && go vet ./internal/domain/...
git add internal/domain/training/program/
git commit -m "feat(program): 伸ばしたい種目ごとに軸のレップ数を持つ

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 3: 保存（`declared_reps` 列）

**Files:**
- Create: `internal/infrastructure/postgres/migrations/0016_add_declared_reps_on_program.sql`
- Modify: `internal/infrastructure/postgres/program_repository.go:39-180`
- Test: `internal/infrastructure/postgres/program_repository_test.go`

**Interfaces:**
- Consumes: `program.NewRepTargets`, `(*Program).WithRepTargets`, `(*Program).DeclaredRepTargets`
- Produces: 保存形 `{"pull_up": {"heavy": 8, "light": 12}}`。NULL は「設定なし」

- [ ] **Step 1: 失敗するテストを書く**

`TestProgramRepository_RoundTrips` の末尾に足す（既定の行は NULL で保存され、既定として読める）。

```go
	// レップ数を設定していないので、どの宣言も既定。
	if got := got.RepTargetsFor("bench"); got != program.DefaultRepTargets() {
		t.Errorf("bench のレップ数が (%d, %d)。既定のはず", got.Heavy(), got.Light())
	}
```

新しいテストを足す。

```go
// 宣言ごとのレップ数が往復すること。設定していない宣言は既定のまま。
func TestProgramRepository_RoundTripsTheRepTargets(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	repo := postgres.NewProgramRepository(pool)

	reps, err := program.NewRepTargets(8, 12)
	if err != nil {
		t.Fatalf("NewRepTargets: %v", err)
	}
	with, err := samplePrograms(t).WithRepTargets("squat", reps)
	if err != nil {
		t.Fatalf("WithRepTargets: %v", err)
	}
	if err := repo.Save(ctx, userA(t), with); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	got, err := postgres.NewProgramRepository(pool).Get(ctx, userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if r := got.RepTargetsFor("squat"); r.Heavy() != 8 || r.Light() != 12 {
		t.Errorf("squat が (%d, %d)。(8, 12) のはず", r.Heavy(), r.Light())
	}
	if r := got.RepTargetsFor("bench"); r != program.DefaultRepTargets() {
		t.Errorf("bench が (%d, %d)。既定のはず", r.Heavy(), r.Light())
	}
}

// 保存された値が範囲外なら、読み出しで弾く。jsonb は形を検査しないので、
// 読み出しが唯一の防波堤になる。
func TestProgramRepository_RejectsStoredRepTargetsOutOfRange(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	repo := postgres.NewProgramRepository(pool)

	if err := repo.Save(ctx, userA(t), samplePrograms(t)); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE program SET declared_reps = '{"bench": {"heavy": 99, "light": 6}}'::jsonb WHERE user_id = $1`,
		userA(t).String()); err != nil {
		t.Fatalf("書き換えに失敗: %v", err)
	}
	if _, err := repo.Get(ctx, userA(t)); err == nil {
		t.Error("範囲外の 99 が読めた")
	}
}
```

- [ ] **Step 2: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/infrastructure/postgres/ -run 'TestProgramRepository' -v`
Expected: `RoundTripsTheRepTargets` が FAIL（squat が既定。保存していない）。`RejectsStoredRepTargetsOutOfRange` が FAIL（列が無いので UPDATE がエラー）

- [ ] **Step 3: マイグレーションを書く**

`0016_add_declared_reps_on_program.sql`：

```sql
-- 伸ばしたい種目ごとに、軸で狙うレップ数（重い番・軽い番）を持たせる。
--
-- 形は {"pull_up": {"heavy": 8, "light": 12}}。設定した宣言だけを持つ。
-- 既存の行は NULL＝どの宣言も既定（重い番3・軽い番6）で、挙動は変わらない。
-- 埋め戻しは要らない。
--
-- jsonb にするのは selected / declared / split_cycle と同じ理由で、要素数が
-- 可変なため。範囲（1〜15）は読み出しのたびに program.NewRepTargets が見る。
ALTER TABLE program ADD COLUMN declared_reps jsonb;
```

- [ ] **Step 4: 読み書きを書く（`program_repository.go`）**

保存形の型を末尾に足す。

```go
// repTargetsRow は宣言1件ぶんのレップ数の保存形。
type repTargetsRow struct {
	Heavy int `json:"heavy"`
	Light int `json:"light"`
}
```

`Get`：変数に `rawReps []byte` を足し、SELECT と Scan に `declared_reps` を足す。分割を読んだ後（`return prog, nil` の前）に足す。

```go
	if len(rawReps) > 0 {
		var rows map[string]repTargetsRow
		if err := json.Unmarshal(rawReps, &rows); err != nil {
			return nil, fmt.Errorf("宣言ごとのレップ数を解釈できない: %w", err)
		}
		// キーの順で当てる。どれが不正かの診断が毎回同じ種目を指すように。
		for _, id := range slices.Sorted(maps.Keys(rows)) {
			row := rows[id]
			reps, err := program.NewRepTargets(row.Heavy, row.Light)
			if err != nil {
				return nil, fmt.Errorf("保存された %s のレップ数が不正: %w", id, err)
			}
			prog, err = prog.WithRepTargets(exercise.ExerciseID(id), reps)
			if err != nil {
				return nil, fmt.Errorf("保存された %s のレップ数が不正: %w", id, err)
			}
		}
	}
```

（`import` に `maps` と `slices` を足す。）

`Save`：分割の後に足す。

```go
	// 設定が無ければ NULL。既存の行と形を揃える。
	var rawReps []byte
	if reps := p.DeclaredRepTargets(); len(reps) > 0 {
		rows := make(map[string]repTargetsRow, len(reps))
		for id, r := range reps {
			rows[string(id)] = repTargetsRow{Heavy: r.Heavy(), Light: r.Light()}
		}
		rawReps, err = json.Marshal(rows)
		if err != nil {
			return fmt.Errorf("宣言ごとのレップ数を書き出せない: %w", err)
		}
	}
```

INSERT の列・値・`ON CONFLICT` に `declared_reps`（`$9`、`declared_reps = EXCLUDED.declared_reps`）を足し、引数の最後に `rawReps` を足す。`TestProgramRepository_RoundTrips` のコメント「jsonb の列が2つ（選択・宣言）と分割で3つある」を「…と分割・レップ数で4つある」に直す。

- [ ] **Step 5: 走らせて緑を確かめ、変異で赤くなることを確かめる**

Run: `go test ./internal/infrastructure/postgres/ -v -run 'TestProgramRepository|TestMigrate'`
Expected: PASS

変異：`Save` の `rawReps` を常に `nil` で渡す → `RoundTripsTheRepTargets` が FAIL。`Get` の `NewRepTargets` を飛ばして範囲外を通す（例：検証のエラーを無視する）→ `RejectsStoredRepTargetsOutOfRange` が FAIL。

- [ ] **Step 6: 旧版の DB に新版を当てる**

CLAUDE.md の手順。`git stash` は未追跡ファイル（新しいマイグレーション）を退避しないので使わない。代わりに次の手順で確かめる。

1. `git worktree add <scratchpad>/old feat/derived-axis-light` で旧版を取り出す
2. 旧版で embedded-postgres を立て、マイグレーションを当て、プログラムを1行保存する（`internal/infrastructure/postgres` のテストの `newTestDB` と同じ要領。メモリ「docker 無しでの DB・画面検査」）
3. 同じデータディレクトリに新版のマイグレーションを当て、`Get` が既定のレップ数で読めることを確かめる
4. worktree を消す

確かめた手順と結果を PR 本文に書く。

- [ ] **Step 7: コミット**

```bash
gofmt -l . && go vet ./...
git add internal/infrastructure/postgres/
git commit -m "feat(postgres): 宣言ごとのレップ数を保存する

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 4: 計画の導出がレップ数を使う（処方を式に置き換える）

**Files:**
- Modify: `internal/domain/training/planning/lane_prescription.go`（`prescriptionFor`、`prescribe`、`prescribeSet`、`overload`）
- Modify: `internal/domain/training/planning/session_planner.go:182-231`（lineup に値を載せる）
- Modify: `internal/domain/training/planning/horizon_projector.go:133,139`（`prescriptionFor` の呼び出し）
- Test: `internal/domain/training/planning/target_reps_test.go`, `axis_rotation_test.go`, `lane_prescription_internal_test.go`

**Interfaces:**
- Consumes: `(*program.Program).RepTargetsFor`, `program.DefaultRepTargets`, `program.NewRepTargets`
- Produces（パッケージ内）：
  - `func axisPrescription(reps, sets int) lanePrescription`
  - `func epleyIntensity(reps, rir int) float64`
  - `func (p SessionPlanner) prescriptionFor(role laneRole, sets int, reps program.RepTargets) lanePrescription`
  - `lineupEntry{exercise, role, reps program.RepTargets}`
  - `func axisRepTargets(prog *program.Program, axis *exercise.Exercise, role laneRole) program.RepTargets`

- [ ] **Step 1: 失敗するテストを書く（式と表の一致）**

`lane_prescription_internal_test.go` に足す。

```go
// 軸の強度は Epley の逆算を小数2桁に丸めた値。
//
// 丸めた値は今の表と一致する（3 → 0.88、6 → 0.81）。一致しないと、
// レップ数を設定していない全員の重量が動く。1〜15 のどれでも、丸めた
// 強度から Epley で逆算したレップ数が元に戻る（TestSessionPlanner_TargetReps
// の式の一致がそのまま通る）。
func TestAxisPrescription_FollowsEpley(t *testing.T) {
	for _, c := range []struct {
		reps int
		want float64
	}{
		{1, 0.94}, {3, 0.88}, {6, 0.81}, {8, 0.77}, {10, 0.73}, {12, 0.70}, {15, 0.65},
	} {
		got := axisPrescription(c.reps, 3)
		if got.intensityPct != c.want {
			t.Errorf("%dレップの強度が %v。%v のはず", c.reps, got.intensityPct, c.want)
		}
	}

	for reps := 1; reps <= 15; reps++ {
		got := axisPrescription(reps, 3)
		back := int(math.Round(30*(1/got.intensityPct-1))) - got.targetRIR
		if back != reps || got.targetReps != reps || got.targetRIR != 1 {
			t.Errorf("%dレップ: 強度 %v・RIR%d・目標 %d、逆算 %d", reps,
				got.intensityPct, got.targetRIR, got.targetReps, back)
		}
	}
}
```

（`import` に `math` を足す。）`TestLanePrescriptions_RoundTripIsCurrentlyContractive` の `planner.prescriptionFor(lane.role, 3)` を `planner.prescriptionFor(lane.role, 3, program.DefaultRepTargets())` に直す（`import` に `program` を足す）。

- [ ] **Step 2: 失敗するテストを書く（宣言のレップ数で処方される）**

`target_reps_test.go` に足す。

```go
// withReps は req のプログラムで、id にレップ数を立てる。
func withReps(t *testing.T, req planning.PlanRequest, id exercise.ExerciseID, heavy, light int) planning.PlanRequest {
	t.Helper()
	reps, err := program.NewRepTargets(heavy, light)
	if err != nil {
		t.Fatalf("NewRepTargets: %v", err)
	}
	req.Program, err = req.Program.WithRepTargets(id, reps)
	if err != nil {
		t.Fatalf("WithRepTargets: %v", err)
	}
	return req
}

// 宣言ごとのレップ数で軸が処方されること。強度は Epley の逆算
// （8レップ RIR1 → 0.77、12レップ RIR1 → 0.70）。
//
// 重点ベンチ（N=8・M=12）の一巡は 8 → 12 → 派生（重点ベンチの M = 12）。
// 派生の日に使うのは派生自身ではなく重点種目の値。
func TestSessionPlanner_AxisFollowsTheDeclaredReps(t *testing.T) {
	cases := []struct {
		name      string
		sessions  int
		want      exercise.ExerciseID
		intensity float64
		reps      int
	}{
		{name: "1周目は重い番", sessions: 3, want: "bench", intensity: 0.77, reps: 8},
		{name: "2周目は軽い番", sessions: 4, want: "bench", intensity: 0.70, reps: 12},
		{name: "3周目の派生は重点種目の軽い番", sessions: 5, want: "tempo", intensity: 0.70, reps: 12},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := withReps(t, rotationRequest(t, c.sessions), "bench", 8, 12)
			set := mustPlan(t, req).Main()[0]
			if set.ExerciseID() != c.want {
				t.Fatalf("前提: 軸が %s。%s のはず", set.ExerciseID(), c.want)
			}
			assertIntensity(t, req, set, c.intensity)
			if got := set.TargetReps().Int(); got != c.reps {
				t.Errorf("目標レップが %d。%d のはず", got, c.reps)
			}
		})
	}
}

// ほかの宣言の値は、軸に立った種目の処方に混ざらないこと。
//
// ベンチに (8, 12) を立てても、スクワットが軸の日は既定（3レップ・0.88）。
func TestSessionPlanner_OtherDeclaredKeepTheirOwnReps(t *testing.T) {
	logs := rotationLogs(t, 4)
	logs = append(logs,
		mkLogOn(t, "sq", planMonday.AddDays(-40), "squat", 110, 8, 2),
		mkLogOn(t, "dl", planMonday.AddDays(-3), "deadlift", 140, 8, 2))

	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(logs)
	req = withReps(t, req, "bench", 8, 12)

	set := mustPlan(t, req).Main()[0]
	if set.ExerciseID() != "squat" {
		t.Fatalf("前提: 軸がスクワットであること: %s", set.ExerciseID())
	}
	assertIntensity(t, req, set, 0.88)
	if got := set.TargetReps().Int(); got != 3 {
		t.Errorf("目標レップが %d。3 のはず", got)
	}
}

// 重点でない宣言も、自分の重い番で出ること（チンニングの例）。
func TestSessionPlanner_NonFocusAxisUsesItsHeavyReps(t *testing.T) {
	req := planRequest(t)
	req.Program, req.Target = benchOnlyProgram(t) // 重点なし・宣言はベンチだけ
	req.History = setlog.NewHistory(rotationLogs(t, 4))
	req = withReps(t, req, "bench", 8, 12)

	set := mustPlan(t, req).Main()[0]
	if set.ExerciseID() != "bench" {
		t.Fatalf("前提: 軸がベンチであること: %s", set.ExerciseID())
	}
	assertIntensity(t, req, set, 0.77)
	if got := set.TargetReps().Int(); got != 8 {
		t.Errorf("目標レップが %d。8 のはず", got)
	}
}
```

（`import` に `exercise` と `program` を足す。）

- [ ] **Step 3: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/domain/training/planning/ -run 'TestAxisPrescription|TestSessionPlanner_AxisFollows|TestSessionPlanner_OtherDeclared|TestSessionPlanner_NonFocusAxisUses|TestLanePrescriptions' -v`
Expected: コンパイルエラー（`axisPrescription` が未定義、`prescriptionFor` の引数が多い）。コンパイルを通すためだけに `prescriptionFor` の引数を先に足した場合は、`AxisFollows`・`NonFocusAxisUses` が「0.88 相当の重量」で FAIL すること

- [ ] **Step 4: 処方を式に置き換える（`lane_prescription.go`）**

```go
// axisRIR は軸の目標 RIR。重い番も軽い番も同じ。
const axisRIR = 1

// epleyIntensity は「reps 回で RIR rir を残す」強度を、推定と同じ Epley を
// 逆に解いて出す。小数2桁に丸める（3レップ RIR1 → 0.88、6レップ → 0.81 と
// 今の表に一致する）。
//
// 根拠は Epley が正確だからではなく、行き（処方）と帰り（推定）が同じ式
// だから。処方どおりにこなした記録から出る推定1RMは元と変わらず、Epley
// 自体の誤差は打ち消し合う。
func epleyIntensity(reps, rir int) float64 {
	return math.Round(100/(1+float64(reps+rir)/30)) / 100
}

// axisPrescription は軸のレップ数から処方を組む。
func axisPrescription(reps, sets int) lanePrescription {
	return lanePrescription{
		intensityPct: epleyIntensity(reps, axisRIR),
		sets:         sets,
		targetRIR:    axisRIR,
		targetReps:   reps,
	}
}
```

`prescriptionFor` を置き換える（doc コメントの「0.88 は3レップ RIR1…」の段落は、軸の強度が宣言のレップ数から式で出ることに書き直す。バリエーションと補助が定数のままである理由の段落は残す）。

```go
func (p SessionPlanner) prescriptionFor(role laneRole, sets int, reps program.RepTargets) lanePrescription {
	switch role {
	case heavyRole:
		return axisPrescription(reps.Heavy(), sets)
	case focusVolumeRole, focusVariationRole:
		return axisPrescription(reps.Light(), sets)
	case variationRole:
		return lanePrescription{intensityPct: 0.80, sets: sets, targetRIR: 2, targetReps: 6}
	case accessoryRole:
		return lanePrescription{intensityPct: 0.71, sets: sets, targetRIR: 2, targetReps: 10}
	}
	return lanePrescription{}
}
```

`reps` は軸の役割にだけ効く。バリエーションと補助では読まない（doc コメントに書く）。

`prescribe` のループの `p.prescribeSet(..., entry.exercise, entry.role, sets, rirBump)` を、`entry.reps` も渡す形にする。`prescribeSet` の引数に `reps program.RepTargets` を足し、`lane := p.prescriptionFor(role, sets, reps)` にする。

`role` の doc コメント（`heavyRole は軸。3レップ相当で…` など）から固定のレップ数を外し、「重い番（宣言のレップ数の heavy）」「軽い番（light）」と書く。

`import` に `math` を足す。

- [ ] **Step 5: lineup に値を載せる（`session_planner.go`）**

```go
// lineupEntry はセッション1回ぶんの種目1つと、その役割。重量はまだ
// 付いていない。
//
// reps は軸の役割で狙うレップ数。軸以外の役割では使わない（既定を入れておく）。
type lineupEntry struct {
	exercise *exercise.Exercise
	role     laneRole
	reps     program.RepTargets
}

// axisRepTargets は軸に立った種目のレップ数を宣言から引く。
//
// 派生が軸に立つ日（focusVariationRole）は、派生自身ではなく重点種目の
// 値を使う。派生を宣言していなくても、重点種目の軽い番で出すため。
func axisRepTargets(prog *program.Program, axis *exercise.Exercise, role laneRole) program.RepTargets {
	if role == focusVariationRole {
		if focus, ok := prog.FocusExercise(); ok {
			return prog.RepTargetsFor(focus)
		}
	}
	return prog.RepTargetsFor(axis.ID())
}
```

`Forecast` の lineup 組み立てを直す。

```go
		if heavy, _, ok := sess.Axis(); ok {
			role := sess.axisLaneRole()
			lineup = append(lineup, lineupEntry{
				exercise: heavy, role: role,
				reps: axisRepTargets(req.Program, heavy, role),
			})
		}
		if v, _, ok := sess.Variation(); ok {
			lineup = append(lineup, lineupEntry{exercise: v, role: variationRole, reps: program.DefaultRepTargets()})
		}
```

補助の行も `reps: program.DefaultRepTargets()` を足す。

- [ ] **Step 6: 先の回のセット数（`horizon_projector.go`）**

`p.prescriptionFor(axisRole, sets)` と `p.prescriptionFor(variationRole, sets)` に第3引数 `program.DefaultRepTargets()` を渡す。ここで使うのはセット数だけで、セット数はレップ数に依らない。その旨を1行コメントで書く。

- [ ] **Step 7: `overload` を確かめる**

`overload` は `lane`（`prescribeSet` で引いた処方）と `intensity` を受け取るだけなので、宣言のレップ数の強度でそのまま判定する。コードは変えない。doc コメントの「0.88 と 0.81 が交互に来る」を「重い番と軽い番の強度が交互に来る」に直す。

- [ ] **Step 8: 走らせて緑を確かめ、変異で赤くなることを確かめる**

Run: `go test ./internal/domain/training/planning/ -v`
Expected: PASS（既存のテストも全部。既定の値で今の表と同じ強度になるため）

変異（1つずつ）：
1. `axisRepTargets` の派生の分岐を消す → `AxisFollows/3周目の派生は重点種目の軽い番` が FAIL（tempo 自身の既定 6）
2. `prescriptionFor` の `heavyRole` で `reps.Light()` を使う → `NonFocusAxisUses`・`AxisFollows/1周目` が FAIL
3. `Forecast` で軸の `reps` を常に `program.DefaultRepTargets()` にする → 新しい3テストが FAIL
4. `epleyIntensity` の丸めを外す → `TestAxisPrescription_FollowsEpley` が FAIL

- [ ] **Step 9: 全体の検収（既定では数字が1つも動かない）**

Run:
```bash
gofmt -l . && go vet ./... && go test ./...
go test ./internal/domain/training/planning/ -run TestSessionPlanner_PlanIsFixedForTheWholeDay -v
```
シミュレーション：Task 1 の Step 10 と同じ手順で、このブランチの前（`feat/derived-axis-light`）と後の `TestSimulation -v` の出力を比べる。
Expected: **差分なし。**動いたら、式の丸めか値の引き方が今の表とずれている。

- [ ] **Step 10: コミット**

```bash
git add internal/domain/training/planning/
git commit -m "feat(planning): 軸の処方を宣言ごとのレップ数から Epley の逆算で出す

既定（3・6）は今の表（0.88・0.81）と一致し、数字は動かない。

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 5: ユースケースと API

**Files:**
- Create: `internal/application/usecase/set_declared_rep_targets.go`
- Modify: `internal/presentation/httpapi/handler.go`（依存・口）, `dto.go`（DTO・`toProgramDTO`）, `router.go`（経路）
- Modify: `cmd/api/main.go:251` 付近（配線）
- Modify: `internal/presentation/httpapi/handler_test.go:115` 付近, `user_internal_test.go:100` 付近（テストの配線）
- Test: `internal/presentation/httpapi/handler_test.go`

**Interfaces:**
- Consumes: `program.NewRepTargets`, `(*Program).WithRepTargets`, `(*Program).RepTargetsFor`
- Produces:
  - `usecase.NewSetDeclaredRepTargets(reader program.Reader, writer program.Writer) *usecase.SetDeclaredRepTargets`
  - `(*SetDeclaredRepTargets).Execute(ctx, user account.UserID, id exercise.ExerciseID, heavy, light int) error`
  - `httpapi.Dependencies.SetDeclaredReps *usecase.SetDeclaredRepTargets`
  - `PUT /api/program/declared/{id}/reps` 本文 `{"heavy": 8, "light": 12}` → 204。範囲外・宣言していない種目は 400、未設定は 409
  - `GET /api/program` に `"declared_reps": {"bench": {"heavy": 3, "light": 6}, ...}`（宣言すべてを既定で埋める）

- [ ] **Step 1: 失敗するテストを書く（`handler_test.go`）**

```go
// 宣言ごとのレップ数を差し替え、GET で読めること。設定していない宣言も
// 既定（3・6）で埋めて返す（既定値をクライアントに二重に持たせない）。
func TestPutProgramDeclaredReps_SavesAndReadsBack(t *testing.T) {
	mux := newServer(t, true)

	if rec := do(t, mux, http.MethodPut, "/api/program/declared/squat/reps",
		`{"heavy":6,"light":10}`); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}

	rec := do(t, mux, http.MethodGet, "/api/program", "")
	var got struct {
		Declared []string `json:"declared_exercises"`
		Reps     map[string]struct {
			Heavy int `json:"heavy"`
			Light int `json:"light"`
		} `json:"declared_reps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if r := got.Reps["squat"]; r.Heavy != 6 || r.Light != 10 {
		t.Errorf("squat が %+v。{6 10} のはず", r)
	}
	for _, id := range got.Declared {
		if _, ok := got.Reps[id]; !ok {
			t.Errorf("宣言 %s のレップ数が返っていない", id)
		}
	}
	if r := got.Reps["bench"]; r.Heavy != 3 || r.Light != 6 {
		t.Errorf("bench が %+v。既定 {3 6} のはず", r)
	}
}

// レップ数の口は、レップ数だけを動かすこと（重点種目の口と同じ理由。D-127）。
func TestPutProgramDeclaredReps_TouchesNothingElse(t *testing.T) {
	mux := newServer(t, true)
	putUpperLowerSplit(t, mux)

	before := do(t, mux, http.MethodGet, "/api/program", "")
	if rec := do(t, mux, http.MethodPut, "/api/program/declared/bench/reps",
		`{"heavy":8,"light":12}`); rec.Code != http.StatusNoContent {
		t.Fatalf("保存に失敗: %d body=%s", rec.Code, rec.Body.String())
	}
	after := do(t, mux, http.MethodGet, "/api/program", "")

	var b, a map[string]json.RawMessage
	if err := json.Unmarshal(before.Body.Bytes(), &b); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	if err := json.Unmarshal(after.Body.Bytes(), &a); err != nil {
		t.Fatalf("JSONが壊れている: %v", err)
	}
	for k, want := range b {
		if k == "declared_reps" {
			continue
		}
		if string(a[k]) != string(want) {
			t.Errorf("%s が変わった: %s → %s", k, want, a[k])
		}
	}
	if string(a["declared_reps"]) == string(b["declared_reps"]) {
		t.Error("declared_reps が動いていない")
	}
}

func TestPutProgramDeclaredReps_Rejects(t *testing.T) {
	cases := []struct {
		name, path, body string
		want             int
	}{
		{"範囲外", "/api/program/declared/bench/reps", `{"heavy":16,"light":6}`, http.StatusBadRequest},
		{"0", "/api/program/declared/bench/reps", `{"heavy":3,"light":0}`, http.StatusBadRequest},
		{"宣言していない", "/api/program/declared/pull_up/reps", `{"heavy":8,"light":12}`, http.StatusBadRequest},
		{"知らないフィールド", "/api/program/declared/bench/reps", `{"heavy":8,"light":12,"x":1}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mux := newServer(t, true)
			if rec := do(t, mux, http.MethodPut, c.path, c.body); rec.Code != c.want {
				t.Errorf("%d が返った。%d のはず: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}

	t.Run("未設定", func(t *testing.T) {
		mux := newServer(t, false)
		if rec := do(t, mux, http.MethodPut, "/api/program/declared/bench/reps",
			`{"heavy":8,"light":12}`); rec.Code != http.StatusConflict {
			t.Errorf("%d が返った。409 のはず", rec.Code)
		}
	})
}
```

既存の表にも1行ずつ足す。
- `TestRoutes_RejectWrongMethod`：`{http.MethodPost, "/api/program/declared/bench/reps"},`
- `TestWrites_StopOnClientDisconnect`：`"declared-reps": {http.MethodPut, "/api/program/declared/bench/reps", `{"heavy":8,"light":12}`},`
- 認証の要る口を並べた表（`handler_test.go:1438` 付近の `{"/api/program/focus", ...}` の並び）に `{"/api/program/declared/bench/reps", `{"heavy":8,"light":12}`},` を足す

**注意：**`"宣言していない"` で `pull_up` を使うのは、`newServer` の既定の宣言が BIG3 だから。既定の宣言を確かめてから使う。`"未設定"` のケースが他の口と同じ 409 を返す前提は、`TestPutProgramFocus_Rejects` を読んで合わせる。

- [ ] **Step 2: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/presentation/httpapi/ -run 'TestPutProgramDeclaredReps|TestRoutes_RejectWrongMethod|TestWrites_StopOnClientDisconnect' -v`
Expected: 新しいテストが 404/405 で FAIL（口が無い）

- [ ] **Step 3: ユースケースを書く（`set_declared_rep_targets.go`）**

```go
package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SetDeclaredRepTargets は1つの宣言のレップ数（重い番・軽い番）だけを差し替える。
//
// SetFocusExercise と同じ形。受け取るのはその宣言の値だけで、プログラムの
// 残りは保存済みのものを使う。
//
// exercise.Reader を持たないのは、種目の実在を確かめる必要が無いため。
// 宣言していない種目は WithRepTargets が弾き、宣言は保存時に種目マスタを
// 通っている。
type SetDeclaredRepTargets struct {
	reader program.Reader
	writer program.Writer
}

func NewSetDeclaredRepTargets(reader program.Reader, writer program.Writer) *SetDeclaredRepTargets {
	return &SetDeclaredRepTargets{reader: reader, writer: writer}
}

// Execute は id のレップ数を差し替える。範囲外・宣言していない種目は入力の誤り。
func (u *SetDeclaredRepTargets) Execute(
	ctx context.Context, user account.UserID, id exercise.ExerciseID, heavy, light int,
) (err error) {
	defer func() { err = apperror.Classify(err) }()

	reps, err := program.NewRepTargets(heavy, light)
	if err != nil {
		return fmt.Errorf("%w: レップ数: %w", apperror.ErrInvalidInput, err)
	}

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}
	next, err := prog.WithRepTargets(id, reps)
	if err != nil {
		return fmt.Errorf("%w: レップ数: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("レップ数の保存が中断された: %w", err)
	}
	return u.writer.Save(ctx, user, next)
}
```

- [ ] **Step 4: DTO と口を書く**

`dto.go`：

```go
// repTargetsDTO は宣言1件ぶんのレップ数。書き込み（PUT …/reps）と
// 読み出し（programDTO.DeclaredReps）で同じ形。
//
// 0 は NewRepTargets が弾くので、欠落と「0回」を区別する必要が無い。
type repTargetsDTO struct {
	Heavy int `json:"heavy"`
	Light int `json:"light"`
}
```

`programDTO` に足す。

```go
	// DeclaredReps は宣言ごとのレップ数。設定していない宣言も既定で埋める。
	// 既定値（3・6）をクライアントに二重に持たせないため。
	DeclaredReps map[string]repTargetsDTO `json:"declared_reps"`
```

`toProgramDTO` の宣言のループで埋める。

```go
	declared := make([]string, 0)
	reps := make(map[string]repTargetsDTO)
	for _, id := range p.DeclaredExercises() {
		declared = append(declared, string(id))
		r := p.RepTargetsFor(id)
		reps[string(id)] = repTargetsDTO{Heavy: r.Heavy(), Light: r.Light()}
	}
```

戻り値に `DeclaredReps: reps,` を足す。

`handler.go`：`Handler` に `setDeclaredReps *usecase.SetDeclaredRepTargets`、`Dependencies` に `SetDeclaredReps *usecase.SetDeclaredRepTargets`、検査に `case d.SetDeclaredReps == nil: return nil, errMissingDependency("SetDeclaredReps")`、組み立てに `setDeclaredReps: d.SetDeclaredReps,` を足す（`SetFocus` の各行の隣）。口を足す。

```go
// handlePutProgramDeclaredReps は1つの宣言のレップ数だけを差し替える。
//
// 宣言していない種目・範囲外は 400。重点種目の口と同じく、他のフィールドを
// 触らない（D-127）。
func (h *Handler) handlePutProgramDeclaredReps(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req repTargetsDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}
	id := exercise.ExerciseID(r.PathValue("id"))
	if err := h.setDeclaredReps.Execute(r.Context(), user, id, req.Heavy, req.Light); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`router.go` の `PUT /api/program/declared` の次の行に足す。

```go
	mux.HandleFunc("PUT /api/program/declared/{id}/reps", h.handlePutProgramDeclaredReps)
```

- [ ] **Step 5: 配線**

`cmd/api/main.go`、`handler_test.go` の `dependencies`、`user_internal_test.go` の依存の並びの `SetFocus:` の次の行に足す。

```go
		SetDeclaredReps:  usecase.NewSetDeclaredRepTargets(programs, programs),
```

- [ ] **Step 6: 走らせて緑を確かめ、変異で赤くなることを確かめる**

Run: `go test ./internal/presentation/httpapi/ ./internal/application/... ./cmd/... -v -run 'Program|Routes|Writes'`
Expected: PASS

変異：`toProgramDTO` で既定を埋めず `p.DeclaredRepTargets()` だけを返す → `SavesAndReadsBack` が FAIL（bench が無い）。ユースケースで `WithRepTargets` のエラーを `ErrInvalidInput` で包まない → `Rejects/宣言していない` が 500 で FAIL。

- [ ] **Step 7: 全体の検収と PR**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/ cmd/
git commit -m "feat(api): 宣言ごとのレップ数を読み書きする口

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git push -u origin feat/declared-rep-targets
gh pr create --base feat/derived-axis-light --title "伸ばしたい種目ごとのレップ数（ドメイン・保存・API）" --body "..."
```

PR 本文：設計書のリンク、「既定では数字が1つも動かない」の確認結果（シミュレーションの差分なし）、旧版の DB に当てた手順と結果。

---

## PR 3：設定画面と devsim

ブランチ：`feat/declared-reps-ui`（`feat/declared-rep-targets` の上に積む）

### Task 6: 設定画面

**Files:**
- Create: `web/src/features/settings/reps.ts`, `web/src/features/settings/reps.test.ts`
- Modify: `web/src/api/types.ts:64-76`（`Program`）
- Modify: `web/src/features/settings/useProgramSettings.ts`
- Modify: `web/src/features/settings/useProgramSettings.test.ts`（`program()` の固定値に `declared_reps` を足す）
- Modify: `web/src/features/settings/ProgramSettings.tsx:173-214`
- Modify: `web/scripts/settings-check.mjs`

**Interfaces:**
- Consumes: `GET /api/program` の `declared_reps`、`PUT /api/program/declared/{id}/reps`
- Produces:
  - `type RepTargets = { heavy: number; light: number }`、`Program.declared_reps: Record<string, RepTargets>`
  - `REP_OPTIONS: number[]`（1〜15）
  - `repsPath(id: string): string`
  - `repsRows(program: Program): { id: string; reps: RepTargets }[]`（宣言の順。値が無い宣言は出さない）
  - フック：`saveReps(id: string, reps: RepTargets): Promise<void>`

- [ ] **Step 1: 失敗するテストを書く（`reps.test.ts`）**

```ts
import { describe, expect, it } from 'vitest';
import { REP_OPTIONS, repsPath, repsRows } from './reps';
import type { Program } from '../../api/types';

const program = (reps: Program['declared_reps']): Program => ({
  per_week: 3,
  exercises_per_session: 4,
  sets_per_exercise: 3,
  selected_exercises: ['bench', 'pull_up', 'squat'],
  declared_exercises: ['bench', 'pull_up'],
  focus_exercise: 'bench',
  splits: [],
  declared_reps: reps,
});

describe('REP_OPTIONS', () => {
  // サーバーの範囲（program.minAxisReps〜maxAxisReps）と同じ。ずれると
  // 選べるのに 400 が返る値か、選べない正当な値ができる。
  it('1〜15 を並べる', () => {
    expect(REP_OPTIONS).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15]);
  });
});

describe('repsPath', () => {
  // 種目 ID は利用者が足した種目だとサーバーが振るが、念のため符号化する。
  it('ID を符号化して口を組む', () => {
    expect(repsPath('pull_up')).toBe('/api/program/declared/pull_up/reps');
    expect(repsPath('a/b')).toBe('/api/program/declared/a%2Fb/reps');
  });
});

describe('repsRows', () => {
  it('宣言の順に並べる', () => {
    const rows = repsRows(
      program({ bench: { heavy: 3, light: 6 }, pull_up: { heavy: 8, light: 12 } }),
    );
    expect(rows).toEqual([
      { id: 'bench', reps: { heavy: 3, light: 6 } },
      { id: 'pull_up', reps: { heavy: 8, light: 12 } },
    ]);
  });

  // 宣言を足した直後、取り直すまで値が無いことがある。既定を手元で作らず、
  // 行を出さない（既定値はサーバーだけが持つ）。
  it('値が無い宣言は出さない', () => {
    expect(repsRows(program({ bench: { heavy: 3, light: 6 } }))).toEqual([
      { id: 'bench', reps: { heavy: 3, light: 6 } },
    ]);
  });
});
```

`useProgramSettings.test.ts` の `program()` に `declared_reps: {}` を足す（型を通すため）。

- [ ] **Step 2: 走らせて、期待した理由で落ちることを確かめる**

Run: `cd web && npx vitest run src/features/settings/reps.test.ts`
Expected: FAIL（`./reps` が無い）

- [ ] **Step 3: 型と判断を書く**

`types.ts`：

```ts
/** RepTargets は宣言1件ぶんの、軸で狙うレップ数。 */
export type RepTargets = {
  /** 重い番（宣言の軸、重点種目の一巡の1番目）。 */
  heavy: number;
  /** 軽い番（重点種目の一巡の2番目と、派生が軸に立つ3番目）。 */
  light: number;
};
```

`Program` に足す。

```ts
  /** 宣言ごとのレップ数。サーバーが既定（3・6）で埋めて返す。 */
  declared_reps: Record<string, RepTargets>;
```

`reps.ts`：

```ts
import type { Program, RepTargets } from '../../api/types';

/** REP_OPTIONS は選べるレップ数。サーバーの範囲（1〜15）と同じ。 */
export const REP_OPTIONS: number[] = Array.from({ length: 15 }, (_, i) => i + 1);

/** repsPath は1つの宣言のレップ数を差し替える口。 */
export const repsPath = (id: string): string => `/api/program/declared/${encodeURIComponent(id)}/reps`;

/** repsRows は宣言ごとのレップ数を、宣言の順に並べる。
 *
 *  値が無い宣言は出さない。宣言を足した直後は取り直すまで値が無いが、
 *  既定（3・6）を手元で作ると、サーバーの既定と二重に持つことになる。 */
export const repsRows = (program: Program): { id: string; reps: RepTargets }[] =>
  program.declared_exercises.flatMap((id) => {
    const reps = program.declared_reps[id];
    return reps ? [{ id, reps }] : [];
  });
```

- [ ] **Step 4: 走らせて緑を確かめる**

Run: `cd web && npx vitest run src/features/settings/`
Expected: PASS

- [ ] **Step 5: 手順を書く（`useProgramSettings.ts`）**

`saveReps` を足す。

```ts
  // レップ数は選んだその場で送る。送るのはその宣言の2つだけ。
  const saveReps = async (id: string, reps: RepTargets) => {
    if (!program || busy) return;
    if (!(await put(repsPath(id), reps))) return;
    setProgram({ ...program, declared_reps: { ...program.declared_reps, [id]: reps } });
    await onChanged();
  };
```

`toggleDeclaredExercise` を直す。宣言を足すと、その種目のレップ数（既定）はサーバーしか知らない。外した種目の値はサーバーが捨てる。どちらもサーバーの値を取り直して揃える。

```ts
  const toggleDeclaredExercise = async (id: string) => {
    if (!program || busy) return;
    const next = toggleDeclared(program.declared_exercises, id);
    if (!(await put('/api/program/declared', { declared_exercises: next }))) return;
    // 宣言ごとのレップ数は取り直す。足した種目の既定はサーバーしか知らず、
    // 外した種目の値はサーバーが捨てる。取り直しに失敗しても宣言は保存
    // できているので、手元の宣言だけ進める（レップ数の行は repsRows が
    // 値の無い宣言を出さない）。
    const fresh = await getJSON<Program>('/api/program').catch(() => null);
    setProgram(fresh ?? { ...program, declared_exercises: next });
    await onChanged();
  };
```

`import` に `RepTargets` と `repsPath` を足し、戻り値に `saveReps` を足す。冒頭のコメント「判断はこのファイル先頭と split.ts・…」に `reps.ts` を足す。

- [ ] **Step 6: 描画を書く（`ProgramSettings.tsx`）**

`useProgramSettings` の分割代入に `saveReps` を足す。`import { REP_OPTIONS, repsRows } from './reps';` を足す。

「伸ばしたい種目」の `ExercisePicker` の直後に足す。

```tsx
        {program && repsRows(program).length > 0 && (
          <>
            <p className="mt-4 text-[13px] font-bold">重い日のレップ数</p>
            <Note className="mb-3 mt-1">
              軸に立ったときに狙う回数です。重さはこの回数から決まります。
              チンニングのように少ない回数でやらない種目は増やしてください。
            </Note>
            <div className="grid gap-2">
              {repsRows(program).map(({ id, reps }) => (
                <label key={id} className="grid grid-cols-[1fr_auto] items-center gap-3">
                  <span className="text-sm">{nameOf(id)}</span>
                  <Select
                    aria-label={`${nameOf(id)}の重い日のレップ数`}
                    value={reps.heavy}
                    disabled={busy}
                    onChange={(e) => void saveReps(id, { ...reps, heavy: Number(e.target.value) })}
                  >
                    {REP_OPTIONS.map((n) => (
                      <option key={n} value={n}>
                        {n}回
                      </option>
                    ))}
                  </Select>
                </label>
              ))}
            </div>
          </>
        )}
```

「重点種目」のボタン群の直後に足す（選んでいる重点種目の分だけ）。

```tsx
        {program?.focus_exercise && program.declared_reps[program.focus_exercise] && (
          <>
            <p className="mt-4 text-[13px] font-bold">軽い日のレップ数</p>
            <Note className="mb-3 mt-1">
              重点種目は、重い日 → 軽い日 → 派生 の順に回ります。派生の日も
              この回数で出ます。
            </Note>
            <Select
              aria-label={`${nameOf(program.focus_exercise)}の軽い日のレップ数`}
              value={program.declared_reps[program.focus_exercise].light}
              disabled={busy}
              onChange={(e) => {
                const id = program.focus_exercise as string;
                void saveReps(id, { ...program.declared_reps[id], light: Number(e.target.value) });
              }}
            >
              {REP_OPTIONS.map((n) => (
                <option key={n} value={n}>
                  {n}回
                </option>
              ))}
            </Select>
          </>
        )}
```

- [ ] **Step 7: 配線の検査を足す（`settings-check.mjs`）**

既存の検査の後（`process.exit` の判定より前）に足す。宣言の先頭の種目の重い日を変え、サーバーに届いたかを API で確かめ、元に戻す。

```js
// 重い日のレップ数を変える。選んだその場でサーバーに届くか。
const repsBefore = await program();
const firstDeclared = repsBefore.declared_exercises[0];
const heavyBefore = repsBefore.declared_reps[firstDeclared].heavy;
const heavyWant = heavyBefore === 3 ? 8 : 3;
await page.locator(`select[aria-label$="の重い日のレップ数"]`).first().selectOption(String(heavyWant));
await page.waitForTimeout(1200);
const repsAfter = await program();
console.log(
  '重い日のレップ数:', heavyBefore, '→', repsAfter.declared_reps[firstDeclared].heavy,
  `（期待 ${heavyWant}）`,
);
if (repsAfter.declared_reps[firstDeclared].heavy !== heavyWant) errs.push('重い日のレップ数が保存されていない');
// 他の宣言は動かない。
for (const id of repsBefore.declared_exercises.slice(1)) {
  if (JSON.stringify(repsAfter.declared_reps[id]) !== JSON.stringify(repsBefore.declared_reps[id])) {
    errs.push(`${id} のレップ数が動いた`);
  }
}
// 元に戻す。
await page.locator(`select[aria-label$="の重い日のレップ数"]`).first().selectOption(String(heavyBefore));
await page.waitForTimeout(1200);
```

**注意：**`errs` の扱い（最後に件数を出して `process.exit(1)` するか）は既存のスクリプトの末尾に合わせる。「種目」の節が畳まれているなら、既存の検査が開いた後に置くか、開く操作を足す。

- [ ] **Step 8: 検収**

Run:
```bash
cd web && npm test && npx tsc -b
```
メモリ「docker 無しでの DB・画面検査」の手順でサーバーと画面を立て、`node scripts/settings-check.mjs` を回す（`APP=`・`API=` を渡す）。
Expected: 全部緑。`settings-check` が「重い日のレップ数: 3 → 8（期待 8）」を出し、エラー0件。

変異：`saveReps` の `put` の後の `setProgram` を消す → 画面の値が戻らないことを `settings-check` で（選択した値が `select` に残らない形で）確かめる。赤くならなければ、検査が「選択肢の表示」ではなく API しか見ていないことを PR 本文に書く。

- [ ] **Step 9: コミット**

```bash
git add web/
git commit -m "feat(web): 設定で伸ばしたい種目ごとのレップ数を選ぶ

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

### Task 7: devsim と開発用画面

**Files:**
- Modify: `internal/application/devsim/simulator.go:25-60`（`Request`・`Set`）, `:340-360`（`buildProgram`）, `:395-405`（`Set` の組み立て）
- Modify: `internal/presentation/httpapi/dev_simulation.go`（キー、解釈、設定の DTO、`devSetDTO`）
- Modify: `web/src/dev/simulate.ts`（`DevSettings`・`Form`・`defaultForm`・`buildQuery`・`parseForm`）, `web/src/dev/DevSimulation.tsx`（入力欄）
- Test: `internal/presentation/httpapi/dev_simulation_test.go`, `web/src/dev/simulate.test.ts`

**Interfaces:**
- Consumes: `program.NewRepTargets`, `(*Program).WithRepTargets`
- Produces:
  - `devsim.Request.Reps map[exercise.ExerciseID]program.RepTargets`
  - `devsim.Set.TargetReps int`、`devSetDTO.TargetReps int "target_reps"`
  - クエリ `reps=pull_up:8:12,squat:6:6`、応答の `settings.reps: {"pull_up": {"heavy": 8, "light": 12}}`
  - web：`Form.reps: string`（クエリと同じ1行のまま持つ。`custom` と同じ扱い）、`DevSettings.reps: Record<string, RepTargets>`

- [ ] **Step 1: 失敗するテストを書く（`dev_simulation_test.go`）**

```go
// 宣言ごとのレップ数をクエリで受け取り、設定に返し、軸の処方に効くこと。
func TestDevSimulation_TakesRepTargetsAndEchoesThem(t *testing.T) {
	rec := devGet(t, "/api/dev/simulate?declared=bench&focus=&split=&frequency=2&weeks=1&reps=bench:8:12")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d が返った。200 のはず: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Settings struct {
			Reps map[string]struct {
				Heavy int `json:"heavy"`
				Light int `json:"light"`
			} `json:"reps"`
		} `json:"settings"`
		Days []struct {
			Main []struct {
				ExerciseID string `json:"exercise_id"`
				TargetReps int    `json:"target_reps"`
			} `json:"main"`
		} `json:"days"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("応答が JSON でない: %v", err)
	}
	if r := got.Settings.Reps["bench"]; r.Heavy != 8 || r.Light != 12 {
		t.Errorf("設定に返った bench が %+v。{8 12} のはず", r)
	}
	if len(got.Days) == 0 || len(got.Days[0].Main) == 0 || got.Days[0].Main[0].TargetReps != 8 {
		t.Errorf("軸の目標レップが 8 になっていない: %+v", got.Days)
	}
}

func TestDevSimulation_RejectsBadRepsQuery(t *testing.T) {
	for _, q := range []string{
		"reps=bench:8",       // 軽い番が無い
		"reps=bench:x:12",    // 数字でない
		"reps=bench:16:12",   // 範囲外（devsim が弾く）
		"reps=squat:8:12",    // 宣言していない
	} {
		t.Run(q, func(t *testing.T) {
			rec := devGet(t, "/api/dev/simulate?declared=bench&"+q)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%d が返った。400 のはず: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
```

**注意：**devsim の入力の誤りが 400 になる経路（`errDevQuery` か、devsim が返すエラーの分類）は `TestDevSimulation_RejectsBadInput` を読んで合わせる。

- [ ] **Step 2: 走らせて、期待した理由で落ちることを確かめる**

Run: `go test ./internal/presentation/httpapi/ -run 'TestDevSimulation_TakesRepTargets|TestDevSimulation_RejectsBadReps' -v`
Expected: FAIL（`reps` は知らないキーなので 400。`TakesRepTargets` は 400 で落ちる）

- [ ] **Step 3: devsim に足す（`simulator.go`）**

`Request` に足す。

```go
	// Reps は宣言ごとの軸のレップ数。無い宣言は既定（3・6）。
	Reps map[exercise.ExerciseID]program.RepTargets
```

`Set` に `TargetReps int` を足し（`TargetRIR` の隣）、組み立てに `TargetReps: set.TargetReps().Int(),` を足す。

`buildProgram` の `NewProgram` の直後（分割の前）に足す。

```go
	// キーの順で当てる。どれが不正かの診断が毎回同じ種目を指すように。
	for _, id := range slices.Sorted(maps.Keys(req.Reps)) {
		prog, err = prog.WithRepTargets(id, req.Reps[id])
		if err != nil {
			return nil, fmt.Errorf("%s のレップ数が不正: %w", id, err)
		}
	}
```

- [ ] **Step 4: クエリと DTO（`dev_simulation.go`）**

`devQueryKeys` に `"reps": true` を足す。`devSetDTO` に `TargetReps int `json:"target_reps"`` を足し（`TargetRIR` の隣）、組み立てに `TargetReps: s.TargetReps,` を足す。`devSettingsDTO` に足す。

```go
	// Reps は宣言ごとの軸のレップ数。指定した宣言だけを返す。
	Reps map[string]repTargetsDTO `json:"reps"`
```

`toDevSettingsDTO` で埋める。

```go
	out.Reps = make(map[string]repTargetsDTO, len(req.Reps))
	for id, r := range req.Reps {
		out.Reps[string(id)] = repTargetsDTO{Heavy: r.Heavy(), Light: r.Light()}
	}
```

解釈を足す（`parseDevOneRepMax` の隣）。

```go
// parseDevReps は "bench:8:12,pull_up:6:10" を宣言ごとのレップ数にする。
// 範囲は program.NewRepTargets が見る。宣言に含まれるかは devsim が見る。
func parseDevReps(v string) (map[exercise.ExerciseID]program.RepTargets, error) {
	out := map[exercise.ExerciseID]program.RepTargets{}
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		fields := strings.Split(item, ":")
		if len(fields) != 3 {
			return nil, errDevQuery("reps", item)
		}
		heavy, err1 := strconv.Atoi(strings.TrimSpace(fields[1]))
		light, err2 := strconv.Atoi(strings.TrimSpace(fields[2]))
		if err1 != nil || err2 != nil {
			return nil, errDevQuery("reps", item)
		}
		r, err := program.NewRepTargets(heavy, light)
		if err != nil {
			return nil, fmt.Errorf("クエリ reps の %s が不正: %w", item, err)
		}
		out[exercise.ExerciseID(strings.TrimSpace(fields[0]))] = r
	}
	return out, nil
}
```

`parseDevRequest` の `orm` の後に足す。

```go
	reps, err := parseDevReps(q.Get("reps"))
	if err != nil {
		return devsim.Request{}, err
	}
	out.Reps = reps
```

**注意：**`fmt.Errorf` で返したエラーがハンドラで 400 になるか、`errDevQuery` でないと 500 になるかを `handleSimulate` を読んで確かめ、400 にならないなら `devQueryError` に合わせる。

- [ ] **Step 5: 走らせて緑を確かめる**

Run: `go test ./internal/application/devsim/ ./internal/presentation/httpapi/ -v -run 'Dev'`
Expected: PASS

- [ ] **Step 6: 開発用画面（`simulate.ts`・`DevSimulation.tsx`）**

`simulate.test.ts` に失敗するテストを足す。

```ts
describe('reps', () => {
  it('buildQuery が reps をそのまま送り、parseForm が読み戻す', () => {
    const form = { ...defaultForm, reps: 'pull_up:8:12' };
    const q = buildQuery(form);
    expect(new URLSearchParams(q).get('reps')).toBe('pull_up:8:12');
    expect(parseForm(q, defaultForm).reps).toBe('pull_up:8:12');
  });

  it('空なら送らない', () => {
    expect(new URLSearchParams(buildQuery(defaultForm)).has('reps')).toBe(false);
  });
});
```

Run: `cd web && npx vitest run src/dev/simulate.test.ts` → FAIL（`reps` が無い）

`Form` に `reps: string;`（コメント：`宣言ごとのレップ数。サーバーと同じ「id:重い:軽い」を , で並べた1行のまま持つ。空なら既定。`）、`defaultForm` に `reps: ''`、`DevSettings` に `reps: Record<string, { heavy: number; light: number }>;` を足す。`buildQuery` に `if (form.reps.trim() !== '') q.set('reps', form.reps.trim());`、`parseForm` に `reps: q.has('reps') ? (q.get('reps') ?? '') : fallback.reps,` を足す。

`DevSimulation.tsx` の `custom` の入力欄と同じ形で、ラベル「レップ数（id:重い:軽い を , 区切り。空なら全部 3:6）」の1行入力を足す。`id="reps"`、`value={form.reps}`、`onChange={(ev) => setForm({ ...form, reps: ev.target.value })}`。

日ごとの表で `target_reps` を出しているなら列を足す。出していないなら、`DevSet` 型（`simulate.ts`）に `target_reps: number` を足し、軸の行に「目標 N 回」を出す（Claude が数字をテキストで読めるように。メモリ「開発用シミュレーションはPC・AIファースト」）。

Run: `cd web && npm test && npx tsc -b` → PASS

- [ ] **Step 7: 実際に回して読む**

dev-simulation スキルの手順で `/api/dev/simulate?...&declared=bench,pull_up&reps=pull_up:8:12` を叩き、`pull_up` が軸の日に `pct_of_1rm` ≈ 0.77・`target_reps` 8 で出ていることをテキストで確かめる。結果を PR 本文に貼る。

- [ ] **Step 8: コミットと PR**

```bash
gofmt -l . && go vet ./... && go test ./... && (cd web && npm test && npx tsc -b)
git add internal/ web/
git commit -m "feat(devsim): 宣言ごとのレップ数を設定できるようにする

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git push -u origin feat/declared-reps-ui
gh pr create --base feat/declared-rep-targets --title "設定画面と devsim で宣言ごとのレップ数を選ぶ" --body "..."
```

PR 本文：設計書のリンク、`settings-check` の出力、devsim の確認結果。設計書の `status` を「実装済み（PR 1〜3）」に直すのはこの PR で行う。
