package program

import "fmt"

const (
	// minAxisReps と maxAxisReps は軸で狙うレップ数の範囲。
	//
	// 上限 15 は、軸の RIR1 を足した限界までの総レップ（16）が、推定に
	// 使える上限（training.maxRepsToFailure = 20）の内側に収まるため。
	// Epley は直線近似で、高レップほど粗くなる。
	minAxisReps = 1
	maxAxisReps = 15

	// defaultHeavyReps と defaultLightReps は今の処方と同じ値（0.88・0.81）。
	// 変えると、レップ数を設定していない全員の重量が動く。
	defaultHeavyReps = 3
	defaultLightReps = 6
)

// RepTargets は宣言種目を軸で出すときに狙うレップ数。不変。
//
// heavy は重い番（宣言の軸、重点種目の一巡の1番目）、light は軽い番
// （重点種目の一巡の2番目と、派生が軸に立つ3番目）。どちらを重くするかは
// 本人の判断なので、heavy ≤ light は強制しない。
//
// 種目マスタではなく宣言に持つ。「この種目を何レップで伸ばしたいか」は
// 種目の性質ではなく本人の目標で、メイン/補助を Program へ移したのと
// 同じ理屈（D-117）。
type RepTargets struct {
	heavy, light int
}

// NewRepTargets は範囲（1〜15）を検証して RepTargets を組み立てる。
func NewRepTargets(heavy, light int) (RepTargets, error) {
	for _, f := range []struct {
		name string
		v    int
	}{{"重い番", heavy}, {"軽い番", light}} {
		if f.v < minAxisReps || f.v > maxAxisReps {
			return RepTargets{}, fmt.Errorf("%sのレップ数は%d〜%dである必要がある: %d",
				f.name, minAxisReps, maxAxisReps, f.v)
		}
	}
	return RepTargets{heavy: heavy, light: light}, nil
}

// DefaultRepTargets は設定していない宣言に使う値（重い番3・軽い番6）。
func DefaultRepTargets() RepTargets {
	return RepTargets{heavy: defaultHeavyReps, light: defaultLightReps}
}

func (r RepTargets) Heavy() int   { return r.heavy }
func (r RepTargets) Light() int   { return r.light }
func (r RepTargets) IsZero() bool { return r == RepTargets{} }
