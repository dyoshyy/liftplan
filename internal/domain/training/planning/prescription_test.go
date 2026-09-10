package planning_test

import (
	"math"
	"sort"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

func mustFrequency(t *testing.T, n int) program.Frequency {
	t.Helper()
	f, err := program.NewFrequency(n)
	if err != nil {
		t.Fatalf("NewFrequency(%d): %v", n, err)
	}
	return f
}

func mustSelect(t *testing.T, c planning.PrescriptionCatalog, f program.Frequency, i int) planning.Prescription {
	t.Helper()
	s, ok := c.Select(f, i)
	if !ok {
		t.Fatalf("週%d回の%d本目のスロットが取れない", f.PerWeek(), i)
	}
	return s
}

func TestNewFrequency_Range(t *testing.T) {
	for _, n := range []int{0, -1, 5, 7, 100} {
		if _, err := program.NewFrequency(n); err == nil {
			t.Errorf("週%d回が通ってしまう", n)
		}
	}
	for _, n := range []int{1, 3, 4} {
		if f, err := program.NewFrequency(n); err != nil || f.PerWeek() != n {
			t.Errorf("週%d回が弾かれた: %v", n, err)
		}
	}

	var zero program.Frequency
	if !zero.IsZero() {
		t.Error("ゼロ値が IsZero でない")
	}
	if mustFrequency(t, 3).IsZero() {
		t.Error("有効な頻度が IsZero になっている")
	}
}

// スロット構成そのものをゴールデンで固定する。
//
// 強度帯とセット数はユーザーの処方を直接決める。ここが黙って変わると、
// 全処方の重量が変わったことに誰も気づけない。
func TestPrescriptionCatalog_GoldenLayout(t *testing.T) {
	c := planning.NewPrescriptionCatalog()

	type entry struct {
		intent    planning.Intent
		intensity float64
		sets      int
		targetRIR int
	}
	want := map[int][]entry{
		1: {
			{planning.IntentStandard, 0.81, 4, 2},
		},
		2: {
			{planning.IntentStandard, 0.81, 4, 2},
			{planning.IntentHeavy, 0.88, 3, 1},
		},
		3: {
			{planning.IntentStandard, 0.81, 4, 2},
			{planning.IntentHeavy, 0.88, 3, 1},
			{planning.IntentLight, 0.76, 4, 2},
		},
		4: {
			{planning.IntentStandard, 0.81, 4, 2},
			{planning.IntentHeavy, 0.88, 3, 1},
			{planning.IntentLight, 0.76, 4, 2},
			{planning.IntentLight, 0.78, 4, 2},
		},
	}

	for freq, expected := range want {
		got := c.For(mustFrequency(t, freq))
		if len(got) != len(expected) {
			t.Errorf("週%d回: 処方の数が誤り got %d, want %d", freq, len(got), len(expected))
			continue
		}
		for i, e := range expected {
			if got[i].Intent() != e.intent {
				t.Errorf("週%d回 %d本目: 役割 got %s, want %s", freq, i, got[i].Intent(), e.intent)
			}
			if math.Abs(got[i].Intensity().Float()-e.intensity) > 1e-9 {
				t.Errorf("週%d回 %d本目: 強度 got %v, want %v", freq, i, got[i].Intensity().Float(), e.intensity)
			}
			if got[i].Sets().Int() != e.sets {
				t.Errorf("週%d回 %d本目: セット数 got %d, want %d", freq, i, got[i].Sets().Int(), e.sets)
			}
			if got[i].TargetRIR().Int() != e.targetRIR {
				t.Errorf("週%d回 %d本目: 目標RIR got %d, want %d", freq, i, got[i].TargetRIR().Int(), e.targetRIR)
			}
		}
	}
}

// 週の1本目は必ず標準スロット。
//
// スロットはその週の何本目かで決まるので、設定した頻度より実際に通う回数が
// 少ないと先頭しか使われない。バリエーションのスロットではメイン種目自体を
// 実施しないため、先頭にバリエーションが来るとメインリフトの記録が増えず、
// 42日後に推定1RMが古すぎると判定されて全部の重量が未確定になる。
// しかも通う回数を増やさない限り回復しない。
func TestPrescriptionCatalog_FirstSlotOfTheWeekIsStandard(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	for freq := 1; freq <= 4; freq++ {
		f := mustFrequency(t, freq)
		if got := mustSelect(t, c, f, 0).Intent(); got != planning.IntentStandard {
			t.Errorf("週%d回の1本目が標準スロットでない: %s", freq, got)
		}
	}
}

// 設定より少ない回数しか通わなくても、メインリフト本体を実施するスロットに
// 到達すること。
func TestPrescriptionCatalog_MainLiftIsReachableWhenUnderperforming(t *testing.T) {
	c := planning.NewPrescriptionCatalog()

	for configured := 1; configured <= 4; configured++ {
		for actual := 1; actual <= configured; actual++ {
			f := mustFrequency(t, configured)

			mainLiftSessions := 0
			for i := range actual {
				if mustSelect(t, c, f, i).Intent() != planning.IntentLight {
					mainLiftSessions++
				}
			}
			if mainLiftSessions == 0 {
				t.Errorf("設定週%d回・実施週%d回でメインリフトを一度も実施しない",
					configured, actual)
			}
		}
	}
}

// RIR は調整ダイヤルではなくガードレール。高強度スロットだけ1にする。
func TestPrescriptionCatalog_OnlyHeavyHasRirOne(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	for freq := 1; freq <= 4; freq++ {
		for _, s := range c.For(mustFrequency(t, freq)) {
			want := 2
			if s.Intent() == planning.IntentHeavy {
				want = 1
			}
			if s.TargetRIR().Int() != want {
				t.Errorf("週%d回 %s: 目標RIR got %d, want %d",
					freq, s.Intent(), s.TargetRIR().Int(), want)
			}
		}
	}
}

// 週2回以上なら必ず高強度スロットが1つある。
func TestPrescriptionCatalog_HasExactlyOneHeavySlot(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	for freq := 2; freq <= 4; freq++ {
		heavy := 0
		for _, s := range c.For(mustFrequency(t, freq)) {
			if s.Intent() == planning.IntentHeavy {
				heavy++
			}
		}
		if heavy != 1 {
			t.Errorf("週%d回: 高強度スロットが %d 個", freq, heavy)
		}
	}
}

// 構成の要素数は頻度と一致すること。
// 一致しないと、週ボリュームを構成の長さから見積もる消費者がずれる。
func TestPrescriptionCatalog_LengthMatchesFrequency(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	for freq := 1; freq <= 4; freq++ {
		f := mustFrequency(t, freq)
		if got := len(c.For(f)); got != freq {
			t.Errorf("週%d回の構成が %d 要素", freq, got)
		}
	}
}

func TestPrescriptionCatalog_SelectWrapsAround(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	f := mustFrequency(t, 3)

	first := mustSelect(t, c, f, 0)
	if got := mustSelect(t, c, f, 3); got != first {
		t.Errorf("剰余で巡回していない: %s", got.Intent())
	}
	if got := mustSelect(t, c, f, 6); got != first {
		t.Errorf("2周目で巡回していない: %s", got.Intent())
	}
	if got := mustSelect(t, c, f, -1); got != first {
		t.Errorf("負のインデックスが先頭に丸められていない: %s", got.Intent())
	}

	// 週内の各本目が構成どおりの順序で取れること。
	slots := c.For(f)
	for i := range slots {
		if got := mustSelect(t, c, f, i); got != slots[i] {
			t.Errorf("%d本目が構成と一致しない: %s", i, got.Intent())
		}
	}
}

func TestPrescriptionCatalog_SelectWithZeroFrequency(t *testing.T) {
	var zero program.Frequency
	if _, ok := planning.NewPrescriptionCatalog().Select(zero, 0); ok {
		t.Error("ゼロ値の頻度でスロットが取れてしまう")
	}
	if got := planning.NewPrescriptionCatalog().For(zero); got != nil {
		t.Errorf("ゼロ値の頻度で構成が返る: %v", got)
	}
}

func TestPrescriptionCatalog_ForIsDefensivelyCopied(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	f := mustFrequency(t, 3)

	got := c.For(f)
	got[0] = planning.Prescription{}
	if c.For(f)[0].IsZero() {
		t.Error("返り値の書き換えが内部状態に波及している")
	}
}

func TestPrescriptionCatalog_ValuesAreRealistic(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	for freq := 1; freq <= 4; freq++ {
		for _, s := range c.For(mustFrequency(t, freq)) {
			if v := s.Intensity().Float(); v < 0.70 || v > 0.92 {
				t.Errorf("強度が範囲外: %v", v)
			}
			if n := s.Sets().Int(); n < 1 || n > 6 {
				t.Errorf("セット数が範囲外: %d", n)
			}
			if !s.Intent().Valid() {
				t.Errorf("役割が不正: %q", s.Intent())
			}
			if s.IsZero() {
				t.Error("ゼロ値のスロットが混ざっている")
			}
		}
	}
}

func TestIntent_Valid(t *testing.T) {
	all := planning.AllIntents()
	if len(all) != 3 {
		t.Errorf("役割は3つであるべき: %d", len(all))
	}
	if !sort.SliceIsSorted(all, func(i, j int) bool { return all[i] < all[j] }) {
		t.Errorf("一覧がソートされていない: %v", all)
	}
	for _, r := range all {
		if !r.Valid() {
			t.Errorf("%s が Valid でない", r)
		}
	}
	for _, r := range []planning.Intent{"", "VARIATION", "heavy", "HEAVY "} {
		if r.Valid() {
			t.Errorf("不正な役割が Valid になっている: %q", r)
		}
	}
}

func TestIntent_GoldenValues(t *testing.T) {
	// 永続化・APIに出る値。変えると既存データが読めなくなる。
	want := []string{"HEAVY", "STANDARD", "LIGHT"}
	got := make([]string, 0, len(want))
	for _, r := range planning.AllIntents() {
		got = append(got, string(r))
	}
	assertGolden(t, "Intent", got, want)
}

// 全頻度で Select が成功すること。取りこぼしがあると、その頻度の
// ユーザーだけセッションが組めなくなる。
func TestPrescriptionCatalog_EveryFrequencyIsCovered(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	for freq := 1; freq <= 4; freq++ {
		f := mustFrequency(t, freq)
		if len(c.For(f)) == 0 {
			t.Errorf("週%d回の構成が空", freq)
		}
		for i := range 10 {
			if _, ok := c.Select(f, i); !ok {
				t.Errorf("週%d回の%d本目が取れない", freq, i)
			}
		}
	}
}

func TestPrescription_ZeroValue(t *testing.T) {
	var zero planning.Prescription
	if !zero.IsZero() {
		t.Error("ゼロ値が IsZero でない")
	}
	if zero.Intent() != "" || zero.Sets().Int() != 0 || zero.TargetRIR().Int() != 0 {
		t.Error("ゼロ値が空でない")
	}

	// ゼロ値のセット数・RIRはコンストラクタが拒否する値。
	// これが流通すると、残差計算が狂い「限界まで追い込め」という指示になる。
	if _, err := training.NewSetCount(zero.Sets().Int()); err == nil {
		t.Error("ゼロ値のセット数が正当な値として作り直せてしまう")
	}
	if zero.Intent().Valid() {
		t.Error("ゼロ値の役割が Valid になっている")
	}
}

func TestAllIntents_IsDefensivelyCopied(t *testing.T) {
	got := planning.AllIntents()
	if len(got) == 0 {
		t.Fatal("役割が空")
	}
	got[0] = planning.Intent("TAMPERED")

	for _, r := range planning.AllIntents() {
		if r == planning.Intent("TAMPERED") {
			t.Fatal("返り値の書き換えが内部状態に波及している")
		}
	}
}

// 処方どおり完遂したとき、推定1RMがどう動くかを固定する。
//
// 現在の強度帯と目標RIRの組は、いずれも Epley の逆算より少ないレップに
// 丸まるため、推定1RMは上がらない。つまりこの構成には漸進的過負荷の
// 仕組みが無く、処方どおりやり続けると重量が停滞する。
//
// これは既知の設計課題（docs/decisions.md の D-014 持ち越し課題2）であり、
// 修正はまだ入っていない。定数を変えたときに挙動の変化が見えるよう、
// 現状を数値で固定しておく。
func TestPrescriptionCatalog_RoundTripIsCurrentlyContractive(t *testing.T) {
	c := planning.NewPrescriptionCatalog()
	baseline := mustOneRepMax(t, 105)
	inc := mustIncrement(t, 2.5)

	for _, s := range c.For(mustFrequency(t, 4)) {
		w, err := baseline.WorkWeight(s.Intensity(), inc)
		if err != nil {
			t.Fatalf("%s の処方に失敗: %v", s.Intent(), err)
		}

		// 強度から Epley で逆算した限界レップ数。目標RIRぶん手前で止める。
		limit := 30 * (1/s.Intensity().Float() - 1)
		performed := int(limit) - s.TargetRIR().Int()
		if performed < 1 {
			t.Fatalf("%s: 実施レップが1未満になる（強度 %v・目標RIR %d）",
				s.Intent(), s.Intensity().Float(), s.TargetRIR().Int())
		}

		reps, err := training.NewReps(performed)
		if err != nil {
			t.Fatalf("NewReps: %v", err)
		}
		got, ok := training.EstimateOneRepMax(w, reps, s.TargetRIR())
		if !ok {
			t.Fatalf("%s: 推定できない", s.Intent())
		}

		if got.Kg() > baseline.Kg() {
			t.Errorf("%s: 処方どおり完遂で推定1RMが上がった（%v → %v）。"+
				"漸進的過負荷が入ったなら、このテストを更新すること",
				s.Intent(), baseline.Kg(), got.Kg())
		}
	}
}
