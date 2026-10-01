package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
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
	ExerciseID exercise.ExerciseID
	Name       string
	Points     []Point
	CurrentKg  float64
	// ChangeKg は期間の最初から最後までの差。
	ChangeKg float64
}

// RegionVolume は筋区分ごとの充足（直近4週の週あたり）。
type RegionVolume struct {
	Region    training.MuscleRegion
	TargetSet float64
	DoneSet   float64
}

// Stats は振り返りのための読み取り経路。
type Stats struct {
	logs      setlog.Reader
	exercises exercise.Reader
	programs  program.Reader
	estimator planning.OneRepMaxEstimator
}

func NewStats(
	logs setlog.Reader,
	exercises exercise.Reader,
	programs program.Reader,
	estimator planning.OneRepMaxEstimator,
) *Stats {
	return &Stats{logs: logs, exercises: exercises, programs: programs, estimator: estimator}
}

// Trends は主要な種目の推定1RMの推移を返す。
//
// セッションごとに1点を出す。日ごとではないのは、同じ日に同じ種目を
// 2回やる運用が無いため。実施重量そのものではなく推定1RMを使うのは、
// レップ数が違う日どうしを比べられるようにするため。
func (q *Stats) Trends(ctx context.Context, user account.UserID, from, to training.Date) (_ []Trend, err error) {
	// 出口で1度だけ翻訳する。usecase と同じ形。ここを通らない公開メソッドは、
	// 一時障害を 500 で返す（#129）。
	defer func() { err = apperror.Classify(err) }()
	if from.IsZero() || to.IsZero() {
		return nil, fmt.Errorf("期間が指定されていない")
	}

	h, pool, prog, err := q.load(ctx, user)
	if err != nil {
		return nil, err
	}

	out := []Trend{}
	for _, e := range pool {
		if e == nil || !prog.Declares(e.ID()) {
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

// WeeklyVolume は週目標に対する充足を、直近4週の週あたり平均で返す。
//
// これはアプリの中心概念なのに、これまでどこにも表示されていなかった。
// 週目標と残差で補助種目を選んでいるのに、利用者にはその存在すら
// 見えていない。「なぜ今日この補助種目が出たのか」がここで分かる。
func (q *Stats) WeeklyVolume(ctx context.Context, user account.UserID, asOf training.Date) (_ []RegionVolume, err error) {
	defer func() { err = apperror.Classify(err) }()
	if asOf.IsZero() {
		return nil, fmt.Errorf("基準日が指定されていない")
	}

	h, pool, prog, err := q.load(ctx, user)
	if err != nil {
		return nil, err
	}

	// 数え方はエンジンと同じものを使う。別々に実装すると、画面に出る
	// 数字とエンジンが使う数字がずれて、どちらが正しいか分からなくなる。
	//
	// 窓もエンジンと同じ4週。1週で見せると、エンジンが4週で均している
	// ものを週ごとの凸凹で見せることになり、「足りていない区分から選ばれる」
	// が画面の上で成り立たなくなる。週目標と並べるので週あたりに直す。
	// 当日を含めるのは、今日やったぶんが画面に反映されないと記録した実感が
	// 無いため（エンジンは当日を見ないが、画面は見せる）。
	coverage := planning.CoverageBetween(h, pool,
		asOf.AddDays(-(planning.CoverageWindowDays - 1)), asOf)

	// 週目標は保存された持ち物ではなく、設定（頻度と1回の量）から組み直す
	// （D-139、#176）。計画と同じ導き方で比べないと、計画が狙う区分と
	// 画面が「足りていない」と言う区分が食い違う。
	target, err := seed.DefaultWeeklyTarget(prog.Frequency(), prog.SessionVolume())
	if err != nil {
		return nil, fmt.Errorf("週目標が組めない: %w", err)
	}
	out := make([]RegionVolume, 0, len(target.Regions()))
	for _, r := range target.Regions() {
		out = append(out, RegionVolume{
			Region:    r,
			TargetSet: target.Sets(r),
			DoneSet:   training.Quantize(coverage.Sets(r) / planning.CoverageWindowWeeks),
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

func (q *Stats) load(ctx context.Context, user account.UserID) (
	setlog.History, []*exercise.Exercise, *program.Program, error,
) {
	if err := ctx.Err(); err != nil {
		return setlog.History{}, nil, nil, fmt.Errorf("読み取りが中断された: %w", err)
	}

	h, err := q.logs.FindAll(ctx, user)
	if err != nil {
		return setlog.History{}, nil, nil, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	pool, err := q.exercises.FindAll(ctx, user)
	if err != nil {
		return setlog.History{}, nil, nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	prog, err := q.programs.Get(ctx, user)
	if err != nil {
		return setlog.History{}, nil, nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	if prog == nil {
		return setlog.History{}, nil, nil,
			fmt.Errorf("プログラムの取得: %w", program.ErrProgramNotConfigured)
	}
	return h, pool, prog, nil
}
