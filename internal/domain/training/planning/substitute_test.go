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
