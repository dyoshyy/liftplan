package seed

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// 配分を動かしても、週の総量が動かないこと。
//
// これが配分と総量を分けた理由そのもの。同じ表で両方を表していたころは、
// 1区分の重みを変えると週全体の量まで動いた。逆に量を絞るには21個すべてを
// 引き直す必要があり、実際そうやって引き直した跡が残っていた。
//
// 1日の種目数を可変にすると総量だけが差し替わる。そのとき配分表を巻き込ま
// ないことを、ここで先に固定しておく。
func TestDefaultWeeklyTarget_ShareDoesNotMoveTotal(t *testing.T) {
	sum := func(target program.WeeklyVolumeTarget) float64 {
		out := 0.0
		for _, r := range target.Regions() {
			out += target.Sets(r)
		}
		return out
	}

	freq, err := program.NewFrequency(baseFrequency)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}

	before, err := DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	// 配分を大きく歪める。総量に触れていなければ合計は動かない。
	restore := regionShare[training.Glute]
	regionShare[training.Glute] = restore * 3
	t.Cleanup(func() { regionShare[training.Glute] = restore })

	after, err := DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	// 許容は区分ごとの量子化（1e-6）が21区分ぶん積もる幅。結合していたころは
	// 配分を3倍にすると総量が28動いたので、これで十分に見分けられる。
	if d := sum(after) - sum(before); d > 1e-4 || d < -1e-4 {
		t.Errorf("配分を変えたら総量が動いた: %.6f → %.6f", sum(before), sum(after))
	}
	// 歪めた区分自体は動いていること。動いていなければ、この検査は
	// 何も触っていないだけで契約を守っていない。
	if after.Sets(training.Glute) <= before.Sets(training.Glute) {
		t.Errorf("配分を3倍にしたのに %s が増えていない: %.3f → %.3f",
			training.Glute, before.Sets(training.Glute), after.Sets(training.Glute))
	}
}
