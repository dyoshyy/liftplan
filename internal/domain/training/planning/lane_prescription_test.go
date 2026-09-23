package planning_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// axisRow は模擬ユーザーが軸をこなした1セッションぶん。
type axisRow struct {
	date     training.Date
	estimate float64 // その日の始まりの推定1RM
	weight   float64 // 処方された重量
	reps     int     // 模擬ユーザーが記録したレップ数
	rir      int
}

// steadyReps は、推定1RMが変わらない記録のレップ数。
//
// 処方はレップ数を持たない。重量と目標 RIR だけを渡し、何レップやるかは
// その日の本人が決める。「目標どおりにこなす」を恣意的に決めないため、
// 模擬ユーザーを**推定1RMのとおりの実力で、目標 RIR ちょうどで止める人**と
// 定義する。Epley を逆に解き、推定1RMに最も近い値を出すレップ数を記録する：
//
//	reps + RIR = round(30 × (推定1RM / 重量 − 1))
//
// レップ数は整数なので、推定1RMをぴったり保つ記録は一般に存在しない。
// round は「最も近い」を取るので、推定を上にも下にも寄せない。
// 刻みの丸めで処方が推定 × 強度より重く出た日は、そのぶんレップが減る。
func steadyReps(t *testing.T, estimate, weight float64, rir int) int {
	t.Helper()
	reps := int(math.Round(30*(estimate/weight-1))) - rir
	if reps < 1 {
		t.Fatalf("推定 %vkg に対して %vkg は重すぎ、RIR%d で1レップも挙がらない",
			estimate, weight, rir)
	}
	return reps
}

// simulateAxis は prog で sessions 回、軸だけを steadyReps でこなし続ける。
// 通うのは月・水・金。補助は記録しない（軸の推定に影響しない）。
func simulateAxis(t *testing.T, prog *program.Program, sessions int) []axisRow {
	t.Helper()

	req := planRequest(t)
	req.Program = prog
	logs := planHistory(t)
	estimator := planning.DefaultOneRepMaxEstimator()

	rows := make([]axisRow, 0, sessions)
	for i := range sessions {
		req.Date = planMonday.AddDays(i/3*7 + i%3*2)
		req.History = setlog.NewHistory(logs)

		s := mustPlan(t, req)
		if len(s.Main()) != 1 {
			t.Fatalf("%d本目: 軸が %d 件", i+1, len(s.Main()))
		}
		set := s.Main()[0]
		w, ok := set.Weight()
		if !ok {
			t.Fatalf("%d本目: 軸の重量が出ていない", i+1)
		}
		orm, ok := estimator.Estimate(req.History.Before(req.Date), set.ExerciseID(), req.Date)
		if !ok {
			t.Fatalf("%d本目: 推定1RMが出ない", i+1)
		}
		rir := set.TargetRIR().Int()
		reps := steadyReps(t, orm.Kg(), w.Kg(), rir)

		for k := range set.Sets().Int() {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("sim-%d-%d", i, k), req.Date,
				string(set.ExerciseID()), w.Kg(), reps, rir))
		}
		rows = append(rows, axisRow{
			date: req.Date, estimate: orm.Kg(), weight: w.Kg(), reps: reps, rir: rir,
		})
	}
	return rows
}

func logAxisRows(t *testing.T, rows []axisRow) {
	t.Helper()
	for i, r := range rows {
		t.Logf("%2d  %v  推定 %7.2f  処方 %6.2f  記録 %dレップ RIR%d",
			i+1, r.date, r.estimate, r.weight, r.reps, r.rir)
	}
}

// 目標 RIR を割らずにこなし続けたら、軸の重量が上がること。
func TestSessionPlanner_AxisWeightProgresses(t *testing.T) {
	cases := []struct {
		name string
		prog func(t *testing.T) *program.Program
		// 同じ役割が回ってくる間隔。重点種目の一巡では 3レップ相当と
		// 6レップ相当が交互に来るので、同じ役割どうしで比べる。
		period int
	}{
		{name: "軸が毎回3レップ相当", prog: benchOnlyProgram, period: 1},
		{
			name: "重点種目の一巡（派生なし）",
			prog: func(t *testing.T) *program.Program {
				return rotationProgramWithout(t, "larsen", "tempo")
			},
			period: 3,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows := simulateAxis(t, c.prog(t), 12)
			logAxisRows(t, rows)

			for pos := range c.period {
				first := rows[pos].weight
				raised := false
				for i := pos; i < len(rows); i += c.period {
					if rows[i].weight >= first+2.5 {
						raised = true
					}
				}
				if !raised {
					t.Errorf("%d本目と同じ役割の軸が %d本やっても %vkg から上がらない",
						pos+1, len(rows), first)
				}
			}
		})
	}
}
