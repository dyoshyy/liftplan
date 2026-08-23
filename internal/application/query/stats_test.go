package query_test

import (
	"context"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/application/query"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

type stubProgram struct {
	program *training.Program
	err     error
}

func (s *stubProgram) Get(context.Context) (*training.Program, error) { return s.program, s.err }
func (s *stubProgram) Save(context.Context, *training.Program) error  { return nil }

func program(t *testing.T, sets map[training.MuscleRegion]float64, selected []training.ExerciseID) *training.Program {
	t.Helper()
	freq, err := training.NewFrequency(3)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := training.NewWeeklyVolumeTarget(sets)
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}
	p, err := training.NewProgram(freq, target, selected)
	if err != nil {
		t.Fatalf("プログラムが不正: %v", err)
	}
	return p
}

func newStats(t *testing.T, logs []*training.SetLog, pool []*training.Exercise, p *training.Program) *query.Stats {
	t.Helper()
	return query.NewStats(
		&stubLogs{history: training.NewHistory(logs)},
		&stubExercises{all: pool},
		&stubProgram{program: p},
		training.DefaultOneRepMaxEstimator(),
	)
}

func defaultProgram(t *testing.T) *training.Program {
	t.Helper()
	return program(t,
		map[training.MuscleRegion]float64{training.ChestMid: 10, training.Lat: 8},
		[]training.ExerciseID{"bench"},
	)
}

// 推移は古い順に並ぶ。現在値は最後の点で、増減は最初との差。
// 並びが崩れると、伸びているのに減っているように見える。
func TestTrends_古い順に並び現在値は最後の点(t *testing.T) {
	q := newStats(t,
		[]*training.SetLog{
			log(t, "c", "2026-08-18", "bench", 100, 5, 1),
			log(t, "a", "2026-08-04", "bench", 90, 5, 1),
			log(t, "b", "2026-08-11", "bench", 95, 5, 1),
		},
		[]*training.Exercise{exercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), date(t, "2026-08-01"), date(t, "2026-08-31"))
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
		[]*training.SetLog{
			log(t, "a", "2026-07-01", "bench", 90, 5, 1),
			log(t, "b", "2026-08-11", "bench", 95, 5, 1),
		},
		[]*training.Exercise{exercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), date(t, "2026-08-01"), date(t, "2026-08-31"))
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
		[]*training.SetLog{log(t, "a", "2026-08-11", "bench", 95, 5, 1)},
		[]*training.Exercise{exercise(t, "bench", "ベンチプレス"), exercise(t, "squat", "スクワット")},
		defaultProgram(t),
	)

	trends, err := q.Trends(context.Background(), date(t, "2026-08-01"), date(t, "2026-08-31"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(trends) != 1 || trends[0].ExerciseID != "bench" {
		t.Fatalf("記録の無い種目が出ている: %+v", trends)
	}
}

func TestTrends_期間が無ければ断る(t *testing.T) {
	q := newStats(t, nil, nil, defaultProgram(t))
	if _, err := q.Trends(context.Background(), training.Date{}, date(t, "2026-08-31")); err == nil {
		t.Fatal("期間が無いのに通った")
	}
}

// 週目標に対する充足。埋まっていない区分が上に来る。
// 補助種目が選ばれる理由がここで見える。
func TestWeeklyVolume_埋まっていない順に並ぶ(t *testing.T) {
	bench := exercise(t, "bench", "ベンチプレス")
	q := newStats(t,
		[]*training.SetLog{
			log(t, "a", "2026-08-18", "bench", 100, 5, 1),
			log(t, "b", "2026-08-18", "bench", 100, 5, 1),
		},
		[]*training.Exercise{bench},
		defaultProgram(t),
	)

	// 2026-08-18 は火曜。週の頭から当日までを数える。
	vols, err := q.WeeklyVolume(context.Background(), date(t, "2026-08-18"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	if len(vols) != 2 {
		t.Fatalf("区分の数が合わない: %d", len(vols))
	}
	if vols[0].DoneSet > vols[1].DoneSet {
		t.Fatalf("埋まっている方が先頭に来ている: %+v", vols)
	}
	// 胸（中部）に2セット入っているはず。
	var chest *query.RegionVolume
	for i := range vols {
		if vols[i].Region == training.ChestMid {
			chest = &vols[i]
		}
	}
	if chest == nil {
		t.Fatal("胸（中部）が出ない")
	}
	if chest.DoneSet != 2 {
		t.Fatalf("こなしたセット数が %v", chest.DoneSet)
	}
	if chest.TargetSet != 10 {
		t.Fatalf("週目標が %v", chest.TargetSet)
	}
}

// 先週の記録は今週の充足に入らない。入ると、やっていない週が
// 埋まって見えて補助種目が選ばれなくなる。
func TestWeeklyVolume_先週を含めない(t *testing.T) {
	q := newStats(t,
		[]*training.SetLog{log(t, "a", "2026-08-11", "bench", 100, 5, 1)},
		[]*training.Exercise{exercise(t, "bench", "ベンチプレス")},
		defaultProgram(t),
	)

	vols, err := q.WeeklyVolume(context.Background(), date(t, "2026-08-18"))
	if err != nil {
		t.Fatalf("読めない: %v", err)
	}
	for _, v := range vols {
		if v.DoneSet != 0 {
			t.Fatalf("先週が混ざっている: %+v", v)
		}
	}
}

func TestWeeklyVolume_プログラムが無ければ断る(t *testing.T) {
	q := query.NewStats(
		&stubLogs{history: training.NewHistory(nil)},
		&stubExercises{},
		&stubProgram{program: nil},
		training.DefaultOneRepMaxEstimator(),
	)
	if _, err := q.WeeklyVolume(context.Background(), date(t, "2026-08-18")); err == nil {
		t.Fatal("プログラム未設定なのに通った")
	}
}
