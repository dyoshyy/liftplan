// Package accounttest は account のリポジトリが満たすべき契約を、
// 実装に依らないテストとして持つ。
//
// インメモリと Postgres の2実装があり、差し替えられることが Onion の要。
// 契約を各実装のテストに書き写すと、片方だけ直したときに「同じ口なのに
// 答えが違う」状態が緑のまま残る。1本を両方から呼ぶ。
//
// ドメインの隣に置くのは、契約が口の仕様そのものだから。インフラ側に
// 置くと、口を変えた人が契約を直す場所を探すことになる。
package accounttest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// AccountRepo は読み書き両方を備えたアカウントの実装。
//
// 使う側は要る半分だけを受け取るのが原則だが、契約テストは両方を
// 使う（作ってから引く以外に、作ったことを観測する手段が無い）。
type AccountRepo interface {
	account.AccountReader
	account.AccountWriter
}

// SessionRepo は読み書き両方を備えたセッションの実装。
type SessionRepo interface {
	account.SessionReader
	account.SessionWriter
}

// Repos は1つのテストケースで使う実装の組。
type Repos struct {
	Accounts AccountRepo
	Sessions SessionRepo
}

// 契約テストで使う固定値。
var (
	// 基準時刻。time.Now() を使うと、期限の境界が「たまたま通る」になる。
	// Postgres の timestamptz はマイクロ秒までなので、ナノ秒は持たせない。
	at = time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	userA = mustUserID("11111111-1111-4111-8111-111111111111")
	userB = mustUserID("22222222-2222-4222-8222-222222222222")

	// 契約で使うアドレス。noEmail は「取れなかった」を表す正当な値。
	emailA  = account.NewEmail("gym@example.com")
	emailB  = account.NewEmail("other@example.com")
	noEmail = account.Email{}
)

func mustUserID(s string) account.UserID {
	id, err := account.NewUserID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// RunAccountContract はアカウントの口の契約を検査する。
//
// newRepos はケースごとに空の実装を返すこと（Postgres なら新しい
// スキーマ）。ケース間で状態が残ると、実行順で結果が変わる。
func RunAccountContract(t *testing.T, newRepos func(t *testing.T) Repos) {
	t.Helper()

	ctx := context.Background()

	// 作ったアカウントを、同じ (provider, subject) で引けること。
	//
	// ここで値まで見るのは、Find が常に nil を返す実装でも「存在しない
	// ものが引けない」側のケースは緑になるため。
	//
	// メールアドレスも見る。Postgres は読み戻しでアカウントを組み直す
	// （列から NewAccount へ渡し直す）ので、渡し忘れるとアドレスが
	// 黙って消える。アドレスから引くほうのテストは書き込みの経路しか
	// 通っておらず、読みの経路はここでしか守られない。
	t.Run("作ったものを引ける", func(t *testing.T) {
		repos := newRepos(t)
		a := mustAccount(t, account.GitHub(), "12345", userA, emailA)

		if err := repos.Accounts.Create(ctx, a); err != nil {
			t.Fatalf("作成に失敗: %v", err)
		}

		got, err := repos.Accounts.Find(ctx, account.GitHub(), "12345")
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if got.UserID() != userA {
			t.Errorf("利用者が %q。%q のはず", got.UserID(), userA)
		}
		if got.Provider() != account.GitHub() {
			t.Errorf("プロバイダが %q。github のはず", got.Provider())
		}
		if got.Subject() != "12345" {
			t.Errorf("識別子が %q。12345 のはず", got.Subject())
		}
		if got.Email() != emailA {
			t.Errorf("メールアドレスが %q。%q のはず", got.Email(), emailA)
		}
	})

	// 同じ (provider, subject) は2つ作れない。
	//
	// 守らないと、コールバックが同時に2回来たときに同じ人のアカウントが
	// 2つでき、ログインのたびに違う UserID になって記録が割れる。
	// 2件目が別の UserID を持つのは、上書きされていないことを見るため。
	t.Run("同じ組み合わせは2つ作れない", func(t *testing.T) {
		repos := newRepos(t)

		if err := repos.Accounts.Create(
			ctx, mustAccount(t, account.GitHub(), "12345", userA, noEmail)); err != nil {
			t.Fatalf("1件目の作成に失敗: %v", err)
		}

		err := repos.Accounts.Create(
			ctx, mustAccount(t, account.GitHub(), "12345", userB, noEmail))
		if !errors.Is(err, account.ErrAccountAlreadyExists) {
			t.Fatalf("2件目のエラーが %v。ErrAccountAlreadyExists のはず", err)
		}

		// 1件目が残っていること。上書きされていたら、本人の記録が
		// 新しい UserID 側に移って見えなくなる。
		got, err := repos.Accounts.Find(ctx, account.GitHub(), "12345")
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if got.UserID() != userA {
			t.Errorf("利用者が %q。上書きされている（%q のはず）", got.UserID(), userA)
		}
	})

	// プロバイダが違えば別のアカウント。
	//
	// 同じ人が GitHub と Google の両方で入っても別人になる、という
	// 決めごとがここ。同時に、引くときに provider を見ていることの検査に
	// なっている（subject だけで引くと、先に入っているほうが返る）。
	t.Run("プロバイダが違えば別のアカウント", func(t *testing.T) {
		repos := newRepos(t)

		if err := repos.Accounts.Create(
			ctx, mustAccount(t, account.GitHub(), "1", userA, noEmail)); err != nil {
			t.Fatalf("GitHub 側の作成に失敗: %v", err)
		}
		if err := repos.Accounts.Create(
			ctx, mustAccount(t, account.Google(), "1", userB, noEmail)); err != nil {
			t.Fatalf("Google 側の作成に失敗: %v", err)
		}

		cases := []struct {
			provider account.Provider
			want     account.UserID
		}{
			{account.GitHub(), userA},
			{account.Google(), userB},
		}
		for _, c := range cases {
			got, err := repos.Accounts.Find(ctx, c.provider, "1")
			if err != nil {
				t.Fatalf("%q の取得に失敗: %v", c.provider, err)
			}
			if got.UserID() != c.want {
				t.Errorf("%q の利用者が %q。%q のはず", c.provider, got.UserID(), c.want)
			}
		}
	})

	// 無いものを引いたら ErrAccountNotFound。
	//
	// 別のエラーで返すと、呼び出し側（SignIn）は「初回ログインだから
	// 作る」と「DBが落ちている」を区別できず、落ちている間に新しい
	// アカウントを作りにいく。
	t.Run("無いものを引くと見つからない", func(t *testing.T) {
		repos := newRepos(t)

		if err := repos.Accounts.Create(
			ctx, mustAccount(t, account.GitHub(), "12345", userA, noEmail)); err != nil {
			t.Fatalf("作成に失敗: %v", err)
		}

		cases := []struct {
			name     string
			provider account.Provider
			subject  string
		}{
			{"識別子が違う", account.GitHub(), "99999"},
			{"プロバイダが違う", account.Google(), "12345"},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got, err := repos.Accounts.Find(ctx, c.provider, c.subject)
				if !errors.Is(err, account.ErrAccountNotFound) {
					t.Fatalf("エラーが %v。ErrAccountNotFound のはず", err)
				}
				if got != nil {
					t.Errorf("アカウントが返った: %+v", got)
				}
			})
		}
	})

	runFindUserByEmailContract(t, newRepos)
	runFindByUserContract(t, newRepos)
}

// runFindByUserContract は利用者から、その人のアカウントを引く口の契約。
//
// 設定画面の「アカウント」に、どのアドレスでログインしているかを出すのに
// 使う。守るのは**他人のアカウントを返さないこと**。混ざると、他人の
// メールアドレスが画面に出る。
func runFindByUserContract(t *testing.T, newRepos func(t *testing.T) Repos) {
	t.Helper()

	ctx := context.Background()

	// 同じ人が GitHub と Google の両方で入っている形と、別の人が同じ
	// プロバイダにいる形を並べる。userB の行が返ったら他人が混ざっている。
	t.Run("その人のアカウントだけをプロバイダ順に返す", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos,
			mustAccount(t, account.Google(), "g-1", userA, emailA),
			mustAccount(t, account.GitHub(), "1", userB, emailB),
			mustAccount(t, account.GitHub(), "2", userA, emailA),
		)

		got, err := repos.Accounts.FindByUser(ctx, userA)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("%d 件返った。userA の2件のはず: %+v", len(got), got)
		}
		// 並びを決めておくのは、画面の表示が読み込むたびに入れ替わらないため。
		want := []struct {
			provider account.Provider
			subject  string
		}{{account.GitHub(), "2"}, {account.Google(), "g-1"}}
		for i, w := range want {
			if got[i].Provider() != w.provider || got[i].Subject() != w.subject {
				t.Errorf("%d 件目が %s/%s。%s/%s のはず",
					i, got[i].Provider(), got[i].Subject(), w.provider, w.subject)
			}
			if got[i].UserID() != userA {
				t.Errorf("%d 件目の利用者が %q。%q のはず", i, got[i].UserID(), userA)
			}
			if got[i].Email() != emailA {
				t.Errorf("%d 件目のアドレスが %q。%q のはず", i, got[i].Email(), emailA)
			}
		}
	})

	// アドレスを取っていないアカウントも返す。落とすと、ログインしている
	// のに「アカウントが無い」ように見える。
	t.Run("アドレスの無いアカウントも返す", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos, mustAccount(t, account.GitHub(), "1", userA, noEmail))

		got, err := repos.Accounts.FindByUser(ctx, userA)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("%d 件返った。1件のはず", len(got))
		}
		if !got[0].Email().IsZero() {
			t.Errorf("アドレスが %q。空のはず", got[0].Email())
		}
	})

	// アカウントを持たない利用者は、エラーではなく空で返す。開発用の
	// セッションはアカウントを通らずに入るので、この形になる。
	t.Run("アカウントが無ければ空", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos, mustAccount(t, account.GitHub(), "1", userB, emailB))

		got, err := repos.Accounts.FindByUser(ctx, userA)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("%d 件返った。空のはず: %+v", len(got), got)
		}
	})
}

// runFindUserByEmailContract はメールアドレスから利用者を引く口の契約。
//
// この口は「GitHub で入った人と Google で入った人が同じ人か」を判断する
// ための材料を返すだけで、結ぶ判断そのものは別に置く。ここで守るのは、
// **間違った材料を返さないこと**。
func runFindUserByEmailContract(t *testing.T, newRepos func(t *testing.T) Repos) {
	t.Helper()

	ctx := context.Background()

	// 保存したアドレスで、その利用者を引けること。
	//
	// 値まで見る。「引けた」だけを見ると、誰で引いても同じ利用者を
	// 返す実装が緑になる。
	t.Run("保存したアドレスで引ける", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos,
			mustAccount(t, account.GitHub(), "1", userA, emailA),
			mustAccount(t, account.Google(), "2", userB, emailB))

		cases := []struct {
			email account.Email
			want  account.UserID
		}{
			{emailA, userA},
			{emailB, userB},
		}
		for _, c := range cases {
			got, err := repos.Accounts.FindUserByEmail(ctx, c.email)
			if err != nil {
				t.Fatalf("%q の取得に失敗: %v", c.email, err)
			}
			if got != c.want {
				t.Errorf("%q の利用者が %q。%q のはず", c.email, got, c.want)
			}
		}
	})

	// 大文字小文字が違っても同じ人に当たること。
	//
	// GitHub が "Gym@Example.com"、Google が "gym@example.com" を返す
	// ことがある。揃えないと、同じアドレスなのに別人のままになる。
	// 保存する側と引く側の両方で確かめる。
	t.Run("大文字小文字が違っても引ける", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos,
			mustAccount(t, account.GitHub(), "1", userA,
				account.NewEmail("GYM@EXAMPLE.COM")))

		got, err := repos.Accounts.FindUserByEmail(ctx, account.NewEmail("gym@example.com"))
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if got != userA {
			t.Errorf("利用者が %q。%q のはず", got, userA)
		}
	})

	// 空のアドレスでは必ず見つからないこと。
	//
	// 空で全件（あるいはアドレスを持たない行）に当たると、アドレスを
	// 持たない利用者が全員同一人物になり、結ぶ判断が他人の記録へ
	// 結びつく。アドレスを持つ行と持たない行の両方を入れておくのは、
	// 空の保存先だと「常に見つからない」実装でも緑になるため。
	t.Run("空のアドレスでは引けない", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos,
			mustAccount(t, account.GitHub(), "1", userA, emailA),
			mustAccount(t, account.Google(), "2", userB, noEmail))

		got, err := repos.Accounts.FindUserByEmail(ctx, noEmail)
		if !errors.Is(err, account.ErrAccountNotFound) {
			t.Fatalf("エラーが %v。ErrAccountNotFound のはず", err)
		}
		if got != (account.UserID{}) {
			t.Errorf("利用者が返った: %q", got)
		}
	})

	// 知らないアドレスでは見つからないこと。
	//
	// アドレスを持たない行がここで返らないことも併せて見る。返ると、
	// 「まだアドレスを取っていない誰か」が問い合わせたアドレスの
	// 持ち主として扱われる。
	t.Run("知らないアドレスでは引けない", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos,
			mustAccount(t, account.GitHub(), "1", userA, emailA),
			mustAccount(t, account.Google(), "2", userB, noEmail))

		got, err := repos.Accounts.FindUserByEmail(ctx,
			account.NewEmail("nobody@example.com"))
		if !errors.Is(err, account.ErrAccountNotFound) {
			t.Fatalf("エラーが %v。ErrAccountNotFound のはず", err)
		}
		if got != (account.UserID{}) {
			t.Errorf("利用者が返った: %q", got)
		}
	})

	// 同じアドレスに別の利用者が2人いたら、どちらも返さないこと。
	//
	// 黙ってどちらかを選ぶと、選び方（保存順・索引の走査順）次第で
	// 他人の記録に結びつく。呼び出し側が「分からないので結ばない」と
	// 判断できるように、専用のエラーで返す。
	t.Run("同じアドレスに別の利用者が2人いたら曖昧", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos,
			mustAccount(t, account.GitHub(), "1", userA, emailA),
			mustAccount(t, account.Google(), "2", userB, emailA))

		got, err := repos.Accounts.FindUserByEmail(ctx, emailA)
		if !errors.Is(err, account.ErrAmbiguousEmail) {
			t.Fatalf("エラーが %v。ErrAmbiguousEmail のはず", err)
		}
		if got != (account.UserID{}) {
			t.Errorf("曖昧なのに利用者が返った: %q", got)
		}
	})

	// 同じ人が GitHub と Google の両方で入っていても、曖昧にしないこと。
	//
	// 一意制約を付けない理由がここ。同じアドレスの行が2つあるのは正常な
	// 形で、行数を数える実装だとこれが曖昧になり、結べるはずの2つが
	// 永久に結ばれない。数えるのは行ではなく利用者の種類。
	t.Run("同じ人が2つのプロバイダで入っていても引ける", func(t *testing.T) {
		repos := newRepos(t)
		createAll(t, repos,
			mustAccount(t, account.GitHub(), "1", userA, emailA),
			mustAccount(t, account.Google(), "2", userA, emailA))

		got, err := repos.Accounts.FindUserByEmail(ctx, emailA)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if got != userA {
			t.Errorf("利用者が %q。%q のはず", got, userA)
		}
	})
}

// createAll はアカウントをまとめて作る。
func createAll(t *testing.T, repos Repos, accounts ...*account.Account) {
	t.Helper()
	for _, a := range accounts {
		if err := repos.Accounts.Create(context.Background(), a); err != nil {
			t.Fatalf("%s/%s の作成に失敗: %v", a.Provider(), a.Subject(), err)
		}
	}
}

// RunSessionContract はセッションの口の契約を検査する。
func RunSessionContract(t *testing.T, newRepos func(t *testing.T) Repos) {
	t.Helper()

	ctx := context.Background()

	// 発行したセッションを、トークンのハッシュで引けること。
	//
	// 値まで見る。「引けた」だけを見ていると、誰のセッションでも
	// 同じ人を返す実装が緑になる。
	t.Run("発行したものを引ける", func(t *testing.T) {
		repos := newRepos(t)
		token, s := mustIssue(t, userA, at)

		if err := repos.Sessions.Create(ctx, s); err != nil {
			t.Fatalf("発行に失敗: %v", err)
		}

		got, err := repos.Sessions.Find(ctx, token.Hash(), at)
		if err != nil {
			t.Fatalf("取得に失敗: %v", err)
		}
		if got.UserID() != userA {
			t.Errorf("持ち主が %q。%q のはず", got.UserID(), userA)
		}
		if !got.ExpiresAt().Equal(s.ExpiresAt()) {
			t.Errorf("期限が %v。%v のはず", got.ExpiresAt(), s.ExpiresAt())
		}
		if got.TokenHash() != token.Hash() {
			t.Errorf("ハッシュが %q。%q のはず", got.TokenHash(), token.Hash())
		}
	})

	// 期限の境界。切れたセッションで他人の記録に入れないこと。
	//
	// 期限ちょうどは切れている扱い（Session.IsExpired と同じ規則）。
	// ここは実装ごとに書かず、ドメインの判定を通すこと。SQL の WHERE で
	// 判定すると規則が2箇所になり、片方だけ直したときに実装で答えが違う。
	t.Run("期限切れは引けない", func(t *testing.T) {
		expiresAt := at.Add(account.SessionLifetime)

		cases := []struct {
			name      string
			now       time.Time
			wantFound bool
		}{
			{name: "期限の1秒前は引ける", now: expiresAt.Add(-time.Second), wantFound: true},
			{name: "期限ちょうどは引けない", now: expiresAt},
			{name: "期限を過ぎたら引けない", now: expiresAt.Add(time.Second)},
		}

		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				repos := newRepos(t)
				token, s := mustIssue(t, userA, at)
				if err := repos.Sessions.Create(ctx, s); err != nil {
					t.Fatalf("発行に失敗: %v", err)
				}

				got, err := repos.Sessions.Find(ctx, token.Hash(), c.now)

				if !c.wantFound {
					if !errors.Is(err, account.ErrSessionNotFound) {
						t.Fatalf("エラーが %v。ErrSessionNotFound のはず", err)
					}
					if got != nil {
						t.Errorf("期限切れのセッションが返った: %+v", got)
					}
					return
				}
				if err != nil {
					t.Fatalf("取得に失敗: %v", err)
				}
				if got.UserID() != userA {
					t.Errorf("持ち主が %q。%q のはず", got.UserID(), userA)
				}
			})
		}
	})

	// 消したセッションは引けず、他のセッションは残ること。
	//
	// 2件作るのは、WHERE の無い DELETE を捕まえるため。1件だけだと
	// 「全部消す」実装でも緑になり、ログアウトのたびに全端末が落ちる。
	t.Run("消したものは引けない", func(t *testing.T) {
		repos := newRepos(t)

		tokenA, sessionA := mustIssue(t, userA, at)
		tokenB, sessionB := mustIssue(t, userB, at)
		for _, s := range []*account.Session{sessionA, sessionB} {
			if err := repos.Sessions.Create(ctx, s); err != nil {
				t.Fatalf("発行に失敗: %v", err)
			}
		}

		if err := repos.Sessions.Delete(ctx, tokenA.Hash()); err != nil {
			t.Fatalf("削除に失敗: %v", err)
		}

		got, err := repos.Sessions.Find(ctx, tokenA.Hash(), at)
		if !errors.Is(err, account.ErrSessionNotFound) {
			t.Fatalf("消したはずのセッションのエラーが %v。ErrSessionNotFound のはず", err)
		}
		if got != nil {
			t.Errorf("消したセッションが返った: %+v", got)
		}

		remaining, err := repos.Sessions.Find(ctx, tokenB.Hash(), at)
		if err != nil {
			t.Fatalf("消していないセッションの取得に失敗: %v", err)
		}
		if remaining.UserID() != userB {
			t.Errorf("持ち主が %q。%q のはず", remaining.UserID(), userB)
		}
		if !remaining.ExpiresAt().Equal(sessionB.ExpiresAt()) {
			t.Errorf("期限が %v。%v のはず", remaining.ExpiresAt(), sessionB.ExpiresAt())
		}
	})

	// 知らないハッシュでは何も返らない。
	t.Run("知らないハッシュでは引けない", func(t *testing.T) {
		repos := newRepos(t)

		// 1件は入れておく。空の保存先では、常に見つからない実装でも通る。
		_, s := mustIssue(t, userA, at)
		if err := repos.Sessions.Create(ctx, s); err != nil {
			t.Fatalf("発行に失敗: %v", err)
		}

		other, err := account.ParseSessionToken("誰も発行していないトークン")
		if err != nil {
			t.Fatalf("トークンを作れない: %v", err)
		}

		got, err := repos.Sessions.Find(ctx, other.Hash(), at)
		if !errors.Is(err, account.ErrSessionNotFound) {
			t.Fatalf("エラーが %v。ErrSessionNotFound のはず", err)
		}
		if got != nil {
			t.Errorf("セッションが返った: %+v", got)
		}
	})

	// 無いセッションの削除は成功として扱う。
	//
	// ログアウトの目的は「消えていること」で、既に消えているなら
	// 達成されている。エラーにすると、画面が期限切れのトークンで
	// ログアウトできない。
	t.Run("無いものの削除は成功", func(t *testing.T) {
		repos := newRepos(t)

		token, err := account.ParseSessionToken("発行していないトークン")
		if err != nil {
			t.Fatalf("トークンを作れない: %v", err)
		}
		if err := repos.Sessions.Delete(ctx, token.Hash()); err != nil {
			t.Errorf("削除がエラーになった: %v", err)
		}
	})
}

func mustAccount(
	t *testing.T, p account.Provider, subject string,
	uid account.UserID, email account.Email,
) *account.Account {
	t.Helper()
	a, err := account.NewAccount(p, subject, uid, email)
	if err != nil {
		t.Fatalf("アカウントを作れない: %v", err)
	}
	return a
}

func mustIssue(
	t *testing.T, uid account.UserID, now time.Time,
) (account.SessionToken, *account.Session) {
	t.Helper()
	token, s, err := account.IssueSession(uid, now)
	if err != nil {
		t.Fatalf("セッションを発行できない: %v", err)
	}
	return token, s
}
