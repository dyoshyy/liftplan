package training_test

import (
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// センチネルは集約ごとのパッケージに分かれたので、取り違えても
// 同じパッケージ内の重複としては現れない。横断で突き合わせる。
//
// 未設定は状態なので、呼び出し側が errors.Is で判別して初期設定へ
// 誘導できること。文字列比較を強いてはいけない。
func TestSentinelErrors_AreIdentifiableAndDistinct(t *testing.T) {
	sentinels := map[string]error{
		"program.ErrProgramNotConfigured":   program.ErrProgramNotConfigured,
		"program.ErrNoDeclaredExercise":     program.ErrNoDeclaredExercise,
		"exercise.ErrExerciseNotFound":      exercise.ErrExerciseNotFound,
		"setlog.ErrConflictingSetLog":       setlog.ErrConflictingSetLog,
		"training.ErrRepositoryUnavailable": training.ErrRepositoryUnavailable,
	}

	for name, err := range sentinels {
		if err == nil {
			t.Errorf("%s が定義されていない", name)
			continue
		}
		if err.Error() == "" {
			t.Errorf("%s の説明が空である", name)
		}
	}

	// 取り違えて同じ値を割り当てると、呼び出し側が分岐できない。
	for aName, a := range sentinels {
		for bName, b := range sentinels {
			if aName >= bName {
				continue
			}
			if errors.Is(a, b) {
				t.Errorf("%s と %s が区別できない", aName, bName)
			}
		}
	}
}
