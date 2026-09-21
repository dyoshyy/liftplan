package postgres_test

import (
	"context"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

func samplePrograms(t *testing.T) *program.Program {
	t.Helper()

	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("NewFrequency: %v", err)
	}
	target, err := program.NewWeeklyVolumeTarget(map[training.MuscleRegion]float64{
		training.ChestMid: 12, training.Quad: 12, training.Hamstring: 10,
	})
	if err != nil {
		t.Fatalf("NewWeeklyVolumeTarget: %v", err)
	}
	p, err := program.NewProgram(freq, target,
		[]exercise.ExerciseID{"bench", "squat", "deadlift"},
		[]exercise.ExerciseID{"bench", "squat"}, "bench")
	if err != nil {
		t.Fatalf("NewProgram: %v", err)
	}
	return p
}

// プログラムが往復すること。
//
// jsonb の列が3つ（週目標・選択・宣言）と分割で4つある。形を変えたときに
// 読み出し側だけ直し忘れると、保存はできるのに起動後に読めなくなる。
func TestProgramRepository_RoundTrips(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	repo := postgres.NewProgramRepository(pool)

	want := samplePrograms(t)
	if err := repo.Save(ctx, userA(t), want); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	got, err := postgres.NewProgramRepository(pool).Get(ctx, userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if got.Frequency().PerWeek() != 3 {
		t.Errorf("頻度が %d", got.Frequency().PerWeek())
	}
	if !slices.Equal(got.SelectedExercises(), want.SelectedExercises()) {
		t.Errorf("選択種目が %v", got.SelectedExercises())
	}
	if !slices.Equal(got.DeclaredExercises(), want.DeclaredExercises()) {
		t.Errorf("宣言種目が %v", got.DeclaredExercises())
	}
	if id, ok := got.FocusExercise(); !ok || id != "bench" {
		t.Errorf("重点種目が %v(%v)", id, ok)
	}
	if got.WeeklyTarget().Sets(training.ChestMid) != 12 {
		t.Errorf("週目標が %v", got.WeeklyTarget().Sets(training.ChestMid))
	}
	// 分割を設定していないので空。既存の行は NULL で読めること。
	if c := got.Cycle(); len(c) != 0 {
		t.Errorf("分割が %v。空のはず", c)
	}
}

// 分割の周期が順序ごと往復すること。
//
// 順序が周期そのものなので、並びが変わると別の設定になる。同じ分割の
// 繰り返しも保たれること。
func TestProgramRepository_RoundTripsTheSplitCycle(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	repo := postgres.NewProgramRepository(pool)

	upper, err := program.NewSplit("上半身",
		[]training.MuscleRegion{training.ChestMid, training.Lat})
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}
	lower, err := program.NewSplit("下半身",
		[]training.MuscleRegion{training.Quad, training.Hamstring})
	if err != nil {
		t.Fatalf("NewSplit: %v", err)
	}

	// 上・下・上。同じ分割が2度出る。
	with, err := samplePrograms(t).WithCycle([]program.Split{upper, lower, upper})
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}
	if err := repo.Save(ctx, userA(t), with); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	got, err := postgres.NewProgramRepository(pool).Get(ctx, userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	cycle := got.Cycle()
	if len(cycle) != 3 {
		t.Fatalf("周期が %d 日。3日のはず: %v", len(cycle), cycle)
	}
	for i, want := range []string{"上半身", "下半身", "上半身"} {
		if cycle[i].Name() != want {
			t.Errorf("%d 日目が %q。%q のはず", i, cycle[i].Name(), want)
		}
	}
	if want := []training.MuscleRegion{training.ChestMid, training.Lat}; !slices.Equal(cycle[0].Regions(), want) {
		t.Errorf("区分が %v。%v のはず", cycle[0].Regions(), want)
	}

	// 空を保存すれば分割なしに戻ること。列が NULL に戻る。
	cleared, err := got.WithCycle(nil)
	if err != nil {
		t.Fatalf("WithCycle(nil): %v", err)
	}
	if err := repo.Save(ctx, userA(t), cleared); err != nil {
		t.Fatalf("解除の保存に失敗: %v", err)
	}
	again, err := postgres.NewProgramRepository(pool).Get(ctx, userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if c := again.Cycle(); len(c) != 0 {
		t.Errorf("分割が残っている: %v", c)
	}
}
