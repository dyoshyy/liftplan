package memory_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/runlog/runlogtest"
	"github.com/dyoshyy/liftplan/internal/infrastructure/memory"
)

func TestRunLogRepository_Contract(t *testing.T) {
	runlogtest.RunContract(t, func(*testing.T) runlogtest.Repo { return memory.NewRunLogRepository() })
}
