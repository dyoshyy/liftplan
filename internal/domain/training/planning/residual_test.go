package planning_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/program"
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

func TestSessionResidual_SplitsRemainderAcrossRemainingSessions(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})

	// 週の頭、まだ何もしていない。3セッション残っているので4セット狙う。
	got := planning.SessionResidual(target, planning.StimulusCoverage{}, 3)
	if math.Abs(got[training.ChestMid]-4) > 1e-9 {
		t.Errorf("残り3セッションでの配分が誤り: %v", got[training.ChestMid])
	}

	// 最終セッションでは残り全部を狙う。
	got = planning.SessionResidual(target, planning.StimulusCoverage{}, 1)
	if math.Abs(got[training.ChestMid]-12) > 1e-9 {
		t.Errorf("最終セッションでの配分が誤り: %v", got[training.ChestMid])
	}
}

// 過不足が翌セッションへ繰り越されること。
//
// 週目標を頻度で割った固定値を毎回使うと繰り越しが起きない。
// 補助種目は3セット固定なので、目標の小さい区分は毎回超過し、
// 目標の大きい区分は毎回埋まらないまま、誤差が次に伝わらない。
func TestSessionResidual_CarriesOverShortfall(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})

	// 1本目で目標4に対し1セットしか埋まらなかった。
	after := planning.StimulusCoverage{}.Plus(
		singleRegionProfile(t, training.ChestMid, 1.0), mustSetCount(t, 1))

	// 残り2セッションで 11 セットを分け合うので、1回あたり 5.5。
	got := planning.SessionResidual(target, after, 2)
	if math.Abs(got[training.ChestMid]-5.5) > 1e-9 {
		t.Errorf("不足が繰り越されていない: %v", got[training.ChestMid])
	}
}

func TestSessionResidual_CarriesOverExcess(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})

	// 1本目で目標4に対し8セット埋まった。
	after := planning.StimulusCoverage{}.Plus(
		singleRegionProfile(t, training.ChestMid, 1.0), mustSetCount(t, 8))

	// 残り2セッションで 4 セットなので、1回あたり 2。
	got := planning.SessionResidual(target, after, 2)
	if math.Abs(got[training.ChestMid]-2) > 1e-9 {
		t.Errorf("超過が繰り越されていない: %v", got[training.ChestMid])
	}
}

func TestSessionResidual_DropsSatisfiedRegions(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 4})

	for _, sets := range []int{4, 6} {
		covered := planning.StimulusCoverage{}.Plus(
			singleRegionProfile(t, training.ChestMid, 1.0), mustSetCount(t, sets))

		if got := planning.SessionResidual(target, covered, 2); len(got) != 0 {
			t.Errorf("%dセット埋めたのに残差が残っている: %v", sets, got)
		}
	}
}

// 目標に無い区分はカバレッジがあっても残差に現れないこと。
func TestSessionResidual_IgnoresRegionsOutsideTheTarget(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})
	covered := planning.StimulusCoverage{}.Plus(
		singleRegionProfile(t, training.Calf, 1.0), mustSetCount(t, 5))

	got := planning.SessionResidual(target, covered, 3)
	if _, ok := got[training.Calf]; ok {
		t.Errorf("目標に無い区分が残差に現れている: %v", got)
	}
}

func TestSessionResidual_EdgeCases(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})

	for _, remaining := range []int{0, -1} {
		if got := planning.SessionResidual(target, planning.StimulusCoverage{}, remaining); len(got) != 0 {
			t.Errorf("残りセッション %d で残差が出る: %v", remaining, got)
		}
	}

	var zero program.WeeklyVolumeTarget
	if got := planning.SessionResidual(zero, planning.StimulusCoverage{}, 3); len(got) != 0 {
		t.Errorf("ゼロ値の目標から残差が出る: %v", got)
	}
}

func TestSessionResidual_IsQuantized(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 10})

	got := planning.SessionResidual(target, planning.StimulusCoverage{}, 3)[training.ChestMid]
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v（小数点以下 %d 桁）", got, n)
	}
}

// 週を通して目標が達成できること。
//
// 毎セッション「残りを残りセッション数で割る」ので、補助種目のセット数が
// 固定でも、超過・不足が次に繰り越されて週の終わりには目標に収束する。
func TestSessionResidual_ConvergesOverTheWeek(t *testing.T) {
	const (
		weeklyTarget = 12.0
		frequency    = 4
		setsPerBout  = 3 // 補助種目のセット数は固定
	)
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: weeklyTarget})
	profile := singleRegionProfile(t, training.ChestMid, 1.0)

	coverage := planning.StimulusCoverage{}
	for session := range frequency {
		remaining := frequency - session
		residual := planning.SessionResidual(target, coverage, remaining)

		// 残差がある限り、3セット単位で埋める（実際の補助種目の挙動）。
		for residual[training.ChestMid] > 0 {
			coverage = coverage.Plus(profile, mustSetCount(t, setsPerBout))
			residual = planning.SessionResidual(target, coverage, remaining)
		}
	}

	got := coverage.Sets(training.ChestMid)
	if got < weeklyTarget {
		t.Errorf("週目標に届かない: %v / %v", got, weeklyTarget)
	}
	// 3セット単位なので多少の超過は避けられないが、1単位以内に収まること。
	if got > weeklyTarget+setsPerBout {
		t.Errorf("週目標を大きく超過している: %v / %v", got, weeklyTarget)
	}
}

// singleRegionProfile は1区分だけに寄与する種目の刺激分布を返す。
func singleRegionProfile(t *testing.T, r training.MuscleRegion, contribution float64) exercise.StimulusProfile {
	t.Helper()
	p := benchParams()
	p.Stimulus = map[training.MuscleRegion]float64{r: contribution}
	return mustExercise(t, p).Stimulus()
}
