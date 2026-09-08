package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/program"
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

func (u *GetProgram) Execute(ctx context.Context) (*program.Program, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("プログラムの取得が中断された: %w", err)
	}

	prog, err := u.programs.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	// リポジトリの契約違反。呼び出し側が nil を「未設定」と
	// 「取得成功」のどちらとも解釈できてしまう。
	if prog == nil {
		return nil, fmt.Errorf("プログラムの取得: %w", program.ErrProgramNotConfigured)
	}
	return prog, nil
}
