package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

func TestRecordSets_SavesLogs(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	uc := newRecordSets(t, repo)

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
	if err := newRecordSets(t, repo).Execute(context.Background(), nil); err != nil {
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
	if err := newRecordSets(t, repo).Execute(context.Background(), []*training.SetLog{log}); !errors.Is(err, boom) {
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

	err = newRecordSets(t, repo).Execute(
		context.Background(), []*training.SetLog{log, nil})
	if !errors.Is(err, usecase.ErrInvalidInput) {
		t.Errorf("nil が弾かれていない: %v", err)
	}
	if repo.callCount() != 0 {
		t.Errorf("検証に失敗したのにリポジトリを叩いた: %d回", repo.callCount())
	}
}

// newRecordSets はシードの種目マスタを使う RecordSets を作る。
func newRecordSets(t *testing.T, repo training.SetLogWriter) *usecase.RecordSets {
	t.Helper()
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	return usecase.NewRecordSets(repo, &fakeExercises{all: pool})
}

// 種目マスタに無い種目のログを受け取らないこと。
// 受け取ると、その実績はどの筋区分にも計上されないまま履歴に残り続ける。
// 削除の口が無く、同じIDの再送は衝突になるので復旧できない。
func TestRecordSets_RejectsUnknownExercise(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	log, err := training.NewSetLog(training.SetLogParams{
		ID: "01J-U", PerformedOn: testDate, ExerciseID: "存在しない種目",
		WeightKg: 85, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}

	err = newRecordSets(t, repo).Execute(context.Background(), []*training.SetLog{log})
	if !errors.Is(err, training.ErrExerciseNotFound) {
		t.Errorf("未知の種目が弾かれていない: %v", err)
	}
	if !errors.Is(err, usecase.ErrInvalidInput) {
		t.Errorf("入力の不正として分類されていない: %v", err)
	}
	if repo.callCount() != 0 {
		t.Errorf("検証に失敗したのにリポジトリを叩いた: %d回", repo.callCount())
	}
}

// キャンセル済みの context では保存しないこと。
func TestRecordSets_StopsOnCancelledContext(t *testing.T) {
	repo := &fakeLogs{history: training.NewHistory(nil)}
	log, _ := training.NewSetLog(training.SetLogParams{
		ID: "01J-K", PerformedOn: testDate, ExerciseID: "bench",
		WeightKg: 85, Reps: 9, RIR: 2,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := newRecordSets(t, repo).Execute(ctx, []*training.SetLog{log}); !errors.Is(err, context.Canceled) {
		t.Errorf("キャンセルが伝わっていない: %v", err)
	}
	if repo.callCount() != 0 {
		t.Errorf("キャンセル済みなのに保存した: %d回", repo.callCount())
	}
}

func TestRecordConditions_StopsOnCancelledContext(t *testing.T) {
	repo := &fakeConditions{log: training.NewConditionLog(nil)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	item := training.NewDailyCondition(testDate).WithBodyWeight(75)
	err := usecase.NewRecordConditions(repo).Execute(ctx, []training.DailyCondition{item})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("キャンセルが伝わっていない: %v", err)
	}
	if repo.callCount() != 0 {
		t.Errorf("キャンセル済みなのに保存した: %d回", repo.callCount())
	}
}
