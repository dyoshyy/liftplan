package planning

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// raiseForFocus は ProjectedSession の非公開フィールドを読むので、
// パッケージ内から ProjectedSession を直接組んで検査する。
//
// 足すのは「重点種目を指定したから出る回」の刺激だけ。軸は重点種目でも
// 重い番で、重点でなくても宣言種目として来る回なので足さない。
func TestRaiseForFocus_AddsOnlyTheVariationLane(t *testing.T) {
	bench, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "bench", Name: "bench",
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0, training.TricepsLateral: 0.5},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("種目の生成に失敗: %v", err)
	}
	larsen, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "larsen", Name: "larsen", DerivedFrom: "bench",
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("種目の生成に失敗: %v", err)
	}
	three, err := training.NewSetCount(3)
	if err != nil {
		t.Fatalf("セット数が不正: %v", err)
	}
	// TricepsLateral は目標に無い（0 = 狙わない）。足しても 0 のまま。
	base, err := program.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.Quad: 12,
	})
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	cases := []struct {
		name     string
		sessions []ProjectedSession
		wantMid  float64
	}{
		{
			name:     "軸だけなら足さない",
			sessions: []ProjectedSession{{axis: bench, axisSets: three}},
			wantMid:  12,
		},
		{
			name:     "バリエーションレーンは足す",
			sessions: []ProjectedSession{{axis: bench, axisSets: three, variation: larsen, variationSets: three}},
			wantMid:  12 + 3,
		},
		{
			name: "回をまたいで積む",
			sessions: []ProjectedSession{
				{axis: bench, axisSets: three},
				{axis: bench, axisSets: three, variation: larsen, variationSets: three},
				{variation: larsen, variationSets: three},
			},
			wantMid: 12 + 6,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := raiseForFocus(base, c.sessions)
			if math.Abs(got.Sets(training.ChestMid)-c.wantMid) > 1e-9 {
				t.Errorf("ChestMid の目標が %v。%v のはず", got.Sets(training.ChestMid), c.wantMid)
			}
			if got.Sets(training.Quad) != 12 {
				t.Errorf("系統が触らない Quad が %v に動いた。12 のまま", got.Sets(training.Quad))
			}
			if got.Sets(training.TricepsLateral) != 0 {
				t.Errorf("目標0の区分に %v が足された。狙わない区分は 0 のまま", got.Sets(training.TricepsLateral))
			}
		})
	}
}
