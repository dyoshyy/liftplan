package program_test

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
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
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 6, 3), simpleTarget(t),
		[]exercise.ExerciseID{"bench", "squat"}, []exercise.ExerciseID{"bench"}, "")
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
	if _, ok := p.FocusExercise(); ok {
		t.Error("指定していない重点種目が入っている")
	}
}

// 重点種目を指定すると往復すること。指定なしと区別できること。
func TestNewProgram_Focus(t *testing.T) {
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 6, 3), simpleTarget(t),
		[]exercise.ExerciseID{"bench", "squat"}, []exercise.ExerciseID{"bench"}, "bench")
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}
	got, ok := p.FocusExercise()
	if !ok {
		t.Fatal("重点種目が指定なしとして返る")
	}
	if got != exercise.ExerciseID("bench") {
		t.Errorf("重点種目が誤り: %v", got)
	}
}

func TestNewProgram_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name     string
		freq     program.Frequency
		target   program.WeeklyVolumeTarget
		selected []exercise.ExerciseID
		declared []exercise.ExerciseID
		focus    exercise.ExerciseID
	}{
		{"頻度が未設定", program.Frequency{}, simpleTarget(t), []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, ""},
		{"週目標が未設定", mustFrequency(t, 3), program.WeeklyVolumeTarget{}, []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, ""},
		{"種目が空", mustFrequency(t, 3), simpleTarget(t), nil, nil, ""},
		{"空の種目ID", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench", ""}, []exercise.ExerciseID{"bench", ""}, ""},
		{"種目が重複", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench", "bench"}, []exercise.ExerciseID{"bench", "bench"}, ""},
		{"宣言が空", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench"}, nil, ""},
		{"宣言が選択に含まれていない", mustFrequency(t, 3), simpleTarget(t), []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"squat"}, ""},
		{
			// 宣言していない種目を重点にできると、「伸ばしたい種目の中で
			// さらに重点」という意味が崩れる。
			"重点種目が伸ばしたい種目に含まれていない",
			mustFrequency(t, 3), simpleTarget(t),
			[]exercise.ExerciseID{"bench", "squat"}, []exercise.ExerciseID{"bench"}, "squat",
		},
		{
			"重点種目の前後に空白",
			mustFrequency(t, 3), simpleTarget(t),
			[]exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, " bench ",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := program.NewProgram(c.freq, mustVolume(t, 6, 3), c.target, c.selected, c.declared, c.focus)
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
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 6, 3), simpleTarget(t), []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, "")
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
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 6, 3), simpleTarget(t), input, []exercise.ExerciseID{"bench"}, "")
	if err != nil {
		t.Fatalf("生成に失敗: %v", err)
	}

	input[0] = "tampered"
	if !p.Includes("bench") || p.Includes("tampered") {
		t.Error("入力スライスの書き換えが集約に波及している")
	}
}

// fieldsOf は Program の6フィールドを、名前つきの文字列に写す。
//
// 長さではなく中身を写す。分割が1日ぶんだけ残る、並びが変わる、といった
// 壊れ方でも周期は狂う。
func fieldsOf(p *program.Program) map[string]string {
	target := ""
	for _, r := range p.WeeklyTarget().Regions() {
		target += fmt.Sprintf("%s=%v ", r, p.WeeklyTarget().Sets(r))
	}
	focus, _ := p.FocusExercise()
	cycle := ""
	for _, s := range p.Cycle() {
		cycle += fmt.Sprintf("%s%v ", s.Name(), s.Regions())
	}
	return map[string]string{
		"frequency": strconv.Itoa(p.Frequency().PerWeek()),
		"target":    target,
		"selected":  fmt.Sprint(p.SelectedExercises()),
		"declared":  fmt.Sprint(p.DeclaredExercises()),
		"focus":     string(focus),
		"cycle":     cycle,
	}
}

// With* は、自分が差し替えるフィールド以外を全部引き継ぐこと。
//
// 引き継がないと、分割を設定した人が頻度を変えただけで分割が黙って消えて
// 全身法に戻る（#122）。重点種目も同じで、消えると変化種目の日が出なく
// なる（#132）。どちらもあとから足されたフィールドで、D-127（全置換の口が
// declared と focus を落とす）と同じ形の事故。
//
// 重点と分割を両方立てたプログラムから始める。この2つは空が正当な値
// なので、落ちても newProgram の検証では止まらない。ほかの4つは落ちれば
// 検証がエラーにするが、focus と cycle はこのテストだけが守りになる。
// シードのプログラムのように片方が空だと、前後とも空で一致して空振りする。
//
// 分割だけを見ていた TestProgram_WithKeepsCycle（#136）はここに含めた。
func TestProgram_WithKeepsOtherFields(t *testing.T) {
	quadOnly := map[training.MuscleRegion]float64{training.Quad: 10}

	cases := []struct {
		name string
		// changed は、その With* が動かしてよいフィールド。ほかは動かない。
		changed []string
		apply   func(*program.Program) (*program.Program, error)
	}{
		{
			name: "WithFocus", changed: []string{"focus"},
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithFocus("squat")
			},
		},
		{
			// 重点の bench は宣言に残す。外すと検証で弾かれる。
			name: "WithDeclared", changed: []string{"declared"},
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithDeclared([]exercise.ExerciseID{"bench", "deadlift"})
			},
		},
		{
			// 頻度は週目標を道連れにするが、それ以外は道連れにしない。
			name: "WithFrequency", changed: []string{"frequency", "target"},
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithFrequency(mustFrequency(t, 4), mustTarget(t, quadOnly))
			},
		},
		{
			// 宣言の2種目は選択に残す。外すと検証で弾かれる。
			name: "WithSelected", changed: []string{"selected"},
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithSelected([]exercise.ExerciseID{"bench", "squat"})
			},
		},
		{
			name: "WithCycle", changed: []string{"cycle"},
			apply: func(p *program.Program) (*program.Program, error) {
				return p.WithCycle([]program.Split{mustSplit(t, "脚", training.Quad)})
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 6, 3), simpleTarget(t),
				big3(), []exercise.ExerciseID{"bench", "squat"}, "bench")
			if err != nil {
				t.Fatalf("NewProgram: %v", err)
			}
			base, err = base.WithCycle([]program.Split{
				mustSplit(t, "上半身", training.ChestMid, training.Lat),
				mustSplit(t, "下半身", training.Quad, training.Hamstring),
			})
			if err != nil {
				t.Fatalf("WithCycle: %v", err)
			}

			next, err := c.apply(base)
			if err != nil {
				t.Fatalf("差し替えに失敗: %v", err)
			}

			before, after := fieldsOf(base), fieldsOf(next)

			// 分割は WithCycle でしか立てられないので、出発点も With* を
			// 1回通っている。そこで重点が落ちると、前後とも「重点なし」で
			// 一致して全ケースが緑になる（実際に一度そうなった）。
			for field, got := range before {
				if got == "" {
					t.Fatalf("出発点の %s が空。NewProgram か WithCycle が落としている", field)
				}
			}

			for field, want := range before {
				if slices.Contains(c.changed, field) {
					// 何もせず元を返す実装だと、「ほかを保つ」は全部通る。
					if after[field] == want {
						t.Errorf("%s が動いていない: %s", field, want)
					}
					continue
				}
				if after[field] != want {
					t.Errorf("%s が変わった: %s → %s", field, want, after[field])
				}
			}
		})
	}
}

// 週目標のアクセサ。Task 16 の入口で使われるので、空を返すと
// 全セッションの補助種目が消える。
func TestProgram_WeeklyTarget(t *testing.T) {
	target := simpleTarget(t)
	p, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 6, 3), target, []exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, "")
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
		got, err := program.NewProgram(mustFrequency(t, 3), mustVolume(t, 6, 3), simpleTarget(t),
			[]exercise.ExerciseID{id}, []exercise.ExerciseID{id}, "")
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

// mustVolume はテスト用の1回の量。
func mustVolume(t *testing.T, exercises, sets int) program.SessionVolume {
	t.Helper()
	v, err := program.NewSessionVolume(exercises, sets)
	if err != nil {
		t.Fatalf("NewSessionVolume(%d, %d): %v", exercises, sets, err)
	}
	return v
}
