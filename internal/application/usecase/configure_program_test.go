package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

func configureInput(t *testing.T) usecase.ConfigureProgramInput {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, err := training.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	sets := map[training.MuscleRegion]float64{}
	for _, r := range target.Regions() {
		sets[r] = target.Sets(r)
	}
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}
	return usecase.ConfigureProgramInput{PerWeek: 3, Target: sets, Selected: selected}
}

func newConfigure(t *testing.T, programs *fakeProgram) *usecase.ConfigureProgram {
	t.Helper()
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	return usecase.NewConfigureProgram(fakeExercises{all: pool}, programs)
}

func TestConfigureProgram_SavesTheProgram(t *testing.T) {
	programs := &fakeProgram{}
	if err := newConfigure(t, programs).Execute(context.Background(), configureInput(t)); err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if programs.saved == nil {
		t.Fatal("プログラムが保存されていない")
	}
	if got := programs.saved.Frequency().PerWeek(); got != 3 {
		t.Errorf("頻度が保存されていない: %d", got)
	}
	if !programs.saved.Includes("bench") {
		t.Error("選択した種目が保存されていない")
	}
}

// 種目マスタに無いIDを黙って受け入れてはいけない。
// 受け入れると SessionPlanner が黙って落とし、ユーザーが選んだ種目が
// 理由の説明なくメニューから消える。
func TestConfigureProgram_RejectsUnknownExercise(t *testing.T) {
	programs := &fakeProgram{}
	in := configureInput(t)
	in.Selected = append(in.Selected, "存在しない種目")

	err := newConfigure(t, programs).Execute(context.Background(), in)
	if !errors.Is(err, training.ErrExerciseNotFound) {
		t.Errorf("未知の種目が弾かれていない: %v", err)
	}
	if programs.saved != nil {
		t.Error("検証に失敗したのに保存された")
	}
}

func TestConfigureProgram_RejectsInvalidFrequency(t *testing.T) {
	programs := &fakeProgram{}
	in := configureInput(t)
	in.PerWeek = 7

	if err := newConfigure(t, programs).Execute(context.Background(), in); err == nil {
		t.Error("範囲外の頻度が通った")
	}
	if programs.saved != nil {
		t.Error("検証に失敗したのに保存された")
	}
}

func TestConfigureProgram_PropagatesSaveError(t *testing.T) {
	boom := errors.New("書けない")
	programs := &fakeProgram{err: boom}

	if err := newConfigure(t, programs).Execute(context.Background(), configureInput(t)); !errors.Is(err, boom) {
		t.Errorf("エラーが伝播していない: %v", err)
	}
}

// 種目の取得に失敗したら、検証できないので保存もしない。
func TestConfigureProgram_DoesNotSaveWhenExercisesAreUnavailable(t *testing.T) {
	boom := errors.New("読めない")
	programs := &fakeProgram{}
	uc := usecase.NewConfigureProgram(fakeExercises{err: boom}, programs)

	if err := uc.Execute(context.Background(), configureInput(t)); !errors.Is(err, boom) {
		t.Errorf("エラーが伝播していない: %v", err)
	}
	if programs.saved != nil {
		t.Error("種目マスタが読めないのに保存された")
	}
}
