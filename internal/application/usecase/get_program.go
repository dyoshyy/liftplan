package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// GetProgram は保存されているプログラムを返すユースケース。
//
// プレゼンテーション層からリポジトリを直に叩かせないために置く。
// 直に叩くと、キャンセルの扱いやエラーの包み方がその経路だけ他と違い、
// 実装を差し替えたときに誰も気づけない。
type GetProgram struct {
	programs training.ProgramRepository
}

func NewGetProgram(programs training.ProgramRepository) *GetProgram {
	return &GetProgram{programs: programs}
}

func (u *GetProgram) Execute(ctx context.Context) (*training.Program, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("プログラムの取得が中断された: %w", err)
	}

	program, err := u.programs.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("プログラムの取得に失敗: %w", err)
	}
	// リポジトリの契約違反。呼び出し側が nil を「未設定」と
	// 「取得成功」のどちらとも解釈できてしまう。
	if program == nil {
		return nil, fmt.Errorf("プログラムの取得: %w", training.ErrProgramNotConfigured)
	}
	return program, nil
}
