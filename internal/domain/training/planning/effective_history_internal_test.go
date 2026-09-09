package planning

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

// このリポジトリで唯一のパッケージ内テスト。
//
// 件数が保たれることは呼び出し側から観測できない。カバレッジも残差も生の履歴で
// 計算されるため、変換後が何件になろうと出力に現れない。外から書こうとして、
// 実際に間引く実装へ戻しても緑のままのテストができた。

func TestEffectiveHistory_KeepsEverySet(t *testing.T) {
	day := training.MustDate(2026, time.August, 17)

	chin, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "chin", Name: "チンニング",
		Stimulus:         map[training.MuscleRegion]float64{training.Lat: 1.0},
		IncrementKg:      2.5,
		BodyweightFactor: 0.95,
	})
	if err != nil {
		t.Fatalf("種目の生成に失敗: %v", err)
	}

	// 体重の記録は3日前だけ。それより前のセットは負荷が出せず 0kg になる。
	conditions := condition.NewConditionLog([]condition.DailyCondition{
		condition.NewDailyCondition(day.AddDays(-3)).WithBodyWeight(75),
	})

	logs := make([]*setlog.SetLog, 0, 6)
	for i, daysAgo := range []int{10, 9, 8, 2, 1, 0} {
		l, err := setlog.NewSetLog(setlog.SetLogParams{
			ID:          string(rune('a' + i)),
			PerformedOn: day.AddDays(-daysAgo),
			ExerciseID:  "chin",
			WeightKg:    10,
			Reps:        8,
			RIR:         2,
		})
		if err != nil {
			t.Fatalf("記録の生成に失敗: %v", err)
		}
		logs = append(logs, l)
	}
	in := setlog.NewHistory(logs)

	got := effectiveHistory(in, []*exercise.Exercise{chin}, conditions)

	if len(got.Logs()) != len(in.Logs()) {
		t.Fatalf("変換で件数が変わった: %d → %d。負荷が出せないセットも残すこと",
			len(in.Logs()), len(got.Logs()))
	}

	// 前半3件は 0kg（体重より前）、後半3件は実効負荷になっていること。
	// 件数だけ合わせて中身が素通りしていないかを見る。
	var zero, loaded int
	for _, l := range got.Logs() {
		if l.Weight().Kg() == 0 {
			zero++
			continue
		}
		loaded++
	}
	if zero != 3 || loaded != 3 {
		t.Errorf("0kg が %d件、負荷ありが %d件。3件ずつのはず", zero, loaded)
	}
}
