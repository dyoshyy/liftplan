package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// EditExerciseInput は種目を直す入力。Add と同じ形
// （docs/specs/2026-09-26-custom-exercises-design.md
// 「直せるもの | 名前、効き方（区分ごとの寄与）、刻み」）。
type EditExerciseInput struct {
	Name        string
	Stimulus    map[training.MuscleRegion]float64
	IncrementKg float64
}

// EditExercise は利用者が種目の名前・効き方・刻みを直す。
//
// プリセット由来かどうかで扱いを変えない。ID・自重係数・派生元は
// Exercise.Edit がそのまま引き継ぐので、ここでは触らない。
//
// programs は「宣言種目がどの分割日にも出られなくなっていないか」を
// 見るためだけに読む。SetDeclaredExercises・SetSplitCycle が保存の前に
// 見ているのと同じ境界を、効き方を直す入口にも張る（さもないと、直した
// 結果その種目の寄与1.0の区分がどの分割日にも無くなり、保存はできても
// 二度と軸に出ない種目ができる）。プログラムが無ければ判定をスキップする
// （Add と違い、Edit は使う種目に入れる操作ではなくカタログを直す操作
// なので、プログラムを前提にしない）。
type EditExercise struct {
	exercises exerciseStore
	programs  program.Reader
}

func NewEditExercise(exercises exerciseStore, programs program.Reader) *EditExercise {
	return &EditExercise{exercises: exercises, programs: programs}
}

func (u *EditExercise) Execute(ctx context.Context, user account.UserID, id exercise.ExerciseID, in EditExerciseInput) (_ *exercise.Exercise, err error) {
	defer func() { err = apperror.Classify(err) }()

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	var target *exercise.Exercise
	for _, e := range pool {
		if e != nil && e.ID() == id && !e.IsDeleted() {
			target = e
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("%w: %s", apperror.ErrExerciseNotFound, id)
	}

	edited, err := target.Edit(exercise.ExerciseEdit{
		Name: in.Name, Stimulus: in.Stimulus, IncrementKg: in.IncrementKg,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", apperror.ErrInvalidInput, err)
	}

	// 重複チェックは自分自身を除く。消していない他の種目とだけ比べる
	// （docs/specs/2026-09-26-custom-exercises-design.md「自分自身を除く」）。
	for _, other := range pool {
		if other != nil && other.ID() != id && !other.IsDeleted() && other.Name() == edited.Name() {
			return nil, fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, edited.Name())
		}
	}

	prog, err := u.programs.Get(ctx, user)
	switch {
	case err == nil:
		pool = replaceByID(pool, id, edited)
		// verifyDeclaredHaveADay ではなく直接 planning を見るのは、あちらの
		// 文言が raw ID を埋め込んでいるため。SetDeclaredExercises・
		// SetSplitCycle では「そのIDの種目を選び直す／分割を直す」操作
		// なので ID で十分だが、Edit の原因は「いま直した効き方」であり、
		// 利用者は画面の名前でしか種目を特定できない。
		if without := planning.DeclaredWithoutADay(pool, prog); len(without) > 0 {
			return nil, editLosesADayError(pool, without[0])
		}
	case errors.Is(err, program.ErrProgramNotConfigured):
		// プログラムがまだ無い。出られる日の判定は保留し、カタログの
		// 変更だけ通す。
	default:
		return nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("種目の変更が中断された: %w", err)
	}
	if err := u.exercises.Save(ctx, user, edited); err != nil {
		return nil, err
	}
	return edited, nil
}

// editLosesADayError は、効き方の変更で宣言種目がどの分割日にも出られなく
// なったことを、名前で伝えるエラーを作る。id は pool（直した後の姿）から
// 名前を引く。見つからない場合（起こらないはずだが）は ID をそのまま使う。
func editLosesADayError(pool []*exercise.Exercise, id exercise.ExerciseID) error {
	name := string(id)
	for _, e := range pool {
		if e != nil && e.ID() == id {
			name = e.Name()
			break
		}
	}
	return fmt.Errorf(
		"%w: 効き方をこう直すと、伸ばしたい種目 %q がどの分割日にも出られなくなる。主働の筋区分をどれかの分割に入れること",
		apperror.ErrInvalidInput, name)
}

// replaceByID は同じ ID の要素を differ に差し替えた新しいスライスを返す。
// 元は変えない。verifyDeclaredHaveADay に「直した後」の姿で種目マスタを
// 渡すために使う。まだ保存していない edited を混ぜて見るので、pool を
// 直接書き換えると呼び出し側の pool も一緒に変わってしまう。
func replaceByID(pool []*exercise.Exercise, id exercise.ExerciseID, replacement *exercise.Exercise) []*exercise.Exercise {
	out := make([]*exercise.Exercise, len(pool))
	for i, e := range pool {
		if e != nil && e.ID() == id {
			out[i] = replacement
			continue
		}
		out[i] = e
	}
	return out
}
