package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// プリセットが実シードで成立すること。
//
// 分割を選んだのに補助の候補が1つも無い日があると、その日は軸だけで
// 終わる。区分の割り当てを変えたときに気づけるよう、候補数を固定する。
func TestSplitPresets_AreUsableWithTheSeed(t *testing.T) {
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("プリセットが不正: %v", err)
	}
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	// 軸の候補がゼロになる日があるのは 5分割 の肩と腕だけ。
	// BIG3＋チンニングに肩・腕を主働にする種目が無いため。
	declared := map[exercise.ExerciseID]bool{
		"bench": true, "squat": true, "deadlift": true, "pull_up": true,
	}
	wantNoAxis := map[string]bool{"肩": true, "腕": true}

	for _, p := range presets {
		if p.Key == "" || p.Name == "" {
			t.Errorf("キーか名前が空: %+v", p)
		}
		if len(p.Cycle) == 0 {
			t.Errorf("%s の周期が空", p.Key)
		}

		for _, s := range p.Cycle {
			axes, candidates := 0, 0
			for _, e := range pool {
				primary := false
				for _, r := range e.Stimulus().Regions() {
					c, ok := e.Stimulus().Contribution(r)
					if ok && c.Float() >= 1.0 && s.Includes(r) {
						primary = true
					}
				}
				if !primary {
					continue
				}
				candidates++
				if declared[e.ID()] {
					axes++
				}
			}

			// 補助の候補は最低3種目。1セッション最大8枠なので、
			// 3を割ると9セットを下回って通し検証の下限に触れる。
			if candidates < 3 {
				t.Errorf("%s の %s: 補助候補が %d 種目しかない", p.Key, s.Name(), candidates)
			}
			if axes == 0 && !wantNoAxis[s.Name()] {
				t.Errorf("%s の %s: 軸の候補が無い。想定外", p.Key, s.Name())
			}
			if axes > 0 && wantNoAxis[s.Name()] {
				t.Errorf("%s の %s: 軸の候補ができた。想定と違う（良い変化なら期待値を直すこと）",
					p.Key, s.Name())
			}
		}
	}
}
