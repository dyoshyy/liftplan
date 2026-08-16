package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/seed"
)

var testDate = training.MustDate(2026, time.August, 17)

type fakeExercises struct {
	all []*training.Exercise
	err error
}

func (f fakeExercises) FindAll(context.Context) ([]*training.Exercise, error) {
	return f.all, f.err
}

type fakeLogs struct {
	history training.History
	saved   []*training.SetLog
	calls   int
	err     error
}

func (f *fakeLogs) FindAll(context.Context) (training.History, error) {
	return f.history, f.err
}
func (f *fakeLogs) Save(_ context.Context, logs []*training.SetLog) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, logs...)
	return nil
}

type fakeConditions struct {
	log   training.ConditionLog
	saved []training.DailyCondition
	calls int
	err   error
}

func (f *fakeConditions) FindAll(context.Context) (training.ConditionLog, error) {
	return f.log, f.err
}
func (f *fakeConditions) Save(_ context.Context, items []training.DailyCondition) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, items...)
	return nil
}

type fakeProgram struct {
	program *training.Program
	saved   *training.Program
	err     error
}

func (f *fakeProgram) Get(context.Context) (*training.Program, error) {
	return f.program, f.err
}
func (f *fakeProgram) Save(_ context.Context, p *training.Program) error {
	if f.err != nil {
		return f.err
	}
	f.saved = p
	return nil
}

func buildProgram(t *testing.T, pool []*training.Exercise) *training.Program {
	t.Helper()
	freq, err := training.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	selected := make([]training.ExerciseID, 0, len(pool))
	for _, e := range pool {
		if e.Kind() != training.KindVariation {
			selected = append(selected, e.ID())
		}
	}
	p, err := training.NewProgram(freq, target, selected)
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
		fakeExercises{all: pool}, logs, conditions, program,
		training.DefaultSessionPlanner(),
	)
}

func TestGetSession_ReturnsPlannedSession(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(nil)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)

	got, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	if len(got.Main()) != 3 {
		t.Errorf("メインが3種目でない: %d", len(got.Main()))
	}
	if !got.Date().Equal(testDate) {
		t.Errorf("日付が誤り: %v", got.Date())
	}
}

func TestGetSession_PropagatesProgramNotConfigured(t *testing.T) {
	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(nil)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		&fakeProgram{err: training.ErrProgramNotConfigured},
	)

	_, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if !errors.Is(err, training.ErrProgramNotConfigured) {
		t.Errorf("未設定エラーが伝播していない: %v", err)
	}
}

func TestGetSession_PropagatesRepositoryError(t *testing.T) {
	boom := errors.New("DBが落ちている")
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{err: boom},
		&fakeConditions{log: training.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate}); !errors.Is(err, boom) {
		t.Errorf("リポジトリのエラーが伝播していない: %v", err)
	}
}

func TestGetSession_RejectsZeroDate(t *testing.T) {
	pool, _ := seed.Exercises()
	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(nil)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)},
	)

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{}); err == nil {
		t.Error("日付無しが通ってしまう")
	}
}

// デロードの承認が素通しでドメインに届くこと。
// ユースケースが握りつぶすと、承認しても重量が下がらない。
func TestGetSession_PassesDeloadAcceptanceThrough(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	// 重量が確定するだけの履歴を積む。
	logs := make([]*training.SetLog, 0, 12)
	for i := range 4 {
		l, err := training.NewSetLog(training.SetLogParams{
			ID: "b" + string(rune('0'+i)), PerformedOn: testDate.AddDays(-7 * (4 - i)),
			ExerciseID: "bench", WeightKg: 85, Reps: 8, RIR: 2,
		})
		if err != nil {
			t.Fatalf("ログ生成に失敗: %v", err)
		}
		logs = append(logs, l)
	}

	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(logs)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		&fakeProgram{program: buildProgram(t, pool)})

	weightOf := func(t *testing.T, s training.PlannedSession) float64 {
		t.Helper()
		for _, set := range s.Main() {
			if set.ExerciseID() != "bench" {
				continue
			}
			w, ok := set.Weight()
			if !ok {
				t.Fatal("ベンチの重量が確定していない")
			}
			return w.Kg()
		}
		t.Fatal("ベンチがメインに無い")
		return 0
	}

	normal, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}
	deloaded, err := uc.Execute(context.Background(), usecase.GetSessionInput{
		Date:           testDate,
		DeloadAccepted: []training.ExerciseID{"bench"},
	})
	if err != nil {
		t.Fatalf("実行に失敗: %v", err)
	}

	if weightOf(t, deloaded) >= weightOf(t, normal) {
		t.Errorf("承認がドメインに届いていない: %v → %v",
			weightOf(t, normal), weightOf(t, deloaded))
	}
}

// 対象日が未指定なら、リポジトリを一度も叩かずに弾くこと。
func TestGetSession_RejectsZeroDateBeforeTouchingRepositories(t *testing.T) {
	logs := &fakeLogs{history: training.NewHistory(nil), err: errors.New("叩かれてはいけない")}
	conditions := &fakeConditions{log: training.NewConditionLog(nil)}
	uc := newGetSession(t, logs, conditions, &fakeProgram{err: errors.New("叩かれてはいけない")})

	if _, err := uc.Execute(context.Background(), usecase.GetSessionInput{}); err == nil {
		t.Error("対象日が未指定なのに通った")
	}
}

// 未設定は errors.Is で判別できる形で伝わること。
// 潰すと、プレゼンテーション層が初期設定へ誘導できない。
func TestGetSession_KeepsProgramNotConfiguredIdentifiable(t *testing.T) {
	uc := newGetSession(t,
		&fakeLogs{history: training.NewHistory(nil)},
		&fakeConditions{log: training.NewConditionLog(nil)},
		&fakeProgram{err: training.ErrProgramNotConfigured})

	_, err := uc.Execute(context.Background(), usecase.GetSessionInput{Date: testDate})
	if !errors.Is(err, training.ErrProgramNotConfigured) {
		t.Errorf("未設定が判別できない形になっている: %v", err)
	}
}
