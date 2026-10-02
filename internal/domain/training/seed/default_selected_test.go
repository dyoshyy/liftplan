package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// 新規の利用者に最初から使わせる種目だけで、出荷している設定がすべて成り立つこと。
//
// 使う種目を最小限にすると、補助の割り振りが選べる種目が減る。減らしすぎると、
// 週目標に届かない区分が出るだけでなく、分割（特に five_way）で空の日が出る。
// 空の日は周期が止まり、ジムに来て空のリストが出る（TestSimulation_SplitAlwaysProducesASession）。
// 「決めなくていい」ためのアプリで、最初から壊れた計画を渡さないための検収。
//
// 見る構成は、全身法・upper_lower・ppl・five_way × 週2〜7回（週1回は想定しない）。
// 受け入れ条件は2つ。
//
//   - どの区分も達成率が帯（60〜145%）に収まる（僧帽筋上部を除く。下の注）
//   - 0セットの日が1日も無い
//
// 僧帽筋上部を除くのは、シュラッグを既定で使わないと本人が決めたため。元の
// カタログで僧帽筋上部に寄与するのはシュラッグだけで、使わなければ達成率は
// 0%のまま。目標の側から外すのは別の設計になる（週目標は頻度と1回の量だけで
// 決まる）ので、ここでは受け入れ条件から除く。
//
// 種目を足したり除いたりしたら、このテストが赤くなる。
func TestDefaultSelected_PlansEveryShippedSetup(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	selected := map[exercise.ExerciseID]bool{}
	for _, id := range seed.DefaultSelected() {
		selected[id] = true
	}
	var excluded []exercise.ExerciseID
	for _, e := range all {
		if !selected[e.ID()] {
			excluded = append(excluded, e.ID())
		}
	}

	check := func(t *testing.T, cfg simConfig) {
		t.Helper()
		cfg.excluded = excluded
		res := runSim(t, cfg)
		for _, r := range training.AllMuscleRegions() {
			if r == training.TrapUpper {
				continue
			}
			if res.outOfBand(r) {
				t.Errorf("%s の達成率が範囲外: %.0f%%（目標 %.1f、実測 %.1f）",
					r, res.rate(r)*100, res.target.Sets(r), res.achieved[r])
			}
		}
		for _, s := range res.sessions {
			if s.sets == 0 {
				t.Errorf("%v に0セットの日がある（分割 %q）", s.date, s.split.Name())
			}
		}
	}

	for f := 2; f <= maxSimFrequency; f++ {
		t.Run(label("全身法", f), func(t *testing.T) {
			check(t, simConfig{frequency: f, weeks: 8})
		})
	}
	for _, p := range splitCycles(t) {
		for f := 2; f <= maxSimFrequency; f++ {
			t.Run(label(p.Key, f), func(t *testing.T) {
				check(t, simConfig{frequency: f, weeks: splitWeeks(f, len(p.Cycle)), cycle: p.Cycle})
			})
		}
	}
}

func label(name string, f int) string { return name + "・週" + string(rune('0'+f)) + "回" }
