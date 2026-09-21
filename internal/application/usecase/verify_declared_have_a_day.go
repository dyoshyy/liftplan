package usecase

import (
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// verifyDeclaredHaveADay は、どの分割にも出られない宣言種目が無いことを確かめる。
//
// 出られるかどうかの判断は planning が持つ。ここに写しを置くと、計画の側と
// 閾値がずれたときに「保存は通るのに軸に出ない」が黙って起きる。
//
// pool に無い宣言種目は planning が返さないので、そのまま通る。選択との
// 突合は ConfigureProgram が済ませていて、ここで見つからないのは保存済みの
// 不整合であり、分割の問題ではない。
func verifyDeclaredHaveADay(pool []*exercise.Exercise, prog *program.Program) error {
	without := planning.DeclaredWithoutADay(pool, prog)
	if len(without) == 0 {
		return nil
	}
	// 1件ずつ直してもらう。宣言の順なので、同じ入力なら同じ種目を指す。
	return fmt.Errorf(
		"%w: 伸ばしたい種目 %q が出られる日が無い。主働の筋区分をどれかの分割に入れること",
		apperror.ErrInvalidInput, without[0])
}
