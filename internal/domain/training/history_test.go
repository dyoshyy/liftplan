package training_test

import (
	"fmt"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mkLog(t *testing.T, id string, day int, exercise string, kg float64, reps, rir int) *training.SetLog {
	t.Helper()
	s, err := training.NewSetLog(training.SetLogParams{
		ID:          id,
		PerformedOn: training.MustDate(2026, time.August, day),
		ExerciseID:  exercise,
		WeightKg:    kg,
		Reps:        reps,
		RIR:         rir,
	})
	if err != nil {
		t.Fatalf("ログ生成に失敗 (%s): %v", id, err)
	}
	return s
}

func TestHistory_Empty(t *testing.T) {
	h := training.NewHistory(nil)

	if !h.IsEmpty() {
		t.Error("空の履歴が IsEmpty でない")
	}
	if h.Len() != 0 || h.SessionCount() != 0 {
		t.Errorf("件数が誤り: %d %d", h.Len(), h.SessionCount())
	}
	if len(h.Sessions()) != 0 {
		t.Error("空の履歴からセッションが出てくる")
	}
	if _, ok := h.LastPerformed("bench"); ok {
		t.Error("空の履歴で最終実施日が取れてしまう")
	}
}

func TestNewHistory_SkipsNil(t *testing.T) {
	// pool や logs に nil が混ざっても落ちないこと。
	h := training.NewHistory([]*training.SetLog{
		nil, mkLog(t, "a", 10, "bench", 85, 8, 2), nil,
	})
	if h.Len() != 1 {
		t.Errorf("nil が除かれていない: %d", h.Len())
	}
}

// 同じ ID のログが複数含まれる場合、後のものを採用する。
// リポジトリが冪等に上書きする挙動と揃えないと、
// 「保存したのに古い値で計画される」というズレが生まれる。
func TestNewHistory_DeduplicatesByID(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "same", 10, "bench", 85, 8, 2),
		mkLog(t, "other", 10, "bench", 80, 8, 2),
		mkLog(t, "same", 10, "bench", 90, 8, 2),
	})

	if h.Len() != 2 {
		t.Fatalf("重複が除かれていない: %d", h.Len())
	}
	for _, l := range h.Logs() {
		if l.ID() == "same" && l.Weight().Kg() != 90 {
			t.Errorf("後のログが採用されていない: %v", l.Weight().Kg())
		}
	}
}

func TestHistory_SessionsAreGroupedAndSorted(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "c", 20, "bench", 85, 8, 2),
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "b", 10, "bench", 80, 8, 2),
	})

	sessions := h.Sessions()
	if len(sessions) != 2 {
		t.Fatalf("セッション数が誤り: %d", len(sessions))
	}
	if !sessions[0].Date().Before(sessions[1].Date()) {
		t.Error("セッションが日付昇順になっていない")
	}
	if len(sessions[0].Logs()) != 2 {
		t.Errorf("同日ログがまとまっていない: %d", len(sessions[0].Logs()))
	}
	if h.SessionCount() != 2 {
		t.Errorf("SessionCount が誤り: %d", h.SessionCount())
	}
}

// 何度呼んでも同じ順序であること。マップの反復順に依存すると、
// EWMA の適用順が変わって推定1RMがぶれる。
func TestHistory_SessionsAreStable(t *testing.T) {
	logs := make([]*training.SetLog, 0, 20)
	for i := 1; i <= 20; i++ {
		logs = append(logs, mkLog(t, fmt.Sprintf("l%02d", i), i, "bench", 80+float64(i), 8, 2))
	}
	h := training.NewHistory(logs)

	first := h.Sessions()
	for range 30 {
		got := h.Sessions()
		if len(got) != len(first) {
			t.Fatalf("件数が変わる: %d vs %d", len(first), len(got))
		}
		for i := range got {
			if !got[i].Date().Equal(first[i].Date()) {
				t.Fatalf("順序が変わる: %v vs %v", first[i].Date(), got[i].Date())
			}
		}
	}
}

func TestHistory_SessionsAreDefensivelyCopied(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 8, 2),
	})

	logs := h.Logs()
	logs[0] = nil
	if h.Logs()[0] == nil {
		t.Error("Logs の書き換えが内部状態に波及している")
	}

	sessionLogs := h.Sessions()[0].Logs()
	sessionLogs[0] = nil
	if h.Sessions()[0].Logs()[0] == nil {
		t.Error("Session.Logs の書き換えが内部状態に波及している")
	}
}

func TestHistory_ForExercise(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "b", 10, "squat", 100, 8, 2),
	})

	only := h.ForExercise("bench")
	if only.Len() != 1 {
		t.Fatalf("絞り込みが誤り: %d", only.Len())
	}
	if only.Logs()[0].ExerciseID() != training.ExerciseID("bench") {
		t.Error("違う種目が混ざっている")
	}
	if training.NewHistory(nil).ForExercise("bench").Len() != 0 {
		t.Error("空の履歴からログが出てくる")
	}
}

func TestHistory_DateFilters(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "b", 15, "bench", 82.5, 8, 2),
		mkLog(t, "c", 20, "bench", 85, 8, 2),
	})
	cut := training.MustDate(2026, time.August, 15)

	// 境界日を含むかどうかは 48時間ルールの判定に直結する。
	if got := h.OnOrAfter(cut).Len(); got != 2 {
		t.Errorf("OnOrAfter が境界日を含んでいない: %d", got)
	}
	if got := h.Before(cut).Len(); got != 1 {
		t.Errorf("Before が境界日を含んでしまっている: %d", got)
	}
	if got := h.OnOrAfter(cut).Before(cut).Len(); got != 0 {
		t.Errorf("両方の条件を満たすログがある: %d", got)
	}
}

func TestHistory_LastPerformed(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 8, 2),
		mkLog(t, "c", 20, "bench", 85, 8, 2),
		mkLog(t, "b", 15, "bench", 82.5, 8, 2),
		mkLog(t, "d", 25, "squat", 120, 8, 2),
	})

	got, ok := h.LastPerformed("bench")
	if !ok {
		t.Fatal("最終実施日が取れない")
	}
	if want := training.MustDate(2026, time.August, 20); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if _, ok := h.LastPerformed("deadlift"); ok {
		t.Error("実施していない種目の最終実施日が取れる")
	}
}

func TestTrainingSession_MedianResistsOutlier(t *testing.T) {
	// 同日3セット。1セットだけ異常に高い記録が混ざっている。
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 85, 9, 2),
		mkLog(t, "b", 10, "bench", 85, 9, 2),
		mkLog(t, "c", 10, "bench", 200, 9, 2), // 外れ値
	})

	got, ok := h.Sessions()[0].MedianOneRepMax()
	if !ok {
		t.Fatal("中央値が取れない")
	}
	want := 85.0 * (1 + 11.0/30.0)
	if math.Abs(got.Kg()-want) > 1e-5 {
		t.Errorf("外れ値に引きずられている: got %v, want %v", got.Kg(), want)
	}
}

func TestTrainingSession_MedianOfEvenCount(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 80, 10, 0),
		mkLog(t, "b", 10, "bench", 90, 10, 0),
	})

	got, _ := h.Sessions()[0].MedianOneRepMax()
	want := (80.0*(1+10.0/30.0) + 90.0*(1+10.0/30.0)) / 2
	if math.Abs(got.Kg()-want) > 1e-5 {
		t.Errorf("偶数個の中央値が誤り: got %v, want %v", got.Kg(), want)
	}
}

// 推定できないセットを 0 として混ぜると代表値が崩壊する。
//
// 加重ディップスで始めて疲労したら自重に切り替える、という日常的な
// セッションで起きる。0 を混ぜると 46kg が 0kg になる。
func TestTrainingSession_MedianExcludesUnestimableSets(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "dip", 40, 8, 2),
		mkLog(t, "b", 10, "dip", 30, 8, 1),
		mkLog(t, "c", 10, "dip", 0, 10, 1), // 自重
		mkLog(t, "d", 10, "dip", 0, 9, 0),  // 自重
		mkLog(t, "e", 10, "dip", 0, 8, 0),  // 自重
	})

	got, ok := h.Sessions()[0].MedianOneRepMax()
	if !ok {
		t.Fatal("推定できるセットがあるのに中央値が取れない")
	}
	if got.IsZero() {
		t.Fatal("代表値が0に崩壊している")
	}

	// 加重2セットの推定1RMの平均が期待値。
	a := 40.0 * (1 + 10.0/30.0)
	b := 30.0 * (1 + 9.0/30.0)
	want := (a + b) / 2
	if math.Abs(got.Kg()-want) > 1e-5 {
		t.Errorf("got %v, want %v", got.Kg(), want)
	}
}

func TestTrainingSession_MedianWithNoEstimableSets(t *testing.T) {
	// 全セットが自重なら、そのセッションは推定1RMを持たない。
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "dip", 0, 10, 1),
		mkLog(t, "b", 10, "dip", 0, 9, 0),
	})

	if got, ok := h.Sessions()[0].MedianOneRepMax(); ok {
		t.Errorf("推定できないセットだけなのに中央値が取れる: %v", got.Kg())
	}
}

// 返り値は必ずコンストラクタを通った有効な値であること。
// 構造体リテラルで組み立てると、ok=true なのに不変条件違反の値が流通する。
func TestTrainingSession_MedianReturnsValidOneRepMax(t *testing.T) {
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "bench", 85, 9, 2),
		mkLog(t, "b", 10, "bench", 87.5, 8, 2),
		mkLog(t, "c", 10, "bench", 0, 10, 0),
	})

	got, ok := h.Sessions()[0].MedianOneRepMax()
	if !ok {
		t.Fatal("中央値が取れない")
	}
	if got.IsZero() {
		t.Error("ok=true なのにゼロ値")
	}
	if _, err := training.NewOneRepMax(got.Kg()); err != nil {
		t.Errorf("コンストラクタが拒否する値が返っている: %v", err)
	}
}

func TestTrainingSession_EmptyIsSafe(t *testing.T) {
	var s training.TrainingSession

	if !s.IsEmpty() {
		t.Error("ゼロ値が空でない")
	}
	if len(s.Logs()) != 0 {
		t.Error("ゼロ値からログが出てくる")
	}
	if _, ok := s.MedianOneRepMax(); ok {
		t.Error("ゼロ値から中央値が取れる")
	}
}

// 中央値も量子化されていること。
//
// 偶数個の平均は 118.08333350000001 のような端数を生む。
// 生成を NewOneRepMax に通していれば、量子化と検証が同時に効く。
func TestTrainingSession_MedianIsQuantized(t *testing.T) {
	// この組み合わせは、量子化済みの値どうしの平均が
	// 21.333333500000002 という7桁の端数を生む。
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "curl", 20, 1, 0),
		mkLog(t, "b", 10, "curl", 20, 3, 0),
	})

	got, ok := h.Sessions()[0].MedianOneRepMax()
	if !ok {
		t.Fatal("中央値が取れない")
	}
	if n := decimalPlaces(strconv.FormatFloat(got.Kg(), 'f', -1, 64)); n > 6 {
		t.Errorf("中央値に端数が残っている: %s（小数点以下 %d 桁）",
			strconv.FormatFloat(got.Kg(), 'f', -1, 64), n)
	}
}
