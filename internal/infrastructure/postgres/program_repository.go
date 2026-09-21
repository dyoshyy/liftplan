package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
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
func (r *ProgramRepository) Get(
	ctx context.Context, userID account.UserID,
) (*program.Program, error) {
	var (
		perWeek                             int
		rawTarget, rawSelected, rawDeclared []byte
		// 重点種目は指定なしが正当な既定値なので NULL を許す。
		rawFocus *string
		// 分割なしが正当な既定値なので NULL を許す。
		rawCycle []byte
	)
	err := r.pool.QueryRow(ctx, `
		SELECT per_week, weekly_target, selected, declared, focus, split_cycle
		FROM program WHERE user_id = $1`, userID.String()).
		Scan(&perWeek, &rawTarget, &rawSelected, &rawDeclared, &rawFocus, &rawCycle)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, program.ErrProgramNotConfigured
	}
	if err != nil {
		return nil, wrapUnavailable(err, "プログラムを読めない")
	}

	var target map[training.MuscleRegion]float64
	if err := json.Unmarshal(rawTarget, &target); err != nil {
		return nil, fmt.Errorf("週目標を解釈できない: %w", err)
	}
	var selected []exercise.ExerciseID
	if err := json.Unmarshal(rawSelected, &selected); err != nil {
		return nil, fmt.Errorf("選択種目を解釈できない: %w", err)
	}
	var declared []exercise.ExerciseID
	if err := json.Unmarshal(rawDeclared, &declared); err != nil {
		return nil, fmt.Errorf("宣言種目を解釈できない: %w", err)
	}

	// 保存済みの値も必ずコンストラクタを通す。jsonb は形を検査しないので、
	// ここが唯一の防波堤になる。
	// NewProgram もゼロ値の頻度を弾くので、ここを省いても
	// 不正な値がドメインへ届くことはない。それでも通すのは、
	// 「週の頻度が設定されていない」ではなく「99回は範囲外」という
	// 診断が出るようにするため。原因の分かるエラーは運用の資産になる。
	frequency, err := program.NewFrequency(perWeek)
	if err != nil {
		return nil, fmt.Errorf("保存された頻度が不正: %w", err)
	}
	weeklyTarget, err := program.NewWeeklyVolumeTarget(target)
	if err != nil {
		return nil, fmt.Errorf("保存された週目標が不正: %w", err)
	}
	var focus exercise.ExerciseID
	if rawFocus != nil {
		focus = exercise.ExerciseID(*rawFocus)
	}
	prog, err := program.NewProgram(frequency, weeklyTarget, selected, declared, focus)
	if err != nil {
		return nil, fmt.Errorf("保存されたプログラムが不正: %w", err)
	}

	if len(rawCycle) > 0 {
		var rows []splitRow
		if err := json.Unmarshal(rawCycle, &rows); err != nil {
			return nil, fmt.Errorf("分割を解釈できない: %w", err)
		}
		cycle := make([]program.Split, 0, len(rows))
		for _, row := range rows {
			s, err := program.NewSplit(row.Name, row.Regions)
			if err != nil {
				return nil, fmt.Errorf("保存された分割が不正: %w", err)
			}
			cycle = append(cycle, s)
		}
		prog, err = prog.WithCycle(cycle)
		if err != nil {
			return nil, fmt.Errorf("保存された分割が不正: %w", err)
		}
	}
	return prog, nil
}

// Save はプログラムを保存する。プログラムはユーザーごとに1つなので、
// 保存は常に全体の置き換えになる。主キーが user_id なので、
// 「1人につき1行」はスキーマが保つ。
func (r *ProgramRepository) Save(
	ctx context.Context, userID account.UserID, p *program.Program,
) error {
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
	rawDeclared, err := json.Marshal(p.DeclaredExercises())
	if err != nil {
		return fmt.Errorf("宣言種目を書き出せない: %w", err)
	}

	// 指定が無ければ NULL で保存する。空文字を入れると、読み出しで
	// NewExerciseID が弾いて「保存されたプログラムが不正」になる。
	var rawFocus *string
	if id, ok := p.FocusExercise(); ok {
		s := string(id)
		rawFocus = &s
	}

	// 分割なしは NULL。空配列と区別する必要は無いが、既存行と形を揃える。
	var rawCycle []byte
	if cycle := p.Cycle(); len(cycle) > 0 {
		rows := make([]splitRow, 0, len(cycle))
		for _, s := range cycle {
			rows = append(rows, splitRow{Name: s.Name(), Regions: s.Regions()})
		}
		rawCycle, err = json.Marshal(rows)
		if err != nil {
			return fmt.Errorf("分割を書き出せない: %w", err)
		}
	}

	if _, err := r.pool.Exec(ctx, `
		INSERT INTO program (user_id, per_week, weekly_target, selected, declared, focus, split_cycle)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			per_week      = EXCLUDED.per_week,
			weekly_target = EXCLUDED.weekly_target,
			selected      = EXCLUDED.selected,
			declared      = EXCLUDED.declared,
			focus         = EXCLUDED.focus,
			split_cycle   = EXCLUDED.split_cycle`,
		userID.String(), p.Frequency().PerWeek(),
		rawTarget, rawSelected, rawDeclared, rawFocus, rawCycle); err != nil {
		return fmt.Errorf("プログラムを保存できない: %w", err)
	}
	return nil
}

var (
	_ program.Reader = (*ProgramRepository)(nil)
	_ program.Writer = (*ProgramRepository)(nil)
)

// splitRow は分割1件の保存形。順序が周期そのものなので、配列の並びを保つ。
type splitRow struct {
	Name    string                  `json:"name"`
	Regions []training.MuscleRegion `json:"regions"`
}
