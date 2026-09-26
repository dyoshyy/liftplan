// Package usecase はアプリケーション層。
//
// ユースケースはデータを集めてドメインに渡すだけで、判断は一切しない。
// 判断がここに漏れ出したら、それはドメイン層に置くべきもの。
//
// **利用者は ctx の直後、第2引数で受け取る。**入力の構造体
// （GetSessionInput など）に混ぜない。入力は送り主が書いたリクエストから
// 組み立てられるので、所有者をそこに置くと、送り主が名乗った
// 名前で他人の記録を読み書きできる形が1回のミスで作れる。所有者は
// 認証から来るもので、入力から来るものではない。位置を全ての口で
// 揃えているのは、呼び出し側が並びを覚えずに済むようにするため。
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

func (u *GetSession) Execute(ctx context.Context, user account.UserID, in GetSessionInput) (_ planning.PlannedSession, err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

	if in.Date.IsZero() {
		return planning.PlannedSession{}, errors.New("対象日が指定されていない")
	}

	prog, err := u.programs.Get(ctx, user)
	if err != nil {
		return planning.PlannedSession{}, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	// リポジトリの契約違反。(nil, nil) を返されると SessionPlanner の
	// 匿名エラーになり、プレゼンテーション層が「未設定」と判別できない。
	if prog == nil {
		return planning.PlannedSession{}, fmt.Errorf(
			"プログラムの取得: %w", program.ErrProgramNotConfigured)
	}
	// 週目標は保存値を使わず、設定から組み直す（理由は seed.WithDerivedTarget）。
	prog, err = seed.WithDerivedTarget(prog)
	if err != nil {
		return planning.PlannedSession{}, err
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
	history, err := u.logs.FindAll(ctx, user)
	if err != nil {
		return planning.PlannedSession{}, fmt.Errorf("実績の取得に失敗: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return planning.PlannedSession{}, fmt.Errorf("セッションの導出が中断された: %w", err)
	}

	conditions, err := u.conditions.FindAll(ctx, user)
	if err != nil {
		return planning.PlannedSession{}, fmt.Errorf("コンディションの取得に失敗: %w", err)
	}

	return u.planner.Plan(planning.PlanRequest{
		Program:    prog,
		Target:     prog.WeeklyTarget(),
		Pool:       pool,
		History:    history,
		Conditions: conditions,
		Date:       in.Date,
	})
}
