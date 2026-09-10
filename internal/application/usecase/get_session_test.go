package usecase_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

var testDate = training.MustDate(2026, time.August, 17)

type fakeExercises struct {
	mu    sync.Mutex
	all   []*exercise.Exercise
	calls int
	err   error
}

func (f *fakeExercises) FindAll(context.Context) ([]*exercise.Exercise, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.all, f.err
}
func (f *fakeExercises) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fake も「FindAll は Save と並行に呼ばれる」というリポジトリの契約を
// 代理するので、同期を持たせる。持たせないと、並行性を検証するテストを
// 書いた瞬間に fake 側で -race が発火し、本体の問題と紛れる。

type fakeLogs struct {
	mu      sync.Mutex
	history setlog.History
	saved   []*setlog.SetLog
	deleted []setlog.SetLogID
	calls   int
	finds   int
	err     error
}

func (f *fakeLogs) FindAll(context.Context) (setlog.History, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds++
	return f.history, f.err
}
func (f *fakeLogs) findCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.finds
}
func (f *fakeLogs) Save(_ context.Context, logs []*setlog.SetLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, logs...)
	return nil
}
func (f *fakeLogs) Delete(_ context.Context, id setlog.SetLogID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, id)
	return f.err
}
func (f *fakeLogs) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeConditions struct {
	mu    sync.Mutex
	log   condition.ConditionLog
	saved []condition.DailyCondition
	calls int
	err   error
}

func (f *fakeConditions) FindAll(context.Context) (condition.ConditionLog, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.log, f.err
}
func (f *fakeConditions) Save(_ context.Context, items []condition.DailyCondition) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, items...)
	return nil
}
func (f *fakeConditions) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeProgram struct {
	mu      sync.Mutex
	program *program.Program
	saved   *program.Program
	calls   int
	err     error
}

func (f *fakeProgram) Get(context.Context) (*program.Program, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.program, f.err
}
func (f *fakeProgram) Save(_ context.Context, p *program.Program) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.saved = p
	return nil
}
func (f *fakeProgram) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
func (f *fakeProgram) savedProgram() *program.Program {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saved
}

func buildProgram(t *testing.T, pool []*exercise.Exercise) *program.Program {
	t.Helper()
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}
	p, err := program.NewProgram(freq, target, selected, []exercise.ExerciseID{"bench", "squat", "deadlift"})
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return p
}

func newGetSession(t *testing.T, logs *fakeLogs, conditions *fakeConditions, program *fakeProgram) *usecase.GetSession {
	t.Helper()
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	return usecase.NewGetSession(
		&fakeExercises{all: pool}, logs, conditions, program,
		planning.DefaultSessionPlanner(),
	)
}

func TestGetSession_ReturnsPlannedSession(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)

	got, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(got.Main()) != 1 {
		t.Errorf("ヘビー枠が1つでない: %d", len(got.Main()))
	}
	if !got.Date().Equal(testDate) {
		t.Errorf("日付が誤り: %v", got.Date())
	}
}

func TestGetSession_PropagatesProgramNotConfigured(t *testing.T) {
	uc := newGetSession(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{err: program.ErrProgramNotConfigured},
	)

	_, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("未設定エラーが伝播していない: %v", err)
	}
}

func TestGetSession_PropagatesRepositoryError(t *testing.T) {
	boom := errors.New("DBが落ちている")
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{err: boom},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate}); !errors.Is(err, boom) {
		t.Errorf("リポジトリのエラーが伝播していない: %v", err)
	}
}

func TestGetSession_RejectsZeroDate(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{}); err == nil {
		t.Error("日付無しが通ってしまう")
	}
}

// 対象日が未指定なら、リポジトリを一度も叩かずに弾くこと。
func TestGetSession_RejectsZeroDateBeforeTouchingRepositories(t *testing.T) {
	logs := &fakeLogs{history: setlog.NewHistory(nil)}
	conditions := &fakeConditions{log: condition.NewConditionLog(nil)}
	programs := &fakeProgram{}
	uc := newGetSession(t, logs, conditions, programs)

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{}); err == nil {
		t.Error("対象日が未指定なのに通った")
	}
	// 「エラーが返ること」だけを見ると、ガードを消しても
	// リポジトリ由来のエラーで成立してしまう。叩いていないことを見る。
	if programs.callCount() != 0 {
		t.Errorf("対象日が未指定なのにリポジトリを叩いた: %d回", programs.callCount())
	}
}

// 未設定は errors.Is で判別できる形で伝わること。
// 潰すと、プレゼンテーション層が初期設定へ誘導できない。
func TestGetSession_KeepsProgramNotConfiguredIdentifiable(t *testing.T) {
	uc := newGetSession(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{err: program.ErrProgramNotConfigured})

	_, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("未設定が判別できない形になっている: %v", err)
	}
}

// コンディションが実際に計画へ届くこと。
// 配線し忘れても、コンディションを渡さないテストばかりだと気づけない。
func TestGetSession_ConditionsReachTheDomain(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	items := []condition.DailyCondition{
		condition.NewDailyCondition(testDate).WithSleepHours(4),
	}
	for i := 1; i <= 14; i++ {
		items = append(items,
			condition.NewDailyCondition(testDate.AddDays(-i)).WithSleepHours(7))
	}

	rirOf := func(t *testing.T, conditions *fakeConditions) int {
		t.Helper()
		uc := newGetSession(t,
			&fakeLogs{history: setlog.NewHistory(nil)},
			conditions,
			&fakeProgram{program: buildProgram(t, pool)})
		s, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
		if err != nil {
			t.Fatalf("実行に失敗: %v", err)
		}
		if len(s.Main()) == 0 {
			t.Fatal("メイン種目が出ていない")
		}
		return s.Main()[0].TargetRIR().Int()
	}

	base := rirOf(t, &fakeConditions{log: condition.NewConditionLog(nil)})
	deprived := rirOf(t, &fakeConditions{log: condition.NewConditionLog(items)})

	if deprived <= base {
		t.Errorf("睡眠不足がRIR補正に届いていない: %d → %d", base, deprived)
	}
}

func TestGetSession_PropagatesConditionError(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	boom := errors.New("読めない")
	uc := newGetSession(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil), err: boom},
		&fakeProgram{program: buildProgram(t, pool)})

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate}); !errors.Is(err, boom) {
		t.Errorf("コンディションのエラーが伝播していない: %v", err)
	}
}

// リポジトリが契約に反して (nil, nil) を返しても、未設定として扱えること。
func TestGetSession_TreatsNilProgramAsNotConfigured(t *testing.T) {
	uc := newGetSession(t,
		&fakeLogs{history: setlog.NewHistory(nil)},
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{})

	_, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if !errors.Is(err, program.ErrProgramNotConfigured) {
		t.Errorf("nil のプログラムが未設定として扱われていない: %v", err)
	}
}

// キャンセル済みの context では履歴の全件読み込みまで走らせないこと。
func TestGetSession_StopsOnCancelledContext(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	logs := &fakeLogs{history: setlog.NewHistory(nil)}
	uc := newGetSession(t, logs,
		&fakeConditions{log: condition.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := uc.Execute(ctx, usecase.GetSessionInput{Date: testDate}); !errors.Is(err, context.Canceled) {
		t.Errorf("キャンセルが伝わっていない: %v", err)
	}
	// 履歴の全件読み込みは最も高くつく。切断済みなら払わない。
	if logs.findCount() != 0 {
		t.Errorf("キャンセル済みなのに履歴を読んだ: %d回", logs.findCount())
	}
}
