# 利用者ごとの種目一覧 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 種目をすべて利用者ごとの一覧にし（プリセットは初期値としてコピー）、どの種目も足す・直す（名前・効き方・刻み）・消すができるようにする。管理は別ページで行う。

**Architecture:** `exercise.Reader.FindAll(ctx, user)` が「その人の行が無ければプリセットを入れてから返す」。種目はプリセット由来も足したものも同じ `*exercise.Exercise`。表は `user_exercises`（未適用の 0013 を置き換える）。計画の導出と週目標（プリセットから計算）は触らない。

**Tech Stack:** Go（pgx v5、Postgres）、React + TypeScript（Vite、vitest）。

**Spec:** `docs/specs/2026-09-26-custom-exercises-design.md`（2026-09-27 改訂版。「決めたこと」の表が拘束力を持つ）

## 前提と進め方

- ブランチは積み重ね。**Part A は `dyoshyy/custom-exercises`（#220）**、Part B は `dyoshyy/custom-exercises-web`（#221）、Part C は `dyoshyy/custom-exercises-devsim`（#224）。Part A が終わったら B・C のブランチを `git rebase --onto` で載せ直す（コントローラが行う）
- 各タスク：テストを書く → 期待した理由で赤 → 実装 → 緑 → **変異を手で入れて赤を確認**（`.claude/skills/writing-tests/`）
- 検収コマンド：`gofmt -l .`（空）、`go vet ./...`、`go test ./...`。Postgres を触るタスクは `TEST_DATABASE_URL=postgres://postgres:x@127.0.0.1:55432/postgres`（コントローラが組み込み Postgres を立てる。docker は使えない）。web は `cd web && npx tsc --noEmit && npx vitest run`
- アプリケーション層のテストは `internal/infrastructure` を import できない（`TestOnion_DependenciesPointInward`）。usecase のテストはファイル内のフェイク（既存の `add_custom_exercise_test.go` の `exerciseRepo`・`programRepo`）を使う
- コミットの末尾に `Co-Authored-By` の行を付ける。push しない

## Global Constraints

- 種目は利用者ごと。プリセットは、その人の行が1件も無いときに一度だけコピーする（消した行も「行がある」に数える）
- プリセット由来の ID はそのまま（`bench` など）。足した種目の ID は `u-`＋16進16文字（`exercise.NewRandomCustomExerciseID` を使い続けてよい。名前は Task 1 で変える）
- 直せるのは名前・効き方・刻み。自重係数と派生元はコピーしたまま保つ
- 効き方：各区分 0.1〜1.0（`training.NewContribution` の範囲）、合わせて8区分まで、**寄与1.0の区分が1つ以上**。この規則は `NewExercise` に置き、全種目に効かせる
- 名前：前後の空白を落として1〜40文字（rune 数）。消していない種目の中で重複しない（直すときは自分自身を除く）
- 足すと使う種目に入る。消すと使う種目から外れる（プログラムを先に保存、種目を後）。伸ばしたい種目に入っていれば 409
- プリセット由来も消せる・直せる。プリセット由来かどうかで扱いを変えない（`IsCustom` は無くす）
- 週目標はプリセットの一覧から（`seed.averageStimulusPerSet` は触らない）
- エラー：400 `INVALID_INPUT`、409 `DUPLICATE_NAME`、404 `EXERCISE_NOT_FOUND`、409 `STILL_DECLARED`（既存の apperror を使う）
- `FindAll` の並びは ID 昇順（memory・Postgres で揃える）

## Review Focus

1. **既に使っている利用者が新版を開いたとき**：その人の行は0件なのでプリセットが入り、記録・使う種目・伸ばしたい種目・重点がそのまま通ること → Task 2・3 の「初回の読み出しでプリセットが入る」と、Task 5 の `TestSimulation` 不変
2. **全部消した利用者**：行は残る（論理削除）ので、プリセットが入り直さないこと → Task 2・3 のテスト
3. **同時に2回読む**（ログイン直後に画面が並列で取りに来る）：プリセットが二重に入らない・エラーにならないこと → Task 3 の並行テスト
4. **プリセットの寄与（0.7・0.4）を持つ種目を、名前だけ直す**：寄与が変わらないこと → Task 1・4 のテスト
5. **直した名前が、消した種目と同じ**：通ること／消していない別の種目と同じ：409 → Task 4 のテスト

---

## Part A：サーバー（`dyoshyy/custom-exercises`、#220）

### Task 1: ドメイン — 全種目に共通の規則と `Edit`

**Files:**
- Modify: `internal/domain/training/exercise/exercise.go`
- Modify: `internal/domain/training/exercise/custom_exercise.go`（このタスクでは消さない。Task 5 で消す）
- Test: `internal/domain/training/exercise/exercise_test.go`

**Interfaces:**
- Produces:
  - `NewExercise` が「寄与1.0の区分が無い」「名前が40 rune 超」を弾く
  - `type ExerciseEdit struct { Name string; Stimulus map[training.MuscleRegion]float64; IncrementKg float64 }`
  - `func (e *Exercise) Edit(p ExerciseEdit) (*Exercise, error)` — 元を変えず、名前・効き方・刻みを差し替えた新しい値を返す。ID・自重係数・派生元・deleted は引き継ぐ。検証は `NewExercise` と同じ（`NewExercise` に委譲して組み直す）
  - `func NewRandomExerciseID() (ExerciseID, error)`（`NewRandomCustomExerciseID` の新しい名前。旧名は Task 5 まで残す）

- [ ] **Step 1: テストを書く**（`exercise_test.go` に追記。既存のヘルパに合わせる）

```go
func TestNewExercise_RequiresARegionAtFullContribution(t *testing.T) {
	_, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "x", Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 0.9},
	})
	if err == nil {
		t.Error("寄与1.0の区分が無い種目が通った")
	}
}

func TestNewExercise_NameUpToFortyRunes(t *testing.T) {
	p := exercise.ExerciseParams{ID: "x", IncrementKg: 2.5, Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}}
	p.Name = strings.Repeat("あ", 40)
	if _, err := exercise.NewExercise(p); err != nil {
		t.Fatal(err)
	}
	p.Name = strings.Repeat("あ", 41)
	if _, err := exercise.NewExercise(p); err == nil {
		t.Error("41文字が通った")
	}
}

// 名前だけ直しても、プリセットの細かい寄与（0.7・0.4）は変わらないこと。
// 画面は直さない項目も今の値のまま送るので、Edit は渡された値をそのまま使う。
func TestExercise_EditKeepsBodyweightAndDerivedFrom(t *testing.T) {
	squat, _ := exercise.NewExercise(exercise.ExerciseParams{
		ID: "pull_up_like", Name: "チンニング", IncrementKg: 2.5, BodyweightFactor: 0.95, DerivedFrom: "parent",
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1, training.Biceps: 0.5},
	})
	got, err := squat.Edit(exercise.ExerciseEdit{
		Name: "  懸垂  ", IncrementKg: 1.25,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1, training.Biceps: 0.7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != "pull_up_like" || got.Name() != "懸垂" || got.Increment().Kg() != 1.25 {
		t.Errorf("直した値が違う: %s %s %v", got.ID(), got.Name(), got.Increment().Kg())
	}
	if c, _ := got.Stimulus().Contribution(training.Biceps); c.Float() != 0.7 {
		t.Errorf("寄与が %v", c.Float())
	}
	if got.BodyweightFactor().Float() != 0.95 {
		t.Error("自重係数が引き継がれていない")
	}
	if from, ok := got.DerivedFrom(); !ok || from != "parent" {
		t.Error("派生元が引き継がれていない")
	}
	if c, _ := squat.Stimulus().Contribution(training.Biceps); c.Float() != 0.5 {
		t.Error("元の値が書き換わった")
	}
}

func TestExercise_EditValidatesLikeNewExercise(t *testing.T) {
	e, _ := exercise.NewExercise(exercise.ExerciseParams{ID: "x", Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}})
	if _, err := e.Edit(exercise.ExerciseEdit{Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 0.5}}); err == nil {
		t.Error("寄与1.0の区分が無くなる編集が通った")
	}
}

func TestExercise_EditKeepsDeleted(t *testing.T) {
	e, _ := exercise.NewExercise(exercise.ExerciseParams{ID: "x", Name: "x", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}})
	got, _ := e.Delete().Edit(exercise.ExerciseEdit{Name: "y", IncrementKg: 2.5,
		Stimulus: map[training.MuscleRegion]float64{training.Lat: 1}})
	if !got.IsDeleted() {
		t.Error("消した印が落ちた")
	}
}
```

- [ ] **Step 2: 赤を確認** — `go test ./internal/domain/training/exercise/`。期待：`undefined: exercise.ExerciseEdit` など
- [ ] **Step 3: 実装する**
  - `NewExercise`：`NewStimulusProfile` の後に「寄与が 1.0 の区分が1つも無ければエラー」、名前の trim の後に rune 数の上限（`maxNameRunes = 40`。`custom_exercise.go` の `maxCustomNameRunes` はこれに寄せる）
  - `Edit`：`NewExercise(ExerciseParams{ID: string(e.id), Name: p.Name, Stimulus: p.Stimulus, IncrementKg: p.IncrementKg, BodyweightFactor: e.bodyweightFactor.Float(), DerivedFrom: 派生元があれば string(e.derivedFrom)})` で組み直し、`custom`・`deleted` を元から写す
  - `NewRandomExerciseID` を足し、`NewRandomCustomExerciseID` はそれを呼ぶだけにする
  - 「寄与1.0」の定数は `custom_exercise.go` の `primaryContribution` を使う
- [ ] **Step 4: 緑** — `go test ./...`（シードの38種目が全部通ること＝`seed.Exercises()` がエラーにならないこと）
- [ ] **Step 5: 変異** — 寄与1.0の検査を消す → 赤 ／ `Edit` で自重係数を渡し忘れる → 赤 ／ `Edit` で deleted を写し忘れる → 赤
- [ ] **Step 6: コミット** — `feat(exercise): 全種目に寄与1.0の区分と名前の上限を課し、Edit を足す`

### Task 2: インメモリの取得口を「その人の一覧」にする

**Files:**
- Modify: `internal/infrastructure/memory/repositories.go`（`ExerciseRepository`）
- Modify: `internal/infrastructure/memory/repositories_test.go`、`cross_user_test.go`

**Interfaces:**
- Produces: `memory.NewExerciseRepository(seed []*exercise.Exercise)` は変えない。`FindAll(ctx, user)`：その人の map が無ければ（nil なら）seed を全部入れてから、ID 昇順で返す。`Save(ctx, user, e)`：どの種目でも受け付ける（`IsCustom` の検査を外す）。名前の重複（消していない他の ID と同名）は `exercise.ErrDuplicateExerciseName`

- [ ] **Step 1: テストを書き直す**
  - `TestExerciseRepository_SeedsOnFirstRead`：新しい利用者の `FindAll` がシードと同じ件数・同じ ID を返す
  - `TestExerciseRepository_DoesNotReseedAfterDeletingAll`：全部 `Delete()` して保存 → `FindAll` は全件 `IsDeleted()` のまま、件数が増えない
  - `TestExerciseRepository_SavesAnyExercise`：シードの種目を `Edit` して保存 → `FindAll` で直した名前が返る（旧 `RefusesSeedExercises` は消す）
  - `TestExerciseRepository_OrdersByID`：`u-000000000000000b`・`u-000000000000000a` の順に足す → `FindAll` 全体が ID 昇順
  - 既存の `KeepsDeletedCustoms`・`NameIsUniqueAmongAliveCustoms`・`SaveOverwritesTheSameID`・利用者分離は、シードが先に入ることを前提に直す（件数の期待値を `len(seed)+n` に）
- [ ] **Step 2: 赤** → **Step 3: 実装**（`byUser map[account.UserID]map[exercise.ExerciseID]*exercise.Exercise`。`FindAll` は書き込みロックで「無ければ入れる」を行う）→ **Step 4: 緑**（`go test -race ./internal/infrastructure/memory/`）
- [ ] **Step 5: 変異** — 「無ければ入れる」を「空なら入れる（消した行を数えない）」に変える → `DoesNotReseedAfterDeletingAll` が赤 ／ ソートを消す → `OrdersByID` が赤
- [ ] **Step 6: コミット** — `feat(memory): 種目をその人の一覧にし、初回にプリセットを入れる`

### Task 3: Postgres を `user_exercises` にする

**Files:**
- Modify: `internal/infrastructure/postgres/migrations/0013_custom_exercises.sql` → ファイル名を `0013_user_exercises.sql` に変え、中身を設計書の DDL に置き換える（未適用なので書き換えてよい。**本番・共有 DB に 0013 が当たっていないことを確認**：`git log origin/main -- internal/infrastructure/postgres/migrations/` に 0013 が無いこと）
- Modify: `internal/infrastructure/postgres/exercise_repository.go`、`exercise_repository_test.go`、`cross_user_test.go`、`migrate_test.go`（表名）
- Modify: `cmd/api/main.go`（コメントのみ。組み立ては同じ）

**Interfaces:**
- Produces: `postgres.NewExerciseRepository(pool, seed)` は変えない。`FindAll`：`SELECT count(*) ... WHERE user_id=$1` が0なら、シードを1つのトランザクションで `INSERT ... ON CONFLICT (user_id, id) DO NOTHING` → `SELECT ... ORDER BY id COLLATE "C"` を `NewExercise` で組み直し、`deleted_at` があれば `Delete()`。`Save`：全列を upsert（`deleted_at` は最初に消した時刻を保つ `COALESCE`）。部分一意索引 `user_exercises_alive_name` の 23505 → `ErrDuplicateExerciseName`

- [ ] **Step 1: テスト**（Task 2 と同じケース名で揃える）＋ `TestExerciseRepository_Postgres_ConcurrentFirstReadsSeedOnce`：同じ新しい利用者で `FindAll` を2つの goroutine から同時に呼び、どちらもエラー無し・件数はシードと同じ
  ＋ `TestExerciseRepository_Postgres_RoundTripsBodyweightAndDerivedFrom`：シードの `pull_up`（自重0.95）と `close_grip_bench`（派生元 bench）が読み戻しで保たれる
- [ ] **Step 2: 赤** → **Step 3: 実装** → **Step 4: 緑**（スキップ0件を `-v` で確認）
- [ ] **Step 5: 旧版からの移行**：`/tmp/.../scratchpad` に `git worktree add` で origin/main を出し、新しいデータベースに旧版のバイナリで 0012 まで当て、新版で 0013 が当たり `/health` が ok（手順は前回と同じ。docker は使わない）
- [ ] **Step 6: 変異** — `ON CONFLICT DO NOTHING` を外す → 並行テストが赤 ／ `WHERE user_id` を外す → 分離が赤 ／ derived_from を保存しない → 往復が赤
- [ ] **Step 7: コミット** — `feat(postgres): 種目を user_exercises に置き、初回にプリセットを入れる`

### Task 4: ユースケース — 足す・直す・消す

**Files:**
- Rename/Modify: `internal/application/usecase/add_custom_exercise.go` → `add_exercise.go`（型名 `AddExercise`、入力 `AddExerciseInput{Name string; Stimulus map[training.MuscleRegion]float64; IncrementKg float64}`）
- Create: `internal/application/usecase/edit_exercise.go`（`EditExercise`、`Execute(ctx, user, id, EditExerciseInput) (*exercise.Exercise, error)`。入力の形は Add と同じ）
- Rename/Modify: `delete_custom_exercise.go` → `delete_exercise.go`（`DeleteExercise`。`IsCustom` の条件を外す）
- テストも同じく改名・追記

**Interfaces:**
- Consumes: `exercise.NewExercise`・`Edit`・`NewRandomExerciseID`・`ErrDuplicateExerciseName`
- Produces: `usecase.NewAddExercise(exercises, programReader, programWriter)`、`usecase.NewEditExercise(exercises)`、`usecase.NewDeleteExercise(exercises, programReader, programWriter)`

- [ ] **Step 1: テスト**
  - Add：`AddsAndSelects`、重複（プリセットと同名／足した種目と同名／前後空白）、主（1.0）が無いと 400、プログラム未設定で 409
  - Edit：`RenamesAndChangesStimulus`、`KeepsFineContributionsWhenOnlyRenaming`（プリセット相当 0.7・0.4 を持つ種目を、同じ寄与のまま名前だけ変える → 寄与が保たれる）、`RejectsNameOfAnotherAliveExercise`（409）、`AllowsItsOwnName`（同じ名前のまま刻みだけ変える → 通る）、`AllowsNameOfADeletedExercise`、`NotFound`（無い ID・消した種目 → 404）
  - Delete：既存のケースから「共通の種目は 404」を**「プリセット由来も消せる」に反転**。他は維持
- [ ] **Step 2: 赤** → **Step 3: 実装**（Edit の重複チェックは `other.ID() != id && !other.IsDeleted() && other.Name() == edited.Name()`）→ **Step 4: 緑**
- [ ] **Step 5: 変異** — Edit の重複チェックで自分自身を除かない → `AllowsItsOwnName` が赤 ／ Delete に `IsCustom` を戻す → プリセット由来を消すテストが赤
- [ ] **Step 6: コミット** — `feat(usecase): どの種目も足す・直す・消すにする`

### Task 5: API と後片付け

**Files:**
- Modify: `internal/presentation/httpapi/{dto.go,handler.go,router.go,custom_exercise_test.go,handler_test.go,user_internal_test.go}`、`internal/application/query/exercises.go`(+test)、`cmd/api/main.go`
- Delete: `exercise.NewCustomExercise`・`CustomExerciseParams`・`PrimaryRegions`・`SecondaryRegions`・`regionsAt`・`custom` フィールド・`IsCustom`・`NewRandomCustomExerciseID`（呼び手が無くなったもの）と、そのテスト
- Modify: `README.md`（種目の保存の説明）

**Interfaces:**
- API：`GET` の各要素から `custom` を外す（`deleted` は残す）。`POST /api/exercises` 本文 `{"name","stimulus":{"LAT":1,"BICEPS":0.5},"increment_kg"}` → 201。`PUT /api/exercises/{id}` 同じ本文 → 200。`DELETE` → 204

- [ ] **Step 1: テスト** — `TestExercises_AddEditDelete`（POST → PUT で名前と寄与を変える → GET に反映 → DELETE → `deleted: true`）、エラー表（400 寄与1.0なし、409 同名、404 PUT 無い ID、409 伸ばしたい種目を DELETE、**プリセット `side_raise` を DELETE → 204**）。409 の本文の文言が二重にならない既存の検査は残す
- [ ] **Step 2: 赤** → **Step 3: 実装** → **Step 4: 緑**
- [ ] **Step 5: 全体の検収** — `TEST_DATABASE_URL=... go test ./... -count=1`（スキップ0）。**`go test ./internal/domain/training/seed/ -run TestSimulation -v` の出力が #219 のブランチ（`dyoshyy/custom-exercises-reader`）と同じ**（一時 worktree で両方回して diff）
- [ ] **Step 6: 変異** — PUT の経路を消す → 赤 ／ `deleted` を DTO に詰め忘れる → 赤
- [ ] **Step 7: コミット** — `feat(api): 種目を直す PUT を足し、プリセットも消せるようにする`

---

## Part B：画面（`dyoshyy/custom-exercises-web`、#221）

コントローラが Part A の上に載せ直してから始める。初版の `CustomExercises.tsx`・`customExercise.ts` は作り直す。

### Task 6: 判断（純粋関数）

**Files:**
- Rewrite: `web/src/features/exercises/exerciseDraft.ts`（新しい場所。旧 `features/settings/customExercise.ts` は消す）
- Test: `web/src/features/exercises/exerciseDraft.test.ts`
- Modify: `web/src/api/types.ts`（`custom` を外す）

**Interfaces（すべて純粋関数）:**
- `type ExerciseDraft = { name: string; stimulus: Record<string, number>; incrementKg: number }`
- `emptyDraft()`、`draftOf(e: Exercise): ExerciseDraft`（直すときの初期値。寄与はそのまま）
- `cycleRegion(d, region)`：無し → 1.0 → 0.5 → 無し。**1.0・0.5 以外の値（0.7 など）の区分を押したら 1.0 にする**
- `setContribution(d, region, value)`：数値の欄。0.1〜1.0 に丸めず、そのまま入れる（検証は `draftProblem`）
- `draftProblem(d)`：名前空・40 rune 超・寄与1.0の区分が無い・9区分以上・0.1未満または1.0超の寄与・刻みが範囲外
- `draftBody(d)`：`{name: trim, stimulus, increment_kg}`（キーは並べる）
- `aliveExercises(xs)`、`deleteBlockedReason(declared, id)`（既存の意味のまま移す）
- `stimulusSummary(stimulus)`：`大腿四頭筋 1.0・臀筋 0.7・…`（寄与の大きい順、同点は区分名順）

- [ ] テスト → 赤 → 実装 → 緑 → 変異（各関数に1つ以上）→ コミット `feat(web): 種目の編集の判断を足す`

### Task 7: 手順と別ページ

**Files:**
- Create: `web/src/features/exercises/useExerciseManager.ts`（足す POST・直す PUT・消す DELETE。成功したら `onChanged()`（一覧の取り直し）。失敗は `describePutFailure` の文言）
- Create: `web/src/features/exercises/ExerciseManager.tsx`（部位ごとに1行ずつ：名前・`stimulusSummary`・「直す」「消す」。先頭に「種目を足す」）、`ExerciseEditor.tsx`（名前・部位チップ・選んだ区分の数値欄・刻み。足すと直すで共用）
- Modify: `web/src/app/route.ts`（`'exercises'` を足す。`isRoute` も）、`App.tsx`（`route === 'exercises'` で `<ExerciseManager ... onBack={() => go('settings')} />`）
- Modify: `web/src/features/settings/ProgramSettings.tsx`（初版で足した「自分の種目」を消し、「種目」の節に「種目を管理する」ボタン → `go('exercises')`。`SettingsScreen` に `onOpenExercises` を通す）
- Modify: `web/src/features/settings/useProgramSettings.ts`（初版で足した add/delete を消す。`request` の汎用化は残してよい）
- Modify: `web/scripts/settings-check.mjs`（初版の「足す→消す」を新しいページで行うように直す：設定 → 種目を管理する → 足す → 一覧に出る → 直す（名前を変える）→ 変わった名前で出る → 消す → 消える。プリセット `サイドレイズ` を消して、設定の「使う種目」から消えることも見る。最後に消した分は API で足し直さなくてよい（インメモリなので再起動で戻る））

- [ ] 判断のテストが緑のまま、`tsc`・`vitest` 緑
- [ ] コントローラが API（インメモリ）と preview を立てて `settings`・`nav`・`a11y`・`ui` の check を回す。足した後の取り直しを抜く変異で `settings-check` が落ちることを確認
- [ ] コミット `feat(web): 種目を別ページで足す・直す・消す`

---

## Part C：開発用シミュレーション（`dyoshyy/custom-exercises-devsim`、#224）

### Task 8: `custom=` を数値の効き方に揃える

**Files:**
- Modify: `internal/application/devsim/simulator.go`（`CustomExercise{Name; Stimulus map[training.MuscleRegion]float64; IncrementKg}`、`poolFor` は `exercise.NewExercise` で組む）、`simulator_test.go`
- Modify: `internal/presentation/httpapi/dev_simulation.go`（書式を `名前|区分:寄与,区分:寄与|刻み` を `;` で並べる形に。echo は `{id, name, stimulus, increment_kg}`）、`dev_simulation_test.go`
- Modify: `web/src/dev/{simulate.ts,summary.ts,DevSimulation.tsx}` と各テスト（型・プレースホルダ・要約 `名前（広背筋 1.0・上腕二頭筋 0.5）`）
- Modify: 設計書の付録の書き方に合わせた例

- [ ] テスト → 赤 → 実装 → 緑 → 変異 → コミット `feat(devsim): 自分の種目を寄与の数値で渡す`
