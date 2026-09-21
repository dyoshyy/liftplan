package postgres_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/account/accounttest"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// インメモリ実装と同じ契約を Postgres でも回す。
//
// ケースごとに新しいスキーマを作る（migratedDB が t.Name() で分ける）。
// 共有すると、前のケースが残した行を次のケースが踏む。
func newAccountRepos(t *testing.T) accounttest.Repos {
	t.Helper()
	pool := migratedDB(t)
	return accounttest.Repos{
		Accounts: postgres.NewAccountRepository(pool),
		Sessions: postgres.NewSessionRepository(pool),
	}
}

func TestAccountRepository_Contract(t *testing.T) {
	accounttest.RunAccountContract(t, newAccountRepos)
}

func TestSessionRepository_Contract(t *testing.T) {
	accounttest.RunSessionContract(t, newAccountRepos)
}

// 再起動しても残ること。インメモリ実装との唯一の違いがここ。
//
// UserID が UUID として往復することも見る。uuid 列を経由すると表記が
// 変わりうる（大文字で書き戻すなど）ので、値そのものを確かめる。
// メールアドレスを同じ理由で見る。読み戻しでアカウントを組み直すので、
// 渡し忘れると黙って消える。
func TestAccountRepository_SurvivesReconnect(t *testing.T) {
	pool := migratedDB(t)
	ctx := context.Background()

	uid, err := account.NewUserID("8d5e743e-f1b0-4430-9998-89d313e89da8")
	if err != nil {
		t.Fatalf("UserID を作れない: %v", err)
	}
	email := account.NewEmail("gym@example.com")
	a, err := account.NewAccount(account.GitHub(), "12345", uid, email)
	if err != nil {
		t.Fatalf("アカウントを作れない: %v", err)
	}
	if err := postgres.NewAccountRepository(pool).Create(ctx, a); err != nil {
		t.Fatalf("作成に失敗: %v", err)
	}

	// 別のリポジトリインスタンスから読む。
	got, err := postgres.NewAccountRepository(pool).Find(ctx, account.GitHub(), "12345")
	if err != nil {
		t.Fatalf("取得に失敗: %v", err)
	}
	if got.UserID() != uid {
		t.Errorf("利用者が %q。%q のはず（UUID が往復していない）", got.UserID(), uid)
	}
	if got.Email() != email {
		t.Errorf("メールアドレスが %q。%q のはず（往復していない）", got.Email(), email)
	}
}
