package setlog_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

func setLogParams() setlog.SetLogParams {
	return setlog.SetLogParams{
		ID:          "01J0000000000000000000BNCH",
		PerformedOn: training.MustDate(2026, time.August, 16),
		ExerciseID:  "bench",
		WeightKg:    85,
		Reps:        9,
		RIR:         2,
	}
}

func mustSetLog(t *testing.T, p setlog.SetLogParams) *setlog.SetLog {
	t.Helper()
	s, err := setlog.NewSetLog(p)
	if err != nil {
		t.Fatalf("NewSetLog(%s): %v", p.ID, err)
	}
	return s
}

func TestNewSetLog(t *testing.T) {
	s := mustSetLog(t, setLogParams())

	if s.ID() != setlog.SetLogID("01J0000000000000000000BNCH") {
		t.Errorf("ID が誤り: %v", s.ID())
	}
	if s.Weight().Kg() != 85 {
		t.Errorf("重量が誤り: %v", s.Weight().Kg())
	}
	if s.Reps().Int() != 9 || s.RIR().Int() != 2 {
		t.Errorf("レップ/RIRが誤り: %d %d", s.Reps().Int(), s.RIR().Int())
	}
	if s.ExerciseID() != exercise.ExerciseID("bench") {
		t.Errorf("種目IDが誤り: %v", s.ExerciseID())
	}
	if !s.PerformedOn().Equal(training.MustDate(2026, time.August, 16)) {
		t.Errorf("実施日が誤り: %v", s.PerformedOn())
	}
}

func TestNewSetLog_RejectsInvalidFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*setlog.SetLogParams)
	}{
		{"IDが空", func(p *setlog.SetLogParams) { p.ID = "" }},
		{"IDが空白のみ（前後空白として弾かれる）", func(p *setlog.SetLogParams) { p.ID = "   " }},
		{"IDの前後に空白", func(p *setlog.SetLogParams) { p.ID = " 01J " }},
		{"IDが長すぎる", func(p *setlog.SetLogParams) { p.ID = strings.Repeat("x", 65) }},
		{"実施日が無い", func(p *setlog.SetLogParams) { p.PerformedOn = training.Date{} }},
		{"種目IDが空", func(p *setlog.SetLogParams) { p.ExerciseID = "" }},
		{"レップが0", func(p *setlog.SetLogParams) { p.Reps = 0 }},
		{"レップが負", func(p *setlog.SetLogParams) { p.Reps = -1 }},
		{"レップが上限超", func(p *setlog.SetLogParams) { p.Reps = 1001 }},
		{"RIRが負", func(p *setlog.SetLogParams) { p.RIR = -1 }},
		{"RIRが上限超", func(p *setlog.SetLogParams) { p.RIR = 101 }},
		{"重量が負", func(p *setlog.SetLogParams) { p.WeightKg = -5 }},
		{"重量が上限超", func(p *setlog.SetLogParams) { p.WeightKg = 1001 }},
		{"重量がNaN", func(p *setlog.SetLogParams) { p.WeightKg = math.NaN() }},
		{"重量が無限大", func(p *setlog.SetLogParams) { p.WeightKg = math.Inf(1) }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := setLogParams()
			c.mutate(&p)
			got, err := setlog.NewSetLog(p)
			if err == nil {
				t.Fatalf("不正なログが通ってしまう: %+v", got)
			}
			if got != nil {
				t.Errorf("失敗時に nil でない値が返る: %+v", got)
			}
		})
	}
}

// 自重種目は 0kg で記録する。記録自体は正当で、1RMが推定できないだけ。
func TestNewSetLog_AcceptsBodyweight(t *testing.T) {
	p := setLogParams()
	p.WeightKg = 0
	s := mustSetLog(t, p)

	if s.Weight().Kg() != 0 {
		t.Errorf("重量が誤り: %v", s.Weight().Kg())
	}
	if got, ok := s.EstimatedOneRepMax(); ok {
		t.Errorf("自重種目から1RMが推定できてしまう: %v", got.Kg())
	}
}

// エラーメッセージがどのログの問題か特定できること。
// クライアントがまとめて送ってきたログのうち1件が不正なとき、
// どれを直せばいいのか分からないと運用できない。
func TestNewSetLog_ErrorsIdentifyTheLog(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*setlog.SetLogParams)
	}{
		{"実施日", func(p *setlog.SetLogParams) { p.PerformedOn = training.Date{} }},
		{"種目ID", func(p *setlog.SetLogParams) { p.ExerciseID = "" }},
		{"重量", func(p *setlog.SetLogParams) { p.WeightKg = -1 }},
		{"レップ", func(p *setlog.SetLogParams) { p.Reps = 0 }},
		{"RIR", func(p *setlog.SetLogParams) { p.RIR = -1 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := setLogParams()
			c.mutate(&p)
			_, err := setlog.NewSetLog(p)
			if err == nil {
				t.Fatal("エラーにならない")
			}
			if !strings.Contains(err.Error(), "01J0000000000000000000BNCH") {
				t.Errorf("エラーメッセージにログIDが含まれない: %v", err)
			}
		})
	}
}

func TestSetLog_EstimatedOneRepMax(t *testing.T) {
	s := mustSetLog(t, setLogParams())

	got, ok := s.EstimatedOneRepMax()
	if !ok {
		t.Fatal("推定できない")
	}
	want := 85.0 * (1 + 11.0/30.0)
	if math.Abs(got.Kg()-want) > 1e-5 {
		t.Errorf("got %v, want %v", got.Kg(), want)
	}
}

// Epley 式の適用範囲外は推定しない。
// 20kg×100レップから 86.7kg を推定すると、実績の3.5倍が処方される。
func TestSetLog_EstimatedOneRepMaxRejectsOutOfRange(t *testing.T) {
	p := setLogParams()
	p.WeightKg, p.Reps, p.RIR = 20, 100, 0

	if got, ok := mustSetLog(t, p).EstimatedOneRepMax(); ok {
		t.Errorf("適用範囲外の記録から推定できてしまう: %v", got.Kg())
	}
}

func TestSetLog_SameIdentity(t *testing.T) {
	a := mustSetLog(t, setLogParams())

	// 同じIDなら、内容が違っても同じログ（再送とみなす）。
	p := setLogParams()
	p.WeightKg, p.Reps = 90, 5
	if !a.SameIdentity(mustSetLog(t, p)) {
		t.Error("同じIDのログが別物と判定された")
	}

	p2 := setLogParams()
	p2.ID = "01J0000000000000000000OTHR"
	if a.SameIdentity(mustSetLog(t, p2)) {
		t.Error("違うIDのログが同一と判定された")
	}

	if a.SameIdentity(nil) {
		t.Error("nil と同一と判定された")
	}
	var nilLog *setlog.SetLog
	if nilLog.SameIdentity(a) {
		t.Error("nil レシーバが同一と判定された")
	}
}

// クライアント採番の ID をそのまま使うこと。
// サーバーが振り直すと、同じログの再送が別のログとして二重登録される。
func TestSetLog_PreservesClientAssignedID(t *testing.T) {
	for _, id := range []string{
		"01J0000000000000000000BNCH",
		"01ARZ3NDEKTSV4RRFFQ69G5FAV",
		"a",
		strings.Repeat("x", 64),
	} {
		p := setLogParams()
		p.ID = id
		if got := mustSetLog(t, p).ID(); string(got) != id {
			t.Errorf("IDが書き換えられた: %q → %q", id, got)
		}
	}
}

// ID は永続化キーとログ出力に乗る。制御文字を通すとログが分断され、
// キーとして扱えない値が入り込む。
func TestNewSetLog_RejectsControlCharactersInID(t *testing.T) {
	for _, id := range []string{
		"01J\nDROP", "01J\tX", "01J\x00X", "\x01\x02\x03", "01J\x7fX", "01J\rX",
	} {
		p := setLogParams()
		p.ID = id
		if got, err := setlog.NewSetLog(p); err == nil {
			t.Errorf("制御文字を含むIDが通ってしまう: %q → %+v", id, got)
		}
	}

	// 通常の識別子は通ること。
	for _, id := range []string{
		"01ARZ3NDEKTSV4RRFFQ69G5FAV",
		"550e8400-e29b-41d4-a716-446655440000",
		"1",
	} {
		p := setLogParams()
		p.ID = id
		if _, err := setlog.NewSetLog(p); err != nil {
			t.Errorf("正当なIDが弾かれた: %q (%v)", id, err)
		}
	}
}

// 「同じIDだが内容が違う」を検出できること。
//
// リポジトリは ID をキーに上書きするので、クライアントの採番ミスで
// 異なるセットに同じ ID が振られると実績が黙って1件消える。
// SameIdentity だけでは区別できないため、値等価が必要。
func TestSetLog_Equals(t *testing.T) {
	a := mustSetLog(t, setLogParams())

	same := mustSetLog(t, setLogParams())
	if !a.Equals(same) {
		t.Error("同じ内容のログが等しくない")
	}

	// 同じID・違う内容＝採番ミス。SameIdentity は true だが Equals は false。
	p := setLogParams()
	p.WeightKg, p.Reps = 100, 5
	mistaken := mustSetLog(t, p)
	if !a.SameIdentity(mistaken) {
		t.Error("同じIDが別物と判定された")
	}
	if a.Equals(mistaken) {
		t.Error("内容が違うのに等しいと判定された。採番ミスを検出できない")
	}

	// 各フィールドの違いを検出すること。
	for _, c := range []struct {
		name   string
		mutate func(*setlog.SetLogParams)
	}{
		{"ID", func(p *setlog.SetLogParams) { p.ID = "OTHER" }},
		{"実施日", func(p *setlog.SetLogParams) { p.PerformedOn = training.MustDate(2026, time.August, 17) }},
		{"種目", func(p *setlog.SetLogParams) { p.ExerciseID = "squat" }},
		{"重量", func(p *setlog.SetLogParams) { p.WeightKg = 90 }},
		{"レップ", func(p *setlog.SetLogParams) { p.Reps = 10 }},
		{"RIR", func(p *setlog.SetLogParams) { p.RIR = 1 }},
	} {
		t.Run(c.name, func(t *testing.T) {
			q := setLogParams()
			c.mutate(&q)
			if a.Equals(mustSetLog(t, q)) {
				t.Errorf("%s が違うのに等しいと判定された", c.name)
			}
		})
	}

	if a.Equals(nil) {
		t.Error("nil と等しいと判定された")
	}
	var nilLog *setlog.SetLog
	if !nilLog.Equals(nil) {
		t.Error("nil 同士が等しくない")
	}
}
