package training_test

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mustEstimator(t *testing.T, alpha, hysteresis float64) training.OneRepMaxEstimator {
	t.Helper()
	e, err := training.NewOneRepMaxEstimator(alpha, hysteresis)
	if err != nil {
		t.Fatalf("NewOneRepMaxEstimator(%v, %v): %v", alpha, hysteresis, err)
	}
	return e
}

func TestOneRepMaxEstimator_NoHistory(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	if got, ok := est.Estimate(training.NewHistory(nil), "bench", nil); ok {
		t.Errorf("履歴が無いのに推定値が返る: %v", got.Kg())
	}
}

func TestOneRepMaxEstimator_OtherExerciseOnly(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "squat", 100, 8, 2)})

	if got, ok := est.Estimate(h, "bench", nil); ok {
		t.Errorf("別種目の履歴から推定値が返る: %v", got.Kg())
	}
}

func TestOneRepMaxEstimator_SingleSession(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	got, ok := est.Estimate(h, "bench", nil)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	want := 85.0 * (1 + 11.0/30.0)
	if math.Abs(got.Kg()-want) > 1e-4 {
		t.Errorf("got %v, want %v", got.Kg(), want)
	}
}

// EWMA が「新しいほど重く、しかし直近値そのものにはならない」こと。
func TestOneRepMaxEstimator_SmoothsAcrossSessions(t *testing.T) {
	est := mustEstimator(t, 0.5, 0.02)
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "bench", 80, 8, 2),
		mkLog(t, "b", 8, "bench", 90, 8, 2),
	})

	got, ok := est.Estimate(h, "bench", nil)
	if !ok {
		t.Fatal("推定値が返らない")
	}

	old := 80.0 * (1 + 10.0/30.0)
	recent := 90.0 * (1 + 10.0/30.0)
	if got.Kg() <= old {
		t.Errorf("新しい記録が反映されていない: %v", got.Kg())
	}
	if got.Kg() >= recent {
		t.Errorf("平滑化されず直近値そのものになっている: %v", got.Kg())
	}

	// alpha=0.5 なので、ちょうど中間になる。
	want := 0.5*recent + 0.5*old
	if math.Abs(got.Kg()-want) > 1e-4 {
		t.Errorf("EWMA の重みが誤り: got %v, want %v", got.Kg(), want)
	}
}

// 畳み込みは必ず古い順。順序が逆だと、直近の記録が最も軽く扱われる。
func TestOneRepMaxEstimator_FoldsOldestFirst(t *testing.T) {
	est := mustEstimator(t, 0.5, 0)

	// 入力の並びを変えても結果が同じこと（History が日付順に整える）。
	logs := []*training.SetLog{
		mkLog(t, "a", 1, "bench", 60, 8, 2),
		mkLog(t, "b", 8, "bench", 80, 8, 2),
		mkLog(t, "c", 15, "bench", 100, 8, 2),
	}
	forward, ok := est.Estimate(training.NewHistory(logs), "bench", nil)
	if !ok {
		t.Fatal("推定値が返らない")
	}

	reversed := []*training.SetLog{logs[2], logs[0], logs[1]}
	backward, _ := est.Estimate(training.NewHistory(reversed), "bench", nil)
	if forward != backward {
		t.Errorf("入力順で結果が変わる: %v vs %v", forward.Kg(), backward.Kg())
	}

	// 期待値: acc = 60分 → 0.5*80分 + 0.5*acc → 0.5*100分 + 0.5*acc
	v := func(kg float64) float64 { return kg * (1 + 10.0/30.0) }
	want := 0.5*v(100) + 0.5*(0.5*v(80)+0.5*v(60))
	if math.Abs(forward.Kg()-want) > 1e-4 {
		t.Errorf("古い順に畳み込まれていない: got %v, want %v", forward.Kg(), want)
	}
}

// 直近の記録が最も強く効くこと。
func TestOneRepMaxEstimator_RecentSessionsDominate(t *testing.T) {
	// alpha=0.5 だと2セッションの重みが対称になり差が出ないので、0.7 を使う。
	est := mustEstimator(t, 0.7, 0)

	rising := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "bench", 60, 8, 2),
		mkLog(t, "b", 8, "bench", 100, 8, 2),
	})
	falling := training.NewHistory([]*training.SetLog{
		mkLog(t, "c", 1, "bench", 100, 8, 2),
		mkLog(t, "d", 8, "bench", 60, 8, 2),
	})

	up, _ := est.Estimate(rising, "bench", nil)
	down, _ := est.Estimate(falling, "bench", nil)
	if up.Kg() <= down.Kg() {
		t.Errorf("直近の記録が支配的でない: 上昇 %v, 下降 %v", up.Kg(), down.Kg())
	}
}

func TestOneRepMaxEstimator_HysteresisHoldsSmallChange(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	candidate := 85.0 * (1 + 11.0/30.0)
	prev, err := training.NewOneRepMax(candidate * 0.99) // 1%低い
	if err != nil {
		t.Fatalf("NewOneRepMax: %v", err)
	}

	got, ok := est.Estimate(h, "bench", &prev)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	if got != prev {
		t.Errorf("閾値未満の変化でヒステリシスが効いていない: got %v, want %v", got.Kg(), prev.Kg())
	}
}

func TestOneRepMaxEstimator_HysteresisPassesLargeChange(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	prev, _ := training.NewOneRepMax(85.0 * (1 + 11.0/30.0) * 0.8) // 20%低い
	got, ok := est.Estimate(h, "bench", &prev)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	if got == prev {
		t.Errorf("大きな変化が通っていない: %v", got.Kg())
	}
}

// ヒステリシスは上下どちらの方向にも効くこと。
func TestOneRepMaxEstimator_HysteresisIsSymmetric(t *testing.T) {
	est := mustEstimator(t, 1.0, 0.05)
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 100, 8, 2)})
	candidate := 100.0 * (1 + 10.0/30.0)

	for _, factor := range []float64{0.97, 1.03} {
		prev, err := training.NewOneRepMax(candidate * factor)
		if err != nil {
			t.Fatalf("NewOneRepMax: %v", err)
		}
		got, _ := est.Estimate(h, "bench", &prev)
		if got != prev {
			t.Errorf("係数 %v の方向でヒステリシスが効いていない: got %v, want %v",
				factor, got.Kg(), prev.Kg())
		}
	}
}

// 閾値の前後で挙動が切り替わること。
func TestOneRepMaxEstimator_HysteresisBoundary(t *testing.T) {
	const threshold = 0.1
	est := mustEstimator(t, 1.0, threshold)
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 100, 8, 2)})
	candidate := 100.0 * (1 + 10.0/30.0)

	// 変化率がちょうど r になる前回値は candidate/(1+r)。
	below, err := training.NewOneRepMax(candidate / (1 + threshold*0.9))
	if err != nil {
		t.Fatalf("NewOneRepMax: %v", err)
	}
	if got, _ := est.Estimate(h, "bench", &below); got != below {
		t.Errorf("閾値未満の変化が通ってしまう: %v", got.Kg())
	}

	above, err := training.NewOneRepMax(candidate / (1 + threshold*1.1))
	if err != nil {
		t.Fatalf("NewOneRepMax: %v", err)
	}
	if got, _ := est.Estimate(h, "bench", &above); got == above {
		t.Errorf("閾値を超える変化が通っていない: %v", got.Kg())
	}
}

func TestOneRepMaxEstimator_ZeroPreviousIsIgnored(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	var zero training.OneRepMax
	got, ok := est.Estimate(h, "bench", &zero)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	if got.IsZero() {
		t.Error("ゼロ値の前回値がそのまま維持されている")
	}
}

// 推定できないセッション（全セット自重）は畳み込みから除くこと。
// 0 として混ぜると、その回だけで推定1RMが大きく落ちる。
func TestOneRepMaxEstimator_SkipsUnestimableSessions(t *testing.T) {
	est := mustEstimator(t, 0.5, 0)

	withBodyweight := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "dip", 40, 8, 2),
		mkLog(t, "b", 8, "dip", 0, 12, 1), // 全セット自重の回
		mkLog(t, "c", 15, "dip", 45, 8, 2),
	})
	without := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "dip", 40, 8, 2),
		mkLog(t, "c", 15, "dip", 45, 8, 2),
	})

	a, ok := est.Estimate(withBodyweight, "dip", nil)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	b, _ := est.Estimate(without, "dip", nil)
	if a != b {
		t.Errorf("自重の回が畳み込みに影響している: %v vs %v", a.Kg(), b.Kg())
	}
}

func TestOneRepMaxEstimator_AllSessionsUnestimable(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "dip", 0, 10, 1),
		mkLog(t, "b", 8, "dip", 0, 9, 0),
	})

	if got, ok := est.Estimate(h, "dip", nil); ok {
		t.Errorf("推定できるセッションが無いのに値が返る: %v", got.Kg())
	}
}

func TestNewOneRepMaxEstimator_RejectsBadParams(t *testing.T) {
	for _, c := range []struct{ alpha, hysteresis float64 }{
		{0, 0.02}, {-0.1, 0.02}, {1.1, 0.02}, {math.NaN(), 0.02},
		{0.3, -0.1}, {0.3, 1}, {0.3, 1.5}, {0.3, math.NaN()},
	} {
		if _, err := training.NewOneRepMaxEstimator(c.alpha, c.hysteresis); err == nil {
			t.Errorf("不正なパラメータが通ってしまう: alpha=%v hysteresis=%v", c.alpha, c.hysteresis)
		}
	}
	if _, err := training.NewOneRepMaxEstimator(1.0, 0); err != nil {
		t.Errorf("境界値が弾かれた: %v", err)
	}
}

// 既定値そのものを固定する。定数を変えると全スロットの重量の追随速度が変わる。
func TestDefaultOneRepMaxEstimator_Constants(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	if got := est.Alpha(); math.Abs(got-0.3) > 1e-9 {
		t.Errorf("alpha が誤り: got %v, want 0.3", got)
	}
	if got := est.Hysteresis(); math.Abs(got-0.02) > 1e-9 {
		t.Errorf("ヒステリシスが誤り: got %v, want 0.02", got)
	}
}

func TestOneRepMaxEstimator_ZeroValueIsSafe(t *testing.T) {
	var est training.OneRepMaxEstimator
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	if got, ok := est.Estimate(h, "bench", nil); ok {
		t.Errorf("ゼロ値の推定器が値を返す: %v", got.Kg())
	}
}

// 返り値は必ずコンストラクタを通った有効な値であること。
func TestOneRepMaxEstimator_ReturnsValidOneRepMax(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()

	logs := make([]*training.SetLog, 0, 30)
	for i := 1; i <= 28; i++ {
		logs = append(logs, mkLog(t, fmt.Sprintf("l%02d", i), i, "bench", 80+float64(i%5)*2.5, 8, 2))
	}

	got, ok := est.Estimate(training.NewHistory(logs), "bench", nil)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	if _, err := training.NewOneRepMax(got.Kg()); err != nil {
		t.Errorf("コンストラクタが拒否する値が返っている: %v", err)
	}
	if n := decimalPlaces(strconv.FormatFloat(got.Kg(), 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v", got.Kg())
	}
}

// 何度呼んでも同じ値。マップの反復順などに依存しないこと。
func TestOneRepMaxEstimator_IsDeterministic(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()

	logs := make([]*training.SetLog, 0, 20)
	for i := 1; i <= 20; i++ {
		logs = append(logs, mkLog(t, fmt.Sprintf("l%02d", i), i, "bench", 80+float64(i), 8, 2))
	}
	h := training.NewHistory(logs)

	first, ok := est.Estimate(h, "bench", nil)
	if !ok {
		t.Fatal("推定値が返らない")
	}
	for range 50 {
		got, _ := est.Estimate(h, "bench", nil)
		if got != first {
			t.Fatalf("呼び出しごとに結果が変わる: %v vs %v", first.Kg(), got.Kg())
		}
	}
}
