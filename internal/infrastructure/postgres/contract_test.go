package postgres_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/postgres"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/repositorytest"
)

func TestPostgres_ConditionSatisfiesTheContract(t *testing.T) {
	repositorytest.RunConditionContract(t, func(t *testing.T) training.ConditionRepository {
		return postgres.NewConditionRepository(migratedDB(t))
	})
}

func TestPostgres_ProgramSatisfiesTheContract(t *testing.T) {
	repositorytest.RunProgramContract(t, func(t *testing.T) training.ProgramRepository {
		return postgres.NewProgramRepository(migratedDB(t))
	})
}

// 小数が往復すること。numeric にしたのは double precision だと
// 75.5kg や 7.25時間が往復で揺れうるため。
func TestConditionRepository_RoundTripsDecimals(t *testing.T) {
	repo := postgres.NewConditionRepository(migratedDB(t))
	ctx := context.Background()

	want := []float64{60.05, 75.5, 82.25, 99.99}
	items := make([]training.DailyCondition, 0, len(want))
	for i, kg := range want {
		items = append(items,
			training.NewDailyCondition(training.MustDate(2026, 8, 10+i)).
				WithBodyWeight(kg).WithSleepHours(7.25))
	}
	if err := repo.Save(ctx, items); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	log, _ := repo.FindAll(ctx)
	for i, kg := range want {
		got, ok := log.On(training.MustDate(2026, 8, 10+i))
		if !ok {
			t.Fatalf("%d日目の記録が無い", 10+i)
		}
		if v, ok := got.BodyWeightKg(); !ok || v != kg {
			t.Errorf("%vkg が %v になった", kg, v)
		}
		if v, ok := got.SleepHours(); !ok || v != 7.25 {
			t.Errorf("7.25時間が %v になった", v)
		}
	}
}

// DB 上でも同じ暦日であること。
func TestConditionRepository_StoresTheSameCalendarDate(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	if err := postgres.NewConditionRepository(pool).Save(ctx,
		[]training.DailyCondition{
			training.NewDailyCondition(training.MustDate(2026, 8, 17)).WithBodyWeight(75),
		}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	var stored string
	if err := pool.QueryRow(ctx, "SELECT date::text FROM daily_conditions").Scan(&stored); err != nil {
		t.Fatalf("生の値を読めない: %v", err)
	}
	if stored != "2026-08-17" {
		t.Errorf("DB 上の日付がずれている: %s", stored)
	}
}

// 欠損が NULL として保存されること。0 で埋めると、体重0kg・睡眠0時間という
// 有意味な値と区別できなくなる。
func TestConditionRepository_StoresMissingAsNull(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	if err := postgres.NewConditionRepository(pool).Save(ctx,
		[]training.DailyCondition{
			training.NewDailyCondition(training.MustDate(2026, 8, 17)).WithBodyWeight(75),
		}); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	var isNull bool
	if err := pool.QueryRow(ctx,
		"SELECT sleep_hours IS NULL FROM daily_conditions").Scan(&isNull); err != nil {
		t.Fatalf("生の値を読めない: %v", err)
	}
	if !isNull {
		t.Error("送っていない睡眠時間が NULL でない")
	}
}

// jsonb が往復すること。筋区分の追加でマイグレーションが要らない形。
func TestProgramRepository_RoundTripsJSON(t *testing.T) {
	repo := postgres.NewProgramRepository(migratedDB(t))
	ctx := context.Background()

	freq, _ := training.NewFrequency(4)
	target, err := training.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid: 14.5, training.Quad: 16, training.Calf: 6,
	})
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	program, err := training.NewProgram(freq, target,
		[]training.ExerciseID{"bench", "squat", "deadlift", "calf_raise"})
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	if err := repo.Save(ctx, program); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if got.WeeklyTarget().Sets(training.ChestMid) != 14.5 {
		t.Errorf("小数の週目標が往復していない: %v", got.WeeklyTarget().Sets(training.ChestMid))
	}
	if len(got.WeeklyTarget().Regions()) != 3 {
		t.Errorf("筋区分の数が誤り: %d", len(got.WeeklyTarget().Regions()))
	}
	if len(got.SelectedExercises()) != 4 {
		t.Errorf("選択種目が往復していない: %v", got.SelectedExercises())
	}
}

// 2行目を作れないこと。単一ユーザー前提をスキーマで担保する。
func TestProgramRepository_HoldsExactlyOneRow(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO program (id, per_week, weekly_target, selected)
		VALUES (false, 3, '{}', '[]')`); err == nil {
		t.Error("2行目が作れてしまう")
	}
}

// DB に不正な値が入っていたら、境界で止めてドメインに届けないこと。
func TestProgramRepository_RejectsInvalidStoredRow(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `
		INSERT INTO program (id, per_week, weekly_target, selected)
		VALUES (true, 99, '{"CHEST_MID": 12}', '["bench"]')`); err != nil {
		t.Fatalf("準備に失敗: %v", err)
	}

	if _, err := postgres.NewProgramRepository(pool).Get(ctx); err == nil {
		t.Error("範囲外の頻度が黙って読み込まれた")
	}
}
