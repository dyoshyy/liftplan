package program_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// 週の通う回数として現実的な範囲だけを通す。
//
// 上限4は「週目標を4等分しても1セッションが現実的なセット数に収まる」
// ところから来ている（D-117）。バリエーションレーンが入ってこの根拠は
// 弱くなったが、まだ動かしていない（docs/refactoring.md）。
func TestNewFrequency_Range(t *testing.T) {
	for _, n := range []int{0, -1, 5, 7, 100} {
		if _, err := program.NewFrequency(n); err == nil {
			t.Errorf("週%d回が通ってしまう", n)
		}
	}
	for _, n := range []int{1, 3, 4} {
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
