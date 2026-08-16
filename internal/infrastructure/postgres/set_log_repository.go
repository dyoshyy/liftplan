package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// SetLogRepository は実績ログの Postgres 実装。
type SetLogRepository struct {
	pool *pgxpool.Pool
}

func NewSetLogRepository(pool *pgxpool.Pool) *SetLogRepository {
	return &SetLogRepository{pool: pool}
}

// FindAll は全ログを ID の昇順で返す。
//
// 順序を固定するのは、揺れると History の重複解決や推定1RMの畳み込みが
// 呼び出しごとに変わり、同じ入力から違う計画が出るため。
func (r *SetLogRepository) FindAll(ctx context.Context) (training.History, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, performed_on, exercise_id, weight_kg, reps, rir
		FROM set_logs
		ORDER BY id`)
	if err != nil {
		return training.History{}, fmt.Errorf("実績を読めない: %w", err)
	}
	defer rows.Close()

	var out []*training.SetLog
	for rows.Next() {
		log, err := scanSetLog(rows)
		if err != nil {
			return training.History{}, err
		}
		out = append(out, log)
	}
	if err := rows.Err(); err != nil {
		return training.History{}, fmt.Errorf("実績を読めない: %w", err)
	}
	return training.NewHistory(out), nil
}

func scanSetLog(rows pgx.Rows) (*training.SetLog, error) {
	var (
		id, exerciseID string
		performedOn    time.Time
		weightKg       float64
		reps, rir      int
	)
	if err := rows.Scan(&id, &performedOn, &exerciseID, &weightKg, &reps, &rir); err != nil {
		return nil, fmt.Errorf("実績を読めない: %w", err)
	}

	date, err := training.FromTime(performedOn, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("実績 %s の日付が不正: %w", id, err)
	}

	// 保存済みの値も必ずコンストラクタを通す。DB に不正な値が
	// 入っていても、ここで止めればドメインには届かない。
	log, err := training.NewSetLog(training.SetLogParams{
		ID: id, PerformedOn: date, ExerciseID: exerciseID,
		WeightKg: weightKg, Reps: reps, RIR: rir,
	})
	if err != nil {
		return nil, fmt.Errorf("実績 %s が不正: %w", id, err)
	}
	return log, nil
}

// Save は実績ログを保存する。
//
// トランザクションで包むことが、そのまま「全か無か」の実装になる。
// 衝突を1件でも見つけたら1件も書かない。半分だけ保存された状態は、
// その週の刺激量を実態とずらしたまま計画に効き続ける。
//
// ON CONFLICT DO NOTHING を使わないのは、「入らなかった理由が
// 同一だからか衝突だからか」を後から判別する必要があり、往復が増えるため。
func (r *SetLogRepository) Save(ctx context.Context, logs []*training.SetLog) error {
	if len(logs) == 0 {
		return nil
	}

	staged := make(map[training.SetLogID]*training.SetLog, len(logs))
	ids := make([]string, 0, len(logs))
	for i, l := range logs {
		if l == nil {
			return fmt.Errorf("%d番目のセットログが nil である", i)
		}
		if prev, dup := staged[l.ID()]; dup {
			if !prev.Equals(l) {
				return fmt.Errorf("%w: %s", training.ErrConflictingSetLog, l.ID())
			}
			continue
		}
		staged[l.ID()] = l
		ids = append(ids, string(l.ID()))
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("トランザクションを開始できない: %w", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	// 対象の行を締めてから比較する。締めないと、比較と INSERT の間に
	// 別のトランザクションが同じIDを書き、衝突を見逃す。
	rows, err := tx.Query(ctx, `
		SELECT id, performed_on, exercise_id, weight_kg, reps, rir
		FROM set_logs
		WHERE id = ANY($1)
		FOR UPDATE`, ids)
	if err != nil {
		return fmt.Errorf("既存の実績を読めない: %w", err)
	}

	existing := make(map[training.SetLogID]*training.SetLog, len(ids))
	for rows.Next() {
		log, err := scanSetLog(rows)
		if err != nil {
			rows.Close()
			return err
		}
		existing[log.ID()] = log
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("既存の実績を読めない: %w", err)
	}

	insert := make([]*training.SetLog, 0, len(staged))
	for id, l := range staged {
		prev, ok := existing[id]
		if !ok {
			insert = append(insert, l)
			continue
		}
		// 同じ内容の再送は黙って受け入れる。クライアントは
		// タイムアウト後に再送するので、これは日常的に起きる。
		if !prev.Equals(l) {
			return fmt.Errorf("%w: %s", training.ErrConflictingSetLog, id)
		}
	}

	if len(insert) > 0 {
		batch := &pgx.Batch{}
		for _, l := range insert {
			batch.Queue(`
				INSERT INTO set_logs (id, performed_on, exercise_id, weight_kg, reps, rir)
				VALUES ($1, $2, $3, $4, $5, $6)`,
				string(l.ID()), toTime(l.PerformedOn()), string(l.ExerciseID()),
				l.Weight().Kg(), l.Reps().Int(), l.RIR().Int())
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			// FOR UPDATE で読めなかった行を、別のトランザクションが
			// 先に入れた場合。主キー違反として返るので衝突に翻訳する。
			// 生のエラーのままだと 500 になり、クライアントは自分の
			// ID 採番ミスに気づかず再送を繰り返す。
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
				return fmt.Errorf("%w: 保存中に別の書き込みと衝突した", training.ErrConflictingSetLog)
			}
			return fmt.Errorf("実績を保存できない: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("実績の保存をコミットできない: %w", err)
	}
	return nil
}

// toTime はドメインの日付を DB に渡せる形にする。
//
// 場所を UTC に固定するのは、date 型に時刻もタイムゾーンも無いため。
// ローカルタイムで渡すと、サーバーのタイムゾーン設定で日付が1日ずれる。
func toTime(d training.Date) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

var _ training.SetLogRepository = (*SetLogRepository)(nil)
