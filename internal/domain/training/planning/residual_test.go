package planning_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
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

// 窓（4週）ぶんの目標に対する不足がそのまま出ること。割らない。
//
// 暦週のころは「残りセッション数で割る」だったので、週の後半ほど1回
// あたりの量が増えた。ローリング窓には「残り」という区切りが無い。
//
// 窓ぶんの目標と比べるのは、窓が4週だから。週目標のまま比べると、4週ぶんの
// 実績を1週ぶんの目標から引くことになり、すぐ満たされて補助が出なくなる。
func TestSessionResidual_ReportsTheWholeGap(t *testing.T) {
	const weekly = 12.0
	window := weekly * planning.CoverageWindowWeeks
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: weekly})

	got := planning.SessionResidual(target, planning.StimulusCoverage{}, planning.StimulusCoverage{}, nil)
	if math.Abs(got[training.ChestMid]-window) > 1e-9 {
		t.Errorf("不足が %v。%v（週目標×窓の週数）のはず", got[training.ChestMid], window)
	}

	// 窓の中で「窓ぶんの目標 − 2」埋まっていれば残りは2。
	covered := planning.StimulusCoverage{}.Plus(
		singleRegionProfile(t, training.ChestMid, 1.0), mustSetCount(t, int(window)-2))
	got = planning.SessionResidual(target, covered, planning.StimulusCoverage{}, nil)
	if math.Abs(got[training.ChestMid]-2) > 1e-9 {
		t.Errorf("不足が %v。2のはず", got[training.ChestMid])
	}
}

// 今日すでに積んだ分を引くこと。
//
// 引かないと、軸が胸を3セット埋めた日でも補助が同じだけ上乗せする。
func TestSessionResidual_SubtractsWhatThisSessionAlreadyCovers(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})
	today := planning.StimulusCoverage{}.Plus(
		singleRegionProfile(t, training.ChestMid, 1.0), mustSetCount(t, 3))

	want := 12*planning.CoverageWindowWeeks - 3.0
	got := planning.SessionResidual(target, planning.StimulusCoverage{}, today, nil)
	if math.Abs(got[training.ChestMid]-want) > 1e-9 {
		t.Errorf("今日の分が引かれていない: %v（%vのはず）", got[training.ChestMid], want)
	}
}

func TestSessionResidual_DropsSatisfiedRegions(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 4})

	// 窓ぶんの目標ちょうどと、それを超えたとき。
	window := 4 * planning.CoverageWindowWeeks
	for _, sets := range []int{window, window + 2} {
		covered := planning.StimulusCoverage{}.Plus(
			singleRegionProfile(t, training.ChestMid, 1.0), mustSetCount(t, sets))

		if got := planning.SessionResidual(target, covered, planning.StimulusCoverage{}, nil); len(got) != 0 {
			t.Errorf("%dセット埋めたのに残差が残っている: %v", sets, got)
		}
	}
}

// 目標に無い区分はカバレッジがあっても残差に現れないこと。
func TestSessionResidual_IgnoresRegionsOutsideTheTarget(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})
	covered := planning.StimulusCoverage{}.Plus(
		singleRegionProfile(t, training.Calf, 1.0), mustSetCount(t, 5))

	got := planning.SessionResidual(target, covered, planning.StimulusCoverage{}, nil)
	if _, ok := got[training.Calf]; ok {
		t.Errorf("目標に無い区分が残差に現れている: %v", got)
	}
}

func TestSessionResidual_EdgeCases(t *testing.T) {
	var zero program.WeeklyVolumeTarget
	if got := planning.SessionResidual(zero, planning.StimulusCoverage{},
		planning.StimulusCoverage{}, nil); len(got) != 0 {
		t.Errorf("ゼロ値の目標から残差が出る: %v", got)
	}
}

func TestSessionResidual_IsQuantized(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 10})

	got := planning.SessionResidual(target, planning.StimulusCoverage{}, planning.StimulusCoverage{}, nil)[training.ChestMid]
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v（小数点以下 %d 桁）", got, n)
	}
}

// 窓を通して目標に届き、大きく超えないこと。
//
// 補助種目は3セット単位なので、天井が小さすぎると届かず、大きすぎると
// 超過する。ローリング窓では窓から落ちた分だけ不足が戻ってくるので、
// 定常状態では毎回ほぼ1回ぶんを出し続けることになる。
func TestSessionResidual_ConvergesOverTheWindow(t *testing.T) {
	const (
		weeklyTarget = 12.0
		frequency    = 4
		setsPerBout  = 3 // 補助種目のセット数は固定
	)
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: weeklyTarget})
	profile := singleRegionProfile(t, training.ChestMid, 1.0)

	// 回数に上限を置く。残差が減らなくなる変異を入れたとき、赤ではなく
	// ハングになるとテストとして役に立たない。
	const maxBouts = 100

	coverage := planning.StimulusCoverage{}
	bouts := 0
	for range frequency {
		residual := planning.SessionResidual(target, coverage, planning.StimulusCoverage{}, nil)
		for residual[training.ChestMid] > 0 {
			bouts++
			if bouts > maxBouts {
				t.Fatalf("残差が減らない: %v セット埋めても残差 %v",
					coverage.Sets(training.ChestMid), residual[training.ChestMid])
			}
			coverage = coverage.Plus(profile, mustSetCount(t, setsPerBout))
			residual = planning.SessionResidual(target, coverage, planning.StimulusCoverage{}, nil)
		}
	}

	windowTarget := weeklyTarget * planning.CoverageWindowWeeks
	got := coverage.Sets(training.ChestMid)
	if got < windowTarget {
		t.Errorf("窓ぶんの目標に届かない: %v / %v", got, windowTarget)
	}
	if got > windowTarget+setsPerBout {
		t.Errorf("窓ぶんの目標を大きく超過している: %v / %v", got, windowTarget)
	}
}

func singleRegionProfile(t *testing.T, r training.MuscleRegion, contribution float64) exercise.StimulusProfile {
	t.Helper()
	p := benchParams()
	p.Stimulus = map[training.MuscleRegion]float64{r: contribution}
	return mustExercise(t, p).Stimulus()
}

// 天井を渡すと、1回ぶんを超えて出さないこと。
//
// 分割があるときだけ効かせる。上下2分割・週5なら下半身は2〜3日なので、
// 週目標をその日数で割ったぶんが1回ぶんになる。
func TestSessionResidual_CapsWhenActiveCountIsGiven(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Quad: 24})
	twice := func(training.MuscleRegion) int { return 2 }

	// 不足24だが、週2回しか狙わないので1回ぶんは12。
	got := planning.SessionResidual(target, planning.StimulusCoverage{},
		planning.StimulusCoverage{}, twice)
	if math.Abs(got[training.Quad]-12) > 1e-9 {
		t.Errorf("天井が効いていない: %v（12のはず）", got[training.Quad])
	}

	// 天井を渡さなければ不足（窓ぶん）をそのまま出す。
	want := 24.0 * planning.CoverageWindowWeeks
	got = planning.SessionResidual(target, planning.StimulusCoverage{},
		planning.StimulusCoverage{}, nil)
	if math.Abs(got[training.Quad]-want) > 1e-9 {
		t.Errorf("天井なしで %v。%vのはず", got[training.Quad], want)
	}
}

// 今日すでに積んだ分は天井からも引くこと。
//
// 引かないと、軸が大腿四頭筋を3セット埋めた日でも補助が1回ぶんを上乗せする。
func TestSessionResidual_CapAccountsForThisSession(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Quad: 24})
	today := planning.StimulusCoverage{}.Plus(
		singleRegionProfile(t, training.Quad, 1.0), mustSetCount(t, 3))

	got := planning.SessionResidual(target, planning.StimulusCoverage{}, today,
		func(training.MuscleRegion) int { return 2 })
	if math.Abs(got[training.Quad]-9) > 1e-9 {
		t.Errorf("天井から今日の分が引かれていない: %v（12-3=9のはず）", got[training.Quad])
	}
}

// 0回しか狙われない区分は出ないこと。ゼロ除算で +Inf を出すと、
// その区分が残差の先頭に居座ってスロットを食い尽くす。
func TestSessionResidual_SkipsRegionsThatAreNeverActive(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.Quad: 12,
	})
	active := func(r training.MuscleRegion) int {
		if r == training.Quad {
			return 0
		}
		return 2
	}

	got := planning.SessionResidual(target, planning.StimulusCoverage{},
		planning.StimulusCoverage{}, active)
	if _, ok := got[training.Quad]; ok {
		t.Errorf("狙わない区分が残差に出ている: %v", got)
	}
	if _, ok := got[training.ChestMid]; !ok {
		t.Errorf("狙う区分が残差から消えている: %v", got)
	}
}
