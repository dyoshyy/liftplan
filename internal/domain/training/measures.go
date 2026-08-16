package training

import (
	"fmt"
	"math"
)

// maxRatio は対メイン係数の現実的な上限。
// バリエーションが通常フォームより大きく挙がることは無いが、フォームの違いで
// わずかに上回る種目はあるため 1.0 ちょうどでは切らない。
const maxRatio = 1.2

// quantum は浮動小数点演算の残差を落とすための桁数。
// 0.1 刻みのような増加単位で math.Round を使うと 82.50000000000001 のような値が出る。
const quantum = 1e6

// quantize は浮動小数点の計算残差を落とす。
func quantize(v float64) float64 { return math.Round(v*quantum) / quantum }

func rejectNonFinite(name string, v float64) error {
	if math.IsNaN(v) {
		return fmt.Errorf("%sが数値ではない", name)
	}
	if math.IsInf(v, 0) {
		return fmt.Errorf("%sが無限大である", name)
	}
	return nil
}

// Increment はジムのプレート構成に対応する重量の刻み。
type Increment struct {
	kg float64
}

func NewIncrement(kg float64) (Increment, error) {
	if err := rejectNonFinite("増加単位", kg); err != nil {
		return Increment{}, err
	}
	if kg <= 0 {
		return Increment{}, fmt.Errorf("増加単位は正の数である必要がある: %v", kg)
	}
	return Increment{kg: quantize(kg)}, nil
}

func (i Increment) Kg() float64 { return i.kg }

// IsZero はゼロ値（未設定）かどうか。
func (i Increment) IsZero() bool { return i == Increment{} }

// Weight は重量（kg）。不変。
type Weight struct {
	kg float64
}

func NewWeight(kg float64) (Weight, error) {
	if err := rejectNonFinite("重量", kg); err != nil {
		return Weight{}, err
	}
	if kg < 0 {
		return Weight{}, fmt.Errorf("重量は0以上である必要がある: %v", kg)
	}
	return Weight{kg: quantize(kg)}, nil
}

func (w Weight) Kg() float64 { return w.kg }

// RoundTo は増加単位へ丸める。
//
// 丸めた結果は必ず0以上の有効な重量になるためエラーを返さない。
// ゼロ値の Increment を渡された場合は丸めずそのまま返す（0除算を避ける）。
func (w Weight) RoundTo(inc Increment) Weight {
	if inc.IsZero() {
		return w
	}
	rounded := quantize(math.Round(w.kg/inc.kg) * inc.kg)
	if rounded < 0 {
		rounded = 0
	}
	return Weight{kg: rounded}
}

// Reps は実際に挙げた回数。1以上。
type Reps struct {
	v int
}

func NewReps(v int) (Reps, error) {
	if v < 1 {
		return Reps{}, fmt.Errorf("レップ数は1以上である必要がある: %d", v)
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
	if v < 0 {
		return RIR{}, fmt.Errorf("RIRは0以上である必要がある: %d", v)
	}
	return RIR{v: v}, nil
}

func (r RIR) Int() int { return r.v }

// Plus は補正を加える。睡眠不足の日に目標RIRを上げるために使う。
// 下限0で丸めるため、常に有効な値を返す。
func (r RIR) Plus(n int) RIR {
	v := r.v + n
	if v < 0 {
		v = 0
	}
	return RIR{v: v}
}

// IntensityPct は推定1RMに対する割合。
type IntensityPct struct {
	v float64
}

func NewIntensityPct(v float64) (IntensityPct, error) {
	if err := rejectNonFinite("強度", v); err != nil {
		return IntensityPct{}, err
	}
	if v <= 0 || v > 1 {
		return IntensityPct{}, fmt.Errorf("強度は0より大きく1以下である必要がある: %v", v)
	}
	return IntensityPct{v: quantize(v)}, nil
}

func (i IntensityPct) Float() float64 { return i.v }

// Scale は係数を掛ける。デロードで強度を下げるために使う。
//
// 引数を検証済みの Ratio に限ることで、結果が0以下になる経路を型で塞いでいる。
// 上限は 1.0 に丸める（1RM を超える強度は意味を持たないため）。
func (i IntensityPct) Scale(r Ratio) IntensityPct {
	v := quantize(i.v * r.v)
	if v > 1 {
		v = 1
	}
	return IntensityPct{v: v}
}

// Ratio は比率。バリエーションの対メイン係数と、デロードの強度低下に使う。
type Ratio struct {
	v float64
}

func NewRatio(v float64) (Ratio, error) {
	if err := rejectNonFinite("比率", v); err != nil {
		return Ratio{}, err
	}
	if v <= 0 || v > maxRatio {
		return Ratio{}, fmt.Errorf("比率は0より大きく%v以下である必要がある: %v", maxRatio, v)
	}
	return Ratio{v: quantize(v)}, nil
}

func (r Ratio) Float() float64 { return r.v }

// SetCount はセット数。
type SetCount struct {
	v int
}

func NewSetCount(v int) (SetCount, error) {
	if v < 1 {
		return SetCount{}, fmt.Errorf("セット数は1以上である必要がある: %d", v)
	}
	return SetCount{v: v}, nil
}

func (s SetCount) Int() int { return s.v }

// Contribution は「1セット実施したとき、その筋区分に何セット分の刺激が入るか」。
type Contribution struct {
	v float64
}

func NewContribution(v float64) (Contribution, error) {
	if err := rejectNonFinite("寄与度", v); err != nil {
		return Contribution{}, err
	}
	if v <= 0 || v > 1 {
		return Contribution{}, fmt.Errorf("寄与度は0より大きく1以下である必要がある: %v", v)
	}
	return Contribution{v: quantize(v)}, nil
}

func (c Contribution) Float() float64 { return c.v }
