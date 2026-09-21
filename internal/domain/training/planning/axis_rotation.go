package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// axis は今日の軸と、その強度を返す。
//
// 重点種目の番に来たときだけ一巡する。3レップ相当 → 6レップ相当 → 派生。
//
// 派生を軸に出すのは、バリエーションレーンが届かない日があるため。あちらは
// 軸が系統に含まれる日は出ない（D-125）ので、上半身の日が毎回ベンチになる
// 構成では派生がどこにも出ない。
func (p SessionPlanner) axis(
	req PlanRequest, pool, declared []*exercise.Exercise,
	today program.Split, hasSplit bool,
) (*exercise.Exercise, float64) {
	lift := p.heavyLift(req, declared, today, hasSplit)
	if lift == nil {
		return nil, heavyIntensityPct
	}

	focus, ok := req.Program.FocusExercise()
	if !ok || lift.ID() != focus {
		return lift, heavyIntensityPct
	}

	h := historyBefore(req)
	switch focusCyclePosition(h, lineage(pool, focus)) {
	case 1:
		return lift, focusVolumeIntensityPct
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
		if d := stalest(h, candidates); d != nil {
			return d, heavyIntensityPct
		}
	}
	return lift, heavyIntensityPct
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
func (p SessionPlanner) heavyLift(
	req PlanRequest, declared []*exercise.Exercise,
	today program.Split, hasSplit bool,
) *exercise.Exercise {
	candidates := declared
	if hasSplit {
		candidates = primaryIn(candidates, today)
	}
	return stalest(historyBefore(req), candidates)
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
// 渡す履歴は前日まで（historyBefore）。当日を含めると、ジムで1セット記録した
// 瞬間に「最も古い」が入れ替わり、今日のメニューが自分の下で変わる（D-116）。
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
