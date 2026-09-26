package program

import "fmt"

// 1セッションの量の範囲。
//
// 下限が2なのは、軸だけの日を既定にしないため。1にすると宣言種目しか
// 出ず、補助レーンも配分表も働く余地が無くなる。
//
// 上限が6なのは、それ以上をやりきれる人がいないから。以前は上限が無く、
// 軸1＋補助8の9種目27セットが全頻度・全セッションで固定的に出ていた
// （通し検証で実測）。長いリストは「上から消して好きなところでやめて
// よい」と言っても、全部やれという圧としてしか働かない。
const (
	minExercisesPerSession = 2
	maxExercisesPerSession = 6
	minSetsPerExercise     = 2
	maxSetsPerExercise     = 6
)

// SessionVolume は1セッションの量。**利用者が答える問い。**
//
// 何kgでやるか・補助を何にするかはアプリが決めるが、1回にどれだけ時間を
// 取れるかはアプリには知りようがない。週の頻度と同じ性質の設定で、
// 「何セットが適切か」（本人に答えられない）とは別の問いである。
//
// 種目数とセット数を1つの値にまとめているのは、常に一緒に決まるため。
// 片方だけ動かす場面が無く、分けると週の総量を出すのに毎回2つ持ち回る。
type SessionVolume struct {
	exercises int
	sets      int
}

func NewSessionVolume(exercises, sets int) (SessionVolume, error) {
	if exercises < minExercisesPerSession || exercises > maxExercisesPerSession {
		return SessionVolume{}, fmt.Errorf("1回の種目数は%d〜%dである必要がある: %d",
			minExercisesPerSession, maxExercisesPerSession, exercises)
	}
	if sets < minSetsPerExercise || sets > maxSetsPerExercise {
		return SessionVolume{}, fmt.Errorf("1種目あたりのセット数は%d〜%dである必要がある: %d",
			minSetsPerExercise, maxSetsPerExercise, sets)
	}
	return SessionVolume{exercises: exercises, sets: sets}, nil
}

func (v SessionVolume) Exercises() int { return v.exercises }
func (v SessionVolume) Sets() int      { return v.sets }

// TotalSets は1セッションの総セット数。
func (v SessionVolume) TotalSets() int { return v.exercises * v.sets }

func (v SessionVolume) IsZero() bool { return v == SessionVolume{} }
