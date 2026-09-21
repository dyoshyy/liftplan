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
	t.Run("作ったものを引ける", func(t *testing.T) {
		repos := newRepos(t)
		a := mustAccount(t, account.GitHub(), "12345", userA)

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
	})

	// 同じ (provider, subject) は2つ作れない。
	//
	// 守らないと、コールバックが同時に2回来たときに同じ人のアカウントが
	// 2つでき、ログインのたびに違う UserID になって記録が割れる。
	// 2件目が別の UserID を持つのは、上書きされていないことを見るため。
	t.Run("同じ組み合わせは2つ作れない", func(t *testing.T) {
		repos := newRepos(t)

		if err := repos.Accounts.Create(
			ctx, mustAccount(t, account.GitHub(), "12345", userA)); err != nil {
			t.Fatalf("1件目の作成に失敗: %v", err)
		}

		err := repos.Accounts.Create(
			ctx, mustAccount(t, account.GitHub(), "12345", userB))
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
			ctx, mustAccount(t, account.GitHub(), "1", userA)); err != nil {
			t.Fatalf("GitHub 側の作成に失敗: %v", err)
		}
		if err := repos.Accounts.Create(
			ctx, mustAccount(t, account.Google(), "1", userB)); err != nil {
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
			ctx, mustAccount(t, account.GitHub(), "12345", userA)); err != nil {
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
	t *testing.T, p account.Provider, subject string, uid account.UserID,
) *account.Account {
	t.Helper()
	a, err := account.NewAccount(p, subject, uid)
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
