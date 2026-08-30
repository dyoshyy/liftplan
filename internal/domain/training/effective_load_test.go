package training_test

import (
	"math"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// 体重の既定値。体重を一度も記録していない利用者に使う。
const defaultBodyWeight = 70.0

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

func chinLog(t *testing.T, on training.Date, kg float64) *training.SetLog {
	t.Helper()
	return mkLogOn(t, "chin-1", on, "chin", kg, 8, 2)
}

// --- EffectiveLoad（記録 → 推定に使う負荷） ---

// 体重を一度も記録していないなら既定値で計算する。
//
// 落とすと履歴の件数が変わり、やったはずのセットが残差から消える。
// 生の加重のまま残すと、実際は 66.5kg 相当の自重セットが 0kg として
// 推定に入る。どちらも避けたいので既定値を使う。
func TestEffectiveLoad_UsesDefaultWhenNoBodyWeightRecorded(t *testing.T) {
	got := training.EffectiveLoad(
		chinLog(t, loadDay, 0), chinning(t), training.NewConditionLog(nil))

	// 0 + 0.95 × 70 = 66.5
	if math.Abs(got.Kg()-66.5) > 1e-9 {
		t.Errorf("実効負荷が %vkg。既定体重70kgから 66.5 のはず", got.Kg())
	}
}

// 実測があるなら、それを使う。既定値には倒さない。
func TestEffectiveLoad_PrefersRecordedBodyWeight(t *testing.T) {
	got := training.EffectiveLoad(
		chinLog(t, loadDay, 10), chinning(t), bodyWeightLog(t, loadDay, 75))

	// 10 + 0.95 × 75 = 81.25
	if math.Abs(got.Kg()-81.25) > 1e-9 {
		t.Errorf("実効負荷が %vkg。実測75kgから 81.25 のはず", got.Kg())
	}
}

// 実測はあるが、そのセットより前には無い日は 0kg にして推定から落とす。
//
// ここで既定値に倒すと、実測を持っている利用者の履歴に既定値由来の点が
// 混ざり、推定が実測と既定値のあいだで揺れる。0kg なら NewOneRepMax の
// 下限が効いて下流が除外する。セットは残るので件数は変わらない。
func TestEffectiveLoad_FallsBackToZeroWhenRecordedOnlyLater(t *testing.T) {
	// 体重の記録は3日前だけ。セットはその5日前。
	log := bodyWeightLog(t, loadDay.AddDays(-3), 75)

	got := training.EffectiveLoad(chinLog(t, loadDay.AddDays(-8), 10), chinning(t), log)

	if got.Kg() != 0 {
		t.Errorf("実効負荷が %vkg。実測より前の日付なので 0 のはず", got.Kg())
	}
}

// 自重が乗らない種目は記録された重量がそのまま。体重を見ない。
func TestEffectiveLoad_PassesThroughWhenNoBodyWeightFactor(t *testing.T) {
	bench := mustExercise(t, training.ExerciseParams{
		ID: "bench", Name: "ベンチプレス", Kind: training.KindMain,
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5,
	})

	got := training.EffectiveLoad(
		mkLogOn(t, "b1", loadDay, "bench", 80, 5, 2), bench, training.NewConditionLog(nil))

	if math.Abs(got.Kg()-80) > 1e-9 {
		t.Errorf("実効負荷が %vkg。80 がそのまま出るはず", got.Kg())
	}
}

// --- AddedWeight（処方 → 実際に付ける加重） ---

// 処方は加重で返す。体重込みの総負荷を渡されても、画面に出すのは
// 「プレートを何kg付けるか」。77.5 − 75×0.95 = 6.25kg。
func TestAddedWeight_SubtractsBodyWeight(t *testing.T) {
	got := training.AddedWeight(
		mustWeight(t, 77.5), chinning(t), bodyWeightLog(t, loadDay, 75), loadDay)

	if math.Abs(got.Kg()-6.25) > 1e-9 {
		t.Errorf("加重が %vkg。77.5 − 75×0.95 = 6.25 のはず", got.Kg())
	}
}

// 体重を一度も記録していないなら既定値で引く。
//
// ここで体重を0と見なすと引き算が消えて added = total になり、
// 自重種目に総負荷をそのまま処方する（チンニングに「77.5kg 付けろ」）。
// 既定値なら引き算が生きるので、この形の破綻が起きない。
func TestAddedWeight_UsesDefaultWhenNoBodyWeightRecorded(t *testing.T) {
	got := training.AddedWeight(
		mustWeight(t, 77.5), chinning(t), training.NewConditionLog(nil), loadDay)

	// 77.5 − 0.95 × 70 = 11.0
	if math.Abs(got.Kg()-11.0) > 1e-9 {
		t.Errorf("加重が %vkg。既定体重70kgから 11.0 のはず", got.Kg())
	}
	if got.Kg() >= 77.5 {
		t.Errorf("加重が %vkg。総負荷がそのまま処方されている", got.Kg())
	}
}

// 引いたあとの数字は刻みに乗らない。丸めは総負荷に既にかかっているので、
// ここで丸め直すと総負荷が目標からずれる。
func TestAddedWeight_DoesNotRoundToIncrement(t *testing.T) {
	got := training.AddedWeight(
		mustWeight(t, 75), chinning(t), bodyWeightLog(t, loadDay, 75), loadDay)

	// 75 − 71.25 = 3.75。増加単位 2.5 に丸めると 2.5 か 5.0 になる。
	if math.Abs(got.Kg()-3.75) > 1e-9 {
		t.Errorf("加重が %vkg。3.75 のはず（刻みに丸めない）", got.Kg())
	}
}

// 目標が体重×係数を超えないなら、付けるものは何も無い。「自重」。
func TestAddedWeight_ClampsToZeroWhenBodyWeightAlreadyExceedsTarget(t *testing.T) {
	got := training.AddedWeight(
		mustWeight(t, 60), chinning(t), bodyWeightLog(t, loadDay, 75), loadDay)

	if got.Kg() != 0 {
		t.Errorf("加重が %vkg。自重で足りるので 0 のはず", got.Kg())
	}
}

// 対象日以前で最も新しい体重を使う。鮮度は見ない。
func TestAddedWeight_UsesLatestBodyWeightOnOrBeforeTheDate(t *testing.T) {
	log := training.NewConditionLog([]training.DailyCondition{
		training.NewDailyCondition(loadDay.AddDays(-30)).WithBodyWeight(70),
		training.NewDailyCondition(loadDay.AddDays(-3)).WithBodyWeight(75),
		training.NewDailyCondition(loadDay.AddDays(1)).WithBodyWeight(90),
	})

	got := training.AddedWeight(mustWeight(t, 77.5), chinning(t), log, loadDay)

	// 3日前の75kgを使う。翌日の90kgも30日前の70kgも使わない。
	if math.Abs(got.Kg()-6.25) > 1e-9 {
		t.Errorf("加重が %vkg。3日前の体重75kgから 6.25 のはず", got.Kg())
	}
}

// 自重が乗らない種目は総負荷がそのまま加重。
func TestAddedWeight_PassesThroughWhenNoBodyWeightFactor(t *testing.T) {
	bench := mustExercise(t, training.ExerciseParams{
		ID: "bench", Name: "ベンチプレス", Kind: training.KindMain,
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5,
	})

	got := training.AddedWeight(
		mustWeight(t, 77.5), bench, training.NewConditionLog(nil), loadDay)

	if math.Abs(got.Kg()-77.5) > 1e-9 {
		t.Errorf("加重が %vkg。77.5 がそのまま出るはず", got.Kg())
	}
}

// 既定体重は、記録が一件でもあれば使わない。
//
// 実測75kgの利用者に既定値70kgが混ざると、推定が実測と既定値のあいだで
// 揺れる。処方は当日で体重を引くので、記録が一件でもあれば必ず実測になる。
func TestAddedWeight_DefaultNeverMixesWithRecordedWeight(t *testing.T) {
	log := bodyWeightLog(t, loadDay.AddDays(-100), 75)

	got := training.AddedWeight(mustWeight(t, 77.5), chinning(t), log, loadDay)

	// 100日前でも実測を使う。既定値なら 11.0 になる。
	if math.Abs(got.Kg()-6.25) > 1e-9 {
		t.Errorf("加重が %vkg。実測75kgから 6.25 のはず（既定値が混ざっている）", got.Kg())
	}
}
