package training_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// 各リポジトリインターフェースを満たす最小実装。
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
func (stubProgramRepo) Save(context.Context, *training.Program) error { return nil }

// インターフェースの形を両方向から固定する。
//
// スタブをインターフェースに代入するだけだと、メソッドを削っても
// スタブは依然として満たすのでコンパイルが通ってしまう。検出できるのは
// 追加とシグネチャ変更だけ。メソッド値を期待する関数型に取り出す向きが
// 要る。この2つを揃えて初めて形が固定される。
func TestRepositoryInterfaces_KeepTheirShape(t *testing.T) {
	var exercises training.ExerciseRepository = stubExerciseRepo{}
	var logs training.SetLogRepository = stubSetLogRepo{}
	var conditions training.ConditionRepository = stubConditionRepo{}
	var programs training.ProgramRepository = stubProgramRepo{}

	var (
		_ func(context.Context) ([]*training.Exercise, error)    = exercises.FindAll
		_ func(context.Context) (training.History, error)        = logs.FindAll
		_ func(context.Context, []*training.SetLog) error        = logs.Save
		_ func(context.Context) (training.ConditionLog, error)   = conditions.FindAll
		_ func(context.Context, []training.DailyCondition) error = conditions.Save
		_ func(context.Context) (*training.Program, error)       = programs.Get
		_ func(context.Context, *training.Program) error         = programs.Save
	)
}

// 未設定は状態なので、呼び出し側が errors.Is で判別して
// 初期設定へ誘導できること。文字列比較を強いてはいけない。
func TestSentinelErrors_AreIdentifiableAndDistinct(t *testing.T) {
	sentinels := map[string]error{
		"ErrProgramNotConfigured": training.ErrProgramNotConfigured,
		"ErrExerciseNotFound":     training.ErrExerciseNotFound,
		"ErrConflictingSetLog":    training.ErrConflictingSetLog,
	}

	for name, err := range sentinels {
		if err == nil {
			t.Errorf("%s が定義されていない", name)
			continue
		}
		if err.Error() == "" {
			t.Errorf("%s の説明が空である", name)
		}
	}

	// 取り違えて同じ値を割り当てると、呼び出し側が分岐できない。
	for aName, a := range sentinels {
		for bName, b := range sentinels {
			if aName >= bName {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("%s と %s が区別できない", aName, bName)
			}
		}
	}
}

// Get は未設定のとき (nil, nil) を返してはならない。
// nil を「未設定」と「取得成功」のどちらとも解釈できてしまう。
func TestProgramRepository_ReportsMissingProgramAsAnError(t *testing.T) {
	p, err := stubProgramRepo{}.Get(context.Background())
	if p == nil && err == nil {
		t.Fatal("未設定を (nil, nil) で返している")
	}
	if !errors.Is(err, training.ErrProgramNotConfigured) {
		t.Errorf("未設定が ErrProgramNotConfigured で表現されていない: %v", err)
	}
}
