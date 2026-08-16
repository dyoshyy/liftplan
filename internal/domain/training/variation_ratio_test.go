package training_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func larsen(t *testing.T, seedRatio float64) *training.Exercise {
	t.Helper()
	return mustExercise(t, training.ExerciseParams{
		ID:                 "larsen",
		Name:               "ラーセンプレス",
		Kind:               training.KindVariation,
		Stimulus:           map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg:        2.5,
		MainLift:           training.LiftBench,
		DefaultRatioToMain: seedRatio,
	})
}

// baseDay は履歴の起点。8月1日を1日目とする。
func baseDay(day int) training.Date {
	return training.MustDate(2026, time.August, 1).AddDays(day - 1)
}

// weekly は指定種目を週1回ずつ n セッション記録する。weight は回ごとの重量。
func weekly(t *testing.T, prefix, exercise string, startDay, n int, weight func(i int) float64) []*training.SetLog {
	t.Helper()
	out := make([]*training.SetLog, 0, n)
	for i := range n {
		out = append(out, mkLogOn(t, fmt.Sprintf("%s%02d", prefix, i),
			baseDay(startDay+i*7), exercise, weight(i), 8, 2))
	}
	return out
}

func flat(kg float64) func(int) float64 { return func(int) float64 { return kg } }

// これがこのサービスの存在理由。
//
// 処方は「メインの推定1RM × 強度 × 係数」で決まる。もし係数を
// 「現在のバリエーション1RM ÷ 現在のメイン1RM」で求めると、掛け算で
// メインの1RMが約分されて消え、処方がバリエーション自身の1RMだけで決まる。
// するとメインが伸びてもバリエーションが一切追随しない固定点に落ちる。
func TestVariationRatioResolver_VariationFollowsMainProgression(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	est := training.DefaultOneRepMaxEstimator()
	intensity := mustIntensity(t, 0.75)
	inc := mustIncrement(t, 2.5)

	// メインは毎週 2.5kg 伸びる。バリエーションは同じ比率で伸びている。
	const weeks = 8
	logs := append(
		weekly(t, "m", "bench", 1, weeks, func(i int) float64 { return 100 + float64(i)*2.5 }),
		weekly(t, "v", "larsen", 3, weeks, func(i int) float64 { return 85 + float64(i)*2.125 })...,
	)
	h := training.NewHistory(logs)

	prescribe := func(day int) float64 {
		t.Helper()
		date := baseDay(day)
		mainORM, ok := est.Estimate(h, "bench", date)
		if !ok {
			t.Fatalf("%v でメインの推定1RMが取れない", date)
		}
		ratio := r.Resolve(h, larsen(t, 0.85), "bench", date)
		w, err := mainORM.WorkWeight(intensity, ratio, inc)
		if err != nil {
			t.Fatalf("%v で処方できない: %v", date, err)
		}
		return w.Kg()
	}

	early := prescribe(24)
	late := prescribe(3 + (weeks-1)*7)

	if late <= early {
		t.Errorf("メインが伸びてもバリエーションの処方が追随しない: %v → %v", early, late)
	}
}

// 固定点に落ちないこと。
//
// バリエーションだけ一時的に軽く（怪我明けなど）記録したあと、メインが
// 伸び続けても処方が凍結してしまうと、いつまでも軽い重量が出続ける。
func TestVariationRatioResolver_DoesNotLockIn(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	est := training.DefaultOneRepMaxEstimator()
	intensity := mustIntensity(t, 0.75)
	inc := mustIncrement(t, 2.5)

	// バリエーションは3回だけ、メインの約6割で記録して以後やっていない…
	// のではなく、以後も同じ比率を保つ前提でメインだけ伸ばす。
	logs := append(
		weekly(t, "m", "bench", 1, 10, func(i int) float64 { return 120 + float64(i)*2.5 }),
		weekly(t, "v", "larsen", 3, 10, func(i int) float64 { return 72 + float64(i)*1.5 })...,
	)
	h := training.NewHistory(logs)

	weights := make([]float64, 0, 3)
	for _, day := range []int{24, 45, 64} {
		date := baseDay(day)
		mainORM, ok := est.Estimate(h, "bench", date)
		if !ok {
			t.Fatalf("%v でメインの推定1RMが取れない", date)
		}
		w, err := mainORM.WorkWeight(intensity, r.Resolve(h, larsen(t, 0.85), "bench", date), inc)
		if err != nil {
			t.Fatalf("%v で処方できない: %v", date, err)
		}
		weights = append(weights, w.Kg())
	}

	if weights[0] == weights[1] && weights[1] == weights[2] {
		t.Errorf("処方が凍結している: %v", weights)
	}
	if !(weights[0] < weights[2]) {
		t.Errorf("メインの伸びに追随していない: %v", weights)
	}
}

// 係数は「同時点で観測した組の比」を平滑したものであって、
// 「現在のバリエーション1RM ÷ 現在のメイン1RM」ではない。
//
// 後者だと、処方の式（メイン1RM × 強度 × 係数）でメインの1RMが約分されて消え、
// 処方がバリエーション自身の1RMだけで決まる。するとバリエーションが停滞した
// とき、メインがいくら伸びても処方が押し上がらない。
//
// バリエーションが横ばい・メインが伸びている履歴で、この2つは違う答えを出す。
func TestVariationRatioResolver_PushesStagnatingVariation(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	est := training.DefaultOneRepMaxEstimator()
	intensity := mustIntensity(t, 0.75)
	inc := mustIncrement(t, 2.5)

	// メインは毎週 2.5kg 伸びる。バリエーションは 80kg で横ばい。
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 8, func(i int) float64 { return 100 + float64(i)*2.5 }),
		weekly(t, "v", "larsen", 3, 8, flat(80))...))

	date := baseDay(3 + 7*7)
	mainORM, ok := est.Estimate(h, "bench", date)
	if !ok {
		t.Fatal("メインの推定1RMが取れない")
	}
	variationORM, ok := est.Estimate(h, "larsen", date)
	if !ok {
		t.Fatal("バリエーションの推定1RMが取れない")
	}

	got, err := mainORM.WorkWeight(intensity, r.Resolve(h, larsen(t, 0.85), "bench", date), inc)
	if err != nil {
		t.Fatalf("処方できない: %v", err)
	}

	// 「現在の1RMどうしの比」を係数にすると、処方はバリエーション自身の
	// 1RM × 強度 に一致してしまう。
	selfReferential, err := variationORM.WorkWeight(intensity, unitRatioForTest(t), inc)
	if err != nil {
		t.Fatalf("比較値を算出できない: %v", err)
	}

	if got.Kg() <= selfReferential.Kg() {
		t.Errorf("停滞したバリエーションが押し上がらない: 処方 %v、自己参照 %v",
			got.Kg(), selfReferential.Kg())
	}
}

func unitRatioForTest(t *testing.T) training.Ratio {
	t.Helper()
	r, err := training.NewRatio(1.0)
	if err != nil {
		t.Fatalf("NewRatio(1.0): %v", err)
	}
	return r
}

// 係数の平滑は直近の組ほど重く効くこと。
func TestVariationRatioResolver_RecentPairsDominate(t *testing.T) {
	r := training.DefaultVariationRatioResolver()

	// メインは 100kg 固定。バリエーションは 90 → 70 と落ちていく。
	declining := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 5, flat(100)),
		weekly(t, "v", "larsen", 2, 5, func(i int) float64 { return 90 - float64(i)*5 })...))
	// 逆に 70 → 90 と上がっていく。
	rising := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 5, flat(100)),
		weekly(t, "v", "larsen", 2, 5, func(i int) float64 { return 70 + float64(i)*5 })...))

	down := r.Resolve(declining, larsen(t, 0.85), "bench", asOf(32))
	up := r.Resolve(rising, larsen(t, 0.85), "bench", asOf(32))

	if !(down.Float() < up.Float()) {
		t.Errorf("直近の組が支配的でない: 下降 %v, 上昇 %v", down.Float(), up.Float())
	}

	// 単純平均なら両者は一致する（同じ値の集合を並べ替えただけ）。
	if math.Abs(down.Float()-up.Float()) < 1e-6 {
		t.Errorf("平滑が順序を見ていない: %v vs %v", down.Float(), up.Float())
	}
}

func TestVariationRatioResolver_FallsBackWhenTooFewSessions(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 3, flat(100)),
		weekly(t, "v", "larsen", 2, 2, flat(85))...))

	if got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(20)); math.Abs(got.Float()-0.9) > 1e-9 {
		t.Errorf("初期値にフォールバックしていない: %v", got.Float())
	}
}

// 境界。minSessions ちょうどで実測に切り替わること。
func TestVariationRatioResolver_SwitchesAtMinSessions(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	if r.MinSessions() != 3 {
		t.Fatalf("既定の minSessions が誤り: %d", r.MinSessions())
	}
	main := weekly(t, "m", "bench", 1, 4, flat(100))

	two := training.NewHistory(append(append([]*training.SetLog{}, main...),
		weekly(t, "v", "larsen", 2, 2, flat(80))...))
	if got := r.Resolve(two, larsen(t, 0.9), "bench", asOf(20)); math.Abs(got.Float()-0.9) > 1e-9 {
		t.Errorf("2セッションで実測に切り替わっている: %v", got.Float())
	}

	three := training.NewHistory(append(append([]*training.SetLog{}, main...),
		weekly(t, "v", "larsen", 2, 3, flat(80))...))
	if got := r.Resolve(three, larsen(t, 0.9), "bench", asOf(20)); math.Abs(got.Float()-0.8) > 1e-3 {
		t.Errorf("3セッションで実測に切り替わっていない: %v", got.Float())
	}
}

func TestVariationRatioResolver_UsesMeasuredRatio(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 4, flat(100)),
		weekly(t, "v", "larsen", 2, 4, flat(80))...))

	// 同じレップ・RIR なので、比は重量の比に一致する。
	if got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(25)); math.Abs(got.Float()-0.8) > 1e-3 {
		t.Errorf("実測値になっていない: %v", got.Float())
	}
}

// 引数の順序を取り違えると係数が逆数になる。
// 実際の係数域（0.85〜0.95）では逆数も範囲内に入るため、範囲検査では防げない。
func TestVariationRatioResolver_RatioIsVariationOverMain(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 4, flat(100)),
		weekly(t, "v", "larsen", 2, 4, flat(85))...))

	got := r.Resolve(h, larsen(t, 0.95), "bench", asOf(25))
	if math.Abs(got.Float()-0.85) > 1e-3 {
		t.Errorf("バリエーション/メイン になっていない: got %v, want 0.85", got.Float())
	}
}

// 実測比が1を超える場合も、対メイン係数として妥当な範囲なら採用すること。
func TestVariationRatioResolver_AcceptsRatioAboveOne(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 4, flat(100)),
		weekly(t, "v", "larsen", 2, 4, flat(105))...))

	if got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(25)); math.Abs(got.Float()-1.05) > 1e-3 {
		t.Errorf("1を超える実測比が採用されていない: %v", got.Float())
	}
}

// 対メイン係数として現実的でない実測は捨てること。
//
// NewRatio の範囲（1e-6〜1.2）は比率として成立するかしか見ておらず、
// ベンチ200kgの人にラーセン2.5kgを処方する係数（0.0139）が通ってしまう。
func TestVariationRatioResolver_RejectsRatiosOutsideTheRealisticBand(t *testing.T) {
	r := training.DefaultVariationRatioResolver()

	cases := []struct {
		name         string
		variationKg  float64
		wantFallback bool
	}{
		{"半分未満は別種目", 40, true},
		{"極端に軽い", 2.5, true},
		{"下限のすぐ上", 55, false},
		{"上限を超える", 115, true},
		{"上限のすぐ下", 105, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := training.NewHistory(append(
				weekly(t, "m", "bench", 1, 4, flat(100)),
				weekly(t, "v", "larsen", 2, 4, flat(c.variationKg))...))

			got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(25))
			isFallback := math.Abs(got.Float()-0.9) < 1e-9
			if isFallback != c.wantFallback {
				t.Errorf("バリエーション %vkg: フォールバック=%v（期待 %v）係数=%v",
					c.variationKg, isFallback, c.wantFallback, got.Float())
			}
		})
	}
}

func TestVariationRatioResolver_FallsBackWhenMainMissing(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(weekly(t, "v", "larsen", 2, 4, flat(80)))

	if got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(25)); math.Abs(got.Float()-0.9) > 1e-9 {
		t.Errorf("メイン履歴が無いのに実測を使っている: %v", got.Float())
	}
}

// メインの記録がバリエーションより後にしか無い場合、過去の組は作れない。
// 未来の記録を混ぜて過去の比を求めてはいけない。
func TestVariationRatioResolver_DoesNotUseFutureMainRecords(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		weekly(t, "v", "larsen", 1, 3, flat(80)),
		weekly(t, "m", "bench", 25, 3, flat(100))...))

	if got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(40)); math.Abs(got.Float()-0.9) > 1e-9 {
		t.Errorf("バリエーションより後のメイン記録が使われている: %v", got.Float())
	}
}

// 鮮度はバリエーション側とメイン側で独立に効くこと。
func TestVariationRatioResolver_StalenessIsCheckedOnBothSides(t *testing.T) {
	r := training.DefaultVariationRatioResolver()

	t.Run("バリエーションが古い", func(t *testing.T) {
		h := training.NewHistory(append(
			weekly(t, "m", "bench", 1, 8, flat(100)),
			weekly(t, "v", "larsen", 2, 3, flat(80))...))
		// バリエーションの最終は 8/16。そこから43日後。
		got := r.Resolve(h, larsen(t, 0.9), "bench", training.MustDate(2026, time.September, 28))
		if math.Abs(got.Float()-0.9) > 1e-9 {
			t.Errorf("古いバリエーション履歴から実測値が採用された: %v", got.Float())
		}
	})

	t.Run("メインが古い", func(t *testing.T) {
		// メインは起点の1回だけ。バリエーションは50日目以降に3回。
		// 各バリエーションのセッション時点で、メインは既に43日以上前になるので
		// 組を作れない。
		h := training.NewHistory(append(
			weekly(t, "m", "bench", 1, 1, flat(100)),
			weekly(t, "v", "larsen", 50, 3, flat(80))...))

		got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(65))
		if math.Abs(got.Float()-0.9) > 1e-9 {
			t.Errorf("古いメイン履歴から実測値が採用された: %v", got.Float())
		}
	})
}

func TestVariationRatioResolver_NonVariationReturnsOne(t *testing.T) {
	r := training.DefaultVariationRatioResolver()

	bench := mustExercise(t, benchParams())
	if got := r.Resolve(training.NewHistory(nil), bench, "bench", asOf(20)); math.Abs(got.Float()-1.0) > 1e-9 {
		t.Errorf("メイン種目の係数が1でない: %v", got.Float())
	}

	// 補助種目に実測できるだけの履歴があっても、換算してはいけない。
	p := benchParams()
	p.ID, p.Kind, p.MainLift = "pec_fly", training.KindAccessory, ""
	accessory := mustExercise(t, p)
	withHistory := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 4, flat(100)),
		weekly(t, "a", "pec_fly", 2, 4, flat(60))...))
	if got := r.Resolve(withHistory, accessory, "bench", asOf(25)); math.Abs(got.Float()-1.0) > 1e-9 {
		t.Errorf("補助種目が換算された: %v", got.Float())
	}

	if got := r.Resolve(training.NewHistory(nil), nil, "bench", asOf(20)); math.Abs(got.Float()-1.0) > 1e-9 {
		t.Errorf("nil の係数が1でない: %v", got.Float())
	}
}

func TestVariationRatioResolver_ReturnsValidRatio(t *testing.T) {
	r := training.DefaultVariationRatioResolver()

	cases := []struct {
		name string
		h    training.History
	}{
		{"履歴なし", training.NewHistory(nil)},
		{"実測あり", training.NewHistory(append(
			weekly(t, "m", "bench", 1, 4, flat(100)), weekly(t, "v", "larsen", 2, 4, flat(80))...))},
		{"記録ミス", training.NewHistory(append(
			weekly(t, "m", "bench", 1, 4, flat(50)), weekly(t, "v", "larsen", 2, 4, flat(200))...))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Resolve(c.h, larsen(t, 0.9), "bench", asOf(25))
			if _, err := training.NewRatio(got.Float()); err != nil {
				t.Errorf("無効な係数が返っている: %v (%v)", got.Float(), err)
			}
		})
	}
}

func TestNewVariationRatioResolver_RejectsBadParams(t *testing.T) {
	est := training.DefaultOneRepMaxEstimator()

	for _, n := range []int{0, -1} {
		if _, err := training.NewVariationRatioResolver(est, n); err == nil {
			t.Errorf("minSessions %d が通ってしまう", n)
		}
	}
	var zeroEst training.OneRepMaxEstimator
	if _, err := training.NewVariationRatioResolver(zeroEst, 3); err == nil {
		t.Error("ゼロ値の推定器が通ってしまう")
	}
}

// ゼロ値のリゾルバは実測に踏み込まず、初期値をそのまま返すこと。
func TestVariationRatioResolver_ZeroValueFallsBack(t *testing.T) {
	var r training.VariationRatioResolver
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 4, flat(100)),
		weekly(t, "v", "larsen", 2, 4, flat(80))...))

	if got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(25)); math.Abs(got.Float()-0.9) > 1e-9 {
		t.Errorf("ゼロ値のリゾルバが実測値を返した: %v", got.Float())
	}
}

func TestVariationRatioResolver_RejectsZeroAsOf(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 4, flat(100)),
		weekly(t, "v", "larsen", 2, 4, flat(80))...))

	if got := r.Resolve(h, larsen(t, 0.9), "bench", training.Date{}); math.Abs(got.Float()-0.9) > 1e-9 {
		t.Errorf("基準日が無いのに実測値が返る: %v", got.Float())
	}
}

func TestVariationRatioResolver_IsDeterministic(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		weekly(t, "m", "bench", 1, 4, flat(100)),
		weekly(t, "v", "larsen", 2, 4, flat(80))...))

	first := r.Resolve(h, larsen(t, 0.9), "bench", asOf(25))
	for range 30 {
		if got := r.Resolve(h, larsen(t, 0.9), "bench", asOf(25)); got != first {
			t.Fatalf("呼び出しごとに結果が変わる: %v vs %v", first.Float(), got.Float())
		}
	}
}
