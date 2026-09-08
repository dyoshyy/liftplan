package setlog_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

// Reader と Writer の両方を満たす最小実装。
type stubRepo struct{}

func (stubRepo) FindAll(context.Context) (setlog.History, error) {
	return setlog.NewHistory(nil), nil
}
func (stubRepo) Save(context.Context, []*setlog.SetLog) error  { return nil }
func (stubRepo) Delete(context.Context, setlog.SetLogID) error { return nil }

func TestRepository_KeepsItsShape(t *testing.T) {
	var r setlog.Reader = stubRepo{}
	var w setlog.Writer = stubRepo{}

	var (
		_ func(context.Context) (setlog.History, error) = r.FindAll
		_ func(context.Context, []*setlog.SetLog) error = w.Save
		_ func(context.Context, setlog.SetLogID) error  = w.Delete
	)
}

// 読みと書きが混ざらないこと。
//
// Reader に Save が紛れ込むと、読むだけの経路が書ける口を持ってしまい、
// 分けた意味がそこで消える。代入で満たすことを確かめるだけでは
// 検出できない（メソッドが増えてもスタブは満たし続ける）ので、
// メソッド集合そのものを固定する。
func TestRepository_ReadAndWriteStaySeparate(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"Reader", reflect.TypeOf((*setlog.Reader)(nil)).Elem(), []string{"FindAll"}},
		{"Writer", reflect.TypeOf((*setlog.Writer)(nil)).Elem(), []string{"Delete", "Save"}},
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
