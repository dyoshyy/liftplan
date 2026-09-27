package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// SetSplitCycle は分割の周期を差し替える。
//
// 種目マスタを読むのは、宣言種目がどの分割にも属さない状態を弾くため。
// 属さない種目は毎日「今日の候補ではない」と判定され、**二度と軸に
// 出ない**。他の宣言が毎日1つは該当するのでフォールバックも発火せず、
// エラーも立たないまま消える。
//
// プリセットが最小頻度を持つ場合（いまは five_way だけ）、いまの頻度が
// それを下回っていれば拒否する。既存の保存済みプログラムがすでに
// 下回っている状態は動かさない。ここで弾くのは新しく分割を選ぶ操作だけで、
// 読み出しと計画はこれまで通り動く。
type SetSplitCycle struct {
	exercises exercise.Reader
	reader    program.Reader
	writer    program.Writer
}

func NewSetSplitCycle(
	exercises exercise.Reader,
	reader program.Reader,
	writer program.Writer,
) *SetSplitCycle {
	return &SetSplitCycle{exercises: exercises, reader: reader, writer: writer}
}

func (u *SetSplitCycle) Execute(ctx context.Context, user account.UserID, cycle []program.Split) (err error) {
	defer func() { err = apperror.Classify(err) }()

	prog, err := u.reader.Get(ctx, user)
	if err != nil {
		return err
	}

	// プリセットの下限は「選ぶ」瞬間に見る。プリセットキーはここに届かない
	// （周期を展開した中身しか送られてこない）ので、中身で引き当てる
	// （seed.MatchPreset、seed.SplitPreset.MinFrequencyPerWeek）。
	//
	// 下限0（下限なし）を別条件で弾かない。頻度は1以上しか無い
	// （program.NewFrequency）ので、下限0の比較は常に偽になり、素通しと
	// 同じになる。
	if preset, ok, err := seed.MatchPreset(cycle); err != nil {
		return fmt.Errorf("分割プリセットの取得に失敗: %w", err)
	} else if ok && prog.Frequency().PerWeek() < preset.MinFrequencyPerWeek {
		return fmt.Errorf("%w: %sは週%d回以上で使える", apperror.ErrInvalidInput,
			preset.Name, preset.MinFrequencyPerWeek)
	}

	next, err := prog.WithCycle(cycle)
	if err != nil {
		return fmt.Errorf("%w: 分割: %w", apperror.ErrInvalidInput, err)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("分割の保存が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx, user)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	if err := verifyDeclaredHaveADay(pool, next); err != nil {
		return err
	}

	return u.writer.Save(ctx, user, next)
}
