package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// adjacentRepeats は予定の隣り合う回（今日と次の回、次の回と2回後…）で、
// 片方の補助の主働の区分を、もう片方の回が主働に持つ組の数（#309）。
//
// 予定の画面は日付ではなく「次の回」「2回後」と出す。本人は連日通うことが
// 多いので、隣の回に同じ部位が出れば連日に見える。
func adjacentRepeats(f []planning.PlannedSession, byID map[exercise.ExerciseID]*exercise.Exercise) int {
	primaries := func(sets []planning.PlannedSet) map[training.MuscleRegion]bool {
		out := map[training.MuscleRegion]bool{}
		for _, set := range sets {
			for _, r := range primaryOf(byID[set.ExerciseID()]) {
				out[r] = true
			}
		}
		return out
	}
	all := func(s planning.PlannedSession) map[training.MuscleRegion]bool {
		return primaries(append(append(s.Main(), s.Variation()...), s.Accessories()...))
	}
	overlaps := func(a, b map[training.MuscleRegion]bool) bool {
		for r := range a {
			if b[r] {
				return true
			}
		}
		return false
	}
	n := 0
	for k := 0; k+1 < len(f); k++ {
		if overlaps(primaries(f[k].Accessories()), all(f[k+1])) ||
			overlaps(primaries(f[k+1].Accessories()), all(f[k])) {
			n++
		}
	}
	return n
}

// 予定の隣り合う回に同じ部位が出た組の数を目で見るための出力（#309）。
// `go test -run TestSimulation_ForecastAdjacencyReport -v` で使う。
func TestSimulation_ForecastAdjacencyReport(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("-v のときだけ出力する")
	}
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(all))
	for _, e := range all {
		byID[e.ID()] = e
	}
	type setup struct {
		key   string
		cycle []program.Split
	}
	setups := []setup{{key: "full_body"}}
	for _, p := range splitCycles(t) {
		setups = append(setups, setup{key: p.Key, cycle: p.Cycle})
	}
	total, pairs := 0, 0
	for _, su := range setups {
		for f := 2; f <= maxSimFrequency; f++ {
			weeks := 4
			res := runSim(t, simConfig{frequency: f, weeks: weeks, cycle: su.cycle, forecast: true})
			n, m := 0, 0
			for _, fc := range res.forecasts {
				n += adjacentRepeats(fc, byID)
				m += len(fc) - 1
			}
			total += n
			pairs += m
			t.Logf("%-12s 週%d回 隣の回に同じ部位 %3d/%3d", su.key, f, n, m)
		}
	}
	t.Logf("合計 隣の回に同じ部位 %d/%d", total, pairs)
}
