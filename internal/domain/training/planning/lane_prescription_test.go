package planning_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
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
//
// 始まりの履歴はベンチ1セッションだけ（85kg×8 RIR2、推定 113.33kg）。
// 上乗せには「前の日に推定が立っていたセッション」が overloadSessions 個
// 要るので、最初の3本は必ず推定どおりの重量になり、比べる起点にできる。
func simulateAxis(t *testing.T, prog *program.Program, sessions int) []axisRow {
	t.Helper()

	req := planRequest(t)
	req.Program = prog
	req.Target = req.Program.WeeklyTarget()
	logs := []*setlog.SetLog{
		mkLogOn(t, "start", planMonday.AddDays(-7), "bench", 85, 8, 2),
	}
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

// 目標 RIR を割らずにこなし続けたら、軸の重量が刻みちょうど1つ上がること。
//
// 上げないと、推定1RMのとおりの実力で目標 RIR ちょうどで止める人には
// 推定と処方が固定点に落ち、重量が永久に同じになる（D-014）。上に行く
// 判断だけ本人に残る（#169）。実装前はどちらのケースも 12本やって
// 100kg（6レップ相当は 92.5kg）のまま一度も動かなかった。
//
// 上がる幅も見る。刻み2つ上げる実装でも「上がった」は満たすので、
// 「上がった重量 = 起点 + 刻み1つ」で固定する。
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

			// ベンチの刻みは 2.5kg。
			const increment = 2.5
			for pos := range c.period {
				first := rows[pos].weight
				top := first
				for i := pos; i < len(rows); i += c.period {
					top = max(top, rows[i].weight)
				}
				if top != first+increment {
					t.Errorf("%d本目と同じ役割の軸が、%d本やって %vkg → 最大 %vkg。%vkg のはず",
						pos+1, len(rows), first, top, first+increment)
				}
			}
		})
	}
}

// 軸の上乗せが、どの条件で効いてどの条件で効かないか。
//
// どのケースも、ベンチ85kg×8 RIR2（推定 113.33kg、0.88 で 100kg）の
// 1セッションを起点に積む。起点の日は前に推定が無いので、判定の窓に
// 入っていれば平坦とは見なされない。記録は各セッション3セット。
func TestSessionPlanner_AxisOverload(t *testing.T) {
	type session struct {
		daysAgo   int
		kg        float64
		reps, rir int
	}
	cases := []struct {
		name     string
		sessions []session
		exercise exercise.ExerciseID
		want     float64
	}{
		{
			// 100×3 RIR1 は Epley で 113.33kg。推定は動かない。
			// 100 + 2.5
			name: "推定が刻み単位で平坦で、目標 RIR を割っていなければ刻みを1つ乗せる",
			sessions: []session{
				{35, 85, 8, 2}, {21, 100, 3, 1}, {14, 100, 3, 1}, {7, 100, 3, 1},
			},
			exercise: "bench", want: 102.5,
		},
		{
			// 100×4 RIR0 も Epley で 113.33kg。推定は上と同じで、RIR だけが違う。
			name: "窓の中で1度でも目標 RIR を割っていたら乗せない",
			sessions: []session{
				{35, 85, 8, 2}, {21, 100, 3, 1}, {14, 100, 3, 1}, {7, 100, 4, 0},
			},
			exercise: "bench", want: 100,
		},
		{
			// 上げてもらった 102.5kg で RIR0 まで追い込んだ（×3 で 112.75kg）。
			// 推定は 0.3×112.75 + 0.7×113.33 = 113.16kg、0.88 で 99.6 → 100kg。
			// 下げる規則が無くても、RIR の条件が外れて推定どおりに戻る。
			name: "上げた重量で目標 RIR を割ったら、次は推定どおりに戻る",
			sessions: []session{
				{35, 85, 8, 2}, {28, 100, 3, 1}, {21, 100, 3, 1}, {14, 100, 3, 1},
				{7, 102.5, 3, 0},
			},
			exercise: "bench", want: 100,
		},
		{
			// 100×5 RIR1 は 120kg。推定は 0.3×120 + 0.7×113.33 = 115.33kg、
			// 0.88 で 101.5 → 102.5kg。推定が既に刻み1つ上げているので、
			// さらに乗せて 105kg にはしない。
			name: "推定が刻み1つ動いていたら乗せない",
			sessions: []session{
				{35, 85, 8, 2}, {21, 100, 3, 1}, {14, 100, 3, 1}, {7, 100, 5, 1},
			},
			exercise: "bench", want: 102.5,
		},
		{
			// 窓の3セッションのうち最古（起点）は、その日の始まりに推定が
			// 無い。平坦だったかどうか分からないので判定しない。
			name: "窓に推定の立たない日があれば乗せない",
			sessions: []session{
				{21, 85, 8, 2}, {14, 100, 3, 1}, {7, 100, 3, 1},
			},
			exercise: "bench", want: 100,
		},
		{
			// 40×10 RIR2 は 56kg、0.71 で 39.8 → 40kg。軸なら 42.5kg になる形。
			name: "補助には乗せない",
			sessions: []session{
				{35, 40, 10, 2}, {21, 40, 10, 2}, {14, 40, 10, 2}, {7, 40, 10, 2},
			},
			exercise: "incline", want: 40,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := make([]*setlog.SetLog, 0, len(c.sessions)*3)
			for i, s := range c.sessions {
				for k := range 3 {
					logs = append(logs, mkLogOn(t, fmt.Sprintf("s%d-%d", i, k),
						planMonday.AddDays(-s.daysAgo), string(c.exercise), s.kg, s.reps, s.rir))
				}
			}
			req := planRequest(t)
			req.Program = benchOnlyProgram(t)
			req.Target = req.Program.WeeklyTarget()
			req.History = setlog.NewHistory(logs)

			got, ok := plannedWeight(t, mustPlan(t, req), c.exercise)
			if !ok {
				t.Fatalf("前提: %s の重量が出ること", c.exercise)
			}
			if got != c.want {
				t.Errorf("%s が %vkg。%vkg のはず", c.exercise, got, c.want)
			}
		})
	}
}

// mixedWindowProgram は宣言がベンチとスクワットの2つ、重点種目がベンチ、
// 派生の選択は tempo だけのプログラム。larsen は選択から外す。
//
// 宣言を2つにするのは、軸をベンチとスクワットで交互に回すため。ベンチの
// 番の日は派生（tempo）が一巡の3番目（派生の番）で軸に立ち、スクワットの
// 番の日は「軸が系統に含まれない」条件が満たされてバリエーションレーンに
// tempo が出る。同じ種目が2つの役割を行き来する状況を、宣言を絞らずに作る。
func mixedWindowProgram(t *testing.T) *program.Program {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t), target,
		[]exercise.ExerciseID{"bench", "squat", "incline", "curl", "tempo"},
		[]exercise.ExerciseID{"bench", "squat"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
}

// 派生（tempo）が、軸の重い日とバリエーションの軽い日を行き来する窓で、
// 上乗せの判定がバリエーションの日を証拠に使わないこと。
//
// 履歴は役割を持たない。tempo は「派生の番」で軸（0.88・RIR1）に立つ日と、
// スクワットが軸の日にバリエーション（0.80・RIR2）で出る日を両方持つ。
// 直近3セッションの窓にバリエーションの日が混ざると、その日の記録RIR（2）は
// 軸の目標RIR（1）を割っていないので素通りし、「推定が平坦」の判定も
// （その日の始まりの推定から今日の役割の強度で引き直すだけなので）軽い
// 重量で実施したことと無関係に成立してしまう。窓の3セッションのうち
// 実際に重い処方でやったのは1日（6日前）だけなのに、上乗せが発火する。
//
// 数値は実測（DefaultOneRepMaxEstimator）で選んだ。tempo の起点は
// 75kg×8 RIR2（Epley 100kg）。0.88 で 87.5kg（100×0.88=88→2.5刻みで
// 87.5に丸まる）。バリエーションの日は 80kg×5 RIR2（Epley 98.67kg）、
// 軸の日は 87.5kg×3 RIR1（Epley 99.17kg）。EWMA（α=0.3）で畳み込んでも
// 今日までのどの時点でも「その日の始まりの推定 × 0.88」は 87.5kg に
// 丸まったまま動かない。
func TestSessionPlanner_AxisOverload_ExcludesVariationDays(t *testing.T) {
	req := planRequest(t)
	req.Program = mixedWindowProgram(t)
	req.Target = req.Program.WeeklyTarget()
	req.History = setlog.NewHistory([]*setlog.SetLog{
		// 軸がベンチとスクワットを交互に回すための最小限の履歴。
		// ベンチを一度も遠くに離しておくと、スクワットより古いので
		// 毎回ベンチが軸に立つ（スクワットは直近にやったことにして外す）。
		mkLogOn(t, "bench-old", planMonday.AddDays(-25), "bench", 85, 8, 2),
		mkLogOn(t, "squat-recent", planMonday.AddDays(-2), "squat", 110, 8, 2),

		// tempo 自身の履歴。起点 → バリエーション → 軸 → バリエーション。
		mkLogOn(t, "baseline", planMonday.AddDays(-16), "tempo", 75, 8, 2),
		mkLogOn(t, "var9-0", planMonday.AddDays(-9), "tempo", 80, 5, 2),
		mkLogOn(t, "var9-1", planMonday.AddDays(-9), "tempo", 80, 5, 2),
		mkLogOn(t, "var9-2", planMonday.AddDays(-9), "tempo", 80, 5, 2),
		mkLogOn(t, "heavy6-0", planMonday.AddDays(-6), "tempo", 87.5, 3, 1),
		mkLogOn(t, "heavy6-1", planMonday.AddDays(-6), "tempo", 87.5, 3, 1),
		mkLogOn(t, "heavy6-2", planMonday.AddDays(-6), "tempo", 87.5, 3, 1),
		mkLogOn(t, "var3-0", planMonday.AddDays(-3), "tempo", 80, 5, 2),
		mkLogOn(t, "var3-1", planMonday.AddDays(-3), "tempo", 80, 5, 2),
		mkLogOn(t, "var3-2", planMonday.AddDays(-3), "tempo", 80, 5, 2),
	})

	s := mustPlan(t, req)
	if len(s.Main()) != 1 || s.Main()[0].ExerciseID() != "tempo" {
		t.Fatalf("前提: 今日は tempo が派生の番で軸に立つこと: %v", s.Main())
	}
	if rir := s.Main()[0].TargetRIR().Int(); rir != 1 {
		t.Fatalf("前提: 今日の tempo は軸（目標RIR1）であること: RIR%d", rir)
	}

	got, ok := plannedWeight(t, s, "tempo")
	if !ok {
		t.Fatal("前提: tempo の重量が出ること")
	}
	const want = 87.5 // バリエーションの日を除けば、重い処方でやったのは1日だけで窓が満たない
	if got != want {
		t.Errorf("tempo が %vkg。%vkg のはず（バリエーションの日を上乗せの証拠に使っている）", got, want)
	}
}
