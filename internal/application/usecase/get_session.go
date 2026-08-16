package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

type GetSessionInput struct {
	Date           training.Date
	DeloadAccepted []ExerciseID
	AccessorySlots int
}

type GetSession struct {
	exercises  training.ExerciseRepository
	logs       training.SetLogRepository
	conditions training.ConditionRepository
	programs   training.ProgramRepository
	planner    training.SessionPlanner
}

func NewGetSession(
	exercises training.ExerciseRepository,
	logs training.SetLogRepository,
	conditions training.ConditionRepository,
	programs training.ProgramRepository,
	planner training.SessionPlanner,
) *GetSession {
	return &GetSession{
		exercises: exercises, logs: logs,
		conditions: conditions, programs: programs, planner: planner,
	}
}

func (u *GetSession) Execute(ctx context.Context, in GetSessionInput) (training.PlannedSession, error) {
	if in.Date.IsZero() {
		return training.PlannedSession{}, errors.New("対象日が指定されていない")
	}

	program, err := u.programs.Get(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	history, err := u.logs.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	conditions, err := u.conditions.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("コンディションの取得に失敗: %w", err)
	}

	return u.planner.Plan(training.PlanRequest{
		Program:        program,
		Pool:           pool,
		History:        history,
		Conditions:     conditions,
		Date:           in.Date,
		DeloadAccepted: in.DeloadAccepted,
		AccessorySlots: in.AccessorySlots,
	})
}
