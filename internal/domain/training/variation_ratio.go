package training

import "fmt"

const defaultMinRatioSessions = 3

// unitRatio は「換算しない」を表す係数。
var unitRatio = Ratio{v: 1.0}

// VariationRatioResolver はバリエーション種目の対メイン係数を決めるドメインサービス。無状態。
//
// ラーセンプレスやテンポは通常フォームより挙がらない。同じ推定1RMの物差しに
// 乗せるため比率で換算する。
//
// シードが持つ初期値は「最初の一歩を踏み出すための仮の値」であり正確である
// 必要はない。minSessions 回こなせば実測値に置き換わる。
type VariationRatioResolver struct {
	estimator   OneRepMaxEstimator
	minSessions int
}

func NewVariationRatioResolver(est OneRepMaxEstimator, minSessions int) (VariationRatioResolver, error) {
	if est.IsZero() {
		return VariationRatioResolver{}, fmt.Errorf("推定器が未設定である")
	}
	if minSessions < 1 {
		return VariationRatioResolver{}, fmt.Errorf("minSessions は1以上である必要がある: %d", minSessions)
	}
	return VariationRatioResolver{estimator: est, minSessions: minSessions}, nil
}

func DefaultVariationRatioResolver() VariationRatioResolver {
	return VariationRatioResolver{
		estimator:   DefaultOneRepMaxEstimator(),
		minSessions: defaultMinRatioSessions,
	}
}

func (r VariationRatioResolver) MinSessions() int { return r.minSessions }

// IsZero はゼロ値（未設定）かどうか。
func (r VariationRatioResolver) IsZero() bool { return r == VariationRatioResolver{} }

// Resolve は asOf 時点での換算係数を返す。バリエーション以外は常に 1.0。
//
// 実測比が現実的な範囲を外れる場合は初期値へ戻す。記録ミスで係数が壊れると
// 以後の全セッションの重量が狂うため。
//
// 引数の順序は「分子となるバリエーション、分母となるメイン」で固定している。
// 逆に呼ぶと係数が逆数になり、実際の係数域（0.85〜0.95）では範囲検査を
// すり抜けて処方重量が最大36%増える。
func (r VariationRatioResolver) Resolve(
	h History,
	variation *Exercise,
	mainID ExerciseID,
	asOf Date,
) Ratio {
	fallback := unitRatio
	if variation == nil || variation.Kind() != KindVariation {
		return fallback
	}
	if seed, ok := variation.DefaultRatioToMain(); ok {
		fallback = seed
	}
	if r.IsZero() {
		return fallback
	}

	// 実測に足るセッション数が無ければ、シードの仮の値を使う。
	if h.ForExercise(variation.ID()).SessionCount() < r.minSessions {
		return fallback
	}

	variationOneRM, ok := r.estimator.Estimate(h, variation.ID(), asOf)
	if !ok {
		return fallback
	}
	mainOneRM, ok := r.estimator.Estimate(h, mainID, asOf)
	if !ok {
		return fallback
	}

	measured, err := NewRatio(variationOneRM.Kg() / mainOneRM.Kg())
	if err != nil {
		return fallback
	}
	return measured
}
