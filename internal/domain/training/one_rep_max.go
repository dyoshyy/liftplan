package training

import "fmt"

// epleyDivisor は Epley 式の定数。
const epleyDivisor = 30.0

// maxRepsToFailure は Epley 式を適用してよい「限界までの総レップ数」の上限。
//
// Epley 式は線形近似で、10レップ前後までは実用的な精度を持つが、それを超えると
// 1RMを大きく過大評価する。20kg×100レップは 86.7kg と推定されるが、
// この人が実際に 86.7kg を1回挙げられる保証はどこにもない。
//
// 無制限にすると、高レップの補助種目や入力ミス（reps に 100 と打つ）が1本
// 混ざるだけで、実際に扱った重量の3倍以上が処方される。中央値は
// 「1セットだけの外れ値」には効くが、高レップ種目は全セットが高レップなので守れない。
//
// 20 に置いたのは、通常のトレーニング（12レップ + RIR2 程度）を通しつつ、
// 明らかに範囲外の記録を弾くため。
const maxRepsToFailure = 20

// maxOneRepMaxKg は推定1RMの上限。
//
// 実重量の上限（maxWeightKg = 1000）に、Epley 式が maxRepsToFailure で取りうる
// 最大の倍率（1 + 20/30 ≒ 1.67）を掛けた値に余裕を持たせてある。
const maxOneRepMaxKg = 2000

// OneRepMax は推定1RM。不変。
type OneRepMax struct {
	kg float64
}

func NewOneRepMax(kg float64) (OneRepMax, error) {
	q := Quantize(kg)
	if err := ValidateRange("推定1RM", q, SmallestPositive, maxOneRepMaxKg); err != nil {
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
// 推定できない場合は false を返す。次の3つの場合がある。
//   - 重量0の自重種目（NewOneRepMax の下限が弾く）
//   - 限界までの総レップが maxRepsToFailure を超え、Epley 式の適用範囲外
//   - 結果が推定1RMの上限を超える
func EstimateOneRepMax(w Weight, r Reps, rir RIR) (OneRepMax, bool) {
	repsToFailure := r.Int() + rir.Int()
	if repsToFailure > maxRepsToFailure {
		return OneRepMax{}, false
	}

	orm, err := NewOneRepMax(w.Kg() * (1 + float64(repsToFailure)/epleyDivisor))
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
// 丸めた結果が0kgになる場合はエラーを返す。粗い増加単位と軽い種目の
// 組み合わせで起こりうるが、0kg のセットを処方するのは
// 「推定できないなら重量を出さない」という設計を 0kg という捏造で貫通する。
func (o OneRepMax) WorkWeight(i IntensityPct, inc Increment) (Weight, error) {
	raw, err := NewWeight(o.kg * i.Float())
	if err != nil {
		return Weight{}, fmt.Errorf("実施重量を算出できない: %w", err)
	}

	rounded, err := raw.RoundTo(inc)
	if err != nil {
		return Weight{}, err
	}
	if rounded.Kg() <= 0 {
		return Weight{}, fmt.Errorf(
			"実施重量が増加単位（%vkg）の半分に満たず丸められない: %vkg", inc.Kg(), raw.Kg())
	}
	return rounded, nil
}
