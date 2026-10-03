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
	p, err := program.NewProgram(freq, mustVolume(t, 6, 3),
		[]exercise.ExerciseID{"bench", "squat", "deadlift"},
		[]exercise.ExerciseID{"bench", "squat"}, "bench")
	if err != nil {
		t.Fatalf("NewProgram: %v", err)
	}
	return p
}

// プログラムが往復すること。
//
// jsonb の列が2つ（選択・宣言）と分割・レップ数で4つある。形を変えたときに
// 読み出し側だけ直し忘れると、保存はできるのに起動後に読めなくなる。
// 週目標はもう保存しない（#176）。
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
	// 分割を設定していないので空。既存の行は NULL で読めること。
	if c := got.Cycle(); len(c) != 0 {
		t.Errorf("分割が %v。空のはず", c)
	}
	// レップ数を設定していないので、どの宣言も既定。
	if got := got.RepTargetsFor("bench"); got != program.DefaultRepTargets() {
		t.Errorf("bench のレップ数が (%d, %d)。既定のはず", got.Heavy(), got.Light())
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

// 宣言ごとのレップ数が往復すること。設定していない宣言は既定のまま。
func TestProgramRepository_RoundTripsTheRepTargets(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	repo := postgres.NewProgramRepository(pool)

	reps, err := program.NewRepTargets(8, 12)
	if err != nil {
		t.Fatalf("NewRepTargets: %v", err)
	}
	with, err := samplePrograms(t).WithRepTargets("squat", reps)
	if err != nil {
		t.Fatalf("WithRepTargets: %v", err)
	}
	if err := repo.Save(ctx, userA(t), with); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}

	got, err := postgres.NewProgramRepository(pool).Get(ctx, userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if r := got.RepTargetsFor("squat"); r.Heavy() != 8 || r.Light() != 12 {
		t.Errorf("squat が (%d, %d)。(8, 12) のはず", r.Heavy(), r.Light())
	}
	if r := got.RepTargetsFor("bench"); r != program.DefaultRepTargets() {
		t.Errorf("bench が (%d, %d)。既定のはず", r.Heavy(), r.Light())
	}

	// 既存の行への保存でも反映されること（ON CONFLICT 側）。設定は初回の
	// INSERT より、あとから変える方が多い。
	again, err := program.NewRepTargets(5, 10)
	if err != nil {
		t.Fatalf("NewRepTargets: %v", err)
	}
	changed, err := got.WithRepTargets("squat", again)
	if err != nil {
		t.Fatalf("WithRepTargets: %v", err)
	}
	if err := repo.Save(ctx, userA(t), changed); err != nil {
		t.Fatalf("上書きの保存に失敗: %v", err)
	}
	got, err = postgres.NewProgramRepository(pool).Get(ctx, userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if r := got.RepTargetsFor("squat"); r.Heavy() != 5 || r.Light() != 10 {
		t.Errorf("上書き後の squat が (%d, %d)。(5, 10) のはず", r.Heavy(), r.Light())
	}

	// 設定の無いプログラムを保存すれば、列は NULL に戻り既定で読めること。
	if err := repo.Save(ctx, userA(t), samplePrograms(t)); err != nil {
		t.Fatalf("解除の保存に失敗: %v", err)
	}
	got, err = postgres.NewProgramRepository(pool).Get(ctx, userA(t))
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if r := got.RepTargetsFor("squat"); r != program.DefaultRepTargets() {
		t.Errorf("解除後の squat が (%d, %d)。既定のはず", r.Heavy(), r.Light())
	}
}

// 保存された値が範囲外なら、読み出しで弾く。jsonb は形を検査しないので、
// 読み出しが唯一の防波堤になる。
func TestProgramRepository_RejectsStoredRepTargetsOutOfRange(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()
	repo := postgres.NewProgramRepository(pool)

	if err := repo.Save(ctx, userA(t), samplePrograms(t)); err != nil {
		t.Fatalf("保存に失敗: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE program SET declared_reps = '{"bench": {"heavy": 99, "light": 6}}'::jsonb WHERE user_id = $1`,
		userA(t).String()); err != nil {
		t.Fatalf("書き換えに失敗: %v", err)
	}
	if _, err := repo.Get(ctx, userA(t)); err == nil {
		t.Error("範囲外の 99 が読めた")
	}
}

// mustVolume はテスト用の1回の量。
func mustVolume(t *testing.T, exercises, sets int) program.SessionVolume {
	t.Helper()
	v, err := program.NewSessionVolume(exercises, sets)
	if err != nil {
		t.Fatalf("NewSessionVolume(%d, %d): %v", exercises, sets, err)
	}
	return v
}
