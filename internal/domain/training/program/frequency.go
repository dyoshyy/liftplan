package program

import "fmt"

// maxFrequencyPerWeek は週の頻度の上限。
//
// 7は「1週間は7日しかない」以上の意味を持たない。上限を置いているのは
// 入力の検証のためで、それ以上の設計上の理由は無い。
//
// 以前は4だった。理由は「毎セッションで BIG3 すべてにスロットを割り当てる
// 設計だから。週7回ならデッドリフトを週27セットこなす前提になる」。
// D-117 で軸が1セッション1種目になり、宣言のうち最後にやったのが最も古い
// ものを回すようになった時点で、この根拠は消えていた。
//
// 週5〜7を通し検証に足して測った（simulation_test.go）。
// 1セッション27セットは全頻度で変わらず、区分ごとの達成率も許容内に収まる。
//
//	週1回 77〜125%   週5回 80〜130%
//	週2回 80〜123%   週6回 71〜118%
//	週3回 75〜133%   週7回 70〜108%
//	週4回 85〜108%
const maxFrequencyPerWeek = 7

// Frequency は週あたりのトレーニング回数。
type Frequency struct {
	perWeek int
}

func NewFrequency(perWeek int) (Frequency, error) {
	if perWeek < 1 || perWeek > maxFrequencyPerWeek {
		return Frequency{}, fmt.Errorf("週の頻度は1〜%d回である必要がある: %d", maxFrequencyPerWeek, perWeek)
	}
	return Frequency{perWeek: perWeek}, nil
}

func (f Frequency) PerWeek() int { return f.perWeek }

func (f Frequency) IsZero() bool { return f == Frequency{} }
