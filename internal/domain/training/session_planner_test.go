package training_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

var planMonday = training.MustDate(2026, time.August, 17) // 月曜

func mainExercise(t *testing.T, id string, lift training.MainLift, stimulus map[training.MuscleRegion]float64) *training.Exercise {
	t.Helper()
	return mustExercise(t, training.ExerciseParams{
		ID: id, Name: id, Kind: training.KindMain,
		Stimulus: stimulus, IncrementKg: 2.5, MainLift: lift,
	})
}

func planPool(t *testing.T) []*training.Exercise {
	t.Helper()

	p := training.ExerciseParams{
		ID: "larsen", Name: "larsen", Kind: training.KindVariation,
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5, MainLift: training.LiftBench, DefaultRatioToMain: 0.9,
	}

	return []*training.Exercise{
		mainExercise(t, "bench", training.LiftBench, map[training.MuscleRegion]float64{
			training.ChestMid: 1.0, training.TricepsLateral: 0.5,
		}),
		mainExercise(t, "squat", training.LiftSquat, map[training.MuscleRegion]float64{
			training.Quad: 1.0, training.Glute: 0.5,
		}),
		mainExercise(t, "deadlift", training.LiftDeadlift, map[training.MuscleRegion]float64{
			training.Hamstring: 1.0, training.Erector: 1.0,
		}),
		mustExercise(t, p),
		mkAccessory(t, "incline", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
	}
}

func planProgram(t *testing.T) *training.Program {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := training.NewProgram(mustFrequency(t, 3), target,
		[]training.ExerciseID{"bench", "squat", "deadlift", "incline", "curl"})
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
}

// planHistory は3週分の履歴（推定1RMが立つ量）。
func planHistory(t *testing.T) []*training.SetLog {
	t.Helper()

	logs := make([]*training.SetLog, 0, 9)
	for i, daysAgo := range []int{21, 14, 7} {
		for _, spec := range []struct {
			id string
			kg float64
		}{{"bench", 85}, {"squat", 110}, {"deadlift", 140}} {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("%s-%d", spec.id, i),
				planMonday.AddDays(-daysAgo), spec.id, spec.kg, 8, 2))
		}
	}
	return logs
}

func planRequest(t *testing.T) training.PlanRequest {
	t.Helper()
	return training.PlanRequest{
		Program:    planProgram(t),
		Pool:       planPool(t),
		History:    training.NewHistory(planHistory(t)),
		Conditions: training.NewConditionLog(nil),
		Date:       planMonday,
	}
}

func mustPlan(t *testing.T, req training.PlanRequest) training.PlannedSession {
	t.Helper()
	s, err := training.DefaultSessionPlanner().Plan(req)
	if err != nil {
		t.Fatalf("Plan が失敗: %v", err)
	}
	return s
}

func TestSessionPlanner_AllMainLiftsGetASlot(t *testing.T) {
	s := mustPlan(t, planRequest(t))

	got := map[training.ExerciseID]bool{}
	for _, set := range s.Main() {
		got[set.ExerciseID()] = true
	}
	for _, want := range []training.ExerciseID{"bench", "squat", "deadlift"} {
		if !got[want] {
			t.Errorf("%s にスロットが割り当てられていない: %v", want, got)
		}
	}
	if !s.Date().Equal(planMonday) {
		t.Errorf("日付が誤り: %v", s.Date())
	}
}

// 週の1本目は標準スロット。設定より少ない回数しか通わなくても
// メインリフト本体を実施できるようにするため。
func TestSessionPlanner_FirstSessionOfWeekIsStandard(t *testing.T) {
	s := mustPlan(t, planRequest(t))
	for _, set := range s.Main() {
		role, ok := set.Role()
		if !ok || role != training.RoleStandard {
			t.Errorf("週1本目の役割が誤り: %v", role)
		}
	}
}

func TestSessionPlanner_SecondSessionOfWeekIsHeavy(t *testing.T) {
	req := planRequest(t)
	req.History = training.NewHistory(append(planHistory(t),
		mkLogOn(t, "monday", planMonday, "bench", 80, 8, 2)))
	req.Date = planMonday.AddDays(2)

	for _, set := range mustPlan(t, req).Main() {
		if role, _ := set.Role(); role != training.RoleHeavy {
			t.Errorf("週2本目の役割が誤り: %v", role)
		}
	}
}

// バリエーションのスロットでは、メイン種目が派生に差し替わること。
func TestSessionPlanner_VariationSlotSwapsTheExercise(t *testing.T) {
	req := planRequest(t)
	logs := planHistory(t)
	logs = append(logs,
		mkLogOn(t, "d1", planMonday, "bench", 80, 8, 2),
		mkLogOn(t, "d2", planMonday.AddDays(2), "bench", 90, 5, 1))
	req.History = training.NewHistory(logs)
	req.Date = planMonday.AddDays(4) // 週3本目 = バリエーション

	found := false
	for _, set := range mustPlan(t, req).Main() {
		if set.ExerciseID() == training.ExerciseID("larsen") {
			found = true
		}
		if set.ExerciseID() == training.ExerciseID("bench") {
			t.Error("バリエーションのスロットでメイン種目のままになっている")
		}
	}
	if !found {
		t.Error("バリエーションに差し替わっていない")
	}
}

func TestSessionPlanner_NoHistoryMeansNoWeight(t *testing.T) {
	req := planRequest(t)
	req.History = training.NewHistory(nil)

	for _, set := range mustPlan(t, req).Main() {
		if w, ok := set.Weight(); ok {
			t.Errorf("履歴が無いのに重量が出ている: %v %v", set.ExerciseID(), w.Kg())
		}
	}
}

func TestSessionPlanner_FillsResidualWithAccessories(t *testing.T) {
	s := mustPlan(t, planRequest(t))
	if len(s.Accessories()) == 0 {
		t.Fatal("補助種目が1つも選ばれていない")
	}

	for _, set := range s.Accessories() {
		if _, ok := set.Role(); ok {
			t.Errorf("補助種目に役割が付いている: %v", set.ExerciseID())
		}
		if set.Sets().Int() <= 0 {
			t.Errorf("補助種目のセット数が0以下: %v", set.ExerciseID())
		}
	}
}

// メインが埋めた区分は補助で狙わないこと。
func TestSessionPlanner_AccessoriesAvoidRegionsCoveredByMains(t *testing.T) {
	s := mustPlan(t, planRequest(t))

	// ベンチが大胸筋中部を埋めているので、そこを狙う補助は要らない。
	// このプールで大胸筋中部を狙う補助種目は無いが、上部と二頭は残る。
	got := map[training.ExerciseID]bool{}
	for _, set := range s.Accessories() {
		got[set.ExerciseID()] = true
	}
	if !got["incline"] && !got["curl"] {
		t.Errorf("残差を埋める補助が選ばれていない: %v", got)
	}
}

func TestSessionPlanner_SleepDeprivationRaisesTargetRIR(t *testing.T) {
	base := mustPlan(t, planRequest(t))

	req := planRequest(t)
	items := make([]training.DailyCondition, 0, 15)
	for i := range 15 {
		h := 7.5
		if i == 0 {
			h = 4.0
		}
		items = append(items, training.NewDailyCondition(planMonday.AddDays(-i)).WithSleepHours(h))
	}
	req.Conditions = training.NewConditionLog(items)
	tired := mustPlan(t, req)

	if tired.Main()[0].TargetRIR().Int() != base.Main()[0].TargetRIR().Int()+1 {
		t.Errorf("睡眠不足で目標RIRが上がっていない: %d → %d",
			base.Main()[0].TargetRIR().Int(), tired.Main()[0].TargetRIR().Int())
	}
	if len(tired.Accessories()) > 0 && len(base.Accessories()) > 0 {
		if tired.Accessories()[0].TargetRIR().Int() != base.Accessories()[0].TargetRIR().Int()+1 {
			t.Error("補助種目の目標RIRが上がっていない")
		}
	}
}

// デロードは停滞した種目にだけ適用すること。
// 伸びている種目まで一律に下げると、本人の実感と噛み合わない。
func TestSessionPlanner_DeloadAppliesOnlyToStalledLifts(t *testing.T) {
	// ベンチだけ停滞、スクワットとデッドは伸びている履歴を作る。
	logs := make([]*training.SetLog, 0, 30)
	for i := range 9 {
		day := planMonday.AddDays(-7 * (9 - i))
		logs = append(logs,
			mkLogOn(t, fmt.Sprintf("b%02d", i), day, "bench", 85, 8, 2),
			mkLogOn(t, fmt.Sprintf("s%02d", i), day, "squat", 110+float64(i)*2.5, 8, 2),
			mkLogOn(t, fmt.Sprintf("d%02d", i), day, "deadlift", 140+float64(i)*2.5, 8, 2))
	}

	conditions := make([]training.DailyCondition, 0, 28)
	for i := range 28 {
		conditions = append(conditions, training.NewDailyCondition(planMonday.AddDays(-i)).
			WithBodyWeight(75))
	}

	req := planRequest(t)
	req.History = training.NewHistory(logs)
	req.Conditions = training.NewConditionLog(conditions)

	normal := mustPlan(t, req)
	proposal, ok := normal.DeloadProposal()
	if !ok {
		t.Fatal("停滞しているのに提案が無い")
	}
	if len(proposal.StalledExercises()) != 1 || proposal.StalledExercises()[0] != "bench" {
		t.Fatalf("停滞種目が誤り: %v", proposal.StalledExercises())
	}

	req.DeloadAccepted = true
	deloaded := mustPlan(t, req)

	weightOf := func(s training.PlannedSession, id training.ExerciseID) float64 {
		t.Helper()
		for _, set := range s.Main() {
			if set.ExerciseID() == id {
				w, ok := set.Weight()
				if !ok {
					t.Fatalf("%s の重量が確定していない", id)
				}
				return w.Kg()
			}
		}
		t.Fatalf("%s が見つからない", id)
		return 0
	}

	if weightOf(deloaded, "bench") >= weightOf(normal, "bench") {
		t.Errorf("停滞した種目の重量が下がっていない: %v → %v",
			weightOf(normal, "bench"), weightOf(deloaded, "bench"))
	}
	for _, id := range []training.ExerciseID{"squat", "deadlift"} {
		if weightOf(deloaded, id) != weightOf(normal, id) {
			t.Errorf("伸びている種目 %s の重量が変わった: %v → %v",
				id, weightOf(normal, id), weightOf(deloaded, id))
		}
	}
}

func TestSessionPlanner_DeloadKeepsSetCount(t *testing.T) {
	req := planRequest(t)
	normal := mustPlan(t, req)

	req.DeloadAccepted = true
	deloaded := mustPlan(t, req)

	if normal.Main()[0].Sets().Int() != deloaded.Main()[0].Sets().Int() {
		t.Error("デロードでセット数が変わっている")
	}
}

func TestSessionPlanner_NoDeloadProposalWhenProgressing(t *testing.T) {
	if _, ok := mustPlan(t, planRequest(t)).DeloadProposal(); ok {
		t.Error("停滞していないのに提案が付いている")
	}
}

func TestSessionPlanner_RejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*training.PlanRequest)
	}{
		{"プログラムが nil", func(r *training.PlanRequest) { r.Program = nil }},
		{"対象日が無い", func(r *training.PlanRequest) { r.Date = training.Date{} }},
		{"種目プールが空", func(r *training.PlanRequest) { r.Pool = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planRequest(t)
			c.mutate(&req)
			if _, err := training.DefaultSessionPlanner().Plan(req); err == nil {
				t.Error("不正な入力が通ってしまう")
			}
		})
	}
}

// メイン種目が1つも選ばれていないプログラムはエラーにすること。
//
// 空のセッションを黙って返すと、ユーザーには中身の無いメニューが出て
// どこにもエラーが立たない。
func TestSessionPlanner_RejectsProgramWithoutMainLifts(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Biceps: 9})
	program, err := training.NewProgram(mustFrequency(t, 3), target,
		[]training.ExerciseID{"curl"})
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Program = program

	if _, err := training.DefaultSessionPlanner().Plan(req); err == nil {
		t.Error("メイン種目の無いプログラムが通ってしまう")
	}
}

func TestSessionPlanner_ZeroValueIsSafe(t *testing.T) {
	var p training.SessionPlanner
	if _, err := p.Plan(planRequest(t)); err == nil {
		t.Error("ゼロ値の生成器が通ってしまう")
	}
}

func TestNewSessionPlanner_RejectsZeroDependencies(t *testing.T) {
	slots := training.NewSlotCatalog()
	est := training.DefaultOneRepMaxEstimator()
	ratios := training.DefaultVariationRatioResolver()
	acc := training.DefaultAccessorySelector()
	deload := training.DefaultDeloadPolicy()

	cases := []struct {
		name string
		call func() error
	}{
		{"推定器", func() error {
			_, err := training.NewSessionPlanner(slots, training.OneRepMaxEstimator{}, ratios, acc, deload)
			return err
		}},
		{"係数の解決器", func() error {
			_, err := training.NewSessionPlanner(slots, est, training.VariationRatioResolver{}, acc, deload)
			return err
		}},
		{"補助の選択器", func() error {
			_, err := training.NewSessionPlanner(slots, est, ratios, training.AccessorySelector{}, deload)
			return err
		}},
		{"デロードのポリシー", func() error {
			_, err := training.NewSessionPlanner(slots, est, ratios, acc, training.DeloadPolicy{})
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.call(); err == nil {
				t.Error("ゼロ値の依存が通ってしまう")
			}
		})
	}

	if _, err := training.NewSessionPlanner(slots, est, ratios, acc, deload); err != nil {
		t.Errorf("正常な依存が弾かれた: %v", err)
	}
}

func TestSessionPlanner_ReturnValuesAreDefensivelyCopied(t *testing.T) {
	s := mustPlan(t, planRequest(t))

	main := s.Main()
	main[0] = training.PlannedSet{}
	if s.Main()[0].IsZero() {
		t.Error("Main の書き換えが波及している")
	}

	accessories := s.Accessories()
	if len(accessories) > 0 {
		accessories[0] = training.PlannedSet{}
		if s.Accessories()[0].IsZero() {
			t.Error("Accessories の書き換えが波及している")
		}
	}
}

func TestSessionPlanner_IsDeterministic(t *testing.T) {
	first := mustPlan(t, planRequest(t))

	for range 30 {
		got := mustPlan(t, planRequest(t))
		if len(got.Main()) != len(first.Main()) || len(got.Accessories()) != len(first.Accessories()) {
			t.Fatal("実行のたびに構成が変わる")
		}
		for i := range got.Main() {
			if got.Main()[i] != first.Main()[i] {
				t.Fatalf("メインが変わる: %v vs %v", first.Main()[i], got.Main()[i])
			}
		}
		for i := range got.Accessories() {
			if got.Accessories()[i] != first.Accessories()[i] {
				t.Fatalf("補助が変わる: %v vs %v", first.Accessories()[i], got.Accessories()[i])
			}
		}
	}
}

// その週にすでに埋めた分が残差から引かれること。
//
// 週目標を頻度で割った固定値を毎回使うと繰り越しが起きず、
// 週の前半に多くこなしても後半の量が変わらない。
func TestSessionPlanner_ResidualCarriesOverWithinTheWeek(t *testing.T) {
	req := planRequest(t)
	first := mustPlan(t, req)

	firstIDs := map[training.ExerciseID]bool{}
	for _, set := range first.Accessories() {
		firstIDs[set.ExerciseID()] = true
	}
	if !firstIDs["incline"] || !firstIDs["curl"] {
		t.Fatalf("前提が崩れている（1本目の補助）: %v", firstIDs)
	}

	// 月曜に大胸筋上部の週目標（9セット）を全部こなしたことにする。
	logs := planHistory(t)
	for i := range 9 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("inc-%d", i),
			planMonday, "incline", 30, 10, 2))
	}
	req.History = training.NewHistory(logs)
	req.Date = planMonday.AddDays(2)

	second := mustPlan(t, req)
	for _, set := range second.Accessories() {
		if set.ExerciseID() == "incline" {
			t.Errorf("週目標を満たした区分がまだ狙われている: %v", set.ExerciseID())
		}
	}

	// 二頭はまだ残っているので選ばれる。
	found := false
	for _, set := range second.Accessories() {
		if set.ExerciseID() == "curl" {
			found = true
		}
	}
	if !found {
		t.Error("残っている区分が狙われていない")
	}
}

// 週内カバレッジに当日の記録を二重計上しないこと。
//
// セッション中に記録してから計画を開き直したとき、当日のメイン種目が
// 履歴と計画の両方で数えられると、残差が実際より小さくなる。
func TestSessionPlanner_DoesNotDoubleCountTodaysLogs(t *testing.T) {
	req := planRequest(t)
	base := mustPlan(t, req)

	// 当日のメイン種目を記録してから開き直す。
	logs := planHistory(t)
	for _, set := range base.Main() {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("today-%s", set.ExerciseID()),
			planMonday, string(set.ExerciseID()), 80, 8, 2))
	}
	req.History = training.NewHistory(logs)

	reopened := mustPlan(t, req)
	if len(reopened.Accessories()) != len(base.Accessories()) {
		t.Errorf("当日の記録で補助の数が変わった: %d → %d",
			len(base.Accessories()), len(reopened.Accessories()))
	}
}

// メインが埋めた刺激を残差から差し引くこと。
//
// 差し引かないと、メインで十分に刺激した区分を補助でもう一度狙い、
// 週目標を大きく超過する。
func TestSessionPlanner_SubtractsMainCoverageFromResidual(t *testing.T) {
	// 大胸筋中部を狙う補助種目をプールに足す。
	pool := append(planPool(t),
		mkAccessory(t, "pec_fly", map[training.MuscleRegion]float64{training.ChestMid: 1.0}))

	// 週目標4・頻度3。ベンチが1セッションで4セット埋めるので、
	// メインの刺激を差し引けば残差は0になる。
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 4})
	program, err := training.NewProgram(mustFrequency(t, 3), target,
		[]training.ExerciseID{"bench", "squat", "deadlift", "pec_fly"})
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Pool, req.Program = pool, program

	got := mustPlan(t, req)
	for _, set := range got.Accessories() {
		if set.ExerciseID() == "pec_fly" {
			t.Errorf("メインが埋めた区分を補助でも狙っている: %v", got.Accessories())
		}
	}
}

// 残りセッション数で割ること。週の後半ほど1回あたりの量が増える。
func TestSessionPlanner_DividesByRemainingSessions(t *testing.T) {
	// 大胸筋上部を狙う補助を十分に用意する。種目が足りないと
	// スロット数が頭打ちになり、割り算の違いが見えない。
	pool := planPool(t)
	ids := []training.ExerciseID{"bench", "squat", "deadlift", "incline"}
	for i := range 5 {
		id := fmt.Sprintf("chest_up_%d", i)
		pool = append(pool, mkAccessory(t, id,
			map[training.MuscleRegion]float64{training.ChestUpper: 1.0}))
		ids = append(ids, training.ExerciseID(id))
	}

	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestUpper: 12})
	program, err := training.NewProgram(mustFrequency(t, 3), target, ids)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Pool, req.Program = pool, program

	// 週の1本目: 12 / 3 = 4セット → 2種目（3セットずつ）
	first := mustPlan(t, req)
	if len(first.Accessories()) != 2 {
		t.Fatalf("1本目の補助数が誤り: %d (%v)", len(first.Accessories()), first.Accessories())
	}

	// 週の3本目に何もこなしていない状態: 12 / 1 = 12セット → 上限まで
	logs := planHistory(t)
	logs = append(logs,
		mkLogOn(t, "w1", planMonday, "squat", 110, 8, 2),
		mkLogOn(t, "w2", planMonday.AddDays(2), "squat", 110, 8, 2))
	req.History = training.NewHistory(logs)
	req.Date = planMonday.AddDays(4)

	last := mustPlan(t, req)
	if len(last.Accessories()) <= len(first.Accessories()) {
		t.Errorf("残りセッション数で割っていない: 1本目 %d → 3本目 %d",
			len(first.Accessories()), len(last.Accessories()))
	}
}

// バリエーションの重量はメインの推定1RMから導くこと。
//
// バリエーション自身の1RMを使うと、履歴の少ない種目で数字が暴れるうえ、
// 履歴が無い間は重量が出ない。
func TestSessionPlanner_VariationWeightComesFromTheMainLift(t *testing.T) {
	req := planRequest(t)
	logs := planHistory(t)
	logs = append(logs,
		mkLogOn(t, "d1", planMonday, "bench", 80, 8, 2),
		mkLogOn(t, "d2", planMonday.AddDays(2), "bench", 90, 5, 1))
	req.History = training.NewHistory(logs)
	req.Date = planMonday.AddDays(4)

	for _, set := range mustPlan(t, req).Main() {
		if set.ExerciseID() != "larsen" {
			continue
		}
		w, ok := set.Weight()
		if !ok {
			t.Fatal("バリエーションの重量が確定していない（自身の履歴が無くても出るべき）")
		}
		if w.Kg() <= 0 {
			t.Errorf("バリエーションの重量が0以下: %v", w.Kg())
		}
	}
}

// 週内カバレッジは週初から当日の前日まで。
//
// 前の週の記録まで数えると残差が過小になり、当日の記録まで数えると
// 計画中のメインと二重に数える。
func TestSessionPlanner_WeeklyCoverageWindow(t *testing.T) {
	pool := planPool(t)
	ids := []training.ExerciseID{"bench", "squat", "deadlift", "incline"}
	for i := range 5 {
		id := fmt.Sprintf("chest_up_%d", i)
		pool = append(pool, mkAccessory(t, id,
			map[training.MuscleRegion]float64{training.ChestUpper: 1.0}))
		ids = append(ids, training.ExerciseID(id))
	}
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestUpper: 12})
	program, err := training.NewProgram(mustFrequency(t, 3), target, ids)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	base := training.PlanRequest{
		Program: program, Pool: pool,
		History:    training.NewHistory(planHistory(t)),
		Conditions: training.NewConditionLog(nil),
		Date:       planMonday,
	}
	want := len(mustPlan(t, base).Accessories())

	t.Run("前の週の記録は数えない", func(t *testing.T) {
		logs := planHistory(t)
		for i := range 12 {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("prev-%d", i),
				planMonday.AddDays(-3), "incline", 30, 10, 2))
		}
		req := base
		req.History = training.NewHistory(logs)

		if got := len(mustPlan(t, req).Accessories()); got != want {
			t.Errorf("前の週の記録が残差に影響している: %d → %d", want, got)
		}
	})

	t.Run("当日の記録は数えない", func(t *testing.T) {
		logs := planHistory(t)
		for i := range 12 {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("today-%d", i),
				planMonday, "incline", 30, 10, 2))
		}
		req := base
		req.History = training.NewHistory(logs)

		if got := len(mustPlan(t, req).Accessories()); got != want {
			t.Errorf("当日の記録が残差に影響している: %d → %d", want, got)
		}
	})
}

// 補助種目の重量も基準日を見て推定すること。
func TestSessionPlanner_AccessoryWeightRespectsTheDate(t *testing.T) {
	req := planRequest(t)

	logs := planHistory(t)
	for i, daysAgo := range []int{21, 14, 7} {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("inc-%d", i),
			planMonday.AddDays(-daysAgo), "incline", 30, 10, 2))
	}
	req.History = training.NewHistory(logs)

	found := false
	for _, set := range mustPlan(t, req).Accessories() {
		if set.ExerciseID() != "incline" {
			continue
		}
		found = true
		if _, ok := set.Weight(); !ok {
			t.Error("履歴があるのに補助の重量が確定していない")
		}
	}
	if !found {
		t.Fatal("インクラインが選ばれていない")
	}

	// ブランク明け（基準日から十分離れた履歴）では重量を出さない。
	req.Date = planMonday.AddDays(90)
	for _, set := range mustPlan(t, req).Accessories() {
		if set.ExerciseID() != "incline" {
			continue
		}
		if w, ok := set.Weight(); ok {
			t.Errorf("古い履歴から補助の重量が出ている: %v", w.Kg())
		}
	}
}

func TestSessionPlanner_SkipsNilExercisesInPool(t *testing.T) {
	req := planRequest(t)
	req.Pool = append([]*training.Exercise{nil}, append(planPool(t), nil)...)

	s := mustPlan(t, req)
	if len(s.Main()) != 3 {
		t.Errorf("nil が混ざるとメインが揃わない: %d", len(s.Main()))
	}
}
