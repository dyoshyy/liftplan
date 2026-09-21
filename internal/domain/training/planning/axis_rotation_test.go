package planning_test

import (
	"fmt"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// 重点種目の番に、軸が一巡すること。
//
// 分割が入るまでは散らす相手がいなかった。軸は宣言のうち最も古いもので
// 回るので、宣言が3つなら各種目は週1回しか軸に来ない（D-117）。だから
// 強度は定数でよかった（D-126）。
//
// 分割を入れると前提が変わる。上下2分割で宣言がBIG3なら、上半身の日に
// 立てる宣言はベンチだけで、同じ強度を週2回やることになる。散らす相手が
// 戻ったので、重点種目の番だけ一巡を持たせる。
//
// 強度は Epley の逆算に合わせる（割合 = 1 / (1 + (レップ + RIR) / 30)）。
// 0.88 は3レップ RIR1、0.81 は6レップ RIR1。推定と処方を同じ式で閉じる。
func TestSessionPlanner_FocusAxisRotates(t *testing.T) {
	cases := []struct {
		name string
		// sessions は系統が軸に来た回数（履歴に積むセッション数）。
		sessions  int
		want      exercise.ExerciseID
		intensity float64
	}{
		{name: "1周目は3レップ相当", sessions: 3, want: "bench", intensity: 0.88},
		{name: "2周目は6レップ相当", sessions: 4, want: "bench", intensity: 0.81},
		{name: "3周目は派生", sessions: 5, want: "tempo", intensity: 0.88},
		{name: "4周目で本体に戻る", sessions: 6, want: "bench", intensity: 0.88},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := rotationRequest(t, c.sessions)
			got := mustPlan(t, req)

			if len(got.Main()) != 1 {
				t.Fatalf("軸が %d 件。1件のはず: %v", len(got.Main()), got.Main())
			}
			set := got.Main()[0]
			if set.ExerciseID() != c.want {
				t.Fatalf("軸が %s。%s のはず", set.ExerciseID(), c.want)
			}
			assertIntensity(t, req, set, c.intensity)
		})
	}
}

// 一巡は、派生をやった日にも進むこと。
//
// 数えているのは「系統が軸に来たセッション」。本体の実施回数で数えると、
// 派生の日で止まって同じ派生が出続ける。
func TestSessionPlanner_RotationAdvancesOnDerivativeDays(t *testing.T) {
	// ベンチ2回のあと、派生だけをやった日が1つ。系統としては3回目。
	logs := []*setlog.SetLog{
		mkLogOn(t, "b1", planMonday.AddDays(-28), "bench", 85, 8, 2),
		mkLogOn(t, "b2", planMonday.AddDays(-21), "bench", 85, 8, 2),
		mkLogOn(t, "t1", planMonday.AddDays(-14), "tempo", 75, 8, 2),
	}

	req := planRequest(t)
	req.Program = rotationProgram(t)
	req.History = setlog.NewHistory(logs)

	set := mustPlan(t, req).Main()[0]
	if set.ExerciseID() != "bench" {
		t.Fatalf("軸が %s。bench のはず", set.ExerciseID())
	}
	// 3回ぶん進んでいれば一巡の先頭＝3レップ相当に戻る。
	// 本体の回数（2回）で数えていると6レップ相当になる。
	assertIntensity(t, req, set, 0.88)
}

// 重点種目でない宣言が軸の日は、一巡しないこと。
func TestSessionPlanner_NonFocusAxisKeepsTheHeavyPrescription(t *testing.T) {
	// ベンチを4回やっているので、一巡の位置は6レップ相当。
	// 軸がスクワットの日はその影響を受けない。
	logs := rotationLogs(t, 4)
	logs = append(logs,
		mkLogOn(t, "sq", planMonday.AddDays(-40), "squat", 110, 8, 2),
		mkLogOn(t, "dl", planMonday.AddDays(-3), "deadlift", 140, 8, 2))

	req := planRequest(t)
	req.Program = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(logs)

	set := mustPlan(t, req).Main()[0]
	if set.ExerciseID() != "squat" {
		t.Fatalf("前提: 軸がスクワットであること: %s", set.ExerciseID())
	}
	assertIntensity(t, req, set, 0.88)
}

// 重点種目を指定していなければ、一巡しないこと。
func TestSessionPlanner_NoFocusNoRotation(t *testing.T) {
	req := planRequest(t)
	req.Program = benchOnlyProgram(t) // 重点種目なし・宣言はベンチだけ
	req.History = setlog.NewHistory(rotationLogs(t, 4))

	set := mustPlan(t, req).Main()[0]
	if set.ExerciseID() != "bench" {
		t.Fatalf("前提: 軸がベンチであること: %s", set.ExerciseID())
	}
	assertIntensity(t, req, set, 0.88)
}

// 派生が選択に無ければ、派生の番でも本体が出ること。
//
// 使う種目から外した派生は軸にも出さない。D-114 で塞いだ抜け道
// （選択されていなくてもバリエーションは回る）を開け直さない。
func TestSessionPlanner_RotationSkipsUnselectedDerivatives(t *testing.T) {
	req := planRequest(t)
	req.Program = rotationProgramWithout(t, "larsen", "tempo")
	req.History = setlog.NewHistory(rotationLogs(t, 5)) // 派生の番

	set := mustPlan(t, req).Main()[0]
	if set.ExerciseID() != "bench" {
		t.Errorf("選択していない派生が軸に出ている: %s", set.ExerciseID())
	}
}

// rotationProgram は宣言がベンチだけ・重点種目もベンチのプログラム。
//
// 軸が必ずベンチの番になるので、一巡だけを見られる。
func rotationProgram(t *testing.T) *program.Program {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), target,
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl", "larsen", "tempo"},
		[]exercise.ExerciseID{"bench"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
}

// rotationLogs は系統が軸に来たセッションを sessions 回ぶん積む。
//
// 派生の最終実施日は本体より古くする（tempo が larsen より古い）。
// 派生の番にどちらが出るかを「最も古いもの」で決めていることを見るため。
// rotationProgramWithout は指定した種目を選択から外した rotationProgram。
func rotationProgramWithout(t *testing.T, drop ...exercise.ExerciseID) *program.Program {
	t.Helper()

	dropped := map[exercise.ExerciseID]bool{}
	for _, id := range drop {
		dropped[id] = true
	}
	selected := make([]exercise.ExerciseID, 0, 7)
	for _, id := range []exercise.ExerciseID{
		"bench", "squat", "deadlift", "incline", "curl", "larsen", "tempo",
	} {
		if !dropped[id] {
			selected = append(selected, id)
		}
	}

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), target, selected,
		[]exercise.ExerciseID{"bench"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
}

func rotationLogs(t *testing.T, sessions int) []*setlog.SetLog {
	t.Helper()
	if sessions < 3 {
		t.Fatalf("派生の日を含められない: %d", sessions)
	}

	logs := make([]*setlog.SetLog, 0, sessions+2)
	for i := 1; i <= sessions; i++ {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("bench-%d", i),
			planMonday.AddDays(-7*i), "bench", 85, 8, 2))
	}
	logs = append(logs,
		mkLogOn(t, "larsen-1", planMonday.AddDays(-14), "larsen", 75, 8, 2),
		mkLogOn(t, "tempo-1", planMonday.AddDays(-21), "tempo", 75, 8, 2))
	return logs
}

func rotationRequest(t *testing.T, sessions int) planning.PlanRequest {
	t.Helper()
	req := planRequest(t)
	req.Program = rotationProgram(t)
	req.History = setlog.NewHistory(rotationLogs(t, sessions))
	return req
}

// assertIntensity は処方された重量が、その種目の推定1RMの want 倍であること。
//
// 種目をまたいだ kg の比較は無意味（推定1RMが別物）なので、出てきた
// 種目自身の推定に対する比で見る。
func assertIntensity(t *testing.T, req planning.PlanRequest, set planning.PlannedSet, want float64) {
	t.Helper()

	orm, ok := planning.DefaultOneRepMaxEstimator().
		Estimate(req.History, set.ExerciseID(), req.Date)
	if !ok {
		t.Fatalf("前提: %s の推定1RMが出ること", set.ExerciseID())
	}
	target := findInPool(t, req.Pool, set.ExerciseID())
	pct, err := training.NewIntensityPct(want)
	if err != nil {
		t.Fatalf("強度: %v", err)
	}
	wantWeight, err := orm.WorkWeight(pct, target.Increment())
	if err != nil {
		t.Fatalf("実施重量: %v", err)
	}

	got, ok := set.Weight()
	if !ok {
		t.Fatal("重量が確定していない")
	}
	if got.Kg() != wantWeight.Kg() {
		t.Errorf("%s が %vkg。推定1RM %vkg の %v = %vkg のはず",
			set.ExerciseID(), got.Kg(), orm.Kg(), want, wantWeight.Kg())
	}
}
