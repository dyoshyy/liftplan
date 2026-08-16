package training_test

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func mkAccessory(t *testing.T, id string, stimulus map[training.MuscleRegion]float64) *training.Exercise {
	t.Helper()
	return mustExercise(t, training.ExerciseParams{
		ID:          id,
		Name:        id,
		Kind:        training.KindAccessory,
		Stimulus:    stimulus,
		IncrementKg: 2.5,
	})
}

func accessoryPool(t *testing.T) []*training.Exercise {
	t.Helper()
	return []*training.Exercise{
		mkAccessory(t, "incline", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "incline_db", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "dip", map[training.MuscleRegion]float64{training.ChestLower: 1.0}),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
		mkAccessory(t, "calf_raise", map[training.MuscleRegion]float64{training.Calf: 1.0}),
	}
}

func today() training.Date { return training.MustDate(2026, time.August, 16) }

func TestAccessorySelector_PicksLargestResidualFirst(t *testing.T) {
	s := training.DefaultAccessorySelector()
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 3, training.Biceps: 1},
		accessoryPool(t), training.NewHistory(nil), today(),
	)
	if len(got) == 0 || got[0] != training.ExerciseID("incline") {
		t.Errorf("残差最大の区分が先に選ばれていない: %v", got)
	}
}

// スロット数は残差の合計から決まる。固定にすると、残差が小さい日に
// 過剰なボリュームを積み、大きい日には週目標に届かない。
func TestAccessorySelector_SlotCountFollowsResidual(t *testing.T) {
	s := training.DefaultAccessorySelector()
	pool := accessoryPool(t)
	h := training.NewHistory(nil)

	cases := []struct {
		name     string
		residual map[training.MuscleRegion]float64
		want     int
	}{
		{"残差1セット分", map[training.MuscleRegion]float64{training.ChestUpper: 1}, 1},
		{"残差3セット分", map[training.MuscleRegion]float64{training.ChestUpper: 3}, 1},
		{"残差4セット分", map[training.MuscleRegion]float64{
			training.ChestUpper: 3, training.Biceps: 1}, 2},
		{"残差なし", map[training.MuscleRegion]float64{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.Select(c.residual, pool, h, today()); len(got) != c.want {
				t.Errorf("スロット数が誤り: got %d (%v), want %d", len(got), got, c.want)
			}
		})
	}
}

// セッションが長くなりすぎないよう、スロット数には上限がある。
func TestAccessorySelector_SlotCountIsCapped(t *testing.T) {
	s, err := training.NewAccessorySelector(2, 3, 2)
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}

	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 20, training.Biceps: 20, training.Calf: 20,
	}
	if got := s.Select(residual, accessoryPool(t), training.NewHistory(nil), today()); len(got) > 2 {
		t.Errorf("スロット数の上限が効いていない: %d (%v)", len(got), got)
	}
}

func TestAccessorySelector_SkipsRecentlyStimulatedRegion(t *testing.T) {
	s := training.DefaultAccessorySelector()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "recent", 15, "incline", 30, 10, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 6, training.Biceps: 3},
		accessoryPool(t), h, today(),
	)
	for _, id := range got {
		if id == training.ExerciseID("incline") || id == training.ExerciseID("incline_db") {
			t.Errorf("48時間以内に刺激済みの区分が選ばれている: %v", got)
		}
	}
}

// 当日のログは回復期間の判定に含めない。
//
// 含めると、セッション中に数セット記録してから計画を開き直したとき、
// たった今やった種目の筋区分が「最近刺激した」と判定され、
// そのセッションの補助枠から自分自身が消える。
func TestAccessorySelector_IgnoresTodaysOwnLogs(t *testing.T) {
	s := training.DefaultAccessorySelector()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "today", 16, "incline", 30, 10, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 6},
		accessoryPool(t), h, today(),
	)
	if len(got) == 0 {
		t.Error("当日のログで自分自身の枠が消えている")
	}
}

// 回復期間の境界。2日前は含み、3日前は含まない。
func TestAccessorySelector_RecoveryBoundary(t *testing.T) {
	s := training.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{training.ChestUpper: 6}

	cases := []struct {
		name        string
		day         int
		wantBlocked bool
	}{
		{"1日前は回復期間内", 15, true},
		{"2日前は回復期間内", 14, true},
		{"3日前は回復期間外", 13, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := training.NewHistory([]*training.SetLog{
				mkLog(t, "past", c.day, "incline", 30, 10, 2),
			})
			got := s.Select(residual, accessoryPool(t), h, today())

			blocked := len(got) == 0
			if blocked != c.wantBlocked {
				t.Errorf("塞がれ=%v（期待 %v）: %v", blocked, c.wantBlocked, got)
			}
		})
	}
}

func TestAccessorySelector_PrefersLeastRecentlyUsed(t *testing.T) {
	s := training.DefaultAccessorySelector()
	h := training.NewHistory([]*training.SetLog{
		mkLog(t, "a", 10, "incline", 30, 10, 2),
		mkLog(t, "b", 1, "incline_db", 30, 10, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 3},
		accessoryPool(t), h, today(),
	)
	if len(got) != 1 || got[0] != training.ExerciseID("incline_db") {
		t.Errorf("間隔が空いている種目が選ばれていない: %v", got)
	}
}

func TestAccessorySelector_NoDuplicatesInOneSession(t *testing.T) {
	s := training.DefaultAccessorySelector()
	got := s.Select(
		map[training.MuscleRegion]float64{
			training.ChestUpper: 20, training.Biceps: 20, training.Calf: 20,
		},
		accessoryPool(t), training.NewHistory(nil), today(),
	)

	seen := map[training.ExerciseID]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("同じ種目が二度選ばれた: %v", got)
		}
		seen[id] = true
	}
}

func TestAccessorySelector_EmptyInputs(t *testing.T) {
	s := training.DefaultAccessorySelector()
	pool := accessoryPool(t)
	h := training.NewHistory(nil)
	residual := map[training.MuscleRegion]float64{training.ChestUpper: 6}

	if got := s.Select(nil, pool, h, today()); len(got) != 0 {
		t.Errorf("残差が無いのに選ばれた: %v", got)
	}
	if got := s.Select(residual, nil, h, today()); len(got) != 0 {
		t.Errorf("種目プールが空なのに選ばれた: %v", got)
	}
	if got := s.Select(residual, pool, h, training.Date{}); len(got) != 0 {
		t.Errorf("基準日が無いのに選ばれた: %v", got)
	}

	var zero training.AccessorySelector
	if got := zero.Select(residual, pool, h, today()); len(got) != 0 {
		t.Errorf("ゼロ値のセレクタが選んだ: %v", got)
	}
}

func TestAccessorySelector_IgnoresNonAccessory(t *testing.T) {
	s := training.DefaultAccessorySelector()
	bench := mustExercise(t, benchParams())

	p := benchParams()
	p.ID, p.Kind, p.DefaultRatioToMain = "larsen", training.KindVariation, 0.9
	larsen := mustExercise(t, p)

	pool := append(accessoryPool(t), bench, larsen)
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestMid: 10, training.TricepsLateral: 5},
		pool, training.NewHistory(nil), today(),
	)

	for _, id := range got {
		if id == training.ExerciseID("bench") || id == training.ExerciseID("larsen") {
			t.Errorf("補助種目でないものが選ばれた: %v", got)
		}
	}
}

func TestAccessorySelector_SkipsNilExercises(t *testing.T) {
	s := training.DefaultAccessorySelector()
	pool := append([]*training.Exercise{nil}, accessoryPool(t)...)
	pool = append(pool, nil)

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 3},
		pool, training.NewHistory(nil), today(),
	)
	if len(got) != 1 {
		t.Errorf("nil が混ざると選べない: %v", got)
	}
}

// 埋められる種目が無い区分は諦めて、次の区分へ進むこと。
// 諦めないと同じ区分を選び続けて無限ループになる。
func TestAccessorySelector_GivesUpOnUncoverableRegions(t *testing.T) {
	s := training.DefaultAccessorySelector()

	// 大腿四頭筋を埋められる補助種目がプールに無い。
	got := s.Select(
		map[training.MuscleRegion]float64{training.Quad: 20, training.Biceps: 3},
		accessoryPool(t), training.NewHistory(nil), today(),
	)
	if len(got) != 1 || got[0] != training.ExerciseID("curl") {
		t.Errorf("埋められる区分に進んでいない: %v", got)
	}
}

func TestAccessorySelector_IsDeterministic(t *testing.T) {
	s := training.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 5, training.ChestLower: 5, training.Biceps: 5,
	}
	pool := accessoryPool(t)
	h := training.NewHistory(nil)

	first := s.Select(residual, pool, h, today())
	for range 50 {
		got := s.Select(residual, pool, h, today())
		if len(got) != len(first) {
			t.Fatalf("実行のたびに件数が変わる: %v vs %v", first, got)
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("実行のたびに結果が変わる: %v vs %v", first, got)
			}
		}
	}
}

// 残差マップを破壊しないこと。呼び出し側が同じマップを使い回す。
func TestAccessorySelector_DoesNotMutateResidual(t *testing.T) {
	s := training.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 6, training.Biceps: 3,
	}
	before := len(residual)
	beforeChest := residual[training.ChestUpper]

	s.Select(residual, accessoryPool(t), training.NewHistory(nil), today())

	if len(residual) != before || residual[training.ChestUpper] != beforeChest {
		t.Errorf("残差マップが書き換わっている: %v", residual)
	}
}

func TestNewAccessorySelector_RejectsBadParams(t *testing.T) {
	cases := []struct{ recovery, sets, maxSlots int }{
		{-1, 3, 8}, {2, 0, 8}, {2, -1, 8}, {2, 3, 0}, {2, 3, -1},
	}
	for _, c := range cases {
		if _, err := training.NewAccessorySelector(c.recovery, c.sets, c.maxSlots); err == nil {
			t.Errorf("不正なパラメータが通ってしまう: %+v", c)
		}
	}
	if _, err := training.NewAccessorySelector(0, 1, 1); err != nil {
		t.Errorf("境界値が弾かれた: %v", err)
	}
}

func TestDefaultAccessorySelector_Constants(t *testing.T) {
	s := training.DefaultAccessorySelector()
	if s.RecoveryDays() != 2 {
		t.Errorf("回復日数が誤り: %d", s.RecoveryDays())
	}
	if s.SetsPerAccessory() != 3 {
		t.Errorf("セット数が誤り: %d", s.SetsPerAccessory())
	}
	if s.MaxSlots() != 8 {
		t.Errorf("スロット上限が誤り: %d", s.MaxSlots())
	}
}

// 1種目で埋まらない残差には、同じ区分の別の種目を続けて充てること。
//
// 選んだ時点で区分ごと消すと、残差6セットの区分に3セットしか充てられない。
func TestAccessorySelector_ConsumesResidualPartially(t *testing.T) {
	s := training.DefaultAccessorySelector()

	// 大胸筋上部に6セット必要。補助は3セットずつなので2種目要る。
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 6},
		accessoryPool(t), training.NewHistory(nil), today(),
	)

	if len(got) != 2 {
		t.Fatalf("残差6セットに対し %d 種目しか選ばれない: %v", len(got), got)
	}
	for _, id := range got {
		if id != training.ExerciseID("incline") && id != training.ExerciseID("incline_db") {
			t.Errorf("大胸筋上部を埋めない種目が選ばれた: %v", got)
		}
	}
}

// 部分的に埋まった区分は、残りぶんだけ残差に残ること。
func TestAccessorySelector_LeavesRemainderForOtherRegions(t *testing.T) {
	s := training.DefaultAccessorySelector()

	// 上部に4セット（3セットでは埋まらない）、二頭に3セット。
	// 上部→二頭→上部 の順に3種目選ばれるはず。
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 4, training.Biceps: 3},
		accessoryPool(t), training.NewHistory(nil), today(),
	)

	chest, biceps := 0, 0
	for _, id := range got {
		switch id {
		case "incline", "incline_db":
			chest++
		case "curl":
			biceps++
		}
	}
	if chest != 2 {
		t.Errorf("大胸筋上部の残りが埋められていない: %v", got)
	}
	if biceps != 1 {
		t.Errorf("二頭が埋められていない: %v", got)
	}
}

// 種目プールの並び順で結果が変わらないこと。
// プールはリポジトリの取得順に依存するので、順序に敏感だと再現性が壊れる。
func TestAccessorySelector_IsIndependentOfPoolOrder(t *testing.T) {
	s := training.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 6, training.Biceps: 3, training.Calf: 3,
	}
	h := training.NewHistory(nil)

	forward := accessoryPool(t)
	first := s.Select(residual, forward, h, today())

	reversed := make([]*training.Exercise, 0, len(forward))
	for i := len(forward) - 1; i >= 0; i-- {
		reversed = append(reversed, forward[i])
	}
	got := s.Select(residual, reversed, h, today())

	if len(got) != len(first) {
		t.Fatalf("プール順で件数が変わる: %v vs %v", first, got)
	}
	for i := range got {
		if got[i] != first[i] {
			t.Fatalf("プール順で結果が変わる: %v vs %v", first, got)
		}
	}
}
