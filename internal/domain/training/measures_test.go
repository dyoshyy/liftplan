package training_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mustWeight(t *testing.T, kg float64) training.Weight {
	t.Helper()
	w, err := training.NewWeight(kg)
	if err != nil {
		t.Fatalf("NewWeight(%v): %v", kg, err)
	}
	return w
}

func mustIncrement(t *testing.T, kg float64) training.Increment {
	t.Helper()
	i, err := training.NewIncrement(kg)
	if err != nil {
		t.Fatalf("NewIncrement(%v): %v", kg, err)
	}
	return i
}

func mustRatio(t *testing.T, v float64) training.Ratio {
	t.Helper()
	r, err := training.NewRatio(v)
	if err != nil {
		t.Fatalf("NewRatio(%v): %v", v, err)
	}
	return r
}

func mustIntensity(t *testing.T, v float64) training.IntensityPct {
	t.Helper()
	i, err := training.NewIntensityPct(v)
	if err != nil {
		t.Fatalf("NewIntensityPct(%v): %v", v, err)
	}
	return i
}

// 非有限値はどのコンストラクタでも弾かれること。
// NaN が1つでも通ると、以降の計算がすべて NaN に汚染され、しかも
// 比較がすべて false になるため検証をすり抜けて静かに壊れる。
func TestMeasures_RejectNonFinite(t *testing.T) {
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := training.NewWeight(v); err == nil {
			t.Errorf("NewWeight(%v) が通ってしまう", v)
		}
		if _, err := training.NewIncrement(v); err == nil {
			t.Errorf("NewIncrement(%v) が通ってしまう", v)
		}
		if _, err := training.NewIntensityPct(v); err == nil {
			t.Errorf("NewIntensityPct(%v) が通ってしまう", v)
		}
		if _, err := training.NewRatio(v); err == nil {
			t.Errorf("NewRatio(%v) が通ってしまう", v)
		}
		if _, err := training.NewContribution(v); err == nil {
			t.Errorf("NewContribution(%v) が通ってしまう", v)
		}
	}
}

func TestMeasures_FailureReturnsZeroValue(t *testing.T) {
	// 失敗時に中途半端な値を返すと、err を見落としたときに静かに壊れる。
	if got, _ := training.NewWeight(-1); got.Kg() != 0 {
		t.Errorf("Weight の失敗時にゼロ値でない: %v", got.Kg())
	}
	if got, _ := training.NewIntensityPct(2); got.Float() != 0 {
		t.Errorf("IntensityPct の失敗時にゼロ値でない: %v", got.Float())
	}
	if got, _ := training.NewRatio(0); got.Float() != 0 {
		t.Errorf("Ratio の失敗時にゼロ値でない: %v", got.Float())
	}
	if got, _ := training.NewReps(0); got.Int() != 0 {
		t.Errorf("Reps の失敗時にゼロ値でない: %v", got.Int())
	}
}

func TestWeight_RejectsNegative(t *testing.T) {
	if _, err := training.NewWeight(-0.1); err == nil {
		t.Error("負の重量が通ってしまう")
	}
	// 自重種目を0kgで記録する運用があるため、0は許す。
	if _, err := training.NewWeight(0); err != nil {
		t.Errorf("0kg が弾かれた: %v", err)
	}
}

func TestWeight_RoundTo(t *testing.T) {
	cases := []struct {
		in        float64
		increment float64
		want      float64
	}{
		{83.1, 2.5, 82.5},
		{84.0, 2.5, 85.0},
		{85.0, 2.5, 85.0},
		{83.0, 5.0, 85.0},
		{82.0, 5.0, 80.0},
		{0.0, 2.5, 0.0},
		{1.3, 2.5, 2.5},
		{1.0, 2.5, 0.0},
	}
	for _, c := range cases {
		got := mustWeight(t, c.in).RoundTo(mustIncrement(t, c.increment)).Kg()
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("RoundTo(%v, %v) = %v, want %v", c.in, c.increment, got, c.want)
		}
	}
}

func TestWeight_RoundToProducesCleanValues(t *testing.T) {
	// math.Round(v/inc)*inc は 3.7000000000000006 のような残差を生む。
	// 誤差としては無視できる大きさだが、JSON に出た瞬間に
	// "weight_kg": 3.7000000000000006 という文字列になってユーザーの目に触れる。
	//
	// したがって許容誤差ではなく、10進表記の桁数で検査する。
	cases := []struct {
		increment float64
		maxDigits int
	}{
		{2.5, 1},
		{0.5, 1},
		{0.1, 1},
		{1.25, 2},
	}
	for _, c := range cases {
		inc := mustIncrement(t, c.increment)
		for i := 1; i <= 3000; i++ {
			v := float64(i) * 0.37
			got := mustWeight(t, v).RoundTo(inc).Kg()

			if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > c.maxDigits {
				t.Fatalf("刻み %v で丸めた結果に端数が残っている: %s（元: %v、小数点以下 %d 桁）",
					c.increment, strconv.FormatFloat(got, 'f', -1, 64), v, n)
			}
		}
	}
}

func decimalPlaces(s string) int {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return 0
	}
	return len(s) - dot - 1
}

func TestWeight_RoundToIsIdempotent(t *testing.T) {
	inc := mustIncrement(t, 2.5)
	for i := range 500 {
		once := mustWeight(t, float64(i)*0.73).RoundTo(inc)
		if twice := once.RoundTo(inc); twice != once {
			t.Fatalf("丸めが冪等でない: %v → %v", once.Kg(), twice.Kg())
		}
	}
}

func TestWeight_RoundToNeverGoesNegative(t *testing.T) {
	// 0 に近い重量を大きな刻みで丸めても、負にはならない。
	if got := mustWeight(t, 0.5).RoundTo(mustIncrement(t, 20)).Kg(); got < 0 {
		t.Errorf("丸めで負になった: %v", got)
	}
}

func TestWeight_RoundToWithZeroIncrementIsSafe(t *testing.T) {
	// ゼロ値の Increment が混入しても0除算で落ちないこと。
	var zero training.Increment
	w := mustWeight(t, 83.1)
	if got := w.RoundTo(zero); got != w {
		t.Errorf("ゼロ値の刻みで値が変わった: %v", got.Kg())
	}
}

func TestReps_MustBePositive(t *testing.T) {
	for _, v := range []int{0, -1, -100} {
		if _, err := training.NewReps(v); err == nil {
			t.Errorf("%d レップが通ってしまう", v)
		}
	}
	r, err := training.NewReps(8)
	if err != nil {
		t.Fatalf("正常値が失敗: %v", err)
	}
	if r.Int() != 8 {
		t.Errorf("got %d, want 8", r.Int())
	}
}

func TestRIR_RejectsNegative(t *testing.T) {
	if _, err := training.NewRIR(-1); err == nil {
		t.Error("負のRIRが通ってしまう")
	}
	// 限界まで追い込んだセットは RIR 0。
	if _, err := training.NewRIR(0); err != nil {
		t.Errorf("RIR 0 が弾かれた: %v", err)
	}
}

func TestRIR_PlusStaysInRange(t *testing.T) {
	r, err := training.NewRIR(2)
	if err != nil {
		t.Fatalf("NewRIR: %v", err)
	}
	if got := r.Plus(1).Int(); got != 3 {
		t.Errorf("got %d, want 3", got)
	}
	if got := r.Plus(0).Int(); got != 2 {
		t.Errorf("got %d, want 2", got)
	}
	if got := r.Plus(-10).Int(); got != 0 {
		t.Errorf("下限で丸められていない: got %d, want 0", got)
	}
}

func TestRIR_PlusDoesNotMutateReceiver(t *testing.T) {
	r, _ := training.NewRIR(2)
	_ = r.Plus(5)
	if r.Int() != 2 {
		t.Errorf("元の値が書き換わっている: %d", r.Int())
	}
}

func TestIntensityPct_Range(t *testing.T) {
	for _, v := range []float64{0, -0.1, 1.0001, 1.5, 100} {
		if _, err := training.NewIntensityPct(v); err == nil {
			t.Errorf("%v が通ってしまう", v)
		}
	}
	// 1RM そのものを指す 1.0 は正当。
	if _, err := training.NewIntensityPct(1.0); err != nil {
		t.Errorf("1.0 が弾かれた: %v", err)
	}
}

func TestIntensityPct_Scale(t *testing.T) {
	i := mustIntensity(t, 0.81)
	if got := i.Scale(mustRatio(t, 0.9)).Float(); math.Abs(got-0.729) > 1e-9 {
		t.Errorf("got %v, want 0.729", got)
	}
}

func TestIntensityPct_ScaleNeverExceedsOne(t *testing.T) {
	// Ratio は 1.2 まで許すため、強度が 1.0 を超えうる。1RM超は意味を持たない。
	i := mustIntensity(t, 0.95)
	if got := i.Scale(mustRatio(t, 1.2)).Float(); got > 1.0 {
		t.Errorf("強度が1.0を超えた: %v", got)
	}
}

func TestIntensityPct_ScaleNeverReachesZero(t *testing.T) {
	// 引数を検証済みの Ratio に限ることで、結果が0以下になる経路を型で塞いでいる。
	i := mustIntensity(t, 0.76)
	for _, f := range []float64{0.0001, 0.5, 0.9, 1.0, 1.2} {
		if got := i.Scale(mustRatio(t, f)).Float(); got <= 0 {
			t.Errorf("Scale(%v) が0以下になった: %v", f, got)
		}
	}
}

func TestIntensityPct_ScaleDoesNotMutateReceiver(t *testing.T) {
	i := mustIntensity(t, 0.81)
	_ = i.Scale(mustRatio(t, 0.5))
	if math.Abs(i.Float()-0.81) > 1e-9 {
		t.Errorf("元の値が書き換わっている: %v", i.Float())
	}
}

func TestRatio_Range(t *testing.T) {
	for _, v := range []float64{0, -0.5, 1.21, 3} {
		if _, err := training.NewRatio(v); err == nil {
			t.Errorf("%v が通ってしまう", v)
		}
	}
	for _, v := range []float64{0.7, 0.85, 1.0, 1.2} {
		if _, err := training.NewRatio(v); err != nil {
			t.Errorf("%v が弾かれた: %v", v, err)
		}
	}
}

func TestSetCount_MustBePositive(t *testing.T) {
	for _, v := range []int{0, -1} {
		if _, err := training.NewSetCount(v); err == nil {
			t.Errorf("%d セットが通ってしまう", v)
		}
	}
	s, err := training.NewSetCount(4)
	if err != nil {
		t.Fatalf("正常値が失敗: %v", err)
	}
	if s.Int() != 4 {
		t.Errorf("got %d, want 4", s.Int())
	}
}

func TestContribution_Range(t *testing.T) {
	for _, v := range []float64{0, -0.1, 1.1, 2} {
		if _, err := training.NewContribution(v); err == nil {
			t.Errorf("%v が通ってしまう", v)
		}
	}
	for _, v := range []float64{0.1, 0.5, 1.0} {
		if _, err := training.NewContribution(v); err != nil {
			t.Errorf("%v が弾かれた: %v", v, err)
		}
	}
}

// すべての値オブジェクトが比較可能で、同じ入力から同じ値を作れること。
// マップのキーや == による比較に使われるため、これが崩れると重複判定が壊れる。
func TestMeasures_AreComparable(t *testing.T) {
	if mustWeight(t, 85.0) != mustWeight(t, 85.0) {
		t.Error("同じ重量が等しくない")
	}
	if mustIncrement(t, 2.5) != mustIncrement(t, 2.5) {
		t.Error("同じ増加単位が等しくない")
	}
	if mustIntensity(t, 0.81) != mustIntensity(t, 0.81) {
		t.Error("同じ強度が等しくない")
	}
	if mustRatio(t, 0.9) != mustRatio(t, 0.9) {
		t.Error("同じ比率が等しくない")
	}

	// 演算を経由しても、同じ結果なら等しくなる。
	a := mustIntensity(t, 0.9).Scale(mustRatio(t, 0.9))
	b := mustIntensity(t, 0.81)
	if a != b {
		t.Errorf("演算結果が等値にならない: %v vs %v", a.Float(), b.Float())
	}
}

// ゼロ値は「未設定」であり、有効な値と混同されないこと。
func TestMeasures_ZeroValuesAreDistinguishable(t *testing.T) {
	var w training.Weight
	if w.Kg() != 0 {
		t.Errorf("Weight のゼロ値が0でない: %v", w.Kg())
	}
	var i training.IntensityPct
	if i.Float() != 0 {
		t.Errorf("IntensityPct のゼロ値が0でない: %v", i.Float())
	}
	// 強度と比率と寄与度は0を正当な値として持たないので、ゼロ値は必ず無効。
	if _, err := training.NewIntensityPct(i.Float()); err == nil {
		t.Error("IntensityPct のゼロ値が正当な値として作り直せてしまう")
	}
	var r training.Ratio
	if _, err := training.NewRatio(r.Float()); err == nil {
		t.Error("Ratio のゼロ値が正当な値として作り直せてしまう")
	}
	var c training.Contribution
	if _, err := training.NewContribution(c.Float()); err == nil {
		t.Error("Contribution のゼロ値が正当な値として作り直せてしまう")
	}
	var s training.SetCount
	if _, err := training.NewSetCount(s.Int()); err == nil {
		t.Error("SetCount のゼロ値が正当な値として作り直せてしまう")
	}
}
