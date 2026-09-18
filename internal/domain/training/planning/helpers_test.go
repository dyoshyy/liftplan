package planning_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// このパッケージのテストだけで使う小さなヘルパー。
//
// 分割前は1つのテストパッケージに置いていた。共有用のパッケージを
// 立てるより、数行の重複のほうが安い。

// baseDay は履歴の起点。8月1日を1日目とする。
func baseDay(day int) training.Date {
	return training.MustDate(2026, time.August, 1).AddDays(day - 1)
}

// decimalPlaces は文字列表現の小数点以下の桁数。
func decimalPlaces(s string) int {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return 0
	}
	return len(s) - dot - 1
}

func mustWeight(t *testing.T, kg float64) training.Weight {
	t.Helper()
	w, err := training.NewWeight(kg)
	if err != nil {
		t.Fatalf("NewWeight(%v): %v", kg, err)
	}
	return w
}

func benchParams() exercise.ExerciseParams {
	return exercise.ExerciseParams{
		ID:   "bench",
		Name: "ベンチプレス",
		Stimulus: map[training.MuscleRegion]float64{
			training.ChestMid:       1.0,
			training.TricepsLateral: 0.5,
		},
		IncrementKg: 2.5,
	}
}

func mustExercise(t *testing.T, p exercise.ExerciseParams) *exercise.Exercise {
	t.Helper()
	e, err := exercise.NewExercise(p)
	if err != nil {
		t.Fatalf("NewExercise(%s): %v", p.ID, err)
	}
	return e
}

func mkLog(t *testing.T, id string, day int, exerciseID string, kg float64, reps, rir int) *setlog.SetLog {
	t.Helper()
	return mkLogOn(t, id, baseDay(day), exerciseID, kg, reps, rir)
}

// mkLogOn は日付を直接指定する。月をまたぐ長期の履歴を組むときに使う。
func mkLogOn(t *testing.T, id string, date training.Date, exerciseID string, kg float64, reps, rir int) *setlog.SetLog {
	t.Helper()
	s, err := setlog.NewSetLog(setlog.SetLogParams{
		ID:          id,
		PerformedOn: date,
		ExerciseID:  exerciseID,
		WeightKg:    kg,
		Reps:        reps,
		RIR:         rir,
	})
	if err != nil {
		t.Fatalf("NewSetLog(%s): %v", id, err)
	}
	return s
}

func mustSetCount(t *testing.T, v int) training.SetCount {
	t.Helper()
	s, err := training.NewSetCount(v)
	if err != nil {
		t.Fatalf("NewSetCount(%d): %v", v, err)
	}
	return s
}

func mustTarget(t *testing.T, m map[training.MuscleRegion]float64) program.WeeklyVolumeTarget {
	t.Helper()
	target, err := program.NewWeeklyVolumeTarget(m)
	if err != nil {
		t.Fatalf("NewWeeklyVolumeTarget: %v", err)
	}
	return target
}

func big3() []exercise.ExerciseID {
	return []exercise.ExerciseID{"bench", "squat", "deadlift"}
}

// assertGolden は永続化される列挙値を集合として突き合わせる。
// taxonomy 側の同名ヘルパーと同じ実装。
func assertGolden(t *testing.T, name string, got, want []string) {
	t.Helper()

	inWant := map[string]bool{}
	for _, v := range want {
		inWant[v] = true
	}
	inGot := map[string]bool{}
	for _, v := range got {
		inGot[v] = true
	}

	for _, v := range want {
		if !inGot[v] {
			t.Errorf("%s: 永続化される値 %q が消えている。既存データが読めなくなる", name, v)
		}
	}
	for _, v := range got {
		if !inWant[v] {
			t.Errorf("%s: 未知の値 %q が増えている。意図した追加ならゴールデンに追記すること", name, v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%s: 件数が違う: got %d, want %d", name, len(got), len(want))
	}
}

func mustOneRepMax(t *testing.T, kg float64) training.OneRepMax {
	t.Helper()
	o, err := training.NewOneRepMax(kg)
	if err != nil {
		t.Fatalf("NewOneRepMax(%v): %v", kg, err)
	}
	return o
}

func mustIncrement(t *testing.T, kg float64) training.Increment {
	t.Helper()
	i, err := training.NewIncrement(kg)
	if err != nil {
		t.Fatalf("NewIncrement(%v): %v", kg, err)
	}
	return i
}

func mustFrequency(t *testing.T, n int) program.Frequency {
	t.Helper()
	f, err := program.NewFrequency(n)
	if err != nil {
		t.Fatalf("NewFrequency(%d): %v", n, err)
	}
	return f
}
