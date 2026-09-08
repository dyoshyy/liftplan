package program

import "fmt"

// maxFrequencyPerWeek は週の頻度の上限。
//
// 4に抑えているのは、毎セッションで BIG3 すべてにスロットを割り当てる設計だから。
// 週5回にすると1種目あたり19セット/週、週7回なら27セット/週になり、
// デッドリフトを週27セットこなす前提のプログラムになってしまう。
// それより多く通う場合は、頻度を上げるのではなく補助種目の日を自分で足す。
const maxFrequencyPerWeek = 4

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
