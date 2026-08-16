# 進行エンジン（liftplan engine）Implementation Plan

> **⚠️ この計画は破棄された（2026-08-16）。実行しないこと。**
> エンジンをサーバー（Go）へ移す判断をしたため、Kotlin 側にエンジンを置く前提が失効した。
> 後継は `2026-08-16-liftplan-server.md`。ドメインロジックの中身（Epley式、平滑化の手順、
> スロット配分、残差アルゴリズム、シードデータ）は後継計画へそのまま引き継いでいる。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 実績ログ・種目プール・週目標・コンディションを入力として、その日のセッション（種目・重量・目標RIR・セット数）を返す純粋関数を、Android にも DB にも依存しない Kotlin/JVM ライブラリとして実装する。

**Architecture:** Gradle マルチプロジェクトの `:engine` モジュールとして作る。Android プラグインを一切適用しない純粋な Kotlin/JVM ライブラリなので、JUnit だけで全機能を検証できる。副作用は持たず、状態は引数で受け取り結果を返すだけ。日付は `kotlinx-datetime` の `LocalDate` を使う（JVM と Android の両方で同じ型が使え、Android の desugaring 問題を回避できる）。

**Tech Stack:** Kotlin 2.0.21 / Gradle 8.10 / JDK 17 / kotlinx-datetime 0.6.1 / kotlin-test + JUnit 5

**設計ドキュメント:** `~/repos/second-brain/docs/superpowers/specs/2026-08-16-workout-app-design.md`

## Global Constraints

- 実装先は **新規リポジトリ `~/repos/liftplan`**。second-brain リポジトリには一切ファイルを作らない
- `:engine` モジュールに **Android 依存を入れてはいけない**。`com.android.*` の import が現れたらそれは設計違反
- パッケージ名は `dev.dyoshyy.liftplan.engine` で統一する
- **すべての public 関数は純粋関数**。現在時刻の取得（`Clock.System.now()` 等）、ファイルI/O、ログ出力を関数内で行わない。日付は必ず引数で受け取る
- 日付型は `kotlinx.datetime.LocalDate` を使う。`java.time.LocalDate` は使わない
- 重量の単位はすべて kg（`Double`）。セット数・レップ・RIR は `Int`
- 週の開始は **月曜**
- テストは各タスクで必ず「失敗を確認してから実装する」順序で進める
- 各タスクの最後に必ずコミットする

## 前提としている仕様上の判断

計画を書くにあたり、spec に明示されていなかった点をこう解釈した。実装前に違和感があれば先に指摘すること。

1. **全身法なので、毎セッションで SQUAT / BENCH / DEADLIFT の3種目すべてにスロットを割り当てる。** 強度帯が週内で散る（76% / 81% / 88%）ため、頻度が高くても総負荷は破綻しない
2. **履歴のない種目は重量を算出できない。** その場合 `weightKg = null` を返し、初回だけユーザーが自分で決めて記録する。推定に足る履歴が無いのに数字を捏造しない
3. **補助種目のセット数は3固定、1セッションあたりの補助スロット数も3固定**（どちらも引数でデフォルト値として与え、後から変更可能にする）
4. **セッションが週の何本目かは、その週の既存ログから導出する。** 曜日の割り当てはエンジンの責務ではない

---

### Task 1: プロジェクト骨格と `:engine` モジュール

**Files:**
- Create: `~/repos/liftplan/settings.gradle.kts`
- Create: `~/repos/liftplan/build.gradle.kts`
- Create: `~/repos/liftplan/gradle/libs.versions.toml`
- Create: `~/repos/liftplan/engine/build.gradle.kts`
- Create: `~/repos/liftplan/.gitignore`
- Test: `~/repos/liftplan/engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SmokeTest.kt`

**Interfaces:**
- Consumes: なし（最初のタスク）
- Produces: `:engine` Gradle モジュール。以降のすべてのタスクが `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/` にコードを、`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/` にテストを置く

- [ ] **Step 1: リポジトリを作る**

```bash
mkdir -p ~/repos/liftplan/engine/src/main/kotlin/dev/dyoshyy/liftplan/engine
mkdir -p ~/repos/liftplan/engine/src/test/kotlin/dev/dyoshyy/liftplan/engine
cd ~/repos/liftplan && git init
```

- [ ] **Step 2: Gradle 設定ファイルを書く**

`settings.gradle.kts`:

```kotlin
rootProject.name = "liftplan"
include(":engine")

dependencyResolutionManagement {
    repositories {
        mavenCentral()
    }
}
```

`gradle/libs.versions.toml`:

```toml
[versions]
kotlin = "2.0.21"
kotlinxDatetime = "0.6.1"

[libraries]
kotlinx-datetime = { module = "org.jetbrains.kotlinx:kotlinx-datetime", version.ref = "kotlinxDatetime" }

[plugins]
kotlin-jvm = { id = "org.jetbrains.kotlin.jvm", version.ref = "kotlin" }
```

`build.gradle.kts`（ルート）:

```kotlin
plugins {
    alias(libs.plugins.kotlin.jvm) apply false
}
```

`engine/build.gradle.kts`:

```kotlin
plugins {
    alias(libs.plugins.kotlin.jvm)
}

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation(libs.kotlinx.datetime)
    testImplementation(kotlin("test"))
}

tasks.test {
    useJUnitPlatform()
}
```

`.gitignore`:

```
.gradle/
build/
local.properties
.idea/
*.iml
```

- [ ] **Step 3: スモークテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SmokeTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.LocalDate
import kotlin.test.Test
import kotlin.test.assertEquals

class SmokeTest {
    @Test
    fun `kotlinx-datetime が使える`() {
        val date = LocalDate(2026, 8, 16)
        assertEquals(8, date.monthNumber)
    }
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `cd ~/repos/liftplan && ./gradlew :engine:test`

Gradle wrapper がまだ無い場合は先に `gradle wrapper --gradle-version 8.10` を実行する（システムに Gradle が無ければ `sdk install gradle 8.10` などで導入する）。

Expected: BUILD SUCCESSFUL

- [ ] **Step 5: コミット**

```bash
cd ~/repos/liftplan
git add -A
git commit -m "chore: engine モジュールの骨格を作る"
```

---

### Task 2: ドメインモデル

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Domain.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/DomainTest.kt`

**Interfaces:**
- Consumes: Task 1 の `:engine` モジュール
- Produces: `MuscleRegion`(enum, 21値), `MainLift`(enum), `ExerciseKind`(enum), `ExerciseId`(value class), `Exercise`(data class), `SetLog`(data class), `DailyCondition`(data class)。以降のすべてのタスクがこれらを使う

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/DomainTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.LocalDate
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class DomainTest {
    @Test
    fun `筋区分は20から25の範囲で定義されている`() {
        val count = MuscleRegion.entries.size
        assertTrue(count in 20..25, "筋区分は20〜25個であるべきだが $count 個だった")
    }

    @Test
    fun `種目は複数の筋区分に寄与度を持てる`() {
        val bench = Exercise(
            id = ExerciseId("bench"),
            name = "ベンチプレス",
            kind = ExerciseKind.MAIN,
            mainLift = MainLift.BENCH,
            stimulus = mapOf(
                MuscleRegion.CHEST_MID to 1.0,
                MuscleRegion.TRICEPS_LATERAL to 0.5,
                MuscleRegion.FRONT_DELT to 0.5,
            ),
            incrementKg = 2.5,
        )
        assertEquals(1.0, bench.stimulus[MuscleRegion.CHEST_MID])
        assertEquals(3, bench.stimulus.size)
    }

    @Test
    fun `セットログは重量とレップとRIRを持つ`() {
        val log = SetLog(
            id = "01J000000000000000000000",
            date = LocalDate(2026, 8, 16),
            exerciseId = ExerciseId("bench"),
            weightKg = 85.0,
            reps = 9,
            rir = 2,
        )
        assertEquals(85.0, log.weightKg)
        assertEquals(2, log.rir)
    }

    @Test
    fun `日次コンディションは体重と睡眠を欠損可能で持つ`() {
        val c = DailyCondition(date = LocalDate(2026, 8, 16))
        assertEquals(null, c.bodyWeightKg)
        assertEquals(null, c.sleepHours)
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*DomainTest*'`
Expected: コンパイルエラー（`MuscleRegion` などが未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Domain.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.LocalDate

/** 筋区分。粗い「部位」ではなくこの粒度で週ボリュームを管理する。 */
enum class MuscleRegion {
    CHEST_UPPER, CHEST_MID, CHEST_LOWER,
    LAT, TRAP_MID, TRAP_UPPER, ERECTOR,
    FRONT_DELT, SIDE_DELT, REAR_DELT,
    TRICEPS_LONG, TRICEPS_LATERAL,
    BICEPS, FOREARM,
    QUAD, HAMSTRING, GLUTE, ADDUCTOR, CALF,
    ABS, OBLIQUE,
}

/** メイン種目。週内スロットで強度帯を振り分ける対象。 */
enum class MainLift { SQUAT, BENCH, DEADLIFT }

/**
 * MAIN      … 通常フォームのメイン種目
 * VARIATION … メインの派生（ラーセン、テンポなど）。対メイン係数を持つ
 * ACCESSORY … 補助種目。残差を埋めるために選ばれる
 */
enum class ExerciseKind { MAIN, VARIATION, ACCESSORY }

@JvmInline
value class ExerciseId(val value: String)

/**
 * @param stimulus 各筋区分への寄与度（0.0〜1.0）。1セット実施したとき、その筋区分に
 *   何セット分の刺激が入るかを表す。ベンチなら大胸筋中部 1.0、三頭 0.5 のように置く。
 * @param defaultRatioToMain VARIATION のみ。通常フォームに対する挙上重量の比の初期値。
 *   実績が溜まれば実測値で上書きされるため、正確である必要はない。
 */
data class Exercise(
    val id: ExerciseId,
    val name: String,
    val kind: ExerciseKind,
    val stimulus: Map<MuscleRegion, Double>,
    val incrementKg: Double = 2.5,
    val mainLift: MainLift? = null,
    val defaultRatioToMain: Double? = null,
)

/** 確定した実績。エンジンにとって唯一の真実。 */
data class SetLog(
    val id: String,
    val date: LocalDate,
    val exerciseId: ExerciseId,
    val weightKg: Double,
    val reps: Int,
    val rir: Int,
)

/** Health Connect から取り込んだ日次スナップショット。欠損しうる。 */
data class DailyCondition(
    val date: LocalDate,
    val bodyWeightKg: Double? = null,
    val sleepHours: Double? = null,
)
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*DomainTest*'`
Expected: PASS（4件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Domain.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/DomainTest.kt
git commit -m "feat: ドメインモデルを定義する"
```

---

### Task 3: 推定1RM（Epley）

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/OneRepMax.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/OneRepMaxTest.kt`

**Interfaces:**
- Consumes: Task 2 のドメインモデル
- Produces: `fun estimatedOneRepMax(weightKg: Double, reps: Int, rir: Int): Double`

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/OneRepMaxTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlin.test.Test
import kotlin.test.assertEquals

class OneRepMaxTest {
    @Test
    fun `RIR0で1レップなら重量そのものが1RM`() {
        assertEquals(100.0, estimatedOneRepMax(100.0, reps = 1, rir = 0), 0.001)
    }

    @Test
    fun `RIRは限界までの残りレップとして加算される`() {
        // 85kg x 9reps RIR2 → 限界まで11レップ → 85 * (1 + 11/30)
        assertEquals(85.0 * (1 + 11 / 30.0), estimatedOneRepMax(85.0, reps = 9, rir = 2), 0.001)
    }

    @Test
    fun `同じ総レップならRIRの内訳によらず同じ値になる`() {
        val a = estimatedOneRepMax(80.0, reps = 8, rir = 3)
        val b = estimatedOneRepMax(80.0, reps = 11, rir = 0)
        assertEquals(a, b, 0.001)
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*OneRepMaxTest*'`
Expected: コンパイルエラー（`estimatedOneRepMax` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/OneRepMax.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

/**
 * Epley 式による推定1RM。
 *
 * RIR を「限界までの残りレップ数」として実績レップに足すことで、
 * 追い込みきっていないセットからも強度を推定できる。
 * 全セットに RIR を入力する設計は、毎セットを1RM測定に変えるためにある。
 */
fun estimatedOneRepMax(weightKg: Double, reps: Int, rir: Int): Double {
    require(reps > 0) { "reps は1以上である必要がある: $reps" }
    require(rir >= 0) { "rir は0以上である必要がある: $rir" }
    val repsToFailure = reps + rir
    return weightKg * (1 + repsToFailure / 30.0)
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*OneRepMaxTest*'`
Expected: PASS（3件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/OneRepMax.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/OneRepMaxTest.kt
git commit -m "feat: Epley式による推定1RMを実装する"
```

---

### Task 4: 平滑化（セッション代表値・EWMA・ヒステリシス）

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Smoothing.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SmoothingTest.kt`

**Interfaces:**
- Consumes: Task 2 の `SetLog` / `ExerciseId`、Task 3 の `estimatedOneRepMax`
- Produces:
  - `internal fun median(values: List<Double>): Double`
  - `internal fun ewma(values: List<Double>, alpha: Double): Double`
  - `internal fun applyHysteresis(previous: Double?, candidate: Double, threshold: Double): Double`
  - `fun smoothedOneRepMax(logs: List<SetLog>, exerciseId: ExerciseId, previousPublished: Double? = null, alpha: Double = 0.3, hysteresis: Double = 0.02): Double?`

**なぜ必要か:** 単発の記録で全スロットの重量が動くと不安定になる。調子が良かった日の1セットで重量が跳ね上がり、翌週それを引きずって潰れるのを防ぐ。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SmoothingTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.LocalDate
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class SmoothingTest {
    private val bench = ExerciseId("bench")

    private fun log(day: Int, weight: Double, reps: Int, rir: Int) = SetLog(
        id = "log-$day-$weight-$reps",
        date = LocalDate(2026, 8, day),
        exerciseId = bench,
        weightKg = weight,
        reps = reps,
        rir = rir,
    )

    @Test
    fun `中央値は奇数個で真ん中を返す`() {
        assertEquals(2.0, median(listOf(3.0, 1.0, 2.0)), 0.001)
    }

    @Test
    fun `中央値は偶数個で中央2つの平均を返す`() {
        assertEquals(2.5, median(listOf(1.0, 2.0, 3.0, 4.0)), 0.001)
    }

    @Test
    fun `EWMAは新しい値に重みをつけて平均する`() {
        // acc = 100 → 0.5*110 + 0.5*100 = 105
        assertEquals(105.0, ewma(listOf(100.0, 110.0), alpha = 0.5), 0.001)
    }

    @Test
    fun `ヒステリシスは閾値未満の変化を無視する`() {
        assertEquals(100.0, applyHysteresis(previous = 100.0, candidate = 101.0, threshold = 0.02), 0.001)
    }

    @Test
    fun `ヒステリシスは閾値以上の変化を通す`() {
        assertEquals(103.0, applyHysteresis(previous = 100.0, candidate = 103.0, threshold = 0.02), 0.001)
    }

    @Test
    fun `前回値がなければ候補をそのまま採用する`() {
        assertEquals(103.0, applyHysteresis(previous = null, candidate = 103.0, threshold = 0.02), 0.001)
    }

    @Test
    fun `履歴がなければnullを返す`() {
        assertNull(smoothedOneRepMax(emptyList(), bench))
    }

    @Test
    fun `セッション内の外れ値は中央値によって効きが弱まる`() {
        // 同日3セット。1セットだけ異常に高い記録が混ざっている
        val logs = listOf(
            log(1, 85.0, 9, 2),
            log(1, 85.0, 9, 2),
            log(1, 85.0, 20, 5), // 外れ値
        )
        val normal = estimatedOneRepMax(85.0, 9, 2)
        val result = smoothedOneRepMax(logs, bench)!!
        assertEquals(normal, result, 0.001)
    }

    @Test
    fun `複数セッションは新しいものほど強く効く`() {
        val logs = listOf(
            log(1, 80.0, 8, 2),
            log(8, 90.0, 8, 2),
        )
        val old = estimatedOneRepMax(80.0, 8, 2)
        val recent = estimatedOneRepMax(90.0, 8, 2)
        val result = smoothedOneRepMax(logs, bench, alpha = 0.5)!!
        assertTrue(result > old, "新しい記録が反映されていない")
        assertTrue(result < recent, "平滑化されずに直近値そのものになっている")
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*SmoothingTest*'`
Expected: コンパイルエラー（`median` などが未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Smoothing.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlin.math.abs

/** 外れ値に強い代表値。セッション内の1セットだけ異常な記録に引きずられないために使う。 */
internal fun median(values: List<Double>): Double {
    require(values.isNotEmpty()) { "空のリストの中央値は定義できない" }
    val sorted = values.sorted()
    val n = sorted.size
    return if (n % 2 == 1) sorted[n / 2] else (sorted[n / 2 - 1] + sorted[n / 2]) / 2.0
}

/** 指数移動平均。values は古い順に並んでいる前提。 */
internal fun ewma(values: List<Double>, alpha: Double): Double {
    require(values.isNotEmpty()) { "空のリストのEWMAは定義できない" }
    require(alpha in 0.0..1.0) { "alpha は0〜1の範囲: $alpha" }
    var acc = values.first()
    for (v in values.drop(1)) {
        acc = alpha * v + (1 - alpha) * acc
    }
    return acc
}

/** 変化が閾値未満なら前回値を維持する。重量が毎回ちらつくのを防ぐ。 */
internal fun applyHysteresis(previous: Double?, candidate: Double, threshold: Double): Double {
    if (previous == null || previous == 0.0) return candidate
    val change = abs(candidate - previous) / previous
    return if (change < threshold) previous else candidate
}

/**
 * 指定種目の平滑化された推定1RM。履歴が無ければ null。
 *
 * 手順: セッション（同一日）ごとに全セットの推定1RMの中央値を取り、
 * それらを古い順に EWMA へ通し、最後にヒステリシスを掛ける。
 */
fun smoothedOneRepMax(
    logs: List<SetLog>,
    exerciseId: ExerciseId,
    previousPublished: Double? = null,
    alpha: Double = 0.3,
    hysteresis: Double = 0.02,
): Double? {
    val bySession = logs
        .filter { it.exerciseId == exerciseId }
        .groupBy { it.date }
    if (bySession.isEmpty()) return null

    val perSession = bySession.entries
        .sortedBy { it.key }
        .map { (_, sets) -> median(sets.map { estimatedOneRepMax(it.weightKg, it.reps, it.rir) }) }

    return applyHysteresis(previousPublished, ewma(perSession, alpha), hysteresis)
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*SmoothingTest*'`
Expected: PASS（9件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Smoothing.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SmoothingTest.kt
git commit -m "feat: 推定1RMの平滑化とヒステリシスを実装する"
```

---

### Task 5: バリエーションの対メイン係数の学習

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/VariationRatio.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/VariationRatioTest.kt`

**Interfaces:**
- Consumes: Task 2 の `Exercise` / `SetLog`、Task 4 の `smoothedOneRepMax`
- Produces: `fun ratioToMain(logs: List<SetLog>, variation: Exercise, mainExerciseId: ExerciseId, minSessions: Int = 3): Double`

**なぜ必要か:** ラーセンプレスやテンポは通常フォームより挙がらない。同じ推定1RMの物差しに乗せるため、比率で換算する。初期値はシードが持ち、実績が溜まったら実測値で上書きされる。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/VariationRatioTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.LocalDate
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class VariationRatioTest {
    private val benchId = ExerciseId("bench")
    private val larsenId = ExerciseId("larsen")

    private val larsen = Exercise(
        id = larsenId,
        name = "ラーセンプレス",
        kind = ExerciseKind.VARIATION,
        stimulus = mapOf(MuscleRegion.CHEST_MID to 1.0),
        mainLift = MainLift.BENCH,
        defaultRatioToMain = 0.85,
    )

    private fun log(id: ExerciseId, day: Int, weight: Double) = SetLog(
        id = "${id.value}-$day",
        date = LocalDate(2026, 8, day),
        exerciseId = id,
        weightKg = weight,
        reps = 8,
        rir = 2,
    )

    @Test
    fun `実績が足りなければ初期値を返す`() {
        val logs = listOf(log(benchId, 1, 85.0), log(larsenId, 2, 75.0))
        assertEquals(0.85, ratioToMain(logs, larsen, benchId), 0.001)
    }

    @Test
    fun `初期値が無く実績も足りなければ1を返す`() {
        val noDefault = larsen.copy(defaultRatioToMain = null)
        assertEquals(1.0, ratioToMain(emptyList(), noDefault, benchId), 0.001)
    }

    @Test
    fun `実績が足りれば実測値で上書きされる`() {
        val logs = listOf(
            log(benchId, 1, 100.0), log(benchId, 8, 100.0), log(benchId, 15, 100.0),
            log(larsenId, 2, 80.0), log(larsenId, 9, 80.0), log(larsenId, 16, 80.0),
        )
        // 同じレップ・RIR なので推定1RMの比は重量の比に一致する
        val ratio = ratioToMain(logs, larsen, benchId)
        assertEquals(0.80, ratio, 0.001)
        assertTrue(ratio != 0.85, "初期値のままになっている")
    }

    @Test
    fun `メイン側の実績が無ければ初期値を返す`() {
        val logs = listOf(log(larsenId, 2, 80.0), log(larsenId, 9, 80.0), log(larsenId, 16, 80.0))
        assertEquals(0.85, ratioToMain(logs, larsen, benchId), 0.001)
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*VariationRatioTest*'`
Expected: コンパイルエラー（`ratioToMain` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/VariationRatio.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

/**
 * バリエーション種目の対メイン係数。
 *
 * シードが持つ初期値は「最初の一歩を踏み出すための仮の値」であり正確である必要はない。
 * minSessions 回こなせば実測値に置き換わる。
 */
fun ratioToMain(
    logs: List<SetLog>,
    variation: Exercise,
    mainExerciseId: ExerciseId,
    minSessions: Int = 3,
): Double {
    val fallback = variation.defaultRatioToMain ?: 1.0

    val variationSessions = logs.filter { it.exerciseId == variation.id }.map { it.date }.distinct().size
    if (variationSessions < minSessions) return fallback

    val variationOneRm = smoothedOneRepMax(logs, variation.id) ?: return fallback
    val mainOneRm = smoothedOneRepMax(logs, mainExerciseId) ?: return fallback
    if (mainOneRm <= 0.0) return fallback

    return variationOneRm / mainOneRm
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*VariationRatioTest*'`
Expected: PASS（4件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/VariationRatio.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/VariationRatioTest.kt
git commit -m "feat: バリエーションの対メイン係数を実績から学習する"
```

---

### Task 6: 週内スロットのテンプレート

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Slots.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SlotsTest.kt`

**Interfaces:**
- Consumes: Task 1 の `:engine` モジュール
- Produces: `enum class SlotRole { VARIATION, STANDARD, HEAVY }`, `data class SlotTemplate(val role: SlotRole, val intensityPct: Double, val sets: Int, val targetRir: Int)`, `fun slotTemplates(frequencyPerWeek: Int): List<SlotTemplate>`

**なぜこの数値か:** 現行のベンチ（1RM 約105kg想定で 80 / 85 / 90〜95kg）は概ね 76% / 81% / 88% に対応する。RIR は常に2で固定し、高強度スロットのみ1とする。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SlotsTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class SlotsTest {
    @Test
    fun `週3回なら軽中重の3スロットになる`() {
        val templates = slotTemplates(3)
        assertEquals(3, templates.size)
        assertEquals(listOf(SlotRole.VARIATION, SlotRole.STANDARD, SlotRole.HEAVY), templates.map { it.role })
    }

    @Test
    fun `強度は昇順に並んでいる`() {
        val intensities = slotTemplates(3).map { it.intensityPct }
        assertEquals(intensities.sorted(), intensities)
    }

    @Test
    fun `高強度スロットだけ目標RIRが1になる`() {
        val templates = slotTemplates(3)
        assertEquals(listOf(2, 2, 1), templates.map { it.targetRir })
    }

    @Test
    fun `週1回なら標準スロットのみ`() {
        val templates = slotTemplates(1)
        assertEquals(1, templates.size)
        assertEquals(SlotRole.STANDARD, templates.first().role)
    }

    @Test
    fun `週5回以上でも4スロット構成を使い回す`() {
        assertEquals(4, slotTemplates(5).size)
        assertEquals(4, slotTemplates(6).size)
    }

    @Test
    fun `頻度0以下は不正`() {
        assertFailsWith<IllegalArgumentException> { slotTemplates(0) }
    }

    @Test
    fun `全スロットの強度は現実的な範囲に収まる`() {
        for (freq in 1..6) {
            for (t in slotTemplates(freq)) {
                assertTrue(t.intensityPct in 0.70..0.92, "強度が範囲外: ${t.intensityPct}")
                assertTrue(t.sets in 1..6, "セット数が範囲外: ${t.sets}")
            }
        }
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*SlotsTest*'`
Expected: コンパイルエラー（`slotTemplates` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Slots.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

/**
 * VARIATION … バリエーション種目で技術と弱点を突く日
 * STANDARD  … 通常フォームでボリュームを積む日
 * HEAVY     … 高強度で神経系に効かせる日
 */
enum class SlotRole { VARIATION, STANDARD, HEAVY }

/**
 * @param intensityPct 推定1RMに対する割合
 * @param targetRir 止め時の指示。レップ数は指示しない（その日の状態が決める）
 */
data class SlotTemplate(
    val role: SlotRole,
    val intensityPct: Double,
    val sets: Int,
    val targetRir: Int,
)

private val FREQ_1 = listOf(
    SlotTemplate(SlotRole.STANDARD, 0.81, 4, 2),
)

private val FREQ_2 = listOf(
    SlotTemplate(SlotRole.STANDARD, 0.81, 4, 2),
    SlotTemplate(SlotRole.HEAVY, 0.88, 3, 1),
)

private val FREQ_3 = listOf(
    SlotTemplate(SlotRole.VARIATION, 0.76, 4, 2),
    SlotTemplate(SlotRole.STANDARD, 0.81, 4, 2),
    SlotTemplate(SlotRole.HEAVY, 0.88, 3, 1),
)

private val FREQ_4 = listOf(
    SlotTemplate(SlotRole.VARIATION, 0.76, 4, 2),
    SlotTemplate(SlotRole.STANDARD, 0.81, 4, 2),
    SlotTemplate(SlotRole.VARIATION, 0.78, 4, 2),
    SlotTemplate(SlotRole.HEAVY, 0.88, 3, 1),
)

/**
 * 週の頻度に対する強度配分。
 *
 * RIR は調整ダイヤルではなくガードレールなので固定し、
 * 動かすのはスロットごとの強度帯とバリエーションの有無。
 * 週5回以上は4スロット構成を循環させる。
 */
fun slotTemplates(frequencyPerWeek: Int): List<SlotTemplate> {
    require(frequencyPerWeek >= 1) { "頻度は1以上である必要がある: $frequencyPerWeek" }
    return when (frequencyPerWeek) {
        1 -> FREQ_1
        2 -> FREQ_2
        3 -> FREQ_3
        else -> FREQ_4
    }
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*SlotsTest*'`
Expected: PASS（7件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Slots.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/SlotsTest.kt
git commit -m "feat: 週内スロットのテンプレートを定義する"
```

---

### Task 7: 実重量の算出と丸め

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/WorkWeight.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/WorkWeightTest.kt`

**Interfaces:**
- Consumes: Task 6 の `SlotTemplate`
- Produces:
  - `fun roundToIncrement(valueKg: Double, incrementKg: Double): Double`
  - `fun workWeight(oneRepMaxKg: Double, intensityPct: Double, ratioToMain: Double, incrementKg: Double): Double`
  - `data class PlannedSet(val exerciseId: ExerciseId, val weightKg: Double?, val sets: Int, val targetRir: Int, val role: SlotRole?)`

`PlannedSet.weightKg` が nullable なのは、履歴の無い種目では重量を推定できないため。数字を捏造せず null を返し、初回だけユーザーが決める。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/WorkWeightTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlin.test.Test
import kotlin.test.assertEquals

class WorkWeightTest {
    @Test
    fun `増加単位に丸める`() {
        assertEquals(82.5, roundToIncrement(83.1, 2.5), 0.001)
        assertEquals(85.0, roundToIncrement(84.0, 2.5), 0.001)
        assertEquals(85.0, roundToIncrement(83.0, 5.0), 0.001)
    }

    @Test
    fun `強度から重量を出す`() {
        // 1RM 105kg の 81% = 85.05 → 2.5kg刻みで 85.0
        assertEquals(85.0, workWeight(105.0, 0.81, ratioToMain = 1.0, incrementKg = 2.5), 0.001)
    }

    @Test
    fun `バリエーション係数が掛かる`() {
        // 105 * 0.76 * 0.85 = 67.83 → 67.5
        assertEquals(67.5, workWeight(105.0, 0.76, ratioToMain = 0.85, incrementKg = 2.5), 0.001)
    }

    @Test
    fun `予定セットは重量を欠損値として持てる`() {
        val set = PlannedSet(
            exerciseId = ExerciseId("bench"),
            weightKg = null,
            sets = 4,
            targetRir = 2,
            role = SlotRole.STANDARD,
        )
        assertEquals(null, set.weightKg)
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*WorkWeightTest*'`
Expected: コンパイルエラー（`roundToIncrement` などが未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/WorkWeight.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlin.math.roundToLong

/** ジムのプレート構成に合わせて増加単位へ丸める。 */
fun roundToIncrement(valueKg: Double, incrementKg: Double): Double {
    require(incrementKg > 0) { "増加単位は正の数: $incrementKg" }
    return (valueKg / incrementKg).roundToLong() * incrementKg
}

/**
 * 実際に使う重量。
 *
 * 推定1RM × スロットの強度帯 × バリエーション係数 を、増加単位へ丸める。
 * 1RMが上がれば全スロットの重量が自動的に追随する。
 */
fun workWeight(
    oneRepMaxKg: Double,
    intensityPct: Double,
    ratioToMain: Double,
    incrementKg: Double,
): Double = roundToIncrement(oneRepMaxKg * intensityPct * ratioToMain, incrementKg)

/**
 * その日にやることの1単位。
 *
 * @param weightKg 履歴が足りず推定できない場合は null。初回だけユーザーが決める
 * @param targetRir 止め時の指示。レップ数は指示しない
 * @param role メイン種目のみ。補助種目は null
 */
data class PlannedSet(
    val exerciseId: ExerciseId,
    val weightKg: Double?,
    val sets: Int,
    val targetRir: Int,
    val role: SlotRole? = null,
)
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*WorkWeightTest*'`
Expected: PASS（4件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/WorkWeight.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/WorkWeightTest.kt
git commit -m "feat: 強度帯から実重量を算出する"
```

---

### Task 8: メインのカバレッジと残差

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Residual.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/ResidualTest.kt`

**Interfaces:**
- Consumes: Task 2 の `Exercise` / `MuscleRegion`、Task 7 の `PlannedSet`
- Produces:
  - `fun stimulusCoverage(sets: List<PlannedSet>, pool: List<Exercise>): Map<MuscleRegion, Double>`
  - `fun residual(target: Map<MuscleRegion, Double>, coverage: Map<MuscleRegion, Double>): Map<MuscleRegion, Double>`

**なぜ必要か:** 補助種目の選択を「自分で設計する」から「メインの残差を解く」に変えるための土台。ベンチが大胸筋中部を埋めているなら、補助が埋めるべきは上部と下部だけになる。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/ResidualTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class ResidualTest {
    private val bench = Exercise(
        id = ExerciseId("bench"),
        name = "ベンチプレス",
        kind = ExerciseKind.MAIN,
        stimulus = mapOf(
            MuscleRegion.CHEST_MID to 1.0,
            MuscleRegion.TRICEPS_LATERAL to 0.5,
        ),
        mainLift = MainLift.BENCH,
    )
    private val pool = listOf(bench)

    @Test
    fun `セット数に寄与度を掛けてカバレッジになる`() {
        val planned = listOf(PlannedSet(bench.id, 85.0, sets = 4, targetRir = 2, role = SlotRole.STANDARD))
        val coverage = stimulusCoverage(planned, pool)
        assertEquals(4.0, coverage[MuscleRegion.CHEST_MID]!!, 0.001)
        assertEquals(2.0, coverage[MuscleRegion.TRICEPS_LATERAL]!!, 0.001)
    }

    @Test
    fun `プールに無い種目は無視される`() {
        val planned = listOf(PlannedSet(ExerciseId("unknown"), 50.0, sets = 3, targetRir = 2))
        assertTrue(stimulusCoverage(planned, pool).isEmpty())
    }

    @Test
    fun `残差は目標からカバレッジを引いた値`() {
        val target = mapOf(MuscleRegion.CHEST_MID to 12.0, MuscleRegion.CHEST_UPPER to 8.0)
        val coverage = mapOf(MuscleRegion.CHEST_MID to 4.0)
        val r = residual(target, coverage)
        assertEquals(8.0, r[MuscleRegion.CHEST_MID]!!, 0.001)
        assertEquals(8.0, r[MuscleRegion.CHEST_UPPER]!!, 0.001)
    }

    @Test
    fun `カバレッジが目標を超えた区分は残差に含まれない`() {
        val target = mapOf(MuscleRegion.CHEST_MID to 4.0)
        val coverage = mapOf(MuscleRegion.CHEST_MID to 6.0)
        assertTrue(residual(target, coverage).isEmpty())
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*ResidualTest*'`
Expected: コンパイルエラー（`stimulusCoverage` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Residual.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

/** 予定セットが各筋区分に与える刺激量（セット換算）。 */
fun stimulusCoverage(sets: List<PlannedSet>, pool: List<Exercise>): Map<MuscleRegion, Double> {
    val byId = pool.associateBy { it.id }
    val acc = mutableMapOf<MuscleRegion, Double>()
    for (planned in sets) {
        val exercise = byId[planned.exerciseId] ?: continue
        for ((region, contribution) in exercise.stimulus) {
            acc[region] = (acc[region] ?: 0.0) + contribution * planned.sets
        }
    }
    return acc
}

/** 目標に対して埋まっていない分。0以下の区分は落とす。 */
fun residual(
    target: Map<MuscleRegion, Double>,
    coverage: Map<MuscleRegion, Double>,
): Map<MuscleRegion, Double> =
    target
        .mapValues { (region, want) -> want - (coverage[region] ?: 0.0) }
        .filterValues { it > 0.0 }
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*ResidualTest*'`
Expected: PASS（4件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Residual.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/ResidualTest.kt
git commit -m "feat: メインのカバレッジと残差を計算する"
```

---

### Task 9: 補助種目の割り当て

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/AccessorySelection.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/AccessorySelectionTest.kt`

**Interfaces:**
- Consumes: Task 2 の `Exercise` / `SetLog`、Task 8 の残差
- Produces: `fun selectAccessories(residual: Map<MuscleRegion, Double>, pool: List<Exercise>, logs: List<SetLog>, date: LocalDate, slots: Int = 3, setsPerAccessory: Int = 3): List<ExerciseId>`

**アルゴリズム:** 残差の大きい筋区分から貪欲に埋める。ただし48時間以内に刺激済みの筋区分は先に除外する。同じ筋区分を狙う種目が複数あるときは、最後に使ってから最も間隔が空いているものを選ぶ（バリエーションが自動で回る）。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/AccessorySelectionTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.LocalDate
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class AccessorySelectionTest {
    private fun accessory(id: String, vararg regions: Pair<MuscleRegion, Double>) = Exercise(
        id = ExerciseId(id),
        name = id,
        kind = ExerciseKind.ACCESSORY,
        stimulus = regions.toMap(),
    )

    private val incline = accessory("incline", MuscleRegion.CHEST_UPPER to 1.0)
    private val inclineDb = accessory("incline_db", MuscleRegion.CHEST_UPPER to 1.0)
    private val dip = accessory("dip", MuscleRegion.CHEST_LOWER to 1.0)
    private val curl = accessory("curl", MuscleRegion.BICEPS to 1.0)
    private val pool = listOf(incline, inclineDb, dip, curl)

    private val today = LocalDate(2026, 8, 16)

    private fun log(id: ExerciseId, date: LocalDate) = SetLog(
        id = "${id.value}-$date",
        date = date,
        exerciseId = id,
        weightKg = 20.0,
        reps = 10,
        rir = 2,
    )

    @Test
    fun `残差の大きい区分から埋める`() {
        val residual = mapOf(
            MuscleRegion.CHEST_UPPER to 8.0,
            MuscleRegion.BICEPS to 2.0,
        )
        val chosen = selectAccessories(residual, pool, logs = emptyList(), date = today, slots = 1)
        assertEquals(listOf(ExerciseId("incline")), chosen)
    }

    @Test
    fun `48時間以内に刺激済みの区分は選ばれない`() {
        val residual = mapOf(
            MuscleRegion.CHEST_UPPER to 8.0,
            MuscleRegion.BICEPS to 2.0,
        )
        val logs = listOf(log(incline.id, LocalDate(2026, 8, 15)))
        val chosen = selectAccessories(residual, pool, logs, date = today, slots = 1)
        assertEquals(listOf(ExerciseId("curl")), chosen)
    }

    @Test
    fun `同じ区分の種目は最後に使ってから間隔が空いている方を選ぶ`() {
        val residual = mapOf(MuscleRegion.CHEST_UPPER to 8.0)
        val logs = listOf(
            log(incline.id, LocalDate(2026, 8, 10)),
            log(inclineDb.id, LocalDate(2026, 8, 1)),
        )
        val chosen = selectAccessories(residual, pool, logs, date = today, slots = 1)
        assertEquals(listOf(ExerciseId("incline_db")), chosen)
    }

    @Test
    fun `同じ種目を1セッションで二度選ばない`() {
        val residual = mapOf(MuscleRegion.CHEST_UPPER to 30.0)
        val chosen = selectAccessories(residual, pool, logs = emptyList(), date = today, slots = 3)
        assertEquals(chosen.size, chosen.distinct().size)
    }

    @Test
    fun `埋めるべき残差が無ければ何も選ばない`() {
        val chosen = selectAccessories(emptyMap(), pool, logs = emptyList(), date = today, slots = 3)
        assertTrue(chosen.isEmpty())
    }

    @Test
    fun `メイン種目は補助として選ばれない`() {
        val main = Exercise(
            id = ExerciseId("bench"),
            name = "ベンチプレス",
            kind = ExerciseKind.MAIN,
            stimulus = mapOf(MuscleRegion.CHEST_MID to 1.0),
            mainLift = MainLift.BENCH,
        )
        val residual = mapOf(MuscleRegion.CHEST_MID to 10.0)
        val chosen = selectAccessories(residual, pool + main, logs = emptyList(), date = today, slots = 2)
        assertTrue(ExerciseId("bench") !in chosen)
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*AccessorySelectionTest*'`
Expected: コンパイルエラー（`selectAccessories` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/AccessorySelection.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.DatePeriod
import kotlinx.datetime.LocalDate
import kotlinx.datetime.minus

private val EPOCH = LocalDate(1970, 1, 1)

/**
 * 残差を埋める補助種目を選ぶ。
 *
 * 1. 48時間以内に刺激された筋区分を候補から外す
 * 2. 残差の大きい区分から貪欲に選ぶ
 * 3. 同じ区分を狙う種目が複数あれば、最後に使ってから最も間隔が空いているものを選ぶ
 */
fun selectAccessories(
    residual: Map<MuscleRegion, Double>,
    pool: List<Exercise>,
    logs: List<SetLog>,
    date: LocalDate,
    slots: Int = 3,
    setsPerAccessory: Int = 3,
): List<ExerciseId> {
    if (slots <= 0) return emptyList()

    val byId = pool.associateBy { it.id }
    val cutoff = date.minus(DatePeriod(days = 2))
    val blockedRegions = logs
        .filter { it.date >= cutoff }
        .flatMap { byId[it.exerciseId]?.stimulus?.keys.orEmpty() }
        .toSet()

    val remaining = residual
        .filterKeys { it !in blockedRegions }
        .filterValues { it > 0.0 }
        .toMutableMap()

    val accessories = pool.filter { it.kind == ExerciseKind.ACCESSORY }
    val lastUsed: Map<ExerciseId, LocalDate> = logs
        .groupBy { it.exerciseId }
        .mapValues { (_, sets) -> sets.maxOf { it.date } }

    val chosen = mutableListOf<ExerciseId>()

    repeat(slots) {
        val topRegion = remaining.maxByOrNull { it.value }?.key ?: return@repeat

        val candidate = accessories
            .filter { it.id !in chosen && (it.stimulus[topRegion] ?: 0.0) > 0.0 }
            .minByOrNull { lastUsed[it.id] ?: EPOCH }

        if (candidate == null) {
            // その区分を埋められる未使用の種目が無い。区分ごと諦める
            remaining.remove(topRegion)
            return@repeat
        }

        chosen += candidate.id
        for ((region, contribution) in candidate.stimulus) {
            val current = remaining[region] ?: continue
            remaining[region] = current - contribution * setsPerAccessory
        }
        remaining.keys.filter { (remaining[it] ?: 0.0) <= 0.0 }.forEach { remaining.remove(it) }
    }

    return chosen
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*AccessorySelectionTest*'`
Expected: PASS（6件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/AccessorySelection.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/AccessorySelectionTest.kt
git commit -m "feat: 残差から補助種目を自動割り当てする"
```

---

### Task 10: コンディション補正（睡眠 → 目標RIR）

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Condition.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/ConditionTest.kt`

**Interfaces:**
- Consumes: Task 2 の `DailyCondition`、Task 4 の `median`（同一パッケージ内の `internal` 関数）
- Produces:
  - `fun rirAdjustment(conditions: List<DailyCondition>, date: LocalDate, baselineDays: Int = 14, deficitHours: Double = 1.5): Int`
  - `fun bodyWeightTrendKgPerWeek(conditions: List<DailyCondition>, date: LocalDate, windowDays: Int = 21): Double?`

**なぜ必要か:** 睡眠が短い日は自動で軽くする。体重トレンドは Task 11 の停滞判定で使う。減量中の停滞とオーバーリーチによる停滞は記録だけ見ると同じ形なので、体重が無いと見分けられない。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/ConditionTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.DatePeriod
import kotlinx.datetime.LocalDate
import kotlinx.datetime.minus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

class ConditionTest {
    private val today = LocalDate(2026, 8, 16)

    private fun history(sleepHours: List<Double>, weights: List<Double> = emptyList()): List<DailyCondition> =
        sleepHours.indices.map { i ->
            DailyCondition(
                date = today.minus(DatePeriod(days = sleepHours.size - 1 - i)),
                sleepHours = sleepHours[i],
                bodyWeightKg = weights.getOrNull(i),
            )
        }

    @Test
    fun `平常どおりの睡眠なら補正しない`() {
        val conditions = history(List(15) { 7.0 })
        assertEquals(0, rirAdjustment(conditions, today))
    }

    @Test
    fun `睡眠が普段より大幅に短ければ目標RIRを1上げる`() {
        val conditions = history(List(14) { 7.0 } + listOf(4.5))
        assertEquals(1, rirAdjustment(conditions, today))
    }

    @Test
    fun `当日の睡眠データが無ければ補正しない`() {
        val conditions = history(List(14) { 7.0 }) // 当日分が無い
        assertEquals(0, rirAdjustment(conditions, today))
    }

    @Test
    fun `基準を作るだけの履歴が無ければ補正しない`() {
        val conditions = history(listOf(4.0))
        assertEquals(0, rirAdjustment(conditions, today))
    }

    @Test
    fun `体重が減っていればトレンドは負になる`() {
        val weights = List(21) { 75.0 - it * 0.05 } // 1日あたり -0.05kg
        val conditions = history(List(21) { 7.0 }, weights)
        val trend = bodyWeightTrendKgPerWeek(conditions, today)!!
        assertTrue(trend < -0.1, "減量中と判定されていない: $trend")
    }

    @Test
    fun `体重が横ばいならトレンドはほぼ0`() {
        val conditions = history(List(21) { 7.0 }, List(21) { 75.0 })
        assertEquals(0.0, bodyWeightTrendKgPerWeek(conditions, today)!!, 0.01)
    }

    @Test
    fun `体重データが足りなければnull`() {
        val conditions = history(List(21) { 7.0 })
        assertNull(bodyWeightTrendKgPerWeek(conditions, today))
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*ConditionTest*'`
Expected: コンパイルエラー（`rirAdjustment` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Condition.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.DatePeriod
import kotlinx.datetime.LocalDate
import kotlinx.datetime.minus

private const val MIN_BASELINE_SAMPLES = 5
private const val MIN_TREND_SAMPLES = 5

/**
 * 睡眠不足の日は目標RIRを上げて自動的に軽くする。
 *
 * 基準は直近 baselineDays 日の睡眠の中央値。そこから deficitHours 以上短ければ +1。
 * データが無い場合は補正しない（推測で軽くしない）。
 */
fun rirAdjustment(
    conditions: List<DailyCondition>,
    date: LocalDate,
    baselineDays: Int = 14,
    deficitHours: Double = 1.5,
): Int {
    val todaySleep = conditions.firstOrNull { it.date == date }?.sleepHours ?: return 0

    val from = date.minus(DatePeriod(days = baselineDays))
    val baselineSamples = conditions
        .filter { it.date >= from && it.date < date }
        .mapNotNull { it.sleepHours }
    if (baselineSamples.size < MIN_BASELINE_SAMPLES) return 0

    val baseline = median(baselineSamples)
    return if (todaySleep <= baseline - deficitHours) 1 else 0
}

/**
 * 体重トレンド（kg/週）。最小二乗法の傾きを週換算する。
 *
 * 減量中かどうかの判定に使う。データが足りなければ null を返し、
 * 呼び出し側は「判定できない」として扱う。
 */
fun bodyWeightTrendKgPerWeek(
    conditions: List<DailyCondition>,
    date: LocalDate,
    windowDays: Int = 21,
): Double? {
    val from = date.minus(DatePeriod(days = windowDays))
    val samples = conditions
        .filter { it.date in from..date && it.bodyWeightKg != null }
        .sortedBy { it.date }
        .map { it.date.toEpochDays().toDouble() to it.bodyWeightKg!! }
    if (samples.size < MIN_TREND_SAMPLES) return null

    val n = samples.size
    val meanX = samples.sumOf { it.first } / n
    val meanY = samples.sumOf { it.second } / n
    val numerator = samples.sumOf { (x, y) -> (x - meanX) * (y - meanY) }
    val denominator = samples.sumOf { (x, _) -> (x - meanX) * (x - meanX) }
    if (denominator == 0.0) return 0.0

    return numerator / denominator * 7.0
}
```

`toEpochDays()` は kotlinx-datetime 0.6.1 の `LocalDate` が持つ。返り値は `Int` なので `.toDouble()` で変換している。

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*ConditionTest*'`
Expected: PASS（7件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Condition.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/ConditionTest.kt
git commit -m "feat: 睡眠と体重トレンドによるコンディション補正を実装する"
```

---

### Task 11: デロード提案

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Deload.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/DeloadTest.kt`

**Interfaces:**
- Consumes: Task 4 の `smoothedOneRepMax`、Task 10 の `bodyWeightTrendKgPerWeek`
- Produces:
  - `data class DeloadProposal(val reason: String, val intensityDropPct: Double)`
  - `fun deloadProposal(logs: List<SetLog>, mainExerciseIds: List<ExerciseId>, conditions: List<DailyCondition>, date: LocalDate, stallSessions: Int = 3, intensityDropPct: Double = 0.10): DeloadProposal?`

**重要:** これは提案であって自動適用ではない。重量が黙って下がるとアプリへの信頼が壊れるため、ユーザーが承認する。提案時には根拠を必ず `reason` に載せる。

**減量中は発火させない。** 減量中の停滞は正常であり、そこでデロードを出すのは誤診。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/DeloadTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.DatePeriod
import kotlinx.datetime.LocalDate
import kotlinx.datetime.minus
import kotlin.test.Test
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class DeloadTest {
    private val today = LocalDate(2026, 8, 30)
    private val bench = ExerciseId("bench")

    /** daysAgo 日前に weight を挙げた記録 */
    private fun log(daysAgo: Int, weight: Double) = SetLog(
        id = "bench-$daysAgo",
        date = today.minus(DatePeriod(days = daysAgo)),
        exerciseId = bench,
        weightKg = weight,
        reps = 8,
        rir = 2,
    )

    private fun conditions(weightPerDay: (Int) -> Double): List<DailyCondition> =
        (0..27).map { daysAgo ->
            DailyCondition(
                date = today.minus(DatePeriod(days = daysAgo)),
                bodyWeightKg = weightPerDay(daysAgo),
                sleepHours = 7.0,
            )
        }

    private val stalled = listOf(log(21, 85.0), log(14, 85.0), log(7, 85.0), log(0, 85.0))
    private val improving = listOf(log(21, 80.0), log(14, 82.5), log(7, 85.0), log(0, 87.5))

    @Test
    fun `体重横ばいで1RMが停滞していれば提案する`() {
        val proposal = deloadProposal(stalled, listOf(bench), conditions { 75.0 }, today)
        assertNotNull(proposal)
        assertTrue(proposal.intensityDropPct > 0.0)
    }

    @Test
    fun `提案には根拠が含まれる`() {
        val proposal = deloadProposal(stalled, listOf(bench), conditions { 75.0 }, today)!!
        assertTrue(proposal.reason.isNotBlank(), "根拠が空になっている")
    }

    @Test
    fun `減量中は停滞していても提案しない`() {
        // daysAgo が大きいほど過去 → 過去ほど重い = 減量中
        val cutting = conditions { daysAgo -> 75.0 + daysAgo * 0.05 }
        assertNull(deloadProposal(stalled, listOf(bench), cutting, today))
    }

    @Test
    fun `1RMが伸びていれば提案しない`() {
        assertNull(deloadProposal(improving, listOf(bench), conditions { 75.0 }, today))
    }

    @Test
    fun `セッション数が足りなければ提案しない`() {
        val few = listOf(log(7, 85.0), log(0, 85.0))
        assertNull(deloadProposal(few, listOf(bench), conditions { 75.0 }, today))
    }

    @Test
    fun `体重データが無ければ提案しない`() {
        val noWeight = (0..27).map {
            DailyCondition(date = today.minus(DatePeriod(days = it)), sleepHours = 7.0)
        }
        assertNull(deloadProposal(stalled, listOf(bench), noWeight, today))
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*DeloadTest*'`
Expected: コンパイルエラー（`deloadProposal` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Deload.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.LocalDate

/** 減量中と判定する体重トレンドの閾値（kg/週）。 */
private const val CUTTING_THRESHOLD_KG_PER_WEEK = -0.1

/** 停滞と見なす推定1RMの改善幅の下限。これ未満の伸びは停滞扱い。 */
private const val STALL_TOLERANCE = 0.005

/**
 * @param reason ユーザーに表示する根拠。黙って重量を下げないための情報
 * @param intensityDropPct 全スロットの強度帯に掛ける低下率（0.10 なら10%減）
 */
data class DeloadProposal(
    val reason: String,
    val intensityDropPct: Double,
)

/**
 * デロードの提案。適用はしない。承認するのはユーザー。
 *
 * 発火条件は「体重トレンドが横ばい以上」かつ「推定1RMが stallSessions 回連続で更新されない」。
 * 減量中の停滞は正常なので発火させない。体重が分からないときも発火させない
 * （減量による停滞とオーバーリーチによる停滞を見分けられないため）。
 */
fun deloadProposal(
    logs: List<SetLog>,
    mainExerciseIds: List<ExerciseId>,
    conditions: List<DailyCondition>,
    date: LocalDate,
    stallSessions: Int = 3,
    intensityDropPct: Double = 0.10,
): DeloadProposal? {
    val trend = bodyWeightTrendKgPerWeek(conditions, date) ?: return null
    if (trend < CUTTING_THRESHOLD_KG_PER_WEEK) return null

    val stalledLifts = mainExerciseIds.filter { isStalled(logs, it, stallSessions) }
    if (stalledLifts.isEmpty()) return null

    val sleepNote = recentSleepAverage(conditions, date)
        ?.let { "、睡眠 %.1fh 平均".format(it) }
        .orEmpty()

    val reason = "推定1RMが${stallSessions}セッション停滞（${stalledLifts.size}種目）" +
        "、体重トレンド %+.2fkg/週".format(trend) + sleepNote

    return DeloadProposal(reason = reason, intensityDropPct = intensityDropPct)
}

/** 直近 stallSessions 回のセッションで推定1RMが実質的に伸びていないか。 */
private fun isStalled(logs: List<SetLog>, exerciseId: ExerciseId, stallSessions: Int): Boolean {
    val perSession = logs
        .filter { it.exerciseId == exerciseId }
        .groupBy { it.date }
        .entries
        .sortedBy { it.key }
        .map { (_, sets) -> median(sets.map { estimatedOneRepMax(it.weightKg, it.reps, it.rir) }) }

    if (perSession.size < stallSessions + 1) return false

    val window = perSession.takeLast(stallSessions + 1)
    val reference = window.first()
    if (reference <= 0.0) return false

    return window.drop(1).none { it > reference * (1 + STALL_TOLERANCE) }
}

private fun recentSleepAverage(conditions: List<DailyCondition>, date: LocalDate): Double? {
    val samples = conditions.filter { it.date <= date }.mapNotNull { it.sleepHours }.takeLast(7)
    return if (samples.isEmpty()) null else samples.average()
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*DeloadTest*'`
Expected: PASS（6件）

- [ ] **Step 5: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Deload.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/DeloadTest.kt
git commit -m "feat: デロード提案の判定を実装する"
```

---

### Task 12: `plan()` の統合

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Plan.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/PlanTest.kt`

**Interfaces:**
- Consumes: Task 2〜11 のすべて
- Produces:
  - `data class PlanInput(...)`, `data class PlannedSession(...)`
  - `fun plan(input: PlanInput): PlannedSession`
  - `internal fun weekStart(date: LocalDate): LocalDate`
  - `internal fun sessionIndexInWeek(logs: List<SetLog>, date: LocalDate): Int`

これがエンジンの唯一の入口。UI も DB も Health Connect も、この関数だけを呼ぶ。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/PlanTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.DatePeriod
import kotlinx.datetime.LocalDate
import kotlinx.datetime.minus
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

class PlanTest {
    private val monday = LocalDate(2026, 8, 17) // 月曜
    private val benchId = ExerciseId("bench")
    private val squatId = ExerciseId("squat")
    private val deadliftId = ExerciseId("deadlift")
    private val inclineId = ExerciseId("incline")

    private val bench = Exercise(benchId, "ベンチプレス", ExerciseKind.MAIN,
        mapOf(MuscleRegion.CHEST_MID to 1.0, MuscleRegion.TRICEPS_LATERAL to 0.5),
        mainLift = MainLift.BENCH)
    private val squat = Exercise(squatId, "スクワット", ExerciseKind.MAIN,
        mapOf(MuscleRegion.QUAD to 1.0, MuscleRegion.GLUTE to 0.5),
        mainLift = MainLift.SQUAT)
    private val deadlift = Exercise(deadliftId, "デッドリフト", ExerciseKind.MAIN,
        mapOf(MuscleRegion.HAMSTRING to 1.0, MuscleRegion.ERECTOR to 1.0),
        mainLift = MainLift.DEADLIFT)
    private val incline = Exercise(inclineId, "インクラインダンベルプレス", ExerciseKind.ACCESSORY,
        mapOf(MuscleRegion.CHEST_UPPER to 1.0))

    private val pool = listOf(bench, squat, deadlift, incline)

    private val weeklyTarget = mapOf(
        MuscleRegion.CHEST_MID to 12.0,
        MuscleRegion.CHEST_UPPER to 10.0,
        MuscleRegion.QUAD to 12.0,
    )

    /** 過去に十分な履歴を作る（推定1RMが立つように） */
    private fun history(): List<SetLog> = listOf(7, 14, 21).flatMap { daysAgo ->
        listOf(
            SetLog("b-$daysAgo", monday.minus(DatePeriod(days = daysAgo)), benchId, 85.0, 8, 2),
            SetLog("s-$daysAgo", monday.minus(DatePeriod(days = daysAgo)), squatId, 110.0, 8, 2),
            SetLog("d-$daysAgo", monday.minus(DatePeriod(days = daysAgo)), deadliftId, 140.0, 8, 2),
        )
    }

    private fun input(
        logs: List<SetLog> = history(),
        date: LocalDate = monday,
        deloadAccepted: Boolean = false,
        conditions: List<DailyCondition> = emptyList(),
    ) = PlanInput(
        logs = logs,
        pool = pool,
        weeklyTarget = weeklyTarget,
        frequencyPerWeek = 3,
        conditions = conditions,
        date = date,
        deloadAccepted = deloadAccepted,
    )

    @Test
    fun `メイン3種目すべてにスロットが割り当てられる`() {
        val session = plan(input())
        assertEquals(setOf(benchId, squatId, deadliftId), session.main.map { it.exerciseId }.toSet())
    }

    @Test
    fun `週の1本目はバリエーション強度になる`() {
        val session = plan(input())
        assertTrue(session.main.all { it.role == SlotRole.VARIATION })
    }

    @Test
    fun `週の2本目は標準強度になる`() {
        // 月曜に記録済みの状態で火曜を計画する
        val mondayLogs = listOf(SetLog("m", monday, benchId, 80.0, 8, 2))
        val session = plan(input(logs = history() + mondayLogs, date = monday.plusDays(1)))
        assertTrue(session.main.all { it.role == SlotRole.STANDARD })
    }

    @Test
    fun `履歴のない種目は重量がnullになる`() {
        val session = plan(input(logs = emptyList()))
        assertTrue(session.main.all { it.weightKg == null })
    }

    @Test
    fun `残差を埋める補助種目が付く`() {
        val session = plan(input())
        assertTrue(session.accessories.any { it.exerciseId == inclineId })
    }

    @Test
    fun `睡眠不足の日は目標RIRが1上がる`() {
        val conditions = (0..14).map {
            DailyCondition(
                date = monday.minus(DatePeriod(days = it)),
                sleepHours = if (it == 0) 4.0 else 7.5,
            )
        }
        val normal = plan(input()).main.first { it.exerciseId == benchId }.targetRir
        val tired = plan(input(conditions = conditions)).main.first { it.exerciseId == benchId }.targetRir
        assertEquals(normal + 1, tired)
    }

    @Test
    fun `デロードを承認すると重量が下がる`() {
        val normal = plan(input()).main.first { it.exerciseId == benchId }.weightKg!!
        val deloaded = plan(input(deloadAccepted = true)).main.first { it.exerciseId == benchId }.weightKg!!
        assertTrue(deloaded < normal, "デロードで重量が下がっていない: $normal → $deloaded")
    }

    @Test
    fun `デロード中もセット数は維持される`() {
        val normal = plan(input()).main.first { it.exerciseId == benchId }.sets
        val deloaded = plan(input(deloadAccepted = true)).main.first { it.exerciseId == benchId }.sets
        assertEquals(normal, deloaded)
    }

    @Test
    fun `停滞していなければデロード提案は付かない`() {
        assertNull(plan(input()).deloadProposal)
    }

    @Test
    fun `体重横ばいで停滞していればデロード提案が付く`() {
        val stalledLogs = listOf(7, 14, 21, 28).flatMap { daysAgo ->
            listOf(SetLog("b-$daysAgo", monday.minus(DatePeriod(days = daysAgo)), benchId, 85.0, 8, 2))
        }
        val conditions = (0..27).map {
            DailyCondition(monday.minus(DatePeriod(days = it)), bodyWeightKg = 75.0, sleepHours = 7.0)
        }
        val session = plan(input(logs = stalledLogs, conditions = conditions))
        assertNotNull(session.deloadProposal)
    }

    @Test
    fun `週の開始は月曜`() {
        assertEquals(monday, weekStart(monday))
        assertEquals(monday, weekStart(monday.plusDays(6)))
        assertEquals(monday.plusDays(7), weekStart(monday.plusDays(7)))
    }

    private fun LocalDate.plusDays(n: Int): LocalDate =
        LocalDate.fromEpochDays(this.toEpochDays() + n)
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*PlanTest*'`
Expected: コンパイルエラー（`plan` / `PlanInput` が未定義）

- [ ] **Step 3: 実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Plan.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import kotlinx.datetime.DayOfWeek
import kotlinx.datetime.LocalDate

/** 補助種目に使う強度。RIR2 で10レップ前後を狙う位置。 */
private const val ACCESSORY_INTENSITY_PCT = 0.71
private const val ACCESSORY_TARGET_RIR = 2
private const val ACCESSORY_SETS = 3

data class PlanInput(
    val logs: List<SetLog>,
    val pool: List<Exercise>,
    val weeklyTarget: Map<MuscleRegion, Double>,
    val frequencyPerWeek: Int,
    val conditions: List<DailyCondition>,
    val date: LocalDate,
    /** ユーザーがデロード提案を承認済みか。承認されるまで重量は下げない */
    val deloadAccepted: Boolean = false,
    val accessorySlots: Int = 3,
)

data class PlannedSession(
    val date: LocalDate,
    val main: List<PlannedSet>,
    val accessories: List<PlannedSet>,
    /** 提案のみ。適用は deloadAccepted 経由でユーザーが決める */
    val deloadProposal: DeloadProposal?,
)

/** 週の開始は月曜。 */
internal fun weekStart(date: LocalDate): LocalDate {
    val offset = date.dayOfWeek.ordinal - DayOfWeek.MONDAY.ordinal
    return LocalDate.fromEpochDays(date.toEpochDays() - offset)
}

/** その週で、この日が何本目のセッションか（0始まり）。曜日の割り当てはエンジンの責務ではない。 */
internal fun sessionIndexInWeek(logs: List<SetLog>, date: LocalDate): Int {
    val start = weekStart(date)
    return logs.map { it.date }
        .filter { it >= start && it < date }
        .distinct()
        .size
}

/**
 * その日のセッションを導出する。
 *
 * 未来のセッションは保存しない。今日のメニューも来週のメニューも、
 * この関数を対象日で呼んだ結果でしかない。だから予定と実績が食い違う状態が発生しない。
 */
fun plan(input: PlanInput): PlannedSession {
    val templates = slotTemplates(input.frequencyPerWeek)
    val template = templates[sessionIndexInWeek(input.logs, input.date) % templates.size]

    val mainIds = input.pool.filter { it.kind == ExerciseKind.MAIN }.map { it.id }
    val proposal = deloadProposal(input.logs, mainIds, input.conditions, input.date)

    val rirBump = rirAdjustment(input.conditions, input.date)
    val intensityScale = if (input.deloadAccepted) 1.0 - (proposal?.intensityDropPct ?: 0.10) else 1.0

    val main = input.pool
        .filter { it.kind == ExerciseKind.MAIN }
        .map { exercise -> plannedMainSet(input, exercise, template, intensityScale, rirBump) }

    val coverage = stimulusCoverage(main, input.pool)
    val perSessionTarget = input.weeklyTarget.mapValues { (_, v) -> v / input.frequencyPerWeek }
    val gaps = residual(perSessionTarget, coverage)

    val accessories = selectAccessories(
        residual = gaps,
        pool = input.pool,
        logs = input.logs,
        date = input.date,
        slots = input.accessorySlots,
        setsPerAccessory = ACCESSORY_SETS,
    ).map { id -> plannedAccessorySet(input, id, intensityScale, rirBump) }

    return PlannedSession(
        date = input.date,
        main = main,
        accessories = accessories,
        deloadProposal = proposal,
    )
}

private fun plannedMainSet(
    input: PlanInput,
    exercise: Exercise,
    template: SlotTemplate,
    intensityScale: Double,
    rirBump: Int,
): PlannedSet {
    // バリエーションスロットなら、同じ mainLift に属する VARIATION を選ぶ
    val useVariation = template.role == SlotRole.VARIATION
    val variation = if (useVariation) pickVariation(input, exercise) else null
    val target = variation ?: exercise

    val oneRm = smoothedOneRepMax(input.logs, exercise.id)
    val ratio = variation?.let { ratioToMain(input.logs, it, exercise.id) } ?: 1.0

    val weight = oneRm?.let {
        workWeight(it, template.intensityPct * intensityScale, ratio, target.incrementKg)
    }

    return PlannedSet(
        exerciseId = target.id,
        weightKg = weight,
        sets = template.sets,
        targetRir = template.targetRir + rirBump,
        role = template.role,
    )
}

/** 同じ mainLift のバリエーションのうち、最後に使ってから最も間隔が空いているもの。無ければ null。 */
private fun pickVariation(input: PlanInput, main: Exercise): Exercise? {
    val lastUsed = input.logs.groupBy { it.exerciseId }.mapValues { (_, s) -> s.maxOf { it.date } }
    return input.pool
        .filter { it.kind == ExerciseKind.VARIATION && it.mainLift == main.mainLift }
        .minByOrNull { lastUsed[it.id] ?: LocalDate(1970, 1, 1) }
}

private fun plannedAccessorySet(
    input: PlanInput,
    exerciseId: ExerciseId,
    intensityScale: Double,
    rirBump: Int,
): PlannedSet {
    val exercise = input.pool.first { it.id == exerciseId }
    val oneRm = smoothedOneRepMax(input.logs, exerciseId)
    val weight = oneRm?.let {
        workWeight(it, ACCESSORY_INTENSITY_PCT * intensityScale, 1.0, exercise.incrementKg)
    }
    return PlannedSet(
        exerciseId = exerciseId,
        weightKg = weight,
        sets = ACCESSORY_SETS,
        targetRir = ACCESSORY_TARGET_RIR + rirBump,
        role = null,
    )
}
```

- [ ] **Step 4: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*PlanTest*'`
Expected: PASS（11件）

- [ ] **Step 5: 全テストを実行する**

Run: `./gradlew :engine:test`
Expected: BUILD SUCCESSFUL

- [ ] **Step 6: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/Plan.kt engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/PlanTest.kt
git commit -m "feat: plan() でエンジンを統合する"
```

---

### Task 13: シードデータ

**Files:**
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/seed/ExerciseSeed.kt`
- Create: `engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/seed/WeeklyTargetSeed.kt`
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/seed/SeedTest.kt`

**Interfaces:**
- Consumes: Task 2 のドメインモデル
- Produces: `object ExerciseSeed { val all: List<Exercise> }`, `object WeeklyTargetSeed { val default: Map<MuscleRegion, Double> }`

**なぜ必要か:** 「メニュー設定が面倒」から始まったのに、自動化を強くするほど初期登録という別の面倒が生まれる。それを潰すのがこのタスク。ユーザーがやるのは「使う種目にチェックを入れる」だけにする。

**注意:** ここに書いた種目リストは出発点であり網羅ではない。対メイン係数も仮の値でよい（Task 5 が実績で上書きする）。足りない種目は使いながら足す。

- [ ] **Step 1: 失敗するテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/seed/SeedTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine.seed

import dev.dyoshyy.liftplan.engine.ExerciseKind
import dev.dyoshyy.liftplan.engine.MainLift
import dev.dyoshyy.liftplan.engine.MuscleRegion
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class SeedTest {
    @Test
    fun `BIG3がメイン種目として含まれる`() {
        val mains = ExerciseSeed.all.filter { it.kind == ExerciseKind.MAIN }.mapNotNull { it.mainLift }
        assertEquals(setOf(MainLift.SQUAT, MainLift.BENCH, MainLift.DEADLIFT), mains.toSet())
    }

    @Test
    fun `種目IDは一意`() {
        val ids = ExerciseSeed.all.map { it.id }
        assertEquals(ids.size, ids.distinct().size)
    }

    @Test
    fun `バリエーションは所属メインと対メイン係数を持つ`() {
        val variations = ExerciseSeed.all.filter { it.kind == ExerciseKind.VARIATION }
        assertTrue(variations.isNotEmpty())
        assertTrue(variations.all { it.mainLift != null }, "所属メインが無いバリエーションがある")
        assertTrue(variations.all { it.defaultRatioToMain != null }, "対メイン係数が無いバリエーションがある")
        assertTrue(variations.all { it.defaultRatioToMain!! in 0.5..1.1 }, "係数が非現実的")
    }

    @Test
    fun `すべての種目が少なくとも1つの筋区分に寄与する`() {
        assertTrue(ExerciseSeed.all.all { it.stimulus.isNotEmpty() })
    }

    @Test
    fun `寄与度は0より大きく1以下`() {
        val values = ExerciseSeed.all.flatMap { it.stimulus.values }
        assertTrue(values.all { it > 0.0 && it <= 1.0 }, "寄与度が範囲外の種目がある")
    }

    @Test
    fun `すべての筋区分を埋められる補助種目が存在する`() {
        val covered = ExerciseSeed.all
            .filter { it.kind == ExerciseKind.ACCESSORY }
            .flatMap { it.stimulus.keys }
            .toSet()
        val missing = MuscleRegion.entries.toSet() - covered
        assertTrue(missing.isEmpty(), "補助種目でカバーできない筋区分がある: $missing")
    }

    @Test
    fun `週目標プリセットは全筋区分に値を持つ`() {
        assertEquals(MuscleRegion.entries.toSet(), WeeklyTargetSeed.default.keys)
        assertTrue(WeeklyTargetSeed.default.values.all { it > 0.0 })
    }
}
```

- [ ] **Step 2: テストを実行して失敗することを確認する**

Run: `./gradlew :engine:test --tests '*SeedTest*'`
Expected: コンパイルエラー（`ExerciseSeed` が未定義）

- [ ] **Step 3: 種目マスタを実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/seed/ExerciseSeed.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine.seed

import dev.dyoshyy.liftplan.engine.Exercise
import dev.dyoshyy.liftplan.engine.ExerciseId
import dev.dyoshyy.liftplan.engine.ExerciseKind
import dev.dyoshyy.liftplan.engine.MainLift
import dev.dyoshyy.liftplan.engine.MuscleRegion as R

/**
 * アプリ同梱の種目マスタ。ユーザーはここから使う種目にチェックを入れるだけでよい。
 *
 * 寄与度は「1セット実施したとき、その筋区分に何セット分の刺激が入るか」。
 * 対メイン係数は仮の値でよい。3セッション分の実績が溜まれば実測値で上書きされる。
 */
object ExerciseSeed {

    private fun main(id: String, name: String, lift: MainLift, increment: Double, stimulus: Map<R, Double>) =
        Exercise(ExerciseId(id), name, ExerciseKind.MAIN, stimulus, increment, lift, null)

    private fun variation(id: String, name: String, lift: MainLift, ratio: Double, increment: Double, stimulus: Map<R, Double>) =
        Exercise(ExerciseId(id), name, ExerciseKind.VARIATION, stimulus, increment, lift, ratio)

    private fun accessory(id: String, name: String, increment: Double, stimulus: Map<R, Double>) =
        Exercise(ExerciseId(id), name, ExerciseKind.ACCESSORY, stimulus, increment, null, null)

    val all: List<Exercise> = listOf(
        // --- メイン ---
        main("squat", "スクワット", MainLift.SQUAT, 2.5,
            mapOf(R.QUAD to 1.0, R.GLUTE to 0.7, R.ADDUCTOR to 0.4, R.ERECTOR to 0.4)),
        main("bench", "ベンチプレス", MainLift.BENCH, 2.5,
            mapOf(R.CHEST_MID to 1.0, R.TRICEPS_LATERAL to 0.5, R.FRONT_DELT to 0.5)),
        main("deadlift", "デッドリフト", MainLift.DEADLIFT, 5.0,
            mapOf(R.HAMSTRING to 1.0, R.GLUTE to 0.8, R.ERECTOR to 1.0, R.TRAP_MID to 0.4, R.FOREARM to 0.4)),

        // --- バリエーション ---
        variation("larsen_press", "ラーセンプレス", MainLift.BENCH, 0.90, 2.5,
            mapOf(R.CHEST_MID to 1.0, R.TRICEPS_LATERAL to 0.5, R.FRONT_DELT to 0.4)),
        variation("tempo_bench", "テンポベンチ", MainLift.BENCH, 0.85, 2.5,
            mapOf(R.CHEST_MID to 1.0, R.TRICEPS_LATERAL to 0.5, R.FRONT_DELT to 0.4)),
        variation("close_grip_bench", "ナローベンチ", MainLift.BENCH, 0.88, 2.5,
            mapOf(R.CHEST_MID to 0.7, R.TRICEPS_LATERAL to 1.0, R.TRICEPS_LONG to 0.6)),
        variation("pause_squat", "ポーズスクワット", MainLift.SQUAT, 0.88, 2.5,
            mapOf(R.QUAD to 1.0, R.GLUTE to 0.7, R.ADDUCTOR to 0.4, R.ERECTOR to 0.4)),
        variation("front_squat", "フロントスクワット", MainLift.SQUAT, 0.80, 2.5,
            mapOf(R.QUAD to 1.0, R.GLUTE to 0.4, R.ERECTOR to 0.5, R.ABS to 0.4)),
        variation("deficit_deadlift", "デフィシットデッドリフト", MainLift.DEADLIFT, 0.90, 5.0,
            mapOf(R.HAMSTRING to 1.0, R.GLUTE to 0.8, R.ERECTOR to 1.0, R.QUAD to 0.4)),
        variation("romanian_deadlift", "ルーマニアンデッドリフト", MainLift.DEADLIFT, 0.70, 2.5,
            mapOf(R.HAMSTRING to 1.0, R.GLUTE to 0.7, R.ERECTOR to 0.7)),

        // --- 胸 ---
        accessory("incline_db_press", "インクラインダンベルプレス", 2.0,
            mapOf(R.CHEST_UPPER to 1.0, R.FRONT_DELT to 0.5, R.TRICEPS_LATERAL to 0.3)),
        accessory("incline_barbell_press", "インクラインベンチプレス", 2.5,
            mapOf(R.CHEST_UPPER to 1.0, R.FRONT_DELT to 0.5, R.TRICEPS_LATERAL to 0.3)),
        accessory("dip", "ディップス", 2.5,
            mapOf(R.CHEST_LOWER to 1.0, R.TRICEPS_LATERAL to 0.6, R.TRICEPS_LONG to 0.4)),
        accessory("decline_press", "デクラインプレス", 2.5,
            mapOf(R.CHEST_LOWER to 1.0, R.TRICEPS_LATERAL to 0.4)),
        accessory("pec_fly", "ペックフライ", 2.5,
            mapOf(R.CHEST_MID to 1.0, R.CHEST_UPPER to 0.3)),

        // --- 背中 ---
        accessory("lat_pulldown", "ラットプルダウン", 2.5,
            mapOf(R.LAT to 1.0, R.BICEPS to 0.4, R.REAR_DELT to 0.2)),
        accessory("pull_up", "チンニング", 2.5,
            mapOf(R.LAT to 1.0, R.BICEPS to 0.5, R.FOREARM to 0.3)),
        accessory("barbell_row", "バーベルロウ", 2.5,
            mapOf(R.LAT to 0.7, R.TRAP_MID to 1.0, R.REAR_DELT to 0.4, R.BICEPS to 0.3)),
        accessory("seated_row", "シーテッドロウ", 2.5,
            mapOf(R.TRAP_MID to 1.0, R.LAT to 0.6, R.BICEPS to 0.3)),
        accessory("back_extension", "バックエクステンション", 2.5,
            mapOf(R.ERECTOR to 1.0, R.GLUTE to 0.5, R.HAMSTRING to 0.4)),
        accessory("shrug", "シュラッグ", 2.5,
            mapOf(R.TRAP_UPPER to 1.0, R.FOREARM to 0.3)),

        // --- 肩 ---
        accessory("overhead_press", "オーバーヘッドプレス", 2.5,
            mapOf(R.FRONT_DELT to 1.0, R.SIDE_DELT to 0.5, R.TRICEPS_LATERAL to 0.4)),
        accessory("side_raise", "サイドレイズ", 1.0,
            mapOf(R.SIDE_DELT to 1.0)),
        accessory("rear_delt_fly", "リアデルトフライ", 1.0,
            mapOf(R.REAR_DELT to 1.0, R.TRAP_MID to 0.3)),

        // --- 腕 ---
        accessory("triceps_pushdown", "トライセプスプレスダウン", 2.5,
            mapOf(R.TRICEPS_LATERAL to 1.0, R.TRICEPS_LONG to 0.4)),
        accessory("overhead_extension", "オーバーヘッドエクステンション", 2.5,
            mapOf(R.TRICEPS_LONG to 1.0, R.TRICEPS_LATERAL to 0.4)),
        accessory("barbell_curl", "バーベルカール", 2.5,
            mapOf(R.BICEPS to 1.0, R.FOREARM to 0.4)),
        accessory("hammer_curl", "ハンマーカール", 2.0,
            mapOf(R.BICEPS to 0.8, R.FOREARM to 1.0)),

        // --- 脚 ---
        accessory("leg_press", "レッグプレス", 5.0,
            mapOf(R.QUAD to 1.0, R.GLUTE to 0.5, R.ADDUCTOR to 0.3)),
        accessory("leg_extension", "レッグエクステンション", 2.5,
            mapOf(R.QUAD to 1.0)),
        accessory("leg_curl", "レッグカール", 2.5,
            mapOf(R.HAMSTRING to 1.0)),
        accessory("hip_thrust", "ヒップスラスト", 5.0,
            mapOf(R.GLUTE to 1.0, R.HAMSTRING to 0.4)),
        accessory("adductor_machine", "アダクション", 2.5,
            mapOf(R.ADDUCTOR to 1.0)),
        accessory("calf_raise", "カーフレイズ", 2.5,
            mapOf(R.CALF to 1.0)),

        // --- 体幹 ---
        accessory("cable_crunch", "ケーブルクランチ", 2.5,
            mapOf(R.ABS to 1.0, R.OBLIQUE to 0.3)),
        accessory("side_bend", "サイドベンド", 2.5,
            mapOf(R.OBLIQUE to 1.0, R.ABS to 0.3)),
    )
}
```

- [ ] **Step 4: 週目標プリセットを実装する**

`engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/seed/WeeklyTargetSeed.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine.seed

import dev.dyoshyy.liftplan.engine.MuscleRegion as R

/**
 * 筋区分ごとの週目標セット数のプリセット。
 *
 * パワーリフティング寄りに、BIG3 が直接使う区分（大腿四頭筋・ハム・臀筋・脊柱起立筋・
 * 大胸筋中部）を厚くし、装飾的な区分は薄くしている。
 * 不満が出た区分だけ後から調整すればよく、最初から自分で全部決める必要はない。
 */
object WeeklyTargetSeed {
    val default: Map<R, Double> = mapOf(
        R.CHEST_UPPER to 8.0,
        R.CHEST_MID to 14.0,
        R.CHEST_LOWER to 6.0,

        R.LAT to 12.0,
        R.TRAP_MID to 12.0,
        R.TRAP_UPPER to 6.0,
        R.ERECTOR to 12.0,

        R.FRONT_DELT to 8.0,
        R.SIDE_DELT to 10.0,
        R.REAR_DELT to 8.0,

        R.TRICEPS_LONG to 8.0,
        R.TRICEPS_LATERAL to 10.0,

        R.BICEPS to 10.0,
        R.FOREARM to 6.0,

        R.QUAD to 16.0,
        R.HAMSTRING to 12.0,
        R.GLUTE to 12.0,
        R.ADDUCTOR to 6.0,
        R.CALF to 8.0,

        R.ABS to 8.0,
        R.OBLIQUE to 6.0,
    )
}
```

- [ ] **Step 5: テストを実行して通ることを確認する**

Run: `./gradlew :engine:test --tests '*SeedTest*'`
Expected: PASS（7件）

- [ ] **Step 6: 全テストを実行する**

Run: `./gradlew :engine:test`
Expected: BUILD SUCCESSFUL

- [ ] **Step 7: コミット**

```bash
git add engine/src/main/kotlin/dev/dyoshyy/liftplan/engine/seed/ engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/seed/
git commit -m "feat: 種目マスタと週目標のシードデータを追加する"
```

---

### Task 14: シードを使った通し検証

**Files:**
- Test: `engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/EndToEndTest.kt`

**Interfaces:**
- Consumes: Task 12 の `plan()`、Task 13 のシード
- Produces: なし（検証のみ）

**なぜ必要か:** 各部品は通っても、シードを実際に食わせると成立しない可能性がある。たとえば補助スロットが3つあるのに残差を埋められる種目が見つからない、といった噛み合わせの不具合はここでしか出ない。

- [ ] **Step 1: 通しテストを書く**

`engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/EndToEndTest.kt`:

```kotlin
package dev.dyoshyy.liftplan.engine

import dev.dyoshyy.liftplan.engine.seed.ExerciseSeed
import dev.dyoshyy.liftplan.engine.seed.WeeklyTargetSeed
import kotlinx.datetime.LocalDate
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class EndToEndTest {
    private val start = LocalDate(2026, 8, 17) // 月曜

    private fun day(offset: Int) = LocalDate.fromEpochDays(start.toEpochDays() + offset)

    private fun input(logs: List<SetLog>, date: LocalDate) = PlanInput(
        logs = logs,
        pool = ExerciseSeed.all,
        weeklyTarget = WeeklyTargetSeed.default,
        frequencyPerWeek = 3,
        conditions = emptyList(),
        date = date,
    )

    @Test
    fun `初回でもセッションが組める`() {
        val session = plan(input(emptyList(), start))
        assertEquals(3, session.main.size)
        assertTrue(session.accessories.isNotEmpty(), "補助種目が1つも選ばれていない")
        assertTrue(session.main.all { it.weightKg == null }, "履歴が無いのに重量が出ている")
    }

    @Test
    fun `4週間回しても毎回セッションが組めて重量が単調に壊れない`() {
        val logs = mutableListOf<SetLog>()
        var counter = 0

        // 月・水・金を12回（4週間）
        val trainingDays = (0 until 4).flatMap { week -> listOf(0, 2, 4).map { week * 7 + it } }

        for (offset in trainingDays) {
            val date = day(offset)
            val session = plan(input(logs, date))

            assertEquals(3, session.main.size, "$date でメインが3種目になっていない")
            assertTrue(session.accessories.isNotEmpty(), "$date で補助が選ばれていない")

            // 指示どおりに実行したことにして記録する（重量未定なら初回の仮値を置く）
            for (planned in session.main + session.accessories) {
                val weight = planned.weightKg ?: 60.0
                logs += SetLog(
                    id = "log-${counter++}",
                    date = date,
                    exerciseId = planned.exerciseId,
                    weightKg = weight,
                    reps = 8,
                    rir = planned.targetRir,
                )
            }
        }

        // 4週間分の履歴があれば重量が確定しているはず
        val final = plan(input(logs, day(28)))
        assertTrue(final.main.all { it.weightKg != null }, "履歴があるのに重量が出ていない")
        assertTrue(final.main.all { it.weightKg!! > 0 }, "重量が0以下になっている")
    }

    @Test
    fun `週内でスロットの役割が一巡する`() {
        val logs = mutableListOf<SetLog>()
        val roles = mutableListOf<SlotRole?>()
        var counter = 0

        for (offset in listOf(0, 2, 4)) {
            val date = day(offset)
            val session = plan(input(logs, date))
            roles += session.main.first().role
            for (planned in session.main) {
                logs += SetLog("l-${counter++}", date, planned.exerciseId, planned.weightKg ?: 60.0, 8, planned.targetRir)
            }
        }

        assertEquals(listOf(SlotRole.VARIATION, SlotRole.STANDARD, SlotRole.HEAVY), roles)
    }
}
```

- [ ] **Step 2: テストを実行する**

Run: `./gradlew :engine:test --tests '*EndToEndTest*'`

Expected: 初回は失敗する可能性がある。失敗した場合は**テストを緩めず、実装側の噛み合わせを直す**。よくある原因は次の2つ。

1. 補助種目が選ばれない → `plan()` が週目標を頻度で割っているため、1セッションあたりの残差が小さくなりすぎている。`perSessionTarget` の割り方を見直す
2. バリエーションスロットで重量が出ない → `plannedMainSet` がメイン種目の1RMを参照しているか確認する（バリエーション自身の履歴ではなくメインの1RMに係数を掛けるのが正しい）

- [ ] **Step 3: 全テストを実行する**

Run: `./gradlew :engine:test`
Expected: BUILD SUCCESSFUL

- [ ] **Step 4: コミット**

```bash
git add engine/src/test/kotlin/dev/dyoshyy/liftplan/engine/EndToEndTest.kt
git commit -m "test: シードを使った通し検証を追加する"
```

---

## この計画で作らないもの

以下は後続の計画（②Androidアプリ本体、③Health Connect 取り込み、④Go API + Neon 同期）で扱う。

- Room / Compose / Android プロジェクト
- Health Connect からの実際のデータ取得（`DailyCondition` は引数で受け取るだけ）
- Go API、Neon、同期
- 分析・可視化UI（spec の判断により MVP 対象外）
- ULID の生成（`SetLog.id` は文字列として受け取るだけ）

## 完了の定義

- `./gradlew :engine:test` が全件パスする
- `:engine` に Android 依存が一つも無い
- `plan()` を呼べば、履歴ゼロの状態からでも4週間分のセッションが破綻せず組める
