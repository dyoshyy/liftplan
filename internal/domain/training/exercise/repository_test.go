package exercise_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// Reader を満たす最小実装。
type stubRepo struct{}

func (stubRepo) FindAll(context.Context, account.UserID) ([]*exercise.Exercise, error) {
	return nil, nil
}

// インターフェースの形を両方向から固定する。
//
// スタブをインターフェースに代入するだけだと、メソッドを削っても
// スタブは依然として満たすのでコンパイルが通ってしまう。検出できるのは
// 追加とシグネチャ変更だけ。メソッド値を期待する関数型に取り出す向きが要る。
func TestReader_KeepsItsShape(t *testing.T) {
	var r exercise.Reader = stubRepo{}
	var _ func(context.Context, account.UserID) ([]*exercise.Exercise, error) = r.FindAll
}

// Writer を満たす最小実装。
type stubWriter struct{}

func (stubWriter) Save(context.Context, account.UserID, *exercise.Exercise) error { return nil }

func TestWriter_KeepsItsShape(t *testing.T) {
	var w exercise.Writer = stubWriter{}
	var _ func(context.Context, account.UserID, *exercise.Exercise) error = w.Save
}

// 読みと書きは分けたまま。1つの口にまとめると、使う側が要らない半分まで
// 受け取る（CLAUDE.md「リポジトリのインターフェースは読みと書きに分ける」）。
func TestReaderAndWriter_StaySplit(t *testing.T) {
	for typ, want := range map[reflect.Type][]string{
		reflect.TypeOf((*exercise.Reader)(nil)).Elem(): {"FindAll"},
		reflect.TypeOf((*exercise.Writer)(nil)).Elem(): {"Save"},
	} {
		var got []string
		for i := range typ.NumMethod() {
			got = append(got, typ.Method(i).Name)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s のメソッド集合が %v（期待 %v）", typ, got, want)
		}
	}
}
