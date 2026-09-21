package planning

import (
	"slices"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// variationLift は今日バリエーションとしてやる種目を返す。出さない日は nil
//
// 出さないのは、重点種目が未指定・軸が系統に含まれる・前回やってから十分に日数がアイていない・派生が選択されていない
// のいずれか。
func (p SessionPlanner) variationLift(
	req PlanRequest, pool []*exercise.Exercise, heavy *exercise.Exercise,
	today program.Split, hasSplit bool,
) *exercise.Exercise {
	focus, ok := req.Program.FocusExercise()
	if !ok {
		return nil
	}

	// 分割があれば、重点種目の主働が今日の集合に含まれる日だけ出す。
	//
	// 止めないと、型が「今日は脚の日」と言いながらベンチの派生が出る。
	// 型の意味を自分で否定することになる。
	//
	// 代償は系統の頻度が下がること。重点ベンチ＋上下2分割なら、上の日の
	// 数がそのまま上限になる。ベンチを週3回やりたいなら上の日を3つ置く
	// 周期を組む、が正しい答えで、順序付きの周期ならそれができる。
	if hasSplit {
		e := findExercise(pool, focus)
		if e == nil || !isPrimaryIn(e, today) {
			return nil
		}
	}

	// 今日の軸が重点種目の系統に含まれる場合、バリエーションは出さない。
	//
	// 軸が空の日がある（分割に該当する宣言が無い日）。そのときは
	// 系統の重複が起きようがないので、この門は素通しする。
	family := lineage(pool, focus)
	if heavy != nil && containsExercise(family, heavy.ID()) {
		return nil
	}

	// 前回やってから十分に日数が空いていない場合、バリエーションは出さない。
	h := historyBefore(req)
	if recentlyPerformed(h, family, req.Date) {
		return nil
	}

	return stalest(h, variationsOf(pool, focus))
}

// accessoryExcluded は補助の候補から外す種目を返す。
//
// 宣言種目そのものを外す理由は D-125 のとおり。これに重点種目の派生を
// 足す。派生はバリエーションレーンで出るものなので、補助にも出ると
// 同じ系統が1日に二度来る。
//
// 実害は週5で出た。脚の日に胸の残差が大きく残っていると、補助が
// ベンチの派生（ラーセンプレス・テンポベンチ）を2つ選び、脚の日の
// 上半身ボリュームが 17.1 まで膨らむ。胸を埋めるならインクラインや
// フライで埋めるほうが、系統の回復日程と衝突しない。
//
// **重点種目の系統だけ**を外す。宣言していても重点でない種目の派生
// （RDL・フロントスクワット・デフィシットデッドリフト）は、補助が
// 唯一の出口なので外すと計画から消える。実際に全部外して測ったら、
// 胸が週目標の163%まで超過し、使われない種目が出た。専用レーンを
// 持っているのは重点種目の系統だけ、というのが線引き。
func accessoryExcluded(pool []*exercise.Exercise, prog *program.Program) []exercise.ExerciseID {
	out := prog.DeclaredExercises()

	// 重点種目が未指定なら focus は空ID。lineage は空を返すので、
	// ここで分けない。分けても到達しない分岐が増えるだけ。
	focus, _ := prog.FocusExercise()
	for _, e := range lineage(pool, focus) {
		out = append(out, e.ID())
	}
	return out
}

// lineage は重点種目とその派生のうち、pool にあるものを返す。
//
// 重点種目自身を含める。含めないと、軸でベンチをやった翌日にラーセンが出る。
//
// 根まで辿らない。辿ると、重点種目に RDL を指定したとき「RDL の系統」に
// 床引きデッドリフトが入り、バリエーションとして出てしまう。床引きは
// 宣言しなければ出ない（D-117）。
func lineage(pool []*exercise.Exercise, focus exercise.ExerciseID) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, 4)
	for _, e := range pool {
		if e.ID() == focus {
			out = append(out, e)
			continue
		}
		if from, ok := e.DerivedFrom(); ok && from == focus {
			out = append(out, e)
		}
	}
	return out
}

// variationsOf は重点種目の派生のうち pool にあるものを返す。重点種目自身は含まない。
func variationsOf(pool []*exercise.Exercise, focus exercise.ExerciseID) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, 3)
	for _, e := range pool {
		if from, ok := e.DerivedFrom(); ok && from == focus {
			out = append(out, e)
		}
	}
	return out
}

// recentlyPerformed は系統のどれかを直近 variationRecoveryDays 日にやったか。
//
// AccessorySelector.recovering と同じ開区間 (date - N, date)。区分ではなく
// 系統で見る点だけが違う。
//
// 渡す履歴は前日まで。当日を含めると、今日ラーセンを1セット記録して
// 開き直した瞬間に系統が「最近やった」になり、バリエーションが自分の下で
// 消える（D-116 系）。
func recentlyPerformed(h setlog.History, family []*exercise.Exercise, date training.Date) bool {
	inFamily := make(map[exercise.ExerciseID]bool, len(family))
	for _, e := range family {
		inFamily[e.ID()] = true
	}

	cutoff := date.AddDays(-variationRecoveryDays)
	for _, l := range h.After(cutoff).Before(date).Logs() {
		if inFamily[l.ExerciseID()] {
			return true
		}
	}
	return false
}

// containsExercise は候補のどれかが id か。
//
// ポインタではなく ID で比べる。エンティティの同一性は ID で決まるので
// （Exercise.SameIdentity）、スライスの作り方が変わっても壊れない。
func containsExercise(candidates []*exercise.Exercise, id exercise.ExerciseID) bool {
	return slices.ContainsFunc(candidates, func(e *exercise.Exercise) bool { return e.ID() == id })
}
