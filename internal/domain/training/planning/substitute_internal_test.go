package planning

import (
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// substituteAllocation の空き枠は Substitute の戻り値には現れない。先の回に
// 空きが残っても、ほとんどの入力では最良の候補が日付の早い回0に入るので
// 結果が変わらず、外部テストでは「今日に弾かれ先の回に置ける候補が残差を
// 食う」入力を組まないと捕まえられない。そこで空き枠の契約を直接固定する。
//
// 【変異】先の回の Slots を len(pinned[k]) + 1 にする → 赤。
// 【変異】回0の Slots++ を消す → 赤。
// 【変異】pinned[k] = slices.Clone(ids) を pinned[k] = ids にする → 元の割り振りが書き換わって赤。
func TestSubstituteAllocation_OnlyTodayHasAFreeSlot(t *testing.T) {
	day := training.MustDate(2026, 8, 17)
	w := weekPlan{
		alloc: AllocationRequest{Sessions: []HorizonSession{
			{Date: day, Slots: 3}, {Date: day.AddDays(2), Slots: 3}, {Date: day.AddDays(5), Slots: 3},
		}},
		allocations: [][]exercise.ExerciseID{{"a", "b"}, {"c"}, {}},
	}

	got := substituteAllocation(w, "a", []exercise.ExerciseID{"x"})

	wantPinned := [][]exercise.ExerciseID{{"b"}, {"c"}, {}}
	for k := range wantPinned {
		if !slices.Equal(got.Pinned[k], wantPinned[k]) {
			t.Errorf("Pinned[%d] = %v, want %v", k, got.Pinned[k], wantPinned[k])
		}
	}
	for k, want := range []int{2, 1, 0} {
		if got.Sessions[k].Slots != want {
			t.Errorf("Sessions[%d].Slots = %d, want %d（置いた数＋回0だけ空き1つ）", k, got.Sessions[k].Slots, want)
		}
	}
	if w.alloc.Sessions[1].Slots != 3 || !slices.Equal(w.allocations[0], []exercise.ExerciseID{"a", "b"}) {
		t.Error("元の weekPlan を書き換えている（Forecast の結果と共有している）")
	}
}
