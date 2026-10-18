package seed_test

import (
	"fmt"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// stackedRegions はその回で、補助を含む2本以上が主働として重なった区分。
//
// 軸とバリエーションだけの重なりは数えない。あれは割り振り器ではなく
// 軸とバリエーションの選定が決めている。
func stackedRegions(s simSession, byID map[exercise.ExerciseID]*exercise.Exercise) []training.MuscleRegion {
	type count struct{ all, accessories int }
	counts := map[training.MuscleRegion]*count{}
	add := func(ids []exercise.ExerciseID, accessory bool) {
		for _, id := range ids {
			for _, r := range primaryOf(byID[id]) {
				c := counts[r]
				if c == nil {
					c = &count{}
					counts[r] = c
				}
				c.all++
				if accessory {
					c.accessories++
				}
			}
		}
	}
	add(s.main, false)
	add(s.variation, false)
	add(s.accessories, true)

	var out []training.MuscleRegion
	for _, r := range training.AllMuscleRegions() {
		if c := counts[r]; c != nil && c.all >= 2 && c.accessories >= 1 {
			out = append(out, r)
		}
	}
	return out
}

func primaryOf(e *exercise.Exercise) []training.MuscleRegion {
	var out []training.MuscleRegion
	for _, r := range e.Stimulus().Regions() {
		if c, ok := e.Stimulus().Contribution(r); ok && c.Float() >= 1.0 {
			out = append(out, r)
		}
	}
	return out
}

// 同じ日に同じ部位を主に使う補助が重なった回の数を目で見るための出力（#308）。
// `go test -run TestSimulation_SameDayStackReport -v` で使う。
func TestSimulation_SameDayStackReport(t *testing.T) {
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

	totalStacked, totalSessions := 0, 0
	for _, su := range setups {
		for f := 2; f <= maxSimFrequency; f++ {
			weeks := 8
			if len(su.cycle) > 0 {
				weeks = splitWeeks(f, len(su.cycle))
			}
			res := runSim(t, simConfig{frequency: f, weeks: weeks, cycle: su.cycle})

			stacked := 0
			for _, s := range res.sessions {
				if len(stackedRegions(s, byID)) > 0 {
					stacked++
				}
			}
			totalStacked += stacked
			totalSessions += len(res.sessions)

			out := []string{}
			for _, r := range training.AllMuscleRegions() {
				if res.outOfBand(r) {
					out = append(out, fmt.Sprintf("%s=%.0f%%", r, res.rate(r)*100))
				}
			}
			t.Logf("%-12s 週%d回 重なった回 %3d/%3d  CALF %3.0f%%  範囲外 %v",
				su.key, f, stacked, len(res.sessions), res.rate(training.Calf)*100, out)
		}
	}
	t.Logf("合計 重なった回 %d/%d", totalStacked, totalSessions)
}
