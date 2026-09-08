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
	"github.com/dyoshyy/liftplan-server/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/program"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

type GetSessionInput struct {
	Date           training.Date
	DeloadAccepted []exercise.ExerciseID
}

// GetSession は指定日のセッションを導出するユースケース。
type GetSession struct {
	exercises  exercise.Reader
	logs       setlog.Reader
	conditions condition.Reader
	programs   program.Reader
	planner    planning.SessionPlanner
}

func NewGetSession(
	exercises exercise.Reader,
	logs setlog.Reader,
	conditions condition.Reader,
	programs program.Reader,
	planner planning.SessionPlanner,
) *GetSession {
	return &GetSession{
		exercises: exercises, logs: logs,
		conditions: conditions, programs: programs, planner: planner,
	}
}

func (u *GetSession) Execute(ctx context.Context, in GetSessionInput) (planning.PlannedSession, error) {
	if in.Date.IsZero() {
		return planning.PlannedSession{}, errors.New("対象日が指定されていない")
	}

	prog, err := u.programs.Get(ctx)
	if err != nil {
		return planning.PlannedSession{}, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	// リポジトリの契約違反。(nil, nil) を返されると SessionPlanner の
	// 匿名エラーになり、プレゼンテーション層が「未設定」と判別できない。
	if prog == nil {
		return planning.PlannedSession{}, fmt.Errorf(
			"プログラムの取得: %w", program.ErrProgramNotConfigured)
	}
	// 途中でキャンセルされたら残りの取得をやめる。履歴は全件を読むので、
	// クライアントが切断済みでも最後まで走らせると数十MBを無駄に確保する。
	if err := ctx.Err(); err != nil {
		return planning.PlannedSession{}, fmt.Errorf("セッションの導出が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return planning.PlannedSession{}, fmt.Errorf("種目の取得に失敗: %w", err)
	}
	history, err := u.logs.FindAll(ctx)
	if err != nil {
		return planning.PlannedSession{}, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return planning.PlannedSession{}, fmt.Errorf("セッションの導出が中断された: %w", err)
	}

	conditions, err := u.conditions.FindAll(ctx)
	if err != nil {
		return planning.PlannedSession{}, fmt.Errorf("コンディションの取得に失敗: %w", err)
	}

	// 承認された種目が実在しないと、デロードは黙って効かない。
	// ユーザーは承認したつもりでいるのに重量が下がらない。
	if len(in.DeloadAccepted) > 0 {
		known := make(map[exercise.ExerciseID]bool, len(pool))
		for _, e := range pool {
			if e == nil {
				continue
			}
			known[e.ID()] = true
		}
		for _, id := range in.DeloadAccepted {
			if !known[id] {
				return planning.PlannedSession{}, fmt.Errorf("%w: %w: %s",
					ErrInvalidInput, exercise.ErrExerciseNotFound, id)
			}
		}
	}

	return u.planner.Plan(planning.PlanRequest{
		Program:        prog,
		Pool:           pool,
		History:        history,
		Conditions:     conditions,
		Date:           in.Date,
		DeloadAccepted: in.DeloadAccepted,
	})
}
