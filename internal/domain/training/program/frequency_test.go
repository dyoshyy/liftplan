package program_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// 週の通う回数として現実的な範囲だけを通す。
//
// 上限7は「1週間は7日しかない」以上の意味を持たない。以前の4は
// 「毎セッションで BIG3 すべてにスロットを割り当てる設計だから」が
// 根拠だったが、D-117 で軸が1セッション1種目になった時点で消えていた。
func TestNewFrequency_Range(t *testing.T) {
	for _, n := range []int{0, -1, 8, 100} {
		if _, err := program.NewFrequency(n); err == nil {
			t.Errorf("週%d回が通ってしまう", n)
		}
	}
	for _, n := range []int{1, 3, 4, 5, 7} {
		if f, err := program.NewFrequency(n); err != nil || f.PerWeek() != n {
			t.Errorf("週%d回が弾かれた: %v", n, err)
		}
	}

	var zero program.Frequency
	if !zero.IsZero() {
		t.Error("ゼロ値が IsZero でない")
	}
	f, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("NewFrequency: %v", err)
	}
	if f.IsZero() {
		t.Error("有効な頻度が IsZero になっている")
	}
}
