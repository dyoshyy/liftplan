package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// Point は推定1RMの1点。
type Point struct {
	Date training.Date
	Kg   float64
}

// Trend は種目ごとの推定1RMの推移。
//
// 伸びているかを答える唯一の線。体感は当てにならないし、実施重量だけを
// 見てもレップ数が違えば比較にならない。
type Trend struct {
	ExerciseID training.ExerciseID
	Name       string
	Points     []Point
	CurrentKg  float64
	// ChangeKg は期間の最初から最後までの差。
	ChangeKg float64
}

// RegionVolume は筋区分ごとの、今週の充足。
type RegionVolume struct {
	Region    training.MuscleRegion
	TargetSet float64
	DoneSet   float64
}

// Stats は振り返りのための読み取り経路。
type Stats struct {
	logs      training.SetLogRepository
	exercises training.ExerciseRepository
	programs  training.ProgramRepository
	estimator training.OneRepMaxEstimator
}

func NewStats(
	logs training.SetLogRepository,
	exercises training.ExerciseRepository,
	programs training.ProgramRepository,
	estimator training.OneRepMaxEstimator,
) *Stats {
	return &Stats{logs: logs, exercises: exercises, programs: programs, estimator: estimator}
}

// Trends は主要な種目の推定1RMの推移を返す。
//
// セッションごとに1点を出す。日ごとではないのは、同じ日に同じ種目を
// 2回やる運用が無いため。実施重量そのものではなく推定1RMを使うのは、
// レップ数が違う日どうしを比べられるようにするため。
func (q *Stats) Trends(ctx context.Context, from, to training.Date) ([]Trend, error) {
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("期間が指定されていない")
	}

	h, pool, program, err := q.load(ctx)
	if err != nil {
		return nil, err
	}

	out := []Trend{}
	for _, e := range pool {
		if e == nil || !program.Declares(e.ID()) {
			continue
		}

		points := []Point{}
		for _, s := range h.ForExercise(e.ID()).OnOrAfter(from).OnOrBefore(to).Sessions() {
			// 推定できないセッション（全セット自重）は点にしない。
			// 0 として混ぜると、線が床まで落ちて推移が読めなくなる。
			v, ok := s.MedianOneRepMax()
			if !ok {
				continue
			}
			points = append(points, Point{Date: s.Date(), Kg: v.Kg()})
		}
		if len(points) == 0 {
			continue
		}

		t := Trend{
			ExerciseID: e.ID(),
			Name:       e.Name(),
			Points:     points,
			CurrentKg:  points[len(points)-1].Kg,
			ChangeKg:   points[len(points)-1].Kg - points[0].Kg,
		}
		out = append(out, t)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ExerciseID < out[j].ExerciseID })
	return out, nil
}

// WeeklyVolume は今週の週目標に対する充足を返す。
//
// これはアプリの中心概念なのに、これまでどこにも表示されていなかった。
// 週目標と残差で補助種目を選んでいるのに、利用者にはその存在すら
// 見えていない。「なぜ今日この補助種目が出たのか」がここで分かる。
func (q *Stats) WeeklyVolume(ctx context.Context, asOf training.Date) ([]RegionVolume, error) {
	if asOf.IsZero() {
		return nil, fmt.Errorf("基準日が指定されていない")
	}

	h, pool, program, err := q.load(ctx)
	if err != nil {
		return nil, err
	}

	// 数え方はエンジンと同じものを使う。別々に実装すると、画面に出る
	// 数字とエンジンが使う数字がずれて、どちらが正しいか分からなくなる。
	coverage := training.CoverageBetween(h, pool, asOf.WeekStart(), asOf)

	target := program.WeeklyTarget()
	out := make([]RegionVolume, 0, len(target.Regions()))
	for _, r := range target.Regions() {
		out = append(out, RegionVolume{
			Region:    r,
			TargetSet: target.Sets(r),
			DoneSet:   coverage.Sets(r),
		})
	}

	// 埋まっていない順。目を向けるべきものが上に来る。
	sort.Slice(out, func(i, j int) bool {
		ri := out[i].DoneSet / max(out[i].TargetSet, 0.001)
		rj := out[j].DoneSet / max(out[j].TargetSet, 0.001)
		if ri != rj {
			return ri < rj
		}
		return out[i].Region < out[j].Region
	})
	return out, nil
}

func (q *Stats) load(ctx context.Context) (
	training.History, []*training.Exercise, *training.Program, error,
) {
	if err := ctx.Err(); err != nil {
		return training.History{}, nil, nil, fmt.Errorf("読み取りが中断された: %w", err)
	}

	h, err := q.logs.FindAll(ctx)
	if err != nil {
		return training.History{}, nil, nil, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	pool, err := q.exercises.FindAll(ctx)
	if err != nil {
		return training.History{}, nil, nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	program, err := q.programs.Get(ctx)
	if err != nil {
		return training.History{}, nil, nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	if program == nil {
		return training.History{}, nil, nil,
			fmt.Errorf("プログラムの取得: %w", training.ErrProgramNotConfigured)
	}
	return h, pool, program, nil
}
