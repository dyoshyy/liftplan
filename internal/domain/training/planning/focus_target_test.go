package planning_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// 重点種目を指定すると、その系統が先の回で余分に入れる刺激のぶんだけ
// 週目標が上がる。
//
// 重点種目は一巡（重い番・軽い番・派生）とバリエーションレーンで週に
// 3回前後出るのに、目標は他の宣言種目と同じだった。胸は目標を大きく
// 超えて超過の罰則（α）がかかり、胸に副次で効く補助まで選ばれなくなる。
//
// 足す量は固定の係数ではなく、先の回（Forecast と同じ ProjectHorizon）に
// 実際に出る系統の刺激。分割や頻度で系統の出方は週 0.75〜3 回まで動くので、
// 定数にすると合わない構成が必ず残る。
func TestSessionPlanner_WeeklyTargetIsRaisedByTheFocusLineage(t *testing.T) {
	planner := planning.DefaultSessionPlanner()
	prog, base := focusedProgram(t, "bench")
	pool := planPool(t)
	history := setlog.NewHistory(planHistory(t))

	// 期待値は同じ先の回から数える。この履歴（系統は3回実施済みで一巡の
	// 先頭）では、軸のベンチは重い番で、余分は派生のバリエーションだけ。
	horizon, err := planner.ProjectHorizon(history, prog, pool, planMonday)
	if err != nil {
		t.Fatalf("先の回の予測に失敗: %v", err)
	}
	extra := 0.0
	for _, s := range horizon {
		if v, sets, ok := s.Variation(); ok {
			c, _ := v.Stimulus().Contribution(training.ChestMid)
			extra += c.TimesSets(sets)
		}
	}
	if extra == 0 {
		t.Fatal("この履歴で派生がバリエーションに一度も出ない。前提が崩れている")
	}

	got, err := planner.WeeklyTarget(base, history, prog, pool, planMonday)
	if err != nil {
		t.Fatalf("週目標の導出に失敗: %v", err)
	}
	want := base.Sets(training.ChestMid) + extra
	if math.Abs(got.Sets(training.ChestMid)-want) > 1e-9 {
		t.Errorf("ChestMid の目標が %v。%v（元の %v ＋ 派生 %v）のはず",
			got.Sets(training.ChestMid), want, base.Sets(training.ChestMid), extra)
	}
	if got.Sets(training.Quad) != base.Sets(training.Quad) {
		t.Errorf("系統が触らない Quad が %v → %v に動いた", base.Sets(training.Quad), got.Sets(training.Quad))
	}
}

// 重点種目が無ければ週目標はそのまま。
func TestSessionPlanner_WeeklyTargetIsUnchangedWithoutFocus(t *testing.T) {
	planner := planning.DefaultSessionPlanner()
	prog, base := planProgram(t)
	pool := planPool(t)
	history := setlog.NewHistory(planHistory(t))

	got, err := planner.WeeklyTarget(base, history, prog, pool, planMonday)
	if err != nil {
		t.Fatalf("週目標の導出に失敗: %v", err)
	}
	for _, r := range base.Regions() {
		if got.Sets(r) != base.Sets(r) {
			t.Errorf("%s が %v → %v に動いた", r, base.Sets(r), got.Sets(r))
		}
	}
}

// 重点種目の系統で胸が目標を超えていても、胸に副次で効く補助は締め出されない。
//
// これが利用者の見た症状。重点ベンチで胸が毎週目標を大きく超え、肩の
// 種目（前部三角筋が主働、胸に副次）まで超過の罰則で選ばれなくなり、
// 補助の枠が空いたまま出ていた。Forecast が上げた目標を割り振り器に
// 渡していなければ、ここが落ちる。
func TestSessionPlanner_FocusDoesNotCrowdOutAccessoriesWithSecondaryChest(t *testing.T) {
	pool := append(planPool(t), mkAccessory(t, "ohp", map[training.MuscleRegion]float64{
		training.FrontDelt: 1.0, training.ChestMid: 0.5,
	}))
	// 胸の目標（6）は系統が週に供給する量（12）の半分。重点を指定した人の
	// 設定から組んだ目標はこの形になる。肩は達成率 75%（遅れは小さい）。
	// 遅れが大きいと罰則を押し切って選ばれてしまい、症状を再現できない。
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 6, training.FrontDelt: 3, training.Quad: 12,
	})
	prog, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 3, 3),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "larsen", "tempo", "ohp"},
		big3(), "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// 4週の窓いっぱいに、重点ベンチの一巡どおりの記録を置く。胸は軸と
	// 系統だけで目標を超える。肩は週1回（窓の3週ぶんで達成率 75%）。
	var logs []*setlog.SetLog
	n := 0
	for _, daysAgo := range []int{6, 13, 20} {
		for set := range 3 {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("ohp-%d-%d", daysAgo, set), planMonday.AddDays(-daysAgo), "ohp", 40, 10, 2))
		}
	}
	for week := 1; week <= 4; week++ {
		for i, id := range []string{"bench", "larsen", "tempo"} {
			day := planMonday.AddDays(-7*week + 2*i)
			for set := range 3 {
				n++
				logs = append(logs, mkLogOn(t, fmt.Sprintf("l%d-%d", n, set), day, id, 80, 6, 2))
			}
		}
		for i, id := range []string{"squat", "deadlift"} {
			day := planMonday.AddDays(-7*week + 2*i + 1)
			for set := range 3 {
				n++
				logs = append(logs, mkLogOn(t, fmt.Sprintf("l%d-%d", n, set), day, id, 120, 6, 2))
			}
		}
	}

	sessions, err := planning.DefaultSessionPlanner().Forecast(planning.PlanRequest{
		Program: prog, Target: target, Pool: pool,
		History: setlog.NewHistory(logs), Conditions: condition.NewConditionLog(nil),
		Date: planMonday,
	})
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}
	for _, s := range sessions {
		if containsAccessory(s, "ohp") {
			return
		}
	}
	t.Errorf("1週の先の回のどこにも ohp が出ない。肩が遅れているのに、胸の超過の罰則で締め出されている")
}
