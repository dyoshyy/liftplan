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
	for reps := 1; reps <= 18; reps++ {
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
	repsList := []int{1, 5, 10, 18}
	rirList := []int{0, 1, 2}

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
	// 1RMの上限は実重量の上限より大きいので、高強度では重量の上限を超えうる。
	// そのとき黙って飽和させず、エラーにする。
	huge := mustOneRepMax(t, 1900)
	if got, err := huge.WorkWeight(mustIntensity(t, 1.0), mustRatio(t, 1.0), mustIncrement(t, 2.5)); err == nil {
		t.Errorf("重量の上限を超えたのに通ってしまう: %v", got.Kg())
	}
}

func TestOneRepMax_IsComparable(t *testing.T) {
	// 同じ入力から同じ値になること。ヒステリシス判定で == を使う。
	if estimate(t, 85, 9, 2) != estimate(t, 85, 9, 2) {
		t.Error("同じ入力から違う値が生まれる")
	}
}

// 推定1RMは量子化されていること。
//
// 量子化しないと 118.08333350000001 のような値が JSON にそのまま出る。
// また同じ意味の値が別のビット列になり、ヒステリシス判定の == が成立しなくなる。
func TestOneRepMax_IsQuantized(t *testing.T) {
	for _, c := range []struct {
		kg   float64
		reps int
		rir  int
	}{
		{85, 9, 2}, {102.5, 7, 1}, {62.5, 11, 3}, {137.5, 5, 2}, {47.5, 13, 2},
	} {
		got := estimate(t, c.kg, c.reps, c.rir).Kg()
		if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
			t.Errorf("%vkg × %d @RIR%d の推定1RMに端数が残っている: %s（小数点以下 %d 桁）",
				c.kg, c.reps, c.rir, strconv.FormatFloat(got, 'f', -1, 64), n)
		}
	}

	if _, err := training.NewOneRepMax(118.08333350000001); err != nil {
		t.Fatalf("NewOneRepMax: %v", err)
	}
	a, _ := training.NewOneRepMax(118.08333350000001)
	b, _ := training.NewOneRepMax(118.0833335)
	if a != b {
		t.Errorf("同じ意味の1RMが等しくない: %v vs %v", a.Kg(), b.Kg())
	}
}

// RIR の寄与を値で固定する。この PR の主張の中核。
//
// RIR を足さない、あるいは途中で飽和させる実装に変えても、
// 「正で有限か」しか見ないテストでは検出できない。
func TestEstimateOneRepMax_GoldenValues(t *testing.T) {
	cases := []struct {
		kg   float64
		reps int
		rir  int
		want float64
	}{
		{100, 1, 0, 100 * (1 + 1.0/30)},
		{85, 9, 2, 85 * (1 + 11.0/30)},
		{85, 5, 6, 85 * (1 + 11.0/30)},
		{85, 5, 10, 85 * (1 + 15.0/30)},
		{85, 2, 18, 85 * (1 + 20.0/30)},
		{60, 12, 8, 60 * (1 + 20.0/30)},
		{140, 3, 1, 140 * (1 + 4.0/30)},
	}
	for _, c := range cases {
		got := estimate(t, c.kg, c.reps, c.rir).Kg()
		if math.Abs(got-c.want) > 1e-5 {
			t.Errorf("%vkg × %d @RIR%d: got %v, want %v", c.kg, c.reps, c.rir, got, c.want)
		}
	}
}

// Epley 式の適用範囲外は推定しない。
//
// 高レップの記録から算出した1RMは実際に挙げられる重量を大きく上回る。
// 20kg×100レップから 86.7kg を推定し、その 0.81 倍を処方すると、
// 実際に扱った重量の3.5倍になる。
func TestEstimateOneRepMax_RejectsOutOfRangeReps(t *testing.T) {
	cases := []struct {
		kg   float64
		reps int
		rir  int
	}{
		{20, 100, 0},
		{40, 50, 0},
		{60, 30, 3},
		{60, 20, 2},
		{85, 19, 2},
	}
	for _, c := range cases {
		w, _ := training.NewWeight(c.kg)
		r, _ := training.NewReps(c.reps)
		ri, _ := training.NewRIR(c.rir)
		if got, ok := training.EstimateOneRepMax(w, r, ri); ok {
			t.Errorf("適用範囲外の %vkg × %d @RIR%d から推定できてしまう: %v",
				c.kg, c.reps, c.rir, got.Kg())
		}
	}

	// 境界のすぐ内側は通ること。
	w, _ := training.NewWeight(85)
	r, _ := training.NewReps(18)
	ri, _ := training.NewRIR(2)
	if _, ok := training.EstimateOneRepMax(w, r, ri); !ok {
		t.Error("適用範囲内なのに推定できない")
	}
}

// 丸めた結果が0kgになるなら、0kg を処方せずエラーにする。
//
// 粗い増加単位（プレートローディング式マシンの20kg刻みなど）と軽い補助種目の
// 組み合わせで到達する。0kg のセットを処方するのは、
// 「推定できないなら重量を出さない」という設計を 0kg という捏造で貫通すること。
func TestOneRepMax_WorkWeightRejectsRoundingToZero(t *testing.T) {
	cases := []struct {
		orm       float64
		intensity float64
		increment float64
	}{
		{1.2, 0.81, 2.5},
		{10, 0.81, 20},
		{30, 0.81, 50},
	}
	for _, c := range cases {
		got, err := mustOneRepMax(t, c.orm).WorkWeight(
			mustIntensity(t, c.intensity), mustRatio(t, 1.0), mustIncrement(t, c.increment))
		if err == nil {
			t.Errorf("1RM %v・強度 %v・刻み %v で 0kg が処方された: %v",
				c.orm, c.intensity, c.increment, got.Kg())
		}
	}
}

// 処方された重量は常に正であること。
func TestOneRepMax_WorkWeightIsAlwaysPositive(t *testing.T) {
	for _, incKg := range []float64{0.5, 1, 2.5, 5, 20, 50} {
		inc := mustIncrement(t, incKg)
		for ormKg := 1.0; ormKg <= 300; ormKg += 3.7 {
			got, err := mustOneRepMax(t, ormKg).WorkWeight(
				mustIntensity(t, 0.71), mustRatio(t, 0.85), inc)
			if err != nil {
				continue // 丸めて0になる組み合わせはエラーになるのが正しい
			}
			if got.Kg() <= 0 {
				t.Fatalf("1RM %v・刻み %v で 0kg 以下が処方された: %v", ormKg, incKg, got.Kg())
			}
		}
	}
}

// 上限そのものを固定する。「1900 は通る／2100 は落ちる」だけでは、
// 上限を 1e9 に緩める改変を検出できない。
// DB から復元した壊れた値が値オブジェクトを素通りするのを防ぐ。
func TestOneRepMax_BoundaryConstant(t *testing.T) {
	const max = 2000

	if _, err := training.NewOneRepMax(max); err != nil {
		t.Errorf("上限ちょうど %v が弾かれた: %v", float64(max), err)
	}
	if got, err := training.NewOneRepMax(max + 0.000001); err == nil {
		t.Errorf("上限をわずかに超える値が通ってしまう: %v", got.Kg())
	}

	// 有効な入力から生まれうる最大の推定1RMが、上限に収まっていること。
	w, err := training.NewWeight(1000)
	if err != nil {
		t.Fatalf("NewWeight: %v", err)
	}
	r, err := training.NewReps(18)
	if err != nil {
		t.Fatalf("NewReps: %v", err)
	}
	ri, err := training.NewRIR(2)
	if err != nil {
		t.Fatalf("NewRIR: %v", err)
	}
	got, ok := training.EstimateOneRepMax(w, r, ri)
	if !ok {
		t.Fatal("有効な入力の上限で推定できない")
	}
	if got.Kg() > max {
		t.Errorf("有効な入力から上限を超える1RMが生まれる: %v", got.Kg())
	}
}
