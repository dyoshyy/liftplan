package planning_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// 処方の目標レップ数は、役割の強度と RIR から Epley を逆に解いた値になる。
//
//	reps = round(30 × (1/強度 − 1) − RIR)
//
// 強度は推定の式と同じ Epley に合わせてあるので、レップ数を別の定数として
// 持つと、強度を動かしたときに食い違う。ここでは役割ごとの値を固定したうえで、
// 式との一致も見る。
func TestSessionPlanner_TargetReps(t *testing.T) {
	// variation は 5.5 → 四捨五入で 6。
	cases := []struct {
		name      string
		plan      func(t *testing.T) planning.PlannedSet
		want      int
		intensity float64
		rir       int
	}{
		{
			name: "軸は3レップ", want: 3, intensity: 0.88, rir: 1,
			plan: func(t *testing.T) planning.PlannedSet {
				req := rotationRequest(t, 3)
				return mustPlan(t, req).Main()[0]
			},
		},
		{
			name: "重点種目の2番目は6レップ", want: 6, intensity: 0.81, rir: 1,
			plan: func(t *testing.T) planning.PlannedSet {
				req := rotationRequest(t, 4)
				return mustPlan(t, req).Main()[0]
			},
		},
		{
			name: "バリエーションは6レップ（5.5 を四捨五入）", want: 6, intensity: 0.80, rir: 2,
			plan: func(t *testing.T) planning.PlannedSet {
				req := variationRequest(t)
				s := mustPlan(t, req)
				if len(s.Variation()) != 1 {
					t.Fatalf("前提: バリエーションが出ること: %v", s.Variation())
				}
				return s.Variation()[0]
			},
		},
		{
			name: "補助は10レップ", want: 10, intensity: 0.71, rir: 2,
			plan: func(t *testing.T) planning.PlannedSet {
				req := planRequest(t)
				s := mustPlan(t, req)
				if len(s.Accessories()) == 0 {
					t.Fatalf("前提: 補助が出ること")
				}
				return s.Accessories()[0]
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set := c.plan(t)

			if got := set.TargetReps().Int(); got != c.want {
				t.Errorf("目標レップが %d。%d のはず", got, c.want)
			}
			// 式との一致。定数を直したのに片方だけ動いた、を拾う。
			formula := int(math.Round(30*(1/c.intensity-1))) - c.rir
			if c.want != formula {
				t.Errorf("前提: 期待値 %d が Epley の逆算 %d と食い違う", c.want, formula)
			}
		})
	}
}

// 睡眠不足の RIR の上乗せは、目標レップ数を変えない。
//
// あちらは「今日はきつめに切り上げてよい」という調整で、狙うレップ数の
// 再計算ではない（#231）。
func TestSessionPlanner_TargetRepsIgnoresTheConditionRIRBump(t *testing.T) {
	req := variationRequest(t)
	base := mustPlan(t, req).Variation()[0]

	conds := []condition.DailyCondition{
		condition.NewDailyCondition(planMonday).WithSleepHours(5.4),
	}
	for i := 1; i <= 14; i++ {
		conds = append(conds,
			condition.NewDailyCondition(planMonday.AddDays(-i)).WithSleepHours(7))
	}
	req.Conditions = condition.NewConditionLog(conds)
	tired := mustPlan(t, req).Variation()[0]

	if tired.TargetRIR().Int() != base.TargetRIR().Int()+1 {
		t.Fatalf("前提: 寝不足で RIR が1つ上がること: %d → %d",
			base.TargetRIR().Int(), tired.TargetRIR().Int())
	}
	if tired.TargetReps() != base.TargetReps() {
		t.Errorf("寝不足で目標レップが %d → %d に動いた。動かないはず",
			base.TargetReps().Int(), tired.TargetReps().Int())
	}
}

// variationRequest はバリエーションレーンが出る日の計画要求。
func variationRequest(t *testing.T) planning.PlanRequest {
	t.Helper()

	logs := planHistory(t)
	for i, daysAgo := range []int{21, 14, 7} {
		for _, id := range []string{"larsen", "tempo"} {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("%s-%d", id, i),
				planMonday.AddDays(-daysAgo), id, 85, 8, 2))
		}
	}
	// ベンチを3日前にやって軸を他へ移すと、派生がバリエーションに出る。
	logs = append(logs, mkLogOn(t, "b-recent", planMonday.AddDays(-3), "bench", 85, 8, 2))

	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(logs)
	return req
}
