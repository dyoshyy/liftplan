// Package usecase はアプリケーション層。
//
// ユースケースはデータを集めてドメインに渡すだけで、判断は一切しない。
// 判断がここに漏れ出したら、それはドメイン層に置くべきもの。
package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

type GetSessionInput struct {
	Date training.Date
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

func (u *GetSession) Execute(ctx context.Context, in GetSessionInput) (_ planning.PlannedSession, err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = classify(err) }()

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

	return u.planner.Plan(planning.PlanRequest{
		Program:    prog,
		Pool:       pool,
		History:    history,
		Conditions: conditions,
		Date:       in.Date,
	})
}
