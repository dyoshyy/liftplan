package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// axis は今日の軸と、その役割（heavyRole・focusVolumeRole・focusVariationRole）を返す。
//
// 重点種目の番に来たときだけ一巡する。3レップ相当 → 6レップ相当 → 派生。
// 一巡のどこにいるかを数えるのは種目の判断で、その位置にどの強度を当てるかは
// 重量の判断。ここは前者だけを扱い、強度の値は持たない（prescriptionFor）。
//
// 派生を軸に出すのは、バリエーションレーンが届かない日があるため。あちらは
// 軸が系統に含まれる日は出ない（D-125）ので、上半身の日が毎回ベンチになる
// 構成では派生がどこにも出ない。
//
// 重点種目の系統は、中1日空けてから軸に立てる（focusRested）。
//
// history は前日まで（Plan が切る）。
func axis(
	history setlog.History, prog *program.Program, pool, declared []*exercise.Exercise,
	today program.Split, hasSplit bool, date training.Date,
) (*exercise.Exercise, laneRole) {
	lift := heavyLift(history, focusRested(history, prog, pool, declared, date), today, hasSplit)
	if lift == nil {
		// 休ませたい系統しか今日の候補に無ければ、それを出す。軸を空に
		// すると、その日の主役が消える。
		lift = heavyLift(history, declared, today, hasSplit)
	}
	if lift == nil {
		return nil, heavyRole
	}

	focus, ok := prog.FocusExercise()
	if !ok || lift.ID() != focus {
		return lift, heavyRole
	}

	switch focusCyclePosition(history, lineage(pool, focus)) {
	case 1:
		return lift, focusVolumeRole
	case 2:
		// 派生も分割で絞る。軸の候補（heavyLift）は絞っているのに
		// ここだけ素通しにすると、胸の日にナローベンチ（主働は三頭）が
		// 軸として出る。型が「今日は胸の日」と言いながら三頭を主役に据える。
		//
		// 該当が無ければ本体を重い側で出す（選択から外した派生も同じ経路）。
		candidates := variationsOf(pool, focus)
		if hasSplit {
			candidates = primaryIn(candidates, today)
		}
		if d := stalest(history, candidates); d != nil {
			return d, focusVariationRole
		}
	}
	return lift, heavyRole
}

// focusRested は宣言のうち、重点種目の系統を直近 variationRecoveryDays 日に
// やっていれば、その系統に属するものを除いて返す。
//
// バリエーションレーンは系統を中1日空けて出す（variationLift）。軸の側が
// それを見ないと、月曜にバリエーションで派生、火曜に軸で本体が出る。
// 軸は「宣言のうち最も古いもの」なので、派生をやっても本体は古いまま
// 選ばれる。
//
// 重点種目の系統だけを見る。重点でない宣言の派生が補助で出た翌日に本体が
// 軸に立つことは止めない。それを止めるには補助の予測まで履歴に入れる
// 必要があるが、ProjectHorizon は軸とバリエーションしか予測に書かないので、
// 予測と実際の計画がずれる。
//
// 除く対象は宣言の中の系統なので、RDL を宣言して重点をデッドリフトに
// した場合は RDL も休ませる（lineage に含まれる）。
func focusRested(
	history setlog.History, prog *program.Program, pool, declared []*exercise.Exercise,
	date training.Date,
) []*exercise.Exercise {
	focus, ok := prog.FocusExercise()
	if !ok {
		return declared
	}
	family := lineage(pool, focus)
	if !recentlyPerformed(history, family, date) {
		return declared
	}
	out := make([]*exercise.Exercise, 0, len(declared))
	for _, e := range declared {
		if !containsExercise(family, e.ID()) {
			out = append(out, e)
		}
	}
	return out
}

// focusCyclePosition は重点種目の一巡のうち、今日がどこかを返す。
//
// 数えるのは「系統のどれかを実施したセッション数」。本体の実施回数で
// 数えると、派生をやった日に位置が進まず同じ派生が出続ける。
func focusCyclePosition(h setlog.History, family []*exercise.Exercise) int {
	inFamily := make(map[exercise.ExerciseID]bool, len(family))
	for _, e := range family {
		inFamily[e.ID()] = true
	}

	n := 0
	for _, s := range h.Sessions() {
		for _, l := range s.Logs() {
			if inFamily[l.ExerciseID()] {
				n++
				break
			}
		}
	}
	return n % focusCycleLength
}

// heavyLift は今日メインでやる＝高重量を扱う種目を返す。
//
// 宣言のうち、最後に実施したのが最も古い種目を返す。未着手の種目が
// あればそれを優先する。
//
// 分割があれば、その日の区分を主働に含む宣言だけが候補になる。該当が
// 無ければ nil。フォールバックで別の日の種目を出すと、その日だけ分割が
// 意味を失う。
func heavyLift(
	history setlog.History, declared []*exercise.Exercise,
	today program.Split, hasSplit bool,
) *exercise.Exercise {
	candidates := declared
	if hasSplit {
		candidates = primaryIn(candidates, today)
	}
	return stalest(history, candidates)
}

// stalest は候補のうち、最後に実施したのが最も古い種目を返す。候補が空なら nil。
//
// 未着手の種目があればそれを優先する。記録が無いのを「最も古い」と解釈する
// ため、ゼロ値の日付と比べるのではなく LastPerformed の第2返り値で分ける。
// 日付のゼロ値が何を表すかを知らなくても読める。
//
// 同点は先に見たものを残す。候補は usablePool が ID 昇順に並べているので、
// 同じ入力から同じ種目が返る。
//
// 渡す履歴は前日まで。理由は Plan に書いた。
func stalest(h setlog.History, candidates []*exercise.Exercise) *exercise.Exercise {
	var best *exercise.Exercise
	var bestDate training.Date

	for _, c := range candidates {
		last, ok := h.LastPerformed(c.ID())
		if !ok {
			return c
		}
		if best == nil || last.Before(bestDate) {
			best, bestDate = c, last
		}
	}
	return best
}
