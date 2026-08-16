package training_test

import (
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// asOf は履歴のテストデータ（2026年8月）から見て十分近い基準日。
func asOf(day int) training.Date { return training.MustDate(2026, time.August, day) }

func mustEstimator(t *testing.T, alpha float64, maxStaleDays int) training.OneRepMaxEstimator {
	t.Helper()
	e, err := training.NewOneRepMaxEstimator(alpha, maxStaleDays)
	if err != nil {
		t.Fatalf("NewOneRepMaxEstimator(%v, %d): %v", alpha, maxStaleDays, err)
	}
	return e
}

func TestOneRepMaxEstimator_NoHistory(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	if got, ok := est.Estimate(training.NewHistory(nil), "bench", asOf(20)); ok {
		t.Errorf("履歴が無いのに推定値が返る: %v", got.Kg())
	}
}

func TestOneRepMaxEstimator_OtherExerciseOnly(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "squat", 100, 8, 2)})

	if got, ok := est.Estimate(h, "bench", asOf(12)); ok {
		t.Errorf("別種目の履歴から推定値が返る: %v", got.Kg())
	}
}

// 同一日に複数種目を記録するのは通常運用。
// 種目で絞り込まずにセッション中央値を取ると、別種目の重量が混ざって
// 推定が大きく狂う（ベンチの基準にスクワットの重量が入る）。
func TestOneRepMaxEstimator_IgnoresOtherExercisesOnTheSameDay(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()

	mixed := training.NewHistory([]*training.SetLog{
		mkLog(t, "b1", 10, "bench", 100, 8, 2),
		mkLog(t, "b2", 10, "bench", 100, 8, 2),
		mkLog(t, "s1", 10, "squat", 180, 8, 2),
		mkLog(t, "s2", 10, "squat", 180, 8, 2),
		mkLog(t, "s3", 10, "squat", 180, 8, 2),
	})
	benchOnly := training.NewHistory([]*training.SetLog{
		mkLog(t, "b1", 10, "bench", 100, 8, 2),
		mkLog(t, "b2", 10, "bench", 100, 8, 2),
	})

	got, ok := est.Estimate(mixed, "bench", asOf(12))
	if !ok {
		t.Fatal("推定値が返らない")
	}
	want, _ := est.Estimate(benchOnly, "bench", asOf(12))
	if got != want {
		t.Errorf("同日の別種目が混ざっている: got %v, want %v", got.Kg(), want.Kg())
	}
}

func TestOneRepMaxEstimator_SingleSession(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	got, ok := est.Estimate(h, "bench", asOf(12))
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
	est := mustEstimator(t, 0.5, 42)
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "bench", 80, 8, 2),
		mkLog(t, "b", 8, "bench", 90, 8, 2),
	})

	got, ok := est.Estimate(h, "bench", asOf(10))
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
	if want := 0.5*recent + 0.5*old; math.Abs(got.Kg()-want) > 1e-4 {
		t.Errorf("EWMA の重みが誤り: got %v, want %v", got.Kg(), want)
	}
}

// 畳み込みは必ず古い順。順序が逆だと、直近の記録が最も軽く扱われる。
func TestOneRepMaxEstimator_FoldsOldestFirst(t *testing.T) {
	est := mustEstimator(t, 0.5, 42)

	logs := []*training.SetLog{
		mkLog(t, "a", 1, "bench", 60, 8, 2),
		mkLog(t, "b", 8, "bench", 80, 8, 2),
		mkLog(t, "c", 15, "bench", 100, 8, 2),
	}
	forward, ok := est.Estimate(training.NewHistory(logs), "bench", asOf(16))
	if !ok {
		t.Fatal("推定値が返らない")
	}

	reversed := []*training.SetLog{logs[2], logs[0], logs[1]}
	backward, _ := est.Estimate(training.NewHistory(reversed), "bench", asOf(16))
	if forward != backward {
		t.Errorf("入力順で結果が変わる: %v vs %v", forward.Kg(), backward.Kg())
	}

	v := func(kg float64) float64 { return kg * (1 + 10.0/30.0) }
	want := 0.5*v(100) + 0.5*(0.5*v(80)+0.5*v(60))
	if math.Abs(forward.Kg()-want) > 1e-4 {
		t.Errorf("古い順に畳み込まれていない: got %v, want %v", forward.Kg(), want)
	}
}

func TestOneRepMaxEstimator_RecentSessionsDominate(t *testing.T) {
	// alpha=0.5 だと2セッションの重みが対称になり差が出ないので、0.7 を使う。
	est := mustEstimator(t, 0.7, 42)

	rising := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "bench", 60, 8, 2),
		mkLog(t, "b", 8, "bench", 100, 8, 2),
	})
	falling := training.NewHistory([]*training.SetLog{
		mkLog(t, "c", 1, "bench", 100, 8, 2),
		mkLog(t, "d", 8, "bench", 60, 8, 2),
	})

	up, _ := est.Estimate(rising, "bench", asOf(10))
	down, _ := est.Estimate(falling, "bench", asOf(10))
	if up.Kg() <= down.Kg() {
		t.Errorf("直近の記録が支配的でない: 上昇 %v, 下降 %v", up.Kg(), down.Kg())
	}
}

// ブランク明けは推定しない。
//
// EWMA はセッション数だけで畳み込むので、3ヶ月空いても直前のセッションと
// 同じ重みになる。復帰初日に離脱前の重量を処方するのは危険。
func TestOneRepMaxEstimator_RejectsStaleHistory(t *testing.T) {
	est := mustEstimator(t, 0.3, 42)
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "bench", 100, 8, 2),
		mkLog(t, "b", 8, "bench", 100, 8, 2),
	})
	last := training.MustDate(2026, time.August, 8)

	// 境界のすぐ内側は推定できる。
	if _, ok := est.Estimate(h, "bench", last.AddDays(42)); !ok {
		t.Error("鮮度の境界ちょうどで推定できない")
	}
	// 1日でも超えたら推定しない。
	if got, ok := est.Estimate(h, "bench", last.AddDays(43)); ok {
		t.Errorf("古すぎる履歴から推定できてしまう: %v", got.Kg())
	}
	// 3ヶ月ブランク。
	if got, ok := est.Estimate(h, "bench", last.AddDays(90)); ok {
		t.Errorf("3ヶ月ブランク後に推定できてしまう: %v", got.Kg())
	}
}

// 鮮度は種目ごとに判定すること。
// 別種目を継続していても、その種目自体が古ければ推定してはいけない。
func TestOneRepMaxEstimator_StalenessIsPerExercise(t *testing.T) {
	est := mustEstimator(t, 0.3, 42)
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "old", 1, "bench", 100, 8, 2),
		mkLog(t, "new", 25, "squat", 150, 8, 2),
	})

	if got, ok := est.Estimate(h, "bench", training.MustDate(2026, time.October, 1)); ok {
		t.Errorf("他種目が新しいだけでベンチが推定できてしまう: %v", got.Kg())
	}
}

func TestOneRepMaxEstimator_RejectsZeroAsOf(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	if got, ok := est.Estimate(h, "bench", training.Date{}); ok {
		t.Errorf("基準日が無いのに推定値が返る: %v", got.Kg())
	}
}

// 推定できないセッション（全セット自重）は畳み込みから除くこと。
func TestOneRepMaxEstimator_SkipsUnestimableSessions(t *testing.T) {
	est := mustEstimator(t, 0.5, 42)

	withBodyweight := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "dip", 40, 8, 2),
		mkLog(t, "b", 8, "dip", 0, 12, 1), // 全セット自重の回
		mkLog(t, "c", 15, "dip", 45, 8, 2),
	})
	without := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 1, "dip", 40, 8, 2),
		mkLog(t, "c", 15, "dip", 45, 8, 2),
	})

	a, ok := est.Estimate(withBodyweight, "dip", asOf(16))
	if !ok {
		t.Fatal("推定値が返らない")
	}
	b, _ := est.Estimate(without, "dip", asOf(16))
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

	if got, ok := est.Estimate(h, "dip", asOf(10)); ok {
		t.Errorf("推定できるセッションが無いのに値が返る: %v", got.Kg())
	}
}

func TestNewOneRepMaxEstimator_RejectsBadParams(t *testing.T) {
	for _, c := range []struct {
		alpha        float64
		maxStaleDays int
	}{
		{0, 42}, {-0.1, 42}, {1.1, 42}, {math.NaN(), 42},
		{0.3, 0}, {0.3, -1},
	} {
		if _, err := training.NewOneRepMaxEstimator(c.alpha, c.maxStaleDays); err == nil {
			t.Errorf("不正なパラメータが通ってしまう: alpha=%v maxStaleDays=%d", c.alpha, c.maxStaleDays)
		}
	}
	if _, err := training.NewOneRepMaxEstimator(1.0, 1); err != nil {
		t.Errorf("境界値が弾かれた: %v", err)
	}
}

// 既定値そのものを固定する。定数を変えると追随速度と鮮度の判定が変わる。
func TestDefaultOneRepMaxEstimator_Constants(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()
	if got := est.Alpha(); math.Abs(got-0.3) > 1e-9 {
		t.Errorf("alpha が誤り: got %v, want 0.3", got)
	}
	if got := est.MaxStaleDays(); got != 42 {
		t.Errorf("鮮度の上限が誤り: got %d, want 42", got)
	}
}

func TestOneRepMaxEstimator_ZeroValueIsSafe(t *testing.T) {
	var est training.OneRepMaxEstimator
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	if got, ok := est.Estimate(h, "bench", asOf(12)); ok {
		t.Errorf("ゼロ値の推定器が値を返す: %v", got.Kg())
	}
}

// 返り値は必ずコンストラクタを通った有効な値であること。
func TestOneRepMaxEstimator_ReturnsValidOneRepMax(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()

	logs := make([]*training.SetLog, 0, 28)
	for i := 1; i <= 28; i++ {
		logs = append(logs, mkLog(t, fmt.Sprintf("l%02d", i), i, "bench", 80+float64(i%5)*2.5, 8, 2))
	}

	got, ok := est.Estimate(training.NewHistory(logs), "bench", asOf(29))
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

func TestOneRepMaxEstimator_IsDeterministic(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()

	logs := make([]*training.SetLog, 0, 20)
	for i := 1; i <= 20; i++ {
		logs = append(logs, mkLog(t, fmt.Sprintf("l%02d", i), i, "bench", 80+float64(i), 8, 2))
	}
	h := training.NewHistory(logs)

	first, ok := est.Estimate(h, "bench", asOf(21))
	if !ok {
		t.Fatal("推定値が返らない")
	}
	for range 50 {
		got, _ := est.Estimate(h, "bench", asOf(21))
		if got != first {
			t.Fatalf("呼び出しごとに結果が変わる: %v vs %v", first.Kg(), got.Kg())
		}
	}
}

// ゼロ値の推定器は、どんな入力に対しても値を返さないこと。
//
// alpha=0・鮮度0日なので、記録当日に呼ぶと鮮度判定を通過し、
// EWMA も初回値をそのまま返してしまう。「未設定の設定で計画される」
// のは最も気づきにくい壊れ方なので、入口で止める。
func TestOneRepMaxEstimator_ZeroValueReturnsNothingEvenOnTheSameDay(t *testing.T) {
	var est training.OneRepMaxEstimator
	h := training.NewHistory([]*training.SetLog{mkLog(t, "a", 10, "bench", 85, 9, 2)})

	for _, day := range []int{10, 11, 12, 20} {
		if got, ok := est.Estimate(h, "bench", asOf(day)); ok {
			t.Errorf("8月%d日にゼロ値の推定器が値を返した: %v", day, got.Kg())
		}
	}
}
