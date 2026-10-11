package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

func newGetSubstitute(t *testing.T, prog *fakeProgram) *usecase.GetSubstitute {
	t.Helper()
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	return usecase.NewGetSubstitute(
		&fakeExercises{all: pool},
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		prog,
		planning.DefaultSessionPlanner(),
	)
}

// 予定に無い種目は入力の誤り（400）に分類される。ドメインの番兵も辿れること。
//
// 【変異】classify.go の ErrNotPlannedAccessory の case を消す → 500 相当の
// 素のエラーのままになり ErrInvalidInput が辿れず赤。
func TestGetSubstitute_NotPlannedIsInvalidInput(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSubstitute(t, &fakeProgram{program: buildProgram(t, pool)})

	_, _, err := uc.Execute(context.Background(), testUser, usecase.GetSubstituteInput{
		Date: testDate, Replace: "no_such_exercise",
	})
	if !errors.Is(err, apperror.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
	if !errors.Is(err, planning.ErrNotPlannedAccessory) {
		t.Errorf("err = %v, ドメインの番兵が辿れない（%%w で包んでいない）", err)
	}
}

func TestGetSubstitute_PropagatesProgramNotConfigured(t *testing.T) {
	uc := newGetSubstitute(t, &fakeProgram{err: program.ErrProgramNotConfigured})
	_, _, err := uc.Execute(context.Background(), testUser, usecase.GetSubstituteInput{
		Date: testDate, Replace: "leg_curl",
	})
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("未設定エラーが伝播していない: %v", err)
	}
}

// リポジトリが契約に反して (nil, nil) を返しても未設定として扱う
// （GetForecast の同名テストと同じ理由。コピー元と同じ穴を踏みやすい）。
func TestGetSubstitute_TreatsNilProgramAsNotConfigured(t *testing.T) {
	uc := newGetSubstitute(t, &fakeProgram{})
	_, _, err := uc.Execute(context.Background(), testUser, usecase.GetSubstituteInput{
		Date: testDate, Replace: "leg_curl",
	})
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("nil のプログラムが未設定として扱われていない: %v", err)
	}
}

func TestGetSubstitute_RejectsZeroDate(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSubstitute(t, &fakeProgram{program: buildProgram(t, pool)})
	if _, _, err := uc.Execute(context.Background(), testUser, usecase.GetSubstituteInput{Replace: "leg_curl"}); err == nil {
		t.Error("日付無しが通ってしまう")
	}
}
