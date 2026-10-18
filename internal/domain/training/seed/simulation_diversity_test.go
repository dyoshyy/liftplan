package seed_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// 補助が同じ部位の中の1種目に偏っていないかを目で見るための出力（#223）。
// `go test -run TestSimulation_DiversityReport -v` で使う。
//
// 主働の区分ごとに、補助として選ばれた回数のうち最多の1種目が占める割合を出す。
// 全カタログ（Hammer Strength を含む）で回す。偏りは同じ部位の候補が多いほど
// 目立つため。
func TestSimulation_DiversityReport(t *testing.T) {
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

	sumShare, nShare, sumUsed := 0.0, 0, 0
	for _, su := range setups {
		for _, f := range []int{3, 5} {
			weeks := 12
			res := runSim(t, simConfig{frequency: f, weeks: weeks, cycle: su.cycle, allExercises: true})

			picks := map[exercise.ExerciseID]int{}
			for _, s := range res.sessions {
				for _, id := range s.accessories {
					picks[id]++
				}
			}
			byRegion := map[training.MuscleRegion]map[exercise.ExerciseID]int{}
			for id, n := range picks {
				for _, r := range primaryOf(byID[id]) {
					if byRegion[r] == nil {
						byRegion[r] = map[exercise.ExerciseID]int{}
					}
					byRegion[r][id] = n
				}
			}
			share, ns := 0.0, 0
			var worst []string
			for _, r := range training.AllMuscleRegions() {
				m := byRegion[r]
				total, top := 0, 0
				for _, n := range m {
					total += n
					top = max(top, n)
				}
				if total < 4 || len(candidatesFor(all, r)) < 2 {
					continue
				}
				share += float64(top) / float64(total)
				ns++
				if float64(top)/float64(total) >= 0.8 && len(candidatesFor(all, r)) >= 3 {
					worst = append(worst, fmt.Sprintf("%s(%d/%d)", r, top, total))
				}
			}
			sumShare += share
			nShare += ns
			sumUsed += len(picks)

			out := []string{}
			for _, r := range training.AllMuscleRegions() {
				if res.outOfBand(r) {
					out = append(out, fmt.Sprintf("%s=%.0f%%", r, res.rate(r)*100))
				}
			}
			t.Logf("%-12s 週%d回 使った補助 %2d種目  最多の割合 平均%3.0f%%  偏り %v  範囲外 %v",
				su.key, f, len(picks), 100*share/float64(max(ns, 1)), worst, out)
			var unused []exercise.ExerciseID
			for _, e := range all {
				if picks[e.ID()] == 0 && res.picked[e.ID()] == 0 {
					unused = append(unused, e.ID())
				}
			}
			t.Logf("    未使用 %d %v", len(unused), unused)
			if su.key == "full_body" && f == 5 {
				for _, r := range []training.MuscleRegion{training.Lat, training.TrapMid, training.ChestMid, training.Quad} {
					t.Logf("    %s %v", r, sorted(byRegion[r]))
				}
			}
		}
	}
	t.Logf("合計 使った補助 %d  最多の割合 平均 %.0f%%", sumUsed, 100*sumShare/float64(nShare))
}

func candidatesFor(all []*exercise.Exercise, r training.MuscleRegion) []exercise.ExerciseID {
	var out []exercise.ExerciseID
	for _, e := range all {
		for _, p := range primaryOf(e) {
			if p == r {
				out = append(out, e.ID())
			}
		}
	}
	return out
}

func sorted(m map[exercise.ExerciseID]int) []string {
	out := make([]string, 0, len(m))
	for id, n := range m {
		out = append(out, fmt.Sprintf("%s=%d", id, n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
