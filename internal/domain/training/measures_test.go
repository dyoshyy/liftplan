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

func mustRoundTo(t *testing.T, w training.Weight, inc training.Increment) training.Weight {
	t.Helper()
	got, err := w.RoundTo(inc)
	if err != nil {
		t.Fatalf("RoundTo(%v, %v): %v", w.Kg(), inc.Kg(), err)
	}
	return got
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
		got := mustRoundTo(t, mustWeight(t, c.in), mustIncrement(t, c.increment)).Kg()
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
		for i := 1; i <= 2700; i++ {
			v := float64(i) * 0.37
			got := mustRoundTo(t, mustWeight(t, v), inc).Kg()

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
		once := mustRoundTo(t, mustWeight(t, float64(i)*0.73), inc)
		if twice := mustRoundTo(t, once, inc); twice != once {
			t.Fatalf("丸めが冪等でない: %v → %v", once.Kg(), twice.Kg())
		}
	}
}

func TestWeight_RoundToNeverGoesNegative(t *testing.T) {
	// 0 に近い重量を大きな刻みで丸めても、負にはならない。
	if got := mustRoundTo(t, mustWeight(t, 0.5), mustIncrement(t, 20)).Kg(); got < 0 {
		t.Errorf("丸めで負になった: %v", got)
	}
}

func TestWeight_RoundToRejectsZeroIncrement(t *testing.T) {
	// 黙って丸めずに返すと、バーに載らない半端な重量がそのまま処方され、
	// しかも誰も気づけない。0除算を避けるだけでは足りない。
	var zero training.Increment
	got, err := mustWeight(t, 83.1).RoundTo(zero)
	if err == nil {
		t.Fatalf("ゼロ値の刻みが通ってしまう: %v", got.Kg())
	}
	if !got.IsZero() {
		t.Errorf("失敗時にゼロ値でない値が返る: %v", got.Kg())
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

func TestIntensityPct_Reduce(t *testing.T) {
	i := mustIntensity(t, 0.9)
	if got := i.Reduce(0.1).Float(); math.Abs(got-0.81) > 1e-9 {
		t.Errorf("got %v, want 0.81", got)
	}
}

func TestIntensityPct_ReduceNeverReachesZero(t *testing.T) {
	// 強度が0に潰れると 0kg のセットが処方される。「履歴が無ければ重量を返さない」
	// という設計を、0kg という捏造で貫通させないための下限。
	i := mustIntensity(t, 0.76)
	for _, pct := range []float64{0.1, 0.5, 0.9, 1.0, 100, 1e9} {
		if got := i.Reduce(pct).Float(); got <= 0 {
			t.Errorf("Reduce(%v) が0以下になった: %v", pct, got)
		}
	}
}

func TestIntensityPct_ReduceIgnoresMeaninglessInput(t *testing.T) {
	// 不正な入力で黙って強度が変わるより、何もしない方が安全。
	i := mustIntensity(t, 0.81)
	for _, pct := range []float64{0, -0.5, math.NaN()} {
		if got := i.Reduce(pct); got != i {
			t.Errorf("Reduce(%v) で値が変わった: %v", pct, got.Float())
		}
	}
}

func TestIntensityPct_ReduceIsCapped(t *testing.T) {
	// 設定ミスで極端な低下率が入っても、トレーニングとして意味のある強度を保つ。
	i := mustIntensity(t, 0.8)
	if got := i.Reduce(0.99).Float(); got < 0.3 {
		t.Errorf("低下率が丸められていない: %v", got)
	}
}

func TestIntensityPct_ReduceDoesNotMutateReceiver(t *testing.T) {
	i := mustIntensity(t, 0.81)
	_ = i.Reduce(0.5)
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
	a := mustIntensity(t, 0.9).Reduce(0.1)
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

// 上限・下限の定数そのものを固定する。
// 「1.2 は通る／1.21 は落ちる」だけでは、定数を 1.205 にする変更を検出できない。
func TestMeasures_BoundaryConstants(t *testing.T) {
	cases := []struct {
		name       string
		construct  func(float64) error
		min, max   float64
		belowMin   float64
		aboveMax   float64
		minAllowed bool
	}{
		{
			name:      "Ratio",
			construct: func(v float64) error { _, err := training.NewRatio(v); return err },
			max:       1.2, aboveMax: 1.200001,
		},
		{
			name:      "IntensityPct",
			construct: func(v float64) error { _, err := training.NewIntensityPct(v); return err },
			max:       1.0, aboveMax: 1.000001,
		},
		{
			name:      "Contribution",
			construct: func(v float64) error { _, err := training.NewContribution(v); return err },
			max:       1.0, aboveMax: 1.000001,
		},
		{
			name:      "Weight",
			construct: func(v float64) error { _, err := training.NewWeight(v); return err },
			max:       1000, aboveMax: 1000.000001,
		},
		{
			name:      "Increment",
			construct: func(v float64) error { _, err := training.NewIncrement(v); return err },
			max:       50, aboveMax: 50.000001,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.construct(c.max); err != nil {
				t.Errorf("上限ちょうど %v が弾かれた: %v", c.max, err)
			}
			if err := c.construct(c.aboveMax); err == nil {
				t.Errorf("上限をわずかに超える %v が通ってしまう", c.aboveMax)
			}
		})
	}
}

func TestMeasures_IntegerBoundaryConstants(t *testing.T) {
	cases := []struct {
		name      string
		construct func(int) error
		max       int
	}{
		{"Reps", func(v int) error { _, err := training.NewReps(v); return err }, 1000},
		{"RIR", func(v int) error { _, err := training.NewRIR(v); return err }, 100},
		{"SetCount", func(v int) error { _, err := training.NewSetCount(v); return err }, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.construct(c.max); err != nil {
				t.Errorf("上限ちょうど %d が弾かれた: %v", c.max, err)
			}
			if err := c.construct(c.max + 1); err == nil {
				t.Errorf("上限を超える %d が通ってしまう", c.max+1)
			}
		})
	}
}

// 上限が無いと、Epley 式の reps+rir が int を溢れて負になり推定1RMが負になる。
// RIR.Plus も同様にラップする。
func TestMeasures_RejectOverflowInducingValues(t *testing.T) {
	if _, err := training.NewReps(math.MaxInt); err == nil {
		t.Error("MaxInt のレップ数が通ってしまう")
	}
	if _, err := training.NewRIR(math.MaxInt); err == nil {
		t.Error("MaxInt の RIR が通ってしまう")
	}
	if _, err := training.NewSetCount(math.MaxInt); err == nil {
		t.Error("MaxInt のセット数が通ってしまう")
	}

	r, err := training.NewRIR(2)
	if err != nil {
		t.Fatalf("NewRIR: %v", err)
	}
	// オーバーフローすると int がラップして負になり、その後の下限クランプで
	// 0 になる。0 は「限界まで追い込む」という正反対の指示なので、
	// 「負でない」だけでは不十分。増える方向に丸まることを確かめる。
	if got := r.Plus(math.MaxInt).Int(); got <= r.Int() {
		t.Errorf("Plus がオーバーフローして値が減った: %d → %d", r.Int(), got)
	}
}

func TestNewIncrement_RejectsInvalid(t *testing.T) {
	// 0 や負の刻みが通ると、以降すべての丸めが壊れる。
	for _, v := range []float64{0, -0.1, -2.5, 51, 1e9} {
		if _, err := training.NewIncrement(v); err == nil {
			t.Errorf("不正な増加単位が通ってしまう: %v", v)
		}
	}
	for _, v := range []float64{0.5, 1.0, 2.5, 5.0, 20} {
		if _, err := training.NewIncrement(v); err != nil {
			t.Errorf("正当な増加単位が弾かれた: %v (%v)", v, err)
		}
	}
}

// 量子化はコンストラクタの検証より前に行われる必要がある。
//
// 後に行うと、検証を通った値が量子化で +Inf になったり 0 に潰れたりして、
// コンストラクタが自分で不変条件を破る。
func TestMeasures_QuantizationHappensBeforeValidation(t *testing.T) {
	// 極端に大きい値: v*quantum がオーバーフローして +Inf になる領域。
	for _, v := range []float64{1e303, 1.7976931348623157e308} {
		if w, err := training.NewWeight(v); err == nil {
			t.Errorf("巨大な重量 %v が通ってしまう（値: %v）", v, w.Kg())
		}
		if i, err := training.NewIncrement(v); err == nil {
			t.Errorf("巨大な増加単位 %v が通ってしまう（値: %v）", v, i.Kg())
		}
	}

	// 極端に小さい値: 量子化で 0 に潰れる領域。
	for _, v := range []float64{1e-7, 4.9e-7, 1e-300} {
		if r, err := training.NewRatio(v); err == nil {
			t.Errorf("量子化で0に潰れる比率 %v が通ってしまう（値: %v）", v, r.Float())
		}
		if i, err := training.NewIntensityPct(v); err == nil {
			t.Errorf("量子化で0に潰れる強度 %v が通ってしまう（値: %v）", v, i.Float())
		}
		if c, err := training.NewContribution(v); err == nil {
			t.Errorf("量子化で0に潰れる寄与度 %v が通ってしまう（値: %v）", v, c.Float())
		}
		if inc, err := training.NewIncrement(v); err == nil {
			t.Errorf("量子化で0に潰れる増加単位 %v が通ってしまう（値: %v）", v, inc.Kg())
		}
	}
}

// 生成された値は必ず有限であること。これが崩れると以降の計算が NaN/Inf に汚染される。
func TestMeasures_ConstructedValuesAreAlwaysFinite(t *testing.T) {
	inputs := []float64{
		0, 1e-300, 1e-7, 0.001, 0.5, 1, 2.5, 100, 999.999, 1e6, 1e100, 1e303,
		-1, -1e300, math.NaN(), math.Inf(1), math.Inf(-1),
	}
	for _, v := range inputs {
		if w, err := training.NewWeight(v); err == nil && (math.IsInf(w.Kg(), 0) || math.IsNaN(w.Kg())) {
			t.Errorf("NewWeight(%v) が非有限値を返した: %v", v, w.Kg())
		}
		if i, err := training.NewIncrement(v); err == nil && (math.IsInf(i.Kg(), 0) || math.IsNaN(i.Kg())) {
			t.Errorf("NewIncrement(%v) が非有限値を返した: %v", v, i.Kg())
		}
		if r, err := training.NewRatio(v); err == nil && (math.IsInf(r.Float(), 0) || math.IsNaN(r.Float())) {
			t.Errorf("NewRatio(%v) が非有限値を返した: %v", v, r.Float())
		}
	}
}

// RoundTo の結果も必ず有限で、上限を超えないこと。
func TestWeight_RoundToStaysValid(t *testing.T) {
	for _, incKg := range []float64{0.5, 2.5, 5, 20, 50} {
		inc := mustIncrement(t, incKg)
		for _, wKg := range []float64{0, 0.1, 83.1, 500, 999.9, 1000} {
			got, err := mustWeight(t, wKg).RoundTo(inc)
			if err != nil {
				t.Fatalf("RoundTo(%v, %v): %v", wKg, incKg, err)
			}
			if math.IsInf(got.Kg(), 0) || math.IsNaN(got.Kg()) {
				t.Errorf("RoundTo(%v, %v) が非有限値: %v", wKg, incKg, got.Kg())
			}
			if got.Kg() < 0 {
				t.Errorf("RoundTo(%v, %v) が負: %v", wKg, incKg, got.Kg())
			}
		}
	}
}

// エラーメッセージはユーザーが HTTP 400 のボディで直接読む。
// どの項目が不正なのか分からないメッセージは、無いのと変わらない。
func TestMeasures_ErrorMessagesIdentifyTheField(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"Weight", errOf(func() error { _, err := training.NewWeight(-1); return err }), "重量"},
		{"Increment", errOf(func() error { _, err := training.NewIncrement(0); return err }), "増加単位"},
		{"IntensityPct", errOf(func() error { _, err := training.NewIntensityPct(2); return err }), "強度"},
		{"Ratio", errOf(func() error { _, err := training.NewRatio(0); return err }), "比率"},
		{"Contribution", errOf(func() error { _, err := training.NewContribution(0); return err }), "寄与度"},
		{"Reps", errOf(func() error { _, err := training.NewReps(0); return err }), "レップ数"},
		{"RIR", errOf(func() error { _, err := training.NewRIR(-1); return err }), "RIR"},
		{"SetCount", errOf(func() error { _, err := training.NewSetCount(0); return err }), "セット数"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil {
				t.Fatal("エラーが返らない")
			}
			if !strings.Contains(c.err.Error(), c.want) {
				t.Errorf("エラーメッセージに %q が含まれない: %v", c.want, c.err)
			}
		})
	}
}

func errOf(f func() error) error { return f() }

func TestContribution_TimesSets(t *testing.T) {
	c, err := training.NewContribution(0.5)
	if err != nil {
		t.Fatalf("NewContribution: %v", err)
	}
	s, err := training.NewSetCount(3)
	if err != nil {
		t.Fatalf("NewSetCount: %v", err)
	}
	if got := c.TimesSets(s); math.Abs(got-1.5) > 1e-9 {
		t.Errorf("got %v, want 1.5", got)
	}

	// 端数が残らないこと。刺激量は残差テーブルに積み上げられ、最終的に比較される。
	c2, _ := training.NewContribution(0.3)
	s2, _ := training.NewSetCount(3)
	if got := c2.TimesSets(s2); decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)) > 6 {
		t.Errorf("端数が残っている: %v", got)
	}
}

// コンストラクタが値を量子化していること。
//
// 量子化しないと、同じ意味の値が別々のビット列になって == が成立しなくなり、
// 端数が JSON にそのまま出る。
func TestMeasures_ConstructorsQuantize(t *testing.T) {
	const maxDigits = 6

	check := func(t *testing.T, name string, v float64) {
		t.Helper()
		if n := decimalPlaces(strconv.FormatFloat(v, 'f', -1, 64)); n > maxDigits {
			t.Errorf("%s が量子化されていない: %s（小数点以下 %d 桁）",
				name, strconv.FormatFloat(v, 'f', -1, 64), n)
		}
	}

	r, err := training.NewRatio(0.12345678901234)
	if err != nil {
		t.Fatalf("NewRatio: %v", err)
	}
	check(t, "Ratio", r.Float())

	c, err := training.NewContribution(0.12345678901234)
	if err != nil {
		t.Fatalf("NewContribution: %v", err)
	}
	check(t, "Contribution", c.Float())

	i, err := training.NewIntensityPct(0.12345678901234)
	if err != nil {
		t.Fatalf("NewIntensityPct: %v", err)
	}
	check(t, "IntensityPct", i.Float())

	w, err := training.NewWeight(83.12345678901234)
	if err != nil {
		t.Fatalf("NewWeight: %v", err)
	}
	check(t, "Weight", w.Kg())

	inc, err := training.NewIncrement(2.51234567890123)
	if err != nil {
		t.Fatalf("NewIncrement: %v", err)
	}
	check(t, "Increment", inc.Kg())
}

// 同じ意味の値が、計算経路の違いで別のビット列にならないこと。
func TestMeasures_QuantizationMakesEquivalentValuesEqual(t *testing.T) {
	a, err := training.NewRatio(0.1 + 0.2)
	if err != nil {
		t.Fatalf("NewRatio: %v", err)
	}
	b, err := training.NewRatio(0.3)
	if err != nil {
		t.Fatalf("NewRatio: %v", err)
	}
	if a != b {
		t.Errorf("同じ意味の比率が等しくない: %v vs %v", a.Float(), b.Float())
	}
}

// 表現できる最小の強度から低下させても0に潰れないこと。
// 潰れると 0kg のセットが hasWeight=true で処方される。
func TestIntensityPct_ReduceFromSmallestValue(t *testing.T) {
	smallest, err := training.NewIntensityPct(1e-6)
	if err != nil {
		t.Fatalf("最小の強度が作れない: %v", err)
	}
	for _, pct := range []float64{0.5, 0.9, 1.0} {
		if got := smallest.Reduce(pct).Float(); got <= 0 {
			t.Errorf("Reduce(%v) が0に潰れた: %v", pct, got)
		}
	}
}

// Reduce の結果は、どんな有効な強度・低下率の組み合わせでも0にならない。
//
// 強度が0に潰れると 0kg のセットが処方され、「履歴が無ければ重量を返さない」
// という設計を 0kg という捏造で貫通する。
// 低下率の上限を 0.5 に抑えることで、最小の強度に適用しても量子化で切り上がる。
func TestIntensityPct_ReduceNeverCollapsesToZero(t *testing.T) {
	intensities := []float64{1e-6, 1e-5, 0.001, 0.1, 0.5, 0.76, 0.88, 1.0}
	reductions := []float64{1e-9, 0.001, 0.1, 0.5, 0.9, 1.0, 1e9}

	for _, iv := range intensities {
		i, err := training.NewIntensityPct(iv)
		if err != nil {
			t.Fatalf("NewIntensityPct(%v): %v", iv, err)
		}
		for _, pct := range reductions {
			got := i.Reduce(pct)
			if got.Float() <= 0 {
				t.Errorf("Reduce(%v) が0以下になった（元: %v）: %v", pct, iv, got.Float())
			}
			// 結果が有効な強度として作り直せること。
			if _, err := training.NewIntensityPct(got.Float()); err != nil {
				t.Errorf("Reduce(%v) の結果が無効な強度（元: %v）: %v", pct, iv, err)
			}
		}
	}
}
