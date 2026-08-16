package training

import (
	"context"
	"errors"
)

// ErrProgramNotConfigured はユーザーがまだプログラムを設定していないことを表す。
var ErrProgramNotConfigured = errors.New("プログラムが未設定である")

// ExerciseRepository は種目マスタの取得口。
type ExerciseRepository interface {
	FindAll(ctx context.Context) ([]*Exercise, error)
}

// SetLogRepository は実績ログの永続化口。
//
// Save は冪等でなければならない。クライアントが採番した ID を使い、
// 同じログを二度送っても重複が生まれない実装にすること。
type SetLogRepository interface {
	FindAll(ctx context.Context) (History, error)
	Save(ctx context.Context, logs []*SetLog) error
}

// ConditionRepository は日次コンディションの永続化口。
//
// Save は冪等でなければならない。同じ日付を二度送ったら上書きになる実装にすること。
type ConditionRepository interface {
	FindAll(ctx context.Context) (ConditionLog, error)
	Save(ctx context.Context, items []DailyCondition) error
}

// ProgramRepository はユーザー設定の取得口。
// 未設定の場合は ErrProgramNotConfigured を返す。
type ProgramRepository interface {
	Get(ctx context.Context) (*Program, error)
}
