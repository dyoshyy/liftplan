package query_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
)

// 公開メソッドはどれも、出口でエラーを分類して返すこと。
//
// 分類は各メソッドの defer 1行で、書き忘れてもコンパイルは通る。忘れると
// 保存先の一時障害が 500 になり、「調べるべき障害」として鳴る（#129）。
//
// HTTP 側の検査（TestReadEndpoints_ClassifyFailures）だけでは足りない。
// ハンドラは Days → LastPerformances、Trends → WeeklyVolume の順に呼ぶので、
// 後ろの1本は前が先に失敗して届かず、defer を外しても緑のままになる。
// だから公開メソッドを1本ずつここで見る。
func TestQueries_ClassifyUnavailable(t *testing.T) {
	unavailable := &stubExercises{
		err: fmt.Errorf("種目の取得: %w", training.ErrRepositoryUnavailable),
	}
	exercises := query.NewExercises(unavailable)
	history := query.NewHistory(&stubLogs{}, unavailable)
	stats := query.NewStats(&stubLogs{}, unavailable, &stubConditions{}, &stubProgram{},
		planning.DefaultOneRepMaxEstimator())

	ctx := context.Background()
	from, to := date(t, "2026-08-01"), date(t, "2026-08-17")

	cases := []struct {
		name string
		call func() error
	}{
		{"Exercises.All", func() error { _, err := exercises.All(ctx, testUser); return err }},
		{"History.Days", func() error { _, err := history.Days(ctx, testUser, from, to); return err }},
		{"History.LastPerformances", func() error { _, err := history.LastPerformances(ctx, testUser, to); return err }},
		{"Stats.Trends", func() error { _, err := stats.Trends(ctx, testUser, from, to); return err }},
		{"Stats.WeeklyVolume", func() error { _, err := stats.WeeklyVolume(ctx, testUser, to); return err }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			if !errors.Is(err, apperror.ErrUnavailable) {
				t.Errorf("UNAVAILABLE に分類されていない: %v", err)
			}
			// 元のエラーは連鎖に残す。切ると、ログに原因が残らない。
			if !errors.Is(err, training.ErrRepositoryUnavailable) {
				t.Errorf("元のエラーが連鎖から消えている: %v", err)
			}
		})
	}
}
