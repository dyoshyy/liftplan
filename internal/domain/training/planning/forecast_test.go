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
