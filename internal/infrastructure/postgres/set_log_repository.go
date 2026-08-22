package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
		-- 照合順序を "C" に固定する。DB の既定に委ねると、ICU 照合の
		-- 環境で大小文字混在のIDの順序が変わり、インメモリ実装と
		-- 食い違う。順序が揺れると同じ入力から違う計画が出る。
		ORDER BY id COLLATE "C"`)
	if err != nil {
		return training.History{}, wrapUnavailable(err, "実績を読めない")
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
		return training.History{}, wrapUnavailable(err, "実績を読めない")
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
		return nil, wrapUnavailable(err, "実績を読めない")
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

// maxSaveAttempts は一時的な失敗に対する再試行の回数。
//
// デッドロックとシリアライズ失敗は、やり直せば成功する類の失敗。
// 500 を返すと、クライアントは「送り直しても無駄」と読んで諦める。
const maxSaveAttempts = 3

// Save は実績ログを保存する。
//
// INSERT ... ON CONFLICT DO NOTHING を撃ってから、対象の行を読み直して
// 内容を比べる。SELECT FOR UPDATE で先に締める形は使えない。
// READ COMMITTED では存在しない行を締められない（述語ロックが無い）ので、
// 同じIDがまだ無いとき全員が「新規」と判断し、後発が主キー違反を踏む。
// その経路で内容を見ずに衝突と決めつけると、内容が同一の再送まで 409 になる。
// 再送はこの設計が日常的に起こると想定しているものなので、これは致命的。
//
// トランザクションで包むことが、そのまま「全か無か」の実装になる。
func (r *SetLogRepository) Save(ctx context.Context, logs []*training.SetLog) error {
	if len(logs) == 0 {
		return nil
	}

	staged, ids, err := stageSetLogs(logs)
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := range maxSaveAttempts {
		err := r.saveOnce(ctx, staged, ids)
		if err == nil {
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		lastErr = err
		_ = attempt
	}
	return fmt.Errorf("実績の保存が競合し続けた: %w", lastErr)
}

// stageSetLogs は保存対象を重複解決してID順に並べる。
//
// ID順に固定するのは、行ロックを取る順序を揃えてデッドロックを消すため。
// map の反復順のまま流すと、同じ入力でも呼び出しごとに INSERT 順が変わり、
// 同一リクエストの二重送信どうしが互いを待って詰まる。
func stageSetLogs(logs []*training.SetLog) (map[training.SetLogID]*training.SetLog, []string, error) {
	staged := make(map[training.SetLogID]*training.SetLog, len(logs))
	for i, l := range logs {
		if l == nil {
			return nil, nil, fmt.Errorf("%d番目のセットログが nil である", i)
		}
		if prev, dup := staged[l.ID()]; dup {
			if !prev.Equals(l) {
				return nil, nil, fmt.Errorf("%w: %s", training.ErrConflictingSetLog, l.ID())
			}
			continue
		}
		staged[l.ID()] = l
	}

	ids := make([]string, 0, len(staged))
	for id := range staged {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	return staged, ids, nil
}

func (r *SetLogRepository) saveOnce(
	ctx context.Context,
	staged map[training.SetLogID]*training.SetLog,
	ids []string,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return wrapUnavailable(err, "トランザクションを開始できない")
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	batch := &pgx.Batch{}
	for _, id := range ids {
		l := staged[training.SetLogID(id)]
		batch.Queue(`
			INSERT INTO set_logs (id, performed_on, exercise_id, weight_kg, reps, rir)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (id) DO NOTHING`,
			string(l.ID()), toTime(l.PerformedOn()), string(l.ExerciseID()),
			l.Weight().Kg(), l.Reps().Int(), l.RIR().Int())
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("実績を保存できない: %w", err)
	}

	// 入らなかった行が「同じ内容だから」なのか「衝突だから」なのかを
	// ここで判別する。同じ内容なら黙って受け入れる。
	rows, err := tx.Query(ctx, `
		SELECT id, performed_on, exercise_id, weight_kg, reps, rir
		FROM set_logs
		WHERE id = ANY($1)`, ids)
	if err != nil {
		return fmt.Errorf("保存後の実績を読めない: %w", err)
	}
	stored := make(map[training.SetLogID]*training.SetLog, len(ids))
	for rows.Next() {
		log, err := scanSetLog(rows)
		if err != nil {
			rows.Close()
			return err
		}
		stored[log.ID()] = log
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("保存後の実績を読めない: %w", err)
	}

	for _, id := range ids {
		want := staged[training.SetLogID(id)]
		got, ok := stored[training.SetLogID(id)]
		if !ok {
			return fmt.Errorf("実績 %s が保存されていない", id)
		}
		if !got.Equals(want) {
			return fmt.Errorf("%w: %s", training.ErrConflictingSetLog, id)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("実績の保存をコミットできない: %w", err)
	}
	return nil
}

// isRetryable はやり直せば成功しうる失敗か。
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == pgerrcode.DeadlockDetected ||
		pgErr.Code == pgerrcode.SerializationFailure
}

// Delete は打ち間違いの訂正。存在しないIDでも成功として扱う。
//
// 影響行数を見ないのは、再送で二度目が来たときにエラーにしないため。
// 「消えているのに消せない」という状態を作らない。
func (r *SetLogRepository) Delete(ctx context.Context, id training.SetLogID) error {
	if _, err := r.pool.Exec(ctx,
		"DELETE FROM set_logs WHERE id = $1", string(id)); err != nil {
		return wrapUnavailable(err, "実績を削除できない")
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
