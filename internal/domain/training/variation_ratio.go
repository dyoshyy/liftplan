package training

import "fmt"

const defaultMinRatioSessions = 3

// 実測した対メイン係数として受け入れる範囲。
//
// NewRatio の範囲（1e-6 〜 1.2）は「比率として成立するか」しか見ておらず、
// 対メイン係数としては広すぎる。ベンチ200kgの人にラーセンプレス2.5kgを
// 処方する係数（0.0139）が通ってしまう。
// メインの半分も挙がらないものはバリエーションではなく別の種目なので、
// 実測がこの範囲を外れたらシードの初期値へ戻す。
const (
	minMeasuredRatio = 0.5
	maxMeasuredRatio = 1.1
)

// unitRatio は「換算しない」を表す係数。
var unitRatio = Ratio{v: 1.0}

// VariationRatioResolver はバリエーション種目の対メイン係数を決めるドメインサービス。無状態。
//
// ラーセンプレスやテンポは通常フォームより挙がらない。同じ推定1RMの物差しに
// 乗せるため比率で換算する。
//
// 係数は「その人にとってその種目が通常フォームの何割か」という、比較的
// 安定した性質を表す。だから係数の推定には、同じ時点で観測されたメインと
// バリエーションの組を使い、その比を平滑する。
//
// EWMA 済みの推定1RMどうしの比を取ってはいけない。処方は
// メインの1RM × 強度 × 係数 で決まるため、係数が現在の
// バリエーション1RM ÷ 現在のメイン1RM だと、掛け算でメインの1RMが約分されて
// 消える。結果として処方はバリエーション自身の1RMだけで決まり、
// メインが伸びてもバリエーションが一切追随しない固定点に落ちる。
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
// 引数の順序は「分子となるバリエーション、分母となるメイン」で固定している。
// 逆に呼ぶと係数が逆数になり、実際の係数域（0.85〜0.95）では範囲検査を
// すり抜けて処方重量が最大36%増える。
//
// 実測に足るだけの組が集まらない場合や、実測比が対メイン係数として
// 現実的な範囲を外れる場合は、シードの初期値へ戻す。
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
	if r.IsZero() || asOf.IsZero() {
		return fallback
	}

	// バリエーション自体がブランク明けなら、係数も信用しない。
	if last, ok := h.LastPerformed(variation.ID()); !ok ||
		asOf.DaysSince(last) > r.estimator.MaxStaleDays() {
		return fallback
	}

	samples := r.pairedRatios(h, variation.ID(), mainID)
	if len(samples) < r.minSessions {
		return fallback
	}

	measured, err := NewRatio(ewma(samples, r.estimator.Alpha()))
	if err != nil {
		return fallback
	}
	return measured
}

// pairedRatios は、バリエーションを実施した各日について
// 「その日のバリエーション代表値 ÷ その時点でのメイン推定1RM」を古い順に返す。
//
// メインの推定は各セッション日を基準に行う。推定器が未来の記録を使わないため、
// その日までの情報だけで比が求まる。
func (r VariationRatioResolver) pairedRatios(h History, variationID, mainID ExerciseID) []float64 {
	sessions := h.ForExercise(variationID).Sessions()
	out := make([]float64, 0, len(sessions))

	for _, s := range sessions {
		variationORM, ok := s.MedianOneRepMax()
		if !ok {
			continue
		}
		// Estimate は asOf より後の記録を使わないので、その日までの
		// 情報だけでメインの1RMが求まる。
		mainORM, ok := r.estimator.Estimate(h, mainID, s.Date())
		if !ok {
			continue
		}

		ratio := variationORM.Kg() / mainORM.Kg()
		if ratio < minMeasuredRatio || ratio > maxMeasuredRatio {
			// 対メイン係数として現実的でない組は捨てる。記録ミスや、
			// リハビリ期の極端に軽いセッションがこれにあたる。
			continue
		}
		out = append(out, ratio)
	}
	return out
}

// ewma は古い順に並んだ値を指数移動平均で畳み込む。呼び出し側が非空を保証すること。
func ewma(values []float64, alpha float64) float64 {
	acc := values[0]
	for _, v := range values[1:] {
		acc = alpha*v + (1-alpha)*acc
	}
	return acc
}
