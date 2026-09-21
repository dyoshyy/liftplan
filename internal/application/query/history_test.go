package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

type stubLogs struct {
	history setlog.History
	err     error
}

func (s *stubLogs) FindAll(context.Context, account.UserID) (setlog.History, error) {
	return s.history, s.err
}
func (s *stubLogs) Save(context.Context, account.UserID, []*setlog.SetLog) error { return nil }
func (s *stubLogs) Delete(context.Context, account.UserID, setlog.SetLogID) error {
	return nil
}

type stubExercises struct {
	all []*exercise.Exercise
	err error
}

func (s *stubExercises) FindAll(context.Context) ([]*exercise.Exercise, error) {
	return s.all, s.err
}

func date(t *testing.T, s string) training.Date {
	t.Helper()
	d, err := training.ParseDate(s)
	if err != nil {
		t.Fatalf("日付が不正 %q: %v", s, err)
	}
	return d
}

func log(t *testing.T, id, day, ex string, kg float64, reps, rir int) *setlog.SetLog {
	t.Helper()
	l, err := setlog.NewSetLog(setlog.SetLogParams{
		ID:          id,
		PerformedOn: date(t, day),
		ExerciseID:  ex,
		WeightKg:    kg,
		Reps:        reps,
		RIR:         rir,
	})
	if err != nil {
		t.Fatalf("実績が作れない %s: %v", id, err)
	}
	return l
}

func newExercise(t *testing.T, id, name string) *exercise.Exercise {
	t.Helper()
	e, err := exercise.NewExercise(exercise.ExerciseParams{
		ID:          id,
		Name:        name,
		IncrementKg: 2.5,
		Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1},
	})
	if err != nil {
		t.Fatalf("種目が作れない %s: %v", id, err)
	}
	return e
}

func newHistory(t *testing.T, logs []*setlog.SetLog, pool []*exercise.Exercise) *query.History {
	t.Helper()
	return query.NewHistory(
		&stubLogs{history: setlog.NewHistory(logs)},
		&stubExercises{all: pool},
	)
}

// 1日に複数の種目があっても、種目ごとに分かれる。
// TrainingSession は日でまとめたものなので、ここで分けないと混ざる。
func TestDays_同じ日の種目を分ける(t *testing.T) {
	q := newHistory(t,
		[]*setlog.SetLog{
			log(t, "a1", "2026-08-20", "bench", 100, 5, 1),
			log(t, "b1", "2026-08-20", "squat", 140, 5, 1),
			log(t, "a2", "2026-08-20", "bench", 100, 5, 2),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス"), newExercise(t, "squat", "スクワット")},
	)

	days, err := q.Days(context.Background(), account.DefaultUserID(), date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(days) != 1 {
		t.Fatalf("1日にまとまらない: %d日", len(days))
	}
	if len(days[0].Exercises) != 2 {
		t.Fatalf("種目が分かれていない: %d種目", len(days[0].Exercises))
	}
	if days[0].TotalSets != 3 {
		t.Fatalf("セット数が合わない: %d", days[0].TotalSets)
	}
	for _, e := range days[0].Exercises {
		if e.Name == "" {
			t.Fatalf("種目名が空: %s", e.ExerciseID)
		}
	}
}

// 振り返りは直近から見るので、新しい日が先頭に来る。
func TestDays_新しい日から並ぶ(t *testing.T) {
	q := newHistory(t,
		[]*setlog.SetLog{
			log(t, "a", "2026-08-10", "bench", 100, 5, 1),
			log(t, "b", "2026-08-20", "bench", 102, 5, 1),
			log(t, "c", "2026-08-15", "bench", 101, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
	)

	days, err := q.Days(context.Background(), account.DefaultUserID(), date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	want := []string{"2026-08-20", "2026-08-15", "2026-08-10"}
	if len(days) != len(want) {
		t.Fatalf("日数が合わない: %d", len(days))
	}
	for i, w := range want {
		if days[i].Date.String() != w {
			t.Fatalf("%d番目が %s、期待は %s", i, days[i].Date, w)
		}
	}
}

func TestDays_期間の外を含めない(t *testing.T) {
	q := newHistory(t,
		[]*setlog.SetLog{
			log(t, "before", "2026-08-09", "bench", 100, 5, 1),
			log(t, "in", "2026-08-10", "bench", 100, 5, 1),
			log(t, "after", "2026-08-21", "bench", 100, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
	)

	days, err := q.Days(context.Background(), account.DefaultUserID(), date(t, "2026-08-10"), date(t, "2026-08-20"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(days) != 1 || days[0].Date.String() != "2026-08-10" {
		t.Fatalf("期間の外が混ざっている: %+v", days)
	}
}

func TestDays_期間が逆なら断る(t *testing.T) {
	q := newHistory(t, nil, nil)
	if _, err := q.Days(context.Background(), account.DefaultUserID(), date(t, "2026-08-20"), date(t, "2026-08-10")); err == nil {
		t.Fatal("終わりが始まりより前なのに通った")
	}
}

func TestDays_取得できなければ理由を返す(t *testing.T) {
	boom := errors.New("接続できない")
	q := query.NewHistory(&stubLogs{err: boom}, &stubExercises{})
	_, err := q.Days(context.Background(), account.DefaultUserID(), date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err == nil {
		t.Fatal("失敗が伝わらない")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("元の理由が消えている: %v", err)
	}
}

// 当日の記録を「前回」に混ぜると、いま入れた1セットが前回になって
// 比較の意味が消える。
func TestLastPerformances_当日を含めない(t *testing.T) {
	q := newHistory(t,
		[]*setlog.SetLog{
			log(t, "old", "2026-08-18", "bench", 100, 5, 1),
			log(t, "today", "2026-08-20", "bench", 105, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
	)

	last, err := q.LastPerformances(context.Background(), account.DefaultUserID(), date(t, "2026-08-20"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	got, ok := last["bench"]
	if !ok {
		t.Fatal("前回が出ない")
	}
	if got.Date.String() != "2026-08-18" {
		t.Fatalf("当日が前回になっている: %s", got.Date)
	}
	if got.DaysAgo != 2 {
		t.Fatalf("何日前かが合わない: %d", got.DaysAgo)
	}
}

// 重量を途中で変えたセットが、代表の1つに畳まれて消えないこと。
// 畳むと、落とした重量も上げた重量も履歴から見えなくなる。
func TestLastPerformances_セットごとの重量を残す(t *testing.T) {
	q := newHistory(t,
		[]*setlog.SetLog{
			log(t, "s1", "2026-08-18", "bench", 100, 5, 1),
			log(t, "s2", "2026-08-18", "bench", 90, 8, 1),
			log(t, "s3", "2026-08-18", "bench", 80, 12, 0),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
	)

	last, err := q.LastPerformances(context.Background(), account.DefaultUserID(), date(t, "2026-08-20"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	got := last["bench"]
	if len(got.Weights) != len(got.Reps) {
		t.Fatalf("重量とレップの数が揃わない: %d と %d", len(got.Weights), len(got.Reps))
	}
	want := []float64{100, 90, 80}
	if len(got.Weights) != len(want) {
		t.Fatalf("重量が畳まれている: %v", got.Weights)
	}
	for i, w := range want {
		if got.Weights[i] != w {
			t.Fatalf("%d番目の重量が %v、期待は %v", i, got.Weights[i], w)
		}
	}
	// 次に何kgから入るかの目安なので、その日の最も重いセット。
	if got.WeightKg != 100 {
		t.Fatalf("代表の重量が最重量でない: %v", got.WeightKg)
	}
}

// 1日に複数種目が入るので、セッション単位で最新を選ぶと
// 別の種目の日付に引きずられる。
func TestLastPerformances_種目ごとに最新を選ぶ(t *testing.T) {
	q := newHistory(t,
		[]*setlog.SetLog{
			log(t, "b1", "2026-08-10", "bench", 100, 5, 1),
			log(t, "s1", "2026-08-18", "squat", 140, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス"), newExercise(t, "squat", "スクワット")},
	)

	last, err := q.LastPerformances(context.Background(), account.DefaultUserID(), date(t, "2026-08-20"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if last["bench"].Date.String() != "2026-08-10" {
		t.Fatalf("ベンチの前回が %s", last["bench"].Date)
	}
	if last["squat"].Date.String() != "2026-08-18" {
		t.Fatalf("スクワットの前回が %s", last["squat"].Date)
	}
}

func TestLastPerformances_基準日が無ければ断る(t *testing.T) {
	q := newHistory(t, nil, nil)
	if _, err := q.LastPerformances(context.Background(), account.DefaultUserID(), training.Date{}); err == nil {
		t.Fatal("基準日が無いのに通った")
	}
}

// 同じ種目に過去の記録が複数あるとき、選ぶのは最も新しい日。
// 古い日を掴むと、伸びているのに「前回」が停滞して見える。
func TestLastPerformances_過去が複数あれば最も新しい日(t *testing.T) {
	q := newHistory(t,
		[]*setlog.SetLog{
			log(t, "old", "2026-08-04", "bench", 90, 5, 1),
			log(t, "mid", "2026-08-11", "bench", 95, 5, 1),
			log(t, "new", "2026-08-18", "bench", 100, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
	)

	last, err := q.LastPerformances(context.Background(), account.DefaultUserID(), date(t, "2026-08-20"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	got := last["bench"]
	if got.Date.String() != "2026-08-18" {
		t.Fatalf("最も新しい日でない: %s", got.Date)
	}
	if len(got.Weights) != 1 || got.Weights[0] != 100 {
		t.Fatalf("古い日のセットが混ざっている: %v", got.Weights)
	}
}
