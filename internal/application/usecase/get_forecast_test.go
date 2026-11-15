package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

func newGetForecast(t *testing.T, logs *fakeLogs, conditions *fakeConditions, prog *fakeProgram) *usecase.GetForecast {
	t.Helper()
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	return usecase.NewGetForecast(
		&fakeExercises{all: pool}, logs, conditions, prog,
		planning.DefaultSessionPlanner(),
	)
}

// 見込みが頻度ぶん返ること。回0が Plan と同じセッションになること自体は
// ドメイン層（TestSessionPlanner_Forecast_FirstSessionMatchesPlan）が
// 守るので、ここでは配線（頻度ぶん返ってくること）だけを見る。
func TestGetForecast_ReturnsOneSessionPerFrequency(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetForecast(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)

	got, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{Date: testDate})
	if err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(got) != 3 { // buildProgram は週3
		t.Errorf("回数が %d。週3のはず", len(got))
	}
}

func TestGetForecast_PropagatesProgramNotConfigured(t *testing.T) {
	uc := newGetForecast(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{err: program.ErrProgramNotConfigured},
	)
	_, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{Date: testDate})
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("未設定エラーが伝播していない: %v", err)
	}
}

// リポジトリが契約に反して (nil, nil) を返しても、未設定として扱えること
// （GetSession の同名テストと同じ理由。コピー元と同じ穴を踏みやすい）。
func TestGetForecast_TreatsNilProgramAsNotConfigured(t *testing.T) {
	uc := newGetForecast(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{},
	)
	_, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{Date: testDate})
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("nil のプログラムが未設定として扱われていない: %v", err)
	}
}

func TestGetForecast_RejectsZeroDate(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetForecast(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)
	if _, err := uc.Execute(context.Background(), testUser, usecase.GetForecastInput{}); err == nil {
		t.Error("日付無しが通ってしまう")
	}
}
