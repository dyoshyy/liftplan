package postgres_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/runlog/runlogtest"
	"github.com/dyoshyy/liftplan/internal/infrastructure/postgres"
)

// インメモリ実装と同じ契約を Postgres でも回す。ケースごとに新しいスキーマを作る。
func TestRunLogRepository_Contract(t *testing.T) {
	runlogtest.RunContract(t, func(t *testing.T) runlogtest.Repo {
		return postgres.NewRunLogRepository(migratedDB(t))
	})
}
