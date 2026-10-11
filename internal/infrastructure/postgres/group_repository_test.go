package postgres_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/group/grouptest"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// インメモリ実装と同じ契約を Postgres でも回す。
//
// 記録と種目は同じスキーマに書く。仲間の記録の読み口は set_logs と
// user_exercises を結合するので、別の保存先に書くと何も読めない。
func newGroupRepos(t *testing.T) grouptest.Repos {
	t.Helper()
	pool := migratedDB(t)
	return grouptest.Repos{
		Groups:    postgres.NewGroupRepository(pool),
		Activity:  postgres.NewGroupActivityReader(pool),
		SetLogs:   postgres.NewSetLogRepository(pool),
		Exercises: postgres.NewExerciseRepository(pool, grouptest.Seed()),
	}
}

func TestGroupRepository_Contract(t *testing.T) {
	grouptest.RunGroupContract(t, newGroupRepos)
}

func TestGroupActivityReader_Contract(t *testing.T) {
	grouptest.RunActivityContract(t, newGroupRepos)
}
