package planning_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

var planMonday = training.MustDate(2026, time.August, 17) // 月曜

func mainExercise(t *testing.T, id string, stimulus map[training.MuscleRegion]float64) *exercise.Exercise {
	t.Helper()
	return mustExercise(t, exercise.ExerciseParams{
		ID: id, Name: id,
		Stimulus: stimulus, IncrementKg: 2.5,
	})
}

func planPool(t *testing.T) []*exercise.Exercise {
	t.Helper()

	// ベンチの派生。重点種目がベンチのとき、バリエーションレーンに出る。
	//
	// 2つあるのは回転を検査するため。1つだと「最終実施日が最も古いものを
	// 取る」が「1つしかないものを取る」と区別できない。
	derived := func(id string) *exercise.Exercise {
		t.Helper()
		return mustExercise(t, exercise.ExerciseParams{
			ID: id, Name: id,
			Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
			IncrementKg: 2.5,
			DerivedFrom: "bench",
		})
	}

	return []*exercise.Exercise{
		mainExercise(t, "bench", map[training.MuscleRegion]float64{
			training.ChestMid: 1.0, training.TricepsLateral: 0.5,
		}),
		mainExercise(t, "squat", map[training.MuscleRegion]float64{
			training.Quad: 1.0, training.Glute: 0.5,
		}),
		mainExercise(t, "deadlift", map[training.MuscleRegion]float64{
			training.Hamstring: 1.0, training.Erector: 1.0,
		}),
		derived("larsen"),
		derived("tempo"),
		mkAccessory(t, "incline", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
	}
}

// planProgram は既定のプランニング用プログラム。週目標も一緒に返す。
//
// Program はもう週目標を持たない（#176）ので、呼び出し側は
// PlanRequest.Target に別途渡す必要がある。
func planProgram(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl"}, big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p, target
}

// focusedProgram は重点種目を指定したプログラムを返す。
//
// 派生（larsen・tempo）も選択に入れる。選択されていない種目は usablePool から
// 落ちるので、バリエーションレーンの候補にもならない。
func focusedProgram(t *testing.T, focus exercise.ExerciseID) (*program.Program, program.WeeklyVolumeTarget) {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl", "larsen", "tempo"},
		big3(), focus)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p, target
}

// planHistory は3週分の履歴（推定1RMが立つ量）。
func planHistory(t *testing.T) []*setlog.SetLog {
	t.Helper()

	logs := make([]*setlog.SetLog, 0, 9)
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

func planRequest(t *testing.T) planning.PlanRequest {
	t.Helper()
	prog, target := planProgram(t)
	return planning.PlanRequest{
		Program:    prog,
		Target:     target,
		Pool:       planPool(t),
		History:    setlog.NewHistory(planHistory(t)),
		Conditions: condition.NewConditionLog(nil),
		Date:       planMonday,
	}
}

func mustPlan(t *testing.T, req planning.PlanRequest) planning.PlannedSession {
	t.Helper()
	s, err := planning.DefaultSessionPlanner().Plan(req)
	if err != nil {
		t.Fatalf("Plan が失敗: %v", err)
	}
	return s
}

// 伸ばしたい種目は、軸でない日に補助として出てこない。
//
// 以前は「宣言した種目も、ヘビー枠でなければ補助として残差を埋める」と
// していた。除きすぎると脚の日にスクワットがどこにも出なくなる、という
// 理由だったが、それは的外れだった。スクワットが軸でない日に脚を埋めるのは
// レッグプレスやレッグカールであって、スクワットである必要が無い。
//
// 宣言は軸レーンで扱うものと割り切ると、レーンの境界がはっきりする。
//
// 差が出る状況は狭い。軸として毎回出ている種目はその区分が常に「最近
// 刺激した」状態なので、放置日数で選ぶ補助の候補に上がらない。3日前に
// ベンチをやって軸を他へ移し、回復期間（2日）を抜け、胸の残差を大きく
// した日に初めて候補へ上がる。だからシミュレーションでは数字が動かない。
func TestSessionPlanner_DeclaredExercisesNeverAppearAsAccessories(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 30})
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl", "larsen"},
		big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Program = p
	req.Target = target
	req.History = setlog.NewHistory(append(planHistory(t),
		mkLogOn(t, "b-recent", planMonday.AddDays(-3), "bench", 85, 8, 2)))

	s := mustPlan(t, req)
	if got := s.Main()[0].ExerciseID(); got == "bench" {
		t.Fatalf("前提: 軸がベンチ以外であること（いま %s）", got)
	}

	ids := accessoryIDs(s)
	for _, id := range big3() {
		if slices.Contains(ids, id) {
			t.Errorf("伸ばしたい種目 %s が補助に出ている: %v", id, ids)
		}
	}
	// 除外が広がりすぎていないこと。ベンチの派生は補助として残る。
	if !slices.Contains(ids, exercise.ExerciseID("larsen")) {
		t.Errorf("宣言していない種目まで補助から消えている: %v", ids)
	}
}

// ヘビー枠は1セッションに1つ。
//
// 以前は宣言した種目すべてにスロットを割り当てていた。週5回にすると
// 1種目あたり19セット/週になり、それが maxFrequencyPerWeek = 4 の
// 理由になっていた。1つに絞ると、週6でも各種目は週1〜2回で収まる。
//
// 選ばれなかった宣言は補助として残差を埋める。「伸ばしたい」という
// 目標であって「ヘビーでしかやらない」ではない（D-117）。
func TestSessionPlanner_HeavySlotIsExactlyOne(t *testing.T) {
	s := mustPlan(t, planRequest(t))

	if len(s.Main()) != 1 {
		t.Errorf("ヘビー枠が %d 件。1件のはず: %v", len(s.Main()), s.Main())
	}
	if !s.Date().Equal(planMonday) {
		t.Errorf("日付が誤り: %v", s.Date())
	}
}

// benchOnlyProgram は宣言をベンチ1つに絞ったプログラムを返す。
//
// ヘビー枠は宣言のうち最終実施日が最も古いものが取るので、宣言が複数
// あると「今日どれが軸になるか」が履歴で動く。役割や強度だけを見たい
// テストでは、宣言を1つにして軸を固定する。
func benchOnlyProgram(t *testing.T) (*program.Program, program.WeeklyVolumeTarget) {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl"},
		[]exercise.ExerciseID{"bench"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p, target
}

// planRequestAt は、ベンチを今週すでに done 回やった状態で days 日目の
// リクエストを返す。宣言はベンチ1つに絞る。
//
// done を受け取るのは、週内の実施回数で処方が変わっていた頃の名残。
// いまは3レーンとも定数なので（D-126）、効くのは推定1RMと残差だけ。
func planRequestAt(t *testing.T, days int, done ...int) planning.PlanRequest {
	t.Helper()

	logs := planHistory(t)
	for i, d := range done {
		logs = append(logs,
			mkLogOn(t, fmt.Sprintf("done-%d", i), planMonday.AddDays(d), "bench", 85, 8, 2))
	}

	req := planRequest(t)
	req.Program, req.Target = benchOnlyProgram(t)
	req.History = setlog.NewHistory(logs)
	req.Date = planMonday.AddDays(days)
	return req
}

// 宣言した種目は順に回ってくる。回数を設定する箇所はどこにも無い。
//
// ヘビー枠は「最後にやったのが最も古い種目」なので、こなすたびに次の
// 種目へ移る。宣言が3つで週3回通えば、各種目は週1回ヘビーになる。
// 宣言を4つに増やせば3回に3回。「ベンチを週2回」というノブを置かずに、
// 頻度が宣言の数と通う回数から導かれる（D-117）。
//
// これが緑になれば設計が成立している。却下した「種目ごとの週N回」を
// 置かずに済んでいる、ということ。
func TestSessionPlanner_DeclaredExercisesTakeTurns(t *testing.T) {
	logs := planHistory(t)
	seen := make([]exercise.ExerciseID, 0, 6)

	// 6日連続で通い、その日のヘビー枠を毎回こなす。
	for day := range 6 {
		req := planRequest(t)
		req.History = setlog.NewHistory(logs)
		req.Date = planMonday.AddDays(day)

		main := mustPlan(t, req).Main()
		if len(main) != 1 {
			t.Fatalf("%d日目のヘビー枠が %d 件", day, len(main))
		}
		heavy := main[0].ExerciseID()
		seen = append(seen, heavy)

		logs = append(logs, mkLogOn(t, fmt.Sprintf("d%d", day),
			planMonday.AddDays(day), string(heavy), 80, 8, 2))
	}

	// 宣言は3つ。連続で同じ種目が来てはいけない。
	for i := 1; i < len(seen); i++ {
		if seen[i] == seen[i-1] {
			t.Errorf("同じ種目が連続した: %v", seen)
			break
		}
	}

	// 6日で各種目が2回ずつ回る。
	count := map[exercise.ExerciseID]int{}
	for _, id := range seen {
		count[id]++
	}
	for _, id := range big3() {
		if count[id] != 2 {
			t.Errorf("%s が %d 回。6日で3種目なら2回のはず: %v", id, count[id], seen)
		}
	}
}

// 同じ日に複数の宣言が候補になったら、最終実施日が古い方を取る
// 一度もやってない種目は最優先
func TestSessionPlanner_HeavySlotGoesToTheStalestDeclared(t *testing.T) {
	cases := []struct {
		name          string
		lastPerformed map[exercise.ExerciseID]int
		want          exercise.ExerciseID
	}{{
		name: "最後にやったのが最も古い種目がヘビー枠になる",
		lastPerformed: map[exercise.ExerciseID]int{
			"bench": -2, "squat": -3, "deadlift": -6,
		},
		want: "deadlift",
	},
		{
			// 記録が無い＝一度もやっていない。日付のゼロ値が最も古い。
			// ここが逆だと、新しく宣言した種目が永久に出ない。
			name: "一度もやっていない種目が最優先",
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -2, "squat": -3,
			},
			want: "deadlift",
		},
		{
			// 同点はマスタ順。決定性のため。
			name: "最終実施日が同じならマスタ順",
			lastPerformed: map[exercise.ExerciseID]int{
				"bench": -3, "squat": -3, "deadlift": -3,
			},
			want: "bench",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := []*setlog.SetLog{}
			for id, daysAgo := range c.lastPerformed {
				logs = append(logs, mkLogOn(t, string(id)+"-last",
					planMonday.AddDays(daysAgo), string(id), 80, 8, 2))
			}

			req := planRequest(t)
			req.History = setlog.NewHistory(logs)

			main := mustPlan(t, req).Main()
			if len(main) != 1 {
				t.Fatalf("ヘビー枠が %d 件。1件のはず", len(main))
			}
			if main[0].ExerciseID() != c.want {
				t.Errorf("ヘビー枠が %s。%s のはず", main[0].ExerciseID(), c.want)
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
func TestSessionPlanner_HeavySlotKeepsTheDeclaredExercise(t *testing.T) {
	s := mustPlan(t, planRequestAt(t, 4, 0, 2))

	found := false
	for _, set := range s.Main() {
		if set.ExerciseID() == exercise.ExerciseID("larsen") {
			t.Error("軸の種目が派生に差し替わっている")
		}
		if set.ExerciseID() == exercise.ExerciseID("bench") {
			found = true
		}
	}
	if !found {
		t.Error("軸のベンチが出ていない")
	}
}

func TestSessionPlanner_NoHistoryMeansNoWeight(t *testing.T) {
	req := planRequest(t)
	req.History = setlog.NewHistory(nil)

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
	items := make([]condition.DailyCondition, 0, 15)
	for i := range 15 {
		h := 7.5
		if i == 0 {
			h = 4.0
		}
		items = append(items, condition.NewDailyCondition(planMonday.AddDays(-i)).WithSleepHours(h))
	}
	req.Conditions = condition.NewConditionLog(items)
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

func TestSessionPlanner_RejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*planning.PlanRequest)
	}{
		{"プログラムが nil", func(r *planning.PlanRequest) { r.Program = nil }},
		// 週目標は Program から取らず、呼び出し側が別に渡す
		// （seed.DefaultWeeklyTarget）。渡し忘れをここで弾かないと、
		// ゼロ値のまま AccessoryAllocator まで届くことになる
		// （Allocate は週目標が空だとエラーを返す）。
		{"週目標が空", func(r *planning.PlanRequest) { r.Target = program.WeeklyVolumeTarget{} }},
		{"対象日が無い", func(r *planning.PlanRequest) { r.Date = training.Date{} }},
		{"種目プールが空", func(r *planning.PlanRequest) { r.Pool = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := planRequest(t)
			c.mutate(&req)
			if _, err := planning.DefaultSessionPlanner().Plan(req); err == nil {
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
	program, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"curl"}, []exercise.ExerciseID{"curl"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Program = program
	req.Target = target

	session, err := planning.DefaultSessionPlanner().Plan(req)
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
	var p planning.SessionPlanner
	if _, err := p.Plan(planRequest(t)); err == nil {
		t.Error("ゼロ値の生成器が通ってしまう")
	}
}

func TestNewSessionPlanner_RejectsZeroDependencies(t *testing.T) {
	est := planning.DefaultOneRepMaxEstimator()
	acc := planning.DefaultAccessoryAllocator()
	analyzer := planning.DefaultConditionAnalyzer()

	cases := []struct {
		name string
		call func() error
	}{
		{"推定器", func() error {
			_, err := planning.NewSessionPlanner(planning.OneRepMaxEstimator{}, acc, analyzer)
			return err
		}},
		{"補助の割り振り器", func() error {
			_, err := planning.NewSessionPlanner(est, planning.AccessoryAllocator{}, analyzer)
			return err
		}},
		{"コンディション分析器", func() error {
			_, err := planning.NewSessionPlanner(est, acc, planning.ConditionAnalyzer{})
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

	if _, err := planning.NewSessionPlanner(est, acc, analyzer); err != nil {
		t.Errorf("正常な依存が弾かれた: %v", err)
	}
}

func TestSessionPlanner_ReturnValuesAreDefensivelyCopied(t *testing.T) {
	s := mustPlan(t, planRequest(t))

	main := s.Main()
	main[0] = planning.PlannedSet{}
	if s.Main()[0].IsZero() {
		t.Error("Main の書き換えが波及している")
	}

	accessories := s.Accessories()
	if len(accessories) > 0 {
		accessories[0] = planning.PlannedSet{}
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

	firstIDs := map[exercise.ExerciseID]bool{}
	for _, set := range first.Accessories() {
		firstIDs[set.ExerciseID()] = true
	}
	if !firstIDs["incline"] || !firstIDs["curl"] {
		t.Fatalf("前提が崩れている（1本目の補助）: %v", firstIDs)
	}

	// 月曜に大胸筋上部の窓ぶんの目標（週9セット × 窓の週数）を全部
	// こなしたことにする。窓は4週なので、週目標ぶんでは満たされない。
	logs := planHistory(t)
	for i := range 9 * planning.CoverageWindowWeeks {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("inc-%d", i),
			planMonday, "incline", 30, 10, 2))
	}
	req.History = setlog.NewHistory(logs)
	req.Date = planMonday.AddDays(2)

	second := mustPlan(t, req)
	for _, set := range second.Accessories() {
		if set.ExerciseID() == "incline" {
			t.Errorf("目標を満たした区分がまだ狙われている: %v", set.ExerciseID())
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
// こと。これが受け入れ条件で、ここが守られていれば「終えた種目が消える」
// 「並びが入れ替わる」「枠が補充されて終わらない」は原理的に起きなくなる。
// かつて別々に手当てしていた不具合は、すべてこの1点の派生だった（D-116）。
//
// 比べるのは種目の並びだけでなく、3レーンの PlannedSet 全体（種目・重量の
// 有無と値・セット数・目標 RIR）。並びだけだと、推定や上乗せ（overload）に
// 当日の記録が混ざって「追い込むほど次のセットが重くなる」形に戻っても緑の
// まま通った（#172）。重量を「どのセットをこなしても」の網羅で守るのは
// TodaysLogsDoNotMoveTodaysWeight で、こちらは計画の導出を触ったときに
// 1本回せば分かる入口。
//
// 上乗せの判定に当日を混ぜる変異を捕まえるのは「分割も重点種目も無い」と
// 「バリエーションが出る」の2ケース。分割のケースは履歴が空で推定が立たず、
// 重点種目の一巡は派生（tempo）の記録が1セッションしか無いので、どちらも
// 上乗せが発火しうる状態にない。
func TestSessionPlanner_PlanIsFixedForTheWholeDay(t *testing.T) {
	cases := []struct {
		name string
		req  func(t *testing.T) planning.PlanRequest
		// wantVariation はバリエーションレーンが出ること。出ない構成のまま
		// 3レーンを比べても、バリエーションの重量にまつわる退行（推定や
		// 上乗せに当日の記録が混ざる）は原理的に発火しないので、比較しても
		// 何も守れない。
		wantVariation bool
		// wantMainWeight は軸に重量が出ること。出ない構成（このテーブルでは
		// 分割のケース。履歴が空で推定が立たない）では、推定や上乗せに当日を
		// 混ぜる変異が発火せず、重量欄の比較が空振りする。
		wantMainWeight bool
		// pinMainIntensity は0でなければ、軸の重量がその種目の推定1RMの
		// ちょうどこの倍率であることも確認する。上乗せ（overload）がまだ
		// 発火していない状態から始めていることのピン。0なら確認しない。
		//
		// plain だけに立てる。plain が上乗せの当日混入を捕まえるのは、
		// 軸（ベンチ）の履歴が planHistory の3セッション（-21・-14・-7日、
		// いずれも同じ85kg・8レップ・RIR2）で overloadSessions=3 をちょうど
		// 満たし、かつ一度も目標RIRを割っていないため。ここが崩れる
		// （セッション数が減る、重量やRIRが変わる）と、1本目の記録で
		// 上乗せが発火する前提そのものが消える。
		pinMainIntensity float64
	}{
		{
			name:             "分割も重点種目も無い",
			req:              planRequest,
			wantVariation:    false,
			wantMainWeight:   true,
			pinMainIntensity: 0.88,
		},
		{
			// 周期の位置を当日込みの出席回数で数えると、1セット記録した瞬間に
			// 今日がセッションになって周期が1つ進む。守っているのは2箇所。
			//
			//   - 今日の分割（SplitOn）：上の日が下の日に変わり、軸がベンチから
			//     スクワットに入れ替わる
			//   - 1回ぶんの天井（activeCount）：胸が来る回数が 上下上＝2 から
			//     下上下＝1 になり、天井が 24/2 から 24/1 に上がって補助が増える
			//
			// 週3回にするのは後者のため。周期の長さ（2）で頻度が割り切れると、
			// どこから歩いても各区分の回数が同じになって差が出ない。
			name:           "分割がある（周期は出席回数で進む）",
			req:            fixedDaySplitRequest,
			wantVariation:  false,
			wantMainWeight: false,
		},
		{
			// 一巡の位置を当日込みで数えると、派生の番（位置2）で1セット記録した
			// 瞬間に位置が0へ進み、軸が派生から本体に入れ替わる。派生を選ぶ
			// 「最も古いもの」も、当日の記録で入れ替わる。守っているのは axis。
			name: "重点種目の一巡が派生の番",
			req: func(t *testing.T) planning.PlanRequest {
				return rotationRequest(t, 5)
			},
			wantVariation:  false,
			wantMainWeight: true,
		},
		{
			// バリエーションレーンを踏む唯一のケース。他の3ケースはどれも
			// バリエーションが出ない構成（重点種目が無い／派生が軸そのもの／
			// 記録が無い）なので、バリエーションの重量に当日の記録が
			// 混ざる退行はこのケースでしか捕まらない。
			//
			// 軸はBIG3のうち最終実施日が最も古いデッドリフト（planHistory の
			// 3セッションのまま）、重点種目はベンチ、バリエーションはテンポ
			// （ラーセンより最終実施日が古い -12日）。テンポにも1セッション
			// 記録があるので推定が立ち、重量が付く。重量が付く構成にしたのは、
			// 重量が無いと planDiff の重量比較が variation で
			// 「有無: false → false」のまま空振りし、推定に当日の記録が
			// 混ざる変異（バリエーションの重量が動く形の退行）が見えなく
			// なるため。
			//
			// 軸（デッドリフト）も planHistory の3セッションが手つかずなので、
			// 上乗せの当日混入は plain に加えてここでも捕まる。
			name: "バリエーションが出る（重点種目あり）",
			req: func(t *testing.T) planning.PlanRequest {
				req := planRequest(t)
				req.Program, req.Target = focusedProgram(t, "bench")
				req.History = setlog.NewHistory(append(planHistory(t),
					mkLogOn(t, "bench-recent", planMonday.AddDays(-3), "bench", 85, 8, 2),
					mkLogOn(t, "larsen-last", planMonday.AddDays(-10), "larsen", 80, 8, 2),
					mkLogOn(t, "tempo-last", planMonday.AddDays(-12), "tempo", 80, 8, 2),
				))
				return req
			},
			wantVariation:  true,
			wantMainWeight: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := c.req(t)
			base := req.History.Logs()

			first := mustPlan(t, req)

			// 前提: 種目が1つも出ていないケースを比べても何も守れない。
			// 3レーンすべて（main・variation・accessories）を数える。
			total := 0
			for _, lane := range plannedLanes(first) {
				total += len(lane.sets)
			}
			if total == 0 {
				t.Fatal("前提: 種目が1つも出ていない")
			}

			// 前提: バリエーションの有無が、このケースが検査しようとしている
			// ものと一致しているか。ここがずれると、バリエーションの重量欄を
			// 比べているつもりで何も比べていないケースが混ざる。
			if got := len(first.Variation()) > 0; got != c.wantVariation {
				t.Fatalf("前提: バリエーションの有無が %v。%v のはず", got, c.wantVariation)
			}

			// 前提: 軸の重量の有無。同上、軸の重量欄が生きているかの確認。
			if len(first.Main()) == 0 {
				if c.wantMainWeight {
					t.Fatal("前提: 軸に重量が出るはずだが軸が無い")
				}
			} else if _, ok := first.Main()[0].Weight(); ok != c.wantMainWeight {
				t.Fatalf("前提: 軸の重量の有無が %v。%v のはず", ok, c.wantMainWeight)
			}

			if c.pinMainIntensity != 0 {
				assertIntensity(t, req, first.Main()[0], c.pinMainIntensity)
			}

			// 提示されたとおりに1セットずつ、3レーンすべて記録しては開き直す。
			logs := append([]*setlog.SetLog{}, base...)
			n := 0
			for _, lane := range plannedLanes(first) {
				for _, set := range lane.sets {
					for range set.Sets().Int() {
						n++
						logs = append(logs, mkLogOn(t, fmt.Sprintf("d%03d", n), req.Date,
							string(set.ExerciseID()), 40, 8, 2))

						req.History = setlog.NewHistory(logs)
						if diff := planDiff(first, mustPlan(t, req)); len(diff) > 0 {
							t.Fatalf("%dセット記録した時点で計画が変わった\n  %s",
								n, strings.Join(diff, "\n  "))
						}
					}
				}
			}
		})
	}
}

// Plan の結果に、その日の分割の日が乗ること。
//
// 今日の sessionDTO はまだこの値を読まない（読み始めるのは見込みの画面
// から、PR2・PR3）。ここで先に固定するのは、次のタスクで Forecast が
// Plan の回0をそのまま返す形になったとき、分割の日だけがすり抜けて
// null になる退行を早期に潰すため。
func TestSessionPlanner_Plan_CarriesTheSplitDay(t *testing.T) {
	cases := []struct {
		name      string
		req       func(t *testing.T) planning.PlanRequest
		wantHas   bool
		wantSplit string
	}{
		{"分割なしは false", planRequest, false, ""},
		{"分割ありは周期の先頭", fixedDaySplitRequest, true, "上"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := mustPlan(t, c.req(t))
			split, hasSplit := got.Split()
			if hasSplit != c.wantHas {
				t.Fatalf("分割の有無が %v。%v のはず", hasSplit, c.wantHas)
			}
			if hasSplit && split.Name() != c.wantSplit {
				t.Errorf("分割の日が %q。%q のはず", split.Name(), c.wantSplit)
			}
		})
	}
}

// fixedDaySplitRequest は上下2分割・週3回で、まだ1度も通っていない入力。
// 周期の先頭＝上の日で、軸はベンチ。
func fixedDaySplitRequest(t *testing.T) planning.PlanRequest {
	t.Helper()

	pool := splitPool(t)
	ids := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		ids = append(ids, e.ID())
	}
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 24, training.Quad: 24,
	})
	prog, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		ids, []exercise.ExerciseID{"bench", "squat"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	prog, err = prog.WithCycle([]program.Split{
		mkSplit(t, "上", training.ChestMid),
		mkSplit(t, "下", training.Quad),
	})
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}
	return splitRequest(t, prog, target)
}

// plannedLane は計画の1レーンと、失敗メッセージに出す名前。
type plannedLane struct {
	name string
	sets []planning.PlannedSet
}

// plannedLanes は計画の3レーンを提示の順に返す。
func plannedLanes(s planning.PlannedSession) []plannedLane {
	return []plannedLane{
		{"main", s.Main()},
		{"variation", s.Variation()},
		{"accessories", s.Accessories()},
	}
}

// planDiff は2つの計画を、分割の日（Split）と3レーンの PlannedSet
// 全体で比べ、違いを「どのレーンの何番目の、どのフィールドが、何から
// 何へ」の形で返す。同じなら空。
//
// Forecast が全回に Split() を持つようになった（PlannedSession.Split()
// タスク2）ので、一日中変わらないことの検査もここで一緒に見る。
func planDiff(want, got planning.PlannedSession) []string {
	var out []string
	wantSplit, wantHasSplit := want.Split()
	gotSplit, gotHasSplit := got.Split()
	if wantHasSplit != gotHasSplit || wantSplit.Name() != gotSplit.Name() {
		out = append(out, fmt.Sprintf("分割の日: %v(%v) → %v(%v)",
			wantSplit.Name(), wantHasSplit, gotSplit.Name(), gotHasSplit))
	}
	gotLanes := plannedLanes(got)
	for i, w := range plannedLanes(want) {
		g := gotLanes[i]
		if len(w.sets) != len(g.sets) {
			out = append(out, fmt.Sprintf("%s の件数: %d → %d", w.name, len(w.sets), len(g.sets)))
			continue
		}
		for j := range w.sets {
			a, b := w.sets[j], g.sets[j]
			at := fmt.Sprintf("%s[%d]", w.name, j)
			if a.ExerciseID() != b.ExerciseID() {
				out = append(out, fmt.Sprintf("%s の種目: %s → %s", at, a.ExerciseID(), b.ExerciseID()))
				// 種目が違えば残りのフィールドを比べても意味がない。
				continue
			}
			at = fmt.Sprintf("%s（%s）", at, a.ExerciseID())
			aw, aok := a.Weight()
			bw, bok := b.Weight()
			if aok != bok {
				out = append(out, fmt.Sprintf("%s の重量の有無: %v → %v", at, aok, bok))
			} else if aok && aw.Kg() != bw.Kg() {
				out = append(out, fmt.Sprintf("%s の重量: %vkg → %vkg", at, aw.Kg(), bw.Kg()))
			}
			if a.Sets().Int() != b.Sets().Int() {
				out = append(out, fmt.Sprintf("%s のセット数: %d → %d", at, a.Sets().Int(), b.Sets().Int()))
			}
			if a.TargetRIR().Int() != b.TargetRIR().Int() {
				out = append(out, fmt.Sprintf("%s の目標 RIR: %d → %d", at, a.TargetRIR().Int(), b.TargetRIR().Int()))
			}
		}
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
	req.History = setlog.NewHistory(logs)

	reopened := mustPlan(t, req)
	if len(reopened.Accessories()) != len(base.Accessories()) {
		t.Errorf("当日の記録で補助の数が変わった: %d → %d",
			len(base.Accessories()), len(reopened.Accessories()))
	}
}

// 補助レーンの処方を固定する。
//
// 3レーンとも定数になった（D-126）ので、軸・バリエーションと同じ形で
// 補助も見ておく。セット数は4レーン共通で、利用者の設定
// （req.Program.SessionVolume().Sets()）から来る。
func TestSessionPlanner_AccessoryPrescriptionIsPinned(t *testing.T) {
	const (
		wantIntensity = 0.71
		wantRIR       = 2
	)

	// 既定の履歴は宣言種目だけなので、補助には重量が付かない。
	// 強度を見るために補助にも記録を積む。
	logs := planHistory(t)
	for i, daysAgo := range []int{21, 14, 7} {
		for _, id := range []string{"incline", "curl"} {
			logs = append(logs, mkLogOn(t, fmt.Sprintf("%s-%d", id, i),
				planMonday.AddDays(-daysAgo), id, 40, 10, 2))
		}
	}

	req := planRequest(t)
	req.History = setlog.NewHistory(logs)

	s := mustPlan(t, req)
	if len(s.Accessories()) == 0 {
		t.Fatal("補助種目が1つも出ていない")
	}

	wantSets := req.Program.SessionVolume().Sets()
	pct, err := training.NewIntensityPct(wantIntensity)
	if err != nil {
		t.Fatalf("強度: %v", err)
	}

	// 重量が確定した補助が1つでもあること。全部未確定だと強度を見ていない
	// のと変わらない。
	checked := 0
	for _, got := range s.Accessories() {
		if n := got.Sets().Int(); n != wantSets {
			t.Errorf("%v のセット数が %d。%d のはず", got.ExerciseID(), n, wantSets)
		}
		if r := got.TargetRIR().Int(); r != wantRIR {
			t.Errorf("%v の目標RIRが %d。%d のはず", got.ExerciseID(), r, wantRIR)
		}

		w, ok := got.Weight()
		if !ok {
			continue
		}
		orm, ok := planning.DefaultOneRepMaxEstimator().
			Estimate(req.History, got.ExerciseID(), req.Date)
		if !ok {
			t.Fatalf("重量が付いているのに推定1RMが出ない: %v", got.ExerciseID())
		}
		want, err := orm.WorkWeight(pct, findInPool(t, req.Pool, got.ExerciseID()).Increment())
		if err != nil {
			t.Fatalf("実施重量: %v", err)
		}
		if w.Kg() != want.Kg() {
			t.Errorf("%v の重量が %vkg。推定1RM %vkg の %v = %vkg のはず",
				got.ExerciseID(), w.Kg(), orm.Kg(), wantIntensity, want.Kg())
		}
		checked++
	}
	if checked == 0 {
		t.Error("重量が確定した補助が1つも無い。強度を見られていない")
	}
}

// 軸レーンの処方を固定する。
//
// 3レーンとも定数になったので、値そのものを見ておかないと書き換えに
// 誰も気づかない。期待する 0.88 は実装とは独立にここへ書く。
func TestSessionPlanner_HeavyPrescriptionIsPinned(t *testing.T) {
	const (
		wantIntensity = 0.88
		wantSets      = 3
		wantRIR       = 1
	)

	req := planRequest(t)
	s := mustPlan(t, req)
	got := s.Main()[0]

	if n := got.Sets().Int(); n != wantSets {
		t.Errorf("セット数が %d。%d のはず", n, wantSets)
	}
	if r := got.TargetRIR().Int(); r != wantRIR {
		t.Errorf("目標RIRが %d。%d のはず", r, wantRIR)
	}

	orm, ok := planning.DefaultOneRepMaxEstimator().
		Estimate(req.History, got.ExerciseID(), req.Date)
	if !ok {
		t.Fatalf("前提: 推定1RMが出ること: %v", got.ExerciseID())
	}
	target := findInPool(t, req.Pool, got.ExerciseID())
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

// findInPool はプールから種目を引く。見つからなければ失敗。
func findInPool(t *testing.T, pool []*exercise.Exercise, id exercise.ExerciseID) *exercise.Exercise {
	t.Helper()
	for _, e := range pool {
		if e.ID() == id {
			return e
		}
	}
	t.Fatalf("プールに %v が無い", id)
	return nil
}

// メインが埋めた刺激を残差から差し引くこと。
//
// 差し引かないと、メインで十分に刺激した区分を補助でもう一度狙い、
// 週目標を大きく超過する。
func TestSessionPlanner_SubtractsMainCoverageFromResidual(t *testing.T) {
	// 大胸筋中部を狙う補助種目をプールに足す。
	pool := append(planPool(t),
		mkAccessory(t, "pec_fly", map[training.MuscleRegion]float64{training.ChestMid: 1.0}))

	// 窓（4週）ぶんの目標をちょうど6にする。planHistory が窓の中にベンチを
	// 3セット置いていて、今日の軸（ベンチ）が3セット埋めるので、メインの
	// 刺激を差し引けば残差は0になる。
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 6.0 / planning.CoverageWindowWeeks,
	})
	program, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "pec_fly"}, big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Pool, req.Program = pool, program
	req.Target = target

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
func chestUpperRequest(t *testing.T) planning.PlanRequest {
	t.Helper()

	pool := planPool(t)
	ids := []exercise.ExerciseID{"bench", "squat", "deadlift", "incline"}
	for i := range 5 {
		id := fmt.Sprintf("chest_up_%d", i)
		pool = append(pool, mkAccessory(t, id,
			map[training.MuscleRegion]float64{training.ChestUpper: 1.0}))
		ids = append(ids, exercise.ExerciseID(id))
	}

	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestUpper: 12})
	program, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		ids, big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Pool, req.Program = pool, program
	req.Target = target
	return req
}

// 直近4週で埋まったぶんだけ、補助が減ること。
//
// 暦週のころは「残りセッション数で割る」だったので、同じ不足でも週の
// 後半ほど1回あたりの量が増えた。ローリング窓では窓から落ちた分が
// 戻ってくるだけなので、週のどこにいるかでは変わらない。
func TestSessionPlanner_AccessoriesFollowTheRollingGap(t *testing.T) {
	req := chestUpperRequest(t)

	// 直近4週に何も無い。窓ぶんの目標を埋めにいくので上限まで出る。
	empty := mustPlan(t, req)

	// 直近4週で「窓ぶんの目標 − 3」埋まっている。残りは3で、1種目ぶん。
	logs := planHistory(t)
	for i := range 12*planning.CoverageWindowWeeks - 3 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("c-%d", i),
			planMonday.AddDays(-2), "incline", 30, 10, 2))
	}
	req.History = setlog.NewHistory(logs)

	partial := mustPlan(t, req)
	if len(partial.Accessories()) >= len(empty.Accessories()) {
		t.Errorf("埋まった分が残差から引かれていない: %d → %d (%v)",
			len(empty.Accessories()), len(partial.Accessories()), accessoryIDs(partial))
	}
	if len(partial.Accessories()) == 0 {
		t.Error("残り3セットあるのに補助が1件も出ていない")
	}
}

// inclineDeselectedRequest は chestUpperRequest から incline だけを選択から
// 外したもの。マスタ（Pool）には残っている。
func inclineDeselectedRequest(t *testing.T) planning.PlanRequest {
	t.Helper()

	ids := []exercise.ExerciseID{"bench", "squat", "deadlift"}
	for i := range 5 {
		ids = append(ids, exercise.ExerciseID(fmt.Sprintf("chest_up_%d", i)))
	}
	deselected, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		ids, big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// req.Target は chestUpperRequest がすでに ChestUpper:12 を持たせて
	// いる。deselected も同じ週目標のプログラムなので、そのまま使い回せる。
	req := chestUpperRequest(t)
	req.Program = deselected
	return req
}

// やったセットは、いま「使う種目」に入れているかに関係なく、やったセット。
//
// 選択から外した種目の記録を読み飛ばすと、外した瞬間にその区分の残差が
// ふくらみ、前日にやっていても回復中にならず、同じ区分の補助が余計に出る。
// 画面の「充足」（query.Stats.WeeklyVolume）はマスタ全件で数えるので、
// 画面では埋まっているのに補助だけが出続ける、という食い違いにもなる。
//
// 選択が決めるのは「これから何を出すか」で、「何をやったか」ではない。
func TestSessionPlanner_DeselectedExercisesStillCountAsDone(t *testing.T) {
	base := inclineDeselectedRequest(t)

	// 記録が無ければ補助は出る。ここが 0 だと、下のケースの「0件」は
	// 何も検査していない。
	if len(mustPlan(t, base).Accessories()) == 0 {
		t.Fatal("前提が崩れている: 記録が無いのに胸上部の補助が出ていない")
	}

	cases := []struct {
		name           string
		daysFromMonday int
		sets           int
	}{
		{
			// 数える経路（CoverageBetween）。窓ぶんの目標（12 × 窓の週数）を
			// 使い切っているので残差は 0。2日前は回復期間 (date-2, date) の外
			// なので、補助が出ないのは残差が埋まっているからでしかない。
			name: "2日前に外した種目で目標を埋めていたら補助は出ない", daysFromMonday: -2,
			sets: 12 * planning.CoverageWindowWeeks,
		},
		{
			// 回復を見る経路（Select の辞書）。残差は窓ぶんの目標から3を
			// 引いたぶん残っているので、補助が出ないのは胸上部が回復中だから
			// でしかない。
			name: "前日に外した種目でやっていたら回復中として補助は出ない", daysFromMonday: -1, sets: 3,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := planHistory(t)
			for i := range c.sets {
				logs = append(logs, mkLogOn(t, fmt.Sprintf("gone-%d", i),
					planMonday.AddDays(c.daysFromMonday), "incline", 30, 10, 2))
			}

			req := base
			req.History = setlog.NewHistory(logs)

			if got := accessoryIDs(mustPlan(t, req)); len(got) != 0 {
				t.Errorf("外した種目の記録が読み飛ばされている: 補助が %v。0件のはず", got)
			}
		})
	}
}

// 選択から外した種目は、補助の候補にならない。
//
// 記録を数えるために Select へマスタ全件を渡すようにしたので（#133）、
// 候補から落とすのは exclude の仕事になった。以前は usablePool を渡して
// いたので構造上ありえなかったが、いまは exclude への追加を消すと、外した
// 種目がそのまま今日のリストに出る。
//
// incline を「最も長くやっていない種目」にしてある。補助は放置日数の長い
// ものから選ばれるので、候補に入っていれば真っ先に出る。全種目が未実施だと
// 同点は ID 昇順で chest_up_* が先に枠を埋め、incline が候補に居ても出ない。
func TestSessionPlanner_DeselectedExercisesAreNeverCandidates(t *testing.T) {
	req := inclineDeselectedRequest(t)

	// 窓（28日）の外に置く。残差は窓ぶんの目標のまま。
	logs := planHistory(t)
	for i := range 5 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("old-%d", i),
			planMonday.AddDays(-(planning.CoverageWindowDays+2)), fmt.Sprintf("chest_up_%d", i), 30, 10, 2))
	}
	req.History = setlog.NewHistory(logs)

	got := accessoryIDs(mustPlan(t, req))
	if len(got) == 0 {
		t.Fatal("前提が崩れている: 胸上部の補助が1件も出ていない")
	}
	if slices.Contains(got, exercise.ExerciseID("incline")) {
		t.Errorf("選択から外した種目が補助に出ている: %v", got)
	}
}

// カバレッジの窓は直近4週。前日までの27日ぶんを数え、当日を足して28日。
//
// 評価日 E（割り振り器が損失を測る基準日）は「予測の最後の回の日」で、
// 頻度が1より大きいと today より先にずれる（PR 3・
// docs/specs/2026-09-26-accessory-allocation-design.md）。この境界検査は
// 「today から数えて何日前か」を厳密に見たいので、頻度1のプログラムに
// 差し替えて E を today に固定する（頻度1なら horizonDates が返す回は
// 今日1回だけで、ずれが起きない）。
func TestSessionPlanner_RollingCoverageWindow(t *testing.T) {
	base := chestUpperRequest(t)
	freq1, err := base.Program.WithFrequency(mustFrequency(t, 1))
	if err != nil {
		t.Fatalf("WithFrequency(1): %v", err)
	}
	base.Program = freq1
	want := len(mustPlan(t, base).Accessories())

	cases := []struct {
		name string
		// 窓ぶんの目標（12 × 窓の週数）の記録を置く日（月曜からの日数）。
		daysFromMonday int
		// 窓に入っていれば目標を使い切り、補助が減る。
		inWindow bool
	}{
		{
			// 境界。ここを -28 にすると、同じ曜日に通う人は4週前の同じ
			// セッションが常に窓に残り、定常状態で残差がほぼ 0 になって
			// 補助が出なくなる。黙って壊れるので固定する。
			name: "28日前は数えない", daysFromMonday: -planning.CoverageWindowDays, inWindow: false,
		},
		{
			name: "27日前は数える", daysFromMonday: -(planning.CoverageWindowDays - 1), inWindow: true,
		},
		{
			// 窓が1週だったころは窓の外だった。
			name: "7日前は数える", daysFromMonday: -7, inWindow: true,
		},
		{
			name: "3日前は数える", daysFromMonday: -3, inWindow: true,
		},
		{
			// 当日の記録は数えない。含めるとセッション中に残差が動き、
			// こなすたびにリストが入れ替わる。今日の計画はその日の
			// 始まりに確定させると決めた（D-116）。
			name: "当日は数えない", daysFromMonday: 0, inWindow: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := planHistory(t)
			for i := range 12 * planning.CoverageWindowWeeks {
				logs = append(logs, mkLogOn(t, fmt.Sprintf("out-%d", i),
					planMonday.AddDays(c.daysFromMonday), "incline", 30, 10, 2))
			}

			req := base
			req.History = setlog.NewHistory(logs)
			got := len(mustPlan(t, req).Accessories())

			if c.inWindow && got >= want {
				t.Errorf("窓の中の記録が残差に効いていない: 補助が %d 件。%d 件より少ないはず",
					got, want)
			}
			if !c.inWindow && got != want {
				t.Errorf("窓の外の記録が残差に効いている: 補助が %d 件。%d 件のはず",
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
	req.History = setlog.NewHistory(logs)

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
	req.Pool = append([]*exercise.Exercise{nil}, append(planPool(t), nil)...)

	s := mustPlan(t, req)
	if len(s.Main()) != 1 {
		t.Errorf("nil が混ざるとヘビー枠が揃わない: %d", len(s.Main()))
	}
}

// --- 敵対的検証（PR16）で見つかった穴を塞ぐテスト ---

func accessoryIDs(s planning.PlannedSession) []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, 0, len(s.Accessories()))
	for _, a := range s.Accessories() {
		out = append(out, a.ExerciseID())
	}
	return out
}

func mainSet(t *testing.T, s planning.PlannedSession, id exercise.ExerciseID) planning.PlannedSet {
	t.Helper()
	for _, set := range s.Main() {
		if set.ExerciseID() == id {
			return set
		}
	}
	t.Fatalf("%s がメインに無い: %v", id, s.Main())
	return planning.PlannedSet{}
}

func mainWeight(t *testing.T, s planning.PlannedSession, id exercise.ExerciseID) float64 {
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
			req.History = setlog.NewHistory(logs)

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
	req.History = setlog.NewHistory(logs)
	req.Date = planMonday.AddDays(4) // 週3設定の5本目

	s := mustPlan(t, req)
	if len(s.Accessories()) == 0 {
		t.Error("頻度を超えたセッションで補助が1つも出ていない")
	}
}

// RIR 補正は注入したコンディション分析器を使う。
// 既定値を直接呼ぶと、注入した設定（睡眠不足のしきい値など）が無視される。
func TestSessionPlanner_UsesInjectedConditionAnalyzer(t *testing.T) {
	analyzer, err := planning.NewConditionAnalyzer(14, 0.05)
	if err != nil {
		t.Fatalf("分析器の生成に失敗: %v", err)
	}
	planner, err := planning.NewSessionPlanner(
		planning.DefaultOneRepMaxEstimator(),
		planning.DefaultAccessoryAllocator(), analyzer)
	if err != nil {
		t.Fatalf("生成器の生成に失敗: %v", err)
	}

	conditions := []condition.DailyCondition{
		condition.NewDailyCondition(planMonday).WithSleepHours(6.9),
	}
	for i := 1; i <= 14; i++ {
		conditions = append(conditions,
			condition.NewDailyCondition(planMonday.AddDays(-i)).WithSleepHours(7))
	}

	req := planRequest(t)
	req.Conditions = condition.NewConditionLog(conditions)

	injected, err := planner.Plan(req)
	if err != nil {
		t.Fatalf("Plan が失敗: %v", err)
	}
	if got := mainSet(t, injected, "bench").TargetRIR().Int(); got != 2 {
		t.Errorf("注入した分析器の閾値が効いていない: 目標RIR %d", got)
	}

	// 既定の閾値（1.5h）なら 0.1h の不足では補正しない。
	base := mustPlan(t, req)
	if got := mainSet(t, base, "bench").TargetRIR().Int(); got != 1 {
		t.Errorf("既定の分析器で補正が入った: 目標RIR %d", got)
	}
}

// 当日すでに記録したメイン種目を、計画ぶんと二重に数えない。
// 二重に数えると、そのセッションの途中から補助が消える。
func TestSessionPlanner_DoesNotDoubleCountTodaysMain(t *testing.T) {
	pool := []*exercise.Exercise{
		mainExercise(t, "bench",
			map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mkAccessory(t, "fly", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mkAccessory(t, "press", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
	}
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 8})
	program, err := program.NewProgram(mustFrequency(t, 1), planVolume(t),
		[]exercise.ExerciseID{"bench", "fly", "press"}, []exercise.ExerciseID{"bench"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	logs := []*setlog.SetLog{
		mkLogOn(t, "b0", planMonday.AddDays(-7), "bench", 85, 8, 2),
	}
	for i := range 4 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("bt-%d", i),
			planMonday, "bench", 85, 8, 2))
	}

	s := mustPlan(t, planning.PlanRequest{
		Program: program, Target: target, Pool: pool,
		History:    setlog.NewHistory(logs),
		Conditions: condition.NewConditionLog(nil),
		Date:       planMonday,
	})
	if len(s.Accessories()) == 0 {
		t.Errorf("当日のメインを二重計上して補助が消えた: 週目標8、実施4セット")
	}
}

// plannedWeight は今日のメニューのうち id の提示重量を返す。
// メインと補助のどちらにあっても引ける。
func plannedWeight(t *testing.T, s planning.PlannedSession, id exercise.ExerciseID) (float64, bool) {
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
		exercise exercise.ExerciseID
		// 基準の提示重量を確定させるために要る履歴。補助種目は過去の記録が
		// 無いと重量が出ないので、そのぶんを先に置く。
		prior func(t *testing.T) []*setlog.SetLog
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
			prior: func(t *testing.T) []*setlog.SetLog {
				t.Helper()
				return []*setlog.SetLog{
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
			req.History = setlog.NewHistory(logs)

			before, ok := plannedWeight(t, mustPlan(t, req), c.exercise)
			if !ok {
				t.Fatalf("前提: %s の重量が提示されること", c.exercise)
			}

			req.History = setlog.NewHistory(append(logs,
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
	// 宣言をベンチ1つに絞る。3つあると、今日ベンチを記録した時点で
	// 最終実施日が最も新しくなり、翌週のヘビー枠が別の種目に移る。
	req := planRequestAt(t, 0)
	base := mainWeight(t, mustPlan(t, req), "bench")

	// 今日、推定を押し上げる内容で記録する。
	req.History = setlog.NewHistory(append(planHistory(t),
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
	target, err := seed.DefaultWeeklyTarget(freq, mustVolume(t, 6, 3))
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	program, err := program.NewProgram(freq, planVolume(t), selected, big3(), "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}

	planner := planning.DefaultSessionPlanner()
	date := planMonday
	var logs []*setlog.SetLog

	plan := func(t *testing.T) planning.PlannedSession {
		t.Helper()
		s, err := planner.Plan(planning.PlanRequest{
			Program: program, Target: target, Pool: pool,
			History:    setlog.NewHistory(logs),
			Conditions: condition.NewConditionLog(nil),
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

	done := map[exercise.ExerciseID]int{}
	recorded := 0
	for step := range planned * 3 {
		s := plan(t)
		var todo *planning.PlannedSet
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
// 契約ではなく偶然だったため（旧 AccessorySelector.Select の時代の話。
// 呼び出し側のソートを逆順にする変異を入れても検査は緑のまま通った）。
//
// いまの AccessoryAllocator が返す順序の契約は「損失（ΔL）を貪欲に確定した
// 順」であって、昇順ではない。同点は最終実施日・空き枠・日付・種目IDの順で
// 崩す（accessory_allocator.go の Allocate のコメント参照）。偶然を固定
// すると、優先度の付け方を変えたときに理由の無い赤が出る。
func TestSessionPlanner_AccessoriesComeBackInAStableOrder(t *testing.T) {
	req := planRequest(t)

	first := mustPlan(t, req)
	ids := make([]exercise.ExerciseID, 0, len(first.Accessories()))
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
func chinRequest(t *testing.T, addedKg, bodyweight float64) planning.PlanRequest {
	t.Helper()

	chin := mustExercise(t, exercise.ExerciseParams{
		ID: "chin", Name: "chin",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})

	target := mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12})
	program, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "chin"},
		big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// 3セッションぶん記録する。推定1RMが立つ量。
	logs := planHistory(t)
	var conds []condition.DailyCondition
	for i, daysAgo := range []int{21, 14, 7} {
		day := planMonday.AddDays(-daysAgo)
		for set := range 3 {
			logs = append(logs, mkLogOn(t,
				fmt.Sprintf("chin-%d-%d", i, set), day, "chin", addedKg, 8, 2))
		}
		if bodyweight > 0 {
			conds = append(conds,
				condition.NewDailyCondition(day).WithBodyWeight(bodyweight))
		}
	}

	return planning.PlanRequest{
		Program:    program,
		Target:     target,
		Pool:       append(planPool(t), chin),
		History:    setlog.NewHistory(logs),
		Conditions: condition.NewConditionLog(conds),
		Date:       planMonday,
	}
}

// accessorySet は補助種目のうち id のものを返す。
func accessorySet(t *testing.T, s planning.PlannedSession, id exercise.ExerciseID) planning.PlannedSet {
	t.Helper()
	for _, set := range s.Accessories() {
		if set.ExerciseID() == id {
			return set
		}
	}
	t.Fatalf("前提: %s が補助として提示されること", id)
	return planning.PlannedSet{}
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
	chin := mustExercise(t, exercise.ExerciseParams{
		ID: "chin", Name: "chin",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})
	pool := append(planPool(t), chin)
	ids := []exercise.ExerciseID{"bench", "squat", "deadlift", "chin"}
	for i := range 5 {
		id := fmt.Sprintf("lat_%d", i)
		pool = append(pool, mkAccessory(t, id,
			map[training.MuscleRegion]float64{training.Lat: 1.0}))
		ids = append(ids, exercise.ExerciseID(id))
	}

	target := mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12})
	program, err := program.NewProgram(mustFrequency(t, 3), planVolume(t), ids, big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// 週の半ばを対象日にして、その手前に記録を置けるようにする。
	date := planMonday.AddDays(2)
	base := planning.PlanRequest{
		Program: program, Target: target, Pool: pool,
		History:    setlog.NewHistory(planHistory(t)),
		Conditions: condition.NewConditionLog(nil),
		Date:       date,
	}
	want := len(mustPlan(t, base).Accessories())
	if want == 0 {
		t.Fatal("前提: 補助が提示されること")
	}

	// 窓の中ですでに自重で窓ぶん（12 × 窓の週数）こなした。ただし体重は一度も
	// 測っていないので、実効負荷が出せず、推定用の履歴からは落ちる。
	logs := planHistory(t)
	for i := range 12 * planning.CoverageWindowWeeks {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("chin-%d", i),
			planMonday, "chin", 0, 8, 2))
	}
	req := base
	req.History = setlog.NewHistory(logs)

	got := len(mustPlan(t, req).Accessories())
	if got >= want {
		t.Errorf("自重のセットが残差に反映されていない: %d → %d", want, got)
	}
}

// 自重種目の提示は加重で出す。体重込みの総負荷を見せられても、
// 何をすればいいか分からない。
func TestSessionPlanner_BodyweightExerciseFallsBackToDefaultBodyWeight(t *testing.T) {
	chin := mustExercise(t, exercise.ExerciseParams{
		ID: "chin", Name: "chin",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})

	pool := append(planPool(t), chin)
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12})
	program, err := program.NewProgram(mustFrequency(t, 3), planVolume(t),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "chin"},
		big3(), "")
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

	s := mustPlan(t, planning.PlanRequest{
		Program:    program,
		Target:     target,
		Pool:       pool,
		History:    setlog.NewHistory(logs),
		Conditions: condition.NewConditionLog(nil),
		Date:       planMonday,
	})

	found := false
	for _, set := range s.Accessories() {
		if set.ExerciseID() != exercise.ExerciseID("chin") {
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

// 1日の種目数が予算を超えないこと。
//
// 予算は利用者の設定（1回の種目数 × 1種目あたりのセット数）。以前は上限が
// AccessorySelector の maxSlots = 8 という定数で、軸を足した9種目27セットが
// 全頻度・全セッションで固定的に出ていた。頻度を上げても1日は短くならず、
// 週7回なら週189セットになる。それでも「1セッション9〜36セット」の検査は
// 通っていた（週の合計を誰も見ていなかった）。
//
// 予算を超えないことだけを見る。届かない日は正当にある（補助が全部回復
// 期間に当たる、分割で狙う区分が尽きる）ので、下回ることは責めない。
func TestSessionPlanner_ExerciseCountNeverExceedsBudget(t *testing.T) {
	pool := planPool(t)
	ids := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		ids = append(ids, e.ID())
	}
	planner := planning.DefaultSessionPlanner()

	for exercises := 2; exercises <= 6; exercises++ {
		for sets := 2; sets <= 6; sets++ {
			volume, err := program.NewSessionVolume(exercises, sets)
			if err != nil {
				t.Fatalf("1回の量が不正: %v", err)
			}
			target := mustTarget(t, map[training.MuscleRegion]float64{
				training.ChestMid: 12, training.ChestUpper: 9,
				training.Quad: 12, training.Biceps: 9,
			})
			prog, err := program.NewProgram(mustFrequency(t, 3), volume,
				ids, big3(), "")
			if err != nil {
				t.Fatalf("プログラムの生成に失敗: %v", err)
			}

			got, err := planner.Plan(planning.PlanRequest{
				Program: prog, Target: target, Pool: pool,
				History: setlog.NewHistory(nil), Date: today(),
			})
			if err != nil {
				t.Fatalf("%d種目×%dセット: 計画に失敗: %v", exercises, sets, err)
			}

			n := len(got.Main()) + len(got.Variation()) + len(got.Accessories())
			if n > exercises {
				t.Errorf("%d種目×%dセット: %d種目が出た（予算%d）",
					exercises, sets, n, exercises)
			}
			// セット数は全レーンで利用者の設定。補助だけでなく軸と
			// バリエーションも見る。
			lanes := map[string][]planning.PlannedSet{
				"軸": got.Main(), "バリエーション": got.Variation(), "補助": got.Accessories(),
			}
			for lane, planned := range lanes {
				for _, s := range planned {
					if s.Sets().Int() != sets {
						t.Errorf("%d種目×%dセット: %sが%dセット", exercises, sets, lane, s.Sets().Int())
					}
				}
			}
		}
	}
}
