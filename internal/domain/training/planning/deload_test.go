package planning_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// benchSessions は週1回ずつ n セッションのベンチ記録を作る。
func benchSessions(t *testing.T, n int, kg func(i int) float64) []*setlog.SetLog {
	t.Helper()
	out := make([]*setlog.SetLog, 0, n)
	for i := range n {
		out = append(out, mkLogOn(t, fmt.Sprintf("b%02d", i),
			baseDay(1+i*7), "bench", kg(i), 8, 2))
	}
	return out
}

// steadyWeight は指定日数ぶんの体重記録。kg は daysAgo を受け取る。
func steadyWeight(from int, kg func(daysAgo int) float64) condition.ConditionLog {
	items := make([]condition.DailyCondition, 0, 28)
	for i := range 28 {
		items = append(items, condition.NewDailyCondition(baseDay(from).AddDays(-i)).
			WithBodyWeight(kg(i)))
	}
	return condition.NewConditionLog(items)
}

func flatWeight(from int) condition.ConditionLog {
	return steadyWeight(from, func(int) float64 { return 75 })
}

const (
	sessions = 9 // 既定の停滞セッション数 8 + 1
	lastDay  = 1 + (sessions-1)*7
)

func TestDeloadPolicy_ProposesWhenStalledAtStableWeight(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	got, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("停滞しているのに提案されない")
	}
	if got.IntensityDropPct() <= 0 || got.IntensityDropPct() >= 1 {
		t.Errorf("低下率が範囲外: %v", got.IntensityDropPct())
	}
	if strings.TrimSpace(got.Reason()) == "" {
		t.Error("根拠が空である")
	}
	if !strings.Contains(got.Reason(), "bench") {
		t.Errorf("根拠にどの種目か書かれていない: %v", got.Reason())
	}
}

func TestDeloadPolicy_DoesNotProposeWhenImproving(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(i int) float64 { return 80 + float64(i)*2.5 }))

	if got, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("伸びているのに提案されている: %v", got.Reason())
	}
}

// 直前の窓の最大値を基準にすること。
//
// 1つ前と比べるだけだと、上下に振れているだけの状態を「更新した」と誤判定する。
func TestDeloadPolicy_ComparesAgainstTheWindowReference(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// 85 → 82.5 → 85 → 82.5 と振れているだけ。一度も 85 を超えていない。
	oscillating := make([]float64, sessions)
	for i := range oscillating {
		oscillating[i] = 85
		if i%2 == 1 {
			oscillating[i] = 82.5
		}
	}
	h := setlog.NewHistory(benchSessions(t, sessions, func(i int) float64 { return oscillating[i] }))

	if _, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("振れているだけの状態が更新と誤判定されている")
	}
}

// 窓の中で一度でも更新していれば停滞ではない。
func TestDeloadPolicy_OneImprovementBreaksTheStall(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// 途中で一度だけ更新している。
	weights := make([]float64, sessions)
	for i := range weights {
		weights[i] = 85
	}
	weights[sessions-3] = 87.5
	h := setlog.NewHistory(benchSessions(t, sessions, func(i int) float64 { return weights[i] }))

	if got, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("窓の中で更新があるのに提案された: %v", got.Reason())
	}
}

// 許容幅を設けないこと。
//
// 推定1RMが動く最小単位はレップ1本ぶん（約2.5%）か重量グリッド1段（約1.25%）で、
// それ未満の変化は起こりえない。0.5% のような閾値は 0 と同じ意味にしかならず、
// 「わずかに伸びている」を停滞と誤判定する余地を作るだけ。
func TestDeloadPolicy_AnyImprovementCounts(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// 最終回だけ、レップを1本だけ増やした（重量は同じ）。
	logs := benchSessions(t, sessions-1, func(int) float64 { return 85 })
	logs = append(logs, mkLogOn(t, "bLast", baseDay(lastDay), "bench", 85, 9, 2))

	if got, ok := p.Propose(setlog.NewHistory(logs), []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("1レップの伸びが停滞と判定された: %v", got.Reason())
	}

	// ごくわずかな更新（+0.3%）でも停滞ではない。
	// 許容幅を置くと、この伸びが停滞と誤判定される。
	tiny := benchSessions(t, sessions-1, func(int) float64 { return 85 })
	tiny = append(tiny, mkLogOn(t, "tLast", baseDay(lastDay), "bench", 85.25, 8, 2))

	if got, ok := p.Propose(setlog.NewHistory(tiny), []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("わずかな伸びが停滞と判定された: %v", got.Reason())
	}
}

// 推定できないセッション（全セット自重）は判定の窓に入れないこと。
//
// 0 として混ぜると、その回が基準になったとき「以降すべて更新した」ことになり、
// 本当は停滞しているのに提案されなくなる。
func TestDeloadPolicy_SkipsUnestimableSessions(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// 2回目だけ全セット自重で、それ以外は 85kg で停滞。
	logs := make([]*setlog.SetLog, 0, sessions+1)
	for i := range sessions + 1 {
		kg := 85.0
		if i == 1 {
			kg = 0
		}
		logs = append(logs, mkLogOn(t, fmt.Sprintf("u%02d", i),
			baseDay(1+i*7), "bench", kg, 8, 2))
	}
	day := 1 + sessions*7

	if _, ok := p.Propose(setlog.NewHistory(logs), []exercise.ExerciseID{"bench"},
		flatWeight(day), baseDay(day)); !ok {
		t.Error("自重の回が窓に混ざって停滞判定が消えている")
	}
}

// 減量中の停滞は正常なので提案しない。
func TestDeloadPolicy_DoesNotProposeWhileCutting(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	// 過去ほど重い＝減量中。
	cutting := steadyWeight(lastDay, func(daysAgo int) float64 { return 75 + float64(daysAgo)*0.05 })

	if got, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, cutting, baseDay(lastDay)); ok {
		t.Errorf("減量中に提案されている: %v", got.Reason())
	}
}

// 増量中の停滞は提案する。減量中とは扱いが違う。
func TestDeloadPolicy_ProposesWhileBulking(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	bulking := steadyWeight(lastDay, func(daysAgo int) float64 { return 75 - float64(daysAgo)*0.05 })

	if _, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, bulking, baseDay(lastDay)); !ok {
		t.Error("増量中の停滞で提案されない")
	}
}

// 体重が分からないときは提案しない。
// 減量による停滞とオーバーリーチによる停滞を見分けられないため。
func TestDeloadPolicy_DoesNotProposeWithoutBodyWeight(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	sleepOnly := make([]condition.DailyCondition, 0, 28)
	for i := range 28 {
		sleepOnly = append(sleepOnly, condition.NewDailyCondition(baseDay(lastDay).AddDays(-i)).
			WithSleepHours(7))
	}

	if got, ok := p.Propose(h, []exercise.ExerciseID{"bench"},
		condition.NewConditionLog(sleepOnly), baseDay(lastDay)); ok {
		t.Errorf("体重が無いのに提案されている: %v", got.Reason())
	}
}

func TestDeloadPolicy_NeedsEnoughSessions(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// stallSessions+1 に1つ足りないと判定できない。
	short := setlog.NewHistory(benchSessions(t, sessions-1, func(int) float64 { return 85 }))
	shortDay := 1 + (sessions-2)*7
	if _, ok := p.Propose(short, []exercise.ExerciseID{"bench"}, flatWeight(shortDay), baseDay(shortDay)); ok {
		t.Error("セッション数が足りないのに提案されている")
	}

	enough := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))
	if _, ok := p.Propose(enough, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("最低セッション数で提案されない")
	}
}

// 基準日より後の記録を使わないこと。
func TestDeloadPolicy_IgnoresFutureSessions(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// 停滞中だが、基準日より後に大きく更新している。
	logs := benchSessions(t, sessions, func(int) float64 { return 85 })
	logs = append(logs, mkLogOn(t, "future", baseDay(lastDay+7), "bench", 120, 8, 2))

	if _, ok := p.Propose(setlog.NewHistory(logs), []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("未来の記録で停滞判定が消えている")
	}
}

// 複数のメイン種目のうち1つでも停滞していれば提案し、根拠に名前を並べること。
func TestDeloadPolicy_ReportsAllStalledLifts(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	logs := benchSessions(t, sessions, func(int) float64 { return 85 })
	for i := range sessions {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("s%02d", i),
			baseDay(1+i*7), "squat", 120, 8, 2))
		logs = append(logs, mkLogOn(t, fmt.Sprintf("d%02d", i),
			baseDay(1+i*7), "deadlift", 140+float64(i)*5, 8, 2))
	}

	got, ok := p.Propose(setlog.NewHistory(logs),
		[]exercise.ExerciseID{"bench", "squat", "deadlift"}, flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("提案されない")
	}
	for _, want := range []string{"bench", "squat"} {
		if !strings.Contains(got.Reason(), want) {
			t.Errorf("根拠に %s が含まれない: %v", want, got.Reason())
		}
	}
	if strings.Contains(got.Reason(), "deadlift") {
		t.Errorf("伸びている種目が根拠に含まれている: %v", got.Reason())
	}
}

// 根拠は決定的であること。種目の並びが実行ごとに変わると、
// 同じ状況で違う文言が出る。
func TestDeloadPolicy_ReasonIsDeterministic(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	logs := benchSessions(t, sessions, func(int) float64 { return 85 })
	for i := range sessions {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("s%02d", i), baseDay(1+i*7), "squat", 120, 8, 2))
	}
	h := setlog.NewHistory(logs)
	ids := []exercise.ExerciseID{"squat", "bench"}

	first, ok := p.Propose(h, ids, flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("提案されない")
	}
	for range 30 {
		got, _ := p.Propose(h, ids, flatWeight(lastDay), baseDay(lastDay))
		if got.Reason() != first.Reason() ||
			got.IntensityDropPct() != first.IntensityDropPct() ||
			len(got.StalledExercises()) != len(first.StalledExercises()) {
			t.Fatalf("実行ごとに提案が変わる: %q vs %q", first.Reason(), got.Reason())
		}
	}
}

func TestDeloadPolicy_EmptyInputs(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	if _, ok := p.Propose(h, nil, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("メイン種目が指定されていないのに提案された")
	}
	if _, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), training.Date{}); ok {
		t.Error("基準日が無いのに提案された")
	}
	if _, ok := p.Propose(setlog.NewHistory(nil), []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("履歴が無いのに提案された")
	}

	var zero planning.DeloadPolicy
	if _, ok := zero.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("ゼロ値のポリシーが提案した")
	}
}

func TestNewDeloadPolicy_RejectsBadParams(t *testing.T) {
	a := planning.DefaultConditionAnalyzer()

	cases := []struct {
		stallSessions    int
		intensityDropPct float64
	}{
		{0, 0.1}, {-1, 0.1}, {21, 0.1},
		{3, 0}, {3, -0.1}, {3, 1}, {3, 1.5}, {3, math.NaN()}, {3, math.Inf(1)},
	}
	for _, c := range cases {
		if _, err := planning.NewDeloadPolicy(a, c.stallSessions, c.intensityDropPct); err == nil {
			t.Errorf("不正なパラメータが通ってしまう: %+v", c)
		}
	}

	var zeroAnalyzer planning.ConditionAnalyzer
	if _, err := planning.NewDeloadPolicy(zeroAnalyzer, 3, 0.1); err == nil {
		t.Error("ゼロ値の分析器が通ってしまう")
	}

	if _, err := planning.NewDeloadPolicy(a, 1, 0.01); err != nil {
		t.Errorf("境界値が弾かれた: %v", err)
	}
}

func TestDefaultDeloadPolicy_Constants(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	if p.StallSessions() != 8 {
		t.Errorf("停滞セッション数が誤り: %d", p.StallSessions())
	}
	if math.Abs(p.IntensityDropPct()-0.10) > 1e-9 {
		t.Errorf("強度低下率が誤り: %v", p.IntensityDropPct())
	}
}

// 減量判定の境界。
func TestDeloadPolicy_CuttingThreshold(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	cases := []struct {
		name         string
		kgPerDay     float64
		wantProposal bool
	}{
		{"週-0.35kg（明確な減量）", 0.05, false},
		{"週-0.07kg（誤差の範囲）", 0.01, true},
		{"横ばい", 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := steadyWeight(lastDay, func(daysAgo int) float64 {
				return 75 + float64(daysAgo)*c.kgPerDay
			})
			_, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, log, baseDay(lastDay))
			if ok != c.wantProposal {
				t.Errorf("提案=%v（期待 %v）", ok, c.wantProposal)
			}
		})
	}
}

// 回復・上昇の途中を停滞と呼ばないこと。
//
// 窓の先頭だけを基準にすると、病み上がりやデロード直後のように
// 「窓の入口が高くてそこから回復している」状態を停滞と判定する。
// 毎回更新しているのに過去の値に届いていないだけ、という状況で
// さらに10%下げるのは誤り。
func TestDeloadPolicy_DoesNotProposeWhileRecovering(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// 最初が高く、そこから毎回わずかに更新しながら戻っている。
	weights := make([]float64, sessions)
	weights[0] = 100
	for i := 1; i < sessions; i++ {
		weights[i] = 80 + float64(i)*1.5
	}
	h := setlog.NewHistory(benchSessions(t, sessions, func(i int) float64 { return weights[i] }))

	if got, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("回復中に提案された: %v", got.Reason())
	}
}

// 停滞した種目を機械可読で返すこと。
//
// 根拠の文字列からしか取れないと、呼び出し側が「どの種目を下げるか」を
// 選べず、伸びている種目まで一律に下げることになる。
func TestDeloadProposal_ReportsStalledExercises(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	logs := benchSessions(t, sessions, func(int) float64 { return 85 })
	for i := range sessions {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("s%02d", i), baseDay(1+i*7), "squat", 120, 8, 2))
		logs = append(logs, mkLogOn(t, fmt.Sprintf("d%02d", i), baseDay(1+i*7),
			"deadlift", 140+float64(i)*5, 8, 2))
	}

	got, ok := p.Propose(setlog.NewHistory(logs),
		[]exercise.ExerciseID{"deadlift", "bench", "squat"}, flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("提案されない")
	}

	stalled := got.StalledExercises()
	want := []exercise.ExerciseID{"bench", "squat"}
	if len(stalled) != len(want) {
		t.Fatalf("停滞種目の数が誤り: %v", stalled)
	}
	for i := range want {
		if stalled[i] != want[i] {
			t.Errorf("停滞種目が誤り（昇順であるべき）: %v", stalled)
		}
	}

	// 返り値を書き換えても提案は変わらない。
	stalled[0] = "tampered"
	if got.StalledExercises()[0] == "tampered" {
		t.Error("返り値の書き換えが提案に波及している")
	}
}

// 同じ種目を複数回渡しても、重複して報告しないこと。
func TestDeloadPolicy_DeduplicatesMainIDs(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	got, ok := p.Propose(h, []exercise.ExerciseID{"bench", "bench", "bench"},
		flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("提案されない")
	}
	if len(got.StalledExercises()) != 1 {
		t.Errorf("重複が除かれていない: %v", got.StalledExercises())
	}
	if strings.Count(got.Reason(), "bench") != 1 {
		t.Errorf("根拠に重複がある: %v", got.Reason())
	}
}

// 既定以外のパラメータが実際に効くこと。
//
// 生成できるかどうかだけ見ても、その値が判定や提案に使われているかは分からない。
func TestDeloadPolicy_HonoursConfiguredParameters(t *testing.T) {
	a := planning.DefaultConditionAnalyzer()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	strict, err := planning.NewDeloadPolicy(a, 2, 0.25)
	if err != nil {
		t.Fatalf("NewDeloadPolicy: %v", err)
	}
	got, ok := strict.Propose(h, []exercise.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("提案されない")
	}
	if math.Abs(got.IntensityDropPct()-0.25) > 1e-9 {
		t.Errorf("設定した低下率が使われていない: %v", got.IntensityDropPct())
	}
	if !strings.Contains(got.Reason(), "2セッション") {
		t.Errorf("設定した停滞セッション数が根拠に反映されていない: %v", got.Reason())
	}

	// 窓の長さで結果が変わること。
	//
	// 5セッション前に更新があり、それ以降は停滞している履歴を使う。
	// 窓が短ければ「最近は停滞」、長ければ「窓の中に更新がある」になる。
	weights := make([]float64, sessions)
	for i := range weights {
		weights[i] = 85
	}
	weights[sessions-5] = 90
	mixed := setlog.NewHistory(benchSessions(t, sessions, func(i int) float64 { return weights[i] }))

	if _, ok := strict.Propose(mixed, []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("短い窓で最近の停滞が検出されない")
	}
	if got, ok := planning.DefaultDeloadPolicy().Propose(mixed, []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("長い窓なのに窓内の更新が無視された: %v", got.Reason())
	}

	// 窓が履歴より長ければ判定できない。
	lenient, err := planning.NewDeloadPolicy(a, 20, 0.1)
	if err != nil {
		t.Fatalf("NewDeloadPolicy: %v", err)
	}
	if _, ok := lenient.Propose(h, []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("履歴より長い窓で提案された")
	}
}

// 減量判定の境界そのものを固定する。
func TestDeloadPolicy_CuttingThresholdBoundary(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(int) float64 { return 85 }))

	// 傾きは1日あたり kgPerDay。週換算は7倍。
	cases := []struct {
		name         string
		kgPerWeek    float64
		wantProposal bool
	}{
		{"閾値ちょうど（-0.1kg/週）は減量とみなさない", 0.1, true},
		{"閾値をわずかに超える減量", 0.11, false},
		{"閾値にわずかに足りない減量", 0.09, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := steadyWeight(lastDay, func(daysAgo int) float64 {
				return 75 + float64(daysAgo)*c.kgPerWeek/7
			})
			_, ok := p.Propose(h, []exercise.ExerciseID{"bench"}, log, baseDay(lastDay))
			if ok != c.wantProposal {
				t.Errorf("提案=%v（期待 %v）", ok, c.wantProposal)
			}
		})
	}
}

// 根拠の種目名は昇順であること。渡された順序に依存すると、
// 同じ状況で違う文言が出る。
func TestDeloadPolicy_ReasonListsExercisesInOrder(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	logs := benchSessions(t, sessions, func(int) float64 { return 85 })
	for i := range sessions {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("s%02d", i), baseDay(1+i*7), "squat", 120, 8, 2))
	}
	h := setlog.NewHistory(logs)

	forward, ok := p.Propose(h, []exercise.ExerciseID{"bench", "squat"}, flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("提案されない")
	}
	backward, _ := p.Propose(h, []exercise.ExerciseID{"squat", "bench"}, flatWeight(lastDay), baseDay(lastDay))

	if forward.Reason() != backward.Reason() {
		t.Errorf("渡す順序で根拠が変わる: %q vs %q", forward.Reason(), backward.Reason())
	}
	if !strings.Contains(forward.Reason(), "bench, squat") {
		t.Errorf("種目名が昇順に並んでいない: %v", forward.Reason())
	}
}

// デロード直後に再提案しない。するとデロードが下降スパイラルになる。
func TestDeloadPolicy_DoesNotProposeRightAfterADeload(t *testing.T) {
	p := planning.DefaultDeloadPolicy()

	// 8セッション停滞したあと、承認して10%落とした状態。
	h := setlog.NewHistory(benchSessions(t, sessions, func(i int) float64 {
		if i == sessions-1 {
			return 75 // 85kg の -10%（2.5kg刻みに丸めた実施重量）
		}
		return 85
	}))

	if _, ok := p.Propose(h, []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("デロード直後にさらにデロードを提案している")
	}
}

// 通常の上下動（レップ1本ぶん程度）は停滞のままにする。
// 上のガードで一律に握りつぶさないこと。
func TestDeloadPolicy_SmallFluctuationIsStillStalled(t *testing.T) {
	p := planning.DefaultDeloadPolicy()
	h := setlog.NewHistory(benchSessions(t, sessions, func(i int) float64 {
		if i%2 == 1 {
			return 82.5
		}
		return 85
	}))

	if _, ok := p.Propose(h, []exercise.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("上下動しているだけの停滞が提案されない")
	}
}
