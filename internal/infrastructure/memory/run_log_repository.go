package memory

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training/runlog"
)

// RunLogRepository はランの記録を「所有者とID」のキーで保持する。
// 利用者ごとに map を分けるのは SetLogRepository と同じ理由。
type RunLogRepository struct {
	mu     sync.RWMutex
	byUser map[account.UserID]map[runlog.RunLogID]*runlog.RunLog
}

func NewRunLogRepository() *RunLogRepository {
	return &RunLogRepository{byUser: map[account.UserID]map[runlog.RunLogID]*runlog.RunLog{}}
}

// FindAll は実施日の昇順、同じ日の中は ID の昇順で返す。
func (r *RunLogRepository) FindAll(_ context.Context, userID account.UserID) ([]*runlog.RunLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	byID := r.byUser[userID]
	out := make([]*runlog.RunLog, 0, len(byID))
	for _, l := range byID {
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b *runlog.RunLog) int {
		return cmp.Or(a.PerformedOn().Compare(b.PerformedOn()), cmp.Compare(a.ID(), b.ID()))
	})
	return out, nil
}

// Save は冪等。同じIDで同じ内容なら黙って受け入れ、違えば
// ErrConflictingRunLog を返して元の値を残す。
func (r *RunLogRepository) Save(_ context.Context, userID account.UserID, l *runlog.RunLog) error {
	if l == nil {
		return errors.New("ランの記録が nil である")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	byID := r.byUser[userID]
	if existing := byID[l.ID()]; existing != nil {
		if !existing.Equals(l) {
			return fmt.Errorf("%w: %s", runlog.ErrConflictingRunLog, l.ID())
		}
		return nil
	}
	if byID == nil {
		byID = map[runlog.RunLogID]*runlog.RunLog{}
		r.byUser[userID] = byID
	}
	byID[l.ID()] = l
	return nil
}

// Delete は打ち間違いの訂正。存在しないIDでも成功として扱う。
func (r *RunLogRepository) Delete(_ context.Context, userID account.UserID, id runlog.RunLogID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byUser[userID], id)
	return nil
}

var (
	_ runlog.Reader = (*RunLogRepository)(nil)
	_ runlog.Writer = (*RunLogRepository)(nil)
)
