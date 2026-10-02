package seed_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// calibrationTolerance は、実測の1セットあたり供給が k からずれてよい割合。
//
// 24構成の実測は 1.905〜1.987 に収まる（k=1.95 で -2.3〜+1.9%）。構成のばらつきに
// 余裕を持たせて5%とし、これを超えたら較正し直す合図にする。広げると、
// 目標と実態がずれていることに気づけなくなる。
const calibrationTolerance = 0.05

// 週目標の大きさ（k）が、処方どおりにこなしたときの実態と合っていること。
//
// k は「1セットが平均していくつ刺激を供給するか」。週目標は
// 「週の総セット数 × k」を配分で割ったものなので、k が実態より小さいと
// 目標は届いた状態のまま止まり（供給÷目標が 1.05〜1.10 で、画面の充足の棒が
// 100% で止まったまま動かなかった）、大きいと届かない目標を置く。
//
// k はプリセットの平均から切り離してあり（定数）、プリセットを足しても動かない。
// 代わりに、プリセットを足す・割り振りを変えるなどで実態がずれたとき、
// このテストが赤くなって、k を較正し直すことに気づける。
//
// 実測は、全身法・upper_lower・ppl・five_way × 週2〜7回（週1回は想定する利用者
// ではないので見ない）。1セットあたりの実測は、通し検証の「週あたりの平均刺激量の
// 合計 ÷ 週あたりのセット数」。
//
// 較正の手順：このテストの実測を `-v` で出して、24構成の平均を k にする。
// k を変えると割り振りが変わって実測も少し動くので、平均が k の1%以内に
// 収まるまで繰り返す。
func TestSimulation_StimulusPerSetMatchesTheTargetScale(t *testing.T) {
	type config struct {
		name string
		freq int
		cfg  simConfig
	}
	var configs []config
	for f := 2; f <= maxSimFrequency; f++ {
		configs = append(configs, config{"全身法", f, simConfig{frequency: f, weeks: 8}})
	}
	for _, p := range splitCycles(t) {
		for f := 2; f <= maxSimFrequency; f++ {
			w := splitWeeks(f, len(p.Cycle))
			configs = append(configs, config{p.Key, f, simConfig{frequency: f, weeks: w, cycle: p.Cycle}})
		}
	}

	// k は公開された週目標から逆算する（Σ目標 ÷ 週の総セット数）。定数を
	// テストにも書き写すと、較正のたびに2か所を直すことになる。
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	volume := simVolume(t)
	target, err := seed.DefaultWeeklyTarget(freq, volume)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	var sumTarget float64
	for _, r := range training.AllMuscleRegions() {
		sumTarget += target.Sets(r)
	}
	k := sumTarget / float64(freq.PerWeek()*volume.TotalSets())

	var sum float64
	for _, c := range configs {
		t.Run(fmt.Sprintf("%s・週%d回", c.name, c.freq), func(t *testing.T) {
			res := runSim(t, c.cfg)

			var supplied float64
			for _, r := range training.AllMuscleRegions() {
				supplied += res.achieved[r]
			}
			sets := 0
			for _, s := range res.sessions {
				sets += s.sets
			}
			perSet := supplied / (float64(sets) / float64(res.weeks))
			sum += perSet

			if dev := perSet/k - 1; math.Abs(dev) > calibrationTolerance {
				t.Errorf("1セットあたりの供給が %.3f。k=%.3f から %+.1f%% ずれている（許容 ±%.0f%%）",
					perSet, k, dev*100, calibrationTolerance*100)
			}
			t.Logf("1セットあたり供給 %.3f（k=%.3f、%+.1f%%）", perSet, k, (perSet/k-1)*100)
		})
	}
	t.Logf("平均 %.4f（k=%.4f）", sum/float64(len(configs)), k)
}
