package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
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
	// Stimulus はその種目が各筋区分へ与える刺激。
	//
	// 画面が種目の一覧を部位ごとにまとめるのに要る。どれを代表に選ぶかは
	// 表示の判断なので、ここでは argmax を取らず分布のまま渡す。
	//
	// 支配区分（PrimaryRegion）をドメインに作らないのは、あれが「その種目が
	// どの日に出るか」を決めるためのもので、分割法と一緒に入れると決めて
	// あるため（2026-09-06-training-goals-design.md）。表示のために先に
	// 作ると、意味の違う2つが同じ名前で並ぶ。
	Stimulus map[training.MuscleRegion]float64
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
		stimulus := make(map[training.MuscleRegion]float64, len(e.Stimulus().Regions()))
		for _, r := range e.Stimulus().Regions() {
			if c, ok := e.Stimulus().Contribution(r); ok {
				stimulus[r] = c.Float()
			}
		}

		item := Exercise{
			ID:          e.ID(),
			Name:        e.Name(),
			IncrementKg: e.Increment().Kg(),
			Stimulus:    stimulus,
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
