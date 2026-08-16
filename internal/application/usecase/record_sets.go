package usecase

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// RecordSets は実績ログを保存するユースケース。
// リポジトリ側が冪等なので、同じログを二度受け取っても壊れない。
type RecordSets struct {
	repo training.SetLogRepository
}

func NewRecordSets(repo training.SetLogRepository) *RecordSets {
	return &RecordSets{repo: repo}
}

func (u *RecordSets) Execute(ctx context.Context, logs []*training.SetLog) error {
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

	if err := u.repo.Save(ctx, logs); err != nil {
		return fmt.Errorf("実績の保存に失敗: %w", err)
	}
	return nil
}
