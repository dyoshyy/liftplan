package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// OnboardingRepository は初回の案内の Postgres 実装。
type OnboardingRepository struct {
	pool *pgxpool.Pool
}

func NewOnboardingRepository(pool *pgxpool.Pool) *OnboardingRepository {
	return &OnboardingRepository{pool: pool}
}

// OnboardingCompleted は済ませた行があるかを返す。
func (r *OnboardingRepository) OnboardingCompleted(
	ctx context.Context, user account.UserID,
) (bool, error) {
	var done bool
	if err := r.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM onboarding WHERE user_id = $1)", user.String(),
	).Scan(&done); err != nil {
		return false, wrapUnavailable(err, "案内の状態を読めない")
	}
	return done, nil
}

// CompleteOnboarding は済ませた時刻を記録する。既にあれば最初の時刻を残す。
func (r *OnboardingRepository) CompleteOnboarding(
	ctx context.Context, user account.UserID, at time.Time,
) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO onboarding (user_id, completed_at) VALUES ($1, $2)
		ON CONFLICT (user_id) DO NOTHING`, user.String(), at); err != nil {
		return wrapUnavailable(err, "案内を済ませたことを保存できない")
	}
	return nil
}

var (
	_ account.OnboardingReader = (*OnboardingRepository)(nil)
	_ account.OnboardingWriter = (*OnboardingRepository)(nil)
)
