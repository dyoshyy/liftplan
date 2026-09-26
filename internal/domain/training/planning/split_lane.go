package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// affiliated はその区分が、周期のどこかの日に書かれているか。
//
// 書かれていない区分は「その日の分割に無い」のではなく「どの日にも
// 属さない」。前者は別の日に来るが、後者は二度と来ない。
func affiliated(cycle []program.Split, r training.MuscleRegion) bool {
	for _, day := range cycle {
		if day.Includes(r) {
			return true
		}
	}
	return false
}

// primaryIn はその分割の区分を主働に含む種目だけを返す。
//
// 「主働」は寄与 1.0 以上。最大値を取る方式にしないのは、デッドリフトが
// ハムストリングと脊柱起立筋のどちらも 1.0 で、並びのアルファベット順に
// 落ちてしまうため。閾値なら両方の日の候補になり、最終実施日が決める。
func primaryIn(candidates []*exercise.Exercise, s program.Split) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, len(candidates))
	for _, e := range candidates {
		if isPrimaryIn(e, s) {
			out = append(out, e)
		}
	}
	return out
}

// isPrimaryIn はその種目の主働区分が分割に含まれるか。
func isPrimaryIn(e *exercise.Exercise, s program.Split) bool {
	for _, r := range e.Stimulus().Regions() {
		c, ok := e.Stimulus().Contribution(r)
		if ok && c.Float() >= primaryContribution && s.Includes(r) {
			return true
		}
	}
	return false
}

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
