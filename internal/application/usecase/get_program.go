package usecase

import (
	"context"
	"errors"
	"fmt"
	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// GetProgram は保存されているプログラムを返すユースケース。
//
// プレゼンテーション層からリポジトリを直に叩かせないために置く。
// 直に叩くと、キャンセルの扱いやエラーの包み方がその経路だけ他と違い、
// 実装を差し替えたときに誰も気づけない。
type GetProgram struct {
	programs program.Reader
}

func NewGetProgram(programs program.Reader) *GetProgram {
	return &GetProgram{programs: programs}
}

func (u *GetProgram) Execute(ctx context.Context, user account.UserID) (_ *program.Program, err error) {
	// 出口で1度だけ翻訳する。return ごとに書くと、経路が増えたときに
	// 包み忘れた1本だけが 500 で返る。
	defer func() { err = apperror.Classify(err) }()

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("プログラムの取得が中断された: %w", err)
	}

	prog, err := u.programs.Get(ctx, user)
	if err != nil {
		// 取得の文脈では 404。まだ存在しないという意味であって、状態の
		// 衝突ではない。classify は分類済みのものを素通しするので、
		// ここで写しておけば既定の 409 に上書きされない。
		if errors.Is(err, program.ErrProgramNotConfigured) {
			return nil, fmt.Errorf("%w: %w", apperror.ErrNotFound, err)
		}
		return nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	// リポジトリの契約違反。呼び出し側が nil を「未設定」と
	// 「取得成功」のどちらとも解釈できてしまう。
	if prog == nil {
		return nil, fmt.Errorf("%w: プログラムの取得: %w",
			apperror.ErrNotFound, program.ErrProgramNotConfigured)
	}
	return prog, nil
}
