package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/runlog"
)

// RecordRun はランの記録を保存するユースケース。リポジトリ側が冪等なので、
// 同じ記録を二度受け取っても壊れない。
type RecordRun struct {
	repo runlog.Writer
}

func NewRecordRun(repo runlog.Writer) *RecordRun {
	return &RecordRun{repo: repo}
}

func (u *RecordRun) Execute(ctx context.Context, user account.UserID, l *runlog.RunLog) (err error) {
	defer func() { err = apperror.Classify(err) }()

	if l == nil {
		return fmt.Errorf("%w: ランの記録が無い", apperror.ErrInvalidInput)
	}
	// 切断済みのクライアントに 204 を返しつつ書き込むのを避ける。
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("保存が中断された: %w", err)
	}
	if err := u.repo.Save(ctx, user, l); err != nil {
		return fmt.Errorf("ランの記録の保存に失敗: %w", err)
	}
	return nil
}

// DeleteRunLog は打ち間違いを取り消すユースケース。
type DeleteRunLog struct {
	repo runlog.Writer
}

func NewDeleteRunLog(repo runlog.Writer) *DeleteRunLog {
	return &DeleteRunLog{repo: repo}
}

func (u *DeleteRunLog) Execute(ctx context.Context, user account.UserID, id runlog.RunLogID) (err error) {
	defer func() { err = apperror.Classify(err) }()

	if id == "" {
		return fmt.Errorf("%w: 取り消す記録のIDが無い", apperror.ErrInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("取り消しが中断された: %w", err)
	}
	if err := u.repo.Delete(ctx, user, id); err != nil {
		return fmt.Errorf("ランの記録の取り消しに失敗: %w", err)
	}
	return nil
}
