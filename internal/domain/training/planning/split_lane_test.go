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

// splitPool は上半身と下半身がはっきり分かれたプールを返す。
//
// planPool はベンチ系に寄っていて、下半身の補助が足りない。分割の
// 検査には「その日に選べる補助が両側にある」ことが要る。
func splitPool(t *testing.T) []*exercise.Exercise {
	t.Helper()
	pool := []*exercise.Exercise{
		// 三頭は副次（0.5）。主働の閾値を下げると、腕の日にベンチが
		// 軸として出てしまう。
		mainExercise(t, "bench", map[training.MuscleRegion]float64{
			training.ChestMid: 1.0, training.TricepsLateral: 0.5,
		}),
		mainExercise(t, "squat", map[training.MuscleRegion]float64{training.Quad: 1.0}),
	}
	for i := range 6 {
		pool = append(pool, mkAccessory(t, fmt.Sprintf("up_%d", i),
			map[training.MuscleRegion]float64{training.ChestMid: 1.0}))
		pool = append(pool, mkAccessory(t, fmt.Sprintf("lo_%d", i),
			map[training.MuscleRegion]float64{training.Quad: 1.0}))
	}
	return pool
}

func splitProgram(t *testing.T, cycle ...program.Split) *program.Program {
	t.Helper()
	ids := []exercise.ExerciseID{"bench", "squat"}
	for i := range 6 {
		ids = append(ids, exercise.ExerciseID(fmt.Sprintf("up_%d", i)),
			exercise.ExerciseID(fmt.Sprintf("lo_%d", i)))
	}
	p, err := program.NewProgram(mustFrequency(t, 2), planVolume(t), mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 24, training.Quad: 24,
	}),
		ids, []exercise.ExerciseID{"bench", "squat"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	if len(cycle) == 0 {
		return p
	}
	p, err = p.WithCycle(cycle)
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}
	return p
}

func mkSplit(t *testing.T, name string, regions ...training.MuscleRegion) program.Split {
	t.Helper()
	s, err := program.NewSplit(name, regions)
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}
	return s
}

func splitRequest(t *testing.T, prog *program.Program) planning.PlanRequest {
	t.Helper()
	req := planRequest(t)
	req.Pool = splitPool(t)
	req.Program = prog
	req.History = setlog.NewHistory(nil)
	return req
}

// 分割を設定すると、その日の区分だけが補助に出ること。
func TestSessionPlanner_AccessoriesStayInsideTheSplit(t *testing.T) {
	upper := mkSplit(t, "上", training.ChestMid)
	lower := mkSplit(t, "下", training.Quad)

	req := splitRequest(t, splitProgram(t, upper, lower))
	got := mustPlan(t, req)

	// 出席0回なので周期の先頭＝上の日。
	if len(got.Main()) != 1 || got.Main()[0].ExerciseID() != "bench" {
		t.Fatalf("軸がベンチでない: %v", got.Main())
	}
	for _, a := range got.Accessories() {
		if string(a.ExerciseID())[:2] != "up" {
			t.Errorf("上の日に下半身の補助が出ている: %v", accessoryIDs(got))
		}
	}
	if len(got.Accessories()) == 0 {
		t.Error("補助が1件も出ていない")
	}
}

// 周期は出席回数で進むこと。暦では進めない。
//
// 暦で進めると、休んだ日に分割だけが回る。通っていないのに
// 「昨日は上の日だった」ことになる。
func TestSessionPlanner_CycleAdvancesByAttendance(t *testing.T) {
	upper := mkSplit(t, "上", training.ChestMid)
	lower := mkSplit(t, "下", training.Quad)
	prog := splitProgram(t, upper, lower)

	cases := []struct {
		name string
		// 記録を置く日（月曜からの日数）。1日につき1セッション。
		done []int
		// 対象日（月曜からの日数）
		date int
		want exercise.ExerciseID
	}{
		{"1回も通っていなければ周期の先頭", nil, 0, "bench"},
		{"1回通えば2日目", []int{-1}, 0, "squat"},
		{"2回通えば折り返して先頭", []int{-2, -1}, 0, "bench"},
		{
			// 日付は5日進むが、通っていないので周期は動かない。
			name: "通わなければ日付が進んでも動かない",
			done: nil, date: 5, want: "bench",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var logs []*setlog.SetLog
			for i, d := range c.done {
				logs = append(logs, mkLogOn(t, fmt.Sprintf("s-%d", i),
					planMonday.AddDays(d), "bench", 60, 8, 2))
			}

			req := splitRequest(t, prog)
			req.History = setlog.NewHistory(logs)
			req.Date = planMonday.AddDays(c.date)

			got := mustPlan(t, req)
			if len(got.Main()) != 1 {
				t.Fatalf("軸が1つでない: %v", got.Main())
			}
			if got.Main()[0].ExerciseID() != c.want {
				t.Errorf("軸が %v。%v のはず", got.Main()[0].ExerciseID(), c.want)
			}
		})
	}
}

// その日の区分を主働に含む宣言が無ければ、軸は空になること。
//
// 5分割の肩・腕がこれ。BIG3 の中に主働を持つ種目が無い。0.88 の
// スクワットを肩の日に出すより、軸の枠が無いほうが正直。
func TestSessionPlanner_AxisIsEmptyWhenNoDeclaredFitsTheDay(t *testing.T) {
	// 腕の日。ベンチは三頭に 0.5 寄与するが主働ではないので軸にならない。
	// 閾値を下げるとここでベンチが軸に出る。
	arms := mkSplit(t, "腕", training.TricepsLateral)

	pool := append(splitPool(t),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.TricepsLateral: 1.0}))
	ids := []exercise.ExerciseID{"bench", "squat", "curl"}
	for i := range 6 {
		ids = append(ids, exercise.ExerciseID(fmt.Sprintf("up_%d", i)),
			exercise.ExerciseID(fmt.Sprintf("lo_%d", i)))
	}
	prog, err := program.NewProgram(mustFrequency(t, 2), planVolume(t), mustTarget(t, map[training.MuscleRegion]float64{
		// 三頭の目標を胸・脚より大きく置く。1日の枠が有限なので、目標の
		// 小さい区分は枠を取り合って負け、「ゲートを通っているのに一度も
		// 選ばれない」と「ゲートで落ちている」が見分けられなくなる。
		training.ChestMid: 6, training.Quad: 6, training.TricepsLateral: 24,
	}),
		ids, []exercise.ExerciseID{"bench", "squat"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	prog, err = prog.WithCycle([]program.Split{arms})
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}

	req := splitRequest(t, prog)
	req.Pool = pool

	got := mustPlan(t, req)
	if len(got.Main()) != 0 {
		t.Errorf("軸が空でない: %v", got.Main())
	}
	// 補助は出る。軸が無いだけでセッションが消えるわけではない。
	if len(got.Accessories()) == 0 {
		t.Error("補助が1件も出ていない")
	}
	if !containsAccessory(got, "curl") {
		t.Errorf("腕の日に腕の補助が出ていない: %v", accessoryIDs(got))
	}

	// 元は「腕の日には curl しか出ない」を検査していた。やめた理由は、
	// この周期が1日しか無く、胸と脚が**どの日にも属さない**区分になる
	// ため。属さない区分は毎日活きるので、ここに出るのが正しい
	// （出ないほうが壊れている。腹が永久に埋まらないのと同じ形）。
	//
	// 「その日の分割に属する区分だけを狙う」は
	// TestSessionPlanner_AffiliatedRegionsStayInsideTheirDay が守る。
	// あちらは周期が2日あり、胸も脚もどこかの日に属している。
	if !containsAccessory(got, "up_0") {
		t.Errorf("どの日にも属さない区分が落ちている: %v", accessoryIDs(got))
	}
}

// 分割を設定していなければ、挙動は今までどおりであること。
func TestSessionPlanner_NoSplitKeepsEverything(t *testing.T) {
	req := splitRequest(t, splitProgram(t))
	got := mustPlan(t, req)

	if len(got.Main()) != 1 {
		t.Fatalf("軸が1つでない: %v", got.Main())
	}
	up, lo := 0, 0
	for _, a := range got.Accessories() {
		switch string(a.ExerciseID())[:2] {
		case "up":
			up++
		case "lo":
			lo++
		}
	}
	if up == 0 || lo == 0 {
		t.Errorf("分割なしなのに片側しか出ていない: %v", accessoryIDs(got))
	}
}

// 宣言がプールに1つも無いのは設定の破れ。分割で絞られてゼロになるのとは
// 別物で、こちらは計画を出さずに止める。
func TestSessionPlanner_StillRejectsAnEmptyPool(t *testing.T) {
	req := splitRequest(t, splitProgram(t))
	req.Pool = nil

	if _, err := planning.DefaultSessionPlanner().Plan(req); err == nil {
		t.Error("プールが空でも計画が出る")
	}
}

// 同じ区分の日が週に2回あるとき、1日目が週の目標を使い切らないこと。
//
// 天井が無いと、最初の下半身の日が週の下半身目標を丸ごと出し（実測38.1）、
// 次の下半身の日が6セットまで落ちる。週の量をその区分が出る日数で割る。
func TestSessionPlanner_SplitSpreadsTheWeeklyDoseAcrossItsDays(t *testing.T) {
	lower := mkSplit(t, "下", training.Quad)

	// 週2回とも下半身の日。大腿四頭筋は週2回狙われる。
	prog := splitProgram(t, lower, lower)

	req := splitRequest(t, prog)
	first := mustPlan(t, req)
	if len(first.Accessories()) == 0 {
		t.Fatal("1日目に補助が出ていない")
	}

	// 1日目をこなした記録を積む。
	var logs []*setlog.SetLog
	n := 0
	for _, set := range append(first.Main(), first.Accessories()...) {
		for range set.Sets().Int() {
			n++
			logs = append(logs, mkLogOn(t, fmt.Sprintf("d1-%d", n),
				planMonday, string(set.ExerciseID()), 60, 8, 2))
		}
	}

	// 中2日空ける。連日に置くと筋区分の回復判定で補助がゼロになり、
	// 天井の効果を見られない。回復は生理なので分割より優先する。
	req.History = setlog.NewHistory(logs)
	req.Date = planMonday.AddDays(3)
	second := mustPlan(t, req)

	if len(second.Accessories()) == 0 {
		t.Errorf("2日目の補助が空。1日目が週の量を使い切っている（1日目 %d 件）",
			len(first.Accessories()))
	}
}

// 筋区分の回復判定は分割に優先すること。
//
// 同じ区分の日を連日に置くと、前日に刺激した区分が回復中で残差から
// 落ちるので補助が出ない。分割の並びで殴っても生理は変わらない、
// というのが決めごと。周期の組み方の問題として扱う。
func TestSessionPlanner_RecoveryBeatsTheSplit(t *testing.T) {
	lower := mkSplit(t, "下", training.Quad)
	req := splitRequest(t, splitProgram(t, lower, lower))

	first := mustPlan(t, req)
	if len(first.Accessories()) == 0 {
		t.Fatal("1日目に補助が出ていない")
	}

	var logs []*setlog.SetLog
	n := 0
	for _, set := range append(first.Main(), first.Accessories()...) {
		for range set.Sets().Int() {
			n++
			logs = append(logs, mkLogOn(t, fmt.Sprintf("d1-%d", n),
				planMonday, string(set.ExerciseID()), 60, 8, 2))
		}
	}

	req.History = setlog.NewHistory(logs)
	req.Date = planMonday.AddDays(1)

	second := mustPlan(t, req)
	if len(second.Accessories()) != 0 {
		t.Errorf("連日で同じ区分の補助が出ている: %v", accessoryIDs(second))
	}
	// 軸は出る。回復判定は補助の選択にしか効かない。
	if len(second.Main()) != 1 {
		t.Errorf("軸が消えている: %v", second.Main())
	}
}

// その区分が週に1日しか来なければ、その日に週のぶんを出すこと。
//
// 天井は「週目標 ÷ その区分が出る日数」。今日の分割だけを見て数えると、
// 上下2分割なら常に頻度と同じ回数になり、週1日しか無い区分まで
// 半分に割られる。周期を今日の位置から頻度ぶん歩いて数える必要がある。
func TestSessionPlanner_CapCountsTheWholeCycle(t *testing.T) {
	upper := mkSplit(t, "上", training.ChestMid)
	lower := mkSplit(t, "下", training.Quad)

	// 週2回・上下1日ずつ。大腿四頭筋が来るのは週1日だけ。
	prog := splitProgram(t, upper, lower)

	// 1回通った状態にして、周期を下の日へ進める。
	req := splitRequest(t, prog)
	req.History = setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "w0", planMonday.AddDays(-4), "bench", 60, 8, 2),
	})

	got := mustPlan(t, req)
	if len(got.Main()) != 1 || got.Main()[0].ExerciseID() != "squat" {
		t.Fatalf("前提: 下の日であること: %v", got.Main())
	}

	// 週目標24・軸が3セット。週1日しか無いので残り21セットぶんを
	// この日に出す。半分に割られると3件まで落ちる。
	if n := len(got.Accessories()); n < 5 {
		t.Errorf("補助が %d 件。週のぶんを出していない: %v", n, accessoryIDs(got))
	}
}

// 軸が空の日でもバリエーションの判定が落ちないこと。
//
// 軸が空になる経路を足したとき、系統の重複を見る門が軸のIDを引いていた。
// 分割に該当する宣言が無い日に重点種目が設定されていると落ちる。
func TestSessionPlanner_VariationSurvivesAnEmptyAxis(t *testing.T) {
	arms := mkSplit(t, "腕", training.TricepsLateral)

	pool := append(splitPool(t),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.TricepsLateral: 1.0}),
		mustExercise(t, exercise.ExerciseParams{
			ID: "larsen", Name: "larsen",
			Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
			IncrementKg: 2.5, DerivedFrom: "bench",
		}))
	ids := []exercise.ExerciseID{"bench", "squat", "curl", "larsen"}
	for i := range 6 {
		ids = append(ids, exercise.ExerciseID(fmt.Sprintf("up_%d", i)),
			exercise.ExerciseID(fmt.Sprintf("lo_%d", i)))
	}

	// 重点はベンチ。腕の日に軸は空になる。
	prog, err := program.NewProgram(mustFrequency(t, 2), planVolume(t), mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 24, training.Quad: 24, training.TricepsLateral: 12,
	}),
		ids, []exercise.ExerciseID{"bench", "squat"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	prog, err = prog.WithCycle([]program.Split{arms})
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}

	req := splitRequest(t, prog)
	req.Pool = pool

	// 落ちないこと自体が検査。
	got := mustPlan(t, req)
	if len(got.Main()) != 0 {
		t.Errorf("軸が空でない: %v", got.Main())
	}
}

// 分割があれば、重点種目の主働が今日の集合に含まれる日だけバリエーションが出る。
//
// 止めないと、型が「今日は脚の日」と言いながらベンチの派生が出る。
func TestSessionPlanner_VariationStaysInsideTheSplit(t *testing.T) {
	pool, ids := variationSplitPool(t)
	lower := mkSplit(t, "下", training.Quad)
	upper := mkSplit(t, "上", training.ChestMid, training.Lat)

	// ベンチを3日前にやっておく。分割が無いときの軸がスクワットになる。
	logs := []*setlog.SetLog{
		mkLogOn(t, "b0", planMonday.AddDays(-3), "bench", 60, 8, 2),
		mkLogOn(t, "r0", planMonday.AddDays(-2), "row", 60, 8, 2),
	}

	req := splitRequest(t, variationProgram(t, ids, lower, upper))
	req.Pool = pool
	req.History = setlog.NewHistory(logs)

	onLower := mustPlan(t, req)
	if len(onLower.Main()) != 1 || onLower.Main()[0].ExerciseID() != "squat" {
		t.Fatalf("前提: 下の日であること: %v", onLower.Main())
	}
	if len(onLower.Variation()) != 0 {
		t.Errorf("下の日にベンチの派生が出ている: %v", onLower.Variation())
	}

	// 分割が無ければ、同じ状況でも出る。止めているのが分割だと分かる。
	req.Program = variationProgram(t, ids)
	noSplit := mustPlan(t, req)
	if len(noSplit.Main()) != 1 || noSplit.Main()[0].ExerciseID() != "squat" {
		t.Fatalf("前提: 軸がスクワットであること: %v", noSplit.Main())
	}
	if len(noSplit.Variation()) != 1 {
		t.Errorf("分割なしでバリエーションが出ていない: %v", noSplit.Variation())
	}
}

// 重点種目の日にはバリエーションが出ること。止めすぎていないことを見る。
//
// 上の日の軸がベンチだとバリエーションは元々出ない（同じ系統が1日に
// 二度来る）。軸が別の上半身種目になる日を作る。
func TestSessionPlanner_VariationAppearsOnItsOwnDay(t *testing.T) {
	pool, ids := variationSplitPool(t)
	upper := mkSplit(t, "上", training.ChestMid, training.Lat)

	req := splitRequest(t, variationProgram(t, ids, upper))
	req.Pool = pool
	// ベンチを3日前、ロウを9日前。軸はロウになる。
	req.History = setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "r0", planMonday.AddDays(-9), "row", 60, 8, 2),
		mkLogOn(t, "b0", planMonday.AddDays(-3), "bench", 60, 8, 2),
	})

	got := mustPlan(t, req)
	if len(got.Main()) != 1 || got.Main()[0].ExerciseID() != "row" {
		t.Fatalf("前提: 軸がロウであること: %v", got.Main())
	}
	if len(got.Variation()) != 1 {
		t.Error("重点種目の日にバリエーションが出ていない")
	}
}

// variationSplitPool はバリエーションの検査用に、上半身の宣言を2つ持つ
// プールと選択IDを返す。
func variationSplitPool(t *testing.T) ([]*exercise.Exercise, []exercise.ExerciseID) {
	t.Helper()
	pool := append(splitPool(t),
		mainExercise(t, "row", map[training.MuscleRegion]float64{training.Lat: 1.0}),
		mustExercise(t, exercise.ExerciseParams{
			ID: "larsen", Name: "larsen",
			Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
			IncrementKg: 2.5, DerivedFrom: "bench",
		}))
	ids := []exercise.ExerciseID{"bench", "squat", "row", "larsen"}
	for i := range 6 {
		ids = append(ids, exercise.ExerciseID(fmt.Sprintf("up_%d", i)),
			exercise.ExerciseID(fmt.Sprintf("lo_%d", i)))
	}
	return pool, ids
}

// variationProgram は重点をベンチにしたプログラムを返す。
func variationProgram(t *testing.T, ids []exercise.ExerciseID, cycle ...program.Split) *program.Program {
	t.Helper()
	p, err := program.NewProgram(mustFrequency(t, 2), planVolume(t), mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 24, training.Quad: 24, training.Lat: 18,
	}),
		ids, []exercise.ExerciseID{"bench", "squat", "row"}, "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	if len(cycle) == 0 {
		return p
	}
	p, err = p.WithCycle(cycle)
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}
	return p
}

// どの日にも属さない区分は、分割を設定していても毎日活きること。
//
// 腹はどの日にやってもよい部位で、どのプリセットにも入っていない。
// 「今日の分割に無い区分は狙わない」を素直に適用すると、腹は永久に
// 埋まらない（実測で腹斜筋の達成率が全プリセット・全頻度で 0%）。
//
// 全部の集合に区分を書かせる案は採らない。書き忘れた区分が黙って
// 死ぬので、書き忘れが仕様違反ではなく事故になる（2026-09-19 の仕様）。
func TestSessionPlanner_UnaffiliatedRegionsStayActive(t *testing.T) {
	upper := mkSplit(t, "上", training.ChestMid)
	lower := mkSplit(t, "下", training.Quad)

	req := unaffiliatedRequest(t, upper, lower)
	for i, want := range []string{"上", "下"} {
		req.Date = planMonday.AddDays(i * 2)
		req.History = setlog.NewHistory(sessionsBefore(t, i))

		got := mustPlan(t, req)
		abs := 0
		for _, a := range got.Accessories() {
			if a.ExerciseID() == "crunch" {
				abs++
			}
		}
		if abs == 0 {
			t.Errorf("%sの日に腹が1つも出ていない: %v", want, accessoryIDs(got))
		}
	}
}

// 分割に属する区分は、その日以外では狙わないこと。
//
// 上のテストだけだと「全部の区分を毎日活かす」実装でも緑になる。
func TestSessionPlanner_AffiliatedRegionsStayInsideTheirDay(t *testing.T) {
	upper := mkSplit(t, "上", training.ChestMid)
	lower := mkSplit(t, "下", training.Quad)

	req := unaffiliatedRequest(t, upper, lower)
	req.Date = planMonday.AddDays(2)
	req.History = setlog.NewHistory(sessionsBefore(t, 1)) // 下の日

	for _, a := range mustPlan(t, req).Accessories() {
		if id := string(a.ExerciseID()); len(id) >= 2 && id[:2] == "up" {
			t.Errorf("下の日に上半身の補助が出ている: %s", id)
		}
	}
}

// sessionsBefore は出席 n 回ぶんの履歴。周期を進めるためだけのもの。
func sessionsBefore(t *testing.T, n int) []*setlog.SetLog {
	t.Helper()

	logs := make([]*setlog.SetLog, 0, n)
	for i := range n {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("s-%d", i),
			planMonday.AddDays(i*2-14), "bench", 80, 8, 2))
	}
	return logs
}

// unaffiliatedRequest は、どの日にも属さない区分（腹）を持つ入力。
func unaffiliatedRequest(t *testing.T, cycle ...program.Split) planning.PlanRequest {
	t.Helper()

	pool := splitPool(t)
	pool = append(pool, mkAccessory(t, "crunch",
		map[training.MuscleRegion]float64{training.Abs: 1.0}))

	ids := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		ids = append(ids, e.ID())
	}
	prog, err := program.NewProgram(mustFrequency(t, 2), planVolume(t), mustTarget(t, map[training.MuscleRegion]float64{
		// 腹の目標を胸・脚より大きく置く。理由は
		// TestSessionPlanner_AxisIsEmptyWhenNoDeclaredFitsTheDay と同じ。
		training.ChestMid: 6, training.Quad: 6, training.Abs: 24,
	}),
		ids, []exercise.ExerciseID{"bench", "squat"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	if prog, err = prog.WithCycle(cycle); err != nil {
		t.Fatalf("WithCycle: %v", err)
	}

	req := splitRequest(t, prog)
	req.Pool = pool
	return req
}

// containsAccessory は補助にその種目が含まれるか。
func containsAccessory(s planning.PlannedSession, id exercise.ExerciseID) bool {
	for _, a := range s.Accessories() {
		if a.ExerciseID() == id {
			return true
		}
	}
	return false
}
