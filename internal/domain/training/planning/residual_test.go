package planning_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
)

func TestStimulusCoverage_Plus(t *testing.T) {
	bench := mustExercise(t, benchParams())
	coverage := planning.StimulusCoverage{}.Plus(bench.Stimulus(), mustSetCount(t, 4))

	if got := coverage.Sets(training.ChestMid); math.Abs(got-4) > 1e-9 {
		t.Errorf("大胸筋中部のカバレッジが誤り: %v", got)
	}
	if got := coverage.Sets(training.TricepsLateral); math.Abs(got-2) > 1e-9 {
		t.Errorf("三頭のカバレッジが誤り: %v", got)
	}
	if got := coverage.Sets(training.Calf); got != 0 {
		t.Errorf("寄与しない区分が埋まっている: %v", got)
	}
}

func TestStimulusCoverage_Accumulates(t *testing.T) {
	bench := mustExercise(t, benchParams())
	coverage := planning.StimulusCoverage{}.
		Plus(bench.Stimulus(), mustSetCount(t, 4)).
		Plus(bench.Stimulus(), mustSetCount(t, 3))

	if got := coverage.Sets(training.ChestMid); math.Abs(got-7) > 1e-9 {
		t.Errorf("積み上がっていない: %v", got)
	}
}

// ゼロ値に対する操作が panic しないこと。
// 破壊的な Add だと nil マップへの代入で落ちる。
func TestStimulusCoverage_ZeroValueIsUsable(t *testing.T) {
	var zero planning.StimulusCoverage
	if !zero.IsEmpty() {
		t.Error("ゼロ値が空でない")
	}
	if zero.Sets(training.ChestMid) != 0 {
		t.Error("ゼロ値から値が出てくる")
	}

	bench := mustExercise(t, benchParams())
	got := zero.Plus(bench.Stimulus(), mustSetCount(t, 3))
	if got.IsEmpty() {
		t.Error("ゼロ値に加算しても空のまま")
	}
	if !zero.IsEmpty() {
		t.Error("元のカバレッジが書き換わっている")
	}
}

// 端数が残ると「わずかに残っている」区分が生まれ、選択が不安定になる。
func TestStimulusCoverage_IsQuantized(t *testing.T) {
	p := benchParams()
	p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 0.3}
	e := mustExercise(t, p)

	coverage := planning.StimulusCoverage{}
	for range 7 {
		coverage = coverage.Plus(e.Stimulus(), mustSetCount(t, 3))
	}

	got := coverage.Sets(training.ChestMid)
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v（小数点以下 %d 桁）", got, n)
	}
}
