package account_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// 4つの口を全て満たす最小実装。
type stubRepo struct{}

func (stubRepo) Find(context.Context, account.Provider, string) (*account.Account, error) {
	return nil, account.ErrAccountNotFound
}
func (stubRepo) Create(context.Context, *account.Account) error { return nil }

type stubSessionRepo struct{}

func (stubSessionRepo) Find(
	context.Context, account.TokenHash, time.Time,
) (*account.Session, error) {
	return nil, account.ErrSessionNotFound
}
func (stubSessionRepo) Create(context.Context, *account.Session) error  { return nil }
func (stubSessionRepo) Delete(context.Context, account.TokenHash) error { return nil }

// 口の形を両方向から固定する。
//
// 代入だけだと「スタブが満たす」ことしか言えず、引数や戻り値の形が
// 変わっても別のスタブで満たせてしまう。メソッド値を関数型に取り出すと、
// 形そのものが固定される。
//
// とくに Find が time.Time を取ることが効く。ここを落として
// time.Now() を中で呼ぶ形に戻すと、期限切れのテストが書けなくなる。
func TestRepository_KeepsItsShape(t *testing.T) {
	var (
		ar account.AccountReader = stubRepo{}
		aw account.AccountWriter = stubRepo{}
		sr account.SessionReader = stubSessionRepo{}
		sw account.SessionWriter = stubSessionRepo{}
	)

	var (
		_ func(context.Context, account.Provider, string) (*account.Account, error)     = ar.Find
		_ func(context.Context, *account.Account) error                                 = aw.Create
		_ func(context.Context, account.TokenHash, time.Time) (*account.Session, error) = sr.Find
		_ func(context.Context, *account.Session) error                                 = sw.Create
		_ func(context.Context, account.TokenHash) error                                = sw.Delete
	)
}

// 読みと書きが混ざらないこと。理由は setlog 側と同じ。
//
// Reader に Create が紛れ込むと、読むだけの経路が書ける口を持ってしまい、
// 分けた意味がそこで消える。代入で満たすことを確かめるだけでは検出
// できない（メソッドが増えてもスタブは満たし続ける）ので、メソッド集合
// そのものを固定する。
func TestRepository_ReadAndWriteStaySeparate(t *testing.T) {
	cases := []struct {
		name string
		typ  reflect.Type
		want []string
	}{
		{
			"AccountReader",
			reflect.TypeOf((*account.AccountReader)(nil)).Elem(),
			[]string{"Find"},
		},
		{
			"AccountWriter",
			reflect.TypeOf((*account.AccountWriter)(nil)).Elem(),
			[]string{"Create"},
		},
		{
			"SessionReader",
			reflect.TypeOf((*account.SessionReader)(nil)).Elem(),
			[]string{"Find"},
		},
		{
			"SessionWriter",
			reflect.TypeOf((*account.SessionWriter)(nil)).Elem(),
			[]string{"Create", "Delete"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for i := range c.typ.NumMethod() {
				got = append(got, c.typ.Method(i).Name)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("メソッド集合が %v（期待 %v）", got, c.want)
			}
		})
	}
}
