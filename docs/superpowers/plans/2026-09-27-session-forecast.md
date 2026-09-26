# この先の予定 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 補助の割り振りが既に1週ぶん計算している「先の回」を、`SessionPlanner.Forecast` として全回・全種目・重量つきで返し、`GET /api/sessions/forecast` と専用ページ「この先の予定」で見せる。`Plan`（今日）は `Forecast` の回0を取り出すだけにして、今日の計画と見込みの経路を1本にする。

**Architecture:** ドメイン層（`internal/domain/training/planning`）に `SessionPlanner.Forecast(req) ([]PlannedSession, error)` を追加し、既存の `selectLineup`（今日だけを組み立てていた部分）を削除して `Forecast` へ一本化する。`Plan` は `Forecast(req)[0]` を返すだけになる。アプリケーション層に `GetForecast`、プレゼンテーション層に `GET /api/sessions/forecast` を足し、`index`（0=今日）と `split`（分割の日の名前、無ければ null）だけを返す（日付は返さない）。画面は `features/forecast/` を3層（判断 `forecast.ts` ／ 手順 `useForecast.ts` ／ 描画 `Forecast.tsx`）で新設し、`Today.tsx` に1行リンクを足す。既存の一覧タブには出さない。

**Tech Stack:** Go 1.26（標準ライブラリの `net/http` ルーティング）、React 19 + TypeScript + Vite + Vitest、Playwright（`web/scripts/*-check.mjs`）。

**Spec:** `docs/specs/2026-09-27-session-forecast-design.md`

## Global Constraints

- 応答に日付を含めない。回の識別は `index`（0=今日）だけ（設計書「日付は返さない」）。
- `Plan(req)` は常に `Forecast(req)[0]` と一致する（構造上一致するように実装し、テストでも固定する）。
- `PlanRequest.History` は必ず `Before(req.Date)` で切ってから使う。当日の記録を混ぜない（今日の計画と同じ規約。CLAUDE.md「Plan が当日の記録を見ていたせいで…」の教訓）。
- 先の回の重量の評価日は常に「今日」（`req.Date`）。その回自身の未来の日付を評価日に使わない（42日の鮮度判定・上乗せの判定が今日と同じ基準で動くようにするため）。
- 画面のコードは3層に割る（判断 `.ts` ／ 手順 `use*.ts` ／ 描画 `.tsx`）。`vitest` は `*.test.ts` しか拾わないので、判断は必ず `.ts` に置く。
- PR は3本、1本に1つの判断。PR 1 ドメイン → PR 2 API → PR 3 画面。3本とも「今日の計画の数字は動かない」（`TestSimulation`・`TestGetSession_*` は無変更で緑のまま）。
- コミットメッセージは日本語。末尾に次の行を付ける：
  ```
  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  ```
- 検収コマンド（各タスク共通）：
  ```bash
  gofmt -l .
  go vet ./...
  go test ./...
  ```
  PR 1（ドメイン・シードに触れる）は追加で：
  ```bash
  go test ./internal/domain/training/seed/ -run TestSimulation -v
  ```
  PR 3（画面）は追加で：
  ```bash
  cd web && pnpm typecheck && pnpm test && pnpm build
  ```

## Review Focus

仕様が要求しているが、素直に実装すると見落としやすい5つ。優先度が高い順。

1. **今日の枠が0でも先の回には補助が入ること。** 設計書が名指しで「直すところ」と書いている唯一の挙動変更で、既存の早抜け（`if slots <= 0 { return lineup, nil }`）を消し忘れると PR 1 の主目的が達成されない。→ PR 1 タスク3 `TestSessionPlanner_Forecast_TodaysEmptySlotsDoNotBlockFutureAccessories`。
2. **先の回の軸が、その回自身の役割（重い日／重点種目の一巡のボリュームの日）で処方されること。** `ProjectedSession` はいま役割を外に出していない。素直に実装すると「軸は常に `heavyRole`」という決め打ちになり、一巡2番目の回だけ重量が重すぎるまま気づかれない。→ PR 1 タスク1 `TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle` とタスク3 `TestSessionPlanner_Forecast_FutureSessionUsesItsOwnAxisRole`。
3. **頻度1（回が1つしかない）の境界。** `horizon[len(horizon)-1]` のような「最後の回」を使う式が1件ある（評価日 E）。回が1つだけのとき添字がずれる・panic するといった壊れ方を作り込みやすい。→ PR 1 タスク3 `TestSessionPlanner_Forecast_FrequencyOneHasOnlyToday`。
4. **重量が付かない種目（推定できない・自重の初回など）が `weight_kg: null` のまま通ること。** 新しい DTO 変換経路（`toForecastResponse`）を素朴に書くと、ポインタを経由しない代入で `0` になり「自分で決める」が「0kgで処方された」に化ける。→ PR 2 タスク2 `TestGetForecast_Success`（`weight_kg` が null であることを明示的に見る）。
5. **オフライン・401・409 がこの先の予定のページ単体で正しく出ること。** `useForecast` は `useLiftplan` と独立した読み込み経路なので、今日の画面のオフライン処理を流用したつもりで実は繋がっていない、という配線ミスが起きやすい。→ PR 3 タスク4 `forecast-check.mjs`（実機でオフラインを模擬）と PR 2 タスク2 の 409/401 テスト。

---

## PR 1 ドメイン

**このPRの数字は動かない。** `TestSimulation` の結果と、既存の `Plan` を使うテスト（`TestGetSession_*` など）はすべて無変更で緑のまま通ること。

### タスク1: `ProjectedSession` が軸の役割をパッケージ内に公開する

**Files:**
- `internal/domain/training/planning/horizon_projector.go`（modify: 構造体 19-28行、`ProjectHorizon` のループ 116-122行）
- `internal/domain/training/planning/horizon_projector_internal_test.go`（create）

**Interfaces:**
- Consumes: `laneRole`（`lane_prescription.go` 既存の非公開型）
- Produces（パッケージ内限定）: `func (s ProjectedSession) axisLaneRole() laneRole`

**手順:**

- [ ] 失敗するテストを書く。`internal/domain/training/planning/horizon_projector_internal_test.go` を新規作成（`package planning`。非公開の契約を検査する唯一の理由は `axisLaneRole` が外部から呼べないため。ファイル名の `_internal_test.go` はこのリポジトリの規約どおり）。

  ```go
  package planning

  import (
  	"testing"
  	"time"

  	"github.com/dyoshyy/liftplan/internal/domain/training"
  	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
  	"github.com/dyoshyy/liftplan/internal/domain/training/program"
  	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
  )

  // axisLaneRole は非公開なので、パッケージ内から直接検査する。
  //
  // 呼び出し側（外部テスト）から見えるのは Axis() が返すセット数だけで、
  // heavyRole（0.88・3レップ相当）と focusVolumeRole（0.81・6レップ相当）の
  // どちらで出たかは出力に現れない。Forecast がこの役割を取り違えると、
  // 先の回の重点種目の一巡（1回目は重い・2回目はボリューム）が全部
  // heavyRole 扱いになり、一巡2番目の回の処方が実際より重く出る。
  func TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle(t *testing.T) {
  	bench, err := exercise.NewExercise(exercise.ExerciseParams{
  		ID: "bench", Name: "ベンチプレス",
  		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
  		IncrementKg: 2.5,
  	})
  	if err != nil {
  		t.Fatalf("種目の生成に失敗: %v", err)
  	}
  	freq, err := program.NewFrequency(7)
  	if err != nil {
  		t.Fatalf("頻度が不正: %v", err)
  	}
  	volume, err := program.NewSessionVolume(6, 3)
  	if err != nil {
  		t.Fatalf("1回の量が不正: %v", err)
  	}
  	// 宣言も重点種目も bench 1つだけ。派生を選択していないので、一巡の
  	// 3番目（派生の番）は該当が無く本体へフォールバックする
  	// （axis_rotation.go の axis のコメントどおり）。
  	prog, err := program.NewProgram(freq, volume,
  		[]exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, "bench")
  	if err != nil {
  		t.Fatalf("プログラムの生成に失敗: %v", err)
  	}
  	date := training.MustDate(2026, time.August, 17)

  	got, err := DefaultSessionPlanner().ProjectHorizon(
  		setlog.NewHistory(nil), prog, []*exercise.Exercise{bench}, date)
  	if err != nil {
  		t.Fatalf("ProjectHorizon が失敗: %v", err)
  	}
  	if len(got) != 7 {
  		t.Fatalf("回数が %d。頻度7のはず", len(got))
  	}

  	// 位置0→heavy, 1→focusVolume, 2→派生無しでheavyへフォールバック、を
  	// 3回ぶん繰り返す（focusCycleLength=3、宣言1つだけなので毎回進む）。
  	want := []laneRole{
  		heavyRole, focusVolumeRole, heavyRole,
  		heavyRole, focusVolumeRole, heavyRole, heavyRole,
  	}
  	for k, sess := range got {
  		if role := sess.axisLaneRole(); role != want[k] {
  			t.Errorf("回%d: 役割が %v。%v のはず", k, role, want[k])
  		}
  	}
  }
  ```

- [ ] 走らせる。期待する失敗理由：`sess.axisLaneRole undefined (type ProjectedSession has no field or method axisLaneRole)`（コンパイルエラー）。
  ```bash
  go test ./internal/domain/training/planning/ -run TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle -v
  ```

- [ ] 実装する。`horizon_projector.go` の `ProjectedSession` 構造体に `axisRole laneRole` を足し、`ProjectHorizon` のループで代入し、非公開アクセサを足す。

  構造体（19-28行を置換）:
  ```go
  type ProjectedSession struct {
  	date          training.Date
  	split         program.Split
  	hasSplit      bool
  	axis          *exercise.Exercise
  	axisRole      laneRole
  	axisSets      training.SetCount
  	variation     *exercise.Exercise
  	variationSets training.SetCount
  	stimulus      StimulusCoverage
  }
  ```

  `Axis()` の直後（44行目あたり）にアクセサを追加:
  ```go
  // axisLaneRole はその回の軸が担う役割（重い日・重点種目の一巡の
  // ボリュームの日）。パッケージの外には出さない。Forecast がその回を
  // prescribe するときに、軸を heavyRole 固定ではなく実際の役割で処方する
  // ために要る（設計書「役割（重い日・ボリュームの日・派生）も使う」）。
  // 役割が外へ与える効果は Axis() が既に表現しているので、公開はしない
  // （必要になるまで作らない）。
  func (s ProjectedSession) axisLaneRole() laneRole { return s.axisRole }
  ```

  `ProjectHorizon` のループ（116-122行）に1行足す:
  ```go
  		session := ProjectedSession{date: d, split: today, hasSplit: hasSplit}
  		if heavy != nil {
  			session.axis = heavy
  			session.axisRole = axisRole
  			session.axisSets = p.prescriptionFor(axisRole, sets).setCount()
  			session.stimulus = session.stimulus.Plus(heavy.Stimulus(), session.axisSets)
  			logs = append(logs, projectedLog(k, "axis", d, heavy.ID()))
  		}
  ```

- [ ] 走らせて緑になることを確認する。
  ```bash
  go test ./internal/domain/training/planning/ -run TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle -v
  ```

- [ ] 変異を入れる。`session.axisRole = axisRole` の行をコメントアウトし（ゼロ値 `heavyRole` のまま残る）、`TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle` が回1・回4（期待 `focusVolumeRole`）で赤くなることを確認してから戻す。
  ```bash
  # 1箇所コメントアウトして実行
  go test ./internal/domain/training/planning/ -run TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle -v
  # 戻す
  git checkout internal/domain/training/planning/horizon_projector.go
  ```

- [ ] コミットする。
  ```bash
  git add internal/domain/training/planning/horizon_projector.go internal/domain/training/planning/horizon_projector_internal_test.go
  git commit -m "$(cat <<'EOF'
  feat: ProjectedSession が軸の役割をパッケージ内に公開する

  Forecast が先の回を処方するとき、軸を heavyRole 固定ではなく
  その回自身の役割（重い日／重点種目の一巡のボリュームの日）で
  処方できるようにする。外部には公開しない（必要になるまで作らない）。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

### タスク2: `PlannedSession` が分割の日を持つ

**Files:**
- `internal/domain/training/planning/planned_session.go`（modify: 構造体 27-33行、`Date()` の直後にアクセサ追加）
- `internal/domain/training/planning/lane_prescription.go`（modify: `prescribe` シグネチャ 116-127行）
- `internal/domain/training/planning/session_planner.go`（modify: `Plan` 122-134行、`prescribe` 呼び出しに分割を渡す）
- `internal/domain/training/planning/session_planner_test.go`（modify: 新規テスト追加）

**Interfaces:**
- Produces: `func (s PlannedSession) Split() (program.Split, bool)`
- Consumes（`prescribe` の新シグネチャ）: `func (p SessionPlanner) prescribe(lineup []lineupEntry, estimable setlog.History, conditions condition.ConditionLog, date training.Date, sets int, split program.Split, hasSplit bool) PlannedSession`

**手順:**

- [ ] 失敗するテストを書く。`session_planner_test.go` に追記（`fixedDaySplitRequest` は同ファイルの `TestSessionPlanner_PlanIsFixedForTheWholeDay` の直後に既存）。

  ```go
  // Plan の結果に、その日の分割の日が乗ること。
  //
  // 今日の sessionDTO はまだこの値を読まない（読み始めるのは見込みの画面
  // から、PR2・PR3）。ここで先に固定するのは、次のタスクで Forecast が
  // Plan の回0をそのまま返す形になったとき、分割の日だけがすり抜けて
  // null になる退行を早期に潰すため。
  func TestSessionPlanner_Plan_CarriesTheSplitDay(t *testing.T) {
  	cases := []struct {
  		name      string
  		req       func(t *testing.T) planning.PlanRequest
  		wantHas   bool
  		wantSplit string
  	}{
  		{"分割なしは false", planRequest, false, ""},
  		{"分割ありは周期の先頭", fixedDaySplitRequest, true, "上"},
  	}
  	for _, c := range cases {
  		t.Run(c.name, func(t *testing.T) {
  			got := mustPlan(t, c.req(t))
  			split, hasSplit := got.Split()
  			if hasSplit != c.wantHas {
  				t.Fatalf("分割の有無が %v。%v のはず", hasSplit, c.wantHas)
  			}
  			if hasSplit && split.Name() != c.wantSplit {
  				t.Errorf("分割の日が %q。%q のはず", split.Name(), c.wantSplit)
  			}
  		})
  	}
  }
  ```

- [ ] 走らせる。期待する失敗理由：`got.Split undefined (type planning.PlannedSession has no field or method Split)`。
  ```bash
  go test ./internal/domain/training/planning/ -run TestSessionPlanner_Plan_CarriesTheSplitDay -v
  ```

- [ ] 実装する。

  `planned_session.go`（27-33行の構造体を置換、import に `program` を追加、`Date()` の直後にアクセサを追加）:
  ```go
  import (
  	"github.com/dyoshyy/liftplan/internal/domain/training"
  	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
  	"github.com/dyoshyy/liftplan/internal/domain/training/program"
  )

  // PlannedSession は導出されたセッション。保存はしない。
  type PlannedSession struct {
  	date        training.Date
  	split       program.Split
  	hasSplit    bool
  	main        []PlannedSet
  	variation   []PlannedSet // バリエーションレーンの種目。nilなら出ない
  	accessories []PlannedSet
  }

  func (s PlannedSession) Date() training.Date { return s.date }

  // Split はこのセッションの分割の日。分割が無ければ2番目の戻り値が false
  // （全区分を狙う）。Forecast の各回が「n回後・日の名前」を組み立てるのに
  // 使う（設計書「split はその回の分割の日の名前。分割なしは null」）。
  func (s PlannedSession) Split() (program.Split, bool) { return s.split, s.hasSplit }
  ```

  `lane_prescription.go` の `prescribe`（116-127行）に `split`・`hasSplit` を追加し、`import` に `program` を足す:
  ```go
  import (
  	"github.com/dyoshyy/liftplan/internal/domain/training"
  	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
  	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
  	"github.com/dyoshyy/liftplan/internal/domain/training/program"
  	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
  )

  func (p SessionPlanner) prescribe(
  	lineup []lineupEntry, estimable setlog.History,
  	conditions condition.ConditionLog, date training.Date, sets int,
  	split program.Split, hasSplit bool,
  ) PlannedSession {
  	rirBump := p.analyzer.RIRAdjustment(conditions, date)

  	session := PlannedSession{
  		date:        date,
  		split:       split,
  		hasSplit:    hasSplit,
  		main:        make([]PlannedSet, 0, 1),
  		variation:   make([]PlannedSet, 0, 1),
  		accessories: make([]PlannedSet, 0, len(lineup)),
  	}
  ```
  （関数の残りは無変更）

  `session_planner.go` の `Plan`（122-134行）で分割を求めて渡す。1行追加し、`prescribe` の呼び出しに引数を足す:
  ```go
  	history := req.History.Before(req.Date)
  	estimable := effectiveHistory(history, pool, req.Conditions)

  	// selectLineup が内部で使うのと同じ式（純粋関数なので二重に呼んでも
  	// ずれない）。次のタスクで Plan は Forecast(req)[0] に置き換わり、
  	// この行は消える。
  	splitToday, hasSplit := req.Program.SplitOn(history.SessionCount())

  	lineup, err := p.selectLineup(history, req.Program, req.Target, pool, req.Pool, req.Date)
  	if err != nil {
  		return PlannedSession{}, err
  	}
  	return p.prescribe(lineup, estimable, req.Conditions, req.Date,
  		req.Program.SessionVolume().Sets(), splitToday, hasSplit), nil
  ```

- [ ] 走らせて緑になることを確認する。
  ```bash
  go test ./internal/domain/training/planning/... 2>&1 | tail -20
  ```

- [ ] 変異を入れる。`Plan` の `prescribe` 呼び出しの最後2引数を `program.Split{}, false` に変え（分割を渡し忘れた形）、`TestSessionPlanner_Plan_CarriesTheSplitDay`（分割ありケース）が赤くなることを確認してから戻す。
  ```bash
  go test ./internal/domain/training/planning/ -run TestSessionPlanner_Plan_CarriesTheSplitDay -v
  git checkout internal/domain/training/planning/session_planner.go
  ```

- [ ] コミットする。
  ```bash
  git add internal/domain/training/planning/planned_session.go internal/domain/training/planning/lane_prescription.go internal/domain/training/planning/session_planner.go internal/domain/training/planning/session_planner_test.go
  git commit -m "$(cat <<'EOF'
  feat: PlannedSession が分割の日を持つ

  Forecast が各回の見出しに「・ 日の名前」を添えられるように、
  PlannedSession に Split() を足す。今日の応答（sessionDTO）はまだ
  この値を読まない。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

### タスク3: `SessionPlanner.Forecast` を実装し、今日の早抜けを外す

**Files:**
- `internal/domain/training/planning/session_planner.go`（modify: `Plan` を書き換え、`Forecast` を新設、`selectLineup` を削除。137-248行が対象）
- `internal/domain/training/planning/forecast_test.go`（create）

**Interfaces:**
- Produces: `func (p SessionPlanner) Forecast(req PlanRequest) ([]PlannedSession, error)`
- Produces（書き換え）: `func (p SessionPlanner) Plan(req PlanRequest) (PlannedSession, error)` — 中身が `Forecast(req)[0]` を返すだけになる

**手順:**

- [ ] 失敗するテストを書く。`internal/domain/training/planning/forecast_test.go` を新規作成（`package planning_test`。既存の `planRequest`・`fixedDaySplitRequest`・`rotationRequest`・`focusedProgram`・`planHistory`・`mkLogOn`・`mustPlan`・`assertIntensity`・`mainExercise`・`mustExercise`・`mustAccessory` 等は同パッケージの既存ヘルパーを再利用する）。

  ```go
  package planning_test

  import (
  	"strings"
  	"testing"

  	"github.com/dyoshyy/liftplan/internal/domain/training"
  	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
  	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
  	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
  	"github.com/dyoshyy/liftplan/internal/domain/training/program"
  	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
  )

  // Forecast の回0は常に Plan と一致する。分割・重点種目の一巡・
  // バリエーションが絡む構成でも崩れないこと（設計書「Plan(req) =
  // Forecast(req) の回0」）。
  func TestSessionPlanner_Forecast_FirstSessionMatchesPlan(t *testing.T) {
  	cases := []struct {
  		name string
  		req  func(t *testing.T) planning.PlanRequest
  	}{
  		{"分割も重点種目も無い", planRequest},
  		{"分割がある", fixedDaySplitRequest},
  		{"重点種目の一巡が派生の番", func(t *testing.T) planning.PlanRequest { return rotationRequest(t, 5) }},
  		{"バリエーションが出る", func(t *testing.T) planning.PlanRequest {
  			req := planRequest(t)
  			req.Program, req.Target = focusedProgram(t, "bench")
  			req.History = setlog.NewHistory(append(planHistory(t),
  				mkLogOn(t, "bench-recent", planMonday.AddDays(-3), "bench", 85, 8, 2),
  				mkLogOn(t, "larsen-last", planMonday.AddDays(-10), "larsen", 80, 8, 2),
  				mkLogOn(t, "tempo-last", planMonday.AddDays(-12), "tempo", 80, 8, 2),
  			))
  			return req
  		}},
  	}
  	planner := planning.DefaultSessionPlanner()
  	for _, c := range cases {
  		t.Run(c.name, func(t *testing.T) {
  			req := c.req(t)
  			plan, err := planner.Plan(req)
  			if err != nil {
  				t.Fatalf("Plan が失敗: %v", err)
  			}
  			sessions, err := planner.Forecast(req)
  			if err != nil {
  				t.Fatalf("Forecast が失敗: %v", err)
  			}
  			if diff := planDiff(plan, sessions[0]); len(diff) > 0 {
  				t.Errorf("Forecast の回0が Plan と食い違う\n  %s", strings.Join(diff, "\n  "))
  			}
  		})
  	}
  }

  // 頻度1のとき、見込みは今日の1回だけ。horizon[len(horizon)-1] のような
  // 「最後の回」を使う式が1件ある（評価日E）ので、回が1つしかない境界を
  // 別立てで見る。
  func TestSessionPlanner_Forecast_FrequencyOneHasOnlyToday(t *testing.T) {
  	req := planRequest(t)
  	prog, err := program.NewProgram(mustFrequency(t, 1), planVolume(t),
  		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl"}, big3(), "")
  	if err != nil {
  		t.Fatalf("プログラムの生成に失敗: %v", err)
  	}
  	req.Program = prog
  	req.Target = mustTarget(t, map[training.MuscleRegion]float64{
  		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
  	})

  	planner := planning.DefaultSessionPlanner()
  	sessions, err := planner.Forecast(req)
  	if err != nil {
  		t.Fatalf("Forecast が失敗: %v", err)
  	}
  	if len(sessions) != 1 {
  		t.Fatalf("回数が %d。頻度1なら1回のはず", len(sessions))
  	}
  	plan, err := planner.Plan(req)
  	if err != nil {
  		t.Fatalf("Plan が失敗: %v", err)
  	}
  	if diff := planDiff(plan, sessions[0]); len(diff) > 0 {
  		t.Errorf("頻度1で Plan と Forecast[0] が食い違う\n  %s", strings.Join(diff, "\n  "))
  	}
  }

  // 回の数は頻度と一致し、各回の分割の日は ProjectHorizon の予測と一致する。
  func TestSessionPlanner_Forecast_CountMatchesFrequencyAndSplitMatchesProjector(t *testing.T) {
  	planner := planning.DefaultSessionPlanner()
  	for _, fx := range horizonFixtures(t) {
  		t.Run(fx.name, func(t *testing.T) {
  			date := planMonday
  			projected, err := planner.ProjectHorizon(setlog.NewHistory(fx.history), fx.prog, fx.pool, date)
  			if err != nil {
  				t.Fatalf("ProjectHorizon が失敗: %v", err)
  			}
  			got, err := planner.Forecast(planning.PlanRequest{
  				Program: fx.prog, Target: fx.target, Pool: fx.pool,
  				History:    setlog.NewHistory(fx.history),
  				Conditions: condition.NewConditionLog(nil), Date: date,
  			})
  			if err != nil {
  				t.Fatalf("Forecast が失敗: %v", err)
  			}
  			f := fx.prog.Frequency().PerWeek()
  			if len(got) != f {
  				t.Fatalf("回数が %d。頻度 %d のはず", len(got), f)
  			}
  			for k := range got {
  				wantSplit, wantHas := projected[k].Split()
  				gotSplit, gotHas := got[k].Split()
  				if gotHas != wantHas || gotSplit.Name() != wantSplit.Name() {
  					t.Errorf("回%d: 分割の日が %v(%v)。%v(%v) のはず",
  						k, gotSplit.Name(), gotHas, wantSplit.Name(), wantHas)
  				}
  			}
  		})
  	}
  }

  // slotVaryingRequest は今日の枠が0で、次の回には枠が空く入力。
  //
  // declared は squat と bench。squat は一度も実施していないので
  // stalest が必ず先に返す（未着手を最優先で返す短絡）ため、回0の軸は
  // 必ず squat になる。bench は30日前に実施済みなので、回0で squat が
  // 記録された直後の回1では bench のほうが古株として選ばれる
  // （回1の軸は bench）。重点種目は bench なので、bench が軸の回は
  // バリエーションが出ない（軸が重点種目の系統に含まれる日は出さない
  // 規則）。結果、回0は 軸(squat)+バリエーション(larsen) の2種目で
  // 1回2種目の枠を使い切り、回1は 軸(bench) だけの1種目で枠が1つ余る。
  func slotVaryingRequest(t *testing.T) planning.PlanRequest {
  	t.Helper()
  	pool := []*exercise.Exercise{
  		mainExercise(t, "squat", map[training.MuscleRegion]float64{training.Quad: 1.0}),
  		mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
  		mustExercise(t, exercise.ExerciseParams{
  			ID: "larsen", Name: "ラーセンプレス",
  			Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
  			IncrementKg: 2.5, DerivedFrom: "bench",
  		}),
  		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
  	}
  	target := mustTarget(t, map[training.MuscleRegion]float64{
  		training.Quad: 12, training.ChestMid: 12, training.Biceps: 9,
  	})
  	prog, err := program.NewProgram(mustFrequency(t, 7), mustVolume(t, 2, 3),
  		[]exercise.ExerciseID{"squat", "bench", "larsen", "curl"},
  		[]exercise.ExerciseID{"squat", "bench"}, "bench")
  	if err != nil {
  		t.Fatalf("プログラムの生成に失敗: %v", err)
  	}
  	return planning.PlanRequest{
  		Program: prog, Target: target, Pool: pool,
  		History: setlog.NewHistory([]*setlog.SetLog{
  			mkLogOn(t, "bench-old", planMonday.AddDays(-30), "bench", 80, 8, 2),
  		}),
  		Conditions: condition.NewConditionLog(nil),
  		Date:       planMonday,
  	}
  }

  // 今日の枠が0（軸+バリエーションで埋まる）でも、次の回には補助が入る
  // こと。2つの退行をまとめて捕まえる。(1) 以前は今日の枠が0だと
  // Allocate 自体を呼ばずに抜けていた（回を跨いだ割り振りが丸ごと消える）。
  // (2) 割り振りを呼んでいても、各回に自分の回の割り振り
  // （allocations[k]）ではなく回0のものを配ると、今日が0件なので
  // 全回が0件に見えてしまう。どちらの壊れ方も「回1の補助が空になる」
  // という同じ観測で捕まる。
  func TestSessionPlanner_Forecast_TodaysEmptySlotsDoNotBlockFutureAccessories(t *testing.T) {
  	req := slotVaryingRequest(t)
  	sessions, err := planning.DefaultSessionPlanner().Forecast(req)
  	if err != nil {
  		t.Fatalf("Forecast が失敗: %v", err)
  	}
  	if len(sessions) < 2 {
  		t.Fatalf("回数が %d。2回以上のはず", len(sessions))
  	}
  	if n := len(sessions[0].Main()) + len(sessions[0].Variation()); n != 2 {
  		t.Fatalf("前提: 回0が軸+バリエーションの2種目で埋まっていない: main=%v variation=%v",
  			sessions[0].Main(), sessions[0].Variation())
  	}
  	if len(sessions[0].Accessories()) != 0 {
  		t.Fatalf("前提: 回0の枠が0でない: %v", sessions[0].Accessories())
  	}
  	if n := len(sessions[1].Main()) + len(sessions[1].Variation()); n != 1 {
  		t.Fatalf("前提: 回1が軸1つだけで埋まっていない: main=%v variation=%v",
  			sessions[1].Main(), sessions[1].Variation())
  	}
  	if len(sessions[1].Accessories()) == 0 {
  		t.Error("回1に枠があるのに補助が1つも入っていない")
  	}
  }

  // 先の回の重量は今日の時点の推定で付く。評価日をその回の日付にすると、
  // 42日の鮮度判定がその回の日付を基準に動いてしまい、まだ来ていない
  // 未来の期日を基準に「古すぎる」と誤判定する
  // （設計書「評価する日付も今日」）。
  func TestSessionPlanner_Forecast_WeightsUseTodaysEstimate(t *testing.T) {
  	bench := mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0})
  	prog, err := program.NewProgram(mustFrequency(t, 7), planVolume(t),
  		[]exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, "")
  	if err != nil {
  		t.Fatalf("プログラムの生成に失敗: %v", err)
  	}
  	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})

  	// 41日前の1本だけ。今日の時点では鮮度の窓（42日）にちょうど収まる。
  	req := planning.PlanRequest{
  		Program: prog, Target: target, Pool: []*exercise.Exercise{bench},
  		History:    setlog.NewHistory([]*setlog.SetLog{mkLogOn(t, "old", planMonday.AddDays(-41), "bench", 80, 8, 2)}),
  		Conditions: condition.NewConditionLog(nil),
  		Date:       planMonday,
  	}

  	sessions, err := planning.DefaultSessionPlanner().Forecast(req)
  	if err != nil {
  		t.Fatalf("Forecast が失敗: %v", err)
  	}
  	if len(sessions) != 7 {
  		t.Fatalf("回数が %d。頻度7のはず", len(sessions))
  	}

  	// 回2（頻度7なので今日+2日）で見る。評価日がその回の日付なら
  	// 41+2=43日で鮮度の窓（42日）を割る。評価日が今日なら41日のまま
  	// 窓の中。
  	k := 2
  	if len(sessions[k].Main()) != 1 {
  		t.Fatalf("回%d: 軸が1つでない: %v", k, sessions[k].Main())
  	}
  	if _, ok := sessions[k].Main()[0].Weight(); !ok {
  		t.Errorf("回%d: 重量が付いていない。評価日が今日なら41日で窓の中のはず", k)
  	}
  }

  // 先の回の軸も、その回自身の役割（重い日／重点種目の一巡のボリューム
  // の日）で処方されること。予測器が持つ役割を Forecast が実際に使って
  // いないと、一巡2番目の回まで一律 0.88 になる。
  func TestSessionPlanner_Forecast_FutureSessionUsesItsOwnAxisRole(t *testing.T) {
  	bench := mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0})
  	prog, err := program.NewProgram(mustFrequency(t, 7), planVolume(t),
  		[]exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, "bench")
  	if err != nil {
  		t.Fatalf("プログラムの生成に失敗: %v", err)
  	}
  	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})
  	req := planning.PlanRequest{
  		Program: prog, Target: target, Pool: []*exercise.Exercise{bench},
  		History:    setlog.NewHistory(planHistory(t)),
  		Conditions: condition.NewConditionLog(nil),
  		Date:       planMonday,
  	}

  	sessions, err := planning.DefaultSessionPlanner().Forecast(req)
  	if err != nil {
  		t.Fatalf("Forecast が失敗: %v", err)
  	}
  	// 回1が一巡の位置1（focusVolumeRole・0.81）になることは
  	// TestProjectedSession_AxisLaneRoleFollowsTheFocusCycle が固定した
  	// 並びと同じ（宣言・重点種目とも bench 1つだけ）。
  	if len(sessions[1].Main()) != 1 {
  		t.Fatalf("回1の軸が1つでない: %v", sessions[1].Main())
  	}
  	assertIntensity(t, req, sessions[1].Main()[0], 0.81)
  }

  // 各回の種目数が1回の量を超えないこと（軸・バリエーション・補助の合計）。
  func TestSessionPlanner_Forecast_ExerciseCountNeverExceedsBudget(t *testing.T) {
  	pool := planPool(t)
  	ids := make([]exercise.ExerciseID, 0, len(pool))
  	for _, e := range pool {
  		ids = append(ids, e.ID())
  	}
  	planner := planning.DefaultSessionPlanner()

  	for exercises := 2; exercises <= 6; exercises++ {
  		volume, err := program.NewSessionVolume(exercises, 3)
  		if err != nil {
  			t.Fatalf("1回の量が不正: %v", err)
  		}
  		target := mustTarget(t, map[training.MuscleRegion]float64{
  			training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
  		})
  		prog, err := program.NewProgram(mustFrequency(t, 3), volume, ids, big3(), "")
  		if err != nil {
  			t.Fatalf("プログラムの生成に失敗: %v", err)
  		}

  		sessions, err := planner.Forecast(planning.PlanRequest{
  			Program: prog, Target: target, Pool: pool,
  			History: setlog.NewHistory(nil), Conditions: condition.NewConditionLog(nil),
  			Date: today(),
  		})
  		if err != nil {
  			t.Fatalf("%d種目: Forecast に失敗: %v", exercises, err)
  		}
  		for k, s := range sessions {
  			if n := len(s.Main()) + len(s.Variation()) + len(s.Accessories()); n > exercises {
  				t.Errorf("%d種目: 回%dで%d種目が出た（予算%d）", exercises, k, n, exercises)
  			}
  		}
  	}
  }
  ```

- [ ] 走らせる。期待する失敗理由：`planner.Forecast undefined (type planning.SessionPlanner has no field or method Forecast)`。
  ```bash
  go test ./internal/domain/training/planning/ -run TestSessionPlanner_Forecast -v
  ```

- [ ] 実装する。`session_planner.go` の `selectLineup`（137-248行、`lineupEntry` 型定義とその後の関数全体）を削除し、`Plan`（71-135行）を次のように書き換える。

  ```go
  // Forecast は今日を含めて頻度ぶんの先の回を、各回に重量まで付けて返す。
  // 回0が今日。Plan はその回を取り出すだけ（経路を1本にする。設計書
  // 「Plan(req) = Forecast(req) の回0」）。
  //
  // 先の回は「このまま予定どおりこなした場合の、今日の時点の見込み」。
  // 軸・バリエーション・分割の日は ProjectHorizon の予測をそのまま使い、
  // 補助は割り振り器が回ごとに割り当てたものを使う。重量は前日までの
  // 履歴（今日の時点の推定1RM）でその回を prescribe するだけで、先の回を
  // 実際にこなした場合の伸びは見込まない（設計書「今日の実力でその回の
  // 処方をしたら何kgか」）。評価日は常に req.Date（今日）で、
  // sess.Date()（その回自身の日付）はここでは使わない。使うと、42日の
  // 鮮度判定と上乗せの判定が「その回が来たとき」を基準に動いてしまう。
  //
  // 直すところ：以前（selectLineup）は今日の空き枠が0だと補助の割り振り
  // を丸ごと飛ばしていた。先の回に枠があるかもしれないので、今日の枠が
  // 0でも割り振りは最後まで回す（回0の補助はその場合ちょうど0件になる
  // だけ）。
  func (p SessionPlanner) Forecast(req PlanRequest) ([]PlannedSession, error) {
  	if p.IsZero() {
  		return nil, errors.New("セッション生成器が未設定である")
  	}
  	if req.Program == nil {
  		return nil, errors.New("プログラムが指定されていない")
  	}
  	if req.Target.IsEmpty() {
  		return nil, errors.New("週目標が指定されていない")
  	}
  	if req.Date.IsZero() {
  		return nil, errors.New("対象日が指定されていない")
  	}

  	pool := usablePool(req.Pool, req.Program)

  	// 当日の記録を落とすのはここ1箇所（Plan と同じ規約。理由は
  	// Plan に長く書いてあったコメントのとおりで、Forecast がその
  	// 唯一の経路になった今も変わらない）。
  	history := req.History.Before(req.Date)
  	estimable := effectiveHistory(history, pool, req.Conditions)

  	declared := declaredExercises(pool, req.Program)
  	if len(declared) == 0 {
  		// 到達しない。NewProgram が宣言ゼロを弾き、declared ⊂ selected
  		// なので pool に必ず1つ以上残る（req.Pool 自体が空の異常入力を
  		// 除く。その場合はこの分岐が最後の砦になる）。
  		return nil, errors.New("伸ばしたい種目が1つも選ばれていない")
  	}

  	sessions, err := p.ProjectHorizon(history, req.Program, pool, req.Date)
  	if err != nil {
  		return nil, fmt.Errorf("先の回の予測に失敗: %w", err)
  	}
  	exercisesPerSession := req.Program.SessionVolume().Exercises()
  	horizon := toHorizonSessions(sessions, exercisesPerSession)

  	// 補助の候補プールから外す種目：宣言種目と、重点種目の系統全体
  	// （lineage は重点種目本体と全派生を返すので、回ごとに選ばれる
  	// バリエーションがどれでもここで一括して外れる）。選択されていない
  	// 種目も外す。
  	exclude := accessoryExcluded(pool, req.Program)
  	for _, e := range req.Pool {
  		if e != nil && !req.Program.Includes(e.ID()) {
  			exclude = append(exclude, e.ID())
  		}
  	}

  	// 評価日 E は予測の最後の回の日。窓は [E-27, E]（設計書「損失」）。
  	evalDate := horizon[len(horizon)-1].Date
  	baseline := CoverageBetween(history, req.Pool, evalDate.AddDays(-(CoverageWindowDays-1)), evalDate)

  	setsPerAccessory, err := training.NewSetCount(req.Program.SessionVolume().Sets())
  	if err != nil {
  		return nil, fmt.Errorf("補助のセット数が不正: %w", err)
  	}

  	allocations, err := p.accessory.Allocate(AllocationRequest{
  		Target: req.Target, Baseline: baseline, Sessions: horizon,
  		Cycle: req.Program.Cycle(), SetsPerAccessory: setsPerAccessory,
  		Pool: candidateAccessories(req.Pool, exclude), Master: req.Pool,
  		History: history,
  	})
  	if err != nil {
  		return nil, fmt.Errorf("補助の割り振りに失敗: %w", err)
  	}

  	out := make([]PlannedSession, len(sessions))
  	for k, sess := range sessions {
  		var lineup []lineupEntry
  		if heavy, _, ok := sess.Axis(); ok {
  			lineup = append(lineup, lineupEntry{exercise: heavy, role: sess.axisLaneRole()})
  		}
  		if v, _, ok := sess.Variation(); ok {
  			lineup = append(lineup, lineupEntry{exercise: v, role: variationRole})
  		}
  		for _, id := range allocations[k] {
  			// Allocate が返すのは Pool（= pool から exclude を引いたもの）
  			// の中の種目に限るので、findExercise が nil を返す経路は無い。
  			if e := findExercise(pool, id); e != nil {
  				lineup = append(lineup, lineupEntry{exercise: e, role: accessoryRole})
  			}
  		}

  		split, hasSplit := sess.Split()
  		out[k] = p.prescribe(lineup, estimable, req.Conditions, req.Date,
  			req.Program.SessionVolume().Sets(), split, hasSplit)
  	}
  	return out, nil
  }

  // Plan はその日のセッションを導出する。Forecast の回0を取り出すだけ。
  //
  // 経路を1本にする（設計書「Plan(req) = Forecast(req) の回0」）。2本の
  // 経路があると、どちらかだけ直したときに今日の計画と見込みがずれる。
  //
  // 未来のセッションはどこにも保存しない。今日のメニューも先のメニューも
  // Forecast を対象日で呼んだ結果でしかない。だから予定と実績が食い違う
  // 状態が原理的に発生しない。
  func (p SessionPlanner) Plan(req PlanRequest) (PlannedSession, error) {
  	sessions, err := p.Forecast(req)
  	if err != nil {
  		return PlannedSession{}, err
  	}
  	return sessions[0], nil
  }

  // lineupEntry はセッション1回ぶんの種目1つと、その役割。重量はまだ
  // 付いていない。
  type lineupEntry struct {
  	exercise *exercise.Exercise
  	role     laneRole
  }
  ```

  （`usablePool`・`declaredExercises`・`findExercise` は既存のまま残す。`selectLineup` の削除に伴い不要になった `import "sort"` は `usablePool` がまだ使っているので残る。`import "fmt"` は引き続き使う。）

- [ ] 走らせて緑になることを確認する。
  ```bash
  go build ./... && go test ./internal/domain/training/planning/... -v 2>&1 | tail -60
  ```
  この時点で `TestSessionPlanner_*`（既存）と `TestProjectHorizon_MatchesPlanWhenFollowedExactly` を含め、パッケージ全体が緑であることを確認する。既存テストが赤くなった場合は「別の理由」なので、原因を先に潰してから次へ進む。

- [ ] 変異を入れる（3箇所、それぞれ確認してから戻す）。
  1. `Plan` の `return sessions[0], nil` を `return sessions[len(sessions)-1], nil` に変える → `TestSessionPlanner_Forecast_FirstSessionMatchesPlan` が赤くなることを確認。
  2. `Forecast` のループ内 `split, hasSplit := sess.Split()` を `sess := sessions[0]` 相当（毎回回0を参照）に変える → `TestSessionPlanner_Forecast_CountMatchesFrequencyAndSplitMatchesProjector` の分割ありフィクスチャが赤くなることを確認。
  3. `for _, id := range allocations[k]` を `for _, id := range allocations[0]` に変える → `TestSessionPlanner_Forecast_TodaysEmptySlotsDoNotBlockFutureAccessories` が赤くなることを確認（回1の補助が0件になる）。
  ```bash
  go test ./internal/domain/training/planning/ -run TestSessionPlanner_Forecast -v
  git checkout internal/domain/training/planning/session_planner.go
  ```

- [ ] コミットする。
  ```bash
  git add internal/domain/training/planning/session_planner.go internal/domain/training/planning/forecast_test.go
  git commit -m "$(cat <<'EOF'
  feat: SessionPlanner.Forecast を実装し、Plan をその回0にする

  補助の割り振りが既に1週ぶん計算している先の回を、全回・全種目・
  重量つきで返す Forecast を追加する。Plan は Forecast(req)[0] を
  返すだけにして経路を1本にした。今日の空き枠が0でも、先の回に
  枠があれば補助の割り振りを最後まで回す（以前は今日が0だと
  丸ごと飛ばしていた）。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

### タスク4: 見込みが一日中変わらないことを固定し、`TestSimulation` の数字を確認する

**Files:**
- `internal/domain/training/planning/forecast_test.go`（modify: 追記）

**Interfaces:** なし（既存 API の検証のみ）

**手順:**

- [ ] 失敗するテストを書く。`forecast_test.go` に追記。

  ```go
  // 見込みも一日中変わらない。今日の記録が増えても、先の回（回1以降）の
  // 並び・重量・セット数・目標RIRは変わらないこと（PlanIsFixedForTheWholeDay
  // の考え方を Forecast 全体へ広げたもの）。
  func TestSessionPlanner_Forecast_IsFixedForTheWholeDay(t *testing.T) {
  	req := planRequest(t)
  	base := req.History.Logs()

  	first, err := planning.DefaultSessionPlanner().Forecast(req)
  	if err != nil {
  		t.Fatalf("Forecast が失敗: %v", err)
  	}
  	if len(first) < 2 {
  		t.Fatalf("前提: 回が2つ未満: %d", len(first))
  	}

  	logs := append([]*setlog.SetLog{}, base...)
  	n := 0
  	for _, lane := range plannedLanes(first[0]) {
  		for _, set := range lane.sets {
  			for range set.Sets().Int() {
  				n++
  				logs = append(logs, mkLogOn(t, fmt.Sprintf("f%03d", n), req.Date,
  					string(set.ExerciseID()), 40, 8, 2))
  				req.History = setlog.NewHistory(logs)

  				again, err := planning.DefaultSessionPlanner().Forecast(req)
  				if err != nil {
  					t.Fatalf("%dセット記録した時点で Forecast が失敗: %v", n, err)
  				}
  				for k := 1; k < len(first); k++ {
  					if diff := planDiff(first[k], again[k]); len(diff) > 0 {
  						t.Fatalf("%dセット記録した時点で回%dが変わった\n  %s",
  							n, k, strings.Join(diff, "\n  "))
  					}
  				}
  			}
  		}
  	}
  }
  ```

  （`fmt` を import に追加する）

- [ ] 走らせる。期待する失敗理由：この時点では実装は既にタスク3で完成しているので、素直に走らせれば **緑になるはず**。緑になることを確認してから、次の変異ステップで「正しい理由で守られている」ことを確かめる（このタスクは実装追加ではなく検査の追加であるため、通常の「先に赤」ではなく「変異で赤にできるか」が主眼）。
  ```bash
  go test ./internal/domain/training/planning/ -run TestSessionPlanner_Forecast_IsFixedForTheWholeDay -v
  ```

- [ ] 変異を入れる。`Forecast` の `history := req.History.Before(req.Date)` を `history := req.History` に変え、`TestSessionPlanner_Forecast_IsFixedForTheWholeDay` が赤くなることを確認してから戻す。
  ```bash
  go test ./internal/domain/training/planning/ -run TestSessionPlanner_Forecast_IsFixedForTheWholeDay -v
  git checkout internal/domain/training/planning/session_planner.go
  ```

- [ ] PR 1 全体の検収を回す。
  ```bash
  gofmt -l .
  go vet ./...
  go test ./...
  go test ./internal/domain/training/seed/ -run TestSimulation -v
  ```
  `TestSimulation` の出力（週ボリュームの達成率など）が、このPR適用前後で **数字が一致すること** を確認する（`git stash` で一時的に戻して比較するか、適用前のログを別途残しておく）。動いていたら、`Plan` が `selectLineup` 経由で出していた結果と `Forecast(req)[0]` 経由の結果がどこかで食い違っている。

- [ ] コミットする。
  ```bash
  git add internal/domain/training/planning/forecast_test.go
  git commit -m "$(cat <<'EOF'
  test: Forecast の見込みが一日中変わらないことを固定する

  今日の記録が増えても、先の回（回1以降）の並び・重量・セット数・
  目標RIRが変わらないことを検査する。PlanIsFixedForTheWholeDay の
  考え方を Forecast 全体へ広げたもの。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## PR 2 API

**このPRの数字は動かない。** `GET /api/sessions` の応答・ステータスは無変更。

### タスク1: `GetForecast` ユースケース

**Files:**
- `internal/application/usecase/get_forecast.go`（create）
- `internal/application/usecase/get_forecast_test.go`（create）

**Interfaces:**
- Consumes: `exercise.Reader`・`setlog.Reader`・`condition.Reader`・`program.Reader`・`planning.SessionPlanner`（`GetSession` と同じ4つの読み口 + プランナー）
- Produces: `func NewGetForecast(exercises exercise.Reader, logs setlog.Reader, conditions condition.Reader, programs program.Reader, planner planning.SessionPlanner) *GetForecast`、`func (u *GetForecast) Execute(ctx context.Context, user account.UserID, in GetForecastInput) ([]planning.PlannedSession, error)`

**手順:**

- [ ] 失敗するテストを書く。`internal/application/usecase/get_forecast_test.go` を新規作成（`package usecase_test`。`fakeExercises`・`fakeLogs`・`fakeConditions`・`fakeProgram`・`buildProgram`・`testUser`・`testDate` は `get_session_test.go` に既存のものを同パッケージから再利用する）。

  ```go
  package usecase_test

  import (
  	"context"
  	"errors"
  	"testing"

  	"github.com/dyoshyy/liftplan/internal/application/usecase"
  	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
  	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
  	"github.com/dyoshyy/liftplan/internal/domain/training/program"
  	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
  	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
  )

  func newGetForecast(t *testing.T, logs *fakeLogs, conditions *fakeConditions, prog *fakeProgram) *usecase.GetForecast {
  	t.Helper()
  	pool, err := seed.Exercises()
  	if err != nil {
  		t.Fatalf("シードが不正: %v", err)
  	}
  	return usecase.NewGetForecast(
  		&fakeExercises{all: pool}, logs, conditions, prog,
  		planning.DefaultSessionPlanner(),
  	)
  }

  // 見込みが頻度ぶん返ること。回0が Plan と同じセッションになること自体は
  // ドメイン層（TestSessionPlanner_Forecast_FirstSessionMatchesPlan）が
  // 守るので、ここでは配線（頻度ぶん返ってくること）だけを見る。
  func TestGetForecast_ReturnsOneSessionPerFrequency(t *testing.T) {
  	pool, _ := seed.Exercises()
  	uc := newGetForecast(t,
  		&fakeLogs{history: setlog.NewHistory(nil)},
  		&fakeConditions{log: condition.NewConditionLog(nil)},
  		&fakeProgram{program: buildProgram(t, pool)},
  	)

  	got, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{Date: testDate})
  	if err != nil {
  		t.Fatalf("実行に失敗: %v", err)
  	}
  	if len(got) != 3 { // buildProgram は週3
  		t.Errorf("回数が %d。週3のはず", len(got))
  	}
  }

  func TestGetForecast_PropagatesProgramNotConfigured(t *testing.T) {
  	uc := newGetForecast(t,
  		&fakeLogs{history: setlog.NewHistory(nil)},
  		&fakeConditions{log: condition.NewConditionLog(nil)},
  		&fakeProgram{err: program.ErrProgramNotConfigured},
  	)
  	_, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{Date: testDate})
  	if !errors.Is(err, program.ErrProgramNotConfigured) {
  		t.Errorf("未設定エラーが伝播していない: %v", err)
  	}
  }

  // リポジトリが契約に反して (nil, nil) を返しても、未設定として扱えること
  // （GetSession の同名テストと同じ理由。コピー元と同じ穴を踏みやすい）。
  func TestGetForecast_TreatsNilProgramAsNotConfigured(t *testing.T) {
  	uc := newGetForecast(t,
  		&fakeLogs{history: setlog.NewHistory(nil)},
  		&fakeConditions{log: condition.NewConditionLog(nil)},
  		&fakeProgram{},
  	)
  	_, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{Date: testDate})
  	if !errors.Is(err, program.ErrProgramNotConfigured) {
  		t.Errorf("nil のプログラムが未設定として扱われていない: %v", err)
  	}
  }

  func TestGetForecast_RejectsZeroDate(t *testing.T) {
  	pool, _ := seed.Exercises()
  	uc := newGetForecast(t,
  		&fakeLogs{history: setlog.NewHistory(nil)},
  		&fakeConditions{log: condition.NewConditionLog(nil)},
  		&fakeProgram{program: buildProgram(t, pool)},
  	)
  	if _, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{}); err == nil {
  		t.Error("日付無しが通ってしまう")
  	}
  }
  ```

- [ ] 走らせる。期待する失敗理由：`undefined: usecase.NewGetForecast`（コンパイルエラー）。
  ```bash
  go test ./internal/application/usecase/ -run TestGetForecast -v
  ```

- [ ] 実装する。`internal/application/usecase/get_forecast.go` を新規作成（`get_session.go` と同じ組み立て。設計書「アプリケーション層に GetForecast を足す。組み立ては GetSession と同じ」）。

  ```go
  package usecase

  import (
  	"context"
  	"errors"
  	"fmt"

  	"github.com/dyoshyy/liftplan/internal/application/apperror"
  	"github.com/dyoshyy/liftplan/internal/domain/account"
  	"github.com/dyoshyy/liftplan/internal/domain/training"
  	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
  	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
  	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
  	"github.com/dyoshyy/liftplan/internal/domain/training/program"
  	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
  	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
  )

  type GetForecastInput struct {
  	Date training.Date
  }

  // GetForecast は指定日を起点に、頻度ぶんの先の回をまとめて導出する
  // ユースケース。組み立ては GetSession と同じ（設計書の決定）。今日の
  // 応答とは別の口にするのは、毎回開く「今日」の読み込みに、見ないかも
  // しれない先の回の分まで混ぜないため。
  type GetForecast struct {
  	exercises  exercise.Reader
  	logs       setlog.Reader
  	conditions condition.Reader
  	programs   program.Reader
  	planner    planning.SessionPlanner
  }

  func NewGetForecast(
  	exercises exercise.Reader,
  	logs setlog.Reader,
  	conditions condition.Reader,
  	programs program.Reader,
  	planner planning.SessionPlanner,
  ) *GetForecast {
  	return &GetForecast{
  		exercises: exercises, logs: logs,
  		conditions: conditions, programs: programs, planner: planner,
  	}
  }

  func (u *GetForecast) Execute(ctx context.Context, user account.UserID, in GetForecastInput) (_ []planning.PlannedSession, err error) {
  	defer func() { err = apperror.Classify(err) }()

  	if in.Date.IsZero() {
  		return nil, errors.New("対象日が指定されていない")
  	}

  	prog, err := u.programs.Get(ctx, user)
  	if err != nil {
  		return nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
  	}
  	if prog == nil {
  		return nil, fmt.Errorf("プログラムの取得: %w", program.ErrProgramNotConfigured)
  	}
  	target, err := seed.DefaultWeeklyTarget(prog.Frequency(), prog.SessionVolume())
  	if err != nil {
  		return nil, fmt.Errorf("週目標が組めない: %w", err)
  	}
  	if err := ctx.Err(); err != nil {
  		return nil, fmt.Errorf("見込みの導出が中断された: %w", err)
  	}

  	pool, err := u.exercises.FindAll(ctx)
  	if err != nil {
  		return nil, fmt.Errorf("種目の取得に失敗: %w", err)
  	}
  	history, err := u.logs.FindAll(ctx, user)
  	if err != nil {
  		return nil, fmt.Errorf("実績の取得に失敗: %w", err)
  	}
  	if err := ctx.Err(); err != nil {
  		return nil, fmt.Errorf("見込みの導出が中断された: %w", err)
  	}

  	conditions, err := u.conditions.FindAll(ctx, user)
  	if err != nil {
  		return nil, fmt.Errorf("コンディションの取得に失敗: %w", err)
  	}

  	return u.planner.Forecast(planning.PlanRequest{
  		Program:    prog,
  		Target:     target,
  		Pool:       pool,
  		History:    history,
  		Conditions: conditions,
  		Date:       in.Date,
  	})
  }
  ```

- [ ] 走らせて緑になることを確認する。
  ```bash
  go test ./internal/application/usecase/... -v 2>&1 | tail -40
  ```

- [ ] 変異を入れる。`if prog == nil { return nil, fmt.Errorf(...) }` の行を削除し、`TestGetForecast_TreatsNilProgramAsNotConfigured` が赤くなる（`errors.Is` が false になる／別のエラーで落ちる）ことを確認してから戻す。
  ```bash
  go test ./internal/application/usecase/ -run TestGetForecast_TreatsNilProgramAsNotConfigured -v
  git checkout internal/application/usecase/get_forecast.go
  ```

- [ ] コミットする。
  ```bash
  git add internal/application/usecase/get_forecast.go internal/application/usecase/get_forecast_test.go
  git commit -m "$(cat <<'EOF'
  feat: GetForecast ユースケースを追加する

  組み立ては GetSession と同じ（週目標の導出、種目プール、記録、
  コンディション）。SessionPlanner.Forecast を呼んで頻度ぶんの
  見込みを返す。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

### タスク2: `GET /api/sessions/forecast` ハンドラ・DTO・ルーティング

**Files:**
- `internal/presentation/httpapi/dto.go`（modify: `sessionDTO`／`toSessionDTO` の直後、29-65行あたりに追記）
- `internal/presentation/httpapi/handler.go`（modify: `Handler` struct 23-39行、`Dependencies` struct 45-61行、`NewHandler` 72-123行、`handleGetSession` 199-222行の直後にハンドラ追記）
- `internal/presentation/httpapi/router.go`（modify: `Routes()` 16-41行、24行目の直後に1行追記）
- `internal/presentation/httpapi/handler_test.go`（modify: `dependencies()` 96-115行にフィールド追加、`TestAPI_RequiresAuth` のパス一覧 1949行あたりに1件追加）
- `internal/presentation/httpapi/forecast_handler_test.go`（create）
- `cmd/api/main.go`（modify: `httpapi.Dependencies{...}` 241-257行に1行追加）

**Interfaces:**
- Consumes: `usecase.GetForecast`・`training.ParseDate`・`planning.PlannedSession.Split()`
- Produces: `GET /api/sessions/forecast?date=YYYY-MM-DD` → `{"sessions":[{"index":0,"split":"下半身"|null,"main":[...],"variation":[...],"accessories":[...]}]}`

**手順:**

- [ ] 失敗するテストを書く。`internal/presentation/httpapi/forecast_handler_test.go` を新規作成（`package httpapi_test`。`newServer`・`do`・`putUpperLowerSplit` は `handler_test.go`・`session_helper_test.go` に既存）。

  ```go
  package httpapi_test

  import (
  	"encoding/json"
  	"net/http"
  	"testing"
  )

  // 応答の形：index・split はあるが date が無いこと
  // （設計書「日付は返さない」）。重量が付かない種目は weight_kg が
  // null のまま通ること（履歴が無いので全種目が null のはず）。
  func TestGetForecast_Success(t *testing.T) {
  	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
  	if rec.Code != http.StatusOK {
  		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
  	}

  	var raw struct {
  		Sessions []map[string]json.RawMessage `json:"sessions"`
  	}
  	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
  		t.Fatalf("JSONが壊れている: %v", err)
  	}
  	if len(raw.Sessions) != 3 { // newServer(t, true) は週3
  		t.Fatalf("回数が %d。週3のはず", len(raw.Sessions))
  	}
  	for i, s := range raw.Sessions {
  		if _, ok := s["date"]; ok {
  			t.Errorf("回%d: date が応答に含まれている", i)
  		}
  		if _, ok := s["split"]; !ok {
  			t.Errorf("回%d: split のキーが無い", i)
  		}
  	}

  	var body struct {
  		Sessions []struct {
  			Index int     `json:"index"`
  			Split *string `json:"split"`
  			Main  []struct {
  				ExerciseID string   `json:"exercise_id"`
  				WeightKg   *float64 `json:"weight_kg"`
  			} `json:"main"`
  		} `json:"sessions"`
  	}
  	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
  		t.Fatalf("JSONが壊れている: %v", err)
  	}
  	for i, s := range body.Sessions {
  		if s.Index != i {
  			t.Errorf("index が %d 番目の要素で %d", i, s.Index)
  		}
  	}
  	// 履歴が無いので全回で重量は null のはず。
  	for i, s := range body.Sessions {
  		for _, m := range s.Main {
  			if m.WeightKg != nil {
  				t.Errorf("回%d: 履歴が無いのに %s の重量が入っている", i, m.ExerciseID)
  			}
  		}
  	}
  	// newServer の既定プログラムは分割なし。
  	if body.Sessions[0].Split != nil {
  		t.Errorf("分割なしなのに split が %v", *body.Sessions[0].Split)
  	}
  }

  func TestGetForecast_MissingDate(t *testing.T) {
  	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions/forecast", "")
  	if rec.Code != http.StatusBadRequest {
  		t.Errorf("日付なしが 400 にならない: %d", rec.Code)
  	}
  }

  func TestGetForecast_BadDate(t *testing.T) {
  	rec := do(t, newServer(t, true), http.MethodGet, "/api/sessions/forecast?date=2026/08/17", "")
  	if rec.Code != http.StatusBadRequest {
  		t.Errorf("不正な日付が 400 にならない: %d", rec.Code)
  	}
  }

  func TestGetForecast_ProgramNotConfigured(t *testing.T) {
  	rec := do(t, newServer(t, false), http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
  	if rec.Code != http.StatusConflict {
  		t.Errorf("未設定が 409 にならない: %d", rec.Code)
  	}
  }

  // 分割ありプログラムでは split に日の名前が入ること。
  func TestGetForecast_SplitNameAppears(t *testing.T) {
  	mux := newServer(t, true)
  	putUpperLowerSplit(t, mux)

  	rec := do(t, mux, http.MethodGet, "/api/sessions/forecast?date=2026-08-17", "")
  	if rec.Code != http.StatusOK {
  		t.Fatalf("ステータスが誤り: %d body=%s", rec.Code, rec.Body.String())
  	}
  	var body struct {
  		Sessions []struct {
  			Split *string `json:"split"`
  		} `json:"sessions"`
  	}
  	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
  		t.Fatalf("JSONが壊れている: %v", err)
  	}
  	if body.Sessions[0].Split == nil || *body.Sessions[0].Split != "上半身" {
  		t.Errorf("回0の split が %v。上半身のはず", body.Sessions[0].Split)
  	}
  }
  ```

  加えて、`handler_test.go` の `TestAPI_RequiresAuth`（1949行あたり）のパス一覧に1件追加する：
  ```go
  	for _, path := range []string{
  		"/api/sessions?date=2026-08-17", "/api/sessions/forecast?date=2026-08-17", "/api/program", "/api/program/focus",
  		"/api/program/declared", "/api/program/frequency", "/api/program/selected",
  		"/api/set-logs",
  	} {
  ```

  さらに `dependencies()`（96-115行）に1行追加する（先に足しておかないと、次の実装ステップで `NewHandler` が `GetForecast` の nil チェックを持った瞬間に既存の全ハンドラテストがコンパイルは通るが実行時に `全部揃っているのに組めない` で落ちる）：
  ```go
  	return httpapi.Dependencies{
  		GetSession:       usecase.NewGetSession(exercises, logs, conditions, programs, planning.DefaultSessionPlanner()),
  		GetForecast:      usecase.NewGetForecast(exercises, logs, conditions, programs, planning.DefaultSessionPlanner()),
  		RecordSets:       usecase.NewRecordSets(logs, exercises),
  		...
  ```

- [ ] 走らせる。期待する失敗理由：`dependencies()` 内で `httpapi.Dependencies` に `GetForecast` フィールドが無くコンパイルエラー（`unknown field GetForecast in struct literal`）。
  ```bash
  go test ./internal/presentation/httpapi/... -run 'TestGetForecast|TestAPI_RequiresAuth' -v
  ```

- [ ] 実装する。

  `dto.go` に追記（`toSessionDTO` の直後）:
  ```go
  // forecastSessionDTO は見込みの1回ぶん。sessionDTO と違い日付を持たない
  // （設計書「日付は返さない」）。index が 0=今日、1=次の回、…
  type forecastSessionDTO struct {
  	Index       int             `json:"index"`
  	Split       *string         `json:"split"`
  	Main        []plannedSetDTO `json:"main"`
  	Variation   []plannedSetDTO `json:"variation"`
  	Accessories []plannedSetDTO `json:"accessories"`
  }

  type forecastResponse struct {
  	Sessions []forecastSessionDTO `json:"sessions"`
  }

  func toForecastResponse(sessions []planning.PlannedSession) forecastResponse {
  	out := make([]forecastSessionDTO, 0, len(sessions))
  	for i, s := range sessions {
  		var split *string
  		if sp, ok := s.Split(); ok {
  			name := sp.Name()
  			split = &name
  		}
  		main := make([]plannedSetDTO, 0, len(s.Main()))
  		for _, v := range s.Main() {
  			main = append(main, toPlannedSetDTO(v))
  		}
  		variation := make([]plannedSetDTO, 0, len(s.Variation()))
  		for _, v := range s.Variation() {
  			variation = append(variation, toPlannedSetDTO(v))
  		}
  		accessories := make([]plannedSetDTO, 0, len(s.Accessories()))
  		for _, v := range s.Accessories() {
  			accessories = append(accessories, toPlannedSetDTO(v))
  		}
  		out = append(out, forecastSessionDTO{
  			Index: i, Split: split, Main: main, Variation: variation, Accessories: accessories,
  		})
  	}
  	return forecastResponse{Sessions: out}
  }
  ```

  `handler.go`：`Handler` struct（23行目のフィールド列）に `getForecast *usecase.GetForecast` を追加、`Dependencies` struct（45行目）に `GetForecast *usecase.GetForecast` を追加、`NewHandler` の switch（72行目）に
  ```go
  	case d.GetForecast == nil:
  		return nil, errMissingDependency("GetForecast")
  ```
  を `GetSession` の直後に追加、戻り値の組み立てに `getForecast: d.GetForecast,` を追加。`handleGetSession` の直後にハンドラを追加：
  ```go
  func (h *Handler) handleGetForecast(w http.ResponseWriter, r *http.Request) {
  	user, ok := requireUser(w, r)
  	if !ok {
  		return
  	}
  	raw := r.URL.Query().Get("date")
  	if raw == "" {
  		respondError(w, invalidInput("date クエリパラメータが必要である"))
  		return
  	}
  	date, err := training.ParseDate(raw)
  	if err != nil {
  		respondError(w, invalidInput(err.Error()))
  		return
  	}

  	sessions, err := h.getForecast.Execute(r.Context(), user, usecase.GetForecastInput{Date: date})
  	if err != nil {
  		respondError(w, err)
  		return
  	}
  	writeJSON(w, http.StatusOK, toForecastResponse(sessions))
  }
  ```

  `router.go` の24行目の直後に:
  ```go
  	mux.HandleFunc("GET /api/sessions/forecast", h.handleGetForecast)
  ```

  `cmd/api/main.go` の242行目の直後に:
  ```go
  		GetForecast:      usecase.NewGetForecast(exercises, logs, conditions, programs, planner),
  ```

- [ ] 走らせて緑になることを確認する。
  ```bash
  go build ./... && go test ./internal/presentation/httpapi/... -v 2>&1 | tail -60
  ```

- [ ] 変異を入れる。`toForecastResponse` の `Index: i` を `Index: 0` に変え、`TestGetForecast_Success` が赤くなる（回1以降の `index` が0のまま）ことを確認してから戻す。続けて `s.Split()` の呼び出し先を `sessions[0].Split()`（毎回回0）に変え、`TestGetForecast_SplitNameAppears` は緑のまま動かないことも確かめておく（分割ありの単一分割プリセットでは回0も他回も同じ split 名になりうるため、これは弱い変異点。代わりに `router.go` の `GET /api/sessions/forecast` の登録行を丸ごと消し、`TestGetForecast_Success` が 404 で赤くなることを確認する方がこの層の配線をより確実に守る）。
  ```bash
  go test ./internal/presentation/httpapi/ -run 'TestGetForecast_Success' -v
  git checkout internal/presentation/httpapi/dto.go internal/presentation/httpapi/router.go
  ```

- [ ] PR 2 全体の検収を回す。
  ```bash
  gofmt -l .
  go vet ./...
  go test ./...
  ```

- [ ] コミットする。
  ```bash
  git add internal/presentation/httpapi/dto.go internal/presentation/httpapi/handler.go internal/presentation/httpapi/router.go internal/presentation/httpapi/handler_test.go internal/presentation/httpapi/forecast_handler_test.go cmd/api/main.go
  git commit -m "$(cat <<'EOF'
  feat: GET /api/sessions/forecast を追加する

  index（0=今日）と split（分割の日の名前、無ければ null）だけを
  返す。日付は含めない。GetSession と同じ認証・409・エラーの経路を
  そのまま使う。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

---

## PR 3 画面

**このPRの数字は動かない。** 既存のテスト・ビルドはすべて無変更で緑のまま通ること。

### タスク1: 判断（`forecast.ts`）と型

**Files:**
- `web/src/api/types.ts`（modify: `Session` 型の直後、24行目あたりに追記）
- `web/src/features/forecast/forecast.ts`（create）
- `web/src/features/forecast/forecast.test.ts`（create）

**Interfaces:**
- Consumes: なし（純粋関数）
- Produces: `export type ForecastSession`、`export type ForecastResponse`、`export function sessionHeading(index: number, split: ForecastSession['split']): string`

**手順:**

- [ ] 失敗するテストを書く。`web/src/features/forecast/forecast.test.ts` を新規作成。

  ```ts
  import { describe, expect, it } from 'vitest';
  import { sessionHeading } from './forecast';

  describe('sessionHeading', () => {
    it.each([
      { index: 0, split: null, want: '今日' },
      { index: 1, split: null, want: '次の回' },
      { index: 2, split: null, want: '2回後' },
      { index: 5, split: null, want: '5回後' },
      { index: 6, split: null, want: '6回後' },
    ])('回$index・分割なしは $want', ({ index, split, want }) => {
      expect(sessionHeading(index, split)).toBe(want);
    });

    it.each([
      { index: 0, split: '下半身', want: '今日 ・ 下半身' },
      { index: 1, split: '上半身', want: '次の回 ・ 上半身' },
      { index: 3, split: '肩・腕', want: '3回後 ・ 肩・腕' },
    ])('分割があれば日の名前を添える（回$index）', ({ index, split, want }) => {
      expect(sessionHeading(index, split)).toBe(want);
    });
  });
  ```

- [ ] 走らせる。期待する失敗理由：`Cannot find module './forecast'`。
  ```bash
  cd web && pnpm vitest run src/features/forecast/forecast.test.ts
  ```

- [ ] 実装する。

  `web/src/api/types.ts` の `Session` 型（19-24行）の直後に追記:
  ```ts
  /** 見込みの1回ぶん。dto.go の forecastSessionDTO と対。date は無い
   *  （画面では出さない決定なので、応答にも乗せない）。 */
  export type ForecastSession = {
    index: number;
    /** split はその回の分割の日の名前。分割なしは null。 */
    split: string | null;
    main: PlannedSet[];
    variation: PlannedSet[];
    accessories: PlannedSet[];
  };

  export type ForecastResponse = { sessions: ForecastSession[] };
  ```

  `web/src/features/forecast/forecast.ts` を新規作成:
  ```ts
  import type { ForecastSession } from '../../api/types';

  /**
   * sessionHeading は回の見出し。0→今日、1→次の回、2以上→n回後。
   * 分割があれば「・ 日の名前」を添える（設計書の画面モック）。
   *
   * 日付を出さない決定（設計書「先の回の呼び方」）の帰結として、
   * 見出しは出席回数だけで表す。
   */
  export function sessionHeading(index: number, split: ForecastSession['split']): string {
    const base = index === 0 ? '今日' : index === 1 ? '次の回' : `${index}回後`;
    return split ? `${base} ・ ${split}` : base;
  }
  ```

- [ ] 走らせて緑になることを確認する。
  ```bash
  cd web && pnpm vitest run src/features/forecast/forecast.test.ts
  ```

- [ ] 変異を入れる。`index === 1 ? '次の回' : ...` を `index === 1 ? '2回後' : ...` に変え、`sessionHeading` のテストが赤くなることを確認してから戻す。
  ```bash
  cd web && pnpm vitest run src/features/forecast/forecast.test.ts
  git checkout web/src/features/forecast/forecast.ts
  ```

- [ ] コミットする。
  ```bash
  git add web/src/api/types.ts web/src/features/forecast/forecast.ts web/src/features/forecast/forecast.test.ts
  git commit -m "$(cat <<'EOF'
  feat: 見込みの回の見出しを組み立てる判断（forecast.ts）を足す

  0→今日、1→次の回、2以上→n回後。分割があれば日の名前を添える。
  この先の予定ページの判断層。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

### タスク2: 手順（`useForecast.ts`）

**Files:**
- `web/src/features/forecast/useForecast.ts`（create）

**Interfaces:**
- Consumes: `getJSON` from `api/client.ts`、`ForecastResponse`／`ForecastSession` from `api/types.ts`、`today` from `domain/date.ts`
- Produces: `export function useForecast(): { sessions: ForecastSession[] | null; error: string; reload: () => Promise<void> }`

**手順:**

- [ ] このタスクは単体テストを書かない。理由：`useForecast` は分岐（成功／失敗）を持つが、それは通信の結果を受けるだけの手順であり、判断（`sessionHeading` の分岐）とは性質が違う。既存の同種フック `web/src/features/history/useStats.ts`（開いたときだけ取りに行く・`try/catch` で一言に潰す）にも対応する `*.test.ts` は無く、配線としての正しさは実機（`web/scripts/*-check.mjs`、タスク4）で見る、というのがこのリポジトリの既定の分担（`.claude/skills/orchestration-hooks/SKILL.md`「フックのテストのために jsdom と testing-library を入れる」は「やらないこと」）。

- [ ] 実装する。`web/src/features/forecast/useForecast.ts` を新規作成（`useStats.ts` と同じ形）。

  ```ts
  import { useCallback, useEffect, useState } from 'react';
  import { getJSON } from '../../api/client';
  import type { ForecastResponse, ForecastSession } from '../../api/types';
  import { today } from '../../domain/date';

  export type Forecast = {
    sessions: ForecastSession[] | null;
    /** 読めなかったときの一言。空なら成功。 */
    error: string;
    reload: () => Promise<void>;
  };

  /**
   * useForecast はこの先の予定を取りに行く。
   *
   * 毎回見るものではないので、今日のメニュー（useLiftplan）の読み込みには
   * 混ぜない。ページが開かれたとき（マウント時）に1回取りに行く
   * （useStats と同じ形）。PWA のキャッシュにも Outbox にも載せない
   * （設計書の決定）。古い見込みを見せるより、オフラインでは出さない
   * ほうが正直。
   */
  export function useForecast(): Forecast {
    const [sessions, setSessions] = useState<ForecastSession[] | null>(null);
    const [error, setError] = useState('');

    const reload = useCallback(async () => {
      setError('');
      try {
        const res = await getJSON<ForecastResponse>(`/api/sessions/forecast?date=${today()}`);
        setSessions(res.sessions);
      } catch {
        setSessions(null);
        setError('オフラインでは見られません');
      }
    }, []);

    useEffect(() => {
      void reload();
    }, [reload]);

    return { sessions, error, reload };
  }
  ```

- [ ] 型検査が通ることを確認する。
  ```bash
  cd web && pnpm typecheck
  ```

- [ ] 変異チェック：このタスクに単体テストは無いので、コード変異ではなく「配線」を次のタスク4の `forecast-check.mjs` が守る（`/api/sessions/forecast` を `route().abort()` したとき「オフラインでは見られません」が出ること）。ここでは該当ステップを飛ばし、タスク4で確認する。

- [ ] コミットする。
  ```bash
  git add web/src/features/forecast/useForecast.ts
  git commit -m "$(cat <<'EOF'
  feat: この先の予定を取りに行く手順（useForecast.ts）を足す

  開いたときに1回だけ取りに行く。オフライン・失敗は一言に潰す
  （useStats.ts と同じ形）。PWA のキャッシュにも Outbox にも載せない。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

### タスク3: 描画（`Forecast.tsx`）と「今日」からのリンク

**Files:**
- `web/src/features/today/ExerciseCard.tsx`（modify: 72行目 `function Target` を `export function Target` に）
- `web/src/features/forecast/Forecast.tsx`（create）
- `web/src/app/route.ts`（modify: `Route` 型11行目、`isRoute` 13-14行目）
- `web/src/app/route.test.ts`（modify: 追記）
- `web/src/app/App.tsx`（modify: import追加、`Today` へのprop追加、`route === 'forecast'` の分岐追加）
- `web/src/features/today/Today.tsx`（modify: Props に `onOpenForecast` 追加、リンクのJSX追加）

**Interfaces:**
- Consumes: `Target` from `features/today/ExerciseCard.tsx`（種目の目標表示を読み取り専用で再利用）、`sessionHeading` from `./forecast`、`useForecast` from `./useForecast`
- Produces: `export function Forecast({ nameOf, onBack }: { nameOf: (id: string) => string; onBack: () => void }): JSX.Element`

**手順:**

- [ ] 失敗するテストを書く。`web/src/app/route.test.ts` に追記（純粋関数 `isRoute`／`fromState` の拡張なので、ここだけは判断のテストとして書ける）。

  ```ts
  // route.test.ts の describe('fromState', ...) 内に追加
  it('forecast も読み戻す', () => {
    expect(fromState({ route: 'forecast' })).toBe('forecast');
  });

  // describe('isRoute', ...) 内に追加
  it('forecast も知っている行き先', () => {
    expect(isRoute('forecast')).toBe(true);
  });
  ```

- [ ] 走らせる。期待する失敗理由：`expected 'today' to be 'forecast'`（`isRoute`/`fromState` がまだ `'forecast'` を知らないので既定の `'today'` に倒れる）。
  ```bash
  cd web && pnpm vitest run src/app/route.test.ts
  ```

- [ ] 実装する。

  `route.ts`（11-14行を置換）:
  ```ts
  export type Route = 'today' | 'history' | 'settings' | 'forecast';

  export const isRoute = (v: unknown): v is Route =>
    v === 'today' || v === 'history' || v === 'settings' || v === 'forecast';
  ```

  `ExerciseCard.tsx` の72行目:
  ```ts
  export function Target({ plan }: { plan: CardPlan }) {
  ```

  `web/src/features/forecast/Forecast.tsx` を新規作成（判断は `forecast.ts`、取得は `useForecast.ts`。ここは描画だけ）:
  ```tsx
  import { useState } from 'react';
  import type { ForecastSession, PlannedSet } from '../../api/types';
  import { Card, Note } from '../../ui/Card';
  import { Target } from '../today/ExerciseCard';
  import { sessionHeading } from './forecast';
  import { useForecast } from './useForecast';

  type Props = {
    nameOf: (id: string) => string;
    onBack: () => void;
  };

  export function Forecast({ nameOf, onBack }: Props) {
    const { sessions, error } = useForecast();
    const [openIndex, setOpenIndex] = useState(0); // 今日（回0）だけ開いて始める

    return (
      <div className="grid gap-3.5">
        <button type="button" onClick={onBack} className="w-fit text-sm text-muted">
          ← 今日
        </button>

        <div>
          <h1 className="text-[19px] font-semibold">この先の予定</h1>
          <Note className="mt-1">
            今日の時点の見込みです。休んだり、別の種目をやったりすると変わります。
            重量は今日の実力での見込みで、当日の記録に合わせて付け直されます。
          </Note>
        </div>

        {error && <Note>{error}</Note>}

        {sessions?.map((session) => (
          <SessionCard
            key={session.index}
            session={session}
            nameOf={nameOf}
            open={openIndex === session.index}
            onToggle={() =>
              setOpenIndex((cur) => (cur === session.index ? -1 : session.index))
            }
          />
        ))}
      </div>
    );
  }

  function SessionCard({
    session,
    nameOf,
    open,
    onToggle,
  }: {
    session: ForecastSession;
    nameOf: (id: string) => string;
    open: boolean;
    onToggle: () => void;
  }) {
    const rows = [...session.main, ...session.variation, ...session.accessories];
    return (
      <Card className="grid gap-2.5">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={open}
          className="text-left text-[15px] font-semibold"
        >
          {open ? '▼' : '▶'} {sessionHeading(session.index, session.split)}
        </button>
        {open && (
          <div className="grid gap-3">
            {rows.map((set) => (
              <Row key={set.exercise_id} set={set} name={nameOf(set.exercise_id)} />
            ))}
          </div>
        )}
      </Card>
    );
  }

  function Row({ set, name }: { set: PlannedSet; name: string }) {
    return (
      <div className="grid gap-1">
        <span className="text-[15px] font-medium">{name}</span>
        <Target plan={set} />
      </div>
    );
  }
  ```

  `Today.tsx` の `Props` 型（`canStartRest: boolean;` の直後、25行目あたり）に追加:
  ```ts
    /** onOpenForecast はこの先の予定を開く。 */
    onOpenForecast: () => void;
  ```
  `Today.tsx` の `{done.map(card)}`（98行目）の直後に、今日のリストと見た目を分けて追加:
  ```tsx
        <div className="mt-1 border-t border-line-soft pt-3.5">
          <button
            type="button"
            className="text-[13px] text-muted underline underline-offset-2"
            onClick={props.onOpenForecast}
          >
            この先の予定を見る →
          </button>
        </div>
  ```

  `App.tsx`：import に
  ```ts
  import { Forecast } from '../features/forecast/Forecast';
  ```
  を追加。`<Today ... />`（96-106行）に1行追加:
  ```tsx
            {route === 'today' && (
              <Today
                data={data}
                enqueue={outbox.enqueue}
                onRecordLocally={session.recordLocally}
                onForgetLocally={session.forgetLocally}
                onRecorded={timer.start}
                canStartRest={timer.state.kind === 'idle'}
                onReload={reload}
                onOpenForecast={() => go('forecast')}
              />
            )}
  ```
  `{route === 'settings' && (...)}` ブロック（118-129行）の直後に追加:
  ```tsx
            {route === 'forecast' && (
              <Forecast nameOf={nameOf} onBack={() => go('today')} />
            )}
  ```

- [ ] 走らせて緑になることを確認する。
  ```bash
  cd web && pnpm vitest run src/app/route.test.ts && pnpm typecheck
  ```

- [ ] 変異を入れる。`isRoute` から `v === 'forecast'` の分岐を削り、`route.test.ts` の新規ケースが赤くなることを確認してから戻す。
  ```bash
  cd web && pnpm vitest run src/app/route.test.ts
  git checkout web/src/app/route.ts
  ```
  （`Forecast.tsx`・`App.tsx`・`Today.tsx` は描画とその配線なので、この場でのコード変異チェックは行わない。タスク4の `forecast-check.mjs` が実機で確認する。）

- [ ] コミットする。
  ```bash
  git add web/src/features/today/ExerciseCard.tsx web/src/features/forecast/Forecast.tsx web/src/app/route.ts web/src/app/route.test.ts web/src/app/App.tsx web/src/features/today/Today.tsx
  git commit -m "$(cat <<'EOF'
  feat: この先の予定のページと「今日」からのリンクを足す

  今日は開いて先の回はたたんで並べる。種目の行は ExerciseCard の
  目標表示（Target）を読み取り専用で再利用する。下のタブには出さない。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```

### タスク4: 配線チェック（`forecast-check.mjs`）とキャッシュ対象外の確認

**Files:**
- `web/scripts/forecast-check.mjs`（create）
- `web/README.md`（modify: 39-43行の検収コマンド一覧に1行追加）

**Interfaces:** なし（実機・静的チェックスクリプト）

**手順:**

- [ ] このタスクに事前の「失敗するテスト」は書かない（Playwright スクリプトはサーバーと画面を実際に起動して初めて実行できるため、TDD の赤の確認は「まだファイルが無い」状態そのもので代える）。かわりに、スクリプトの2つの静的チェック（sw.ts が `/api` を扱っていないこと、`useForecast.ts` が Outbox を使っていないこと）が正しい理由で通ることを先に手で確認する。
  ```bash
  grep -n "/api" web/src/sw.ts          # ヒット無しのはず（precacheAndRoute のみ）
  grep -n "outbox" web/src/features/forecast/useForecast.ts   # ヒット無しのはず
  ```

- [ ] 実装する。`web/scripts/forecast-check.mjs` を新規作成（`web/scripts/nav-check.mjs` と同じ骨格。ポートは `APP=`／`API=` で渡す）。

  ```js
  // この先の予定のページを実機で確かめる。
  //
  // 単体テストでは踏めない配線（今日からのリンク、開閉、オフライン表示、
  // service worker がこの経路をキャッシュしていないこと）をここで見る。
  // 使い方は scripts/nav-check.mjs と同じ。ポートは APP= と API= で渡す。
  const { chromium } = await import(process.env.PLAYWRIGHT ?? 'playwright-core');
  const APP = process.env.APP ?? 'http://localhost:4173';
  const TOKEN = 'dev-token-0123456789abcdef0123456789ab';
  const results = [];
  const check = (n, ok, d = '') => { results.push({ n, ok, d }); console.log(`${ok ? '✓' : '✗'} ${n}${d ? ' — ' + d : ''}`); };

  // 静的な確認：service worker がこの経路をキャッシュ対象にしていない
  // こと、useForecast が Outbox（送信の待ち行列）を使っていないこと。
  // 実行時ではなくソースを直接見る。ブラウザ越しだと「キャッシュしな
  // かった」ことは陰性の確認で、通信を1回消すだけでは判定できない。
  const fs = await import('node:fs');
  const swSrc = fs.readFileSync(new URL('../src/sw.ts', import.meta.url), 'utf8');
  check('service worker が /api を扱っていない（キャッシュ対象外）', !swSrc.includes('/api'));
  const hookSrc = fs.readFileSync(new URL('../src/features/forecast/useForecast.ts', import.meta.url), 'utf8');
  check('useForecast が Outbox を使っていない', !hookSrc.includes('outbox'));

  const browser = await chromium.launch({ executablePath: process.env.CHROME ?? '/usr/bin/chromium', args: ['--no-sandbox'] });
  const page = await (await browser.newContext({ viewport: { width: 390, height: 844 } })).newPage();
  const errs = [];
  page.on('pageerror', (e) => errs.push('pageerror: ' + e.message));
  page.on('console', (m) => { if (m.type() === 'error') errs.push(m.text()); });

  await page.goto(`${APP}/#token=${TOKEN}`);
  await page.waitForTimeout(2500);

  const body = () => page.innerText('body');
  check('今日の画面にリンクがある', (await body()).includes('この先の予定を見る'));
  check('タブには出ない', (await page.locator('nav').first().innerText()).includes('予定') === false);

  await page.click('text=この先の予定を見る');
  await page.waitForTimeout(1500);
  const forecastBody = await body();
  check('予定のページが開く', forecastBody.includes('この先の予定'));
  check('今日は開いている', /▼\s*今日/.test(forecastBody));

  // 折りたたみを開く
  const collapsed = page.locator('button[aria-expanded="false"]').first();
  if (await collapsed.count()) {
    await collapsed.click();
    await page.waitForTimeout(400);
    check('たたんだ回を開ける', (await collapsed.getAttribute('aria-expanded')) === 'true');
  }

  // 戻るジェスチャーで今日に帰る
  await page.goBack();
  await page.waitForTimeout(800);
  check('戻るで今日に帰る', (await body()).includes('この先の予定を見る'));

  // オフライン（見込みの通信だけを落とす）
  await page.route('**/api/sessions/forecast*', (route) => route.abort());
  await page.click('text=この先の予定を見る');
  await page.waitForTimeout(1500);
  check('オフラインで一言出る', (await body()).includes('オフラインでは見られません'));

  await page.screenshot({ path: '/tmp/forecast.png' });

  console.log('\nエラー:', errs.length ? errs.join('\n') : '(なし)');
  const failed = results.filter((r) => !r.ok);
  console.log(`\n${results.length - failed.length}/${results.length} 通過`);
  await browser.close();
  process.exit(failed.length ? 1 : 0);
  ```

  `web/README.md` の検収コマンド一覧（39-43行）に1行追加:
  ```
  node scripts/forecast-check.mjs     # この先の予定（今日のリンク・たたみ・オフライン表示）
  ```

- [ ] 実機で走らせる（サーバーと画面を起動したうえで）。
  ```bash
  # 別ターミナルでサーバー
  cd .. && ALLOWED_ORIGINS=http://localhost:4173 DEV_SESSION_TOKEN=dev-token-0123456789abcdef0123456789ab make run
  # 画面をビルドしてプレビュー
  cd web && pnpm build && pnpm preview &
  node scripts/forecast-check.mjs
  ```
  全項目が `✓` になることを確認する。

- [ ] 変異チェック：`useForecast.ts` の `catch` 節を `catch { setSessions([]); }`（エラーメッセージを設定しない）に変え、`forecast-check.mjs` の「オフラインで一言出る」が `✗` になることを確認してから戻す。
  ```bash
  node scripts/forecast-check.mjs
  git checkout web/src/features/forecast/useForecast.ts
  ```

- [ ] PR 3 全体の検収を回す。
  ```bash
  cd web && pnpm typecheck && pnpm test && pnpm build
  node scripts/forecast-check.mjs
  ```

- [ ] コミットする。
  ```bash
  git add web/scripts/forecast-check.mjs web/README.md
  git commit -m "$(cat <<'EOF'
  test: この先の予定の配線を実機で確かめるチェックを足す

  今日からのリンク、開閉、戻るジェスチャー、オフライン表示、
  service worker がこの経路をキャッシュしていないことを見る。

  Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>
  EOF
  )"
  ```
