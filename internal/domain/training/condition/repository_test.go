package condition_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/condition"
)

// Reader と Writer の両方を満たす最小実装。
type stubRepo struct{}

func (stubRepo) FindAll(context.Context) (condition.ConditionLog, error) {
	return condition.NewConditionLog(nil), nil
}
func (stubRepo) Save(context.Context, []condition.DailyCondition) error { return nil }

func TestRepository_KeepsItsShape(t *testing.T) {
	var r condition.Reader = stubRepo{}
	var w condition.Writer = stubRepo{}

	var (
		_ func(context.Context) (condition.ConditionLog, error)   = r.FindAll
		_ func(context.Context, []condition.DailyCondition) error = w.Save
	)
}

// 読みと書きが混ざらないこと。理由は setlog 側と同じ。
func TestRepository_ReadAndWriteStaySeparate(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"Reader", reflect.TypeOf((*condition.Reader)(nil)).Elem(), []string{"FindAll"}},
		{"Writer", reflect.TypeOf((*condition.Writer)(nil)).Elem(), []string{"Save"}},
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
