package training_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
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
func (stubSetLogRepo) Save(context.Context, []*training.SetLog) error  { return nil }
func (stubSetLogRepo) Delete(context.Context, training.SetLogID) error { return nil }

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
	var exercises training.ExerciseReader = stubExerciseRepo{}
	var logsR training.SetLogReader = stubSetLogRepo{}
	var logsW training.SetLogWriter = stubSetLogRepo{}
	var conditionsR training.ConditionReader = stubConditionRepo{}
	var conditionsW training.ConditionWriter = stubConditionRepo{}
	var programsR training.ProgramReader = stubProgramRepo{}
	var programsW training.ProgramWriter = stubProgramRepo{}

	var (
		_ func(context.Context) ([]*training.Exercise, error)    = exercises.FindAll
		_ func(context.Context) (training.History, error)        = logsR.FindAll
		_ func(context.Context, []*training.SetLog) error        = logsW.Save
		_ func(context.Context, training.SetLogID) error         = logsW.Delete
		_ func(context.Context) (training.ConditionLog, error)   = conditionsR.FindAll
		_ func(context.Context, []training.DailyCondition) error = conditionsW.Save
		_ func(context.Context) (*training.Program, error)       = programsR.Get
		_ func(context.Context, *training.Program) error         = programsW.Save
	)
}

// 読みと書きが別のインターフェースであること。
//
// Reader に書き込みメソッドが紛れ込むと、読むだけの経路が書ける口を
// 持ってしまい、分けた意味がそこで消える。代入で満たすことを確かめる
// だけでは検出できない（メソッドが増えてもスタブは満たし続ける）ので、
// メソッド集合そのものを固定する。
func TestRepositoryInterfaces_ReadAndWriteStaySeparate(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"ExerciseReader", reflect.TypeOf((*training.ExerciseReader)(nil)).Elem(), []string{"FindAll"}},
		{"SetLogReader", reflect.TypeOf((*training.SetLogReader)(nil)).Elem(), []string{"FindAll"}},
		{"SetLogWriter", reflect.TypeOf((*training.SetLogWriter)(nil)).Elem(), []string{"Delete", "Save"}},
		{"ConditionReader", reflect.TypeOf((*training.ConditionReader)(nil)).Elem(), []string{"FindAll"}},
		{"ConditionWriter", reflect.TypeOf((*training.ConditionWriter)(nil)).Elem(), []string{"Save"}},
		{"ProgramReader", reflect.TypeOf((*training.ProgramReader)(nil)).Elem(), []string{"Get"}},
		{"ProgramWriter", reflect.TypeOf((*training.ProgramWriter)(nil)).Elem(), []string{"Save"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for i := range tc.typ.NumMethod() {
				got = append(got, tc.typ.Method(i).Name)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("メソッド集合が %v（期待 %v）", got, tc.want)
			}
		})
	}
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
