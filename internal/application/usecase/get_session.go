// Package usecase はアプリケーション層。
//
// ユースケースはデータを集めてドメインに渡すだけで、判断は一切しない。
// 判断がここに漏れ出したら、それはドメイン層に置くべきもの。
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

type GetSessionInput struct {
	Date           training.Date
	DeloadAccepted []training.ExerciseID
}

// GetSession は指定日のセッションを導出するユースケース。
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
	// リポジトリの契約違反。(nil, nil) を返されると SessionPlanner の
	// 匿名エラーになり、プレゼンテーション層が「未設定」と判別できない。
	if program == nil {
		return training.PlannedSession{}, fmt.Errorf(
			"プログラムの取得: %w", training.ErrProgramNotConfigured)
	}
	// 途中でキャンセルされたら残りの取得をやめる。履歴は全件を読むので、
	// クライアントが切断済みでも最後まで走らせると数十MBを無駄に確保する。
	if err := ctx.Err(); err != nil {
		return training.PlannedSession{}, fmt.Errorf("セッションの導出が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	history, err := u.logs.FindAll(ctx)
	if err != nil {
		return training.PlannedSession{}, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return training.PlannedSession{}, fmt.Errorf("セッションの導出が中断された: %w", err)
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
	})
}
