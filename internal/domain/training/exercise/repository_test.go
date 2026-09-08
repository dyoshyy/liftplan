package exercise_test

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
)

// Reader を満たす最小実装。
type stubRepo struct{}

func (stubRepo) FindAll(context.Context) ([]*exercise.Exercise, error) { return nil, nil }

// インターフェースの形を両方向から固定する。
//
// スタブをインターフェースに代入するだけだと、メソッドを削っても
// スタブは依然として満たすのでコンパイルが通ってしまう。検出できるのは
// 追加とシグネチャ変更だけ。メソッド値を期待する関数型に取り出す向きが要る。
func TestReader_KeepsItsShape(t *testing.T) {
	var r exercise.Reader = stubRepo{}
	var _ func(context.Context) ([]*exercise.Exercise, error) = r.FindAll
}

// 種目マスタは実行時に書き換わらないので Writer を持たない。
// 書ける口が生えたら、それは「必要になってから作る」判断を通っていない。
func TestExercise_HasNoWriter(t *testing.T) {
	typ := reflect.TypeOf((*exercise.Reader)(nil)).Elem()

	var got []string
	for i := range typ.NumMethod() {
		got = append(got, typ.Method(i).Name)
	}
	if want := []string{"FindAll"}; !slices.Equal(got, want) {
		t.Errorf("メソッド集合が %v（期待 %v）", got, want)
	}
}
