# 重点種目のバリエーションレーン 実装設計

仕様は `docs/superpowers/specs/2026-09-10-focus-lift-variations-design.md`。
ここに書くのは**どう作るか**——型・シグネチャ・置き場所・流れ・テストの構成。

詳細設計は Fable 5.1 に投げた結果を元にしている。その過程で仕様の穴が3つ
見つかり、仕様側に反映済み（軸との重複の門、系統の向き、割れうる点）。

---

## 0. 前提として確かめたこと

- **種目マスタは DB に無い。**`internal/infrastructure/postgres/` にあるのは
  program / setlog / condition だけ。派生の関係をマスタに足しても**移行は不要**。
  移行が要るのは `Program.focus` だけ
- **`pool` は選択された種目だけ。**`usablePool` が `Program.Includes` で絞る。
  D-114 で「選択されていなくてもバリエーションは回る」抜け道は塞がれているので、
  派生も選択されていなければ候補にならない。**この設計もそれを崩さない**
- **`planMain` は種目に依存しない汎用の処方関数**になっている。バリエーションの
  処方は**これをそのまま使える**
- PWA に `declared_exercises` への参照が無い。設定画面は**API 契約までをこの
  設計の範囲**とし、画面は別の作業

---

## 1. 派生の関係をどう表すか

### 選択肢

| | 形 | 評価 |
|---|---|---|
| A | `Exercise` に親種目への参照を持たせる | 関係が「種目→種目」で閉じる。列挙も分類も持ち込まない |
| B | 値オブジェクトで包む（`type Lineage struct{ root ExerciseID }`） | `CLAUDE.md` の「値オブジェクトは構造体で包む」に字面上は従うが、`NewExerciseID` 以上に検証するものが無い。**包む理由が無い** |
| C | `planning` に静的な対応表（`map[ExerciseID][]ExerciseID`）を置く | 旧 `MainLift` と同じ「マスタの外に表がある」形。シードと二重管理になり、表に無い種目が黙って落ちる |

### 採用：A

「値オブジェクトは構造体で包む」の趣旨は「`X(1.5)` でコンストラクタを
素通りさせない」こと——`Weight` `Ratio` など**量**の話。`ExerciseID` は
同一性で、既にこのリポジトリで**唯一の定義型として例外扱い**されている。

親への参照は「別の種目の同一性」なので `ExerciseID` をそのまま使う。包むと
`NewExerciseID` を二重に呼ぶだけの型が増える。

**B を検討して採らなかった理由をここに残す。**後で「なぜ VO でないのか」を
問われたときに答えられるように。

### 名前

`Family` / `Parent` / `DerivedFrom` / `VariationOf` を比べた。

- `Family` は**集合**を連想させる（「ベンチ系統」＝4種目）が、フィールドが
  持つのは**親1つ**。名前と値の形が合わない
- `Parent` は木構造の一般語で、「派生」という関係の意味が消える
- **`DerivedFrom`** は仕様の「派生の関係」と一致し、`e.DerivedFrom()` →
  `"bench"` と読める。シードの既存コメント「メインの派生」とも揃う

### 型

```go
// internal/domain/training/exercise/exercise.go

type ExerciseParams struct {
	ID               string
	Name             string
	Stimulus         map[training.MuscleRegion]float64
	IncrementKg      float64
	BodyweightFactor float64
	DerivedFrom      string // 空なら派生ではない
}

type Exercise struct {
	// 既存フィールド…
	derivedFrom    ExerciseID
	hasDerivedFrom bool
}

// DerivedFrom はこの種目が派生した元の種目。派生でなければ false。
func (e *Exercise) DerivedFrom() (ExerciseID, bool)
```

`(値, bool)` は `Weight()` `Intent()` と同じ「任意項目」の規約。

### 不変条件と守る場所

| 不変条件 | 場所 | 理由 |
|---|---|---|
| `derivedFrom != id` | `NewExercise` | 自己参照すると、重点種目に指定した瞬間その種目自身がバリエーション候補になり、軸でない日に「宣言した種目がバリエーションとして出る」。**黙って壊れる**ので生成時に弾く |
| 親がマスタに実在する | `seed` のテスト | `Exercise` はマスタを知らないので自分では検証できない |
| 派生の派生が無い | 同上 | 連鎖を許すと系統の定義が根まで辿る処理になる。いま要らない |

`/api/exercises` の `exerciseDTO` には**足さない**。消費者がいない。

### シード

```go
// spec / bodyweightExercise と並ぶ組み立て関数
func derived(from, id, name string, inc float64, s stimulus) exercise.ExerciseParams
```

7種目。`larsen_press` `tempo_bench` `close_grip_bench` → `bench`、
`pause_squat` `front_squat` → `squat`、`deficit_deadlift` `romanian_deadlift`
→ `deadlift`。

セクション見出し「メインの派生（補助として残差を埋める）」は
「派生（重点種目のバリエーションとして回る）」に直す。

---

## 2. 重点種目を `Program` にどう持たせるか

```go
type Program struct {
	frequency Frequency
	target    WeeklyVolumeTarget
	selected  []exercise.ExerciseID
	declared  []exercise.ExerciseID
	focus     exercise.ExerciseID // 重点種目。空なら指定なし
}

func NewProgram(
	freq Frequency, target WeeklyVolumeTarget,
	selected, declared []exercise.ExerciseID,
	focus exercise.ExerciseID,
) (*Program, error)

// Focus は重点種目。指定が無ければ false。
func (p *Program) Focus() (exercise.ExerciseID, bool)
```

`hasFocus bool` を別に持たないのは、`ExerciseID` は**空文字が不正値**
（`NewExerciseID` が弾く）なので、空＝未指定で曖昧さが無いため。

### `focus ∈ declared` の場所

`NewProgram` の中、`declared ⊂ selected` の検査の**直後**。同じ場所に同じ形で
並べる。エラー文は既存に倣い `重点種目 %q が伸ばしたい種目に含まれていない`。

空（未指定）は `normalizeExerciseIDs` を通す前に素通しする。

引数が5つになるので、呼び出し側は全部コンパイルで捕まる
（`configure_program.go` / `program_repository.go` / `cmd/api` の
`defaultProgram` / `simulation_test` / 各テストの `planProgram`）。
`defaultProgram` は `""`。

### 永続化

```sql
-- 0005_add_focus_exercise_in_program.sql
ALTER TABLE program ADD COLUMN focus text;  -- NULL = 指定なし
```

`declared` の移行（0004）と違い、**3段階にしない。**NULL が正当な値
（仕様の既定は指定なし）なので、埋め戻しも `NOT NULL` も要らない。
**コメントにその対比を書く**——0004 を読んだ人が「なぜ今回は1段階か」で
止まらないように。

読み出しは必ず `NewProgram` を通す（既存の方針。`jsonb` と同じく、ここが
唯一の防波堤）。

### API

```go
type programDTO struct {
	// 既存…
	Focus *string `json:"focus_exercise"`
}
```

ポインタなのは `weight_kg` と同じ理由——「指定なし」を `null` で明示し、
**欠落と区別する**。`ConfigureProgramInput` に `Focus` を足す。

---

## 3. レーンの導出をどこに置くか

### 選択肢

| | 形 | 評価 |
|---|---|---|
| A | `SessionPlanner` のメソッド（`heavyLift` の隣） | 軸レーンと同じ形。注入点も `IsZero` も増えない |
| B | `VariationSelector` を新設して注入する | `AccessorySelector` と対称。ただし**コンストラクタに日数を持った瞬間、仕様が禁じたノブになる**。パラメータ無しの `struct{}` にすると名前空間でしかない |
| C | `variation_lane.go` にパッケージ関数を並べる | `doc.go` の規約違反（サービスでない名前のファイルは型に揃える） |

### 採用：A

1. **軸レーンが既に planner のメソッド**（`heavyLift`）。バリエーションレーンは
   「候補から最終実施日が最も古いものを取る」という同じ骨格に、門が2つ
   （軸との重複・中1日）付いただけ。対称にするなら揃える
2. B の唯一の実利は「境界を単体で検査しやすい」だが、境界は `Plan` 経由でも
   **履歴を1日ずらすだけ**で検査できる
3. 消すときの手順が「メソッド2つと定数1つを消す」で済む

`session_planner.go` は約330行。足すのは60行前後で、`stalest` の共通化で
正味の増分はさらに減る。

### シグネチャ

```go
// variationLift は今日バリエーションとしてやる種目を返す。出さない日は nil。
//
// 出さないのは、重点種目が未指定・軸が系統に含まれる・前回その系統をやってから
// 中1日空いていない・派生が1つも選択されていない、のいずれか。
func (p SessionPlanner) variationLift(
	req PlanRequest, pool []*exercise.Exercise, heavy *exercise.Exercise,
) *exercise.Exercise

// lineage は重点種目とその派生のうち、pool にあるものを返す（重点種目自身を含む）。
func lineage(pool []*exercise.Exercise, focus exercise.ExerciseID) []*exercise.Exercise

// variationsOf は重点種目の派生のうち pool にあるものを返す（重点種目自身は含まない）。
func variationsOf(pool []*exercise.Exercise, focus exercise.ExerciseID) []*exercise.Exercise

// stalest は候補のうち最後に実施したのが最も古いものを返す。
// 未着手があればそれを優先する。heavyLift から抜き出し、両レーンで共有する。
func stalest(h setlog.History, candidates []*exercise.Exercise) *exercise.Exercise
```

`heavyLift` は
`stalest(historyBefore(req), declaredExercises(pool, req.Program))` の1行になる。
**ロジックは動かない**ので、この抜き出しは機械的な PR に含めてよい。

---

## 4. `Plan` の中での流れ

```
軸を決める（heavyLift）
  ↓
軸の処方を引く（prescriptions.Select × liftIndexInWeek）
  ↓
台帳：前日までのカバレッジ
  ↓
軸を処方（planMain）→ 台帳に足す
  ↓
★ バリエーションを決める（variationLift）
   出るなら：処方を引く → planMain → 台帳に足す
  ↓
残差を出す（SessionResidual）
  ↓
補助を選ぶ（exclude = 宣言全部 + 今日のバリエーション）
```

**台帳への加算は「軸 → バリエーション → 残差」の順。**`Plus` は可換なので
数値は順序に依らないが、**残差を出す前に両レーンが確定している**ことが本質。
バリエーションを残差の後に足すと、胸をラーセンで埋めたうえに補助でも埋める。

### 処方の引き方

`liftIndexInWeek(historyBefore, variation.ID(), date)` ——
**種目ごと**の週内本数で引く（軸と同じ）。

系統ごと（ベンチ系の週何本目か）で引く案は採らない。ベンチ系が週3回出るなら
3本目が `LIGHT` になるが、それは**「ナローベンチを今週初めてやる日」に
軽い日が当たる**ことを意味する。

`prescriptionsByFrequency` のコメントにある「先頭を標準にするのは、推定1RMが
実力より低いまま固定されないため」は**種目ごとの推定に対する理由**で、
バリエーションは自分の記録で推定する（D-113）以上、この保護は種目ごとに要る。

### 掃除（PR 1 に含める）

- `planMain` の doc「2つ目の返り値は実際に行う種目（バリエーションに
  差し替わることがある）」は **D-114 以降ずっと嘘**。名前を `planLift` に変え、
  2つ目の返り値を落とす
- `usablePool` の doc「その派生バリエーションを返す…選択に含まれていなくても
  候補にする」も、D-114 で塞いだ抜け道の説明が残っている

---

## 5. `AccessorySelector.Select` の除外

### 選択肢

| | 形 | 評価 |
|---|---|---|
| A | `exclude []exercise.ExerciseID` | 呼び出し側が組む集合をそのまま渡す |
| B | 可変長 `exclude ...ExerciseID` | 既存テストが無改修で通るが、`Plan` 側は必ずスライスを組んで展開するので**見た目だけの便利さ** |
| C | `exclude func(ExerciseID) bool` | 柔軟だが、テストの意図（何を除いたか）が読めなくなる |
| D | 呼び出し側で pool から抜いて引数を消す | **不可。**`byID` は除外した種目の履歴も引く必要がある（`ExcludedExerciseStillCountsAsStimulus` が固定している契約）。pool から抜くと「昨日ベンチで胸を刺激した」が消える |
| E | `*program.Program` を渡して中で `Declares` を見る | 選択器が集約に依存する。バリエーションは `Program` に無いので結局もう1引数要る |

### 採用：A

```go
func (s AccessorySelector) Select(
	residual map[training.MuscleRegion]float64,
	pool []*exercise.Exercise,
	h setlog.History,
	date training.Date,
	exclude []exercise.ExerciseID,
) []exercise.ExerciseID
```

**doc を反転させる。**現在の「宣言は『伸ばしたい』であって『ヘビーでしか
やらない』ではないので、除きすぎると脚の日にスクワットがどこにも出なくなる
（D-117）」は、仕様が「的外れだった」と認めた根拠。**なぜ反転したかを残す。**

`Plan` 側：

```go
exclude := append(req.Program.DeclaredExercises(), variationID) // 出ない日は宣言だけ
chosen := p.accessory.Select(gaps, pool, historyBefore(req), req.Date, exclude)
```

**除外の責務は `Plan`。**選択器は「渡されたものを除く」だけで、宣言・
バリエーションという語彙を知らない。

---

## 6. `PlannedSession` の出力

```go
type PlannedSession struct {
	date        training.Date
	main        []PlannedSet
	variation   []PlannedSet
	accessories []PlannedSet
}

func (s PlannedSession) Variation() []PlannedSet // 防御的コピー
```

`(PlannedSet, bool)` にしないのは、`main` が「必ず1件のスライス」で表現されて
いるのに揃えるため。JSON も3レーンが同じ形（配列）になり、**クライアントが
同じコードで描ける**。

```go
type sessionDTO struct {
	Date        string          `json:"date"`
	Main        []plannedSetDTO `json:"main"`
	Variation   []plannedSetDTO `json:"variation"`
	Accessories []plannedSetDTO `json:"accessories"`
}
```

`make([]..., 0, n)` で組み、**`null` ではなく `[]`** を返す。並びは実施順
（軸→バリエーション→補助）。`PlannedSet` は変更なし——バリエーションも
`planMain` が作るので `intent` が付く。

---

## 7. 「中1日以上」の判定

```go
const (
	accessoryIntensityPct = 0.71
	accessoryTargetRIR    = 2

	// variationRecoveryDays は同じ系統を再び出すまでに空ける日数。
	// 2 は「中1日」で、月曜にやったら火曜は出さず水曜から出す。
	// 判定は recovering と同じ開区間 (date - N, date)。
	//
	// AccessorySelector.recoveryDays と値が同じだが共有しない。あちらは
	// 筋区分の回復でコンストラクタの引数、こちらは系統の間隔で設定にしない
	// （仕様）。共有すると片方を動かしたときにもう片方が黙って動く。
	variationRecoveryDays = 2
)
```

```go
// recentlyPerformed は系統のどれかを (date - variationRecoveryDays, date) に
// やったか。
func recentlyPerformed(h setlog.History, lineage []*exercise.Exercise, date training.Date) bool
```

**必ず `historyBefore(req)` を渡す。**`req.History` を使うと、今日ラーセンを
1セット記録して開き直した瞬間に系統が「最近やった」になり、**バリエーションが
自分の下で消える**。D-086 系の再発。

判定に使う系統は `lineage`（**重点種目自身を含む**）。月曜のベンチ（軸）が
火曜のバリエーションを塞ぐのは、ここに重点種目が入っているから。

---

## 8. テストの構成

### 8.1 フィクスチャ

- `planPool` の `larsen` に `DerivedFrom: "bench"` を付け、コメントを反転
  （「かつてベンチのバリエーションだった種目。いまは補助のひとつ」→
  「ベンチの派生。重点種目がベンチのときバリエーションレーンに出る」）
- **回転を検査するため `tempo`（同じく `bench` 派生）を1つ足す**
- `planProgram` は `focus = ""` のまま（既存テストの意味を変えない）。
  `focusedProgram(t, focus)` を `benchOnlyProgram` の隣に置く

### 8.2 受け入れ条件 → テーブル

`TestSessionPlanner_VariationLane`（1関数・テーブル）。

| # | 検査すること |
|---|---|
| 1 | 重点種目を指定しなければ出ない |
| 2 | 軸が重点種目そのものの日は出ない |
| 3 | **軸が重点種目の派生の日も出ない**（仕様の穴1） |
| 4 | 前回の系統から1日しか空いていなければ出ない |
| 5 | 中1日空いていれば出る（境界） |
| 6 | 派生を1つもやっていなければ未着手を優先 |
| 7 | 派生のうち最終実施日が最も古いものが出る |
| 8 | 派生が選択されていなければ出ない |
| 9 | 重量はその種目自身の記録から出る。**重点種目の推定から出た値と不一致**であることも見る |
| 10 | 記録が無ければ重量は未確定 |
| 11 | 今日バリエーションを記録しても今日のリストは変わらない |
| 12 | 意図はその種目の週1本目なら `STANDARD`（同じ週に系統を2回済ませていても） |

構造が違うものは独立関数。

- `SubtractsVariationCoverageFromResidual` — バリエーションが出る日は胸の補助が減る
- `VariationIsNotAlsoAnAccessory`
- `DeclaredExercisesNeverAppearAsAccessories` — 軸かバリエーションでしか現れない
- `ReturnValuesAreDefensivelyCopied` に `Variation()` を足す

### 8.3 シードで回す

`seed/simulation_test.go` に
`TestSimulation_FocusLineageRunsAboutThreeTimesAWeek`。

**実シードで回す理由**は writing-tests skill の「テストが自前データだけで
完結している」罠——`planPool` の派生だけ付けてシードに入れ忘れると、
**本番で機能が丸ごと無効**になる。期待は「ベンチ系が3回（軸1＋バリエーション2）」。

### 8.4 反転する既存テスト（削除しない）

| 既存 | 反転後 |
|---|---|
| `AccessorySelector_DeclaredExercisesCanStillFillResidual` | `ExcludesEveryListedExercise`。「以前は宣言した種目も補助を埋めた（D-117）。バリエーションレーンができたのでやめた」 |
| `AccessorySelector_ExcludesTodaysMainLifts` | 複数除外の1ケースとして吸収。単数の名前を残さない |
| `HeavySlotIsExactlyOne` のコメント「選ばれなかった宣言は補助として残差を埋める」 | 「選ばれなかった宣言はその日は出ない。補助が兼務しない」 |

### 8.5 死んでいるテストを書き直す

`TestSessionPlanner_VariationWeightComesFromItsOwnRecord` は**緑だが何も
守っていない**。`Main()` から `"larsen"` を探しているが `larsen` は宣言に
入っていないので**ループ本体が一度も走らない**（プローブを書いて確認済み）。

8.2 のケース9として `Variation()` に対する値の検査に書き直す。

### 8.6 変異 → 落ちるべきケース

| 変異 | 落ちるテスト |
|---|---|
| 軸との重複の門を消す | #2 #3 |
| `variationRecoveryDays` を 1 に／`After` を `OnOrAfter` に | #4（#5 は緑のまま＝#4 が無いと守れない） |
| `lineage` から重点種目自身を抜く | #4（軸ベンチの翌日にラーセンが出る） |
| 台帳にバリエーションを足さない | `SubtractsVariationCoverageFromResidual` |
| `exclude` をヘビー枠だけに戻す | `DeclaredExercisesNeverAppearAsAccessories` |
| バリエーションを `exclude` に入れない | `VariationIsNotAlsoAnAccessory` |
| `stalest` を ID 昇順の先頭に | #7 |
| 重量を重点種目の記録から出す | #9（値の不一致） |
| `historyBefore` を `req.History` に | #11 |
| 処方の index を系統で数える | #12 |
| シードの `derived` を `spec` に戻す | `DerivedFromResolvesToARootLift`（件数）、`FocusLineageRunsAboutThreeTimesAWeek` |
| `focus ∈ declared` を消す | `TestNewProgram_RejectsInvalid` |

`PlanIsFixedForTheWholeDay` は焦点無しのフィクスチャなので、この機能の
当日固定は #11 が担う。

---

## PR の分け方

仕様の「実装の順序」と同じ5本。判断は `docs/decisions.md` に1本。

D-114 を覆すこと（概念は戻すが形は戻さない、旧 `MainLift` の3つの問題）、
D-117 の「選ばれなかった宣言は補助を埋める」を覆すこと、割れうる点、
`variationRecoveryDays` を `recoveryDays` と共有しない理由。
