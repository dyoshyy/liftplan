package training_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// benchSessions は週1回ずつ n セッションのベンチ記録を作る。
func benchSessions(t *testing.T, n int, kg func(i int) float64) []*training.SetLog {
	t.Helper()
	out := make([]*training.SetLog, 0, n)
	for i := range n {
		out = append(out, mkLogOn(t, fmt.Sprintf("b%02d", i),
			baseDay(1+i*7), "bench", kg(i), 8, 2))
	}
	return out
}

// steadyWeight は指定日数ぶんの体重記録。kg は daysAgo を受け取る。
func steadyWeight(from int, kg func(daysAgo int) float64) training.ConditionLog {
	items := make([]training.DailyCondition, 0, 28)
	for i := range 28 {
		items = append(items, training.NewDailyCondition(baseDay(from).AddDays(-i)).
			WithBodyWeight(kg(i)))
	}
	return training.NewConditionLog(items)
}

func flatWeight(from int) training.ConditionLog {
	return steadyWeight(from, func(int) float64 { return 75 })
}

const lastDay = 1 + 3*7 // 4セッション目の日

func TestDeloadPolicy_ProposesWhenStalledAtStableWeight(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory(benchSessions(t, 4, func(int) float64 { return 85 }))

	got, ok := p.Propose(h, []training.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay))
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
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory(benchSessions(t, 4, func(i int) float64 { return 80 + float64(i)*2.5 }))

	if got, ok := p.Propose(h, []training.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("伸びているのに提案されている: %v", got.Reason())
	}
}

// 直前の窓の最大値を基準にすること。
//
// 1つ前と比べるだけだと、上下に振れているだけの状態を「更新した」と誤判定する。
func TestDeloadPolicy_ComparesAgainstTheWindowReference(t *testing.T) {
	p := training.DefaultDeloadPolicy()

	// 85 → 82.5 → 85 → 82.5 と振れているだけ。一度も 85 を超えていない。
	oscillating := []float64{85, 82.5, 85, 82.5}
	h := training.NewHistory(benchSessions(t, 4, func(i int) float64 { return oscillating[i] }))

	if _, ok := p.Propose(h, []training.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("振れているだけの状態が更新と誤判定されている")
	}
}

// 窓の中で一度でも更新していれば停滞ではない。
func TestDeloadPolicy_OneImprovementBreaksTheStall(t *testing.T) {
	p := training.DefaultDeloadPolicy()

	// 3回目だけ更新している。
	weights := []float64{85, 85, 87.5, 85}
	h := training.NewHistory(benchSessions(t, 4, func(i int) float64 { return weights[i] }))

	if got, ok := p.Propose(h, []training.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("窓の中で更新があるのに提案された: %v", got.Reason())
	}
}

// 許容幅を設けないこと。
//
// 推定1RMが動く最小単位はレップ1本ぶん（約2.5%）か重量グリッド1段（約1.25%）で、
// それ未満の変化は起こりえない。0.5% のような閾値は 0 と同じ意味にしかならず、
// 「わずかに伸びている」を停滞と誤判定する余地を作るだけ。
func TestDeloadPolicy_AnyImprovementCounts(t *testing.T) {
	p := training.DefaultDeloadPolicy()

	// 最終回だけ、レップを1本だけ増やした（重量は同じ）。
	logs := benchSessions(t, 3, func(int) float64 { return 85 })
	logs = append(logs, mkLogOn(t, "b03", baseDay(lastDay), "bench", 85, 9, 2))

	if got, ok := p.Propose(training.NewHistory(logs), []training.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("1レップの伸びが停滞と判定された: %v", got.Reason())
	}

	// ごくわずかな更新（+0.3%）でも停滞ではない。
	// 許容幅を置くと、この伸びが停滞と誤判定される。
	tiny := benchSessions(t, 3, func(int) float64 { return 85 })
	tiny = append(tiny, mkLogOn(t, "t03", baseDay(lastDay), "bench", 85.25, 8, 2))

	if got, ok := p.Propose(training.NewHistory(tiny), []training.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Errorf("わずかな伸びが停滞と判定された: %v", got.Reason())
	}
}

// 推定できないセッション（全セット自重）は判定の窓に入れないこと。
//
// 0 として混ぜると、その回が基準になったとき「以降すべて更新した」ことになり、
// 本当は停滞しているのに提案されなくなる。
func TestDeloadPolicy_SkipsUnestimableSessions(t *testing.T) {
	p := training.DefaultDeloadPolicy()

	// 5セッション。2回目だけ全セット自重で、それ以外は 85kg で停滞。
	logs := make([]*training.SetLog, 0, 5)
	for i := range 5 {
		kg := 85.0
		if i == 1 {
			kg = 0
		}
		logs = append(logs, mkLogOn(t, fmt.Sprintf("u%02d", i),
			baseDay(1+i*7), "bench", kg, 8, 2))
	}
	day := 1 + 4*7

	if _, ok := p.Propose(training.NewHistory(logs), []training.ExerciseID{"bench"},
		flatWeight(day), baseDay(day)); !ok {
		t.Error("自重の回が窓に混ざって停滞判定が消えている")
	}
}

// 減量中の停滞は正常なので提案しない。
func TestDeloadPolicy_DoesNotProposeWhileCutting(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory(benchSessions(t, 4, func(int) float64 { return 85 }))

	// 過去ほど重い＝減量中。
	cutting := steadyWeight(lastDay, func(daysAgo int) float64 { return 75 + float64(daysAgo)*0.05 })

	if got, ok := p.Propose(h, []training.ExerciseID{"bench"}, cutting, baseDay(lastDay)); ok {
		t.Errorf("減量中に提案されている: %v", got.Reason())
	}
}

// 増量中の停滞は提案する。減量中とは扱いが違う。
func TestDeloadPolicy_ProposesWhileBulking(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory(benchSessions(t, 4, func(int) float64 { return 85 }))

	bulking := steadyWeight(lastDay, func(daysAgo int) float64 { return 75 - float64(daysAgo)*0.05 })

	if _, ok := p.Propose(h, []training.ExerciseID{"bench"}, bulking, baseDay(lastDay)); !ok {
		t.Error("増量中の停滞で提案されない")
	}
}

// 体重が分からないときは提案しない。
// 減量による停滞とオーバーリーチによる停滞を見分けられないため。
func TestDeloadPolicy_DoesNotProposeWithoutBodyWeight(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory(benchSessions(t, 4, func(int) float64 { return 85 }))

	sleepOnly := make([]training.DailyCondition, 0, 28)
	for i := range 28 {
		sleepOnly = append(sleepOnly, training.NewDailyCondition(baseDay(lastDay).AddDays(-i)).
			WithSleepHours(7))
	}

	if got, ok := p.Propose(h, []training.ExerciseID{"bench"},
		training.NewConditionLog(sleepOnly), baseDay(lastDay)); ok {
		t.Errorf("体重が無いのに提案されている: %v", got.Reason())
	}
}

func TestDeloadPolicy_NeedsEnoughSessions(t *testing.T) {
	p := training.DefaultDeloadPolicy()

	// 3セッションでは判定できない（stallSessions+1 が必要）。
	three := training.NewHistory(benchSessions(t, 3, func(int) float64 { return 85 }))
	if _, ok := p.Propose(three, []training.ExerciseID{"bench"}, flatWeight(15), baseDay(15)); ok {
		t.Error("セッション数が足りないのに提案されている")
	}

	four := training.NewHistory(benchSessions(t, 4, func(int) float64 { return 85 }))
	if _, ok := p.Propose(four, []training.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("最低セッション数で提案されない")
	}
}

// 基準日より後の記録を使わないこと。
func TestDeloadPolicy_IgnoresFutureSessions(t *testing.T) {
	p := training.DefaultDeloadPolicy()

	// 停滞中だが、基準日より後に大きく更新している。
	logs := benchSessions(t, 4, func(int) float64 { return 85 })
	logs = append(logs, mkLogOn(t, "future", baseDay(lastDay+7), "bench", 120, 8, 2))

	if _, ok := p.Propose(training.NewHistory(logs), []training.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); !ok {
		t.Error("未来の記録で停滞判定が消えている")
	}
}

// 複数のメイン種目のうち1つでも停滞していれば提案し、根拠に名前を並べること。
func TestDeloadPolicy_ReportsAllStalledLifts(t *testing.T) {
	p := training.DefaultDeloadPolicy()

	logs := benchSessions(t, 4, func(int) float64 { return 85 })
	for i := range 4 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("s%02d", i),
			baseDay(1+i*7), "squat", 120, 8, 2))
		logs = append(logs, mkLogOn(t, fmt.Sprintf("d%02d", i),
			baseDay(1+i*7), "deadlift", 140+float64(i)*5, 8, 2))
	}

	got, ok := p.Propose(training.NewHistory(logs),
		[]training.ExerciseID{"bench", "squat", "deadlift"}, flatWeight(lastDay), baseDay(lastDay))
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
	p := training.DefaultDeloadPolicy()

	logs := benchSessions(t, 4, func(int) float64 { return 85 })
	for i := range 4 {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("s%02d", i), baseDay(1+i*7), "squat", 120, 8, 2))
	}
	h := training.NewHistory(logs)
	ids := []training.ExerciseID{"squat", "bench"}

	first, ok := p.Propose(h, ids, flatWeight(lastDay), baseDay(lastDay))
	if !ok {
		t.Fatal("提案されない")
	}
	for range 30 {
		got, _ := p.Propose(h, ids, flatWeight(lastDay), baseDay(lastDay))
		if got != first {
			t.Fatalf("実行ごとに提案が変わる: %q vs %q", first.Reason(), got.Reason())
		}
	}
}

func TestDeloadPolicy_EmptyInputs(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory(benchSessions(t, 4, func(int) float64 { return 85 }))

	if _, ok := p.Propose(h, nil, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("メイン種目が指定されていないのに提案された")
	}
	if _, ok := p.Propose(h, []training.ExerciseID{"bench"}, flatWeight(lastDay), training.Date{}); ok {
		t.Error("基準日が無いのに提案された")
	}
	if _, ok := p.Propose(training.NewHistory(nil), []training.ExerciseID{"bench"},
		flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("履歴が無いのに提案された")
	}

	var zero training.DeloadPolicy
	if _, ok := zero.Propose(h, []training.ExerciseID{"bench"}, flatWeight(lastDay), baseDay(lastDay)); ok {
		t.Error("ゼロ値のポリシーが提案した")
	}
}

func TestNewDeloadPolicy_RejectsBadParams(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	cases := []struct {
		stallSessions    int
		intensityDropPct float64
	}{
		{0, 0.1}, {-1, 0.1}, {21, 0.1},
		{3, 0}, {3, -0.1}, {3, 1}, {3, 1.5}, {3, math.NaN()}, {3, math.Inf(1)},
	}
	for _, c := range cases {
		if _, err := training.NewDeloadPolicy(a, c.stallSessions, c.intensityDropPct); err == nil {
			t.Errorf("不正なパラメータが通ってしまう: %+v", c)
		}
	}

	var zeroAnalyzer training.ConditionAnalyzer
	if _, err := training.NewDeloadPolicy(zeroAnalyzer, 3, 0.1); err == nil {
		t.Error("ゼロ値の分析器が通ってしまう")
	}

	if _, err := training.NewDeloadPolicy(a, 1, 0.01); err != nil {
		t.Errorf("境界値が弾かれた: %v", err)
	}
}

func TestDefaultDeloadPolicy_Constants(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	if p.StallSessions() != 3 {
		t.Errorf("停滞セッション数が誤り: %d", p.StallSessions())
	}
	if math.Abs(p.IntensityDropPct()-0.10) > 1e-9 {
		t.Errorf("強度低下率が誤り: %v", p.IntensityDropPct())
	}
}

// 減量判定の境界。
func TestDeloadPolicy_CuttingThreshold(t *testing.T) {
	p := training.DefaultDeloadPolicy()
	h := training.NewHistory(benchSessions(t, 4, func(int) float64 { return 85 }))

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
			_, ok := p.Propose(h, []training.ExerciseID{"bench"}, log, baseDay(lastDay))
			if ok != c.wantProposal {
				t.Errorf("提案=%v（期待 %v）", ok, c.wantProposal)
			}
		})
	}
}
