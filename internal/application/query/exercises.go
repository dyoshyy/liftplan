package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// Exercise は種目マスタの1件。
//
// 画面が種目IDを日本語で出すために要る。`bench` のままだと、
// ジムで一瞬見て何の種目か分からない。
type Exercise struct {
	ID          exercise.ExerciseID
	Name        string
	IncrementKg float64
}

// Exercises は種目マスタを読む経路。
type Exercises struct {
	repo exercise.Reader
}

func NewExercises(repo exercise.Reader) *Exercises {
	return &Exercises{repo: repo}
}

func (q *Exercises) All(ctx context.Context) ([]Exercise, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("読み取りが中断された: %w", err)
	}

	pool, err := q.repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}

	out := make([]Exercise, 0, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		item := Exercise{
			ID:          e.ID(),
			Name:        e.Name(),
			IncrementKg: e.Increment().Kg(),
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
