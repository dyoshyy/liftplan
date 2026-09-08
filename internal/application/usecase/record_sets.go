package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

// RecordSets は実績ログを保存するユースケース。
// リポジトリ側が冪等なので、同じログを二度受け取っても壊れない。
//
// 種目マスタとの突合を行うのはここ。SetLog は ExerciseID しか持たず
// 実在を検証できない。実在しない種目のログを受け取ると、その実績は
// どの筋区分にも計上されないまま履歴に残り続ける。削除の口が無く、
// 同じIDの再送は衝突になるので、打ち間違い1回で復旧できなくなる。
type RecordSets struct {
	repo      setlog.Writer
	exercises exercise.Reader
}

func NewRecordSets(
	repo setlog.Writer,
	exercises exercise.Reader,
) *RecordSets {
	return &RecordSets{repo: repo, exercises: exercises}
}

func (u *RecordSets) Execute(ctx context.Context, logs []*setlog.SetLog) error {
	// 空は成功として扱う。クライアントは同期のたびに送ってくるので、
	// 送るものが無い回に I/O を起こす理由がない。「空を送ってきた」ことを
	// エラーにすると、正常な同期がエラーログを埋める。
	if len(logs) == 0 {
		return nil
	}
	// nil を混ぜたままリポジトリに渡すと、実装側で panic するか
	// 黙って飛ばされるかが実装依存になる。境界で弾く。
	for i, l := range logs {
		if l == nil {
			return fmt.Errorf("%w: %d番目のセットログが nil である", ErrInvalidInput, i)
		}
	}

	// 切断済みのクライアントに 204 を返しつつ書き込むのを避ける。
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("保存が中断された: %w", err)
	}

	pool, err := u.exercises.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("種目の取得に失敗: %w", err)
	}
	known := make(map[exercise.ExerciseID]bool, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		known[e.ID()] = true
	}
	for i, l := range logs {
		if !known[l.ExerciseID()] {
			return fmt.Errorf("%w: %w: logs[%d] %s",
				ErrInvalidInput, exercise.ErrExerciseNotFound, i, l.ExerciseID())
		}
	}

	if err := u.repo.Save(ctx, logs); err != nil {
		return fmt.Errorf("実績の保存に失敗: %w", err)
	}
	return nil
}
