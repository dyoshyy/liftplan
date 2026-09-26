package memory_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

var day = training.MustDate(2026, time.August, 17)

func mkSetLog(t *testing.T, id string, kg float64) *setlog.SetLog {
	t.Helper()
	l, err := setlog.NewSetLog(setlog.SetLogParams{
		ID: id, PerformedOn: day, ExerciseID: "bench",
		WeightKg: kg, Reps: 9, RIR: 2,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗: %v", err)
	}
	return l
}

func TestExerciseRepository_ReturnsSeed(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(all)

	got, err := repo.FindAll(context.Background(), userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if len(got) != len(all) {
		t.Errorf("件数が誤り: got %d, want %d", len(got), len(all))
	}
}

// FindAll が返す値がリポジトリ内部の状態をエイリアスしないこと。
func TestExerciseRepository_DoesNotAliasItsState(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	repo := memory.NewExerciseRepository(pool)

	// 呼び出し側がスライスを壊しても、次の取得に影響しない。
	got, _ := repo.FindAll(context.Background(), userA(t))
	for i := range got {
		got[i] = nil
	}

	again, _ := repo.FindAll(context.Background(), userA(t))
	for i, e := range again {
		if e == nil {
			t.Fatalf("%d番目が nil になっている", i)
		}
	}

	// コンストラクタに渡したスライスを後から壊しても影響しない。
	pool[0] = nil
	third, _ := repo.FindAll(context.Background(), userA(t))
	if third[0] == nil {
		t.Error("コンストラクタの引数をエイリアスしている")
	}
}

// Save と FindAll を並行に呼んでも壊れないこと。
func TestRepositories_AreSafeForConcurrentUse(t *testing.T) {
	logs := memory.NewSetLogRepository()
	conditions := memory.NewConditionRepository()
	programs := memory.NewProgramRepository()
	ctx := context.Background()
	user := userA(t)

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = logs.Save(ctx, user, []*setlog.SetLog{mkSetLog(t, fmt.Sprintf("c%03d", i), 85)})
			_, _ = logs.FindAll(ctx, user)
			_ = conditions.Save(ctx, user, []condition.DailyCondition{
				condition.NewDailyCondition(day.AddDays(-i)).WithBodyWeight(75),
			})
			_, _ = conditions.FindAll(ctx, user)
			_, _ = programs.Get(ctx, user)
		}(i)
	}
	wg.Wait()

	if logs.Size(user) != 16 {
		t.Errorf("並行保存で件数が合わない: %d", logs.Size(user))
	}
	if conditions.Size(user) != 16 {
		t.Errorf("並行保存で件数が合わない: %d", conditions.Size(user))
	}
}
