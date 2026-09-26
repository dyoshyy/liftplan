package planning_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// バリエーションレーンが出す種目と、出さない条件。
//
// 重点種目の派生を、軸とは別の枠で回す。補助レーンが兼務していた「重点種目を
// 高頻度で回す」を独立させたもの（2026-09-10 の仕様）。
//
// 「たまたま胸の残差が大きいからベンチが週3回出ている」のではなく、
// 「ベンチを重点的に伸ばすと決めたから出ている」にする。前者は週目標を
// 変えると消える。
func TestSessionPlanner_VariationLane(t *testing.T) {
	// lastPerformed は種目を最後にやった日（月曜からの日数）。
	// 挙げなければ未着手。
	cases := []struct {
		name          string
		focus         exercise.ExerciseID
		lastPerformed map[exercise.ExerciseID]int
		// 期待するバリエーション。空なら「出ない」。
		want exercise.ExerciseID
	}{
		{
			name:  "重点種目を指定しなければ出ない",
			focus: "",
			// ベンチを3日前にやって軸を他へ移す。それでも出ない。
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -3, "squat": -7, "deadlift": -7,
			},
			want: "",
		},
		{
			// 同じ系統を1日に2回やることになる。
			name:  "軸が重点種目そのものの日は出ない",
			focus: "bench",
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -7, "squat": -3, "deadlift": -3,
			},
			want: "",
		},
		{
			// 「軸 != 重点種目」では塞げない。派生が軸に来る日がある。
			// ここを見落とすと、同じ系統が軸とバリエーションの両方に出る。
			name:  "軸が重点種目の派生の日も出ない",
			focus: "bench",
			lastPerformed: map[exercise.ExerciseID]int{
				"larsen": -7, "bench": -3, "squat": -3, "deadlift": -3,
			},
			want: "",
		},
		{
			name:  "前回の系統から1日しか空いていなければ出ない",
			focus: "bench",
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -1, "squat": -7, "deadlift": -7,
			},
			want: "",
		},
		{
			// 境界。中1日空いたら出る。月曜にやったら水曜から。
			name:  "中1日空いていれば出る",
			focus: "bench",
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -2, "squat": -7, "deadlift": -7,
			},
			want: "larsen",
		},
		{
			// 記録が無い＝一度もやっていない。ここが逆だと、新しく足した
			// 派生が永久に出ない。larsen < tempo なのでマスタ順で larsen。
			name:  "派生を1つもやっていなければ未着手を優先する",
			focus: "bench",
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -3, "squat": -7, "deadlift": -7,
			},
			want: "larsen",
		},
		{
			name:  "派生のうち最終実施日が最も古いものが出る",
			focus: "bench",
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -3, "squat": -7, "deadlift": -7,
				"larsen": -4, "tempo": -6,
			},
			want: "tempo",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planRequest(t)
			req.Program, req.Target = focusedProgram(t, c.focus)
			req.History = setlog.NewHistory(historyWithLastPerformed(t, c.lastPerformed))

			got := mustPlan(t, req).Variation()

			if c.want == "" {
				if len(got) != 0 {
					t.Errorf("バリエーションが出ている: %v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("バリエーションが %d 件。1件のはず: %v", len(got), got)
			}
			if got[0].ExerciseID() != c.want {
				t.Errorf("バリエーションが %s。%s のはず", got[0].ExerciseID(), c.want)
			}
		})
	}
}

// historyWithLastPerformed は、各種目を指定した日に1セットやった履歴を返す。
//
// 推定1RMが立つ量ではないので、重量を見るテストには使わない。どの種目が
// 選ばれるかだけを見る。
func historyWithLastPerformed(t *testing.T, last map[exercise.ExerciseID]int) []*setlog.SetLog {
	t.Helper()

	logs := make([]*setlog.SetLog, 0, len(last))
	for id, daysAgo := range last {
		logs = append(logs, mkLogOn(t, string(id)+"-last",
			planMonday.AddDays(daysAgo), string(id), 80, 8, 2))
	}
	return logs
}

// 派生が選択に入っていなければ、バリエーションは出ない。
//
// usablePool が選択で絞るので、選ばれていない種目はどのレーンにも現れない。
// D-114 で「選択されていなくてもバリエーションは回る」抜け道を塞いだ状態を
// 崩さないこと。
func TestSessionPlanner_VariationNeedsTheDerivedToBeSelected(t *testing.T) {
	req := planRequest(t)
	// 派生を選択から外したプログラム。重点種目はベンチのまま。
	req.Program, req.Target = mustProgramWithout(t, "bench", "larsen", "tempo")
	req.History = setlog.NewHistory(historyWithLastPerformed(t,
		map[exercise.ExerciseID]int{"bench": -3, "squat": -7, "deadlift": -7}))

	if got := mustPlan(t, req).Variation(); len(got) != 0 {
		t.Errorf("選択していない派生が出ている: %v", got)
	}
}

// mustProgramWithout は派生を選択から外したプログラムを返す。
func mustProgramWithout(t *testing.T, focus exercise.ExerciseID, drop ...exercise.ExerciseID) (*program.Program, program.WeeklyVolumeTarget) {
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
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t), selected, big3(), focus)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p, target
}

// バリエーションの重量は、その種目自身の記録から出る。
//
// 親から換算しない。D-113 で「推定1RMは種目ごとに持つ。係数換算をやめる」と
// 決めた。ここが親の推定を使うと、ラーセンプレスをやったことがない人にも
// ベンチの81%が出る。
//
// 以前ここにあった VariationWeightComesFromItsOwnRecord は、Main() から
// larsen を探していた。larsen は宣言に入っていないのでループ本体が一度も
// 走らず、緑だが何も守っていなかった。
func TestSessionPlanner_VariationWeightComesFromItsOwnRecord(t *testing.T) {
	// ベンチは重い、派生は軽い。推定が別々なら提示も別々になる。
	//
	// 派生2つとも記録する。片方だけ記録すると、未着手のもう片方が最優先で
	// 選ばれて重量が出ない（「一度もやっていない種目を優先する」の帰結）。
	logs := planHistory(t) // bench 85kg / squat 110 / deadlift 140 を3週
	for i, daysAgo := range []int{21, 14, 7} {
		for _, id := range []string{"larsen", "tempo"} {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("%s-%d", id, i),
				planMonday.AddDays(-daysAgo), id, 60, 8, 2))
		}
	}
	// ベンチを3日前にやって軸を他へ移す。系統の中1日もここで満たす。
	// tempo はその前日にして、ラーセンより新しくしておく。
	logs = append(logs,
		mkLogOn(t, "b-recent", planMonday.AddDays(-3), "bench", 85, 8, 2),
		mkLogOn(t, "tempo-recent", planMonday.AddDays(-4), "tempo", 60, 8, 2))

	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(logs)

	s := mustPlan(t, req)
	got := s.Variation()
	if len(got) != 1 || got[0].ExerciseID() != "larsen" {
		t.Fatalf("前提: ラーセンプレスがバリエーションに出ること: %v", got)
	}

	w, ok := got[0].Weight()
	if !ok {
		t.Fatal("バリエーションの重量が確定していない")
	}
	// 60kg の記録から出るので、85kg の記録から出るベンチより軽い。
	// 同じなら親の推定を使っている。
	benchWeight := mainWeight(t, s, s.Main()[0].ExerciseID())
	if w.Kg() >= benchWeight {
		t.Errorf("バリエーションの重量 %vkg が軸の %vkg 以上。親の推定を使っている",
			w.Kg(), benchWeight)
	}
	if w.Kg() <= 0 {
		t.Errorf("バリエーションの重量が0以下: %v", w.Kg())
	}
}

// バリエーションの処方は、軸より軽く補助より重い。
//
// 3レーンの強度とセット数をそれぞれ定数で持つので、値そのものを固定する。
// ここが緩いと、定数を書き換えても誰も気づかない。
//
// 種目をまたいで kg を比べても意味が無い（推定1RMが別物）ので、
// 出てきた種目自身の推定1RMに対する比で見る。期待する 0.81 は
// 実装とは独立にここへ書く。
func TestSessionPlanner_VariationPrescriptionIsPinned(t *testing.T) {
	const (
		wantIntensity = 0.80
		wantSets      = 3
		wantRIR       = 2
	)

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

	s := mustPlan(t, req)
	if len(s.Variation()) != 1 {
		t.Fatalf("前提: バリエーションが出ること: %v", s.Variation())
	}
	got := s.Variation()[0]

	if n := got.Sets().Int(); n != wantSets {
		t.Errorf("セット数が %d。%d のはず", n, wantSets)
	}
	if r := got.TargetRIR(); r.Int() != wantRIR {
		t.Errorf("目標RIRが %v。%d のはず", r, wantRIR)
	}

	// 同じ推定器・同じ増加単位で 0.81 を当て直す。
	orm, ok := planning.DefaultOneRepMaxEstimator().
		Estimate(req.History, got.ExerciseID(), req.Date)
	if !ok {
		t.Fatalf("前提: 推定1RMが出ること: %v", got.ExerciseID())
	}
	target := exerciseByID(t, req.Pool, got.ExerciseID())
	pct, err := training.NewIntensityPct(wantIntensity)
	if err != nil {
		t.Fatalf("強度: %v", err)
	}
	want, err := orm.WorkWeight(pct, target.Increment())
	if err != nil {
		t.Fatalf("実施重量: %v", err)
	}

	w, ok := got.Weight()
	if !ok {
		t.Fatal("重量が確定していない")
	}
	if w.Kg() != want.Kg() {
		t.Errorf("重量が %vkg。推定1RM %vkg の %v = %vkg のはず",
			w.Kg(), orm.Kg(), wantIntensity, want.Kg())
	}
}

// 体調の補正はバリエーションにも乗る。
//
// 3レーンとも同じ rirBump を受け取るが、受け取ったあと使っているかは
// レーンごとに別の話。軸だけ見ていると、バリエーションで落とし忘れても
// 気づかない。
func TestSessionPlanner_VariationTakesTheConditionRIRBump(t *testing.T) {
	logs := planHistory(t)
	for i, daysAgo := range []int{21, 14, 7} {
		for _, id := range []string{"larsen", "tempo"} {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("%s-%d", id, i),
				planMonday.AddDays(-daysAgo), id, 85, 8, 2))
		}
	}
	logs = append(logs, mkLogOn(t, "b-recent", planMonday.AddDays(-3), "bench", 85, 8, 2))

	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(logs)

	// 2週間 7h で寝ていた人が当日 5.4h。既定の閾値 1.5h を割る。
	conds := []condition.DailyCondition{
		condition.NewDailyCondition(planMonday).WithSleepHours(5.4),
	}
	for i := 1; i <= 14; i++ {
		conds = append(conds,
			condition.NewDailyCondition(planMonday.AddDays(-i)).WithSleepHours(7))
	}
	req.Conditions = condition.NewConditionLog(conds)

	s := mustPlan(t, req)
	if len(s.Variation()) != 1 {
		t.Fatalf("前提: バリエーションが出ること: %v", s.Variation())
	}
	if got := s.Variation()[0].TargetRIR().Int(); got != 3 {
		t.Errorf("寝不足なのに目標RIRが %d。素の2に補正+1で3のはず", got)
	}
}

// exerciseByID はプールから種目を引く。見つからなければ失敗。
func exerciseByID(t *testing.T, pool []*exercise.Exercise, id exercise.ExerciseID) *exercise.Exercise {
	t.Helper()
	for _, e := range pool {
		if e.ID() == id {
			return e
		}
	}
	t.Fatalf("プールに %v が無い", id)
	return nil
}

// 記録が無ければ重量は未確定。初回は本人が決める。
func TestSessionPlanner_VariationWithoutRecordHasNoWeight(t *testing.T) {
	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(historyWithLastPerformed(t,
		map[exercise.ExerciseID]int{"bench": -3, "squat": -7, "deadlift": -7}))

	got := mustPlan(t, req).Variation()
	if len(got) != 1 {
		t.Fatalf("バリエーションが出ていない: %v", got)
	}
	if w, ok := got[0].Weight(); ok {
		t.Errorf("記録が無いのに重量が出ている: %v", w.Kg())
	}
}

// 当日バリエーションを記録しても、今日のリストは変わらない。
//
// req.History をそのまま使うと、1セット記録した瞬間に系統が「最近やった」に
// なってバリエーションが自分の下で消える。D-116 が消した失敗の形そのもの。
func TestSessionPlanner_TodaysLogDoesNotRemoveTheVariation(t *testing.T) {
	base := historyWithLastPerformed(t,
		map[exercise.ExerciseID]int{"bench": -3, "squat": -7, "deadlift": -7})

	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(base)

	before := mustPlan(t, req).Variation()
	if len(before) != 1 {
		t.Fatalf("前提: バリエーションが出ること: %v", before)
	}

	// 今日そのバリエーションを1セットこなす。
	req.History = setlog.NewHistory(append(base,
		mkLogOn(t, "today", planMonday, string(before[0].ExerciseID()), 60, 8, 2)))

	after := mustPlan(t, req).Variation()
	if len(after) != 1 || after[0].ExerciseID() != before[0].ExerciseID() {
		t.Errorf("当日の記録でバリエーションが変わった: %v → %v", before, after)
	}
}

// バリエーションが埋めた分は残差から引かれる。
//
// 引かないと、胸をラーセンで埋めたうえに補助でも埋める。台帳への加算は
// 残差を出す前に済んでいる必要がある。
//
// 補助の「件数」で見ると、バリエーションが除外されたぶん1件減るだけでも
// 通ってしまう。減ったのが残差のせいだと分かるよう、同じ区分を狙う補助を
// 十分に用意して、その区分に割り当てられたセット数を見る。
func TestSessionPlanner_SubtractsVariationCoverageFromResidual(t *testing.T) {
	// 大胸筋中部を狙う補助を5つ足す。1つだと、残差が減っても「候補が
	// 尽きた」のか「残差が尽きた」のか区別できない。
	pool := planPool(t)
	ids := []exercise.ExerciseID{
		"bench", "squat", "deadlift", "incline", "curl", "larsen", "tempo",
	}
	for i := range 5 {
		id := fmt.Sprintf("chest_%d", i)
		pool = append(pool, mkAccessory(t, id,
			map[training.MuscleRegion]float64{training.ChestMid: 1.0}))
		ids = append(ids, exercise.ExerciseID(id))
	}

	// 大胸筋中部だけを週目標に置く。ここの消化だけを見る。
	//
	// 12セットにしているのは、補助が3セット刻みで割り当てられるため。
	// 24だと残差が 8 → 6.7 に減っても同じ3種目（9セット）が出て、差が
	// 出力に現れない。
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})
	build := func(focus exercise.ExerciseID) *program.Program {
		t.Helper()
		p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t), ids, big3(), focus)
		if err != nil {
			t.Fatalf("プログラムの生成に失敗: %v", err)
		}
		return p
	}

	logs := historyWithLastPerformed(t,
		map[exercise.ExerciseID]int{"bench": -3, "squat": -7, "deadlift": -7})

	chestSets := func(s planning.PlannedSession) int {
		total := 0
		for _, a := range s.Accessories() {
			if strings.HasPrefix(string(a.ExerciseID()), "chest_") {
				total += a.Sets().Int()
			}
		}
		return total
	}

	req := planRequest(t)
	req.Pool = pool
	req.Target = target
	req.History = setlog.NewHistory(logs)

	req.Program = build("")
	without := mustPlan(t, req)

	req.Program = build("bench")
	with := mustPlan(t, req)

	if len(with.Variation()) != 1 {
		t.Fatalf("前提: バリエーションが出ること: %v", with.Variation())
	}
	if got, base := chestSets(with), chestSets(without); got >= base {
		t.Errorf("バリエーションが残差から引かれていない: 胸の補助が %d → %d セット",
			base, got)
	}
}

// 今日のバリエーションは、補助にも出さない。
//
// 出すと同じ種目が今日のリストに2回並ぶ。
// 重点種目の派生は、選ばれなかったものも補助に出ない。
//
// バリエーションに選ばれた1つを外すだけでは足りない。派生が2つ以上ある
// と、選ばれなかったほうが補助として同じ日に出る。週5で回すと脚の日に
// ベンチの派生が2つ乗り、上半身のボリュームが 17.1 まで膨らんでいた。
//
// 宣言していても重点でない種目の派生は外さない。そちらは補助が唯一の
// 出口で、外すと計画から消える（実測で胸が週目標の163%まで超過した）。
func TestSessionPlanner_FocusDerivativesNeverAppearAsAccessories(t *testing.T) {
	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	// 胸の残差を大きくして、補助が胸を狙いにいく状況を作る。
	// 軸はスクワットに寄せ、ベンチ系は今週まだ。
	req.History = setlog.NewHistory(historyWithLastPerformed(t,
		map[exercise.ExerciseID]int{"bench": -3, "squat": -7, "deadlift": -7}))

	s := mustPlan(t, req)
	if len(s.Variation()) != 1 {
		t.Fatalf("前提: バリエーションが出ること: %v", s.Variation())
	}

	// larsen と tempo は両方ともベンチの派生。片方がバリエーションに
	// 出るので、もう片方が補助に出ていないことを見る。
	derived := map[exercise.ExerciseID]bool{"larsen": true, "tempo": true, "bench": true}
	for _, a := range s.Accessories() {
		if derived[a.ExerciseID()] {
			t.Errorf("重点種目の系統が補助に出ている: %v → %v",
				a.ExerciseID(), accessoryIDs(s))
		}
	}
}

// 重点でない宣言種目の派生は、補助の候補に残ること。
//
// そちらは補助が唯一の出口。外すと RDL やフロントスクワットが計画から
// 消える。線引きは「専用レーンを持っているか」で、持っているのは重点
// 種目の系統だけ。
func TestSessionPlanner_NonFocusDerivativesStayAsAccessories(t *testing.T) {
	// スクワットの派生をプールに足す。重点はベンチなので、この派生は
	// バリエーションレーンには乗らない。
	pool := append(planPool(t), mustExercise(t, exercise.ExerciseParams{
		ID: "front_squat", Name: "front_squat",
		Stimulus:    map[training.MuscleRegion]float64{training.Quad: 1.0},
		IncrementKg: 2.5, DerivedFrom: "squat",
	}))

	target := mustTarget(t, map[training.MuscleRegion]float64{training.Quad: 30})
	prog, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "larsen", "tempo", "front_squat"},
		big3(), "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Pool, req.Program = pool, prog
	req.Target = target
	// 軸をベンチに寄せる。脚の残差が大きいので補助は大腿四頭筋を狙う。
	req.History = setlog.NewHistory(historyWithLastPerformed(t,
		map[exercise.ExerciseID]int{"bench": -7, "squat": -3, "deadlift": -3}))

	s := mustPlan(t, req)
	for _, a := range s.Accessories() {
		if a.ExerciseID() == "front_squat" {
			return
		}
	}
	t.Errorf("重点でない宣言の派生が補助から消えている: %v", accessoryIDs(s))
}

func TestSessionPlanner_VariationIsNotAlsoAnAccessory(t *testing.T) {
	req := planRequest(t)
	req.Program, req.Target = focusedProgram(t, "bench")
	req.History = setlog.NewHistory(historyWithLastPerformed(t,
		map[exercise.ExerciseID]int{"bench": -3, "squat": -7, "deadlift": -7}))

	s := mustPlan(t, req)
	got := s.Variation()
	if len(got) != 1 {
		t.Fatalf("前提: バリエーションが出ること: %v", got)
	}
	for _, a := range s.Accessories() {
		if a.ExerciseID() == got[0].ExerciseID() {
			t.Errorf("バリエーションが補助にも出ている: %v", accessoryIDs(s))
		}
	}
}
