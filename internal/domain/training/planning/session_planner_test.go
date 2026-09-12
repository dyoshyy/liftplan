package planning_test

import (
	"fmt"
	"slices"
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

func planProgram(t *testing.T) *program.Program {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), target,
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl"}, big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
}

// focusedProgram は重点種目を指定したプログラムを返す。
//
// 派生（larsen・tempo）も選択に入れる。選択されていない種目は usablePool から
// 落ちるので、バリエーションレーンの候補にもならない。
func focusedProgram(t *testing.T, focus exercise.ExerciseID) *program.Program {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), target,
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl", "larsen", "tempo"},
		big3(), focus)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
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
	return planning.PlanRequest{
		Program:    planProgram(t),
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
	p, err := program.NewProgram(mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 30}),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl", "larsen"},
		big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Program = p
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
func benchOnlyProgram(t *testing.T) *program.Program {
	t.Helper()

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.ChestUpper: 9, training.Quad: 12, training.Biceps: 9,
	})
	p, err := program.NewProgram(mustFrequency(t, 3), target,
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "incline", "curl"},
		[]exercise.ExerciseID{"bench"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return p
}

// planRequestAt は、ベンチを今週すでに done 回やった状態で days 日目の
// リクエストを返す。宣言はベンチ1つに絞る。
//
// スロットの役割は「その種目にとって今週何本目か」で決まる（liftIndexInWeek）。
// 週の通算本数ではないので、他の種目で日数を進めても役割は動かない。
//
// 以前は curl で週を進めていた。ヘビー枠が1つになる前は、週の通算本数が
// 役割を決めていたため。宣言が3つあれば週3回通ってもベンチは週1回しか
// 出ないので、通算で引くとその1回に「週3本目＝軽い日」が当たる（D-117）。
func planRequestAt(t *testing.T, days int, done ...int) planning.PlanRequest {
	t.Helper()

	logs := planHistory(t)
	for i, d := range done {
		logs = append(logs,
			mkLogOn(t, fmt.Sprintf("done-%d", i), planMonday.AddDays(d), "bench", 85, 8, 2))
	}

	req := planRequest(t)
	req.Program = benchOnlyProgram(t)
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
func TestSessionPlanner_LightIntentKeepsTheSameExercise(t *testing.T) {
	s := mustPlan(t, planRequestAt(t, 4, 0, 2)) // 週3本目 = 軽い日

	found := false
	for _, set := range s.Main() {
		if set.ExerciseID() == exercise.ExerciseID("larsen") {
			t.Error("軽い日で種目が差し替わっている")
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
		// 役割はメインのスロットにだけ付く。補助に付くと、強度帯が
		// 二重に適用される。
		if _, ok := set.Intent(); ok {
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
	program, err := program.NewProgram(mustFrequency(t, 3), target,
		[]exercise.ExerciseID{"curl"}, []exercise.ExerciseID{"curl"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	req := planRequest(t)
	req.Program = program

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
	slots := planning.NewPrescriptionCatalog()
	est := planning.DefaultOneRepMaxEstimator()
	acc := planning.DefaultAccessorySelector()
	analyzer := planning.DefaultConditionAnalyzer()

	cases := []struct {
		name string
		call func() error
	}{
		{"推定器", func() error {
			_, err := planning.NewSessionPlanner(slots, planning.OneRepMaxEstimator{}, acc, analyzer)
			return err
		}},
		{"補助の選択器", func() error {
			_, err := planning.NewSessionPlanner(slots, est, planning.AccessorySelector{}, analyzer)
			return err
		}},
		{"コンディション分析器", func() error {
			_, err := planning.NewSessionPlanner(slots, est, acc, planning.ConditionAnalyzer{})
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

	if _, err := planning.NewSessionPlanner(slots, est, acc, analyzer); err != nil {
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

	// 月曜に大胸筋上部の週目標（9セット）を全部こなしたことにする。
	logs := planHistory(t)
	for i := range 9 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("inc-%d", i),
			planMonday, "incline", 30, 10, 2))
	}
	req.History = setlog.NewHistory(logs)
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
	req.History = setlog.NewHistory(base)

	first := mustPlan(t, req)
	want := lineup(first)
	if len(want) == 0 {
		t.Fatal("前提: 種目が1つも出ていない")
	}

	// 提示されたとおりに1セットずつ記録しては、開き直す。
	logs := append([]*setlog.SetLog{}, base...)
	n := 0
	for _, set := range append(first.Main(), first.Accessories()...) {
		for range set.Sets().Int() {
			n++
			logs = append(logs, mkLogOn(t, fmt.Sprintf("d%03d", n), req.Date,
				string(set.ExerciseID()), 40, 8, 2))

			req.History = setlog.NewHistory(logs)
			got := lineup(mustPlan(t, req))
			if !slices.Equal(got, want) {
				t.Fatalf("%dセット記録した時点で計画が変わった\n  最初: %v\n  いま: %v",
					n, want, got)
			}
		}
	}
}

// lineup は提示された種目を並び順のまま返す。
func lineup(s planning.PlannedSession) []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, 0, len(s.Main())+len(s.Accessories()))
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
	req.History = setlog.NewHistory(logs)

	reopened := mustPlan(t, req)
	if len(reopened.Accessories()) != len(base.Accessories()) {
		t.Errorf("当日の記録で補助の数が変わった: %d → %d",
			len(base.Accessories()), len(reopened.Accessories()))
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

	// 週目標3・頻度3。ベンチが1セッションで3セット埋めるので、
	// メインの刺激を差し引けば残差は0になる。
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 3})
	program, err := program.NewProgram(mustFrequency(t, 3), target,
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "pec_fly"}, big3(), "")
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

	program, err := program.NewProgram(mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.ChestUpper: 12}),
		ids, big3(), "")
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
	req.History = setlog.NewHistory(logs)
	req.Date = planMonday.AddDays(4)

	last := mustPlan(t, req)
	if len(last.Accessories()) <= len(first.Accessories()) {
		t.Errorf("残りセッション数で割っていない: 1本目 %d → 3本目 %d",
			len(first.Accessories()), len(last.Accessories()))
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
			req.History = setlog.NewHistory(logs)

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
	analyzer, err := planning.NewConditionAnalyzer(14, 0.05, 21)
	if err != nil {
		t.Fatalf("分析器の生成に失敗: %v", err)
	}
	planner, err := planning.NewSessionPlanner(planning.NewPrescriptionCatalog(),
		planning.DefaultOneRepMaxEstimator(),
		planning.DefaultAccessorySelector(), analyzer)
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
	program, err := program.NewProgram(mustFrequency(t, 1),
		mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 8}),
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
		Program: program, Pool: pool,
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
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	program, err := program.NewProgram(freq, target, selected, big3(), "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}

	planner := planning.DefaultSessionPlanner()
	date := planMonday
	var logs []*setlog.SetLog

	plan := func(t *testing.T) planning.PlannedSession {
		t.Helper()
		s, err := planner.Plan(planning.PlanRequest{
			Program: program, Pool: pool,
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

	program, err := program.NewProgram(
		mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12}),
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

	program, err := program.NewProgram(mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12}), ids, big3(), "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	// 週の半ばを対象日にして、その手前に記録を置けるようにする。
	date := planMonday.AddDays(2)
	base := planning.PlanRequest{
		Program: program, Pool: pool,
		History:    setlog.NewHistory(planHistory(t)),
		Conditions: condition.NewConditionLog(nil),
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
	program, err := program.NewProgram(
		mustFrequency(t, 3),
		mustTarget(t, map[training.MuscleRegion]float64{training.Lat: 12}),
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
