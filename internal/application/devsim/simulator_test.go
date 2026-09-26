package devsim_test

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/devsim"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

var simStart = training.MustDate(2026, time.August, 3) // 月曜

func newSimulator(t *testing.T) *devsim.Simulator {
	t.Helper()

	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	s, err := devsim.NewSimulator(pool)
	if err != nil {
		t.Fatalf("NewSimulator: %v", err)
	}
	return s
}

func baseRequest() devsim.Request {
	return devsim.Request{
		Declared:  []exercise.ExerciseID{"bench", "squat", "deadlift"},
		Frequency: 4,
		Weeks:     4,
		Start:     simStart,
	}
}

func mustRun(t *testing.T, req devsim.Request) devsim.Result {
	t.Helper()

	got, err := newSimulator(t).Run(req)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	return got
}

// 頻度 × 週数のセッションが出ること。
func TestSimulator_ProducesOneSessionPerTrainingDay(t *testing.T) {
	req := baseRequest()
	got := mustRun(t, req)

	if len(got.Days) != req.Frequency*req.Weeks {
		t.Errorf("セッションが %d 件。%d 件のはず", len(got.Days), req.Frequency*req.Weeks)
	}
	if len(got.Weeks) != req.Weeks {
		t.Errorf("週が %d 件。%d 件のはず", len(got.Weeks), req.Weeks)
	}
	for _, d := range got.Days {
		if d.TotalSets == 0 {
			t.Errorf("%v のセット数が0", d.Date)
		}
	}
}

// 処方どおり積んだ結果が、週ごとの充足に出ること。
//
// ここが0のままだと、画面に「全部赤字」が出続ける。捏造した記録が
// 履歴に入っていないときの壊れ方がこれ。
func TestSimulator_ReportsWeeklyVolume(t *testing.T) {
	got := mustRun(t, baseRequest())

	last := got.Weeks[len(got.Weeks)-1]
	if len(last.Regions) == 0 {
		t.Fatal("区分が1つも出ていない")
	}
	filled := 0
	for _, r := range last.Regions {
		if r.Target <= 0 {
			t.Errorf("%v の週目標が0", r.Region)
		}
		if r.Done > 0 {
			filled++
		}
	}
	if filled == 0 {
		t.Error("実測が全区分で0。記録が履歴に積まれていない")
	}
}

// 分割を指定すると、その日がどの分割かが出ること。
func TestSimulator_NamesTheSplitOfTheDay(t *testing.T) {
	req := baseRequest()
	req.SplitKey = "upper_lower"

	got := mustRun(t, req)
	for _, d := range got.Days {
		if d.SplitName == "" {
			t.Fatalf("%v に分割の名前が無い", d.Date)
		}
	}

	// 分割を指定しなければ空。画面が「分割なし」を見分けられる。
	for _, d := range mustRun(t, baseRequest()).Days {
		if d.SplitName != "" {
			t.Fatalf("分割なしなのに名前が出ている: %s", d.SplitName)
		}
	}
}

// 重点種目を指定すると、軸の強度が一巡すること。
//
// 画面で一巡を目で追えることがこの道具の目的なので、比が出ていない
// （推定1RMが立っていない）と何も見えない。
func TestSimulator_ShowsTheAxisCycle(t *testing.T) {
	req := baseRequest()
	req.SplitKey = "upper_lower"
	req.Focus = "bench"
	req.Weeks = 6

	seen := map[string]int{}
	for _, d := range mustRun(t, req).Days {
		for _, m := range d.Main {
			if m.ExerciseID != "bench" || m.PctOfOneRM == 0 {
				continue
			}
			switch {
			case m.PctOfOneRM > 0.85:
				seen["重い"]++
			case m.PctOfOneRM > 0.75:
				seen["軽い"]++
			}
		}
	}
	if seen["重い"] == 0 || seen["軽い"] == 0 {
		t.Errorf("一巡が見えない: %v", seen)
	}
}

// 範囲外の入力はエラーにすること。画面に 400 を返すため。
func TestSimulator_RejectsBadInput(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*devsim.Request)
	}{
		{"頻度が範囲外", func(r *devsim.Request) { r.Frequency = 8 }},
		{"宣言が空", func(r *devsim.Request) { r.Declared = nil }},
		{"分割プリセットが無い", func(r *devsim.Request) { r.SplitKey = "nope" }},
		{"重点種目が存在しない", func(r *devsim.Request) { r.Focus = "nope" }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := baseRequest()
			c.mutate(&req)
			if _, err := newSimulator(t).Run(req); err == nil {
				t.Error("エラーにならない")
			}
		})
	}
}
