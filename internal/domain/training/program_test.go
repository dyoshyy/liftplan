package training_test

import (
	"math"
	"sort"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mustTarget(t *testing.T, m map[training.MuscleRegion]float64) training.WeeklyVolumeTarget {
	t.Helper()
	target, err := training.NewWeeklyVolumeTarget(m)
	if err != nil {
		t.Fatalf("NewWeeklyVolumeTarget: %v", err)
	}
	return target
}

func simpleTarget(t *testing.T) training.WeeklyVolumeTarget {
	t.Helper()
	return mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid:   12,
		training.ChestUpper: 9,
	})
}

func mustSetCount(t *testing.T, n int) training.SetCount {
	t.Helper()
	s, err := training.NewSetCount(n)
	if err != nil {
		t.Fatalf("NewSetCount(%d): %v", n, err)
	}
	return s
}

func TestNewWeeklyVolumeTarget_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   map[training.MuscleRegion]float64
	}{
		{"空", nil},
		{"0セット", map[training.MuscleRegion]float64{training.ChestMid: 0}},
		{"負のセット数", map[training.MuscleRegion]float64{training.ChestMid: -5}},
		{"下限未満", map[training.MuscleRegion]float64{training.ChestMid: 0.4}},
		{"上限超", map[training.MuscleRegion]float64{training.ChestMid: 41}},
		{"NaN", map[training.MuscleRegion]float64{training.ChestMid: math.NaN()}},
		{"無限大", map[training.MuscleRegion]float64{training.ChestMid: math.Inf(1)}},
		{"未知の筋区分", map[training.MuscleRegion]float64{"NOPE": 10}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got, err := training.NewWeeklyVolumeTarget(c.in); err == nil {
				t.Errorf("不正な週目標が通ってしまう: %+v", got)
			}
		})
	}
}

func TestNewWeeklyVolumeTarget_BoundaryConstants(t *testing.T) {
	for _, v := range []float64{0.5, 40} {
		if _, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
			training.ChestMid: v,
		}); err != nil {
			t.Errorf("境界ちょうど %v が弾かれた: %v", v, err)
		}
	}
	for _, v := range []float64{0.499999, 40.000001} {
		if _, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
			training.ChestMid: v,
		}); err == nil {
			t.Errorf("境界をわずかに外れる %v が通ってしまう", v)
		}
	}
}

func TestNewWeeklyVolumeTarget_ErrorIdentifiesTheRegion(t *testing.T) {
	// 21区分のシードのうちどれが不正か分からないと直せない。
	_, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid: 100,
	})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !contains(err.Error(), "CHEST_MID") {
		t.Errorf("エラーメッセージに筋区分が含まれない: %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestWeeklyVolumeTarget_RegionsAreSorted(t *testing.T) {
	regions := simpleTarget(t).Regions()
	if !sort.SliceIsSorted(regions, func(i, j int) bool { return regions[i] < regions[j] }) {
		t.Errorf("ソートされていない: %v", regions)
	}
}

func TestWeeklyVolumeTarget_IsImmutableAgainstInputMutation(t *testing.T) {
	input := map[training.MuscleRegion]float64{training.ChestMid: 12}
	target := mustTarget(t, input)

	input[training.ChestMid] = 1
	input[training.Calf] = 8

	if got := target.Sets(training.ChestMid); math.Abs(got-12) > 1e-9 {
		t.Errorf("入力マップの書き換えが波及している: %v", got)
	}
	if got := target.Sets(training.Calf); got != 0 {
		t.Errorf("入力マップへの追加が波及している: %v", got)
	}
}

// 未設定の区分は0を返す。これは「狙わない」という意味でエラーではない。
func TestWeeklyVolumeTarget_UnknownRegionIsZero(t *testing.T) {
	if got := simpleTarget(t).Sets(training.Calf); got != 0 {
		t.Errorf("未設定の区分が0でない: %v", got)
	}
}

func TestWeeklyVolumeTarget_PerSession(t *testing.T) {
	per := simpleTarget(t).PerSession(mustFrequency(t, 3))

	if got := per.Sets(training.ChestMid); math.Abs(got-4) > 1e-9 {
		t.Errorf("1セッションあたりの目標が誤り: %v", got)
	}
	if got := per.Sets(training.ChestUpper); math.Abs(got-3) > 1e-9 {
		t.Errorf("1セッションあたりの目標が誤り: %v", got)
	}
}

// 割り切れない値でも端数が残らないこと。残差の比較に使われる。
func TestWeeklyVolumeTarget_PerSessionIsQuantized(t *testing.T) {
	per := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 10}).
		PerSession(mustFrequency(t, 3))

	got := per.Sets(training.ChestMid)
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v（小数点以下 %d 桁）", got, n)
	}
}

// 割った結果が下限を下回る区分は落とす。
// 週0.5セットを頻度4で割ると0.125になり、補助種目1つ（3セット）で
// 大幅に超過する。そういう区分はこのセッションでは狙わない。
func TestWeeklyVolumeTarget_PerSessionDropsNegligibleRegions(t *testing.T) {
	per := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12,
		training.Calf:     0.5,
	}).PerSession(mustFrequency(t, 4))

	if got := per.Sets(training.ChestMid); math.Abs(got-3) > 1e-9 {
		t.Errorf("主要な区分が落ちている: %v", got)
	}
	if got := per.Sets(training.Calf); got != 0 {
		t.Errorf("無視できる区分が残っている: %v", got)
	}
}

func TestWeeklyVolumeTarget_PerSessionWithZeroFrequency(t *testing.T) {
	var zero training.Frequency
	if got := simpleTarget(t).PerSession(zero); !got.IsEmpty() {
		t.Errorf("ゼロ値の頻度で目標が返る: %v", got.Regions())
	}
}

func TestWeeklyVolumeTarget_ZeroValueIsEmpty(t *testing.T) {
	var zero training.WeeklyVolumeTarget
	if !zero.IsEmpty() {
		t.Error("ゼロ値が空でない")
	}
	if len(zero.Regions()) != 0 || zero.Sets(training.ChestMid) != 0 {
		t.Error("ゼロ値から値が出てくる")
	}
}

func TestNewProgram(t *testing.T) {
	p, err := training.NewProgram(mustFrequency(t, 3), simpleTarget(t),
		[]training.ExerciseID{"bench", "squat"})
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	if !p.Includes("bench") || !p.Includes("squat") {
		t.Error("選択した種目が含まれていない")
	}
	if p.Includes("deadlift") {
		t.Error("選択していない種目が含まれている")
	}
	if p.Frequency().PerWeek() != 3 {
		t.Errorf("頻度が誤り: %d", p.Frequency().PerWeek())
	}
}

func TestNewProgram_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name     string
		freq     training.Frequency
		target   training.WeeklyVolumeTarget
		selected []training.ExerciseID
	}{
		{"頻度が未設定", training.Frequency{}, simpleTarget(t), []training.ExerciseID{"bench"}},
		{"週目標が未設定", mustFrequency(t, 3), training.WeeklyVolumeTarget{}, []training.ExerciseID{"bench"}},
		{"種目が空", mustFrequency(t, 3), simpleTarget(t), nil},
		{"空の種目ID", mustFrequency(t, 3), simpleTarget(t), []training.ExerciseID{"bench", ""}},
		{"種目が重複", mustFrequency(t, 3), simpleTarget(t), []training.ExerciseID{"bench", "bench"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := training.NewProgram(c.freq, c.target, c.selected)
			if err == nil {
				t.Fatalf("不正なプログラムが通ってしまう: %+v", got)
			}
			if got != nil {
				t.Errorf("失敗時に nil でない値が返る: %+v", got)
			}
		})
	}
}

func TestProgram_SelectedExercisesIsACopy(t *testing.T) {
	p, err := training.NewProgram(mustFrequency(t, 3), simpleTarget(t), []training.ExerciseID{"bench"})
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}

	got := p.SelectedExercises()
	got[0] = "tampered"
	if !p.Includes("bench") || p.Includes("tampered") {
		t.Error("返り値の書き換えが集約に波及している")
	}
}

func TestProgram_IsImmutableAgainstInputMutation(t *testing.T) {
	input := []training.ExerciseID{"bench", "squat"}
	p, err := training.NewProgram(mustFrequency(t, 3), simpleTarget(t), input)
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}

	input[0] = "tampered"
	if !p.Includes("bench") || p.Includes("tampered") {
		t.Error("入力スライスの書き換えが集約に波及している")
	}
}

func TestStimulusCoverage_Add(t *testing.T) {
	bench := mustExercise(t, benchParams())
	coverage := training.StimulusCoverage{}
	coverage.Add(bench.Stimulus(), mustSetCount(t, 4))

	if got := coverage[training.ChestMid]; math.Abs(got-4) > 1e-9 {
		t.Errorf("大胸筋中部のカバレッジが誤り: %v", got)
	}
	if got := coverage[training.TricepsLateral]; math.Abs(got-2) > 1e-9 {
		t.Errorf("三頭のカバレッジが誤り: %v", got)
	}
}

func TestStimulusCoverage_Accumulates(t *testing.T) {
	bench := mustExercise(t, benchParams())
	coverage := training.StimulusCoverage{}
	coverage.Add(bench.Stimulus(), mustSetCount(t, 4))
	coverage.Add(bench.Stimulus(), mustSetCount(t, 3))

	if got := coverage[training.ChestMid]; math.Abs(got-7) > 1e-9 {
		t.Errorf("積み上がっていない: %v", got)
	}
}

// 端数が残ると、残差の比較で「わずかに残っている」区分が生まれ、
// 補助種目の選択が不安定になる。
func TestStimulusCoverage_IsQuantized(t *testing.T) {
	p := benchParams()
	p.Stimulus = map[training.MuscleRegion]float64{training.ChestMid: 0.3}
	e := mustExercise(t, p)

	coverage := training.StimulusCoverage{}
	for range 7 {
		coverage.Add(e.Stimulus(), mustSetCount(t, 3))
	}

	got := coverage[training.ChestMid]
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v（小数点以下 %d 桁）", got, n)
	}
}

func TestResidual_SubtractsCoverage(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid:   12,
		training.ChestUpper: 8,
	})
	coverage := training.StimulusCoverage{training.ChestMid: 4}

	got := training.Residual(target, coverage)
	if math.Abs(got[training.ChestMid]-8) > 1e-9 {
		t.Errorf("残差が誤り: %v", got[training.ChestMid])
	}
	if math.Abs(got[training.ChestUpper]-8) > 1e-9 {
		t.Errorf("カバーされていない区分の残差が誤り: %v", got[training.ChestUpper])
	}
}

func TestResidual_DropsSatisfiedRegions(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 4})

	for _, covered := range []float64{4, 6} {
		got := training.Residual(target, training.StimulusCoverage{training.ChestMid: covered})
		if len(got) != 0 {
			t.Errorf("カバレッジ %v で残差が残っている: %v", covered, got)
		}
	}
}

// 目標に無い区分はカバレッジがあっても残差に現れないこと。
// 現れると、目標を設定していない区分に補助種目が割り当てられる。
func TestResidual_IgnoresRegionsOutsideTheTarget(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 12})
	coverage := training.StimulusCoverage{training.Calf: 5}

	got := training.Residual(target, coverage)
	if _, ok := got[training.Calf]; ok {
		t.Errorf("目標に無い区分が残差に現れている: %v", got)
	}
}

func TestResidual_IsQuantized(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 10})
	coverage := training.StimulusCoverage{training.ChestMid: 3.333333}

	got := training.Residual(target, coverage)[training.ChestMid]
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v（小数点以下 %d 桁）", got, n)
	}
}

func TestResidual_EmptyInputs(t *testing.T) {
	var zero training.WeeklyVolumeTarget
	if got := training.Residual(zero, training.StimulusCoverage{}); len(got) != 0 {
		t.Errorf("ゼロ値の目標から残差が出る: %v", got)
	}

	target := simpleTarget(t)
	got := training.Residual(target, nil)
	if len(got) != len(target.Regions()) {
		t.Errorf("カバレッジ無しで全区分が残差にならない: %v", got)
	}
}
