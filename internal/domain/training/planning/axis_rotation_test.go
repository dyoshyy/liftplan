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

// 重点種目が軸に来た日は、何回目でも重い番で出ること。
//
// 以前は重点種目の番だけ 重い番 → 軽い番 → 派生 と一巡させていた（D-128）。
// 派生はバリエーションレーンが中1日空けて別の日に出すので、軸の側で軽い日を
// 作る理由が無い。重点種目の軸は常に重い番、派生はバリエーションレーンだけ。
//
// 強度は Epley の逆算に合わせる（割合 = 1 / (1 + (レップ + RIR) / 30)）。
// 0.88 は3レップ RIR1。推定と処方を同じ式で閉じる。
func TestSessionPlanner_FocusAxisIsAlwaysHeavy(t *testing.T) {
	cases := []struct {
		name string
		// sessions は系統が軸に来た回数（履歴に積むセッション数）。
		sessions int
	}{
		{name: "1回目", sessions: 3},
		// ここが軽い番（0.81・6レップ）だった。
		{name: "2回目も重い番", sessions: 4},
		// ここで派生（tempo）が軸に立っていた。
		{name: "3回目も本体を重い番で", sessions: 5},
		{name: "4回目", sessions: 6},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := rotationRequest(t, c.sessions)
			got := mustPlan(t, req)

			if len(got.Main()) != 1 {
				t.Fatalf("軸が %d 件。1件のはず: %v", len(got.Main()), got.Main())
			}
			set := got.Main()[0]
			if set.ExerciseID() != "bench" {
				t.Fatalf("軸が %s。bench のはず（派生は軸に立てない）", set.ExerciseID())
			}
			assertIntensity(t, req, set, 0.88)
			if got := set.TargetReps().Int(); got != 3 {
				t.Errorf("目標レップが %d。3 のはず", got)
			}
		})
	}
}

// rotationProgram は宣言がベンチだけ・重点種目もベンチのプログラム。
//
// 軸が必ずベンチの番になるので、重点種目が軸に来た日の処方だけを見られる。
//
// 週目標も一緒に返す。Program はもう週目標を持たない（#176）ので、
// 呼び出し側は PlanRequest.Target に別途渡す必要がある。
func rotationProgram(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl", "larsen", "tempo"},
		[]exercise.ExerciseID{"bench"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p, target
}

// rotationLogs はベンチを sessions 回ぶん積む。派生の記録も1回ずつ置く
// （派生をやっていても軸はベンチのままであることを見るため）。
func rotationLogs(t *testing.T, sessions int) []*setlog.SetLog {
	t.Helper()

	// ベンチは RIR0（軸の目標 RIR1 を割っている）で積む。同じ重量で目標を
	// 割らずに4セッション並ぶと進行の規則（D-138）が刻みを1つ乗せ、ここで
	// 見たい強度の比が刻みぶんずれる。
	logs := make([]*setlog.SetLog, 0, sessions+2)
	for i := 1; i <= sessions; i++ {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("bench-%d", i),
			planMonday.AddDays(-7*i), "bench", 85, 8, 0))
	}
	logs = append(logs,
		mkLogOn(t, "larsen-1", planMonday.AddDays(-14), "larsen", 75, 8, 2),
		mkLogOn(t, "tempo-1", planMonday.AddDays(-21), "tempo", 75, 8, 2))
	return logs
}

func rotationRequest(t *testing.T, sessions int) planning.PlanRequest {
	t.Helper()
	req := planRequest(t)
	req.Program, req.Target = rotationProgram(t)
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

// 重点種目の系統を中1日空けずに軸へ立てないこと。
//
// バリエーションレーンは系統を中1日空けて出す（variationRecoveryDays）が、
// 軸の側はその間隔を見ていなかった。重点デッドリフト・週5回で、月曜に
// バリエーションでデフィシット、火曜に軸で床引きが出た（開発用シミュ
// レーション）。軸は「宣言のうち最も古いもの」なので、派生を昨日やっても
// 本体は古いまま選ばれる。
//
// 軸を空にはしない。今日立てられる宣言が系統の本体しか無ければ、それを
// 出す。空にすると、その日は分割の意味どおりの主役が消える
// （heavyLift のフォールバックしない方針とは別の話で、あちらは分割に
// 合わない種目を出さないため）。
//
// 【変異1】focusRested が常に declared をそのまま返すようにする。
// 「昨日派生をやったら…」が bench になって落ちる。
// 【変異2】axis のフォールバック（2度目の heavyLift）を外す。
// 「宣言が本体だけなら…」が軸なしになって落ちる。
func TestSessionPlanner_AxisWaitsForTheFocusLineage(t *testing.T) {
	cases := []struct {
		name string
		// program は宣言と重点種目。どちらも重点はベンチ。
		program func(t *testing.T) (*program.Program, program.WeeklyVolumeTarget)
		// larsenDaysAgo はバリエーション（ベンチの派生）をやった日。
		larsenDaysAgo int
		want          exercise.ExerciseID
	}{
		{
			// ベンチが最も古い（9日前）ので、規則が無ければベンチが立つ。
			// 次に古いのはデッドリフト（4日前）。
			name: "昨日派生をやったら、本体は次に古い宣言に譲る",
			program: func(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
				return focusedProgram(t, "bench")
			},
			larsenDaysAgo: 1, want: "deadlift",
		},
		{
			// 開区間 (date-2, date) の外。月曜にやったら水曜から出してよい。
			name: "中1日空いていれば本体が立つ",
			program: func(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
				return focusedProgram(t, "bench")
			},
			larsenDaysAgo: 2, want: "bench",
		},
		{
			name:          "宣言が本体だけなら、昨日派生をやっていても本体が立つ",
			program:       rotationProgram,
			larsenDaysAgo: 1, want: "bench",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planRequest(t)
			req.Program, req.Target = c.program(t)
			req.History = setlog.NewHistory([]*setlog.SetLog{
				mkLogOn(t, "b", planMonday.AddDays(-9), "bench", 85, 8, 2),
				mkLogOn(t, "d", planMonday.AddDays(-4), "deadlift", 140, 8, 2),
				mkLogOn(t, "s", planMonday.AddDays(-3), "squat", 110, 8, 2),
				mkLogOn(t, "l", planMonday.AddDays(-c.larsenDaysAgo), "larsen", 75, 8, 2),
			})

			got := mustPlan(t, req).Main()
			if len(got) != 1 {
				t.Fatalf("軸が %d 件。1件のはず: %v", len(got), got)
			}
			if got[0].ExerciseID() != c.want {
				t.Errorf("軸が %s。%s のはず", got[0].ExerciseID(), c.want)
			}
		})
	}
}

// 全身法で、前日に下半身の宣言を軸でやったら、今日の軸は上半身に譲ること。
//
// 軸は「宣言のうち最後にやったのが最も古いもの」なので、宣言の順番が毎回
// 同じに回る。宣言が ベンチ・懸垂・RDL・スクワット なら RDL の翌回は必ず
// スクワットで、週5回だと連日になる（本番の記録で 10/9 RDL → 10/10 スクワット）。
//
// 下半身かどうかは主働（寄与1.0）の区分で見る。スクワット（大腿四頭）と
// デッドリフト（ハムストリング・脊柱起立筋）は主働が重ならないので、主働の
// 重なりでは判定できない。
//
// 【変異1】lowerBodyRested が常に candidates をそのまま返すようにする。
// 「昨日スクワットなら…」が deadlift になって落ちる。
// 【変異2】axis の2段目（focusRested だけを通した heavyLift）を外す。
// 「昨日ラーセンとスクワットなら…」が bench になって落ちる。
func TestSessionPlanner_AxisAlternatesLowerBody(t *testing.T) {
	lowerOnly := func(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
		t.Helper()
		target := mustTarget(t, map[training.MuscleRegion]float64{training.Quad: 12})
		p, err := program.NewProgram(mustFrequency(t, 5), planVolume(t),
			[]exercise.ExerciseID{"squat", "deadlift", "incline", "curl"},
			[]exercise.ExerciseID{"squat", "deadlift"}, "")
		if err != nil {
			t.Fatalf("プログラムの生成に失敗: %v", err)
		}
		return p, target
	}

	cases := []struct {
		name    string
		program func(t *testing.T) (*program.Program, program.WeeklyVolumeTarget)
		// daysAgo は各種目を最後にやった日。
		daysAgo map[string]int
		want    exercise.ExerciseID
	}{
		{
			// 最も古いのはデッドリフト（5日前）。規則が無ければそれが立つ。
			name:    "昨日スクワットなら、デッドリフトのほうが古くてもベンチ",
			program: planProgram,
			daysAgo: map[string]int{"bench": 3, "deadlift": 5, "squat": 1},
			want:    "bench",
		},
		{
			// focusRested と同じ開区間 (date-2, date)。中1日空けば下半身が続いてよい。
			name:    "中1日空いていればデッドリフト",
			program: planProgram,
			daysAgo: map[string]int{"bench": 3, "deadlift": 5, "squat": 2},
			want:    "deadlift",
		},
		{
			name:    "昨日が上半身なら、下半身の古いほう",
			program: planProgram,
			daysAgo: map[string]int{"bench": 1, "deadlift": 5, "squat": 3},
			want:    "deadlift",
		},
		{
			// 軸を空にすると、その日の主役が消える。
			name:    "宣言が下半身だけなら、昨日スクワットでもデッドリフト",
			program: lowerOnly,
			daysAgo: map[string]int{"deadlift": 5, "squat": 1},
			want:    "deadlift",
		},
		{
			// 昨日ラーセンでベンチを休ませ、昨日スクワットで下半身も休ませると
			// 候補が残らない。ベンチの系統を2日続けるより、下半身を続けるほうを取る。
			// 胸は軸とバリエーションで週に3回来るが、下半身は軸でしか来ない。
			name: "昨日ラーセンとスクワットなら、デッドリフト",
			program: func(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
				return focusedProgram(t, "bench")
			},
			// ベンチを最も古く（9日前）する。どちらの規則も外して選ぶとベンチになり、
			// どちらを優先したかが結果に出る。
			daysAgo: map[string]int{"bench": 9, "deadlift": 5, "squat": 1, "larsen": 1},
			want:    "deadlift",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planRequest(t)
			req.Program, req.Target = c.program(t)
			kg := map[string]float64{"bench": 85, "squat": 110, "deadlift": 140, "larsen": 75}
			logs := make([]*setlog.SetLog, 0, len(c.daysAgo))
			for id, d := range c.daysAgo {
				logs = append(logs, mkLogOn(t, id, planMonday.AddDays(-d), id, kg[id], 8, 2))
			}
			req.History = setlog.NewHistory(logs)

			got := mustPlan(t, req).Main()
			if len(got) != 1 {
				t.Fatalf("軸が %d 件。1件のはず: %v", len(got), got)
			}
			if got[0].ExerciseID() != c.want {
				t.Errorf("軸が %s。%s のはず", got[0].ExerciseID(), c.want)
			}
		})
	}
}
