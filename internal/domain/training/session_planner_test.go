package training_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

var planMonday = training.MustDate(2026, time.August, 17) // 月曜

func mainExercise(t *testing.T, id string, stimulus map[training.MuscleRegion]float64) *training.Exercise {
	t.Helper()
	return mustExercise(t, training.ExerciseParams{
		ID: id, Name: id,
		Stimulus: stimulus, IncrementKg: 2.5,
	})
}

func planPool(t *testing.T) []*training.Exercise {
	t.Helper()

	// かつてベンチのバリエーションだった種目。いまは補助のひとつ。
	p := training.ExerciseParams{
		ID: "larsen", Name: "larsen",
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5,
	}

	return []*training.Exercise{
		mainExercise(t, "bench", map[training.MuscleRegion]float64{
			training.ChestMid: 1.0, training.TricepsLateral: 0.5,
		}),
		mainExercise(t, "squat", map[training.MuscleRegion]float64{
			training.Quad: 1.0, training.Glute: 0.5,
		}),
		mainExercise(t, "deadlift", map[training.MuscleRegion]float64{
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
		[]training.ExerciseID{"bench", "squat", "deadlift", "incline", "curl"}, big3())
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

// planRequestAt は週内で days 日目のリクエストを返す。
//
// done に挙げた日には「その日に通った」ことを表す記録を入れる。週の何本目かは
// 履歴に記録のある日数で決まるので（sessionIndexInWeek）、ここを進めないと
// 日付だけ動かしても常に1本目になる。
//
// 記録に curl を使うのは、メイン種目の推定1RMを動かさないため。bench で
// 進めると、役割を見たいだけのケースで重量まで変わる。
func planRequestAt(t *testing.T, days int, done ...int) training.PlanRequest {
	t.Helper()

	logs := planHistory(t)
	for i, d := range done {
		logs = append(logs,
			mkLogOn(t, fmt.Sprintf("done-%d", i), planMonday.AddDays(d), "curl", 20, 10, 2))
	}

	req := planRequest(t)
	req.History = training.NewHistory(logs)
	req.Date = planMonday.AddDays(days)
	return req
}

// 週の何本目かでスロットの役割が決まる。
//
// 並び順は「重要な役割ほど先」で、強度の昇順ではない（slot.go）。設定した
// 頻度より実際に通う回数が少ないと先頭のスロットしか使われないので、標準を
// 先頭に置くことで、週に一度でも通えば通常の強度で実施することが保証される。
func TestSessionPlanner_SlotRoleFollowsTheSessionIndex(t *testing.T) {
	cases := []struct {
		name string
		done []int // 週内で既に通った日（月曜からの日数）
		date int   // 対象日（月曜からの日数）
		want training.SlotRole
	}{
		{
			// 1本目を軽い日にすると、通常フォームの高い強度がいつまでも
			// 記録されず、推定1RMが実力より低いまま固定される。
			name: "週1本目は標準スロット",
			done: nil, date: 0, want: training.RoleStandard,
		},
		{
			name: "週2本目は高強度スロット",
			done: []int{0}, date: 2, want: training.RoleHeavy,
		},
		{
			name: "週3本目は軽い日",
			done: []int{0, 2}, date: 4, want: training.RoleLight,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			main := mustPlan(t, planRequestAt(t, c.date, c.done...)).Main()
			if len(main) == 0 {
				t.Fatal("メイン種目が1つも出ていない")
			}
			for _, set := range main {
				role, ok := set.Role()
				if !ok || role != c.want {
					t.Errorf("%s の役割が %v。%v のはず", set.ExerciseID(), role, c.want)
				}
			}
		})
	}
}

// 軽い日でも種目は差し替えない。強度とセット数だけが変わる。
//
// 以前は「BENCH のバリエーション」から1つ選んでベンチと入れ替えていた。
// やめたのは、差し替えの対応表（MainLift）を維持する理由が他に無くなったため
// （D-114）。軽い日にやるのは、その日の軸そのものを軽くやること。
//
// larsen はかつてベンチのバリエーションだった種目で、いまは補助のひとつ。
// メインの枠に現れたら、差し替えが復活している。
func TestSessionPlanner_LightSlotKeepsTheSameExercise(t *testing.T) {
	s := mustPlan(t, planRequestAt(t, 4, 0, 2)) // 週3本目 = 軽い日

	found := false
	for _, set := range s.Main() {
		if set.ExerciseID() == training.ExerciseID("larsen") {
			t.Error("軽い日で種目が差し替わっている")
		}
		if set.ExerciseID() == training.ExerciseID("bench") {
			found = true
		}
	}
	if !found {
		t.Error("軸のベンチが出ていない")
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

// 残差は補助種目で埋める。
func TestSessionPlanner_FillsResidualWithAccessories(t *testing.T) {
	s := mustPlan(t, planRequest(t))
	if len(s.Accessories()) == 0 {
		t.Fatal("補助種目が1つも選ばれていない")
	}

	for _, set := range s.Accessories() {
		// 役割はメインのスロットにだけ付く。補助に付くと、強度帯が
		// 二重に適用される。
		if _, ok := set.Role(); ok {
			t.Errorf("補助種目に役割が付いている: %v", set.ExerciseID())
		}
		if set.Sets().Int() <= 0 {
			t.Errorf("補助種目のセット数が0以下: %v", set.ExerciseID())
		}
	}

	// ベンチが大胸筋中部を埋めているので、そこを狙う補助は要らない。
	// このプールで大胸筋中部を狙う補助種目は無いが、上部と二頭は残る。
	ids := accessoryIDs(s)
	if !slices.Contains(ids, "incline") && !slices.Contains(ids, "curl") {
		t.Errorf("残差を埋める補助が選ばれていない: %v", ids)
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

// stalledBenchRequest はベンチだけが停滞し、スクワットとデッドリフトは
// 伸びている履歴のリクエストを返す。
//
// 停滞の判定には体重の記録が要る（減量中かどうかで扱いが変わる）ので、
// 4週ぶんの体重も入れる。
func stalledBenchRequest(t *testing.T) training.PlanRequest {
	t.Helper()

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
	return req
}

// デロードは、承認された種目にだけ効く。
//
// 提案と承認を別々に扱うのは、提案が毎回計算し直されるため。体重の記録が
// 数日途切れただけで提案は消えるので、承認を提案に紐づけると「承認したのに
// 重量が下がらない」という説明のつかない挙動になる。
//
// 承認の粒度を種目にしているのも同じ理由。単一の bool だと、ベンチの提案を
// 承認した状態のまま後からスクワットが停滞判定に入ったとき、新しい承認を
// 経ずにスクワットまで下がる。
func TestSessionPlanner_DeloadAppliesOnlyToAcceptedLifts(t *testing.T) {
	cases := []struct {
		name string
		// ベンチだけが停滞した履歴を使うか。false なら伸びている履歴。
		stalled  bool
		accepted []training.ExerciseID
		// 提案に載るべき種目。nil なら提案そのものが出ない。
		wantProposal []training.ExerciseID
		// 承認の結果、重量が下がる種目と、変わらない種目。
		wantLowered   []training.ExerciseID
		wantUnchanged []training.ExerciseID
	}{
		{
			name:          "停滞していなければ提案は出ない",
			stalled:       false,
			wantProposal:  nil,
			wantUnchanged: []training.ExerciseID{"bench", "squat", "deadlift"},
		},
		{
			// 伸びている種目まで一律に下げると、本人の実感と噛み合わない。
			name:          "停滞した種目だけが提案に載り、承認するとそれだけ下がる",
			stalled:       true,
			accepted:      []training.ExerciseID{"bench"},
			wantProposal:  []training.ExerciseID{"bench"},
			wantLowered:   []training.ExerciseID{"bench"},
			wantUnchanged: []training.ExerciseID{"squat", "deadlift"},
		},
		{
			// 提案の有無と承認は独立に効く。ここが紐づいていると、体重の
			// 記録が途切れた日に「承認したのに下がらない」が起きる。
			name:          "提案が出ていなくても、承認された種目は下がる",
			stalled:       false,
			accepted:      []training.ExerciseID{"bench"},
			wantProposal:  nil,
			wantLowered:   []training.ExerciseID{"bench"},
			wantUnchanged: []training.ExerciseID{"squat", "deadlift"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planRequest(t)
			if c.stalled {
				req = stalledBenchRequest(t)
			}

			normal := mustPlan(t, req)

			proposal, ok := normal.DeloadProposal()
			if len(c.wantProposal) == 0 {
				if ok {
					t.Errorf("提案が出ている: %v", proposal.StalledExercises())
				}
			} else {
				if !ok {
					t.Fatal("停滞しているのに提案が無い")
				}
				if !slices.Equal(proposal.StalledExercises(), c.wantProposal) {
					t.Errorf("停滞種目が %v。%v のはず",
						proposal.StalledExercises(), c.wantProposal)
				}
			}

			if len(c.accepted) == 0 {
				return
			}

			req.DeloadAccepted = c.accepted
			deloaded := mustPlan(t, req)

			for _, id := range c.wantLowered {
				if mainWeight(t, deloaded, id) >= mainWeight(t, normal, id) {
					t.Errorf("承認した %s の重量が下がっていない: %v → %v",
						id, mainWeight(t, normal, id), mainWeight(t, deloaded, id))
				}
			}
			for _, id := range c.wantUnchanged {
				if mainWeight(t, deloaded, id) != mainWeight(t, normal, id) {
					t.Errorf("承認していない %s の重量が変わった: %v → %v",
						id, mainWeight(t, normal, id), mainWeight(t, deloaded, id))
				}
			}

			// デロードで落とすのは強度であって量ではない。セット数まで
			// 減らすと、週目標の消化が止まって残差が埋まらなくなる。
			if got, want := mainSet(t, deloaded, "bench").Sets().Int(),
				mainSet(t, normal, "bench").Sets().Int(); got != want {
				t.Errorf("デロードでセット数が %d に変わった。%d のはず", got, want)
			}
		})
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

// 宣言されていれば、どの種目でも軸になれること。
//
// 元は「メイン種目（Kind == MAIN）が選択に無ければエラー」を検査していた。
// 軸が BIG3 に固定されていたので、ルーマニアンデッドリフトをハムの種目
// として使う人も、背中の軸に懸垂を使う人も表現できず、2回続けて同じ穴を
// 踏んだ（D-113・D-117）。
//
// 「宣言ゼロ」の検出は NewProgram へ移った。ここが見るのは、BIG3 でない
// 種目だけのプログラムでもセッションが組めることと、その種目が軸として
// 先頭に出ること。
func TestSessionPlanner_AnyDeclaredExerciseCanBeTheAxis(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Biceps: 9})
	program, err := training.NewProgram(mustFrequency(t, 3), target,
		[]training.ExerciseID{"curl"}, []training.ExerciseID{"curl"})
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Program = program

	session, err := training.DefaultSessionPlanner().Plan(req)
	if err != nil {
		t.Fatalf("宣言した種目だけのプログラムが通らない: %v", err)
	}

	main := session.Main()
	if len(main) != 1 {
		t.Fatalf("軸の数が誤り: %d（期待 1）", len(main))
	}
	if main[0].ExerciseID() != "curl" {
		t.Errorf("宣言した種目が軸になっていない: %s", main[0].ExerciseID())
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
	acc := training.DefaultAccessorySelector()
	deload := training.DefaultDeloadPolicy()

	cases := []struct {
		name string
		call func() error
	}{
		{"推定器", func() error {
			_, err := training.NewSessionPlanner(slots, training.OneRepMaxEstimator{}, acc, deload)
			return err
		}},
		{"補助の選択器", func() error {
			_, err := training.NewSessionPlanner(slots, est, training.AccessorySelector{}, deload)
			return err
		}},
		{"デロードのポリシー", func() error {
			_, err := training.NewSessionPlanner(slots, est, acc, training.DeloadPolicy{})
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

	if _, err := training.NewSessionPlanner(slots, est, acc, deload); err != nil {
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
// その日の計画は、その日の始まりに確定する。
//
// セッション中に記録を足しながら開き直しても、種目の並びと数が変わらない
// こと。これが今回の受け入れ条件で、ここが守られていれば「終えた種目が
// 消える」「並びが入れ替わる」「枠が補充されて終わらない」は原理的に
// 起きなくなる（D-021・D-087・D-088 はすべてこの1点の派生だった）。
func TestSessionPlanner_PlanIsFixedForTheWholeDay(t *testing.T) {
	req := planRequest(t)
	base := planHistory(t)
	req.History = training.NewHistory(base)

	first := mustPlan(t, req)
	want := lineup(first)
	if len(want) == 0 {
		t.Fatal("前提: 種目が1つも出ていない")
	}

	// 提示されたとおりに1セットずつ記録しては、開き直す。
	logs := append([]*training.SetLog{}, base...)
	n := 0
	for _, set := range append(first.Main(), first.Accessories()...) {
		for range set.Sets().Int() {
			n++
			logs = append(logs, mkLogOn(t, fmt.Sprintf("d%03d", n), req.Date,
				string(set.ExerciseID()), 40, 8, 2))

			req.History = training.NewHistory(logs)
			got := lineup(mustPlan(t, req))
			if !slices.Equal(got, want) {
				t.Fatalf("%dセット記録した時点で計画が変わった\n  最初: %v\n  いま: %v",
					n, want, got)
			}
		}
	}
}

// lineup は提示された種目を並び順のまま返す。
func lineup(s training.PlannedSession) []training.ExerciseID {
	out := make([]training.ExerciseID, 0, len(s.Main())+len(s.Accessories()))
	for _, set := range append(s.Main(), s.Accessories()...) {
		out = append(out, set.ExerciseID())
	}
	return out
}

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
		[]training.ExerciseID{"bench", "squat", "deadlift", "pec_fly"}, big3())
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

// chestUpperRequest は大胸筋上部だけを週目標に持つリクエストを返す。
//
// 同じ区分を狙う補助を6種目そろえるのは、種目が足りないとスロット数が
// 頭打ちになり、残差の計算の違いが出力に現れないため。
func chestUpperRequest(t *testing.T) training.PlanRequest {
	t.Helper()

	pool := planPool(t)
	ids := []training.ExerciseID{"bench", "squat", "deadlift", "incline"}
	for i := range 5 {
		id := fmt.Sprintf("chest_up_%d", i)
		pool = append(pool, mkAccessory(t, id,
			map[training.MuscleRegion]float64{training.ChestUpper: 1.0}))
		ids = append(ids, training.ExerciseID(id))
	}

	program, err := training.NewProgram(mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.ChestUpper: 12}),
		ids, big3())
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Pool, req.Program = pool, program
	return req
}

// 残りセッション数で割ること。週の後半ほど1回あたりの量が増える。
func TestSessionPlanner_DividesByRemainingSessions(t *testing.T) {
	req := chestUpperRequest(t)

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

func TestSessionPlanner_VariationWeightComesFromItsOwnRecord(t *testing.T) {
	req := planRequest(t)

	logs := []*training.SetLog{
		mkLogOn(t, "v1", planMonday.AddDays(-7), "larsen", 80, 5, 1),
		mkLogOn(t, "d1", planMonday, "squat", 80, 8, 2),
		mkLogOn(t, "d2", planMonday.AddDays(2), "deadlift", 90, 5, 1),
	}
	req.History = training.NewHistory(logs)
	req.Date = planMonday.AddDays(4)

	for _, set := range mustPlan(t, req).Main() {
		if set.ExerciseID() != "larsen" {
			continue
		}
		w, ok := set.Weight()
		if !ok {
			t.Fatal("バリエーションの重量が確定していない")
		}
		if w.Kg() <= 0 {
			t.Errorf("バリエーションの重量が０以下: %v", w.Kg())
		}
	}

}

// 週内カバレッジは週初から当日の前日まで。
//
// 前の週の記録まで数えると残差が過小になり、当日の記録まで数えると
// 計画中のメインと二重に数える。
func TestSessionPlanner_WeeklyCoverageWindow(t *testing.T) {
	base := chestUpperRequest(t)
	want := len(mustPlan(t, base).Accessories())

	cases := []struct {
		name string
		// 12セットぶんの記録を置く日（月曜からの日数）。窓に入っていれば
		// 週目標12を使い切り、補助が減るはず。
		daysFromMonday int
	}{
		{
			// 前の週まで数えると残差が過小になり、週の頭から補助が減る。
			name: "前の週の記録は数えない", daysFromMonday: -3,
		},
		{
			// 以前は当日の記録も残差に含めていた。含めるとセッション中に
			// 残差が動き、こなすたびにリストが入れ替わる。今日の計画は
			// その日の始まりに確定させると決めた（D-116）。
			name: "当日の記録は数えない", daysFromMonday: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := planHistory(t)
			for i := range 12 {
				logs = append(logs, mkLogOn(t, fmt.Sprintf("out-%d", i),
					planMonday.AddDays(c.daysFromMonday), "incline", 30, 10, 2))
			}

			req := base
			req.History = training.NewHistory(logs)

			if got := len(mustPlan(t, req).Accessories()); got != want {
				t.Errorf("窓の外の記録が残差に影響している: 補助が %d 件。%d 件のはず",
					got, want)
			}
		})
	}
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

// --- 敵対的検証（PR16）で見つかった穴を塞ぐテスト ---

func accessoryIDs(s training.PlannedSession) []training.ExerciseID {
	out := make([]training.ExerciseID, 0, len(s.Accessories()))
	for _, a := range s.Accessories() {
		out = append(out, a.ExerciseID())
	}
	return out
}

func mainSet(t *testing.T, s training.PlannedSession, id training.ExerciseID) training.PlannedSet {
	t.Helper()
	for _, set := range s.Main() {
		if set.ExerciseID() == id {
			return set
		}
	}
	t.Fatalf("%s がメインに無い: %v", id, s.Main())
	return training.PlannedSet{}
}

func mainWeight(t *testing.T, s training.PlannedSession, id training.ExerciseID) float64 {
	t.Helper()
	w, ok := mainSet(t, s, id).Weight()
	if !ok {
		t.Fatalf("%s の重量が確定していない", id)
	}
	return w.Kg()
}

// 当日の記録は、補助のリストを一切動かさない。
//
// 内容も並びも、その日が始まった時点のまま。ジムで消化している最中に
// リストが自分の下で動くと、どこまでやったか分からなくなる。実ブラウザで
// 踏んだ。
//
// 以前は3セット終えると候補から外していた。外すと枠が空いて新しい種目が
// 補充され、種目マスタが尽きるまでセッションが終わらなかった。それを
// 抑えるために枠の再計算（fitAccessories）が要り、さらに並びが動くので
// IDの昇順で固定する、と手当てが積み上がっていた。
//
// 当日を見なければ、どれも起きない。終えた種目は緑のまま残るだけで、
// 終わりを判断するのは本人（D-116）。
func TestSessionPlanner_TodaysLogsDoNotMoveTheAccessoryList(t *testing.T) {
	cases := []struct {
		name string
		// 記録する種目の位置。負なら末尾から数える。
		index int
		// 記録するセット数。予定のセット数を受け取って決める。
		sets func(planned int) int
	}{
		{
			// 消えると残りのセットが記録できない。実運用では必ず踏む。
			name:  "先頭を1セットだけこなしても消えない",
			index: 0, sets: func(int) int { return 1 },
		},
		{
			name:  "先頭を予定の1つ手前までこなしても消えない",
			index: 0, sets: func(planned int) int { return planned - 1 },
		},
		{
			// ここが「こなしたら外す」との分かれ目。外すと枠が空いて
			// 新しい種目が補充され、セッションが終わらなくなる。
			name:  "先頭を予定ぶん全部こなしても消えない",
			index: 0, sets: func(planned int) int { return planned },
		},
		{
			// 枠の調整は「始めたものを残す → 残りを埋める」という順で
			// 組んでいたので、そのまま返すと着手済みが先頭に寄っていた。
			// 1セット記録しただけでカードが飛ぶことになる。
			name:  "並びの最後をこなしても、それが先頭に飛ばない",
			index: -1, sets: func(int) int { return 1 },
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planRequest(t)
			before := mustPlan(t, req)
			if len(before.Accessories()) < 2 {
				t.Fatalf("前提: 補助が2件以上出ること（いま %d 件）",
					len(before.Accessories()))
			}

			i := c.index
			if i < 0 {
				i += len(before.Accessories())
			}
			target := before.Accessories()[i]

			logs := planHistory(t)
			for n := range c.sets(target.Sets().Int()) {
				logs = append(logs, mkLogOn(t, fmt.Sprintf("today-%d", n),
					planMonday, string(target.ExerciseID()), 30, 10, 2))
			}
			req.History = training.NewHistory(logs)

			after := mustPlan(t, req)
			if !slices.Equal(accessoryIDs(after), accessoryIDs(before)) {
				t.Errorf("当日の記録でリストが変わった:\n  前: %v\n  後: %v",
					accessoryIDs(before), accessoryIDs(after))
			}
		})
	}
}

// 設定より多く通っても補助は出る。出さないとメインのフルスロットだけが積まれ、
// 週あたりのセット数が想定を突き抜ける。
func TestSessionPlanner_ExtraSessionsStillGetAccessories(t *testing.T) {
	req := planRequest(t)
	logs := planHistory(t)
	for i := range 4 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("extra-%d", i),
			planMonday.AddDays(i), "bench", 85, 8, 2))
	}
	req.History = training.NewHistory(logs)
	req.Date = planMonday.AddDays(4) // 週3設定の5本目

	s := mustPlan(t, req)
	if len(s.Accessories()) == 0 {
		t.Error("頻度を超えたセッションで補助が1つも出ていない")
	}
}

// RIR 補正は注入したコンディション分析器を使う。
// 既定値を直接呼ぶと、同じセッションでデロード判定と RIR 補正の
// 前提が食い違う。
func TestSessionPlanner_UsesInjectedConditionAnalyzer(t *testing.T) {
	analyzer, err := training.NewConditionAnalyzer(14, 0.05, 21)
	if err != nil {
		t.Fatalf("分析器の生成に失敗: %v", err)
	}
	policy, err := training.NewDeloadPolicy(analyzer, 8, 0.10)
	if err != nil {
		t.Fatalf("ポリシーの生成に失敗: %v", err)
	}
	planner, err := training.NewSessionPlanner(training.NewSlotCatalog(),
		training.DefaultOneRepMaxEstimator(),
		training.DefaultAccessorySelector(), policy)
	if err != nil {
		t.Fatalf("生成器の生成に失敗: %v", err)
	}

	conditions := []training.DailyCondition{
		training.NewDailyCondition(planMonday).WithSleepHours(6.9),
	}
	for i := 1; i <= 14; i++ {
		conditions = append(conditions,
			training.NewDailyCondition(planMonday.AddDays(-i)).WithSleepHours(7))
	}

	req := planRequest(t)
	req.Conditions = training.NewConditionLog(conditions)

	injected, err := planner.Plan(req)
	if err != nil {
		t.Fatalf("Plan が失敗: %v", err)
	}
	if got := mainSet(t, injected, "bench").TargetRIR().Int(); got != 3 {
		t.Errorf("注入した分析器の閾値が効いていない: 目標RIR %d", got)
	}

	// 既定の閾値（1.5h）なら 0.1h の不足では補正しない。
	base := mustPlan(t, req)
	if got := mainSet(t, base, "bench").TargetRIR().Int(); got != 2 {
		t.Errorf("既定の分析器で補正が入った: 目標RIR %d", got)
	}
}

// スロットの役割ごとに強度が変わる。ここが効かないと
// HEAVY もバリエーション日も標準日と同じ重量になる。
func TestSessionPlanner_SlotRoleChangesIntensity(t *testing.T) {
	standard := mustPlan(t, planRequest(t))

	req := planRequest(t)
	// メインの推定1RMを動かさないよう、補助種目で週内の本数だけ進める。
	req.History = training.NewHistory(append(planHistory(t),
		mkLogOn(t, "warm", planMonday, "curl", 20, 10, 2)))
	req.Date = planMonday.AddDays(1)

	heavy := mustPlan(t, req)
	if role, _ := mainSet(t, heavy, "bench").Role(); role != training.RoleHeavy {
		t.Fatalf("前提: 2本目が高強度スロットであること: %v", role)
	}
	if mainWeight(t, heavy, "bench") <= mainWeight(t, standard, "bench") {
		t.Errorf("高強度スロットの重量が標準スロット以下: %v → %v",
			mainWeight(t, standard, "bench"), mainWeight(t, heavy, "bench"))
	}
}

// 当日すでに記録したメイン種目を、計画ぶんと二重に数えない。
// 二重に数えると、そのセッションの途中から補助が消える。
func TestSessionPlanner_DoesNotDoubleCountTodaysMain(t *testing.T) {
	pool := []*training.Exercise{
		mainExercise(t, "bench",
			map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mkAccessory(t, "fly", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mkAccessory(t, "press", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
	}
	program, err := training.NewProgram(mustFrequency(t, 1),
		mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 8}),
		[]training.ExerciseID{"bench", "fly", "press"}, []training.ExerciseID{"bench"})
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	logs := []*training.SetLog{
		mkLogOn(t, "b0", planMonday.AddDays(-7), "bench", 85, 8, 2),
	}
	for i := range 4 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("bt-%d", i),
			planMonday, "bench", 85, 8, 2))
	}

	s := mustPlan(t, training.PlanRequest{
		Program: program, Pool: pool,
		History:    training.NewHistory(logs),
		Conditions: training.NewConditionLog(nil),
		Date:       planMonday,
	})
	if len(s.Accessories()) == 0 {
		t.Errorf("当日のメインを二重計上して補助が消えた: 週目標8、実施4セット")
	}
}

// plannedWeight は今日のメニューのうち id の提示重量を返す。
// メインと補助のどちらにあっても引ける。
func plannedWeight(t *testing.T, s training.PlannedSession, id training.ExerciseID) (float64, bool) {
	t.Helper()
	for _, set := range append(s.Main(), s.Accessories()...) {
		if set.ExerciseID() != id {
			continue
		}
		w, ok := set.Weight()
		return w.Kg(), ok
	}
	t.Fatalf("%s が今日のメニューに無い", id)
	return 0, false
}

// 当日の記録で、その日の提示重量が動かないこと。
//
// 動くと、1セット目を記録した瞬間に2セット目の提示が変わる。しかも
// RIR を守ってきついセットをこなすほど推定1RMが上がるので、
// **追い込むほど次のセットが重くなる**。実際に画面で踏んだ。
func TestSessionPlanner_TodaysLogsDoNotMoveTodaysWeight(t *testing.T) {
	cases := []struct {
		name     string
		exercise training.ExerciseID
		// 基準の提示重量を確定させるために要る履歴。補助種目は過去の記録が
		// 無いと重量が出ないので、そのぶんを先に置く。
		prior func(t *testing.T) []*training.SetLog
		// 当日こなす1セット。重量は基準からの差で指定する。
		deltaKg   float64
		reps, rir int
	}{
		{
			name: "軸を、提示どおりの重量でこなしても動かない",
			// RIR を守った、それなりにきついセット。推定1RMは上がる方向。
			exercise: "bench", deltaKg: 0, reps: 6, rir: 1,
		},
		{
			// 同じ重量で記録すると推定がそもそも上がらず、当日を混ぜても
			// 数字が変わらないことがある。混ぜたら必ず変わる重さで確かめる。
			name:     "軸を、明らかに重い重量でこなしても動かない",
			exercise: "bench", deltaKg: 40, reps: 8, rir: 3,
		},
		{
			name:     "補助でも動かない",
			exercise: "incline", deltaKg: 5, reps: 12, rir: 0,
			prior: func(t *testing.T) []*training.SetLog {
				t.Helper()
				return []*training.SetLog{
					mkLogOn(t, "inc-old", planMonday.AddDays(-7), "incline", 40, 10, 2),
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := planHistory(t)
			if c.prior != nil {
				logs = append(logs, c.prior(t)...)
			}

			req := planRequest(t)
			req.History = training.NewHistory(logs)

			before, ok := plannedWeight(t, mustPlan(t, req), c.exercise)
			if !ok {
				t.Fatalf("前提: %s の重量が提示されること", c.exercise)
			}

			req.History = training.NewHistory(append(logs,
				mkLogOn(t, "today", planMonday, string(c.exercise),
					before+c.deltaKg, c.reps, c.rir)))

			after, ok := plannedWeight(t, mustPlan(t, req), c.exercise)
			if !ok {
				t.Fatalf("1セット記録したら %s が消えた", c.exercise)
			}
			if after != before {
				t.Errorf("当日の記録で今日の重量が動いた: %v → %v", before, after)
			}
		})
	}
}

// 翌日以降には反映されること。当日を外すのは「その日の中で動かない」
// ためであって、記録を無視するためではない。
func TestSessionPlanner_TodaysLogsMoveLaterSessions(t *testing.T) {
	req := planRequest(t)
	base := mainWeight(t, mustPlan(t, req), "bench")

	// 今日、推定を押し上げる内容で記録する。
	req.History = training.NewHistory(append(planHistory(t),
		mkLogOn(t, "hard-1", planMonday, "bench", base, 10, 0),
		mkLogOn(t, "hard-2", planMonday, "bench", base, 10, 0),
		mkLogOn(t, "hard-3", planMonday, "bench", base, 10, 0)))

	// 翌週の同じ役割の日と比べる。週内の位置を揃えるため7日後を見る。
	req.Date = planMonday.AddDays(7)
	later := mainWeight(t, mustPlan(t, req), "bench")
	if later <= base {
		t.Errorf("記録が後のセッションに反映されていない: %v → %v", base, later)
	}
}

// セッションを最後まで消化すると、予定どおりのセット数で終わること。
//
// 補助を終えるたびに新しい種目が補充されると、種目マスタが尽きるまで
// セッションが終わらない。実サーバーで通しで消化して踏んだ
// （60手やっても8件出続けた）。
// 予定どおりに消化すると、ちょうど予定ぶんのセット数で終わること。
//
// 以前は「消化しきると補助が0件になる」を検査していた。当日を見なくなった
// ので、リストは一日中同じものが返る。終わりを判断するのは本人であって
// サーバーではない（D-116）。ここで見るのは「予定より多く/少なくやることに
// ならないか」だけになった。
func TestSessionPlanner_PlannedWorkIsConsumedExactly(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq := mustFrequency(t, 3)
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	program, err := training.NewProgram(freq, target, selected, big3())
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}

	planner := training.DefaultSessionPlanner()
	date := planMonday
	var logs []*training.SetLog

	plan := func(t *testing.T) training.PlannedSession {
		t.Helper()
		s, err := planner.Plan(training.PlanRequest{
			Program: program, Pool: pool,
			History:    training.NewHistory(logs),
			Conditions: training.NewConditionLog(nil),
			Date:       date,
		})
		if err != nil {
			t.Fatalf("Plan が失敗: %v", err)
		}
		return s
	}

	first := plan(t)
	planned := 0
	for _, set := range append(first.Main(), first.Accessories()...) {
		planned += set.Sets().Int()
	}
	if planned == 0 {
		t.Fatal("前提: 予定のセットがあること")
	}

	done := map[training.ExerciseID]int{}
	recorded := 0
	for step := range planned * 3 {
		s := plan(t)
		var todo *training.PlannedSet
		for _, set := range append(s.Main(), s.Accessories()...) {
			if done[set.ExerciseID()] < set.Sets().Int() {
				v := set
				todo = &v
				break
			}
		}
		if todo == nil {
			break
		}

		kg := 40.0
		if w, ok := todo.Weight(); ok {
			kg = w.Kg()
		}
		recorded++
		done[todo.ExerciseID()]++
		logs = append(logs, mkLogOn(t, fmt.Sprintf("live-%d", step), date,
			string(todo.ExerciseID()), kg, 8, todo.TargetRIR().Int()))
	}

	if recorded != planned {
		t.Errorf("予定 %dセットに対して %dセットこなすことになった", planned, recorded)
	}

	// 消化しきってもリストは同じ。サーバーは「終わり」を言わない。
	// 全部のマスが緑になったかどうかは画面が判断する（D-116）。
	if last := plan(t); len(last.Accessories()) != len(first.Accessories()) {
		t.Errorf("消化しきったら補助の数が変わった: %d → %d",
			len(first.Accessories()), len(last.Accessories()))
	}
}

// 補助種目は、同じ入力なら毎回同じ順で返る。
//
// 選択は集合から選ぶので、そのままだと呼ぶたびに順番が入れ替わる。
// 画面では同じ内容のカードが並び替わり、どこまでやったか見失う。
//
// 以前はここで「IDの昇順であること」も見ていた。やめたのは、それが実装の
// 契約ではなく偶然だったため。usablePool と AccessorySelector.Select の
// ソートを両方とも逆順にする変異を入れても、この検査は緑のまま通った
// （docs/refactoring.md「Select の中のソートが上流と重複している」）。
//
// Select が返す順序の契約は「最も放置している区分から」であって、
// 昇順ではない。偶然を固定すると、優先度の付け方を変えたときに
// 理由の無い赤が出る。
func TestSessionPlanner_AccessoriesComeBackInAStableOrder(t *testing.T) {
	req := planRequest(t)

	first := mustPlan(t, req)
	ids := make([]training.ExerciseID, 0, len(first.Accessories()))
	for _, a := range first.Accessories() {
		ids = append(ids, a.ExerciseID())
	}
	if len(ids) < 2 {
		t.Fatalf("補助が %d 種目しか出ないので順序を確かめられない", len(ids))
	}

	// 何度開き直しても同じ順で出る。
	for n := 0; n < 30; n++ {
		again := mustPlan(t, req)
		if len(again.Accessories()) != len(ids) {
			t.Fatalf("%d回目で補助の数が変わった: %d → %d", n, len(ids), len(again.Accessories()))
		}
		for i, a := range again.Accessories() {
			if a.ExerciseID() != ids[i] {
				t.Fatalf("%d回目で順序が変わった: %v から %v", n, ids, again.Accessories())
			}
		}
	}
}

// chinRequest はチンニング（自重係数0.95）を1つ足したリクエストを返す。
//
// addedKg はこれまで記録してきた加重。bodyweight が0なら体重の記録は
// 一度も無いものとする。
func chinRequest(t *testing.T, addedKg, bodyweight float64) training.PlanRequest {
	t.Helper()

	chin := mustExercise(t, training.ExerciseParams{
		ID: "chin", Name: "chin",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})

	program, err := training.NewProgram(
		mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12}),
		[]training.ExerciseID{"bench", "squat", "deadlift", "chin"},
		big3(),
	)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// 3セッションぶん記録する。推定1RMが立つ量。
	logs := planHistory(t)
	var conds []training.DailyCondition
	for i, daysAgo := range []int{21, 14, 7} {
		day := planMonday.AddDays(-daysAgo)
		for set := range 3 {
			logs = append(logs, mkLogOn(t,
				fmt.Sprintf("chin-%d-%d", i, set), day, "chin", addedKg, 8, 2))
		}
		if bodyweight > 0 {
			conds = append(conds,
				training.NewDailyCondition(day).WithBodyWeight(bodyweight))
		}
	}

	return training.PlanRequest{
		Program:    program,
		Pool:       append(planPool(t), chin),
		History:    training.NewHistory(logs),
		Conditions: training.NewConditionLog(conds),
		Date:       planMonday,
	}
}

// accessorySet は補助種目のうち id のものを返す。
func accessorySet(t *testing.T, s training.PlannedSession, id training.ExerciseID) training.PlannedSet {
	t.Helper()
	for _, set := range s.Accessories() {
		if set.ExerciseID() == id {
			return set
		}
	}
	t.Fatalf("前提: %s が補助として提示されること", id)
	return training.PlannedSet{}
}

// 自重種目の処方は「体重×係数」を引いた加重で出す。
//
// 体重×係数は既に体が負担しているので、付けるプレートはその差分だけ。
// ここが効かないと、チンニングに「85kg」のような総負荷が提示される。
func TestSessionPlanner_BodyweightExerciseIsPrescribedAsAddedWeight(t *testing.T) {
	cases := []struct {
		name       string
		addedKg    float64 // これまで記録してきた加重
		bodyweight float64 // 0 なら体重の記録が一度も無い
		// 体が負担している分。提示はこれを下回らなければならない。
		carriedKg float64
		wantMaxKg float64
	}{
		{
			// 自重だけの記録からでも重量が決まること。以前は0kgのセットを
			// 推定から除外していたので、自重でやり続ける限り推定するものが
			// 何も残らず、永久に「自分で決める」が出ていた。
			name:    "加重0kgの記録からでも重量が決まる",
			addedKg: 0, bodyweight: 75,
			carriedKg: 75 * 0.95, wantMaxKg: 30,
		},
		{
			name:    "加重で記録していれば、その周辺の加重が出る",
			addedKg: 10, bodyweight: 75,
			carriedKg: 75 * 0.95, wantMaxKg: 30,
		},
		{
			// 元は「体重が分からなければ推測せず本人に返す」として
			// 「自分で決める」を出していた。だが「何kgでやるか」はアプリが
			// 答えるべき問いなので、既定体重70kgで処方する。推定と処方の
			// 両側で同じ体重を使うため、既定値が実体からずれても出力は
			// ほとんど動かない（体重20kgのずれで処方は1kg）。
			name:    "体重を一度も測っていなければ既定体重で処方する",
			addedKg: 10, bodyweight: 0,
			carriedKg: defaultBodyWeight * 0.95, wantMaxKg: 30,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := mustPlan(t, chinRequest(t, c.addedKg, c.bodyweight))

			w, ok := accessorySet(t, s, "chin").Weight()
			if !ok {
				t.Fatal("自重種目の重量が決まらない")
			}
			if w.Kg() >= c.carriedKg {
				t.Errorf("提示が %vkg。体重込みの総負荷が出ている（%vkg 未満のはず）",
					w.Kg(), c.carriedKg)
			}
			if w.Kg() > c.wantMaxKg {
				t.Errorf("提示が %vkg。記録した加重 %vkg から出る値としておかしい",
					w.Kg(), c.addedKg)
			}

			// 体重の欠落が、自重の乗らない種目まで巻き込まないこと。
			decided := false
			for _, set := range s.Main() {
				if _, ok := set.Weight(); ok {
					decided = true
				}
			}
			if !decided {
				t.Error("自重が乗らないメイン種目の重量まで決まらなくなっている")
			}
		})
	}
}

// 既定体重が実体からずれても、処方はほとんど動かない。
//
// 体重を測っていない人には既定値70kgで処方する。実体が75kgでも、提示は
// 0.25kgしか変わらない。推定1RMを出すときと処方を加重へ戻すときの両側で
// 同じ体重を使うので、ずれの大半が打ち消し合うため。
//
// この打ち消しが効かないと、既定値の選び方が処方を大きく左右する。実際
// 既定値を0にする変異では、引き算が消えて提示が3kg以上跳ねる。
// 「体重の欠落は自分で決めるに落とす」をやめられたのは、この性質が
// あるからで、性質そのものを検査しておかないと根拠が失われる。
func TestSessionPlanner_DefaultBodyWeightBarelyMovesThePrescription(t *testing.T) {
	const tolerance = 1.0

	measured, ok := accessorySet(t,
		mustPlan(t, chinRequest(t, 10, 75)), "chin").Weight()
	if !ok {
		t.Fatal("体重を測っている場合の重量が決まらない")
	}
	fallback, ok := accessorySet(t,
		mustPlan(t, chinRequest(t, 10, 0)), "chin").Weight()
	if !ok {
		t.Fatal("体重を測っていない場合の重量が決まらない")
	}

	if diff := measured.Kg() - fallback.Kg(); diff > tolerance || diff < -tolerance {
		t.Errorf("体重75kgで %vkg、既定70kgで %vkg。差が %vkg あり、%vkg を超える",
			measured.Kg(), fallback.Kg(), diff, tolerance)
	}
}

// 体重が分からない自重種目の記録も、週の充足には数えること。
//
// 推定に渡す履歴は実効負荷（体重×係数+加重）へ変換し、体重が引けない
// セットは落とす。落とすのは「何kg挙げたか分からない」からであって、
// 「やらなかった」わけではない。
//
// 変換した履歴を残差の計算にも使うと、やったはずのセットが消えて
// 同じ区分の補助が何度でも提示される。カバレッジは重量ではなく
// セット数で数えるので、変換前の履歴を渡すこと。
func TestSessionPlanner_BodyweightSetsStillCountTowardCoverage(t *testing.T) {
	// 広背筋だけを狙う自重種目と、同じ区分を狙う補助を5つ。
	chin := mustExercise(t, training.ExerciseParams{
		ID: "chin", Name: "chin",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})
	pool := append(planPool(t), chin)
	ids := []training.ExerciseID{"bench", "squat", "deadlift", "chin"}
	for i := range 5 {
		id := fmt.Sprintf("lat_%d", i)
		pool = append(pool, mkAccessory(t, id,
			map[training.MuscleRegion]float64{training.Lat: 1.0}))
		ids = append(ids, training.ExerciseID(id))
	}

	program, err := training.NewProgram(mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12}), ids, big3())
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// 週の半ばを対象日にして、その手前に記録を置けるようにする。
	date := planMonday.AddDays(2)
	base := training.PlanRequest{
		Program: program, Pool: pool,
		History:    training.NewHistory(planHistory(t)),
		Conditions: training.NewConditionLog(nil),
		Date:       date,
	}
	want := len(mustPlan(t, base).Accessories())
	if want == 0 {
		t.Fatal("前提: 補助が提示されること")
	}

	// 今週すでに自重で12セットこなした。ただし体重は一度も測っていないので、
	// 実効負荷が出せず、推定用の履歴からは落ちる。
	logs := planHistory(t)
	for i := range 12 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("chin-%d", i),
			planMonday, "chin", 0, 8, 2))
	}
	req := base
	req.History = training.NewHistory(logs)

	got := len(mustPlan(t, req).Accessories())
	if got >= want {
		t.Errorf("自重のセットが残差に反映されていない: %d → %d", want, got)
	}
}

// 自重種目の提示は加重で出す。体重込みの総負荷を見せられても、
// 何をすればいいか分からない。
func TestSessionPlanner_BodyweightExerciseFallsBackToDefaultBodyWeight(t *testing.T) {
	chin := mustExercise(t, training.ExerciseParams{
		ID: "chin", Name: "chin",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})

	pool := append(planPool(t), chin)
	program, err := training.NewProgram(
		mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12}),
		[]training.ExerciseID{"bench", "squat", "deadlift", "chin"},
		big3(),
	)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// 加重10kgでこなしたが、体重は一度も測っていない。
	logs := planHistory(t)
	for i, daysAgo := range []int{21, 14, 7} {
		day := planMonday.AddDays(-daysAgo)
		for set := range 3 {
			logs = append(logs, mkLogOn(t,
				fmt.Sprintf("chin-%d-%d", i, set), day, "chin", 10, 8, 2))
		}
	}

	s := mustPlan(t, training.PlanRequest{
		Program:    program,
		Pool:       pool,
		History:    training.NewHistory(logs),
		Conditions: training.NewConditionLog(nil),
		Date:       planMonday,
	})

	found := false
	for _, set := range s.Accessories() {
		if set.ExerciseID() != training.ExerciseID("chin") {
			continue
		}
		found = true
		w, ok := set.Weight()
		if !ok {
			t.Fatal("体重が無くても既定値で処方されるはず")
		}
		// 既定体重70kg × 0.95 = 66.5kg は体が負担している。付けるプレートは
		// その差分だけなので、提示はこれを下回る。
		if w.Kg() >= defaultBodyWeight*0.95 {
			t.Errorf("提示が %vkg。総負荷がそのまま処方されている", w.Kg())
		}
		// 記録した加重が10kgなので、その周辺に落ちる。
		if w.Kg() <= 0 || w.Kg() > 30 {
			t.Errorf("提示が %vkg。加重10kgの記録から出る値としておかしい", w.Kg())
		}
	}
	if !found {
		t.Fatal("前提: チンニングが補助として提示されること")
	}

	// 自重が乗らないメイン種目は従来どおり決まる。
	decided := false
	for _, set := range s.Main() {
		if _, ok := set.Weight(); ok {
			decided = true
		}
	}
	if !decided {
		t.Error("体重の欠落が、自重の乗らない種目まで巻き込んでいる")
	}
}
