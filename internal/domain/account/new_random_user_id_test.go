package account_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// 採番した UserID が UserID として成立していること。
//
// NewRandomUserID が NewUserID を通らない形（ハイフンの位置が違う、
// 大文字が混じる）を返すと、Postgres の uuid 列には入るのに、読み戻して
// NewUserID にかけたところで落ちる。**書けるのに読めない利用者**ができ、
// 本人には「ログインはできたが毎回エラーになる」として出る。
func TestNewRandomUserID_IsAValidUserID(t *testing.T) {
	id, err := account.NewRandomUserID()
	if err != nil {
		t.Fatalf("採番に失敗: %v", err)
	}
	if id == (account.UserID{}) {
		t.Fatal("ゼロ値が返っている")
	}

	// 文字列にして読み戻せること。ここが通らなければ、DB から読み戻す
	// 経路（NewUserID）を通らない値を作っていることになる。
	back, err := account.NewUserID(id.String())
	if err != nil {
		t.Fatalf("採番した %q を読み戻せない: %v", id.String(), err)
	}
	if back != id {
		t.Errorf("読み戻すと別の値になる: %q → %q", id.String(), back.String())
	}
}

// 採番するたびに違う値になること。
//
// 同じ値を返すと、2人目以降が1人目の記録をそのまま見る。値オブジェクトの
// 形だけ正しくても意味が無いので、ここは「形」ではなく「重複しないこと」を
// 検査する。
func TestNewRandomUserID_DoesNotRepeat(t *testing.T) {
	const n = 100

	seen := make(map[account.UserID]struct{}, n)
	for i := range n {
		id, err := account.NewRandomUserID()
		if err != nil {
			t.Fatalf("%d回目の採番に失敗: %v", i, err)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("%d回目で %q が重複した", i, id.String())
		}
		seen[id] = struct{}{}
	}
}

// UUID version 4 の印が入っていること。
//
// 印そのものに機能は無い。効くのは「乱数をそのまま16進で並べただけの値」と
// 区別が付くこと。version と variant のビットを立てていない実装は、
// uuid 型を持つ他のツール（Neon のコンソール、将来のライブラリ）から見ると
// 不正な UUID で、そこで初めて気づくことになる。
//
// 13文字目が '4'、17文字目が 8/9/a/b。
func TestNewRandomUserID_MarksVersionAndVariant(t *testing.T) {
	const (
		versionAt = 14 // "xxxxxxxx-xxxx-4..." の '4' の位置
		variantAt = 19
	)

	for range 20 {
		id, err := account.NewRandomUserID()
		if err != nil {
			t.Fatalf("採番に失敗: %v", err)
		}
		s := id.String()
		if s[versionAt] != '4' {
			t.Errorf("%q のバージョンが 4 ではない: %q", s, s[versionAt])
		}
		switch s[variantAt] {
		case '8', '9', 'a', 'b':
		default:
			t.Errorf("%q のバリアントが RFC 4122 ではない: %q", s, s[variantAt])
		}
	}
}
