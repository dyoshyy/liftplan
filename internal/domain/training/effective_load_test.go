package training_test

import (
	"math"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

var loadDay = training.MustDate(2026, time.August, 17)

func bodyWeightLog(t *testing.T, date training.Date, kg float64) training.ConditionLog {
	t.Helper()
	return training.NewConditionLog([]training.DailyCondition{
		training.NewDailyCondition(date).WithBodyWeight(kg),
	})
}

func chinning(t *testing.T) *training.Exercise {
	t.Helper()
	return mustExercise(t, training.ExerciseParams{
		ID: "chin", Name: "チンニング", Kind: training.KindAccessory,
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})
}

// 処方は加重で返す。体重込みの総負荷を渡されても、画面に出すのは
// 「プレートを何kg付けるか」。77.5 − 75×0.95 = 6.25kg。
func TestAddedWeight_SubtractsBodyWeight(t *testing.T) {
	got, ok := training.AddedWeight(
		mustWeight(t, 77.5), chinning(t), bodyWeightLog(t, loadDay, 75), loadDay)
	if !ok {
		t.Fatal("体重が記録されているのに加重が出ない")
	}
	if math.Abs(got.Kg()-6.25) > 1e-9 {
		t.Errorf("加重が %vkg。77.5 − 75×0.95 = 6.25 のはず", got.Kg())
	}
}

// 引いたあとの数字は刻みに乗らない。丸めは総負荷にかかっているので、
// ここで丸め直すと総負荷が目標からずれる。
func TestAddedWeight_DoesNotRoundToIncrement(t *testing.T) {
	got, ok := training.AddedWeight(
		mustWeight(t, 75), chinning(t), bodyWeightLog(t, loadDay, 75), loadDay)
	if !ok {
		t.Fatal("体重が記録されているのに加重が出ない")
	}
	// 75 − 71.25 = 3.75。増加単位 2.5 に丸めると 2.5 か 5.0 になる。
	if math.Abs(got.Kg()-3.75) > 1e-9 {
		t.Errorf("加重が %vkg。3.75 のはず（刻みに丸めない）", got.Kg())
	}
}

// 目標が体重×係数を超えないなら、付けるものは何も無い。「自重」。
func TestAddedWeight_ClampsToZeroWhenBodyWeightAlreadyExceedsTarget(t *testing.T) {
	got, ok := training.AddedWeight(
		mustWeight(t, 60), chinning(t), bodyWeightLog(t, loadDay, 75), loadDay)
	if !ok {
		t.Fatal("自重だけで足りる場合も処方は出せるはず")
	}
	if got.Kg() != 0 {
		t.Errorf("加重が %vkg。自重で足りるので 0 のはず", got.Kg())
	}
}

// 体重が一度も記録されていなければ処方できない。「自分で決める」になる。
func TestAddedWeight_FailsWithoutBodyWeight(t *testing.T) {
	if _, ok := training.AddedWeight(
		mustWeight(t, 77.5), chinning(t), training.NewConditionLog(nil), loadDay); ok {
		t.Error("体重が無いのに加重が決まっている")
	}
}

// 対象日以前で最も新しい体重を使う。鮮度は見ない（推定1RM側の42日判定が先に効く）。
func TestAddedWeight_UsesLatestBodyWeightOnOrBeforeTheDate(t *testing.T) {
	log := training.NewConditionLog([]training.DailyCondition{
		training.NewDailyCondition(loadDay.AddDays(-30)).WithBodyWeight(70),
		training.NewDailyCondition(loadDay.AddDays(-3)).WithBodyWeight(75),
		training.NewDailyCondition(loadDay.AddDays(1)).WithBodyWeight(90),
	})

	got, ok := training.AddedWeight(mustWeight(t, 77.5), chinning(t), log, loadDay)
	if !ok {
		t.Fatal("体重が記録されているのに加重が出ない")
	}
	// 3日前の75kgを使う。翌日の90kgも30日前の70kgも使わない。
	if math.Abs(got.Kg()-6.25) > 1e-9 {
		t.Errorf("加重が %vkg。3日前の体重75kgから 6.25 のはず", got.Kg())
	}
}

// 自重が乗らない種目は総負荷がそのまま加重。体重を測っていなくても処方できる。
func TestAddedWeight_PassesThroughWhenNoBodyWeightFactor(t *testing.T) {
	bench := mustExercise(t, training.ExerciseParams{
		ID: "bench", Name: "ベンチプレス", Kind: training.KindMain,
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5,
	})

	got, ok := training.AddedWeight(
		mustWeight(t, 77.5), bench, training.NewConditionLog(nil), loadDay)
	if !ok {
		t.Fatal("自重が乗らない種目は体重が無くても処方できるはず")
	}
	if math.Abs(got.Kg()-77.5) > 1e-9 {
		t.Errorf("加重が %vkg。77.5 がそのまま出るはず", got.Kg())
	}
}
