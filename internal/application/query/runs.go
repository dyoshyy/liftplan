package query

import (
	"context"
	"fmt"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/runlog"
)

// Runs はランの記録を期間で読む。
type Runs struct {
	repo runlog.Reader
}

func NewRuns(repo runlog.Reader) *Runs {
	return &Runs{repo: repo}
}

// Between は [from, to] のランを、実施日の昇順で返す。境界の日を含む。
//
// 期間で絞るのはここ。リポジトリは全件を返す（setlog.Reader と同じ形）。
// 1人ぶんのランの記録は年に数百件で、口を増やすほどの量ではない。
func (q *Runs) Between(
	ctx context.Context, user account.UserID, from, to training.Date,
) (_ []*runlog.RunLog, err error) {
	defer func() { err = apperror.Classify(err) }()

	all, err := q.repo.FindAll(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("ランの記録の取得に失敗: %w", err)
	}
	out := make([]*runlog.RunLog, 0, len(all))
	for _, l := range all {
		if !l.PerformedOn().Before(from) && !l.PerformedOn().After(to) {
			out = append(out, l)
		}
	}
	return out, nil
}
