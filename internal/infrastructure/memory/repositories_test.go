package memory_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

var day = training.MustDate(2026, time.August, 17)

func mustCustom(t *testing.T, id, name string) *exercise.Exercise {
	t.Helper()
	e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: id, Name: name,
		Primary: []training.MuscleRegion{training.Lat}, IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

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

func TestExerciseRepository_KeepsDeletedCustoms(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewExerciseRepository(nil)
	a := newUser(t)
	e := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	_ = repo.Save(ctx, a, e)
	_ = repo.Save(ctx, a, e.Delete())

	got, _ := repo.FindAll(ctx, a)
	if len(got) != 1 || !got[0].IsDeleted() {
		t.Errorf("消した種目が消えた状態で1件残っていない: %v", got)
	}
}

func TestExerciseRepository_RefusesSeedExercises(t *testing.T) {
	seedAll, _ := seed.Exercises()
	repo := memory.NewExerciseRepository(seedAll)
	if err := repo.Save(context.Background(), newUser(t), seedAll[0]); err == nil {
		t.Error("共通の種目を保存できてしまった")
	}
}

// 消していない同じ名前は弾き、消した種目と同じ名前は通す（DB の部分一意
// 索引と同じふるまい）。
func TestExerciseRepository_NameIsUniqueAmongAliveCustoms(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewExerciseRepository(nil)
	a := newUser(t)
	first := mustCustom(t, "u-000000000000000a", "アイソラテラル・ロー")
	_ = repo.Save(ctx, a, first)

	dup := mustCustom(t, "u-000000000000000b", "アイソラテラル・ロー")
	if err := repo.Save(ctx, a, dup); !errors.Is(err, exercise.ErrDuplicateExerciseName) {
		t.Errorf("同名が通った: %v", err)
	}
	_ = repo.Save(ctx, a, first.Delete())
	if err := repo.Save(ctx, a, dup); err != nil {
		t.Errorf("消した種目と同名が弾かれた: %v", err)
	}
}

// 保存した順序に関わらず、FindAll はシードの後ろに自分の種目を ID の
// 昇順で並べて返す。ID の降順で保存しても結果は昇順になること。
func TestExerciseRepository_OrdersCustomsByIDRegardlessOfSaveOrder(t *testing.T) {
	ctx := context.Background()
	seedAll, _ := seed.Exercises()
	repo := memory.NewExerciseRepository(seedAll)
	a := newUser(t)

	second := mustCustom(t, "u-000000000000000b", "アイソラテラル・ロー")
	first := mustCustom(t, "u-000000000000000a", "シーテッドロー")
	// b を先に、a を後に保存する（ID の降順）。
	if err := repo.Save(ctx, a, second); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, first); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(seedAll)+2 {
		t.Fatalf("%d 件（期待 %d）", len(got), len(seedAll)+2)
	}
	tail := got[len(seedAll):]
	if tail[0].ID() != first.ID() || tail[1].ID() != second.ID() {
		t.Errorf("保存順のまま返っている（ID 昇順のはず）: %v, %v", tail[0].ID(), tail[1].ID())
	}
}

// 同じ ID を二度渡したら上書きする（exercise.Writer の契約）。
// 名前・部位・刻みも含めて、2回目の値だけが残ること。
func TestExerciseRepository_SaveOverwritesTheSameID(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewExerciseRepository(nil)
	a := newUser(t)

	first, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: "u-000000000000000a", Name: "アイソラテラル・ロー",
		Primary: []training.MuscleRegion{training.Lat}, IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, first); err != nil {
		t.Fatal(err)
	}

	second, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
		ID: "u-000000000000000a", Name: "シーテッドロー",
		Primary: []training.MuscleRegion{training.TrapMid}, IncrementKg: 5.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, a, second); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindAll(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	back := got[len(got)-1]
	if back.Name() != second.Name() || back.Increment().Kg() != second.Increment().Kg() ||
		!slices.Equal(back.PrimaryRegions(), second.PrimaryRegions()) {
		t.Errorf("2回目の値で上書きされていない: %+v", back)
	}
}
