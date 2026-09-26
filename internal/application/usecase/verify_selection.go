package usecase

import (
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// verifySelection は選択された種目を種目マスタと突合する。
//
// 以前はここでもう1つ、「選択した種目が週目標のどの筋区分も刺激しないなら
// 弾く」を見ていた。週目標が利用者ごとに手で入れる値だった頃は、
// 「上腕二頭筋だけを12セット狙う」設定に対して「スクワットとカーフレイズ
// だけ選ぶ」という、目標と選択がまったく噛み合わない入力が実在した。
//
// 週目標は設定（頻度 × 1回の量）から導く値になり（D-139、#176）、
// 導いた先は常に全21区分が正の値を持つ（seed.regionShare が全区分を
// カバーする配分表であるため）。種目は必ず1区分以上に寄与する
// （exercise.NewStimulusProfile が空を弾く）ので、選択が1種目でもあれば
// 必ずどこかの区分を刺激し、この分岐に到達する経路が無くなった。
// 到達しない検査を残すと、いつか読む人が「まだ効く分岐」と誤解する。
func verifySelection(pool []*exercise.Exercise, prog *program.Program) error {
	known := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		known[e.ID()] = e
	}

	for _, id := range prog.SelectedExercises() {
		if _, ok := known[id]; !ok {
			return fmt.Errorf("%w: %w: %s", apperror.ErrInvalidInput, exercise.ErrExerciseNotFound, id)
		}
	}
	return nil
}
