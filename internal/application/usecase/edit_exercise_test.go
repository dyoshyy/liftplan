package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

func newEdit(t *testing.T) (*usecase.EditExercise, *exerciseRepo, account.UserID) {
	t.Helper()
	_, exercises, _, user := newAdd(t)
	return usecase.NewEditExercise(exercises), exercises, user
}

func findExercise(t *testing.T, exercises *exerciseRepo, user account.UserID, id exercise.ExerciseID) *exercise.Exercise {
	t.Helper()
	all, err := exercises.FindAll(context.Background(), user)
	if err != nil {
		t.Fatalf("種目の取得に失敗: %v", err)
	}
	for _, e := range all {
		if e.ID() == id {
			return e
		}
	}
	t.Fatalf("種目 %s が見つからない", id)
	return nil
}

func TestEditExercise_RenamesAndChangesStimulus(t *testing.T) {
	ctx := context.Background()
	edit, exercises, user := newEdit(t)

	in := usecase.EditExerciseInput{
		Name:        "サイドレイズ改",
		Stimulus:    map[training.MuscleRegion]float64{training.SideDelt: 1.0, training.FrontDelt: 0.5},
		IncrementKg: 1.5,
	}
	got, err := edit.Execute(ctx, user, "side_raise", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name() != "サイドレイズ改" {
		t.Errorf("名前が変わっていない: %q", got.Name())
	}
	if c, ok := got.Stimulus().Contribution(training.FrontDelt); !ok || c.Float() != 0.5 {
		t.Errorf("効き方が変わっていない: %v ok=%v", c, ok)
	}
	if got.Increment().Kg() != 1.5 {
		t.Errorf("刻みが変わっていない: %v", got.Increment().Kg())
	}

	stored := findExercise(t, exercises, user, "side_raise")
	if stored.Name() != "サイドレイズ改" {
		t.Error("直した結果が保存されていない")
	}
}

// プリセット相当の非対称な寄与（0.7・0.4 など）を持つ種目を、同じ寄与の
// まま名前だけ変えても寄与が保たれること。
func TestEditExercise_KeepsFineContributionsWhenOnlyRenaming(t *testing.T) {
	ctx := context.Background()
	edit, exercises, user := newEdit(t)

	before := findExercise(t, exercises, user, "barbell_row")
	stimulus := map[training.MuscleRegion]float64{}
	for _, r := range before.Stimulus().Regions() {
		c, _ := before.Stimulus().Contribution(r)
		stimulus[r] = c.Float()
	}

	got, err := edit.Execute(ctx, user, "barbell_row", usecase.EditExerciseInput{
		Name:        "バーベルロウ改",
		Stimulus:    stimulus,
		IncrementKg: before.Increment().Kg(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := got.Stimulus().Contribution(training.Lat); !ok || c.Float() != 0.7 {
		t.Errorf("Lat の寄与が保たれていない: %v ok=%v", c, ok)
	}
	if c, ok := got.Stimulus().Contribution(training.RearDelt); !ok || c.Float() != 0.4 {
		t.Errorf("RearDelt の寄与が保たれていない: %v ok=%v", c, ok)
	}
}

func TestEditExercise_RejectsNameOfAnotherAliveExercise(t *testing.T) {
	ctx := context.Background()
	edit, _, user := newEdit(t)

	_, err := edit.Execute(ctx, user, "side_raise", usecase.EditExerciseInput{
		Name:        "ベンチプレス",
		Stimulus:    map[training.MuscleRegion]float64{training.SideDelt: 1.0},
		IncrementKg: 1.0,
	})
	if !errors.Is(err, apperror.ErrDuplicateName) {
		t.Errorf("生きている他の種目と同名が通った: %v", err)
	}
}

// 名前をそのままに刻みだけ変えても、自分自身との重複にはならないこと。
func TestEditExercise_AllowsItsOwnName(t *testing.T) {
	ctx := context.Background()
	edit, _, user := newEdit(t)

	got, err := edit.Execute(ctx, user, "side_raise", usecase.EditExerciseInput{
		Name:        "サイドレイズ",
		Stimulus:    map[training.MuscleRegion]float64{training.SideDelt: 1.0},
		IncrementKg: 2.0,
	})
	if err != nil {
		t.Fatalf("自分自身の名前で弾かれた: %v", err)
	}
	if got.Increment().Kg() != 2.0 {
		t.Errorf("刻みが変わっていない: %v", got.Increment().Kg())
	}
}

func TestEditExercise_AllowsNameOfADeletedExercise(t *testing.T) {
	ctx := context.Background()
	add, exercises, programs, user := newAdd(t)
	del := usecase.NewDeleteExercise(exercises, programs, programs)
	edit := usecase.NewEditExercise(exercises)

	e, err := add.Execute(ctx, user, isoRow())
	if err != nil {
		t.Fatalf("足すのに失敗: %v", err)
	}
	if err := del.Execute(ctx, user, e.ID()); err != nil {
		t.Fatalf("削除に失敗: %v", err)
	}

	_, err = edit.Execute(ctx, user, "side_raise", usecase.EditExerciseInput{
		Name:        isoRow().Name,
		Stimulus:    map[training.MuscleRegion]float64{training.SideDelt: 1.0},
		IncrementKg: 1.0,
	})
	if err != nil {
		t.Errorf("消した種目と同名が弾かれた: %v", err)
	}
}

func TestEditExercise_NotFound(t *testing.T) {
	ctx := context.Background()
	cases := map[string]exercise.ExerciseID{
		"無い ID": "u-ffffffffffffffff",
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			edit, _, user := newEdit(t)
			_, err := edit.Execute(ctx, user, id, usecase.EditExerciseInput{
				Name:        "何か",
				Stimulus:    map[training.MuscleRegion]float64{training.SideDelt: 1.0},
				IncrementKg: 1.0,
			})
			if !errors.Is(err, apperror.ErrExerciseNotFound) {
				t.Errorf("404 にならない: %v", err)
			}
		})
	}

	t.Run("消した種目", func(t *testing.T) {
		add, exercises, programs, user := newAdd(t)
		del := usecase.NewDeleteExercise(exercises, programs, programs)
		edit := usecase.NewEditExercise(exercises)

		e, err := add.Execute(ctx, user, isoRow())
		if err != nil {
			t.Fatalf("足すのに失敗: %v", err)
		}
		if err := del.Execute(ctx, user, e.ID()); err != nil {
			t.Fatalf("削除に失敗: %v", err)
		}

		_, err = edit.Execute(ctx, user, e.ID(), usecase.EditExerciseInput{
			Name:        "別の名前",
			Stimulus:    map[training.MuscleRegion]float64{training.Lat: 1.0},
			IncrementKg: 1.0,
		})
		if !errors.Is(err, apperror.ErrExerciseNotFound) {
			t.Errorf("消した種目が 404 にならない: %v", err)
		}
	})
}
