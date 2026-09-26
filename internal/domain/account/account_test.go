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
	uid := testUser

	cases := []struct {
		name        string
		provider    account.Provider
		subject     string
		userID      account.UserID
		email       account.Email
		wantSubject string // 空なら不正
		wantEmail   string
	}{
		{
			name:     "プロバイダ・識別子・利用者が揃っていれば作れる",
			provider: account.GitHub(), subject: "12345", userID: uid,
			wantSubject: "12345",
		},
		{
			// アドレスは置くだけで、同一性は (provider, subject) が決める。
			// ここで弾くと、アドレスを返さないプロバイダで入った人が
			// アカウントを作れなくなる。
			name:     "メールアドレスが空でも作れる",
			provider: account.GitHub(), subject: "12345", userID: uid,
			email:       account.NewEmail(""),
			wantSubject: "12345",
		},
		{
			// 正規化は NewEmail が済ませている。ここで素通しにしたり
			// もう一度かけたりしない。
			name:     "メールアドレスは NewEmail が整えた形のまま持つ",
			provider: account.GitHub(), subject: "12345", userID: uid,
			email:       account.NewEmail(" Gym@Example.COM "),
			wantSubject: "12345", wantEmail: "gym@example.com",
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
			got, err := account.NewAccount(c.provider, c.subject, c.userID, c.email)

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
			if got.Email().String() != c.wantEmail {
				t.Errorf("メールアドレスが %q。%q のはず", got.Email(), c.wantEmail)
			}
		})
	}
}

// 同じアドレスの表記揺れが、同じ Email になること。
//
// 揃わないと、GitHub が "Gym@Example.com"、Google が "gym@example.com" を
// 返したときに別のアドレスとして扱われ、同じ人だと分からない。逆に
// 正規化を引く側でもう一度かける実装に戻ると、規則が2箇所になる。
func TestNewEmail(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "そのままの形は変わらない", in: "gym@example.com", want: "gym@example.com"},
		{
			// 大文字のまま保存されると、小文字で引いたときに当たらない。
			name: "大文字は小文字に揃える", in: "Gym@Example.COM", want: "gym@example.com",
		},
		{name: "前後の空白は落とす", in: "  gym@example.com\n", want: "gym@example.com"},
		{
			// 取れなかったことを表す正当な値。不正にはしない。
			name: "空は空のまま", in: "", want: "",
		},
		{
			// 空白だけを非空として持つと、その行どうしが「同じアドレス」に
			// なる。除去を先にやれば空に落ちる。
			name: "空白だけは空になる", in: " \t ", want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := account.NewEmail(c.in)

			if got.String() != c.want {
				t.Errorf("メールアドレスが %q。%q のはず", got.String(), c.want)
			}
			if got.IsZero() != (c.want == "") {
				t.Errorf("IsZero が %v。%v のはず", got.IsZero(), c.want == "")
			}
		})
	}
}

// 同じアドレスから作った Email が == で等しいこと。
//
// 等しくないと、インメモリ実装が map の鍵に使えず、引く側が文字列に戻る。
func TestEmail_ComparesByValue(t *testing.T) {
	if account.NewEmail("Gym@Example.com") != account.NewEmail("gym@example.com") {
		t.Error("表記だけが違うアドレスが等しくない")
	}
	if account.NewEmail("gym@example.com") == account.NewEmail("other@example.com") {
		t.Error("違うアドレスが等しいと判定された")
	}
}
