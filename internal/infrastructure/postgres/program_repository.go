package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// ProgramRepository はユーザー設定の Postgres 実装。
//
// 主キーは user_id。行は利用者の数だけあり、読みも書きも必ず
// user_id で絞る。絞り忘れると他人の設定が返る。
//
// 「テーブル全体で1行」に固定していた id boolean PRIMARY KEY CHECK (id) は
// もう無い（0007 で主キーを user_id に移し、0009 で列ごと落とした）。
// 0001 と 0007 の SQL コメントには当時の説明が残っているが、適用済みの
// マイグレーションはチェックサムで照合しているので書き換えられない。
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
		perWeek                              int
		exercisesPerSession, setsPerExercise int
		rawSelected, rawDeclared             []byte
		// 重点種目は指定なしが正当な既定値なので NULL を許す。
		rawFocus *string
		// 分割なしが正当な既定値なので NULL を許す。
		rawCycle []byte
		// レップ数は設定した宣言だけを持つ。設定なしが正当な既定値なので NULL を許す。
		rawReps []byte
	)
	err := r.pool.QueryRow(ctx, `
		SELECT per_week, exercises_per_session, sets_per_exercise,
		       selected, declared, focus, split_cycle, declared_reps
		FROM program WHERE user_id = $1`, userID.String()).
		Scan(&perWeek, &exercisesPerSession, &setsPerExercise,
			&rawSelected, &rawDeclared, &rawFocus, &rawCycle, &rawReps)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, program.ErrProgramNotConfigured
	}
	if err != nil {
		return nil, wrapUnavailable(err, "プログラムを読めない")
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
	volume, err := program.NewSessionVolume(exercisesPerSession, setsPerExercise)
	if err != nil {
		return nil, fmt.Errorf("保存された1回の量が不正: %w", err)
	}
	var focus exercise.ExerciseID
	if rawFocus != nil {
		focus = exercise.ExerciseID(*rawFocus)
	}
	prog, err := program.NewProgram(frequency, volume, selected, declared, focus)
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

	if len(rawReps) > 0 {
		var rows map[string]repTargetsRow
		if err := json.Unmarshal(rawReps, &rows); err != nil {
			return nil, fmt.Errorf("宣言ごとのレップ数を解釈できない: %w", err)
		}
		// キーの順で当てる。どれが不正かの診断が毎回同じ種目を指すように。
		for _, id := range slices.Sorted(maps.Keys(rows)) {
			row := rows[id]
			reps, err := program.NewRepTargets(row.Heavy, row.Light)
			if err != nil {
				return nil, fmt.Errorf("保存された %s のレップ数が不正: %w", id, err)
			}
			prog, err = prog.WithRepTargets(exercise.ExerciseID(id), reps)
			if err != nil {
				return nil, fmt.Errorf("保存された %s のレップ数が不正: %w", id, err)
			}
		}
	}
	return prog, nil
}

// Save はプログラムを保存する。プログラムはユーザーごとに1つなので、
// 保存は常に全体の置き換えになる。主キーが user_id なので、
// 「1人につき1行」はスキーマが保つ。
func (r *ProgramRepository) Save(
	ctx context.Context, userID account.UserID, p *program.Program,
) (err error) {
	// 出口で1度だけ包む。return ごとに包むと、経路が増えたときに包み忘れた
	// 1本だけが 500 で返る。中では wrapUnavailable を呼ばない（文言と
	// ErrRepositoryUnavailable が二重になる）。
	defer func() { err = wrapUnavailable(err, "プログラムを保存できない") }()

	if p == nil {
		return fmt.Errorf("プログラムが nil である")
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

	// 設定が無ければ NULL。既存の行と形を揃える。
	var rawReps []byte
	if reps := p.DeclaredRepTargets(); len(reps) > 0 {
		rows := make(map[string]repTargetsRow, len(reps))
		for id, t := range reps {
			rows[string(id)] = repTargetsRow{Heavy: t.Heavy(), Light: t.Light()}
		}
		rawReps, err = json.Marshal(rows)
		if err != nil {
			return fmt.Errorf("宣言ごとのレップ数を書き出せない: %w", err)
		}
	}

	if _, err := r.pool.Exec(ctx, `
		INSERT INTO program (user_id, per_week, exercises_per_session, sets_per_exercise,
		                     selected, declared, focus, split_cycle, declared_reps)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id) DO UPDATE SET
			per_week              = EXCLUDED.per_week,
			exercises_per_session = EXCLUDED.exercises_per_session,
			sets_per_exercise     = EXCLUDED.sets_per_exercise,
			selected              = EXCLUDED.selected,
			declared              = EXCLUDED.declared,
			focus                 = EXCLUDED.focus,
			split_cycle           = EXCLUDED.split_cycle,
			declared_reps         = EXCLUDED.declared_reps`,
		userID.String(), p.Frequency().PerWeek(),
		p.SessionVolume().Exercises(), p.SessionVolume().Sets(),
		rawSelected, rawDeclared, rawFocus, rawCycle, rawReps); err != nil {
		return fmt.Errorf("プログラムを書き込めない: %w", err)
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

// repTargetsRow は宣言1件ぶんのレップ数の保存形。
type repTargetsRow struct {
	Heavy int `json:"heavy"`
	Light int `json:"light"`
}
