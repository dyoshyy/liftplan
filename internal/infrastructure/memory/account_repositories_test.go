package memory_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account/accounttest"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
)

// 契約は accounttest に1本だけ置き、Postgres 実装と同じものを回す。
// 書き写すと、片方だけ直したときに「同じ口なのに答えが違う」が緑で残る。
func newRepos(t *testing.T) accounttest.Repos {
	t.Helper()
	return accounttest.Repos{
		Accounts: memory.NewAccountRepository(),
		Sessions: memory.NewSessionRepository(),
	}
}

func TestAccountRepository_Contract(t *testing.T) {
	accounttest.RunAccountContract(t, newRepos)
}

func TestSessionRepository_Contract(t *testing.T) {
	accounttest.RunSessionContract(t, newRepos)
}
