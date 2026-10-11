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

// GetSubstituteInput は差し替えの入力。Date は画面が表示している計画の日付。
type GetSubstituteInput struct {
	Date     training.Date
	Replace  exercise.ExerciseID
	Rejected []exercise.ExerciseID
}

// GetSubstitute は今日の補助の代わりを1つ導出するユースケース。
// 組み立ては GetForecast と同じ。
type GetSubstitute struct {
	exercises  exercise.Reader
	logs       setlog.Reader
	conditions condition.Reader
	programs   program.Reader
	planner    planning.SessionPlanner
}

func NewGetSubstitute(
	exercises exercise.Reader,
	logs setlog.Reader,
	conditions condition.Reader,
	programs program.Reader,
	planner planning.SessionPlanner,
) *GetSubstitute {
	return &GetSubstitute{
		exercises: exercises, logs: logs,
		conditions: conditions, programs: programs, planner: planner,
	}
}

func (u *GetSubstitute) Execute(ctx context.Context, user account.UserID, in GetSubstituteInput) (_ planning.PlannedSet, _ bool, err error) {
	defer func() { err = apperror.Classify(err) }()

	if in.Date.IsZero() {
		return planning.PlannedSet{}, false, errors.New("対象日が指定されていない")
	}

	prog, err := u.programs.Get(ctx, user)
	if err != nil {
		return planning.PlannedSet{}, false, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	if prog == nil {
		return planning.PlannedSet{}, false, fmt.Errorf("プログラムの取得: %w", program.ErrProgramNotConfigured)
	}
	target, err := seed.DefaultWeeklyTarget(prog.Frequency(), prog.SessionVolume())
	if err != nil {
		return planning.PlannedSet{}, false, fmt.Errorf("週目標が組めない: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return planning.PlannedSet{}, false, fmt.Errorf("代わりの導出が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return planning.PlannedSet{}, false, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	history, err := u.logs.FindAll(ctx, user)
	if err != nil {
		return planning.PlannedSet{}, false, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return planning.PlannedSet{}, false, fmt.Errorf("代わりの導出が中断された: %w", err)
	}

	conditions, err := u.conditions.FindAll(ctx, user)
	if err != nil {
		return planning.PlannedSet{}, false, fmt.Errorf("コンディションの取得に失敗: %w", err)
	}

	return u.planner.Substitute(planning.PlanRequest{
		Program:    prog,
		Target:     target,
		Pool:       pool,
		History:    history,
		Conditions: conditions,
		Date:       in.Date,
	}, in.Replace, in.Rejected)
}
