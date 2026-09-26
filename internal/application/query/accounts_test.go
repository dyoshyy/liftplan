package query_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// stubAccounts は FindByUser だけを持つ読み口。頼まれた利用者を覚えておき、
// 別の利用者を引いていないかを見られるようにする。
type stubAccounts struct {
	account.AccountReader
	byUser map[account.UserID][]*account.Account
	asked  []account.UserID
}

func (s *stubAccounts) FindByUser(_ context.Context, u account.UserID) ([]*account.Account, error) {
	s.asked = append(s.asked, u)
	return s.byUser[u], nil
}

func mustAccount(t *testing.T, p account.Provider, subject string, u account.UserID, email string) *account.Account {
	t.Helper()
	a, err := account.NewAccount(p, subject, u, account.NewEmail(email))
	if err != nil {
		t.Fatalf("アカウントを作れない: %v", err)
	}
	return a
}

// 設定画面の「アカウント」に出す材料。ログイン方法ごとにプロバイダと
// アドレスを返す。重複を畳むか、空をどう見せるかは画面の判断なので、
// ここは事実をそのまま渡す。
func TestAccounts_Of(t *testing.T) {
	other := mustTestUserID("22222222-2222-4222-8222-222222222222")
	repo := &stubAccounts{byUser: map[account.UserID][]*account.Account{
		testUser: {
			mustAccount(t, account.GitHub(), "1", testUser, "Gym@Example.com"),
			mustAccount(t, account.Google(), "g", testUser, ""),
		},
		other: {mustAccount(t, account.GitHub(), "2", other, "other@example.com")},
	}}

	got, err := query.NewAccounts(repo).Of(context.Background(), testUser)
	if err != nil {
		t.Fatalf("読み取りに失敗: %v", err)
	}

	want := []query.Login{
		{Provider: "github", Email: "gym@example.com"},
		{Provider: "google", Email: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("%d 件返った。%d 件のはず: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d 件目が %+v。%+v のはず", i, got[i], want[i])
		}
	}
	if len(repo.asked) != 1 || repo.asked[0] != testUser {
		t.Errorf("引いた利用者が %v。%v だけのはず", repo.asked, testUser)
	}
}
