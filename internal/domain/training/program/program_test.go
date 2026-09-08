package program_test

import (
	"math"
	"sort"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/program"
)

func mustTarget(t *testing.T, m map[training.MuscleRegion]float64) program.WeeklyVolumeTarget {
	t.Helper()
	target, err := program.NewWeeklyVolumeTarget(m)
	if err != nil {
		t.Fatalf("NewWeeklyVolumeTarget: %v", err)
	}
	return target
}

// big3 はテストで「伸ばしたい種目」に使う既定の3つ。
//
// 移行前の Kind == KindMain と同じ顔ぶれにしてある。PR-1 は挙動を
// 変えない回なので、ここが変わるとテストの数字が動く理由が増える。
func big3() []exercise.ExerciseID {
	return []exercise.ExerciseID{"bench", "squat", "deadlift"}
}

func simpleTarget(t *testing.T) program.WeeklyVolumeTarget {
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
			if got, err := program.NewWeeklyVolumeTarget(c.in); err == nil {
				t.Errorf("不正な週目標が通ってしまう: %+v", got)
			}
		})
	}
}

func TestNewWeeklyVolumeTarget_BoundaryConstants(t *testing.T) {
	for _, v := range []float64{0.5, 40} {
		if _, err := program.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
			training.ChestMid: v,
		}); err != nil {
			t.Errorf("境界ちょうど %v が弾かれた: %v", v, err)
		}
	}
	for _, v := range []float64{0.499999, 40.000001} {
		if _, err := program.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
			training.ChestMid: v,
		}); err == nil {
			t.Errorf("境界をわずかに外れる %v が通ってしまう", v)
		}
	}
}

func TestNewWeeklyVolumeTarget_ErrorIdentifiesTheRegion(t *testing.T) {
	// 21区分のシードのうちどれが不正か分からないと直せない。
	_, err := program.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
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
	// 2区分だとマップの反復順が偶然合うことがあるので、多めに入れる。
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Quad: 16, training.Abs: 8, training.ChestMid: 12,
		training.Lat: 12, training.Calf: 8, training.Biceps: 10,
	})
	regions := target.Regions()
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

func TestWeeklyVolumeTarget_ZeroValueIsEmpty(t *testing.T) {
	var zero program.WeeklyVolumeTarget
	if !zero.IsEmpty() {
		t.Error("ゼロ値が空でない")
	}
	if len(zero.Regions()) != 0 || zero.Sets(training.ChestMid) != 0 {
		t.Error("ゼロ値から値が出てくる")
	}
}

func TestNewProgram(t *testing.T) {
	p, err := program.NewProgram(mustFrequency(t, 3), simpleTarget(t),
		[]exercise.ExerciseID{"bench", "squat"}, []exercise.ExerciseID{"bench"})
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
		freq     program.Frequency
		target   program.WeeklyVolumeTarget
		selected []exercise.ExerciseID
		declared []exercise.ExerciseID
	}{
		{"頻度が未設定", program.Frequency{}, simpleTarget(t), []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}},
		{"週目標が未設定", mustFrequency(t, 3), program.WeeklyVolumeTarget{}, []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}},
		{"種目が空", mustFrequency(t, 3), simpleTarget(t), nil, nil},
		{"空の種目ID", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench", ""}, []exercise.ExerciseID{"bench", ""}},
		{"種目が重複", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench", "bench"}, []exercise.ExerciseID{"bench", "bench"}},
		{"宣言が空", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench"}, nil},
		{"宣言が選択に含まれていない", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"squat"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := program.NewProgram(c.freq, c.target, c.selected, c.declared)
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
	p, err := program.NewProgram(mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"})
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
	input := []exercise.ExerciseID{"bench", "squat"}
	p, err := program.NewProgram(mustFrequency(t, 3), simpleTarget(t), input, []exercise.ExerciseID{"bench"})
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}

	input[0] = "tampered"
	if !p.Includes("bench") || p.Includes("tampered") {
		t.Error("入力スライスの書き換えが集約に波及している")
	}
}

// 週目標のアクセサ。Task 16 の入口で使われるので、空を返すと
// 全セッションの補助種目が消える。
func TestProgram_WeeklyTarget(t *testing.T) {
	target := simpleTarget(t)
	p, err := program.NewProgram(mustFrequency(t, 3), target, []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"})
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}

	got := p.WeeklyTarget()
	if got.IsEmpty() {
		t.Fatal("週目標が空で返る")
	}
	if len(got.Regions()) != len(target.Regions()) {
		t.Errorf("区分数が誤り: got %d, want %d", len(got.Regions()), len(target.Regions()))
	}
	for _, r := range target.Regions() {
		if math.Abs(got.Sets(r)-target.Sets(r)) > 1e-9 {
			t.Errorf("%s の目標が誤り: got %v, want %v", r, got.Sets(r), target.Sets(r))
		}
	}
}

// 種目IDの検証を NewExerciseID に委ねていること。
// 独自判定だと前後に空白のあるIDが通り、種目マスタと永久に一致しない。
// 一致しないIDは黙って無視されるので、選んだ種目が理由なく消える。
func TestNewProgram_ValidatesExerciseIDs(t *testing.T) {
	for _, id := range []exercise.ExerciseID{"   ", " bench", "bench ", "\tbench"} {
		got, err := program.NewProgram(mustFrequency(t, 3), simpleTarget(t),
			[]exercise.ExerciseID{id}, []exercise.ExerciseID{id})
		if err == nil {
			t.Errorf("不正な種目ID %q が通ってしまう: %+v", id, got)
		}
	}
}

// 量子化がコンストラクタで効いていること。
// 境界のすぐ外側でも、量子化して境界に乗る値は通す。
func TestNewWeeklyVolumeTarget_Quantizes(t *testing.T) {
	for _, v := range []float64{0.4999996, 40.0000004} {
		if _, err := program.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
			training.ChestMid: v,
		}); err != nil {
			t.Errorf("量子化すれば境界に収まる %v が弾かれた: %v", v, err)
		}
	}

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.ChestMid: 12.12345678901234,
	})
	got := target.Sets(training.ChestMid)
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("量子化されていない: %v（小数点以下 %d 桁）", got, n)
	}
}
