package training

import (
	"fmt"
	"math"
)

// 現実的な上限。
//
// 上限を設ける目的は2つある。ひとつは入力ミスを早期に弾くこと。もうひとつは、
// int の加算や float の演算がオーバーフローする領域へ値を到達させないこと。
// 例えば Reps と RIR に上限が無いと、Epley 式の reps+rir が int を溢れて負になり、
// 推定1RMが負になる。
const (
	maxWeightKg    = 1000
	maxIncrementKg = 50
	maxReps        = 1000
	maxRIR         = 100
	maxSetCount    = 100

	// maxRatio は比率の現実的な上限。バリエーションが通常フォームを大きく上回ることは
	// 無いが、フォームの違いでわずかに上回る種目はあるため 1.0 ちょうどでは切らない。
	maxRatio            = 1.2
	maxBodyweightFactor = 1.0
)

// quantum は浮動小数点演算の残差を落とす桁数。
// math.Round(v/inc)*inc は 3.7000000000000006 のような値を生み、
// それが JSON に出るとユーザーの目に触れる。
const quantum = 1e6

// quantize は浮動小数点の計算残差を落とす。
//
// 検証の「前」に適用すること。後に適用すると、検証を通った値が量子化で
// +Inf になったり 0 に潰れたりして、コンストラクタが自分で不変条件を破る。
func quantize(v float64) float64 { return math.Round(v*quantum) / quantum }

// validateRange は量子化済みの値が [min, max] に収まるかを検査する。
func validateRange(name string, v, min, max float64) error {
	if math.IsNaN(v) {
		return fmt.Errorf("%sが数値ではない", name)
	}
	if math.IsInf(v, 0) {
		return fmt.Errorf("%sが無限大である", name)
	}
	if v < min || v > max {
		return fmt.Errorf("%sは%v〜%vの範囲である必要がある: %v", name, min, max, v)
	}
	return nil
}

// smallestPositive は量子化後に0に潰れない最小の正の値。
const smallestPositive = 1 / quantum

// Increment はジムのプレート構成に対応する重量の刻み。
type Increment struct {
	kg float64
}

func NewIncrement(kg float64) (Increment, error) {
	q := quantize(kg)
	if err := validateRange("増加単位", q, smallestPositive, maxIncrementKg); err != nil {
		return Increment{}, err
	}
	return Increment{kg: q}, nil
}

func (i Increment) Kg() float64 { return i.kg }

// IsZero はゼロ値（未設定）かどうか。
func (i Increment) IsZero() bool { return i == Increment{} }

// Weight は重量（kg）。不変。
type Weight struct {
	kg float64
}

func NewWeight(kg float64) (Weight, error) {
	q := quantize(kg)
	// 自重種目を0kgで記録する運用があるため下限は0。
	if err := validateRange("重量", q, 0, maxWeightKg); err != nil {
		return Weight{}, err
	}
	return Weight{kg: q}, nil
}

func (w Weight) Kg() float64 { return w.kg }

// IsZero はゼロ値かどうか。0kg の自重種目と区別できない点に注意。
func (w Weight) IsZero() bool { return w == Weight{} }

// RoundTo は増加単位へ丸める。
//
// ゼロ値の Increment はエラーにする。黙って丸めずに返すと、バーに載らない
// 半端な重量がそのまま処方され、しかも誰も気づけない。
func (w Weight) RoundTo(inc Increment) (Weight, error) {
	if inc.IsZero() {
		return Weight{}, fmt.Errorf("増加単位が未設定のため丸められない")
	}
	return NewWeight(math.Round(w.kg/inc.kg) * inc.kg)
}

// Reps は実際に挙げた回数。
type Reps struct {
	v int
}

func NewReps(v int) (Reps, error) {
	if v < 1 || v > maxReps {
		return Reps{}, fmt.Errorf("レップ数は1〜%dの範囲である必要がある: %d", maxReps, v)
	}
	return Reps{v: v}, nil
}

func (r Reps) Int() int { return r.v }

// RIR は限界までの残りレップ数（Reps In Reserve）。
//
// 調整ダイヤルではなく「止め時」を表すガードレールとして使う。
// レップ数は指示せず、その日の状態が決める。
type RIR struct {
	v int
}

func NewRIR(v int) (RIR, error) {
	if v < 0 || v > maxRIR {
		return RIR{}, fmt.Errorf("RIRは0〜%dの範囲である必要がある: %d", maxRIR, v)
	}
	return RIR{v: v}, nil
}

func (r RIR) Int() int { return r.v }

// Plus は補正を加える。睡眠不足の日に目標RIRを上げるために使う。
// 有効範囲に丸めるため、常に有効な値を返す。
func (r RIR) Plus(n int) RIR {
	v := r.v + n
	switch {
	case n > 0 && v < r.v: // int のオーバーフロー
		v = maxRIR
	case v < 0:
		v = 0
	case v > maxRIR:
		v = maxRIR
	}
	return RIR{v: v}
}

// IntensityPct は推定1RMに対する割合。
type IntensityPct struct {
	v float64
}

func NewIntensityPct(v float64) (IntensityPct, error) {
	q := quantize(v)
	if err := validateRange("強度", q, smallestPositive, 1); err != nil {
		return IntensityPct{}, err
	}
	return IntensityPct{v: q}, nil
}

func (i IntensityPct) Float() float64 { return i.v }

// maxReduction は一度に下げられる強度の上限。
//
// 設定ミスで極端な低下率が入っても、トレーニングとして意味のある強度を保つ。
// また、これを 0.5 に抑えることで結果が0に潰れないことが保証される。
// 強度の最小値 1/quantum に対して (1-0.5) を掛けても、量子化で切り上がるため。
const maxReduction = 0.5

// Reduce は強度を pct の割合だけ下げる。デロードで使う。
//
// エラーを返さないのは、呼び出し側に `NewRatio(1-drop)` のような
// 握り潰されがちなエラー処理を強いないため。pct は (0, maxReduction] に丸める。
func (i IntensityPct) Reduce(pct float64) IntensityPct {
	if math.IsNaN(pct) || pct <= 0 {
		return i
	}
	if pct > maxReduction {
		pct = maxReduction
	}
	return IntensityPct{v: quantize(i.v * (1 - pct))}
}

// Ratio は比率。バリエーションの対メイン係数に使う。
type Ratio struct {
	v float64
}

func NewRatio(v float64) (Ratio, error) {
	q := quantize(v)
	if err := validateRange("比率", q, smallestPositive, maxRatio); err != nil {
		return Ratio{}, err
	}
	return Ratio{v: q}, nil
}

func (r Ratio) Float() float64 { return r.v }

// SetCount はセット数。
type SetCount struct {
	v int
}

func NewSetCount(v int) (SetCount, error) {
	if v < 1 || v > maxSetCount {
		return SetCount{}, fmt.Errorf("セット数は1〜%dの範囲である必要がある: %d", maxSetCount, v)
	}
	return SetCount{v: v}, nil
}

func (s SetCount) Int() int { return s.v }

// Contribution は「1セット実施したとき、その筋区分に何セット分の刺激が入るか」。
type Contribution struct {
	v float64
}

func NewContribution(v float64) (Contribution, error) {
	q := quantize(v)
	if err := validateRange("寄与度", q, smallestPositive, 1); err != nil {
		return Contribution{}, err
	}
	return Contribution{v: q}, nil
}

func (c Contribution) Float() float64 { return c.v }

// TimesSets は指定セット数ぶんの刺激量。StimulusCoverage の積み上げに使う。
func (c Contribution) TimesSets(s SetCount) float64 {
	return quantize(c.v * float64(s.v))
}

type BodyweightFactor struct {
	v float64
}

func NewBodyweightFactor(factor float64) (BodyweightFactor, error) {
	q := quantize(factor)
	if err := validateRange("自重係数", q, 0, maxBodyweightFactor); err != nil {
		return BodyweightFactor{}, err
	}
	return BodyweightFactor{q}, nil
}

func (f BodyweightFactor) Float() float64 { return f.v }
