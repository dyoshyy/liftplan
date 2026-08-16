package training_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// 各リポジトリインターフェースを満たす最小実装。
// インターフェースの形が壊れたらコンパイルで気づける。
type stubExerciseRepo struct{}

func (stubExerciseRepo) FindAll(context.Context) ([]*training.Exercise, error) { return nil, nil }

type stubSetLogRepo struct{}

func (stubSetLogRepo) FindAll(context.Context) (training.History, error) {
	return training.NewHistory(nil), nil
}
func (stubSetLogRepo) Save(context.Context, []*training.SetLog) error { return nil }

type stubConditionRepo struct{}

func (stubConditionRepo) FindAll(context.Context) (training.ConditionLog, error) {
	return training.NewConditionLog(nil), nil
}
func (stubConditionRepo) Save(context.Context, []training.DailyCondition) error { return nil }

type stubProgramRepo struct{}

func (stubProgramRepo) Get(context.Context) (*training.Program, error) {
	return nil, training.ErrProgramNotConfigured
}

func TestRepositoryInterfaces_AreSatisfiable(t *testing.T) {
	var _ training.ExerciseRepository = stubExerciseRepo{}
	var _ training.SetLogRepository = stubSetLogRepo{}
	var _ training.ConditionRepository = stubConditionRepo{}
	var _ training.ProgramRepository = stubProgramRepo{}
}

// 未設定は「エラー」ではなく状態なので、呼び出し側が errors.Is で
// 判別して初期設定へ誘導できること。文字列比較を強いてはいけない。
func TestErrProgramNotConfigured_IsIdentifiable(t *testing.T) {
	if training.ErrProgramNotConfigured == nil {
		t.Fatal("未設定エラーが定義されていない")
	}
	wrapped := fmt.Errorf("プログラムの取得: %w", training.ErrProgramNotConfigured)
	if !errors.Is(wrapped, training.ErrProgramNotConfigured) {
		t.Error("包んだあとに errors.Is で判別できない")
	}
	if training.ErrProgramNotConfigured.Error() == "" {
		t.Error("エラーの説明が空である")
	}
}
