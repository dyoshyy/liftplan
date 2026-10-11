package memory_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/group/grouptest"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
)

func newGroupRepos(t *testing.T) grouptest.Repos {
	t.Helper()
	logs := memory.NewSetLogRepository()
	exercises := memory.NewExerciseRepository(grouptest.Seed())
	return grouptest.Repos{
		Groups:    memory.NewGroupRepository(),
		Activity:  memory.NewGroupActivityReader(logs, exercises),
		SetLogs:   logs,
		Exercises: exercises,
	}
}

func TestGroupRepository_Contract(t *testing.T) {
	grouptest.RunGroupContract(t, newGroupRepos)
}

func TestGroupActivityReader_Contract(t *testing.T) {
	grouptest.RunActivityContract(t, newGroupRepos)
}
