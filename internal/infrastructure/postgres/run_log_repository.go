package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/runlog"
)

// RunLogRepository はランの記録の Postgres 実装。
type RunLogRepository struct {
	pool *pgxpool.Pool
}

func NewRunLogRepository(pool *pgxpool.Pool) *RunLogRepository {
	return &RunLogRepository{pool: pool}
}

// FindAll は実施日の昇順、同じ日の中は ID の昇順で返す。照合順序を "C" に
// 固定するのは set_logs と同じ理由（インメモリ実装と順序を揃える）。
func (r *RunLogRepository) FindAll(ctx context.Context, userID account.UserID) ([]*runlog.RunLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, performed_on, distance_m, duration_seconds
		FROM run_logs
		WHERE user_id = $1
		ORDER BY performed_on, id COLLATE "C"`, userID.String())
	if err != nil {
		return nil, wrapUnavailable(err, "ランの記録を読めない")
	}
	defer rows.Close()

	var out []*runlog.RunLog
	for rows.Next() {
		l, err := scanRunLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapUnavailable(err, "ランの記録を読めない")
	}
	return out, nil
}

func scanRunLog(row pgx.Row) (*runlog.RunLog, error) {
	var (
		id                string
		performedOn       time.Time
		distanceM, durSec int
	)
	if err := row.Scan(&id, &performedOn, &distanceM, &durSec); err != nil {
		return nil, wrapUnavailable(err, "ランの記録を読めない")
	}
	date, err := training.FromTime(performedOn, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("ランの記録 %s の日付が不正: %w", id, err)
	}
	// 保存済みの値も必ずコンストラクタを通す。DB に不正な値が入っていても、
	// ここで止めればドメインには届かない。
	l, err := runlog.NewRunLog(runlog.RunLogParams{
		ID: id, PerformedOn: date,
		DistanceKm: float64(distanceM) / 1000, DurationSeconds: durSec,
	})
	if err != nil {
		return nil, fmt.Errorf("ランの記録 %s が不正: %w", id, err)
	}
	return l, nil
}

// Save は INSERT ... ON CONFLICT DO NOTHING を撃ってから行を読み直し、内容を
// 比べる。先に SELECT で有無を見る形にしないのは SetLogRepository.Save と
// 同じ理由（READ COMMITTED では存在しない行を締められず、同時の再送が
// 主キー違反を踏む）。1行なのでトランザクションは要らない。
func (r *RunLogRepository) Save(ctx context.Context, userID account.UserID, l *runlog.RunLog) error {
	if l == nil {
		return errors.New("ランの記録が nil である")
	}
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO run_logs (user_id, id, performed_on, distance_m, duration_seconds)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, id) DO NOTHING`,
		userID.String(), string(l.ID()), toTime(l.PerformedOn()),
		l.Distance().Meters(), l.Duration().Seconds()); err != nil {
		return wrapUnavailable(err, "ランの記録を保存できない")
	}

	stored, err := scanRunLog(r.pool.QueryRow(ctx, `
		SELECT id, performed_on, distance_m, duration_seconds
		FROM run_logs
		WHERE user_id = $1 AND id = $2`, userID.String(), string(l.ID())))
	if err != nil {
		return err
	}
	if !stored.Equals(l) {
		return fmt.Errorf("%w: %s", runlog.ErrConflictingRunLog, l.ID())
	}
	return nil
}

// Delete は打ち間違いの訂正。存在しないIDでも成功として扱う。所有者で
// 絞るので、他人の同じIDには届かない。
func (r *RunLogRepository) Delete(ctx context.Context, userID account.UserID, id runlog.RunLogID) error {
	if _, err := r.pool.Exec(ctx,
		"DELETE FROM run_logs WHERE user_id = $1 AND id = $2",
		userID.String(), string(id)); err != nil {
		return wrapUnavailable(err, "ランの記録を削除できない")
	}
	return nil
}

var (
	_ runlog.Reader = (*RunLogRepository)(nil)
	_ runlog.Writer = (*RunLogRepository)(nil)
)
