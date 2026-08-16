package training_test

import (
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

// threeSessions は指定種目を3セッション分（day, day+7, day+14）記録する。
func threeSessions(t *testing.T, prefix, exercise string, startDay int, kg float64) []*training.SetLog {
	t.Helper()
	out := make([]*training.SetLog, 0, 3)
	for i := range 3 {
		out = append(out, mkLog(t, prefix+string(rune('a'+i)), startDay+i*7, exercise, kg, 8, 2))
	}
	return out
}

func TestVariationRatioResolver_FallsBackWhenTooFewSessions(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "m1", 1, "bench", 100, 8, 2),
		mkLog(t, "v1", 2, "larsen", 80, 8, 2),
	})

	if got := r.Resolve(h, larsen(t, 0.85), "bench", asOf(5)); math.Abs(got.Float()-0.85) > 1e-9 {
		t.Errorf("初期値にフォールバックしていない: %v", got.Float())
	}
}

// 境界。minSessions ちょうどで実測に切り替わること。
func TestVariationRatioResolver_SwitchesAtMinSessions(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	if r.MinSessions() != 3 {
		t.Fatalf("既定の minSessions が誤り: %d", r.MinSessions())
	}

	main := threeSessions(t, "m", "bench", 1, 100)

	// 2セッションでは初期値のまま。
	two := training.NewHistory(append(append([]*training.SetLog{}, main...),
		mkLog(t, "va", 2, "larsen", 80, 8, 2),
		mkLog(t, "vb", 9, "larsen", 80, 8, 2)))
	if got := r.Resolve(two, larsen(t, 0.85), "bench", asOf(20)); math.Abs(got.Float()-0.85) > 1e-9 {
		t.Errorf("2セッションで実測に切り替わっている: %v", got.Float())
	}

	// 3セッションで実測に切り替わる。
	three := training.NewHistory(append(append([]*training.SetLog{}, main...),
		threeSessions(t, "v", "larsen", 2, 80)...))
	if got := r.Resolve(three, larsen(t, 0.85), "bench", asOf(20)); math.Abs(got.Float()-0.80) > 1e-6 {
		t.Errorf("3セッションで実測に切り替わっていない: %v", got.Float())
	}
}

func TestVariationRatioResolver_UsesMeasuredRatio(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	logs := append(threeSessions(t, "m", "bench", 1, 100), threeSessions(t, "v", "larsen", 2, 80)...)

	// 同じレップ・RIR なので推定1RMの比は重量の比に一致する。
	got := r.Resolve(training.NewHistory(logs), larsen(t, 0.85), "bench", asOf(20))
	if math.Abs(got.Float()-0.80) > 1e-6 {
		t.Errorf("実測値になっていない: %v", got.Float())
	}
}

// 引数の順序を取り違えると係数が逆数になる。
// 実際の係数域（0.85〜0.95）では逆数も範囲内に入るため、範囲検査では防げない。
// 分子と分母がどちらか、テストで明示的に固定する。
func TestVariationRatioResolver_RatioIsVariationOverMain(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	logs := append(threeSessions(t, "m", "bench", 1, 100), threeSessions(t, "v", "larsen", 2, 85)...)

	got := r.Resolve(training.NewHistory(logs), larsen(t, 0.9), "bench", asOf(20))

	// バリエーション85 / メイン100 = 0.85。逆なら 1.176 になる。
	if math.Abs(got.Float()-0.85) > 1e-6 {
		t.Errorf("バリエーション/メイン になっていない: got %v, want 0.85", got.Float())
	}
	if got.Float() > 1.0 {
		t.Errorf("係数が1を超えている。分子分母が逆: %v", got.Float())
	}
}

func TestVariationRatioResolver_FallsBackWhenMainMissing(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(threeSessions(t, "v", "larsen", 2, 80))

	if got := r.Resolve(h, larsen(t, 0.85), "bench", asOf(20)); math.Abs(got.Float()-0.85) > 1e-9 {
		t.Errorf("メイン履歴が無いのに実測を使っている: %v", got.Float())
	}
}

// 記録ミスで係数が壊れると、以後の全セッションの重量が狂う。
func TestVariationRatioResolver_FallsBackWhenRatioUnrealistic(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	// バリエーションがメインの4倍という記録ミス。
	logs := append(threeSessions(t, "m", "bench", 1, 50), threeSessions(t, "v", "larsen", 2, 200)...)

	if got := r.Resolve(training.NewHistory(logs), larsen(t, 0.85), "bench", asOf(20)); math.Abs(got.Float()-0.85) > 1e-9 {
		t.Errorf("非現実的な実測値が採用された: %v", got.Float())
	}
}

// ブランク明けは推定器が値を返さないので、初期値へ戻ること。
func TestVariationRatioResolver_FallsBackWhenHistoryIsStale(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	logs := append(threeSessions(t, "m", "bench", 1, 100), threeSessions(t, "v", "larsen", 2, 80)...)

	got := r.Resolve(training.NewHistory(logs), larsen(t, 0.85), "bench",
		training.MustDate(2026, time.December, 1))
	if math.Abs(got.Float()-0.85) > 1e-9 {
		t.Errorf("古い履歴から実測値が採用された: %v", got.Float())
	}
}

func TestVariationRatioResolver_NonVariationReturnsOne(t *testing.T) {
	r := training.DefaultVariationRatioResolver()

	bench := mustExercise(t, benchParams())
	if got := r.Resolve(training.NewHistory(nil), bench, "bench", asOf(20)); math.Abs(got.Float()-1.0) > 1e-9 {
		t.Errorf("メイン種目の係数が1でない: %v", got.Float())
	}

	// 補助種目に実測できるだけの履歴があっても、換算してはいけない。
	// 補助種目はメインの派生ではないので、比率に意味が無い。
	p := benchParams()
	p.ID, p.Kind, p.MainLift = "pec_fly", training.KindAccessory, ""
	accessory := mustExercise(t, p)

	withHistory := training.NewHistory(append(
		threeSessions(t, "m", "bench", 1, 100),
		threeSessions(t, "a", "pec_fly", 2, 60)...))
	if got := r.Resolve(withHistory, accessory, "bench", asOf(20)); math.Abs(got.Float()-1.0) > 1e-9 {
		t.Errorf("補助種目が換算された: %v", got.Float())
	}

	// メイン種目も同様。
	withMainHistory := training.NewHistory(append(
		threeSessions(t, "m", "bench", 1, 100),
		threeSessions(t, "s", "squat", 2, 150)...))
	squat := mustExercise(t, training.ExerciseParams{
		ID: "squat", Name: "スクワット", Kind: training.KindMain,
		Stimulus:    map[training.MuscleRegion]float64{training.Quad: 1.0},
		IncrementKg: 2.5, MainLift: training.LiftSquat,
	})
	if got := r.Resolve(withMainHistory, squat, "bench", asOf(20)); math.Abs(got.Float()-1.0) > 1e-9 {
		t.Errorf("メイン種目が換算された: %v", got.Float())
	}

	if got := r.Resolve(training.NewHistory(nil), nil, "bench", asOf(20)); math.Abs(got.Float()-1.0) > 1e-9 {
		t.Errorf("nil の係数が1でない: %v", got.Float())
	}
}

// 初期値の無いバリエーションは作れないが、万一渡されても 1.0 に落ちること。
func TestVariationRatioResolver_ReturnsValidRatio(t *testing.T) {
	r := training.DefaultVariationRatioResolver()

	cases := []struct {
		name string
		h    training.History
	}{
		{"履歴なし", training.NewHistory(nil)},
		{"実測あり", training.NewHistory(append(
			threeSessions(t, "m", "bench", 1, 100), threeSessions(t, "v", "larsen", 2, 80)...))},
		{"記録ミス", training.NewHistory(append(
			threeSessions(t, "m", "bench", 1, 50), threeSessions(t, "v", "larsen", 2, 200)...))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.Resolve(c.h, larsen(t, 0.85), "bench", asOf(20))
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
	logs := append(threeSessions(t, "m", "bench", 1, 100), threeSessions(t, "v", "larsen", 2, 80)...)

	if got := r.Resolve(training.NewHistory(logs), larsen(t, 0.85), "bench", asOf(20)); math.Abs(got.Float()-0.85) > 1e-9 {
		t.Errorf("ゼロ値のリゾルバが実測値を返した: %v", got.Float())
	}
}

func TestVariationRatioResolver_IsDeterministic(t *testing.T) {
	r := training.DefaultVariationRatioResolver()
	h := training.NewHistory(append(
		threeSessions(t, "m", "bench", 1, 100), threeSessions(t, "v", "larsen", 2, 80)...))

	first := r.Resolve(h, larsen(t, 0.85), "bench", asOf(20))
	for range 50 {
		if got := r.Resolve(h, larsen(t, 0.85), "bench", asOf(20)); got != first {
			t.Fatalf("呼び出しごとに結果が変わる: %v vs %v", first.Float(), got.Float())
		}
	}
}
