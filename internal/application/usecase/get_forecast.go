package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

type GetForecastInput struct {
	Date training.Date
}

// GetForecast は指定日を起点に、頻度ぶんの先の回をまとめて導出する
// ユースケース。組み立ては GetSession と同じ（設計書の決定）。今日の
// 応答とは別の口にするのは、毎回開く「今日」の読み込みに、見ないかも
// しれない先の回の分まで混ぜないため。
type GetForecast struct {
	exercises  exercise.Reader
	logs       setlog.Reader
	conditions condition.Reader
	programs   program.Reader
	planner    planning.SessionPlanner
}

func NewGetForecast(
	exercises exercise.Reader,
	logs setlog.Reader,
	conditions condition.Reader,
	programs program.Reader,
	planner planning.SessionPlanner,
) *GetForecast {
	return &GetForecast{
		exercises: exercises, logs: logs,
		conditions: conditions, programs: programs, planner: planner,
	}
}

func (u *GetForecast) Execute(ctx context.Context, user account.UserID, in GetForecastInput) (_ []planning.PlannedSession, err error) {
	defer func() { err = apperror.Classify(err) }()

	if in.Date.IsZero() {
		return nil, errors.New("対象日が指定されていない")
	}

	prog, err := u.programs.Get(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	if prog == nil {
		return nil, fmt.Errorf("プログラムの取得: %w", program.ErrProgramNotConfigured)
	}
	target, err := seed.DefaultWeeklyTarget(prog.Frequency(), prog.SessionVolume())
	if err != nil {
		return nil, fmt.Errorf("週目標が組めない: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("見込みの導出が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	history, err := u.logs.FindAll(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("見込みの導出が中断された: %w", err)
	}

	conditions, err := u.conditions.FindAll(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("コンディションの取得に失敗: %w", err)
	}

	return u.planner.Forecast(planning.PlanRequest{
		Program:    prog,
		Target:     target,
		Pool:       pool,
		History:    history,
		Conditions: conditions,
		Date:       in.Date,
	})
}
