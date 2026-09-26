package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// ExerciseRepository は種目の Postgres 実装。
//
// 共通の種目はバイナリ同梱のシードをそのまま返し、利用者が足した種目
// だけを custom_exercises から読む。合わせる場所はここ1箇所。
type ExerciseRepository struct {
	pool *pgxpool.Pool
	seed []*exercise.Exercise
}

func NewExerciseRepository(pool *pgxpool.Pool, seed []*exercise.Exercise) *ExerciseRepository {
	copied := make([]*exercise.Exercise, len(seed))
	copy(copied, seed)
	return &ExerciseRepository{pool: pool, seed: copied}
}

// FindAll はシード（生成時の順）の後ろに、その利用者の種目を ID 昇順で
// 並べて返す。
func (r *ExerciseRepository) FindAll(ctx context.Context, user account.UserID) ([]*exercise.Exercise, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, primary_regions, secondary_regions, increment_kg, deleted_at IS NOT NULL
		FROM custom_exercises WHERE user_id = $1 ORDER BY id`, user.String())
	if err != nil {
		return nil, wrapUnavailable(err, "種目を読めない")
	}
	defer rows.Close()

	out := make([]*exercise.Exercise, 0, len(r.seed))
	out = append(out, r.seed...)
	for rows.Next() {
		var (
			id, name                 string
			rawPrimary, rawSecondary []byte
			inc                      float64
			deleted                  bool
		)
		if err := rows.Scan(&id, &name, &rawPrimary, &rawSecondary, &inc, &deleted); err != nil {
			return nil, wrapUnavailable(err, "種目を読めない")
		}
		var primary, secondary []training.MuscleRegion
		if err := json.Unmarshal(rawPrimary, &primary); err != nil {
			return nil, fmt.Errorf("種目 %s の主に効く部位を解釈できない: %w", id, err)
		}
		if err := json.Unmarshal(rawSecondary, &secondary); err != nil {
			return nil, fmt.Errorf("種目 %s の少し効く部位を解釈できない: %w", id, err)
		}
		// 保存済みの値も必ずコンストラクタを通す（ProgramRepository.Get と同じ理由）。
		e, err := exercise.NewCustomExercise(exercise.CustomExerciseParams{
			ID: id, Name: name, Primary: primary, Secondary: secondary, IncrementKg: inc,
		})
		if err != nil {
			return nil, fmt.Errorf("保存済みの種目が不正: %w", err)
		}
		if deleted {
			e = e.Delete()
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapUnavailable(err, "種目を読めない")
	}
	return out, nil
}

// Save は利用者が足した種目を保存する。同じ ID は上書きする。
func (r *ExerciseRepository) Save(ctx context.Context, user account.UserID, e *exercise.Exercise) error {
	if e == nil || !e.IsCustom() {
		return errors.New("共通の種目は保存できない")
	}
	primary, _ := json.Marshal(e.PrimaryRegions())
	secondary, _ := json.Marshal(e.SecondaryRegions())

	// 消した時刻は最初に消したときのまま。二度消しても動かさない。
	_, err := r.pool.Exec(ctx, `
		INSERT INTO custom_exercises
			(user_id, id, name, primary_regions, secondary_regions, increment_kg, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, CASE WHEN $7 THEN now() END)
		ON CONFLICT (user_id, id) DO UPDATE SET
			deleted_at = CASE WHEN $7 THEN COALESCE(custom_exercises.deleted_at, now()) END`,
		user.String(), string(e.ID()), e.Name(), primary, secondary, e.Increment().Kg(), e.IsDeleted())
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "custom_exercises_alive_name" {
		return fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, e.Name())
	}
	if err != nil {
		return wrapUnavailable(err, "種目を保存できない")
	}
	return nil
}

var (
	_ exercise.Reader = (*ExerciseRepository)(nil)
	_ exercise.Writer = (*ExerciseRepository)(nil)
)
