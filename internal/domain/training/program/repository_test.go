package program_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/program"
)

// Reader と Writer の両方を満たす最小実装。
type stubRepo struct{}

func (stubRepo) Get(context.Context) (*program.Program, error) {
	return nil, program.ErrProgramNotConfigured
}
func (stubRepo) Save(context.Context, *program.Program) error { return nil }

func TestRepository_KeepsItsShape(t *testing.T) {
	var r program.Reader = stubRepo{}
	var w program.Writer = stubRepo{}

	var (
		_ func(context.Context) (*program.Program, error) = r.Get
		_ func(context.Context, *program.Program) error   = w.Save
	)
}

// 読みと書きが混ざらないこと。理由は setlog 側と同じ。
func TestRepository_ReadAndWriteStaySeparate(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{"Reader", reflect.TypeOf((*program.Reader)(nil)).Elem(), []string{"Get"}},
		{"Writer", reflect.TypeOf((*program.Writer)(nil)).Elem(), []string{"Save"}},
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

// Get は未設定のとき (nil, nil) を返してはならない。
// nil を「未設定」と「取得成功」のどちらとも解釈できてしまう。
func TestReader_ReportsMissingProgramAsAnError(t *testing.T) {
	p, err := stubRepo{}.Get(context.Background())
	if p == nil && err == nil {
		t.Fatal("未設定を (nil, nil) で返している")
	}
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("未設定が ErrProgramNotConfigured で表現されていない: %v", err)
	}
}
