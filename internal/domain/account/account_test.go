package account_test

import (
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

func TestNewProvider(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // 空なら不正
	}{
		{name: "github を受け取る", in: "github", want: "github"},
		{name: "google を受け取る", in: "google", want: "google"},
		{
			// プロバイダ名は DB の列にもそのまま入る。大文字のまま
			// 保存されると、同じ人が別のアカウントとして扱われる。
			name: "大文字は小文字に揃える", in: "GitHub", want: "github",
		},
		{name: "前後の空白は落とす", in: " google\n", want: "google"},
		{
			// 綴り違いを通すと、既存のアカウントに当たらず初回ログイン
			// 扱いになり、新しい UserID ができて記録が割れる。
			name: "知らないプロバイダは受け取らない", in: "gihtub",
		},
		{name: "空は受け取らない", in: ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := account.NewProvider(c.in)

			if c.want == "" {
				if !errors.Is(err, account.ErrUnknownProvider) {
					t.Fatalf("エラーが %v。ErrUnknownProvider のはず", err)
				}
				if got != (account.Provider{}) {
					t.Errorf("不正な入力から %q が返った。ゼロ値のはず", got.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("エラーが返った: %v", err)
			}
			if got.String() != c.want {
				t.Errorf("プロバイダが %q。%q のはず", got.String(), c.want)
			}
		})
	}
}

// 文字列から作ったプロバイダが、定数の返すものと == で等しいこと。
//
// ここが等しくないと、DB から読み戻したアカウントを GitHub() と
// 突き合わせられず、比較が文字列に戻る。
func TestProvider_ComparesByValue(t *testing.T) {
	got, err := account.NewProvider("GITHUB")
	if err != nil {
		t.Fatalf("エラーが返った: %v", err)
	}
	if got != account.GitHub() {
		t.Errorf("%q が GitHub() と等しくない", got.String())
	}
	if account.GitHub() == account.Google() {
		t.Error("GitHub と Google が等しいと判定された")
	}
}

func TestNewAccount(t *testing.T) {
	uid := account.DefaultUserID()

	cases := []struct {
		name        string
		provider    account.Provider
		subject     string
		userID      account.UserID
		wantSubject string // 空なら不正
	}{
		{
			name:     "プロバイダ・識別子・利用者が揃っていれば作れる",
			provider: account.GitHub(), subject: "12345", userID: uid,
			wantSubject: "12345",
		},
		{
			name:     "識別子の前後の空白は落とす",
			provider: account.Google(), subject: "  109\n", userID: uid,
			wantSubject: "109",
		},
		{
			// Google の sub は大小を区別する不透明な文字列。小文字に
			// 揃えると、別人の sub と衝突しうる。
			name:     "識別子の大小はそのまま保つ",
			provider: account.Google(), subject: "AbC123", userID: uid,
			wantSubject: "AbC123",
		},
		{
			// 空の subject を持つアカウントを作れると、subject を
			// 取れなかったプロバイダの応答が全員同じ人になる。
			name:     "識別子が空なら作れない",
			provider: account.GitHub(), subject: "", userID: uid,
		},
		{
			// 空白だけを通さないこと。除去を検証の後に回すと通る。
			name:     "識別子が空白だけなら作れない",
			provider: account.GitHub(), subject: "   ", userID: uid,
		},
		{
			name:    "プロバイダがゼロ値なら作れない",
			subject: "12345", userID: uid,
		},
		{
			// ゼロ値の UserID を許すと、誰のものでもないアカウントができ、
			// そのセッションで入った人は空の記録を見る。
			name:     "利用者の識別子がゼロ値なら作れない",
			provider: account.GitHub(), subject: "12345",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := account.NewAccount(c.provider, c.subject, c.userID)

			if c.wantSubject == "" {
				if !errors.Is(err, account.ErrInvalidAccount) {
					t.Fatalf("エラーが %v。ErrInvalidAccount のはず", err)
				}
				if got != nil {
					t.Errorf("不正な入力からアカウントが返った")
				}
				return
			}
			if err != nil {
				t.Fatalf("エラーが返った: %v", err)
			}
			if got.Subject() != c.wantSubject {
				t.Errorf("識別子が %q。%q のはず", got.Subject(), c.wantSubject)
			}
			if got.Provider() != c.provider {
				t.Errorf("プロバイダが %q。%q のはず", got.Provider(), c.provider)
			}
			if got.UserID() != c.userID {
				t.Errorf("利用者が %q。%q のはず", got.UserID(), c.userID)
			}
		})
	}
}
