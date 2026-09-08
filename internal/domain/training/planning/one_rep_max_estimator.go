package planning

import (
	"fmt"
	"math"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

const defaultEstimatorAlpha = 0.3

// defaultMaxStaleDays はこの日数より古い記録しか無い種目の推定1RMを
// 信用しない、という境界。
//
// 6週間ブランクがあると筋力は明確に落ちる。にもかかわらず推定1RMは
// セッション数だけで畳み込まれるため、離脱前の値がそのまま残る。
// 復帰初日に離脱前の重量を処方するのは危険なので、「推定できない」を返して
// ユーザーに決めさせる。数字を捏造しないという方針と同じ扱いにする。
const defaultMaxStaleDays = 42

// OneRepMaxEstimator は履歴から推定1RMを導くドメインサービス。無状態。
//
// 単発の記録で全スロットの重量が動くと不安定になる。調子が良かった日の
// 1セットで重量が跳ね上がり、翌週それを引きずって潰れるのを防ぐため、
// セッション中央値 → EWMA の順に均す。
//
// 重量のちらつき自体は、実施重量を増加単位のグリッドへ丸めることで
// 吸収される。推定1RMが 1kg 動いても 2.5kg 刻みの処方は変わらない。
type OneRepMaxEstimator struct {
	alpha        float64
	maxStaleDays int
}

// NewOneRepMaxEstimator は平滑化の設定を検証して組み立てる。
//
// alpha は直近セッションの重み。1 に近いほど追随が速く、0 に近いほど鈍い。
// maxStaleDays はこれより古い記録しか無い種目を「推定できない」とする境界。
func NewOneRepMaxEstimator(alpha float64, maxStaleDays int) (OneRepMaxEstimator, error) {
	if math.IsNaN(alpha) || alpha <= 0 || alpha > 1 {
		return OneRepMaxEstimator{}, fmt.Errorf("alpha は0より大きく1以下である必要がある: %v", alpha)
	}
	if maxStaleDays < 1 {
		return OneRepMaxEstimator{}, fmt.Errorf("鮮度の上限は1日以上である必要がある: %d", maxStaleDays)
	}
	return OneRepMaxEstimator{alpha: alpha, maxStaleDays: maxStaleDays}, nil
}

func DefaultOneRepMaxEstimator() OneRepMaxEstimator {
	return OneRepMaxEstimator{alpha: defaultEstimatorAlpha, maxStaleDays: defaultMaxStaleDays}
}

func (e OneRepMaxEstimator) Alpha() float64    { return e.alpha }
func (e OneRepMaxEstimator) MaxStaleDays() int { return e.maxStaleDays }
func (e OneRepMaxEstimator) IsZero() bool      { return e == OneRepMaxEstimator{} }

// Estimate は asOf 時点での、指定種目の平滑化された推定1RM。
//
// 推定できない場合は false を返す。次の3つがある。
//   - 履歴が無い、または推定できるセッションが1つも無い（自重種目だけなど）
//   - 最後の記録が古すぎる（ブランク明け）
//   - 平滑後の値が推定1RMとして無効
//
// asOf を必ず受け取るのは、セッション数だけで畳み込むと3ヶ月のブランクが
// あっても直前のセッションと同じ重みになり、離脱前の重量がそのまま
// 処方されてしまうため。
func (e OneRepMaxEstimator) Estimate(h setlog.History, id exercise.ExerciseID, asOf training.Date) (training.OneRepMax, bool) {
	if e.IsZero() || asOf.IsZero() {
		return training.OneRepMax{}, false
	}

	// asOf より後の記録は使わない。過去のある時点の推定をやり直すとき、
	// 未来の記録が混ざると「その時点で分かっていたこと」にならない。
	known := h.OnOrBefore(asOf).ForExercise(id)
	if e.isStale(known, id, asOf) {
		return training.OneRepMax{}, false
	}

	acc, ok := e.smooth(known)
	if !ok {
		return training.OneRepMax{}, false
	}

	orm, err := training.NewOneRepMax(acc)
	if err != nil {
		return training.OneRepMax{}, false
	}
	return orm, true
}

// isStale は最後の記録が古すぎるか。履歴が無い場合も古い扱いにする。
func (e OneRepMaxEstimator) isStale(h setlog.History, id exercise.ExerciseID, asOf training.Date) bool {
	last, ok := h.LastPerformed(id)
	if !ok {
		return true
	}
	return asOf.DaysSince(last) > e.maxStaleDays
}

// smooth はセッション代表値を古い順に EWMA で畳み込む。
//
// 引数は対象種目だけに絞り込み済みの履歴でなければならない。絞り込まないと、
// 同一日に別種目のセットがあったときその中央値が混ざり、推定が大きく狂う。
//
// 畳み込める代表値が1つも無ければ false を返す。このとき acc は0のままだが、
// 呼び出し側は false を見て打ち切ること。NewOneRepMax が0を弾くことに
// 依存すると、将来1RMの下限を変えたときに静かに壊れる。
func (e OneRepMaxEstimator) smooth(forExercise setlog.History) (float64, bool) {
	var acc float64
	started := false

	for _, s := range forExercise.Sessions() {
		m, ok := s.MedianOneRepMax()
		if !ok {
			// 推定できないセッション（全セット自重など）は畳み込みから除く。
			// 0 として混ぜると、その回だけで推定1RMが大きく落ちる。
			continue
		}
		if !started {
			acc = m.Kg()
			started = true
			continue
		}
		acc = e.alpha*m.Kg() + (1-e.alpha)*acc
	}
	return acc, started
}
