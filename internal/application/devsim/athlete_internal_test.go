package devsim

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// シードの全種目に初日の実力の目安があること。
//
// 無い種目は一律 50kg で始まる。サイドレイズのような小さい種目が 50kg から
// 始まると、模擬ユーザーの記録が現実から離れ、重量の推移が読めなくなる。
// DefaultOneRepMax は無い種目にも値を返すので、外から見ても区別がつかない。
// 表を直接見る。
func TestDefaultOneRepMax_CoversEverySeedExercise(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range all {
		if _, ok := defaultOneRepMax[e.ID()]; !ok {
			t.Errorf("%s（%s）の初日の実力が表に無い", e.ID(), e.Name())
		}
	}
}
