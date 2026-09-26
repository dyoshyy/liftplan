package query_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/account"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

type stubProgram struct {
	prog *program.Program
	err  error
}

func (s *stubProgram) Get(context.Context, account.UserID) (*program.Program, error) {
	return s.prog, s.err
}
func (s *stubProgram) Save(context.Context, account.UserID, *program.Program) error { return nil }

func newProgram(t *testing.T, selected []exercise.ExerciseID) *program.Program {
	t.Helper()
	freq, err := program.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	p, err := program.NewProgram(freq, mustVolume(t, 6, 3), selected, selected, "")
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return p
}

func newStats(t *testing.T, logs []*setlog.SetLog, pool []*exercise.Exercise, p *program.Program) *query.Stats {
	t.Helper()
	return query.NewStats(
		&stubLogs{history: setlog.NewHistory(logs)},
		&stubExercises{all: pool},
		&stubProgram{prog: p},
		planning.DefaultOneRepMaxEstimator(),
	)
}

func defaultProgram(t *testing.T) *program.Program {
	t.Helper()
	return newProgram(t, []exercise.ExerciseID{"bench"})
}

// 推移は古い順に並ぶ。現在値は最後の点で、増減は最初との差。
// 並びが崩れると、伸びているのに減っているように見える。
func TestTrends_古い順に並び現在値は最後の点(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "c", "2026-08-18", "bench", 100, 5, 1),
			log(t, "a", "2026-08-04", "bench", 90, 5, 1),
			log(t, "b", "2026-08-11", "bench", 95, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(trends) != 1 {
		t.Fatalf("種目数が合わない: %d", len(trends))
	}
	tr := trends[0]
	if len(tr.Points) != 3 {
		t.Fatalf("点の数が合わない: %d", len(tr.Points))
	}
	for i := 1; i < len(tr.Points); i++ {
		if !tr.Points[i-1].Date.Before(tr.Points[i].Date) {
			t.Fatalf("古い順に並んでいない: %v", tr.Points)
		}
	}
	if tr.CurrentKg != tr.Points[len(tr.Points)-1].Kg {
		t.Fatalf("現在値が最後の点でない: %v", tr.CurrentKg)
	}
	if tr.ChangeKg <= 0 {
		t.Fatalf("伸びているのに増減が %v", tr.ChangeKg)
	}
}

func TestTrends_期間の外を含めない(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "a", "2026-07-01", "bench", 90, 5, 1),
			log(t, "b", "2026-08-11", "bench", 95, 5, 1),
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(trends) != 1 || len(trends[0].Points) != 1 {
		t.Fatalf("期間の外が混ざっている: %+v", trends)
	}
}

// 記録が無い種目は線が引けないので出さない。0 として出すと床に張り付く。
func TestTrends_記録の無い種目は出さない(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{log(t, "a", "2026-08-11", "bench", 95, 5, 1)},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス"), newExercise(t, "squat", "スクワット")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), testUser, date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(trends) != 1 || trends[0].ExerciseID != "bench" {
		t.Fatalf("記録の無い種目が出ている: %+v", trends)
	}
}

func TestTrends_期間が無ければ断る(t *testing.T) {
	q := newStats(t, nil, nil, defaultProgram(t))
	if _, err := q.Trends(context.Background(), testUser, training.Date{}, date(t, "2026-08-31")); err == nil {
		t.Fatal("期間が無いのに通った")
	}
}

// 週目標に対する充足。埋まっていない区分が上に来る。
// 補助種目が選ばれる理由がここで見える。
func TestWeeklyVolume_埋まっていない順に並ぶ(t *testing.T) {
	bench := newExercise(t, "bench", "ベンチプレス")
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "a", "2026-08-18", "bench", 100, 5, 1),
			log(t, "b", "2026-08-18", "bench", 100, 5, 1),
		},
		[]*exercise.Exercise{bench},
		defaultProgram(t),
	)

	// 直近4週（当日を含む28日）を数え、週あたりに直す。
	vols, err := q.WeeklyVolume(context.Background(), testUser, date(t, "2026-08-18"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	// 週目標は設定から組み直すので、全区分が並ぶ。
	if len(vols) != len(training.AllMuscleRegions()) {
		t.Fatalf("区分の数が合わない: %d", len(vols))
	}
	if vols[0].DoneSet > vols[1].DoneSet {
		t.Fatalf("埋まっている方が先頭に来ている: %+v", vols)
	}
	// 胸（中部）に2セット入っている。窓は4週なので、週あたりでは 2 ÷ 4。
	var chest *query.RegionVolume
	for i := range vols {
		if vols[i].Region == training.ChestMid {
			chest = &vols[i]
		}
	}
	if chest == nil {
		t.Fatal("胸（中部）が出ない")
	}
	if want := 2.0 / planning.CoverageWindowWeeks; chest.DoneSet != want {
		t.Fatalf("こなしたセット数（週あたり）が %v。%v のはず", chest.DoneSet, want)
	}
	p := defaultProgram(t)
	want, err := seed.DefaultWeeklyTarget(p.Frequency(), p.SessionVolume())
	if err != nil {
		t.Fatalf("既定の週目標が組めない: %v", err)
	}
	if chest.TargetSet != want.Sets(training.ChestMid) {
		t.Fatalf("週目標が %v。設定から組み直した %v のはず", chest.TargetSet, want.Sets(training.ChestMid))
	}
}

// 窓（当日を含む28日）より前の記録は充足に入らないこと。
//
// 窓はエンジンと同じ長さ。画面だけ長いと、エンジンがもう数えていない
// 記録で埋まって見え、「足りていない区分から選ばれる」が画面の上で
// 成り立たなくなる。2026-07-21 は 2026-08-18 のちょうど28日前。
func TestWeeklyVolume_窓の外を含めない(t *testing.T) {
	q := newStats(t,
		[]*setlog.SetLog{
			log(t, "out", "2026-07-21", "bench", 100, 5, 1), // 28日前。窓の外
			log(t, "in", "2026-07-22", "bench", 100, 5, 1),  // 27日前。窓の内側の端
		},
		[]*exercise.Exercise{newExercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	vols, err := q.WeeklyVolume(context.Background(), testUser, date(t, "2026-08-18"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	// 窓の中の1セットだけが数えられ、週あたりに直る。
	want := 1.0 / planning.CoverageWindowWeeks
	for _, v := range vols {
		if v.Region == training.ChestMid && v.DoneSet != want {
			t.Fatalf("胸（中部）が %v。窓の中の1セットだけで %v のはず", v.DoneSet, want)
		}
	}
}

func TestWeeklyVolume_プログラムが無ければ断る(t *testing.T) {
	q := query.NewStats(
		&stubLogs{history: setlog.NewHistory(nil)},
		&stubExercises{},
		&stubProgram{prog: nil},
		planning.DefaultOneRepMaxEstimator(),
	)
	if _, err := q.WeeklyVolume(context.Background(), testUser, date(t, "2026-08-18")); err == nil {
		t.Fatal("プログラム未設定なのに通った")
	}
}

// mustVolume はテスト用の1回の量。
func mustVolume(t *testing.T, exercises, sets int) program.SessionVolume {
	t.Helper()
	v, err := program.NewSessionVolume(exercises, sets)
	if err != nil {
		t.Fatalf("NewSessionVolume(%d, %d): %v", exercises, sets, err)
	}
	return v
}

// TestWeeklyVolume_設定から組み直した週目標と比べる は無くなった。
//
// このテストは「保存された週目標が既定と違っていても、画面はそれを無視して
// 設定から組み直した値を見る」ことを、わざと既定と違う保存値（胸中部だけ
// 30）を持つプログラムで確かめていた。Program はもう週目標を保持しない
// （#176）ので、「既定と違う保存値」というプログラム自体が作れなくなり、
// 検査する対象が無くなった。組み直しそのもの（Frequency・SessionVolume
// から導く）は TestWeeklyVolume_埋まっていない順に並ぶ が同じ形で見ている。
