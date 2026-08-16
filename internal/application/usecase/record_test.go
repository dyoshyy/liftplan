package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestRecordSets_SavesLogs(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	uc := usecase.NewRecordSets(repo)

	log, err := training.NewSetLog(training.SetLogParams{
		ID: "01J-A", PerformedOn: testDate, ExerciseID: "bench",
		WeightKg: 85, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}

	if err := uc.Execute(context.Background(), []*training.SetLog{log}); err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Errorf("保存されていない: %d", len(repo.saved))
	}
}

func TestRecordSets_EmptyIsNoop(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	if err := usecase.NewRecordSets(repo).Execute(context.Background(), nil); err != nil {
		t.Errorf("空の保存でエラーになった: %v", err)
	}
	// 件数ではなく呼び出しの有無で見る。空スライスを渡しても
	// saved は増えないので、件数だけでは素通ししていても気づけない。
	if repo.calls != 0 {
		t.Errorf("空なのにリポジトリを叩いた: %d回", repo.calls)
	}
}

func TestRecordSets_PropagatesError(t *testing.T) {
	boom := errors.New("書けない")
	repo := &fakeLogs{err: boom}

	log, _ := training.NewSetLog(training.SetLogParams{
		ID: "01J-B", PerformedOn: testDate, ExerciseID: "bench",
		WeightKg: 85, Reps: 9, RIR: 2,
	})
	if err := usecase.NewRecordSets(repo).Execute(context.Background(), []*training.SetLog{log}); !errors.Is(err, boom) {
		t.Errorf("エラーが伝播していない: %v", err)
	}
}

func TestRecordConditions_SavesItems(t *testing.T) {
	repo := &fakeConditions{log: training.NewConditionLog(nil)}
	uc := usecase.NewRecordConditions(repo)

	item := training.NewDailyCondition(testDate).WithBodyWeight(75).WithSleepHours(7)
	if err := uc.Execute(context.Background(), []training.DailyCondition{item}); err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(repo.saved) != 1 {
		t.Errorf("保存されていない: %d", len(repo.saved))
	}
}

func TestRecordConditions_EmptyIsNoop(t *testing.T) {
	repo := &fakeConditions{log: training.NewConditionLog(nil)}
	if err := usecase.NewRecordConditions(repo).Execute(context.Background(), nil); err != nil {
		t.Errorf("空の保存でエラーになった: %v", err)
	}
	if repo.calls != 0 {
		t.Errorf("空なのにリポジトリを叩いた: %d回", repo.calls)
	}
}

func TestRecordConditions_PropagatesError(t *testing.T) {
	boom := errors.New("書けない")
	repo := &fakeConditions{err: boom}
	item := training.NewDailyCondition(testDate).WithBodyWeight(75)

	err := usecase.NewRecordConditions(repo).Execute(
		context.Background(), []training.DailyCondition{item})
	if !errors.Is(err, boom) {
		t.Errorf("エラーが伝播していない: %v", err)
	}
}

// nil を混ぜたままリポジトリに渡さないこと。
// 実装側で panic するか黙って飛ばされるかが実装依存になる。
func TestRecordSets_RejectsNilEntries(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	log, err := training.NewSetLog(training.SetLogParams{
		ID: "01J-C", PerformedOn: testDate, ExerciseID: "bench",
		WeightKg: 85, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}

	err = usecase.NewRecordSets(repo).Execute(
		context.Background(), []*training.SetLog{log, nil})
	if !errors.Is(err, usecase.ErrInvalidInput) {
		t.Errorf("nil が弾かれていない: %v", err)
	}
	if repo.callCount() != 0 {
		t.Errorf("検証に失敗したのにリポジトリを叩いた: %d回", repo.callCount())
	}
}
