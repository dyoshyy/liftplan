package training

import (
	"fmt"
	"math"
)

const (
	defaultEstimatorAlpha      = 0.3
	defaultEstimatorHysteresis = 0.02
)

// OneRepMaxEstimator は履歴から推定1RMを導くドメインサービス。無状態。
//
// 単発の記録で全スロットの重量が動くと不安定になる。調子が良かった日の
// 1セットで重量が跳ね上がり、翌週それを引きずって潰れるのを防ぐため、
// セッション中央値 → EWMA → ヒステリシス の順に均す。
type OneRepMaxEstimator struct {
	alpha      float64
	hysteresis float64
}

// NewOneRepMaxEstimator は平滑化の設定を検証して組み立てる。
//
// alpha は直近セッションの重み。1 に近いほど追随が速く、0 に近いほど鈍い。
// hysteresis は「この割合未満の変化なら前回値を維持する」閾値。
func NewOneRepMaxEstimator(alpha, hysteresis float64) (OneRepMaxEstimator, error) {
	if math.IsNaN(alpha) || alpha <= 0 || alpha > 1 {
		return OneRepMaxEstimator{}, fmt.Errorf("alpha は0より大きく1以下である必要がある: %v", alpha)
	}
	if math.IsNaN(hysteresis) || hysteresis < 0 || hysteresis >= 1 {
		return OneRepMaxEstimator{}, fmt.Errorf("ヒステリシスは0以上1未満である必要がある: %v", hysteresis)
	}
	return OneRepMaxEstimator{alpha: alpha, hysteresis: hysteresis}, nil
}

func DefaultOneRepMaxEstimator() OneRepMaxEstimator {
	return OneRepMaxEstimator{alpha: defaultEstimatorAlpha, hysteresis: defaultEstimatorHysteresis}
}

func (e OneRepMaxEstimator) Alpha() float64      { return e.alpha }
func (e OneRepMaxEstimator) Hysteresis() float64 { return e.hysteresis }

// IsZero はゼロ値（未設定）かどうか。
func (e OneRepMaxEstimator) IsZero() bool { return e == OneRepMaxEstimator{} }

// Estimate は指定種目の平滑化された推定1RM。
//
// 推定できるセッションが1つも無ければ false を返す。自重種目しか記録が無い
// 場合や、履歴そのものが無い場合がこれにあたる。
//
// previous に前回公表した値を渡すと、変化がヒステリシス閾値未満のとき
// 前回値を維持する。重量が毎回ちらつくのを防ぐ。
func (e OneRepMaxEstimator) Estimate(h History, id ExerciseID, previous *OneRepMax) (OneRepMax, bool) {
	if e.IsZero() {
		return OneRepMax{}, false
	}

	acc, ok := e.smooth(h, id)
	if !ok {
		return OneRepMax{}, false
	}

	candidate, err := NewOneRepMax(acc)
	if err != nil {
		return OneRepMax{}, false
	}
	if previous == nil || previous.IsZero() {
		return candidate, true
	}

	change := math.Abs(candidate.Kg()-previous.Kg()) / previous.Kg()
	if change < e.hysteresis {
		return *previous, true
	}
	return candidate, true
}

// smooth はセッション代表値を古い順に EWMA で畳み込む。
func (e OneRepMaxEstimator) smooth(h History, id ExerciseID) (float64, bool) {
	var acc float64
	started := false

	for _, s := range h.ForExercise(id).Sessions() {
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
