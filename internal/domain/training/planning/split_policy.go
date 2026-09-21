package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// DeclaredWithoutADay はプログラムの宣言種目のうち、周期のどの日にも
// 出られないものを宣言の順に返す。
//
// 出られない種目は毎日「今日の候補ではない」と判定され、二度と軸に出ない。
// 他の宣言が毎日1つは該当するのでフォールバックも発火せず、エラーも立たない
// まま消える。保存の前にこれで弾く。
//
// 判定は軸の候補を絞るのと同じ isPrimaryIn で書く。写しを持つと、片方だけ
// 閾値を動かしたときに「保存は通るのに軸に出ない」が黙って起きる。
//
// 分割なし（周期が空）なら何も返さない。分割で候補を絞らないので、出られない
// 種目が無い。pool に無い宣言も返さない。それは分割の問題ではない。
func DeclaredWithoutADay(pool []*exercise.Exercise, prog *program.Program) []exercise.ExerciseID {
	cycle := prog.Cycle()
	if len(cycle) == 0 {
		return nil
	}

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	for _, e := range pool {
		if e != nil {
			byID[e.ID()] = e
		}
	}

	var out []exercise.ExerciseID
	for _, id := range prog.DeclaredExercises() {
		e, ok := byID[id]
		if !ok {
			continue
		}
		if !hasADay(e, cycle) {
			out = append(out, id)
		}
	}
	return out
}

// hasADay はその種目が出られる日が周期にあるか。
func hasADay(e *exercise.Exercise, cycle []program.Split) bool {
	for _, s := range cycle {
		if isPrimaryIn(e, s) {
			return true
		}
	}
	return false
}
