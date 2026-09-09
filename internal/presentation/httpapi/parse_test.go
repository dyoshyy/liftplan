package httpapi_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/presentation/httpapi"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

func TestParseExerciseIDs(t *testing.T) {
	cases := map[string][]exercise.ExerciseID{
		"":                nil,
		"bench":           {"bench"},
		"bench,squat":     {"bench", "squat"},
		",bench,,squat,":  {"bench", "squat"},
		" bench , squat ": {"bench", "squat"},
		",,,":             {},
	}
	for raw, want := range cases {
		got := httpapi.ParseExerciseIDs(raw)
		if len(got) != len(want) {
			t.Errorf("%q: 件数が誤り: %v（期待 %v）", raw, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q: %d番目が誤り: %q（期待 %q）", raw, i, got[i], want[i])
			}
		}
	}
}
