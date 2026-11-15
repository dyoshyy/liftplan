package runlog_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/runlog"
)

func runLogParams() runlog.RunLogParams {
	return runlog.RunLogParams{
		ID:              "01J0000000000000000000RUN1",
		PerformedOn:     training.MustDate(2026, time.August, 18),
		DistanceKm:      5.2,
		DurationSeconds: 1680,
	}
}

func TestNewRunLog(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*runlog.RunLogParams)
		wantMeters int
	}{
		{name: "受け取った値をそのまま持つ", mutate: func(*runlog.RunLogParams) {}, wantMeters: 5200},
		{
			// 距離はメートルに丸めてから範囲を見る（CLAUDE.md「quantize は
			// 検証の前に」）。後に丸めると 0.0004km が範囲を通ってから0mになる。
			name:       "距離はメートル単位に丸める",
			mutate:     func(p *runlog.RunLogParams) { p.DistanceKm = 5.2004 },
			wantMeters: 5200,
		},
		{
			name:       "上限ちょうどは通す",
			mutate:     func(p *runlog.RunLogParams) { p.DistanceKm = 200 },
			wantMeters: 200000,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runLogParams()
			c.mutate(&p)
			got, err := runlog.NewRunLog(p)
			if err != nil {
				t.Fatalf("NewRunLog: %v", err)
			}
			if got.ID() != runlog.RunLogID(p.ID) {
				t.Errorf("ID が %v。%v のはず", got.ID(), p.ID)
			}
			if !got.PerformedOn().Equal(p.PerformedOn) {
				t.Errorf("実施日が %v。%v のはず", got.PerformedOn(), p.PerformedOn)
			}
			if got.Distance().Meters() != c.wantMeters {
				t.Errorf("距離が %dm。%dm のはず", got.Distance().Meters(), c.wantMeters)
			}
			if got.Distance().Km() != float64(c.wantMeters)/1000 {
				t.Errorf("距離が %vkm。%vkm のはず", got.Distance().Km(), float64(c.wantMeters)/1000)
			}
			if got.Duration().Seconds() != p.DurationSeconds {
				t.Errorf("時間が %d秒。%d秒 のはず", got.Duration().Seconds(), p.DurationSeconds)
			}
		})
	}
}

func TestNewRunLog_Rejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*runlog.RunLogParams)
	}{
		{"IDが空", func(p *runlog.RunLogParams) { p.ID = "" }},
		{"IDの前後に空白", func(p *runlog.RunLogParams) { p.ID = " x" }},
		{"IDが長すぎる", func(p *runlog.RunLogParams) { p.ID = strings.Repeat("a", 65) }},
		{"IDに制御文字", func(p *runlog.RunLogParams) { p.ID = "a\nb" }},
		{"実施日が無い", func(p *runlog.RunLogParams) { p.PerformedOn = training.Date{} }},
		{"距離が0", func(p *runlog.RunLogParams) { p.DistanceKm = 0 }},
		// 丸めると0mになる。検証の後に丸めると、ここが通って0mの記録が残る。
		{"距離が丸めると0", func(p *runlog.RunLogParams) { p.DistanceKm = 0.0004 }},
		{"距離が負", func(p *runlog.RunLogParams) { p.DistanceKm = -1 }},
		// メートルをキロの欄に入れた打ち間違い（5000m → 5000km）を止める。
		{"距離が上限を超える", func(p *runlog.RunLogParams) { p.DistanceKm = 200.001 }},
		{"距離が NaN", func(p *runlog.RunLogParams) { p.DistanceKm = math.NaN() }},
		{"距離が無限大", func(p *runlog.RunLogParams) { p.DistanceKm = math.Inf(1) }},
		{"時間が0", func(p *runlog.RunLogParams) { p.DurationSeconds = 0 }},
		{"時間が負", func(p *runlog.RunLogParams) { p.DurationSeconds = -1 }},
		{"時間が1日を超える", func(p *runlog.RunLogParams) { p.DurationSeconds = 24*60*60 + 1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runLogParams()
			c.mutate(&p)
			if got, err := runlog.NewRunLog(p); err == nil {
				t.Errorf("通ってしまった: %+v", got)
			}
		})
	}
}

// Equals は同じIDで内容が違う再送を見分けるためにある（ErrConflictingRunLog）。
// 1項目でも違えば別物、全項目が同じなら同じ。
func TestRunLog_Equals(t *testing.T) {
	base := mustRunLog(t, runLogParams())
	cases := []struct {
		name   string
		mutate func(*runlog.RunLogParams)
		want   bool
	}{
		{"全項目が同じ", func(*runlog.RunLogParams) {}, true},
		{"日付が違う", func(p *runlog.RunLogParams) { p.PerformedOn = p.PerformedOn.AddDays(1) }, false},
		{"距離が違う", func(p *runlog.RunLogParams) { p.DistanceKm = 5.3 }, false},
		{"時間が違う", func(p *runlog.RunLogParams) { p.DurationSeconds = 1681 }, false},
		{"IDが違う", func(p *runlog.RunLogParams) { p.ID = "01J0000000000000000000RUN2" }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := runLogParams()
			c.mutate(&p)
			if got := base.Equals(mustRunLog(t, p)); got != c.want {
				t.Errorf("Equals が %v。%v のはず", got, c.want)
			}
		})
	}
}

func mustRunLog(t *testing.T, p runlog.RunLogParams) *runlog.RunLog {
	t.Helper()
	l, err := runlog.NewRunLog(p)
	if err != nil {
		t.Fatalf("NewRunLog(%s): %v", p.ID, err)
	}
	return l
}
