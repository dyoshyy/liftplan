package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// ProgramRepository はユーザー設定の Postgres 実装。
//
// テーブルは1行に固定されている（id boolean PRIMARY KEY CHECK (id)）。
// 単一ユーザー前提を型で表しているので、2行目は作れない。
type ProgramRepository struct {
	pool *pgxpool.Pool
}

func NewProgramRepository(pool *pgxpool.Pool) *ProgramRepository {
	return &ProgramRepository{pool: pool}
}

// Get はプログラムを返す。未設定なら ErrProgramNotConfigured。
//
// (nil, nil) を返さない。返すと、呼び出し側が nil を「未設定」と
// 「取得成功」のどちらとも解釈できてしまう。
func (r *ProgramRepository) Get(ctx context.Context) (*training.Program, error) {
	var (
		perWeek                int
		rawTarget, rawSelected []byte
	)
	err := r.pool.QueryRow(ctx, `
		SELECT per_week, weekly_target, selected FROM program WHERE id`).
		Scan(&perWeek, &rawTarget, &rawSelected)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, training.ErrProgramNotConfigured
	}
	if err != nil {
		return nil, wrapUnavailable(err, "プログラムを読めない")
	}

	var target map[training.MuscleRegion]float64
	if err := json.Unmarshal(rawTarget, &target); err != nil {
		return nil, fmt.Errorf("週目標を解釈できない: %w", err)
	}
	var selected []training.ExerciseID
	if err := json.Unmarshal(rawSelected, &selected); err != nil {
		return nil, fmt.Errorf("選択種目を解釈できない: %w", err)
	}

	// 保存済みの値も必ずコンストラクタを通す。jsonb は形を検査しないので、
	// ここが唯一の防波堤になる。
	// NewProgram もゼロ値の頻度を弾くので、ここを省いても
	// 不正な値がドメインへ届くことはない。それでも通すのは、
	// 「週の頻度が設定されていない」ではなく「99回は範囲外」という
	// 診断が出るようにするため。原因の分かるエラーは運用の資産になる。
	frequency, err := training.NewFrequency(perWeek)
	if err != nil {
		return nil, fmt.Errorf("保存された頻度が不正: %w", err)
	}
	weeklyTarget, err := training.NewWeeklyVolumeTarget(target)
	if err != nil {
		return nil, fmt.Errorf("保存された週目標が不正: %w", err)
	}
	program, err := training.NewProgram(frequency, weeklyTarget, selected)
	if err != nil {
		return nil, fmt.Errorf("保存されたプログラムが不正: %w", err)
	}
	return program, nil
}

// Save はプログラムを保存する。プログラムはユーザーごとに1つなので、
// 保存は常に全体の置き換えになる。
func (r *ProgramRepository) Save(ctx context.Context, p *training.Program) error {
	if p == nil {
		return fmt.Errorf("プログラムが nil である")
	}

	target := map[training.MuscleRegion]float64{}
	for _, region := range p.WeeklyTarget().Regions() {
		target[region] = p.WeeklyTarget().Sets(region)
	}
	rawTarget, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("週目標を書き出せない: %w", err)
	}
	rawSelected, err := json.Marshal(p.SelectedExercises())
	if err != nil {
		return fmt.Errorf("選択種目を書き出せない: %w", err)
	}

	if _, err := r.pool.Exec(ctx, `
		INSERT INTO program (id, per_week, weekly_target, selected)
		VALUES (true, $1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET
			per_week      = EXCLUDED.per_week,
			weekly_target = EXCLUDED.weekly_target,
			selected      = EXCLUDED.selected`,
		p.Frequency().PerWeek(), rawTarget, rawSelected); err != nil {
		return fmt.Errorf("プログラムを保存できない: %w", err)
	}
	return nil
}

var _ training.ProgramRepository = (*ProgramRepository)(nil)
