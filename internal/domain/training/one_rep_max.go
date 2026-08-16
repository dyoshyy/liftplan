package training

import "fmt"

// epleyDivisor は Epley 式の定数。
const epleyDivisor = 30.0

// maxOneRepMaxKg は推定1RMの上限。
//
// 実重量の上限（maxWeightKg）に、Epley 式が取りうる最大の倍率を掛けた値に
// 量子化ぶんの余裕を持たせてある。レップ数と RIR に上限があるため、
// 有効な入力からこの値を超える推定1RMが生まれることはない。
//
// 実重量の上限より遥かに大きいのは意図的で、高レップの記録から算出した
// 推定1RMは、実際に挙げられる重量よりずっと大きな値になるため。
const maxOneRepMaxKg = 40000

// OneRepMax は推定1RM。不変。
type OneRepMax struct {
	kg float64
}

func NewOneRepMax(kg float64) (OneRepMax, error) {
	q := quantize(kg)
	if err := validateRange("推定1RM", q, smallestPositive, maxOneRepMaxKg); err != nil {
		return OneRepMax{}, err
	}
	return OneRepMax{kg: q}, nil
}

// EstimateOneRepMax は Epley 式で1セットから1RMを推定する。
//
// RIR を「限界までの残りレップ数」として実績レップに足すため、
// 追い込みきっていないセットからも強度が測れる。
// 全セットに RIR を入力する設計は、毎セットを1RM測定に変えるためにある。
//
// 推定できない場合は false を返す。具体的には重量0の自重種目のときで、
// NewOneRepMax が下限（正の数）で弾くため、ここで個別に判定する必要はない。
func EstimateOneRepMax(w Weight, r Reps, rir RIR) (OneRepMax, bool) {
	repsToFailure := float64(r.Int() + rir.Int())
	orm, err := NewOneRepMax(w.Kg() * (1 + repsToFailure/epleyDivisor))
	if err != nil {
		return OneRepMax{}, false
	}
	return orm, true
}

func (o OneRepMax) Kg() float64 { return o.kg }

// IsZero はゼロ値（未設定）かどうか。
func (o OneRepMax) IsZero() bool { return o == OneRepMax{} }

// WorkWeight は実際に使う重量。
//
// 推定1RM × 強度帯 × 対メイン係数 を増加単位へ丸める。
// 1RMが上がれば全スロットの重量が自動的に追随する。
//
// 増加単位が未設定のときはエラーを返す。丸めずに返すと、バーに載らない
// 半端な重量がそのまま処方される。
func (o OneRepMax) WorkWeight(i IntensityPct, ratio Ratio, inc Increment) (Weight, error) {
	raw, err := NewWeight(o.kg * i.Float() * ratio.Float())
	if err != nil {
		return Weight{}, fmt.Errorf("実施重量を算出できない: %w", err)
	}
	return raw.RoundTo(inc)
}

// RatioTo は自身を基準としたときの other の比率。
// バリエーションの対メイン係数の実測に使う。
//
// 比率が現実的な範囲を外れる場合は false を返す。記録ミスで係数が壊れると
// 以後の全セッションの重量が狂うため、ここで弾いて初期値へ戻させる。
// 自身がゼロ値なら商が無限大になり、NewRatio が弾く。
func (o OneRepMax) RatioTo(other OneRepMax) (Ratio, bool) {
	r, err := NewRatio(other.kg / o.kg)
	if err != nil {
		return Ratio{}, false
	}
	return r, true
}
