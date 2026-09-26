# 自分の種目（サーバー側）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 利用者ごとの種目を API から足し、論理削除で消せるようにする（設計書の PR 2）。

**Architecture:** 種目の取得口 `exercise.Reader.FindAll` に利用者を渡し、「共通の一覧＋その人の種目（消したものを含む）」を返させる。共通と自分の種目を合わせる場所は取得口の1箇所だけにし、使う側はスライスを1つ受け取るまま。計画の導出（`planning`）には手を入れない。

**Tech Stack:** Go 1.2x 標準ライブラリ、pgx v5、Postgres（`TEST_DATABASE_URL`）。

**Spec:** `docs/specs/2026-09-26-custom-exercises-design.md`

## 前提と進め方

- ブランチ：`dyoshyy/custom-exercises`（設計書のコミットが載っている）。着手前に `git rebase origin/main`
- **PR を2本に分ける**（CLAUDE.md「動作の変更と、機械的な移動は混ぜない」）
  - **PR 2a（Task 1）**：`FindAll(ctx)` → `FindAll(ctx, user)`。動作は変わらない。「全テストが緑のまま」が検収
  - **PR 2b（Task 2〜9）**：足す／消すの動作
- 各タスクは CLAUDE.md の手順で進める：テストを書く → 期待した理由で赤 → 実装 → 緑 → **変異を入れて赤くなるか手で確かめる**（`.claude/skills/writing-tests/` を読むこと）
- 検収コマンド（毎タスクの最後）：

```bash
gofmt -l .        # 何も出ないこと
go vet ./...
go test ./...
```

- Postgres のテストは `TEST_DATABASE_URL` が無いとスキップされる。Task 5 からは必ず立てて回す：

```bash
docker run -d --name liftplan-testdb -e POSTGRES_PASSWORD=x -p 5432:5432 postgres:17-alpine
export TEST_DATABASE_URL=postgres://postgres:x@127.0.0.1:5432/postgres
```

## Global Constraints

- 寄与は「主に効く＝1.0」「少し効く＝0.5」に固定。数値は入力させない
- 主に効く部位は1つ以上。主と少しは重ならない。合計8区分まで（`maxStimulusRegions`）
- 自分の種目は `BodyweightFactor` 0、`DerivedFrom` なし
- 自分の種目の ID は `u-` ＋ 16進16文字。サーバーが採番する
- 名前は前後の空白を落として1〜40文字（rune 数）
- 名前の重複は「共通の種目」「自分のまだ消していない種目」と比べる。消した種目と同じ名前は通す
- 足すと「使う種目」（`Program.selected`）に入る
- 消すのは論理削除。消すと「使う種目」から外れる。伸ばしたい種目に入っていれば 409
- 共通の種目・他人の種目・存在しない ID を消そうとしたら 404
- 消した種目は選べない（`verifySelection`）。消した種目の記録は受け付ける
- 週目標は共通の一覧だけから計算する（`seed.averageStimulusPerSet` は触らない）
- 値オブジェクト・エラーの `%w`・doc コメントは宣言名で始める（CLAUDE.md「Go 固有の決めごと」）

## Review Focus

1. **消す途中で落ちたとき**：どこで落ちても「種目は消えたのに使う種目に残っている」状態にならず、もう一度 DELETE すれば 204 で終わること → Task 8 のテスト「途中で落ちても壊れた状態を残さない」
2. **他人の種目 ID を知っている利用者**：DELETE も POST 後の GET も、他人の種目に触れられないこと → Task 4・5 の利用者分離のテストと、Task 8 の「他人の種目は 404」
3. **消した種目と同じ名前で足し直す**：通ること（DB の部分一意索引も通すこと） → Task 5 と Task 7 のテスト
4. **消した種目が残った画面から記録を送る**：記録は受け付け、履歴に名前が出ること → Task 8 のテスト「消した種目の記録と名前は残る」
5. **消した種目を選択に戻そうとする**（古い画面から `PUT /api/program/selected`）：400 になること → Task 6 のテスト

---

## ファイルの地図

| ファイル | 役割 | Task |
|---|---|---|
| `internal/domain/training/exercise/exercise_repository.go` | `Reader`（利用者つき）・`Writer`・センチネル | 1, 3 |
| `internal/domain/training/exercise/custom_exercise.go`（新規） | `NewCustomExercise`・`NewRandomCustomExerciseID`・寄与の固定値 | 2 |
| `internal/domain/training/exercise/exercise.go` | `custom`・`deleted` フィールド、`IsCustom`・`IsDeleted`・`Delete` | 2 |
| `internal/infrastructure/memory/repositories.go` | `ExerciseRepository` を利用者ごとに | 4 |
| `internal/infrastructure/postgres/exercise_repository.go`（新規） | 共通＋`custom_exercises` | 5 |
| `internal/infrastructure/postgres/migrations/0013_custom_exercises.sql`（新規） | 表 | 5 |
| `internal/application/usecase/verify_selection.go` | 消した種目を弾く | 6 |
| `internal/application/usecase/add_custom_exercise.go`（新規） | 足す | 7 |
| `internal/application/usecase/delete_custom_exercise.go`（新規） | 消す | 8 |
| `internal/application/apperror/apperror.go` | 新しい分類3つ | 7, 8 |
| `internal/application/query/exercises.go` | `Custom`・`Deleted` を返す | 9 |
| `internal/presentation/httpapi/{handler.go,read.go,dto.go,router.go}` | POST・DELETE | 9 |
| `cmd/api/main.go` | 組み立て | 5, 9 |

---

## PR 2a

### Task 1: 種目の取得口に利用者を渡す（機械的）

**Files:**
- Modify: `internal/domain/training/exercise/exercise_repository.go`
- Modify: `internal/domain/training/exercise/repository_test.go`
- Modify: `internal/infrastructure/memory/repositories.go:20-36`
- Modify（呼び出し側）: `internal/application/query/{exercises,history,stats}.go`、`internal/application/usecase/{get_session,record_sets,set_declared_exercises,set_selected_exercises,set_split_cycle,sign_in}.go`、`internal/presentation/httpapi/read.go`（`handleGetExercises`）
- Test: 既存のテストをそのまま使う（呼び出しのシグネチャだけ直す）

**Interfaces:**
- Produces: `exercise.Reader.FindAll(ctx context.Context, user account.UserID) ([]*exercise.Exercise, error)`、`query.Exercises.All(ctx context.Context, user account.UserID) ([]query.Exercise, error)`

- [ ] **Step 1: 形のテストを先に直す**

`repository_test.go` の `stubRepo` と `TestReader_KeepsItsShape` を新しい形にする。

```go
func (stubRepo) FindAll(context.Context, account.UserID) ([]*exercise.Exercise, error) {
	return nil, nil
}

func TestReader_KeepsItsShape(t *testing.T) {
	var r exercise.Reader = stubRepo{}
	var _ func(context.Context, account.UserID) ([]*exercise.Exercise, error) = r.FindAll
}
```

- [ ] **Step 2: 赤を確認する**

Run: `go test ./internal/domain/training/exercise/`
Expected: コンパイルエラー（`stubRepo does not implement exercise.Reader (wrong type for method FindAll)`）。

- [ ] **Step 3: インターフェースを変える**

```go
// Reader は種目の取得口。
//
// 共通の種目（シード）に、その利用者が足した種目を加えて返す。所有者を
// 引数で受けるのは program.Reader と同じ理由で、他人の種目が見えては
// いけない。
//
// FindAll が返すスライスと要素は、呼び出し側が自由に扱ってよい。
// リポジトリ内部の可変状態をエイリアスして返してはならない。
type Reader interface {
	FindAll(ctx context.Context, user account.UserID) ([]*Exercise, error)
}
```

`exercise` パッケージが `internal/domain/account` を import するのは、`program` が既にしているのと同じ向き（ドメイン内）なので `architecture_test.go` は通る。

- [ ] **Step 4: インメモリ実装と呼び出し側を直す**

`memory.ExerciseRepository.FindAll(context.Context, account.UserID)`。この段階では利用者を使わない（シードだけを返す）。

呼び出し側は、手元にある `user` を渡すだけ。`query.Exercises.All` は `user` を受け取るようにし、`handleGetExercises` の先頭に他のハンドラと同じ形を足す：

```go
user, ok := requireUser(w, r)
if !ok {
	return
}
items, err := h.exercises.All(r.Context(), user)
```

`sign_in.go` の `seedProgramIfMissing` は `userID` を渡す。

- [ ] **Step 5: 全体を回す**

Run: `gofmt -l . ; go vet ./... ; go test ./...`
Expected: 全部緑。**テストの中身（期待値）は1つも変えていないこと**を `git diff -- '*_test.go'` で確かめる（シグネチャの行だけのはず）。

`GET /api/exercises` がログイン必須になるので、`read_test.go` の `TestGetExercises_ReturnsJapaneseNames` が 401 で落ちる場合は、他の読み出しのテストと同じ認証つきの `do` の呼び方に揃える（期待値は変えない）。画面（`useLiftplan.ts`）はログイン後にしか呼ばないので影響しない。

- [ ] **Step 6: コミットして PR 2a を出す**

```bash
git add -A
git commit -m "refactor(exercise): 種目の取得口に利用者を渡す

自分の種目（docs/specs/2026-09-26-custom-exercises-design.md）の準備。
共通の一覧とその人の種目を合わせる場所を取得口の1箇所にするため、
FindAll が所有者を受け取るようにする。いまは利用者を使わず、動作は
変わらない。"
```

PR 本文には「動作の変更なし。全テストが緑のままが検収」と書く。

---

## PR 2b

### Task 2: 自分の種目をドメインで作る

**Files:**
- Create: `internal/domain/training/exercise/custom_exercise.go`
- Create: `internal/domain/training/exercise/custom_exercise_test.go`
- Modify: `internal/domain/training/exercise/exercise.go`（フィールドとメソッド）
- Modify: `internal/domain/training/seed/seed_test.go`（ID の接頭辞のテスト）

**Interfaces:**
- Produces:
  - `exercise.CustomExerciseParams{ID ExerciseID; Name string; Primary, Secondary []training.MuscleRegion; IncrementKg float64}`
  - `exercise.NewCustomExercise(p CustomExerciseParams) (*Exercise, error)`
  - `exercise.NewRandomCustomExerciseID() (ExerciseID, error)`
  - `exercise.CustomExerciseIDPrefix = "u-"`
  - `(*Exercise).IsCustom() bool`、`(*Exercise).IsDeleted() bool`、`(*Exercise).Delete() *Exercise`
  - `(*Exercise).PrimaryRegions() []training.MuscleRegion`、`(*Exercise).SecondaryRegions() []training.MuscleRegion`（保存に使う。寄与1.0の区分と0.5の区分をソートして返す）

- [ ] **Step 1: テストを書く**

```go
package exercise_test

import (
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

func validCustom() exercise.CustomExerciseParams {
	return exercise.CustomExerciseParams{
		ID:          "u-0123456789abcdef",
		Name:        "アイソラテラル・ロー",
		Primary:     []training.MuscleRegion{training.TrapMid},
		Secondary:   []training.MuscleRegion{training.Lat, training.Biceps},
		IncrementKg: 2.5,
	}
}

// 主は1.0、少しは0.5になること。本人に数値を入れさせない代わりに、
// 対応はここで固定する。
func TestNewCustomExercise_MapsPrimaryAndSecondaryToFixedContributions(t *testing.T) {
	e, err := exercise.NewCustomExercise(validCustom())
	if err != nil {
		t.Fatal(err)
	}
	want := map[training.MuscleRegion]float64{
		training.TrapMid: 1.0, training.Lat: 0.5, training.Biceps: 0.5,
	}
	for r, w := range want {
		c, ok := e.Stimulus().Contribution(r)
		if !ok || c.Float() != w {
			t.Errorf("%s の寄与が %v（期待 %v）", r, c.Float(), w)
		}
	}
	if got := len(e.Stimulus().Regions()); got != len(want) {
		t.Errorf("寄与する区分が %d 個（期待 %d）", got, len(want))
	}
	if !e.IsCustom() || e.IsDeleted() {
		t.Errorf("custom=%v deleted=%v（期待 true, false）", e.IsCustom(), e.IsDeleted())
	}
	if _, ok := e.DerivedFrom(); ok || e.BodyweightFactor().Float() != 0 {
		t.Error("自分の種目に派生元か自重係数が入っている")
	}
}

func TestNewCustomExercise_Rejects(t *testing.T) {
	nine := training.AllMuscleRegions()[:9]
	cases := []struct {
		name   string
		modify func(*exercise.CustomExerciseParams)
	}{
		{"主が無い", func(p *exercise.CustomExerciseParams) { p.Primary = nil }},
		{"主と少しが重なる", func(p *exercise.CustomExerciseParams) {
			p.Secondary = []training.MuscleRegion{training.TrapMid}
		}},
		{"主の中で重なる", func(p *exercise.CustomExerciseParams) {
			p.Primary = []training.MuscleRegion{training.TrapMid, training.TrapMid}
		}},
		{"9区分", func(p *exercise.CustomExerciseParams) {
			p.Primary, p.Secondary = nine[:1], nine[1:]
		}},
		{"知らない区分", func(p *exercise.CustomExerciseParams) {
			p.Primary = []training.MuscleRegion{"NECK"}
		}},
		{"名前が空", func(p *exercise.CustomExerciseParams) { p.Name = "  " }},
		{"名前が41文字", func(p *exercise.CustomExerciseParams) { p.Name = strings.Repeat("あ", 41) }},
		{"刻みが0", func(p *exercise.CustomExerciseParams) { p.IncrementKg = 0 }},
		{"IDの接頭辞が違う", func(p *exercise.CustomExerciseParams) { p.ID = "bench" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validCustom()
			c.modify(&p)
			if _, err := exercise.NewCustomExercise(p); err == nil {
				t.Error("通ってしまった")
			}
		})
	}
}

// 40文字ちょうどは通ること（上の41文字と対で境界を固定する）。
func TestNewCustomExercise_AcceptsFortyRuneName(t *testing.T) {
	p := validCustom()
	p.Name = strings.Repeat("あ", 40)
	if _, err := exercise.NewCustomExercise(p); err != nil {
		t.Fatal(err)
	}
}

// Delete は元を変えず、消した状態の新しい値を返すこと。
func TestExercise_DeleteReturnsANewValue(t *testing.T) {
	e, _ := exercise.NewCustomExercise(validCustom())
	d := e.Delete()
	if e.IsDeleted() {
		t.Error("元が消えた状態になった")
	}
	if !d.IsDeleted() || !d.IsCustom() || d.ID() != e.ID() || d.Name() != e.Name() {
		t.Errorf("消した値が崩れている: %+v", d)
	}
}

// 保存用に主と少しを取り出せること。読み戻しで同じ種目になる。
func TestExercise_PrimaryAndSecondaryRoundTrip(t *testing.T) {
	p := validCustom()
	e, _ := exercise.NewCustomExercise(p)
	back, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: string(e.ID()), Name: e.Name(),
		Primary: e.PrimaryRegions(), Secondary: e.SecondaryRegions(),
		IncrementKg: e.Increment().Kg(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range e.Stimulus().Regions() {
		a, _ := e.Stimulus().Contribution(r)
		b, _ := back.Stimulus().Contribution(r)
		if a != b {
			t.Errorf("%s: %v と %v", r, a.Float(), b.Float())
		}
	}
}

func TestNewRandomCustomExerciseID_HasThePrefixAndIsValid(t *testing.T) {
	id, err := exercise.NewRandomCustomExerciseID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(id), exercise.CustomExerciseIDPrefix) || len(id) != 18 {
		t.Errorf("ID の形が違う: %q", id)
	}
	other, _ := exercise.NewRandomCustomExerciseID()
	if other == id {
		t.Error("2回採番して同じ ID になった")
	}
}
```

`CustomExerciseParams.ID` は `string` にする（`ExerciseParams` と同じ）。上のテストの `ID: string(e.ID())` はそのため。

`seed_test.go` に足す：

```go
// シードの ID は自分の種目の接頭辞で始まらないこと。
//
// 自分の種目の ID はサーバーが "u-" ＋乱数で採番する。シードに "u-" で
// 始まる ID を置くと、いつか採番した ID とぶつかる。実行時に採番し直す
// 仕組みを持たない代わりに、ここで守る。
func TestExercises_NoIDUsesTheCustomPrefix(t *testing.T) {
	all, _ := seed.Exercises()
	for _, e := range all {
		if strings.HasPrefix(string(e.ID()), exercise.CustomExerciseIDPrefix) {
			t.Errorf("%s が自分の種目の接頭辞で始まっている", e.ID())
		}
	}
}
```

- [ ] **Step 2: 赤を確認する**

Run: `go test ./internal/domain/training/exercise/ ./internal/domain/training/seed/`
Expected: コンパイルエラー（`undefined: exercise.CustomExerciseParams` など）。

- [ ] **Step 3: 実装する**

`exercise.go` の `Exercise` に2つ足す：

```go
	// custom はその利用者が足した種目か。共通の種目（シード）は false。
	custom bool
	// deleted は消したか。消した種目も記録と履歴の名前のために残る。
	deleted bool
```

```go
// IsCustom は利用者が足した種目かを返す。共通の種目を消させない判定に使う。
func (e *Exercise) IsCustom() bool { return e.custom }

// IsDeleted は消した種目かを返す。
func (e *Exercise) IsDeleted() bool { return e.deleted }

// Delete は消した状態の新しい種目を返す。元は変えない。
func (e *Exercise) Delete() *Exercise {
	c := *e
	c.deleted = true
	return &c
}

// PrimaryRegions は寄与1.0の区分をソートして返す。自分の種目の保存に使う。
func (e *Exercise) PrimaryRegions() []training.MuscleRegion {
	return e.regionsAt(primaryContribution)
}

// SecondaryRegions は寄与0.5の区分をソートして返す。自分の種目の保存に使う。
func (e *Exercise) SecondaryRegions() []training.MuscleRegion {
	return e.regionsAt(secondaryContribution)
}

func (e *Exercise) regionsAt(v float64) []training.MuscleRegion {
	// nil ではなく空で始める。保存（jsonb）で null ではなく [] にするため。
	out := []training.MuscleRegion{}
	for _, r := range e.stimulus.Regions() {
		if c, ok := e.stimulus.Contribution(r); ok && c.Float() == v {
			out = append(out, r)
		}
	}
	return out
}
```

`Delete` の値コピーは `stimulus` の内部 map を共有するが、`StimulusProfile` はパッケージ外から不変で、`deleted` 以外を書き換えないので問題ない（`TestDomain_StimulusProfileIsNotMutated` の対象外の書き込みは無い）。

`custom_exercise.go`：

```go
package exercise

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// CustomExerciseIDPrefix は利用者が足した種目の ID の接頭辞。
//
// シードの ID は英小文字と "_" だけなので、この接頭辞とは衝突しない
// （seed の TestExercises_NoIDUsesTheCustomPrefix が守る）。
const CustomExerciseIDPrefix = "u-"

// 寄与の固定値。本人には「主に効く」「少し効く」しか選ばせない。
//
// 数値を入れさせないのは、1.0と0.5の違いを本人は答えられないから
// （CLAUDE.md「決めることを増やさない」）。0.5 はシードの副次寄与
// （0.3〜0.6）の真ん中に置いた。
const (
	primaryContribution   = 1.0
	secondaryContribution = 0.5
)

// maxCustomNameRunes は自分の種目の名前の上限。
//
// シードの最長は「デフィシットデッドリフト」の12文字。Hammer Strength の
// 機種名でも20文字に届かない。エラー文や画面に出るので上限を置く。
const maxCustomNameRunes = 40

// CustomExerciseParams は利用者が足す種目の生成入力。
type CustomExerciseParams struct {
	ID          string
	Name        string
	Primary     []training.MuscleRegion
	Secondary   []training.MuscleRegion
	IncrementKg float64
}

// NewCustomExercise は利用者が足す種目を組み立てる。
//
// 主に効く区分を1つ以上要求する。寄与1.0の区分が無い種目は、分割の
// どの日にも入らない（planning の isPrimaryIn）。
func NewCustomExercise(p CustomExerciseParams) (*Exercise, error) {
	if !strings.HasPrefix(p.ID, CustomExerciseIDPrefix) {
		return nil, fmt.Errorf("自分の種目の ID は %q で始まる必要がある: %q", CustomExerciseIDPrefix, p.ID)
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(p.Name)); n > maxCustomNameRunes {
		return nil, fmt.Errorf("種目の名前が長すぎる: %d文字（上限 %d）", n, maxCustomNameRunes)
	}
	if len(p.Primary) == 0 {
		return nil, errors.New("主に効く部位を1つ以上選ぶ必要がある")
	}

	stimulus := make(map[training.MuscleRegion]float64, len(p.Primary)+len(p.Secondary))
	put := func(rs []training.MuscleRegion, v float64) error {
		for _, r := range rs {
			if _, dup := stimulus[r]; dup {
				return fmt.Errorf("部位 %s が2回選ばれている", r)
			}
			stimulus[r] = v
		}
		return nil
	}
	if err := put(p.Primary, primaryContribution); err != nil {
		return nil, err
	}
	if err := put(p.Secondary, secondaryContribution); err != nil {
		return nil, err
	}

	e, err := NewExercise(ExerciseParams{
		ID: p.ID, Name: p.Name, Stimulus: stimulus, IncrementKg: p.IncrementKg,
	})
	if err != nil {
		return nil, err
	}
	e.custom = true
	return e, nil
}

// NewRandomCustomExerciseID は自分の種目の ID を採番する。
//
// サーバーが採番するのは、種目を足すのが設定画面で、圏外で足す必要が
// 無いから。二度押しは名前の重複で止まる。
func NewRandomCustomExerciseID() (ExerciseID, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("乱数を取得できない: %w", err)
	}
	return ExerciseID(CustomExerciseIDPrefix + hex.EncodeToString(b[:])), nil
}
```

区分の妥当性（知らない区分）と8区分の上限は `NewExercise` → `NewStimulusProfile` が見る。二重に書かない。

- [ ] **Step 4: 緑を確認する**

Run: `go test ./internal/domain/training/...`
Expected: PASS。`architecture_test.go` も通ること（`crypto/rand` は標準ライブラリなので許される。D-118）。

- [ ] **Step 5: 変異を入れて赤を確かめる**

1つずつ入れて、該当テストが赤になるのを見てから戻す：
- `secondaryContribution = 0.6` → `MapsPrimaryAndSecondary…` が赤
- `if len(p.Primary) == 0` の分岐を消す → `Rejects/主が無い` が赤
- `put` の `dup` の検査を消す → `主と少しが重なる`・`主の中で重なる` が赤
- `n > maxCustomNameRunes` を `n > maxCustomNameRunes+1` → `名前が41文字` が赤
- `Delete` で `c := *e` をやめて `e.deleted = true; return e` → `DeleteReturnsANewValue` が赤

- [ ] **Step 6: コミット**

```bash
git add internal/domain/training/exercise internal/domain/training/seed/seed_test.go
git commit -m "feat(exercise): 利用者が足す種目をドメインで組み立てる"
```

### Task 3: 書き口（Writer）とセンチネルを足す

**Files:**
- Modify: `internal/domain/training/exercise/exercise_repository.go`
- Modify: `internal/domain/training/exercise/doc.go`
- Modify: `internal/domain/training/exercise/repository_test.go`

**Interfaces:**
- Produces:
  - `exercise.Writer{ Save(ctx context.Context, user account.UserID, e *Exercise) error }`
  - `exercise.ErrDuplicateExerciseName`（リポジトリが DB の一意制約違反をこれに写す）

- [ ] **Step 1: 形のテストを直す**

`TestExercise_HasNoWriter` は「Writer を持たない」を守っていたテスト。必要になったので、同じ観点（メソッド集合を固定する）で Writer 側に書き換える：

```go
// Writer を満たす最小実装。
type stubWriter struct{}

func (stubWriter) Save(context.Context, account.UserID, *exercise.Exercise) error { return nil }

func TestWriter_KeepsItsShape(t *testing.T) {
	var w exercise.Writer = stubWriter{}
	var _ func(context.Context, account.UserID, *exercise.Exercise) error = w.Save
}

// 読みと書きは分けたまま。1つの口にまとめると、使う側が要らない半分まで
// 受け取る（CLAUDE.md「リポジトリのインターフェースは読みと書きに分ける」）。
func TestReaderAndWriter_StaySplit(t *testing.T) {
	for typ, want := range map[reflect.Type][]string{
		reflect.TypeOf((*exercise.Reader)(nil)).Elem(): {"FindAll"},
		reflect.TypeOf((*exercise.Writer)(nil)).Elem(): {"Save"},
	} {
		var got []string
		for i := range typ.NumMethod() {
			got = append(got, typ.Method(i).Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s のメソッド集合が %v（期待 %v）", typ, got, want)
		}
	}
}
```

- [ ] **Step 2: 赤を確認する**

Run: `go test ./internal/domain/training/exercise/`
Expected: `undefined: exercise.Writer`

- [ ] **Step 3: 実装する**

```go
// ErrDuplicateExerciseName は同じ名前の種目が既にあることを表す。
//
// 比べる相手は、共通の種目と、その利用者のまだ消していない種目。
// 一覧で見分けられなくなるのと、二度押しで同じ種目が2つできるのを止める。
var ErrDuplicateExerciseName = errors.New("同じ名前の種目がある")

// Writer は利用者が足した種目の保存口。
//
// 共通の種目（IsCustom が false）は渡さない。共通の種目はバイナリ同梱で、
// 実行時に書き換わらない。渡されたら実装はエラーを返す。
//
// 同じ ID を二度渡したら上書きする（消すときは Delete した値を渡す）。
// 所有者を引数で受ける理由は Reader と同じ。
type Writer interface {
	Save(ctx context.Context, user account.UserID, e *Exercise) error
}
```

`Reader` のコメントにあった「書き手が居ないので Writer は無い。必要になってから足す」は消す。`doc.go` も「シードから流し込まれたあとは実行時に書き換わらない。だから取得口しか持たない」を、「共通の種目はシード、利用者が足した種目は Writer で保存する」に書き換える。

- [ ] **Step 4: 緑を確認して、変異**

Run: `go test ./internal/domain/training/exercise/`
変異：`Writer` に `Delete(...)` を足す → `StaySplit` が赤。戻す。

- [ ] **Step 5: コミット**

```bash
git commit -am "feat(exercise): 利用者が足した種目の書き口を足す"
```

### Task 4: インメモリの取得口を利用者ごとにする

**Files:**
- Modify: `internal/infrastructure/memory/repositories.go:20-36`
- Modify: `internal/infrastructure/memory/repositories_test.go`
- Modify: `internal/infrastructure/memory/cross_user_test.go`（あれば。無ければ `repositories_test.go` に書く）

**Interfaces:**
- Consumes: `exercise.Writer`、`(*Exercise).IsCustom`
- Produces: `(*memory.ExerciseRepository).Save(ctx, user, e) error`。`FindAll` はシード（生成時の順）の後ろに、その利用者の種目を ID 昇順で並べて返す

- [ ] **Step 1: テストを書く**

```go
func mustCustom(t *testing.T, id, name string) *exercise.Exercise {
	t.Helper()
	e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: id, Name: name,
		Primary: []training.MuscleRegion{training.Lat}, IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestExerciseRepository_ReturnsSeedPlusOwnCustoms(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := memory.NewExerciseRepository(seedAll)
	a, b := newUser(t), newUser(t)

	mine := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, mine); err != nil {
		t.Fatal(err)
	}

	gotA, _ := repo.FindAll(ctx, a)
	if len(gotA) != len(seedAll)+1 || gotA[len(gotA)-1].ID() != mine.ID() {
		t.Errorf("A の一覧に自分の種目が末尾に1件足されていない（%d 件）", len(gotA))
	}
	gotB, _ := repo.FindAll(ctx, b)
	if len(gotB) != len(seedAll) {
		t.Errorf("B に A の種目が見えている（%d 件）", len(gotB))
	}
}

func TestExerciseRepository_KeepsDeletedCustoms(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewExerciseRepository(nil)
	a := newUser(t)
	e := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	_ = repo.Save(ctx, a, e)
	_ = repo.Save(ctx, a, e.Delete())

	got, _ := repo.FindAll(ctx, a)
	if len(got) != 1 || !got[0].IsDeleted() {
		t.Errorf("消した種目が消えた状態で1件残っていない: %v", got)
	}
}

func TestExerciseRepository_RefusesSeedExercises(t *testing.T) {
	seedAll, _ := seed.Exercises()
	repo := memory.NewExerciseRepository(seedAll)
	if err := repo.Save(context.Background(), newUser(t), seedAll[0]); err == nil {
		t.Error("共通の種目を保存できてしまった")
	}
}

// 消していない同じ名前は弾き、消した種目と同じ名前は通す（DB の部分一意
// 索引と同じふるまい）。
func TestExerciseRepository_NameIsUniqueAmongAliveCustoms(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewExerciseRepository(nil)
	a := newUser(t)
	first := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	_ = repo.Save(ctx, a, first)

	dup := mustCustom(t, "u-000000000000000b", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, dup); !errors.Is(err, exercise.ErrDuplicateExerciseName) {
		t.Errorf("同名が通った: %v", err)
	}
	_ = repo.Save(ctx, a, first.Delete())
	if err := repo.Save(ctx, a, dup); err != nil {
		t.Errorf("消した種目と同名が弾かれた: %v", err)
	}
}
```

`newUser(t)` は `account.NewRandomUserID()` を包むヘルパ。同じパッケージのテストに似たものがあれば使う。

- [ ] **Step 2: 赤を確認する**

Run: `go test ./internal/infrastructure/memory/`
Expected: `repo.Save undefined`

- [ ] **Step 3: 実装する**

```go
// ExerciseRepository は種目を保持する。共通の種目は起動時にシードを
// 流し込み、利用者が足した種目は利用者ごとの map に持つ。
type ExerciseRepository struct {
	mu     sync.RWMutex
	seed   []*exercise.Exercise
	byUser map[account.UserID]map[exercise.ExerciseID]*exercise.Exercise
}

func NewExerciseRepository(seed []*exercise.Exercise) *ExerciseRepository {
	copied := make([]*exercise.Exercise, len(seed))
	copy(copied, seed)
	return &ExerciseRepository{
		seed:   copied,
		byUser: map[account.UserID]map[exercise.ExerciseID]*exercise.Exercise{},
	}
}

func (r *ExerciseRepository) FindAll(_ context.Context, user account.UserID) ([]*exercise.Exercise, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mine := r.byUser[user]
	out := make([]*exercise.Exercise, 0, len(r.seed)+len(mine))
	out = append(out, r.seed...)
	ids := make([]exercise.ExerciseID, 0, len(mine))
	for id := range mine {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		out = append(out, mine[id])
	}
	return out, nil
}

// Save は利用者が足した種目を保存する。同じ ID は上書きする。
func (r *ExerciseRepository) Save(_ context.Context, user account.UserID, e *exercise.Exercise) error {
	if e == nil || !e.IsCustom() {
		return errors.New("共通の種目は保存できない")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	mine := r.byUser[user]
	if mine == nil {
		mine = map[exercise.ExerciseID]*exercise.Exercise{}
		r.byUser[user] = mine
	}
	if !e.IsDeleted() {
		for id, other := range mine {
			if id != e.ID() && !other.IsDeleted() && other.Name() == e.Name() {
				return fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, e.Name())
			}
		}
	}
	mine[e.ID()] = e
	return nil
}
```

`*Exercise` は外から不変（`Delete` も新しい値を返す）なので、ポインタをそのまま返してエイリアスしても壊れない。

- [ ] **Step 4: 緑と変異**

Run: `go test ./internal/infrastructure/memory/ ./...`
変異：`FindAll` で `mine := r.byUser[user]` を全利用者の和にする → `ReturnsSeedPlusOwnCustoms` が赤。`!other.IsDeleted()` を消す → `NameIsUniqueAmongAliveCustoms` の後半が赤。

- [ ] **Step 5: コミット**

```bash
git commit -am "feat(memory): 種目の取得口に利用者が足した種目を加える"
```

### Task 5: Postgres に `custom_exercises` を置く

**Files:**
- Create: `internal/infrastructure/postgres/migrations/0013_custom_exercises.sql`
- Create: `internal/infrastructure/postgres/exercise_repository.go`
- Create: `internal/infrastructure/postgres/exercise_repository_test.go`
- Modify: `internal/infrastructure/postgres/migrate_test.go`（表の一覧と、掃除の DROP に `custom_exercises` を足す）
- Modify: `internal/infrastructure/postgres/cross_user_test.go`（種目の分離を1ケース足す）
- Modify: `cmd/api/main.go`（`openRepositories`、`repositories.exercises` の型、コメント）

**Interfaces:**
- Consumes: `exercise.Reader`・`exercise.Writer`・`NewCustomExercise`・`PrimaryRegions`/`SecondaryRegions`・`ErrDuplicateExerciseName`
- Produces: `postgres.NewExerciseRepository(pool *pgxpool.Pool, seed []*exercise.Exercise) *postgres.ExerciseRepository`

- [ ] **Step 1: マイグレーションを書く**

```sql
-- 利用者が足した種目。
--
-- 共通の種目（シード）はバイナリ同梱のまま DB には置かない。ここに入るのは
-- 利用者ごとの種目だけで、読み出すときにシードと合わせる
-- （docs/specs/2026-09-26-custom-exercises-design.md）。
--
-- 寄与の数値ではなく「主に効く」「少し効く」の区分を持つ。1.0と0.5の対応は
-- ドメイン（exercise.NewCustomExercise）が持ち、ここに漏らさない。
--
-- user_id に外部キーを張らないのは set_logs などと同じ（利用者の表が無く、
-- accounts の user_id は一意でない。0008）。
--
-- 消すのは論理削除。記録（set_logs.exercise_id）が種目を ID で指していて、
-- 行を消すと履歴から名前が消える。
CREATE TABLE custom_exercises (
    user_id           uuid        NOT NULL,
    id                text        NOT NULL,
    name              text        NOT NULL,
    primary_regions   jsonb       NOT NULL,
    secondary_regions jsonb       NOT NULL,
    increment_kg      numeric     NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    deleted_at        timestamptz,
    PRIMARY KEY (user_id, id)
);

-- 消していない種目の中で名前を一意にする。アプリでも弾くが、同時に2つ
-- 足したときの最後の砦。消した種目と同じ名前で足し直すのは許す。
CREATE UNIQUE INDEX custom_exercises_alive_name
    ON custom_exercises (user_id, name) WHERE deleted_at IS NULL;
```

- [ ] **Step 2: リポジトリのテストを書く**

`exercise_repository_test.go`（`newTestDB(t)` と `userA`/`userB` を使う）：

```go
func TestExerciseRepository_Postgres_SavesAndReadsBack(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := postgres.NewExerciseRepository(migratedDB(t), seedAll)

	e, _ := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: "u-000000000000000a", Name: "アイソラテラル・ロー",
		Primary:   []training.MuscleRegion{training.TrapMid},
		Secondary: []training.MuscleRegion{training.Lat, training.Biceps},
		IncrementKg: 2.5,
	})
	if err := repo.Save(ctx, userA(t), e); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindAll(ctx, userA(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(seedAll)+1 {
		t.Fatalf("%d 件（期待 %d）", len(got), len(seedAll)+1)
	}
	back := got[len(got)-1]
	if back.ID() != e.ID() || back.Name() != e.Name() || !back.IsCustom() || back.IsDeleted() ||
		back.Increment().Kg() != 2.5 ||
		!slices.Equal(back.PrimaryRegions(), e.PrimaryRegions()) ||
		!slices.Equal(back.SecondaryRegions(), e.SecondaryRegions()) {
		t.Errorf("読み戻した種目が違う: %+v", back)
	}
}

func TestExerciseRepository_Postgres_DeleteIsLogical(t *testing.T) {
	// 保存 → e.Delete() を保存 → FindAll に IsDeleted() で1件残ること
}

func TestExerciseRepository_Postgres_NameIsUniqueAmongAlive(t *testing.T) {
	// memory の同名テストと同じ3手順。2件目は errors.Is(err, exercise.ErrDuplicateExerciseName)
}

func TestExerciseRepository_Postgres_RefusesSeedExercises(t *testing.T) {
	// memory と同じ
}
```

2〜4本目の本文は memory 側（Task 4）のテストと同じ手順・同じ期待値で書く（ケース名を揃える。`cross_user_test.go` の方針と同じ）。`cross_user_test.go` には「B の FindAll に A の種目が出ない」を1ケース足す。

- [ ] **Step 3: 赤を確認する**

Run: `go test ./internal/infrastructure/postgres/ -run 'ExerciseRepository|Migrate|CrossUser' -v`
Expected: `undefined: postgres.NewExerciseRepository`。**`SKIP` が出たら `TEST_DATABASE_URL` が無い。立ててから回す。**

- [ ] **Step 4: 実装する**

```go
// ExerciseRepository は種目の Postgres 実装。
//
// 共通の種目はバイナリ同梱のシードをそのまま返し、利用者が足した種目
// だけを custom_exercises から読む。合わせる場所はここ1箇所。
type ExerciseRepository struct {
	pool *pgxpool.Pool
	seed []*exercise.Exercise
}

func NewExerciseRepository(pool *pgxpool.Pool, seed []*exercise.Exercise) *ExerciseRepository {
	copied := make([]*exercise.Exercise, len(seed))
	copy(copied, seed)
	return &ExerciseRepository{pool: pool, seed: copied}
}

func (r *ExerciseRepository) FindAll(ctx context.Context, user account.UserID) ([]*exercise.Exercise, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, primary_regions, secondary_regions, increment_kg, deleted_at IS NOT NULL
		FROM custom_exercises WHERE user_id = $1 ORDER BY id`, user.String())
	if err != nil {
		return nil, wrapUnavailable(err, "種目を読めない")
	}
	defer rows.Close()

	out := make([]*exercise.Exercise, 0, len(r.seed))
	out = append(out, r.seed...)
	for rows.Next() {
		var (
			id, name                   string
			rawPrimary, rawSecondary   []byte
			inc                        float64
			deleted                    bool
		)
		if err := rows.Scan(&id, &name, &rawPrimary, &rawSecondary, &inc, &deleted); err != nil {
			return nil, wrapUnavailable(err, "種目を読めない")
		}
		var primary, secondary []training.MuscleRegion
		if err := json.Unmarshal(rawPrimary, &primary); err != nil {
			return nil, fmt.Errorf("種目 %s の主に効く部位を解釈できない: %w", id, err)
		}
		if err := json.Unmarshal(rawSecondary, &secondary); err != nil {
			return nil, fmt.Errorf("種目 %s の少し効く部位を解釈できない: %w", id, err)
		}
		// 保存済みの値も必ずコンストラクタを通す（ProgramRepository.Get と同じ理由）。
		e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
			ID: id, Name: name, Primary: primary, Secondary: secondary, IncrementKg: inc,
		})
		if err != nil {
			return nil, fmt.Errorf("保存済みの種目が不正: %w", err)
		}
		if deleted {
			e = e.Delete()
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapUnavailable(err, "種目を読めない")
	}
	return out, nil
}

func (r *ExerciseRepository) Save(ctx context.Context, user account.UserID, e *exercise.Exercise) error {
	if e == nil || !e.IsCustom() {
		return errors.New("共通の種目は保存できない")
	}
	primary, _ := json.Marshal(e.PrimaryRegions())
	secondary, _ := json.Marshal(e.SecondaryRegions())

	// 消した時刻は最初に消したときのまま。二度消しても動かさない。
	_, err := r.pool.Exec(ctx, `
		INSERT INTO custom_exercises
			(user_id, id, name, primary_regions, secondary_regions, increment_kg, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $7 THEN now() END)
		ON CONFLICT (user_id, id) DO UPDATE SET
			deleted_at = CASE WHEN $7 THEN COALESCE(custom_exercises.deleted_at, now()) END`,
		user.String(), string(e.ID()), e.Name(), primary, secondary, e.Increment().Kg(), e.IsDeleted())
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "custom_exercises_alive_name" {
		return fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, e.Name())
	}
	if err != nil {
		return wrapUnavailable(err, "種目を保存できない")
	}
	return nil
}
```

`SecondaryRegions` が空でも `regionsAt` が空スライスを返すので、`[]` で保存される（`null` にならない）。`increment_kg` の `numeric` を `float64` で読めない場合は、既存の `set_log_repository.go` の重量の読み方に合わせる。

`cmd/api/main.go`：
- `repositories.exercises` の型を `exerciseStore interface { exercise.Reader; exercise.Writer }` にする（`programStore` と同じ形）
- インメモリ構成は `memory.NewExerciseRepository(pool)`、Postgres 構成は `postgres.NewExerciseRepository(dbPool, pool)`
- `openRepositories` のコメント「種目マスタだけは常にインメモリ…」を「共通の種目はバイナリ同梱。DB に置くのは利用者が足した種目だけ」に書き換える

- [ ] **Step 5: 緑と、旧版からの移行**

Run: `go test ./internal/infrastructure/postgres/ -count=1`
Expected: PASS（SKIP が無いこと）

CLAUDE.md「DBのスキーマを触った → 旧版でDBを作ってから新版を当てる」。`git stash` は未追跡のマイグレーションを退避しないので **worktree で旧版を立てる**：

```bash
git worktree add ../liftplan-old origin/main
(cd ../liftplan-old && DATABASE_URL=$TEST_DATABASE_URL go run ./cmd/api & sleep 5; kill %1)
DATABASE_URL=$TEST_DATABASE_URL go run ./cmd/api &  # 新版。起動ログに 0013 の適用が出て落ちないこと
sleep 5; curl -s localhost:8080/health; kill %1
git worktree remove ../liftplan-old
```

ポート・起動方法は README の「ローカルで動かす」に合わせる。

- [ ] **Step 6: 変異**

- `FindAll` の `WHERE user_id = $1` を外す → 利用者分離のテストが赤
- `Save` の `ON CONFLICT` の `CASE` を `deleted_at = NULL` にする → `DeleteIsLogical` が赤
- 索引の `WHERE deleted_at IS NULL` を外したマイグレーションで回す → `NameIsUniqueAmongAlive` の後半が赤（確認後、マイグレーションを戻して DB を作り直す）

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "feat(postgres): 利用者が足した種目を custom_exercises に置く"
```

### Task 6: 消した種目は選べないようにする

**Files:**
- Modify: `internal/application/usecase/verify_selection.go`
- Modify: `internal/application/usecase/verify_selection_test.go`

**Interfaces:**
- Consumes: `(*Exercise).IsDeleted`

- [ ] **Step 1: テストを書く**

既存の `verify_selection_test.go` の表に1行足す形で書く。形が違えば、次の関数を足す：

```go
// 消した種目を選択に戻せないこと。古い画面から PUT /api/program/selected
// されたときに、計画に消した種目が戻ってくる。
func TestVerifySelection_RejectsDeletedExercise(t *testing.T) {
	e, _ := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: "u-000000000000000a", Name: "アイソラテラル・ロー",
		Primary: []training.MuscleRegion{training.Lat}, IncrementKg: 2.5,
	})
	pool := append(seedPool(t), e.Delete())
	prog := programSelecting(t, "bench", "squat", "deadlift", "u-000000000000000a")

	err := verifySelection(pool, prog)
	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("消した種目が選べた: %v", err)
	}
}
```

`seedPool`・`programSelecting` は同じファイルの既存ヘルパの名前に合わせる（無ければ `seed.Exercises()` と `program.NewProgram` で作る小さいヘルパを書く）。

- [ ] **Step 2: 赤を確認** — Run: `go test ./internal/application/usecase/ -run VerifySelection` → 期待：`消した種目が選べた: <nil>`

- [ ] **Step 3: 実装する**

```go
	for _, id := range prog.SelectedExercises() {
		e, ok := known[id]
		if !ok {
			return fmt.Errorf("%w: %w: %s", apperror.ErrInvalidInput, exercise.ErrExerciseNotFound, id)
		}
		if e.IsDeleted() {
			return fmt.Errorf("%w: 消した種目は選べない: %s", apperror.ErrInvalidInput, id)
		}
	}
```

- [ ] **Step 4: 緑 → 変異（`IsDeleted` の分岐を消して赤）→ コミット**

```bash
git commit -am "feat(usecase): 消した種目を使う種目に選べないようにする"
```

### Task 7: 種目を足す

**Files:**
- Create: `internal/application/usecase/add_custom_exercise.go`
- Create: `internal/application/usecase/add_custom_exercise_test.go`
- Modify: `internal/application/apperror/apperror.go`、`classify.go`、`apperror_test.go`

**Interfaces:**
- Consumes: `exercise.Reader`・`exercise.Writer`・`program.Reader`・`program.Writer`・`NewCustomExercise`・`NewRandomCustomExerciseID`
- Produces:
  - `usecase.AddCustomExerciseInput{Name string; Primary, Secondary []training.MuscleRegion; IncrementKg float64}`
  - `usecase.NewAddCustomExercise(exercises exerciseStore, reader program.Reader, writer program.Writer) *AddCustomExercise`（`exerciseStore` は usecase パッケージ内の `interface{ exercise.Reader; exercise.Writer }`）
  - `(*AddCustomExercise).Execute(ctx, user, in AddCustomExerciseInput) (*exercise.Exercise, error)`
  - `apperror.ErrDuplicateName`（`"DUPLICATE_NAME"`, `"同じ名前の種目がある"`, 409）

- [ ] **Step 1: テストを書く**

`testuser_test.go` の既存ヘルパで利用者とメモリのリポジトリを組む。

```go
func newAdd(t *testing.T) (*usecase.AddCustomExercise, *memory.ExerciseRepository, *memory.ProgramRepository, account.UserID) {
	t.Helper()
	seedAll, _ := seed.Exercises()
	exercises := memory.NewExerciseRepository(seedAll)
	programs := memory.NewProgramRepository()
	user := testUser(t)
	prog, _ := seed.DefaultProgram(seedAll)
	_ = programs.Save(context.Background(), user, prog)
	return usecase.NewAddCustomExercise(exercises, programs, programs), exercises, programs, user
}

func isoRow() usecase.AddCustomExerciseInput {
	return usecase.AddCustomExerciseInput{
		Name: "アイソラテラル・ロー",
		Primary: []training.MuscleRegion{training.TrapMid},
		Secondary: []training.MuscleRegion{training.Lat, training.Biceps},
		IncrementKg: 2.5,
	}
}

func TestAddCustomExercise_AddsAndSelects(t *testing.T) {
	ctx := context.Background()
	add, exercises, programs, user := newAdd(t)

	e, err := add.Execute(ctx, user, isoRow())
	if err != nil {
		t.Fatal(err)
	}
	all, _ := exercises.FindAll(ctx, user)
	if all[len(all)-1].ID() != e.ID() {
		t.Error("足した種目が一覧に無い")
	}
	prog, _ := programs.Get(ctx, user)
	if !prog.Includes(e.ID()) {
		t.Error("足した種目が使う種目に入っていない")
	}
}

func TestAddCustomExercise_RejectsDuplicateNames(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		prep func(*usecase.AddCustomExercise, account.UserID)
		in   string
	}{
		{"共通の種目と同じ", func(*usecase.AddCustomExercise, account.UserID) {}, "サイドレイズ"},
		{"自分の種目と同じ", func(a *usecase.AddCustomExercise, u account.UserID) {
			_, _ = a.Execute(ctx, u, isoRow())
		}, "アイソラテラル・ロー"},
		{"前後の空白だけ違う", func(a *usecase.AddCustomExercise, u account.UserID) {
			_, _ = a.Execute(ctx, u, isoRow())
		}, " アイソラテラル・ロー "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			add, _, _, user := newAdd(t)
			c.prep(add, user)
			in := isoRow()
			in.Name = c.in
			if _, err := add.Execute(ctx, user, in); !errors.Is(err, apperror.ErrDuplicateName) {
				t.Errorf("通ってしまった: %v", err)
			}
		})
	}
}

func TestAddCustomExercise_InvalidInputIs400(t *testing.T) {
	add, _, _, user := newAdd(t)
	in := isoRow()
	in.Primary = nil
	if _, err := add.Execute(context.Background(), user, in); !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("主なしが ErrInvalidInput にならない: %v", err)
	}
}

func TestAddCustomExercise_RequiresAProgram(t *testing.T) {
	seedAll, _ := seed.Exercises()
	programs := memory.NewProgramRepository()
	add := usecase.NewAddCustomExercise(memory.NewExerciseRepository(seedAll), programs, programs)
	_, err := add.Execute(context.Background(), testUser(t), isoRow())
	if !errors.Is(err, apperror.ErrNotConfigured) {
		t.Errorf("プログラム未設定で通った: %v", err)
	}
}
```

「消した種目と同名は通る」は Task 8 のテストで見る（消すユースケースが要るため）。

- [ ] **Step 2: 赤を確認** — `undefined: usecase.NewAddCustomExercise`

- [ ] **Step 3: 実装する**

`apperror.go` に：

```go
	// ErrDuplicateName は同じ名前の種目が既にあること。
	ErrDuplicateName = newError("DUPLICATE_NAME", "同じ名前の種目がある", http.StatusConflict)
```

`classify.go` の `switch` に：

```go
	case errors.Is(err, exercise.ErrDuplicateExerciseName):
		return fmt.Errorf("%w: %w", ErrDuplicateName, err)
```

`apperror_test.go` がコードの一覧や写し先を表で持っていれば、同じ形で1行足す。

`add_custom_exercise.go`：

```go
package usecase

// exerciseStore は種目の読み書き。足す・消すは両方要る。
type exerciseStore interface {
	exercise.Reader
	exercise.Writer
}

// AddCustomExerciseInput は利用者が足す種目の入力。
type AddCustomExerciseInput struct {
	Name        string
	Primary     []training.MuscleRegion
	Secondary   []training.MuscleRegion
	IncrementKg float64
}

// AddCustomExercise は利用者が種目を足す。
//
// 足した種目は使う種目にも入れる。足すのは使うためで、チェックを入れ直す
// 手間を残さない。種目の保存とプログラムの保存の間で落ちると、種目は
// あるが選ばれていない状態になる。設定画面でチェックを入れれば済むので、
// トランザクションは張らない（設計書「ユースケース」）。
type AddCustomExercise struct {
	exercises exerciseStore
	reader    program.Reader
	writer    program.Writer
}

func NewAddCustomExercise(exercises exerciseStore, reader program.Reader, writer program.Writer) *AddCustomExercise {
	return &AddCustomExercise{exercises: exercises, reader: reader, writer: writer}
}

func (u *AddCustomExercise) Execute(ctx context.Context, user account.UserID, in AddCustomExerciseInput) (_ *exercise.Exercise, err error) {
	defer func() { err = apperror.Classify(err) }()

	// プログラムが無ければ種目を作る前に止める。作ってから止まると、
	// 選ばれていない種目だけが残る。
	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return nil, err
	}

	id, err := exercise.NewRandomCustomExerciseID()
	if err != nil {
		return nil, err
	}
	e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: string(id), Name: in.Name,
		Primary: in.Primary, Secondary: in.Secondary, IncrementKg: in.IncrementKg,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", apperror.ErrInvalidInput, err)
	}

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	for _, other := range pool {
		if other != nil && !other.IsDeleted() && other.Name() == e.Name() {
			return nil, fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, e.Name())
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("種目の保存が中断された: %w", err)
	}
	if err := u.exercises.Save(ctx, user, e); err != nil {
		return nil, err
	}

	next, err := prog.WithSelected(append(prog.SelectedExercises(), e.ID()))
	if err != nil {
		return nil, err
	}
	if err := u.writer.Save(ctx, user, next); err != nil {
		return nil, err
	}
	return e, nil
}
```

`e.Name()` は `NewExercise` が前後の空白を落とした後の名前なので、「前後の空白だけ違う」は比較で弾ける。

- [ ] **Step 4: 緑 → 変異**

- 重複チェックの `!other.IsDeleted()` を消す → Task 8 の「消した種目と同名は通る」が赤になることを Task 8 で確かめる
- `WithSelected` の行を飛ばして `prog` のまま保存 → `AddsAndSelects` が赤
- `classify.go` の新しい `case` を消す → `RejectsDuplicateNames` が赤（`ErrDuplicateExerciseName` は usecase が直接返しているので、Classify を通らないと `ErrDuplicateName` にならない）

- [ ] **Step 5: コミット**

```bash
git add -A
git commit -m "feat(usecase): 利用者が種目を足し、使う種目に入れる"
```

### Task 8: 種目を消す

**Files:**
- Create: `internal/application/usecase/delete_custom_exercise.go`
- Create: `internal/application/usecase/delete_custom_exercise_test.go`
- Modify: `internal/application/apperror/apperror.go`、`apperror_test.go`

**Interfaces:**
- Consumes: Task 7 の `exerciseStore`・`AddCustomExercise`（テストで使う）
- Produces:
  - `usecase.NewDeleteCustomExercise(exercises exerciseStore, reader program.Reader, writer program.Writer) *DeleteCustomExercise`
  - `(*DeleteCustomExercise).Execute(ctx, user, id exercise.ExerciseID) error`
  - `apperror.ErrExerciseNotFound`（`"EXERCISE_NOT_FOUND"`, `"種目が見つからない"`, 404）
  - `apperror.ErrStillDeclared`（`"STILL_DECLARED"`, `"伸ばしたい種目に入っている種目は消せない"`, 409）

- [ ] **Step 1: テストを書く**

```go
// 消す／足すを同じリポジトリで組む。
type fixture struct {
	add       *usecase.AddCustomExercise
	del       *usecase.DeleteCustomExercise
	exercises *memory.ExerciseRepository
	programs  *memory.ProgramRepository
	user      account.UserID
}

func newFixture(t *testing.T) fixture { /* newAdd と同じ組み立て＋NewDeleteCustomExercise */ }

func TestDeleteCustomExercise_DeletesAndUnselects(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, _ := f.add.Execute(ctx, f.user, isoRow())

	if err := f.del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatal(err)
	}
	all, _ := f.exercises.FindAll(ctx, f.user)
	if got := all[len(all)-1]; got.ID() != e.ID() || !got.IsDeleted() {
		t.Error("消した種目が消えた状態で残っていない")
	}
	prog, _ := f.programs.Get(ctx, f.user)
	if prog.Includes(e.ID()) {
		t.Error("消した種目が使う種目に残っている")
	}
}

func TestDeleteCustomExercise_IsIdempotent(t *testing.T) {
	// 足す → 消す → もう一度消す が nil
}

func TestDeleteCustomExercise_NotFound(t *testing.T) {
	ctx := context.Background()
	for name, id := range map[string]exercise.ExerciseID{
		"共通の種目":   "side_raise",
		"存在しない":   "u-ffffffffffffffff",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if err := f.del.Execute(ctx, f.user, id); !errors.Is(err, apperror.ErrExerciseNotFound) {
				t.Errorf("404 にならない: %v", err)
			}
		})
	}
	t.Run("他人の種目", func(t *testing.T) {
		f := newFixture(t)
		e, _ := f.add.Execute(ctx, f.user, isoRow())
		other := testUser(t)
		// other にもプログラムを入れておく（未設定の 409 と混ぜない）
		prog, _ := f.programs.Get(ctx, f.user)
		_ = f.programs.Save(ctx, other, prog)
		if err := f.del.Execute(ctx, other, e.ID()); !errors.Is(err, apperror.ErrExerciseNotFound) {
			t.Errorf("他人の種目が 404 にならない: %v", err)
		}
	})
}

func TestDeleteCustomExercise_RefusesDeclared(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, _ := f.add.Execute(ctx, f.user, isoRow())
	prog, _ := f.programs.Get(ctx, f.user)
	next, _ := prog.WithDeclared(append(prog.DeclaredExercises(), e.ID()))
	_ = f.programs.Save(ctx, f.user, next)

	if err := f.del.Execute(ctx, f.user, e.ID()); !errors.Is(err, apperror.ErrStillDeclared) {
		t.Errorf("伸ばしたい種目が消せた: %v", err)
	}
}

// 消した種目と同じ名前で足し直せること。
func TestAddCustomExercise_AllowsTheNameOfADeletedExercise(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, _ := f.add.Execute(ctx, f.user, isoRow())
	_ = f.del.Execute(ctx, f.user, e.ID())
	if _, err := f.add.Execute(ctx, f.user, isoRow()); err != nil {
		t.Errorf("消した種目と同名が弾かれた: %v", err)
	}
}

// 途中で落ちても壊れた状態を残さないこと（Review Focus 1）。
//
// 壊れた状態とは「種目は消えたのに使う種目に残っている」こと。そうなると
// verifySelection が消した種目を弾くので、以後の選択の保存が失敗し続け、
// 設定の一覧にも消した種目は出ないので本人は直せない。
//
// プログラムの保存を1回だけ落とす。書く順が「種目→プログラム」だと、
// 1回目で種目だけが消えてこの状態になる。
func TestDeleteCustomExercise_LeavesNoBrokenStateOnFailure(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	e, _ := f.add.Execute(ctx, f.user, isoRow())

	flaky := &failOnceProgramWriter{Writer: f.programs}
	del := usecase.NewDeleteCustomExercise(f.exercises, f.programs, flaky)
	if err := del.Execute(ctx, f.user, e.ID()); err == nil {
		t.Fatal("1回目が落ちなかった（スタブが効いていない）")
	}

	all, _ := f.exercises.FindAll(ctx, f.user)
	prog, _ := f.programs.Get(ctx, f.user)
	if all[len(all)-1].IsDeleted() && prog.Includes(e.ID()) {
		t.Fatal("種目は消えたのに使う種目に残っている")
	}

	if err := del.Execute(ctx, f.user, e.ID()); err != nil {
		t.Fatalf("消し直せない: %v", err)
	}
	all, _ = f.exercises.FindAll(ctx, f.user)
	prog, _ = f.programs.Get(ctx, f.user)
	if !all[len(all)-1].IsDeleted() || prog.Includes(e.ID()) {
		t.Error("消し直した後の状態が違う")
	}
}

// 消した種目の記録は受け付け、履歴に名前が出ること（Review Focus 4）。
func TestDeletedExercise_KeepsLogsAndNames(t *testing.T) {
	// 足す → 消す →
	// usecase.NewRecordSets(logs, f.exercises).Execute でその ID のセットログを1件 → nil
	// query.NewHistory(logs, f.exercises) で引いた日の ExerciseLog.Name が "アイソラテラル・ロー"
}
```

`failOnceProgramWriter` はテスト内の小さいスタブ：`program.Writer` を埋め込み、最初の `Save` だけ `errors.New("一時的に保存できない")` を返す。

`IsIdempotent` と `KeepsLogsAndNames` の本文は、コメントの手順どおりに `DeletesAndUnselects` と同じ書き方で埋める（`record_test.go` にセットログを作るヘルパ、`history_test.go` に履歴を引くヘルパがあれば使う）。

- [ ] **Step 2: 赤を確認** — `undefined: usecase.NewDeleteCustomExercise`

- [ ] **Step 3: 実装する**

`apperror.go`：

```go
	// ErrExerciseNotFound は消そうとした種目が無いこと。共通の種目と
	// 他人の種目もここに入る（その利用者から見て「消せる種目」が無い）。
	ErrExerciseNotFound = newError("EXERCISE_NOT_FOUND", "種目が見つからない", http.StatusNotFound)

	// ErrStillDeclared は伸ばしたい種目に入っている種目を消そうとしたこと。
	//
	// 黙って宣言から外さない。伸ばしたい種目が0個になりうるのと、軸の
	// 顔ぶれが変わっても気づけないため（Program.WithSelected と同じ理由）。
	ErrStillDeclared = newError("STILL_DECLARED", "伸ばしたい種目に入っている種目は消せない", http.StatusConflict)
```

`delete_custom_exercise.go`：

```go
// DeleteCustomExercise は利用者が足した種目を消す（論理削除）。
//
// **プログラムを先に、種目を後に書く。**逆順だと、途中で落ちたときに
// 消した種目が使う種目に残り、verifySelection がそれを弾くので、以後の
// 選択の保存が失敗し続ける。この順なら途中で落ちても「チェックが外れた
// だけ」で、もう一度消せば終わる。
type DeleteCustomExercise struct {
	exercises exerciseStore
	reader    program.Reader
	writer    program.Writer
}

func NewDeleteCustomExercise(exercises exerciseStore, reader program.Reader, writer program.Writer) *DeleteCustomExercise {
	return &DeleteCustomExercise{exercises: exercises, reader: reader, writer: writer}
}

func (u *DeleteCustomExercise) Execute(ctx context.Context, user account.UserID, id exercise.ExerciseID) (err error) {
	defer func() { err = apperror.Classify(err) }()

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	var target *exercise.Exercise
	for _, e := range pool {
		if e != nil && e.ID() == id && e.IsCustom() {
			target = e
			break
		}
	}
	if target == nil {
		return fmt.Errorf("%w: %s", apperror.ErrExerciseNotFound, id)
	}
	// 消した種目をもう一度消しても、同じ手順をなぞって成功する（冪等）。
	// 「消してあれば何もしない」で抜けると、前回プログラムの保存だけが
	// 落ちていた場合に、選択に残った ID を外す機会が無くなる。

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}
	if prog.Declares(id) {
		return fmt.Errorf("%w: %s", apperror.ErrStillDeclared, id)
	}

	rest := make([]exercise.ExerciseID, 0, len(prog.SelectedExercises()))
	for _, s := range prog.SelectedExercises() {
		if s != id {
			rest = append(rest, s)
		}
	}
	next, err := prog.WithSelected(rest)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("種目の削除が中断された: %w", err)
	}
	if err := u.writer.Save(ctx, user, next); err != nil {
		return err
	}
	return u.exercises.Save(ctx, user, target.Delete())
}
```

- [ ] **Step 4: 緑 → 変異**

- 保存の順を入れ替える（種目を先に）→ `LeavesNoBrokenStateOnFailure` が「種目は消えたのに使う種目に残っている」で赤
- 先頭に `if target.IsDeleted() { return nil }` を足し、かつ保存の順を入れ替える → 2回目の後も選択に残り、同じテストの後半が赤（早期 return を入れない理由の確認）
- `prog.Declares(id)` の分岐を消す → `RefusesDeclared` が赤
- `e.IsCustom()` の条件を消す → `NotFound/共通の種目` が赤（共通の種目は Writer が拒否するので 500 系になる）

- [ ] **Step 5: コミット**

```bash
git add -A
git commit -m "feat(usecase): 利用者が足した種目を論理削除で消す"
```

### Task 9: API を開ける

**Files:**
- Modify: `internal/application/query/exercises.go`（`Custom`・`Deleted`）、`exercises_test.go`
- Modify: `internal/presentation/httpapi/dto.go`、`handler.go`（`Handler`・`Dependencies`・`NewHandler`・2つのハンドラ）、`router.go`
- Modify: `internal/presentation/httpapi/handler_test.go`（`dependencies` に2つ足す）、`read_test.go`
- Modify: `cmd/api/main.go`（`Dependencies` に2つ足す）

**Interfaces:**
- Consumes: Task 7・8 のユースケース
- Produces（API）:
  - `GET /api/exercises` → 各要素に `"custom": bool`, `"deleted": bool`
  - `POST /api/exercises` 本文 `{"name": string, "primary": [string], "secondary": [string], "increment_kg": number}` → 201、本文は `GET` の1要素と同じ形
  - `DELETE /api/exercises/{id}` → 204

- [ ] **Step 1: テストを書く（`read_test.go` か新規 `custom_exercise_test.go`）**

```go
func TestCustomExercises_AddListDelete(t *testing.T) {
	srv := newServer(t, true)

	rec := do(t, srv, http.MethodPost, "/api/exercises",
		`{"name":"アイソラテラル・ロー","primary":["TRAP_MID"],"secondary":["LAT","BICEPS"],"increment_kg":2.5}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST が %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID       string             `json:"id"`
		Custom   bool               `json:"custom"`
		Stimulus map[string]float64 `json:"stimulus"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !created.Custom || created.Stimulus["TRAP_MID"] != 1.0 || created.Stimulus["LAT"] != 0.5 {
		t.Errorf("作った種目が違う: %+v", created)
	}

	if rec := do(t, srv, http.MethodDelete, "/api/exercises/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE が %d: %s", rec.Code, rec.Body.String())
	}

	rec = do(t, srv, http.MethodGet, "/api/exercises", "")
	var list struct {
		Exercises []struct {
			ID      string `json:"id"`
			Deleted bool   `json:"deleted"`
		} `json:"exercises"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	found := false
	for _, e := range list.Exercises {
		if e.ID == created.ID {
			found = e.Deleted
		}
	}
	if !found {
		t.Error("消した種目が deleted: true で一覧に残っていない")
	}
}

func TestCustomExercises_ErrorStatuses(t *testing.T) {
	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"主なし", http.MethodPost, "/api/exercises", `{"name":"x","primary":[],"secondary":[],"increment_kg":2.5}`, 400},
		{"共通と同名", http.MethodPost, "/api/exercises", `{"name":"サイドレイズ","primary":["SIDE_DELT"],"secondary":[],"increment_kg":2.5}`, 409},
		{"共通の種目を消す", http.MethodDelete, "/api/exercises/side_raise", "", 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, newServer(t, true), c.method, c.path, c.body)
			if rec.Code != c.want {
				t.Errorf("%d（期待 %d）: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}
```

`do`・`newServer` の引数は既存のテストに合わせる。`TestNewHandler_RejectsMissingDependency` は全フィールドを回すので、`Dependencies` に足した2つも自動で検査される（`handler_test.go` の `dependencies` に足し忘れると、そのテストが赤になって気づける）。

- [ ] **Step 2: 赤を確認** — POST が 405（ルートが無い）

- [ ] **Step 3: 実装する**

`query.Exercise` に `Custom bool`・`Deleted bool` を足し、`All` で `e.IsCustom()`・`e.IsDeleted()` を詰める。`exerciseDTO` に：

```go
	// Custom は利用者が足した種目か。画面が「消す」を出すかに使う。
	Custom bool `json:"custom"`
	// Deleted は消した種目か。履歴の名前のために一覧には残す。設定の
	// 一覧には出さない（画面が落とす）。
	Deleted bool `json:"deleted"`
```

`addCustomExerciseDTO`：

```go
type addCustomExerciseDTO struct {
	Name        string   `json:"name"`
	Primary     []string `json:"primary"`
	Secondary   []string `json:"secondary"`
	IncrementKg float64  `json:"increment_kg"`
}
```

ハンドラ（`handler.go`）：

```go
// handlePostExercise は利用者が種目を足す。
func (h *Handler) handlePostExercise(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req addCustomExerciseDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}
	toRegions := func(ss []string) []training.MuscleRegion {
		out := make([]training.MuscleRegion, 0, len(ss))
		for _, s := range ss {
			out = append(out, training.MuscleRegion(s))
		}
		return out
	}
	e, err := h.addExercise.Execute(r.Context(), user, usecase.AddCustomExerciseInput{
		Name: req.Name, Primary: toRegions(req.Primary), Secondary: toRegions(req.Secondary),
		IncrementKg: req.IncrementKg,
	})
	if err != nil {
		respondError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, exerciseDTOFrom(e))
}

// handleDeleteExercise は利用者が足した種目を消す。
func (h *Handler) handleDeleteExercise(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if err := h.deleteExercise.Execute(r.Context(), user, exercise.ExerciseID(r.PathValue("id"))); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`respondJSON`・`exerciseDTOFrom` は既存の名前に合わせる。`GET` が `query.Exercise` から DTO を作っている関数があれば、`*exercise.Exercise` → `query.Exercise` の変換を `query` パッケージに1つ置き（`query.ExerciseFrom(e *exercise.Exercise) Exercise`）、`All` と POST の両方でそれを使う。同じ変換を2箇所に書かない。

`router.go`：

```go
	mux.HandleFunc("POST /api/exercises", h.handlePostExercise)
	mux.HandleFunc("DELETE /api/exercises/{id}", h.handleDeleteExercise)
```

`Handler`・`Dependencies`・`NewHandler` に `AddExercise *usecase.AddCustomExercise`・`DeleteExercise *usecase.DeleteCustomExercise` を足す（`NewHandler` の `switch` にも2行）。`cmd/api/main.go` と `handler_test.go` の `dependencies` で組む。

- [ ] **Step 4: 緑 → 変異**

- `router.go` の DELETE の行を消す → `AddListDelete` が赤
- `Deleted: e.IsDeleted()` を詰め忘れる → `AddListDelete` の最後が赤

- [ ] **Step 5: 全体の検収**

```bash
gofmt -l . ; go vet ./... ; go test ./... -count=1
go test ./internal/domain/training/seed/ -run TestSimulation -v -count=1 > /tmp/sim_after.txt
```

**`TestSimulation` の数字が PR 2a の時点と1つも変わらないこと**（`git stash` を使わず、PR 2a のコミットで同じコマンドを回した出力と `diff` する）。動いたら、自分の種目が週目標か計画に漏れている。`TestSessionPlanner_PlanIsFixedForTheWholeDay` が緑のこと。

- [ ] **Step 6: README と設計書を直してコミット**

- `README.md` の「種目マスタだけは常にバイナリ同梱で、DB には置かない…」を「共通の種目はバイナリ同梱。利用者が足した種目だけを `custom_exercises` に置く」に書き換え、API の一覧があれば POST・DELETE を足す
- 設計書の front matter の `status` を「PR 2 実装済み」にする

```bash
git add -A
git commit -m "feat(api): 自分の種目を足す・消す API を開ける"
```

PR 本文には、PR 3（画面）が出るまで利用者が種目を足す手段は API だけであること、`GET /api/exercises` が消した種目も返すようになるが、消した種目は PR 3 の前には作れないので画面に影響しないことを書く。

---

## このあと（別の計画）

- **PR 3（画面）**：設定の「種目を足す」と「消す」。`useLiftplan.ts`・`ExercisePicker.tsx`・入力検査の純粋関数。orchestration-hooks の3層で書く
- **PR 4（開発用シミュレーション）**：自分の種目を渡せるようにし、アイソラテラル9種目を入れて補助の割り振りの偏りを見る
