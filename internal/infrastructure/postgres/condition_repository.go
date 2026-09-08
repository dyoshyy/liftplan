package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// ConditionRepository は日次コンディションの Postgres 実装。
type ConditionRepository struct {
	pool *pgxpool.Pool
}

func NewConditionRepository(pool *pgxpool.Pool) *ConditionRepository {
	return &ConditionRepository{pool: pool}
}

// FindAll は全コンディションを返す。
//
// 並べ替えないのは NewConditionLog が日付で整列するため。ここで
// 並べても結果は変わらず、意味のある処理に見えて実は何もしていない
// コードが残るだけになる。
func (r *ConditionRepository) FindAll(ctx context.Context) (training.ConditionLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT date, body_weight_kg, sleep_hours FROM daily_conditions`)
	if err != nil {
		return training.ConditionLog{}, wrapUnavailable(err, "コンディションを読めない")
	}
	defer rows.Close()

	var out []training.DailyCondition
	for rows.Next() {
		c, err := scanCondition(rows)
		if err != nil {
			return training.ConditionLog{}, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return training.ConditionLog{}, wrapUnavailable(err, "コンディションを読めない")
	}
	return training.NewConditionLog(out), nil
}

func scanCondition(rows pgx.Rows) (training.DailyCondition, error) {
	var (
		date       time.Time
		bodyWeight *float64
		sleepHours *float64
	)
	if err := rows.Scan(&date, &bodyWeight, &sleepHours); err != nil {
		return training.DailyCondition{}, wrapUnavailable(err, "コンディションを読めない")
	}

	d, err := training.FromTime(date, time.UTC)
	if err != nil {
		return training.DailyCondition{}, fmt.Errorf("コンディションの日付が不正: %w", err)
	}

	// ポインタで受けるのは、欠損と 0 を区別するため。非ポインタだと
	// 体重の欠損が 0kg、睡眠の欠損が 0時間になり、どちらも有意味な値と
	// 区別できなくなる。
	c := training.NewDailyCondition(d)
	if bodyWeight != nil {
		c = c.WithBodyWeight(*bodyWeight)
	}
	if sleepHours != nil {
		c = c.WithSleepHours(*sleepHours)
	}
	return c, nil
}

// Save は日次コンディションを保存する。
//
// 同じ日付は項目ごとに上書きする。日付ごと置き換えると、体重だけを
// 送ったときに睡眠時間が消える。クライアントは体重と睡眠を別のタイミングで
// 記録するので、これは日常的に起きる。
//
// EXCLUDED が NULL のときに既存値を残す COALESCE が「項目ごとの上書き」。
func (r *ConditionRepository) Save(ctx context.Context, items []training.DailyCondition) error {
	if len(items) == 0 {
		return nil
	}

	// 同一呼び出し内の重複を先に合成する。1文ずつ流すと、同じ日付が
	// 2件あったときに ON CONFLICT が同一コマンド内で二度当たり、
	// Postgres が「行を二度更新できない」と拒否する。
	staged := make(map[training.Date]training.DailyCondition, len(items))
	order := make([]training.Date, 0, len(items))
	for i, c := range items {
		// 日付の無い記録を黙って捨てない。捨てると、クライアントは
		// 保存に成功したと思ったまま記録が消える。
		if c.Date().IsZero() {
			return fmt.Errorf("%d番目のコンディションに日付が無い", i)
		}
		if prev, ok := staged[c.Date()]; ok {
			staged[c.Date()] = prev.Merge(c)
			continue
		}
		staged[c.Date()] = c
		order = append(order, c.Date())
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return wrapUnavailable(err, "トランザクションを開始できない")
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	batch := &pgx.Batch{}
	for _, date := range order {
		c := staged[date]
		batch.Queue(`
			INSERT INTO daily_conditions (date, body_weight_kg, sleep_hours)
			VALUES ($1, $2, $3)
			ON CONFLICT (date) DO UPDATE SET
				body_weight_kg = COALESCE(EXCLUDED.body_weight_kg, daily_conditions.body_weight_kg),
				sleep_hours    = COALESCE(EXCLUDED.sleep_hours, daily_conditions.sleep_hours)`,
			toTime(date), optional(c.BodyWeightKg()), optional(c.SleepHours()))
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("コンディションを保存できない: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("コンディションの保存をコミットできない: %w", err)
	}
	return nil
}

// optional は「欠損しうる値」を NULL 込みで渡せる形にする。
func optional(v float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	return &v
}

var (
	_ training.ConditionReader = (*ConditionRepository)(nil)
	_ training.ConditionWriter = (*ConditionRepository)(nil)
)
