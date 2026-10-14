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

	// defaultHeavyReps は今の軸の処方と同じ値（0.88）。変えると、レップ数を
	// 設定していない全員の重量が動く。
	defaultHeavyReps = 3
)

// RepTargets は宣言種目を軸で出すときに狙うレップ数。不変。
//
// 軸は重点種目かどうかによらず heavy（重い番）で出る。以前は重点種目だけ
// 重い番 → 軽い番 → 派生 と一巡させ、軽い番のレップ数も持っていたが、
// 軽い日はバリエーションレーンが派生で作るのでやめた（#313）。
//
// 種目マスタではなく宣言に持つ。「この種目を何レップで伸ばしたいか」は
// 種目の性質ではなく本人の目標で、メイン/補助を Program へ移したのと
// 同じ理屈（D-117）。
type RepTargets struct {
	heavy int
}

// NewRepTargets は範囲（1〜15）を検証して RepTargets を組み立てる。
func NewRepTargets(heavy int) (RepTargets, error) {
	if heavy < minAxisReps || heavy > maxAxisReps {
		return RepTargets{}, fmt.Errorf("重い番のレップ数は%d〜%dである必要がある: %d",
			minAxisReps, maxAxisReps, heavy)
	}
	return RepTargets{heavy: heavy}, nil
}

// DefaultRepTargets は設定していない宣言に使う値（重い番3）。
func DefaultRepTargets() RepTargets {
	return RepTargets{heavy: defaultHeavyReps}
}

func (r RepTargets) Heavy() int   { return r.heavy }
func (r RepTargets) IsZero() bool { return r == RepTargets{} }
