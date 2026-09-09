package planning_test

import (
	"math"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/planning"
)

// 体重の既定値。体重を一度も記録していない利用者に使う。
const defaultBodyWeight = 70.0

var loadDay = training.MustDate(2026, time.August, 17)

func bodyWeightOn(t *testing.T, date training.Date, kg float64) condition.ConditionLog {
	t.Helper()
	return condition.NewConditionLog([]condition.DailyCondition{
		condition.NewDailyCondition(date).WithBodyWeight(kg),
	})
}

func chinning(t *testing.T) *exercise.Exercise {
	t.Helper()
	return mustExercise(t, exercise.ExerciseParams{
		ID: "chin", Name: "チンニング",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})
}

func benchPress(t *testing.T) *exercise.Exercise {
	t.Helper()
	return mustExercise(t, exercise.ExerciseParams{
		ID: "bench", Name: "ベンチプレス",
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
		IncrementKg: 2.5,
	})
}

// EffectiveLoad は記録を推定に使う負荷へ読み替える。失敗せず、返せない場合は
// 0kg を返して下流の除外に委ねる。
func TestEffectiveLoad(t *testing.T) {
	cases := []struct {
		name       string
		exercise   *exercise.Exercise
		on         training.Date
		addedKg    float64
		conditions condition.ConditionLog
		want       float64
	}{
		{
			// 落とすと履歴の件数が変わり、やったはずのセットが残差から消える。
			// 生の加重のまま残すと、66.5kg 相当の自重セットが 0kg として推定に入る。
			name: "体重を一度も記録していないなら既定値で計算する",
			// 0 + 0.95 × 70
			exercise: chinning(t), on: loadDay, addedKg: 0,
			conditions: condition.NewConditionLog(nil), want: 66.5,
		},
		{
			name: "実測があるならそれを使う",
			// 10 + 0.95 × 75
			exercise: chinning(t), on: loadDay, addedKg: 10,
			conditions: bodyWeightOn(t, loadDay, 75), want: 81.25,
		},
		{
			// ここで既定値に倒すと、実測を持っている利用者の履歴に既定値由来の
			// 点が混ざり、推定が実測と既定値のあいだで揺れる。0kg なら
			// NewOneRepMax の下限が効いて下流が除外する。
			name:     "実測はあるがそのセットより前に無い日は0kgにして推定から落とす",
			exercise: chinning(t), on: loadDay.AddDays(-8), addedKg: 10,
			conditions: bodyWeightOn(t, loadDay.AddDays(-3), 75), want: 0,
		},
		{
			name:     "自重が乗らない種目は記録された重量がそのまま",
			exercise: benchPress(t), on: loadDay, addedKg: 80,
			conditions: condition.NewConditionLog(nil), want: 80,
		},
		{
			// 選択から外した種目の過去ログなど。体重とは無関係に起こる。
			name:     "種目が引けなければ0kg",
			exercise: nil, on: loadDay, addedKg: 10,
			conditions: bodyWeightOn(t, loadDay, 75), want: 0,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := mkLogOn(t, "s1", c.on, "chin", c.addedKg, 8, 2)

			got := planning.EffectiveLoad(log, c.exercise, c.conditions)

			if math.Abs(got.Kg()-c.want) > 1e-9 {
				t.Errorf("実効負荷が %vkg。%v のはず", got.Kg(), c.want)
			}
		})
	}
}

// AddedWeight は実効負荷の目標から実際に付ける加重を返す。EffectiveLoad の逆。
//
// 体重が引けないケースは「一度も記録していない」しか無い。処方は当日で引くので、
// 記録が一件でもあれば必ず過去にある。よって常に値を返す。
func TestAddedWeight(t *testing.T) {
	cases := []struct {
		name       string
		exercise   *exercise.Exercise
		totalKg    float64
		conditions condition.ConditionLog
		want       float64
	}{
		{
			// 画面に出すのは「プレートを何kg付けるか」。体重込みの 77.5kg と
			// 言われても何をすればいいか分からない。
			name: "体重ぶんを引いて加重で返す",
			// 77.5 − 0.95 × 75
			exercise: chinning(t), totalKg: 77.5,
			conditions: bodyWeightOn(t, loadDay, 75), want: 6.25,
		},
		{
			// ここで体重を0と見なすと引き算が消えて added = total になり、
			// 自重種目に総負荷をそのまま処方する（チンニングに「77.5kg 付けろ」）。
			name: "体重を一度も記録していないなら既定値で引く",
			// 77.5 − 0.95 × 70
			exercise: chinning(t), totalKg: 77.5,
			conditions: condition.NewConditionLog(nil), want: 11.0,
		},
		{
			// 丸めは総負荷に既にかかっている。ここで刻みに乗せ直すと総負荷が
			// 目標からずれ、どちらを信じる数字か分からなくなる。
			name: "引いたあとの数字は刻みに乗せない",
			// 75 − 71.25。増加単位 2.5 に丸めると 2.5 か 5.0 になる
			exercise: chinning(t), totalKg: 75,
			conditions: bodyWeightOn(t, loadDay, 75), want: 3.75,
		},
		{
			// 目標そのものを下げるのはデロードの仕事。
			name:     "自重だけで目標を超えるなら加重は0",
			exercise: chinning(t), totalKg: 60,
			conditions: bodyWeightOn(t, loadDay, 75), want: 0,
		},
		{
			name:     "自重が乗らない種目は総負荷がそのまま",
			exercise: benchPress(t), totalKg: 77.5,
			conditions: condition.NewConditionLog(nil), want: 77.5,
		},
		{
			// 鮮度は見ない。実測がある限り既定値には倒さない。倒すと推定が
			// 実測と既定値のあいだで揺れる。
			name:     "100日前の実測でも既定値より優先する",
			exercise: chinning(t), totalKg: 77.5,
			conditions: bodyWeightOn(t, loadDay.AddDays(-100), 75), want: 6.25,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := planning.AddedWeight(
				mustWeight(t, c.totalKg), c.exercise, c.conditions, loadDay)

			if math.Abs(got.Kg()-c.want) > 1e-9 {
				t.Errorf("加重が %vkg。%v のはず", got.Kg(), c.want)
			}
		})
	}
}

// 対象日以前で最も新しい体重を使う。未来の記録は見ない。
func TestAddedWeight_UsesLatestBodyWeightOnOrBeforeTheDate(t *testing.T) {
	log := condition.NewConditionLog([]condition.DailyCondition{
		condition.NewDailyCondition(loadDay.AddDays(-30)).WithBodyWeight(70),
		condition.NewDailyCondition(loadDay.AddDays(-3)).WithBodyWeight(75),
		condition.NewDailyCondition(loadDay.AddDays(1)).WithBodyWeight(90),
	})

	got := planning.AddedWeight(mustWeight(t, 77.5), chinning(t), log, loadDay)

	// 3日前の75kgを使う。翌日の90kgも30日前の70kgも使わない。
	if math.Abs(got.Kg()-6.25) > 1e-9 {
		t.Errorf("加重が %vkg。3日前の体重75kgから 6.25 のはず", got.Kg())
	}
}

// 0kg のセットからは推定1RMが出ないこと。
//
// これは変異テスト。EffectiveLoad は「負荷が出せない」を 0kg で表し、除外は
// 下流に任せている。NewOneRepMax の下限（training.SmallestPositive）を動かすと
// この前提が静かに壊れ、負荷不明のセットが推定に混ざる。
func TestSetLog_ZeroWeightIsNotEstimable(t *testing.T) {
	log := mkLogOn(t, "x", loadDay, "chin", 0, 8, 2)

	if orm, ok := log.EstimatedOneRepMax(); ok {
		t.Errorf("0kg から推定1RM %vkg が出ている。負荷不明のセットが推定に入る",
			orm.Kg())
	}
}
