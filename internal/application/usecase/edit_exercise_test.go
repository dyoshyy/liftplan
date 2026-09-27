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
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

func newEdit(t *testing.T) (*usecase.EditExercise, *exerciseRepo, account.UserID) {
	t.Helper()
	_, exercises, programs, user := newAdd(t)
	return usecase.NewEditExercise(exercises, programs), exercises, user
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
	edit := usecase.NewEditExercise(exercises, programs)

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
		edit := usecase.NewEditExercise(exercises, programs)

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

// declaredDayFixture は「宣言種目が1つの分割日にしか出られない」最小の
// プログラムを組む。SetSplitCycle の TestSetSplitCycle_PrimaryBoundary と
// 同じ形（自前の種目＋1日だけの周期）。シードだけで書くと、境界となる
// 区分をシードの都合で選ばざるを得ず、テストの意図がぼやける。
func declaredDayFixture(t *testing.T) (*exerciseRepo, *programRepo, account.UserID) {
	t.Helper()
	e, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "leg_focus", Name: "leg_focus",
		Stimulus:    map[training.MuscleRegion]float64{training.Hamstring: 1.0},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("種目の生成に失敗: %v", err)
	}
	exercises := newExerciseRepo([]*exercise.Exercise{e})

	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	ids := []exercise.ExerciseID{"leg_focus"}
	prog, err := program.NewProgram(freq, mustVolume(t, 6, 3), ids, ids, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	split, err := program.NewSplit("脚", []training.MuscleRegion{training.Hamstring})
	if err != nil {
		t.Fatalf("分割の生成に失敗: %v", err)
	}
	prog, err = prog.WithCycle([]program.Split{split})
	if err != nil {
		t.Fatalf("周期の設定に失敗: %v", err)
	}

	programs := newProgramRepo()
	if err := programs.Save(context.Background(), testUser, prog); err != nil {
		t.Fatalf("プログラムの保存に失敗: %v", err)
	}
	return exercises, programs, testUser
}

// 宣言種目の効き方を、どの分割日にも出られない形へ直そうとしたら弾かれる
// こと。SetDeclaredExercises・SetSplitCycle と同じ verifyDeclaredHaveADay
// で守る。守らないと split_lane.go の言う通り、その種目は保存はできても
// 二度と軸に出ない（他の宣言が毎日1つは該当するのでフォールバックも
// 発火せず、エラーも立たないまま消える）。
func TestEditExercise_RefusesToLeaveADeclaredExerciseWithoutADay(t *testing.T) {
	ctx := context.Background()
	exercises, programs, user := declaredDayFixture(t)
	edit := usecase.NewEditExercise(exercises, programs)

	_, err := edit.Execute(ctx, user, "leg_focus", usecase.EditExerciseInput{
		Name:        "leg_focus",
		Stimulus:    map[training.MuscleRegion]float64{training.Biceps: 1.0},
		IncrementKg: 2.5,
	})
	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("出られる日を失う編集が通った: %v", err)
	}

	stored := findExercise(t, exercises, user, "leg_focus")
	if _, ok := stored.Stimulus().Contribution(training.Hamstring); !ok {
		t.Error("弾いたはずなのに保存されてしまった")
	}
}

// 出られる日を保ったままの編集（刻みだけ変える）は通ること。
func TestEditExercise_AllowsEditingADeclaredExerciseThatKeepsADay(t *testing.T) {
	ctx := context.Background()
	exercises, programs, user := declaredDayFixture(t)
	edit := usecase.NewEditExercise(exercises, programs)

	got, err := edit.Execute(ctx, user, "leg_focus", usecase.EditExerciseInput{
		Name:        "leg_focus",
		Stimulus:    map[training.MuscleRegion]float64{training.Hamstring: 1.0, training.Glute: 0.5},
		IncrementKg: 5.0,
	})
	if err != nil {
		t.Fatalf("出られる日を保った編集が弾かれた: %v", err)
	}
	if got.Increment().Kg() != 5.0 {
		t.Errorf("刻みが変わっていない: %v", got.Increment().Kg())
	}
}

// プログラムが未設定のときは、出られる日の判定自体をスキップして直せる
// こと。Add と違い、Edit は使う種目に入れる操作ではないので、プログラム
// を前提にしない（設定前に種目のカタログだけ直す余地を残す）。
func TestEditExercise_SkipsTheDayCheckWhenNoProgramIsConfigured(t *testing.T) {
	ctx := context.Background()
	seedAll, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	exercises := newExerciseRepo(seedAll)
	programs := newProgramRepo()
	edit := usecase.NewEditExercise(exercises, programs)

	_, err = edit.Execute(ctx, testUser, "side_raise", usecase.EditExerciseInput{
		Name:        "サイドレイズ",
		Stimulus:    map[training.MuscleRegion]float64{training.Biceps: 1.0},
		IncrementKg: 1.0,
	})
	if err != nil {
		t.Errorf("プログラム未設定で直せない: %v", err)
	}
}
