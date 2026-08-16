package memory_test

import (
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/memory"
	"github.com/dyoshyy/liftplan-server/internal/infrastructure/repositorytest"
)

// インメモリ実装が共有の契約を満たすこと。
//
// 同じスイートを Postgres 実装にも流す。実装ごとにテストを書き直すと、
// 片方だけが契約を満たす状態に気づけない。
func TestMemory_SatisfiesTheRepositoryContract(t *testing.T) {
	t.Run("SetLog", func(t *testing.T) {
		repositorytest.RunSetLogContract(t, func(*testing.T) training.SetLogRepository {
			return memory.NewSetLogRepository()
		})
	})
	t.Run("Condition", func(t *testing.T) {
		repositorytest.RunConditionContract(t, func(*testing.T) training.ConditionRepository {
			return memory.NewConditionRepository()
		})
	})
	t.Run("Program", func(t *testing.T) {
		repositorytest.RunProgramContract(t, func(*testing.T) training.ProgramRepository {
			return memory.NewProgramRepository(nil)
		})
	})
}
