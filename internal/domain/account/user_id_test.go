package account_test

import (
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

func TestNewUserID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // 空なら不正
	}{
		{
			name: "正規の UUID を受け取る",
			in:   "8d5e743e-f1b0-4430-9998-89d313e89da8",
			want: "8d5e743e-f1b0-4430-9998-89d313e89da8",
		},
		{
			// 大文字のまま通すと、同じ人が別の UserID として扱われ、
			// 記録が2つに割れる。Postgres の uuid 型は小文字で返すので、
			// 揃える先は小文字。
			name: "大文字は小文字に揃える",
			in:   "8D5E743E-F1B0-4430-9998-89D313E89DA8",
			want: "8d5e743e-f1b0-4430-9998-89d313e89da8",
		},
		{
			name: "前後の空白は落とす",
			in:   "  8d5e743e-f1b0-4430-9998-89d313e89da8\n",
			want: "8d5e743e-f1b0-4430-9998-89d313e89da8",
		},
		{
			// これを通すと、ゼロ値と区別できない UserID が生まれる。
			name: "空は受け取らない",
			in:   "",
		},
		{
			name: "UUID でない文字列は受け取らない",
			in:   "yoshhyy",
		},
		{
			// 16進でない文字が混じる。長さだけを見ていると通る。
			name: "16進でない文字は受け取らない",
			in:   "8d5e743e-f1b0-4430-9998-89d313e89dzz",
		},
		{
			// これは16進の検査にも引っかかる（ずれた位置にハイフンが立つ）。
			name: "ハイフンの位置が違うものは受け取らない",
			in:   "8d5e743ef-1b0-4430-9998-89d313e89da8",
		},
		{
			// 上のケースだけでは「ハイフンがその位置にあること」を
			// 検査していない。ハイフンは16進ではないので、位置がずれれば
			// 16進の検査が先に捕まえてしまうため。長さだけ合った36桁の
			// 16進は、位置の検査を外すと通る。
			name: "ハイフンの無い36桁の16進は受け取らない",
			in:   "8d5e743ef1b044309998" + "89d313e89da8abcd",
		},
		{
			name: "ハイフンを抜いた32文字は受け取らない",
			in:   "8d5e743ef1b044309998" + "89d313e89da8",
		},
		{
			name: "短いものは受け取らない",
			in:   "8d5e743e-f1b0-4430-9998-89d313e89da",
		},
		{
			name: "長いものは受け取らない",
			in:   "8d5e743e-f1b0-4430-9998-89d313e89da88",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := account.NewUserID(c.in)

			if c.want == "" {
				if !errors.Is(err, account.ErrInvalidUserID) {
					t.Fatalf("エラーが %v。ErrInvalidUserID のはず", err)
				}
				if got != (account.UserID{}) {
					t.Errorf("不正な入力から %q が返った。ゼロ値のはず", got.String())
				}
				return
			}

			if err != nil {
				t.Fatalf("エラーが返った: %v", err)
			}
			if got.String() != c.want {
				t.Errorf("UserID が %q。%q のはず", got.String(), c.want)
			}
		})
	}
}

// 同じ文字列から作った UserID は == で等しいこと。
//
// リポジトリは「この UserID の行だけ」を返す責務を負う。比較の手段が
// 無いと、突合が文字列に戻り、正規化を通っていない値と比べられる。
func TestUserID_ComparesByValue(t *testing.T) {
	const raw = "8d5e743e-f1b0-4430-9998-89d313e89da8"

	a, err := account.NewUserID(raw)
	if err != nil {
		t.Fatalf("エラーが返った: %v", err)
	}
	b, err := account.NewUserID("8D5E743E-F1B0-4430-9998-89D313E89DA8")
	if err != nil {
		t.Fatalf("エラーが返った: %v", err)
	}

	if a != b {
		t.Errorf("大小だけが違う同じ UUID が等しくない（%q と %q）", a.String(), b.String())
	}

	other, err := account.NewUserID("00000000-0000-4000-8000-000000000000")
	if err != nil {
		t.Fatalf("エラーが返った: %v", err)
	}
	if a == other {
		t.Errorf("別の UUID が等しいと判定された")
	}
}
