package training_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func estimate(t *testing.T, kg float64, reps, rir int) training.OneRepMax {
	t.Helper()

	w, err := training.NewWeight(kg)
	if err != nil {
		t.Fatalf("NewWeight(%v): %v", kg, err)
	}
	r, err := training.NewReps(reps)
	if err != nil {
		t.Fatalf("NewReps(%d): %v", reps, err)
	}
	ri, err := training.NewRIR(rir)
	if err != nil {
		t.Fatalf("NewRIR(%d): %v", rir, err)
	}

	orm, ok := training.EstimateOneRepMax(w, r, ri)
	if !ok {
		t.Fatalf("推定できない: %vkg × %d @RIR%d", kg, reps, rir)
	}
	return orm
}

func mustOneRepMax(t *testing.T, kg float64) training.OneRepMax {
	t.Helper()
	o, err := training.NewOneRepMax(kg)
	if err != nil {
		t.Fatalf("NewOneRepMax(%v): %v", kg, err)
	}
	return o
}

func TestEstimateOneRepMax_SingleAtFailure(t *testing.T) {
	// 1レップを限界まで挙げた場合。Epley 式は 1RM ちょうどにはならず、
	// 1レップぶんの係数が乗る。この挙動を固定しておく。
	got := estimate(t, 100, 1, 0).Kg()
	want := 100.0 * (1 + 1.0/30.0)
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEstimateOneRepMax_RIRCountsTowardFailure(t *testing.T) {
	// 85kg × 9レップ @RIR2 は、限界まで11レップという意味。
	// RIR を足さないと強度を過小評価する。
	got := estimate(t, 85, 9, 2).Kg()
	want := 85.0 * (1 + 11.0/30.0)
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEstimateOneRepMax_SameTotalRepsAreEquivalent(t *testing.T) {
	// RIR の内訳が違っても、限界までの総レップが同じなら同じ強度。
	a := estimate(t, 80, 8, 3)
	b := estimate(t, 80, 11, 0)
	if a != b {
		t.Errorf("同じ総レップで値が違う: %v vs %v", a.Kg(), b.Kg())
	}
}

func TestEstimateOneRepMax_IsMonotonic(t *testing.T) {
	// 同じレップ・RIR なら重量が増えるほど1RMも増える。
	prev := 0.0
	for kg := 50.0; kg <= 200; kg += 2.5 {
		got := estimate(t, kg, 8, 2).Kg()
		if got <= prev {
			t.Fatalf("重量 %v で1RMが増えていない: %v → %v", kg, prev, got)
		}
		prev = got
	}

	// 同じ重量ならレップが増えるほど1RMも増える。
	prev = 0
	for reps := 1; reps <= 20; reps++ {
		got := estimate(t, 85, reps, 2).Kg()
		if got <= prev {
			t.Fatalf("%dレップで1RMが増えていない: %v", reps, got)
		}
		prev = got
	}
}

func TestEstimateOneRepMax_RejectsBodyweight(t *testing.T) {
	// 0kg（自重種目）からは1RMを推定できない。0を返すと重量0のセットが処方される。
	w, err := training.NewWeight(0)
	if err != nil {
		t.Fatalf("NewWeight(0): %v", err)
	}
	r, _ := training.NewReps(10)
	ri, _ := training.NewRIR(2)

	if got, ok := training.EstimateOneRepMax(w, r, ri); ok {
		t.Errorf("自重種目から1RMが推定できてしまう: %v", got.Kg())
	}
}

func TestEstimateOneRepMax_NeverOverflows(t *testing.T) {
	// 値オブジェクトの上限内であれば、どんな組み合わせでも有効な1RMになる。
	// レップと RIR に上限が無いと int が溢れて負の1RMが生まれる。
	weights := []float64{0.5, 1, 85, 500, 1000}
	repsList := []int{1, 10, 100, 1000}
	rirList := []int{0, 5, 100}

	for _, kg := range weights {
		for _, reps := range repsList {
			for _, rir := range rirList {
				w, _ := training.NewWeight(kg)
				r, _ := training.NewReps(reps)
				ri, _ := training.NewRIR(rir)

				got, ok := training.EstimateOneRepMax(w, r, ri)
				if !ok {
					t.Errorf("%vkg × %d @RIR%d で推定できない", kg, reps, rir)
					continue
				}
				if got.Kg() <= 0 || math.IsInf(got.Kg(), 0) || math.IsNaN(got.Kg()) {
					t.Errorf("%vkg × %d @RIR%d が不正な1RM: %v", kg, reps, rir, got.Kg())
				}
				if _, err := training.NewOneRepMax(got.Kg()); err != nil {
					t.Errorf("%vkg × %d @RIR%d の結果が無効: %v", kg, reps, rir, err)
				}
			}
		}
	}
}

func TestNewOneRepMax_RejectsInvalid(t *testing.T) {
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1), 1e300, 1e-9} {
		if got, err := training.NewOneRepMax(v); err == nil {
			t.Errorf("不正な1RMが通ってしまう: %v → %v", v, got.Kg())
		}
	}
	if _, err := training.NewOneRepMax(105); err != nil {
		t.Errorf("正当な1RMが弾かれた: %v", err)
	}
}

func TestOneRepMax_FailureReturnsZeroValue(t *testing.T) {
	got, err := training.NewOneRepMax(-1)
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !got.IsZero() {
		t.Errorf("失敗時にゼロ値でない値が返る: %v", got.Kg())
	}
}

func TestOneRepMax_WorkWeight(t *testing.T) {
	orm := mustOneRepMax(t, 105)
	intensity := mustIntensity(t, 0.81)
	ratio := mustRatio(t, 1.0)
	inc := mustIncrement(t, 2.5)

	// 105 × 0.81 = 85.05 → 2.5kg刻みで 85.0
	got, err := orm.WorkWeight(intensity, ratio, inc)
	if err != nil {
		t.Fatalf("WorkWeight: %v", err)
	}
	if math.Abs(got.Kg()-85.0) > 1e-9 {
		t.Errorf("got %v, want 85", got.Kg())
	}
}

func TestOneRepMax_WorkWeightAppliesRatio(t *testing.T) {
	orm := mustOneRepMax(t, 105)

	// 105 × 0.76 × 0.85 = 67.83 → 67.5
	got, err := orm.WorkWeight(mustIntensity(t, 0.76), mustRatio(t, 0.85), mustIncrement(t, 2.5))
	if err != nil {
		t.Fatalf("WorkWeight: %v", err)
	}
	if math.Abs(got.Kg()-67.5) > 1e-9 {
		t.Errorf("got %v, want 67.5", got.Kg())
	}
}

func TestOneRepMax_WorkWeightFollowsOneRepMax(t *testing.T) {
	// 1RMが上がれば実施重量も追随すること。これがスロット自動調整の土台。
	intensity := mustIntensity(t, 0.81)
	ratio := mustRatio(t, 1.0)
	inc := mustIncrement(t, 2.5)

	prev := 0.0
	for kg := 100.0; kg <= 200; kg += 10 {
		got, err := mustOneRepMax(t, kg).WorkWeight(intensity, ratio, inc)
		if err != nil {
			t.Fatalf("WorkWeight: %v", err)
		}
		if got.Kg() <= prev {
			t.Fatalf("1RM %v で実施重量が増えていない: %v → %v", kg, prev, got.Kg())
		}
		prev = got.Kg()
	}
}

func TestOneRepMax_WorkWeightRejectsZeroIncrement(t *testing.T) {
	var zero training.Increment
	got, err := mustOneRepMax(t, 105).WorkWeight(mustIntensity(t, 0.81), mustRatio(t, 1.0), zero)
	if err == nil {
		t.Fatalf("増加単位が未設定なのに通ってしまう: %v", got.Kg())
	}
}

func TestOneRepMax_WorkWeightIsAlwaysOnTheIncrementGrid(t *testing.T) {
	// バーに載らない重量が処方されないこと。
	for _, incKg := range []float64{0.5, 1.0, 2.5, 5.0} {
		inc := mustIncrement(t, incKg)
		for ormKg := 60.0; ormKg <= 300; ormKg += 7.3 {
			for _, iv := range []float64{0.76, 0.81, 0.88, 0.71} {
				got, err := mustOneRepMax(t, ormKg).WorkWeight(mustIntensity(t, iv), mustRatio(t, 0.85), inc)
				if err != nil {
					t.Fatalf("WorkWeight: %v", err)
				}
				ratio := got.Kg() / incKg
				if math.Abs(ratio-math.Round(ratio)) > 1e-9 {
					t.Fatalf("刻み %v に乗っていない: %v", incKg, got.Kg())
				}
				if n := decimalPlaces(strconv.FormatFloat(got.Kg(), 'f', -1, 64)); n > 6 {
					t.Fatalf("端数が残っている: %v", got.Kg())
				}
			}
		}
	}
}

func TestOneRepMax_WorkWeightRejectsOverflow(t *testing.T) {
	// 1RMの上限は実重量の上限より遥かに大きいので、高強度では重量の上限を超えうる。
	// そのとき黙って飽和させず、エラーにする。
	huge := mustOneRepMax(t, 30000)
	if got, err := huge.WorkWeight(mustIntensity(t, 1.0), mustRatio(t, 1.0), mustIncrement(t, 2.5)); err == nil {
		t.Errorf("重量の上限を超えたのに通ってしまう: %v", got.Kg())
	}
}

func TestOneRepMax_RatioTo(t *testing.T) {
	main := mustOneRepMax(t, 100)
	variation := mustOneRepMax(t, 85)

	got, ok := main.RatioTo(variation)
	if !ok {
		t.Fatal("比率が取れない")
	}
	if math.Abs(got.Float()-0.85) > 1e-9 {
		t.Errorf("got %v, want 0.85", got.Float())
	}
}

func TestOneRepMax_RatioToRejectsUnrealistic(t *testing.T) {
	// 記録ミスで係数が壊れると、以後の全セッションの重量が狂う。
	main := mustOneRepMax(t, 50)
	absurd := mustOneRepMax(t, 200)

	if got, ok := main.RatioTo(absurd); ok {
		t.Errorf("非現実的な比率が通ってしまう: %v", got.Float())
	}
}

func TestOneRepMax_RatioToWithZeroValue(t *testing.T) {
	var zero training.OneRepMax
	if _, ok := zero.RatioTo(mustOneRepMax(t, 100)); ok {
		t.Error("ゼロ値を基準に比率が取れてしまう")
	}
}

func TestOneRepMax_IsComparable(t *testing.T) {
	// 同じ入力から同じ値になること。ヒステリシス判定で == を使う。
	if estimate(t, 85, 9, 2) != estimate(t, 85, 9, 2) {
		t.Error("同じ入力から違う値が生まれる")
	}
}
