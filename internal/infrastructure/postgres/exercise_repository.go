package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// ExerciseRepository は種目の Postgres 実装。
//
// 種目は共通/個人の2つに分かれていない。利用者の行が1件も無いときに
// 一度だけシードをコピーする。消した行も件数に数えるので、全部消しても
// プリセットが入り直らない。以後は user_exercises の行がその人の種目一覧
// そのもの（docs/specs/2026-09-26-custom-exercises-design.md「いつコピーするか」）。
type ExerciseRepository struct {
	pool *pgxpool.Pool
	seed []*exercise.Exercise
}

func NewExerciseRepository(pool *pgxpool.Pool, seed []*exercise.Exercise) *ExerciseRepository {
	copied := make([]*exercise.Exercise, len(seed))
	copy(copied, seed)
	return &ExerciseRepository{pool: pool, seed: copied}
}

// FindAll はその利用者の一覧を ID 昇順（COLLATE "C"）で返す。行が1件も
// 無ければ、シードを1つのトランザクションでコピーしてから読む。
func (r *ExerciseRepository) FindAll(ctx context.Context, user account.UserID) ([]*exercise.Exercise, error) {
	if err := r.ensureSeeded(ctx, user); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, name, stimulus, increment_kg, bodyweight_factor, derived_from,
			deleted_at IS NOT NULL
		FROM user_exercises WHERE user_id = $1 ORDER BY id COLLATE "C"`, user.String())
	if err != nil {
		return nil, wrapUnavailable(err, "種目を読めない")
	}
	defer rows.Close()

	out := make([]*exercise.Exercise, 0, len(r.seed))
	for rows.Next() {
		var (
			id, name    string
			rawStimulus []byte
			inc         float64
			bwFactor    float64
			derivedFrom *string
			deleted     bool
		)
		if err := rows.Scan(&id, &name, &rawStimulus, &inc, &bwFactor, &derivedFrom, &deleted); err != nil {
			return nil, wrapUnavailable(err, "種目を読めない")
		}
		var rawMap map[training.MuscleRegion]float64
		if err := json.Unmarshal(rawStimulus, &rawMap); err != nil {
			return nil, fmt.Errorf("種目 %s の効き方を解釈できない: %w", id, err)
		}
		params := exercise.ExerciseParams{
			ID: id, Name: name, Stimulus: rawMap,
			IncrementKg: inc, BodyweightFactor: bwFactor,
		}
		if derivedFrom != nil {
			params.DerivedFrom = *derivedFrom
		}
		// 保存済みの値も必ずコンストラクタを通す（ProgramRepository.Get と同じ理由）。
		e, err := exercise.NewExercise(params)
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

// ensureSeeded は、その利用者の行が1件も無ければシードを全部コピーする。
//
// 同時に2回呼ばれても安全なのは ON CONFLICT DO NOTHING が二重挿入を吸収
// するからで、トランザクションが守っているのはそこではない。トランザクション
// が要るのは原子性のため。バッチの途中（何行か挿入した後）で失敗すると、
// ロールバックしない限りシードが半端に入った行が残り、count はもう0件では
// ないので次回以降 ensureSeeded がスキップし続け、二度と直らない。
//
// 対象（arbiter）を (user_id, id) に絞らないのは、それだと user_exercises_alive_name
// （名前の部分一意索引）との衝突が素通しになるため。同じ利用者に同じシードを
// 同時に2回コピーしようとすると、id だけでなく名前も同じ行がぶつかる。
// arbiter を指定した ON CONFLICT は「その索引以外」の一意違反を吸収しない
// （Postgres の仕様）ので、無指定にして両方の索引の衝突を吸収する。
// 実測（手で確認）：同じ利用者へ2つの goroutine から同時に初回読み出しを
// 走らせると、arbiter を (user_id, id) に絞った版は user_exercises_alive_name
// の一意違反で時々失敗した。
func (r *ExerciseRepository) ensureSeeded(ctx context.Context, user account.UserID) error {
	if len(r.seed) == 0 {
		return nil
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return wrapUnavailable(err, "種目を読めない")
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	var count int
	if err := tx.QueryRow(ctx,
		"SELECT count(*) FROM user_exercises WHERE user_id = $1", user.String(),
	).Scan(&count); err != nil {
		return wrapUnavailable(err, "種目を読めない")
	}
	if count > 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, e := range r.seed {
		stimulus, err := marshalStimulus(e)
		if err != nil {
			return err
		}
		var derivedFrom *string
		if from, ok := e.DerivedFrom(); ok {
			s := string(from)
			derivedFrom = &s
		}
		batch.Queue(`
			INSERT INTO user_exercises
				(user_id, id, name, stimulus, increment_kg, bodyweight_factor, derived_from)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT DO NOTHING`,
			user.String(), string(e.ID()), e.Name(), stimulus,
			e.Increment().Kg(), e.BodyweightFactor().Float(), derivedFrom)
	}
	br := tx.SendBatch(ctx, batch)
	for range r.seed {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return wrapUnavailable(err, "種目を初期化できない")
		}
	}
	if err := br.Close(); err != nil {
		return wrapUnavailable(err, "種目を初期化できない")
	}

	// 旧版（0013 の custom_exercises）で足した種目も、同じトランザクションで
	// 取り込む。旧版は「主に効く」「少し効く」の区分の配列を持っていたので、
	// 主を1.0、少しを0.5の寄与に直す（旧版の exercise.NewCustomExercise と
	// 同じ対応）。消した行は消したまま、足した時刻も引き継ぐ。
	//
	// ここで取り込むのは、プリセットのコピーと同じ「その人の行が0件のとき」
	// だけ。別の時点で取り込むと、プリセットが入った後の一覧に旧版の種目が
	// 混ざる順序が利用者ごとに変わり、0件の判定とも噛み合わない。
	//
	// 全員が取り込み終えたら custom_exercises ごと消せる（そのときにこの文も消す）。
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_exercises
			(user_id, id, name, stimulus, increment_kg, created_at, deleted_at)
		SELECT c.user_id, c.id, c.name,
			(SELECT jsonb_object_agg(r.region, r.contribution) FROM (
				SELECT jsonb_array_elements_text(c.primary_regions) AS region, 1.0 AS contribution
				UNION ALL
				SELECT jsonb_array_elements_text(c.secondary_regions), 0.5
			) r),
			c.increment_kg, c.created_at, c.deleted_at
		FROM custom_exercises c
		WHERE c.user_id = $1
		ON CONFLICT DO NOTHING`, user.String()); err != nil {
		return wrapUnavailable(err, "旧版の種目を取り込めない")
	}

	if err := tx.Commit(ctx); err != nil {
		return wrapUnavailable(err, "種目を初期化できない")
	}
	return nil
}

// Save は種目を保存する。プリセット由来かどうかで扱いを変えない。
// 同じ ID は上書きする。消した時刻は最初に消したときのまま保つ
// （二度消しても動かさない）。
func (r *ExerciseRepository) Save(ctx context.Context, user account.UserID, e *exercise.Exercise) error {
	if e == nil {
		return errors.New("種目が nil である")
	}
	// FindAll 同様、その利用者の最初の書き込みならプリセットを入れてから
	// 保存する。ここを飛ばすと、Save が先に呼ばれた利用者だけシードの
	// コピーが無いまま進み、名前の重複判定（部分一意索引）がシードを
	// 見落とす。
	if err := r.ensureSeeded(ctx, user); err != nil {
		return err
	}
	stimulus, err := marshalStimulus(e)
	if err != nil {
		return err
	}
	var derivedFrom *string
	if from, ok := e.DerivedFrom(); ok {
		s := string(from)
		derivedFrom = &s
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO user_exercises
			(user_id, id, name, stimulus, increment_kg, bodyweight_factor, derived_from, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $8 THEN now() END)
		ON CONFLICT (user_id, id) DO UPDATE SET
			name              = EXCLUDED.name,
			stimulus          = EXCLUDED.stimulus,
			increment_kg      = EXCLUDED.increment_kg,
			bodyweight_factor = EXCLUDED.bodyweight_factor,
			derived_from      = EXCLUDED.derived_from,
			deleted_at        = CASE WHEN $8 THEN COALESCE(user_exercises.deleted_at, now()) END`,
		user.String(), string(e.ID()), e.Name(), stimulus,
		e.Increment().Kg(), e.BodyweightFactor().Float(), derivedFrom, e.IsDeleted())
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "user_exercises_alive_name" {
		return fmt.Errorf("%w: %s", exercise.ErrDuplicateExerciseName, e.Name())
	}
	if err != nil {
		return wrapUnavailable(err, "種目を保存できない")
	}
	return nil
}

// marshalStimulus は効き方を jsonb 列に入れる形にする。
func marshalStimulus(e *exercise.Exercise) ([]byte, error) {
	m := make(map[training.MuscleRegion]float64, len(e.Stimulus().Regions()))
	for _, r := range e.Stimulus().Regions() {
		c, _ := e.Stimulus().Contribution(r)
		m[r] = c.Float()
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("種目 %s の効き方を保存できない: %w", e.ID(), err)
	}
	return out, nil
}

var (
	_ exercise.Reader = (*ExerciseRepository)(nil)
	_ exercise.Writer = (*ExerciseRepository)(nil)
)
