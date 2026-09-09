package planning_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

func mkAccessory(t *testing.T, id string, stimulus map[training.MuscleRegion]float64) *exercise.Exercise {
	t.Helper()
	return mustExercise(t, exercise.ExerciseParams{
		ID:          id,
		Name:        id,
		Stimulus:    stimulus,
		IncrementKg: 2.5,
	})
}

func accessoryPool(t *testing.T) []*exercise.Exercise {
	t.Helper()
	return []*exercise.Exercise{
		mkAccessory(t, "incline", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "incline_db", map[training.MuscleRegion]float64{training.ChestUpper: 1.0}),
		mkAccessory(t, "dip", map[training.MuscleRegion]float64{training.ChestLower: 1.0}),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
		mkAccessory(t, "calf_raise", map[training.MuscleRegion]float64{training.Calf: 1.0}),
	}
}

func today() training.Date { return training.MustDate(2026, time.August, 16) }

func TestAccessorySelector_PicksLargestResidualFirst(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 3, training.Biceps: 1},
		accessoryPool(t), setlog.NewHistory(nil), today(), nil,
	)
	if len(got) == 0 || got[0] != exercise.ExerciseID("incline") {
		t.Errorf("残差最大の区分が先に選ばれていない: %v", got)
	}
}

// 残差が無くなるまで選び、無くなったら止まること。
//
// 事前にスロット数を計算すると、1種目が複数区分を埋める事実を無視した
// 見積もりになり、実際より多く積む日と、埋める余地を残して終わる日の
// 両方が生まれる。
func TestAccessorySelector_StopsWhenResidualIsGone(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	pool := accessoryPool(t)
	h := setlog.NewHistory(nil)

	cases := []struct {
		name     string
		residual map[training.MuscleRegion]float64
		want     int
	}{
		{"残差1セット分", map[training.MuscleRegion]float64{training.ChestUpper: 1}, 1},
		{"残差3セット分", map[training.MuscleRegion]float64{training.ChestUpper: 3}, 1},
		{"残差4セット分", map[training.MuscleRegion]float64{training.ChestUpper: 4}, 2},
		{"2区分それぞれ3セット", map[training.MuscleRegion]float64{
			training.ChestUpper: 3, training.Biceps: 3}, 2},
		{"残差なし", map[training.MuscleRegion]float64{}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.Select(c.residual, pool, h, today(), nil); len(got) != c.want {
				t.Errorf("選ばれた種目数が誤り: got %d (%v), want %d", len(got), got, c.want)
			}
		})
	}
}

// 埋める余地があるのにスロットを捨てないこと。
//
// 事前にスロット数を決めて「埋められない区分」で打ち切ると、
// 他の区分に空きがあっても使われないまま終わる。
func TestAccessorySelector_UsesSlotsForCoverableRegions(t *testing.T) {
	s := planning.DefaultAccessorySelector()

	// 大腿四頭筋を埋められる補助種目がプールに無い。
	// 二頭は curl 1種目だけなので、埋められるのは1種目。
	got := s.Select(
		map[training.MuscleRegion]float64{training.Quad: 10, training.Biceps: 3},
		accessoryPool(t), setlog.NewHistory(nil), today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("curl") {
		t.Errorf("埋められる区分が使われていない: %v", got)
	}
}

// セッションが長くなりすぎないよう、スロット数には上限がある。
func TestAccessorySelector_SlotCountIsCapped(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 2)
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}

	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 20, training.Biceps: 20, training.Calf: 20,
	}
	if got := s.Select(residual, accessoryPool(t), setlog.NewHistory(nil), today(), nil); len(got) > 2 {
		t.Errorf("スロット数の上限が効いていない: %d (%v)", len(got), got)
	}
}

func TestAccessorySelector_SkipsRecentlyStimulatedRegion(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "recent", 15, "incline", 30, 10, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 6, training.Biceps: 3},
		accessoryPool(t), h, today(), nil,
	)
	for _, id := range got {
		if id == exercise.ExerciseID("incline") || id == exercise.ExerciseID("incline_db") {
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
	s := planning.DefaultAccessorySelector()
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "today", 16, "incline", 30, 10, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 6},
		accessoryPool(t), h, today(), nil,
	)
	if len(got) == 0 {
		t.Error("当日のログで自分自身の枠が消えている")
	}
}

// 回復期間の境界。
//
// recoveryDays=2 は「2日空ければ解禁」の意味。月曜の記録は火曜を塞ぐが
// 水曜は解禁される。閉区間にすると実質72時間ルールになり、
// 月水金の水曜がほぼ何も選べなくなる。
func TestAccessorySelector_RecoveryBoundary(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{training.ChestUpper: 6}

	cases := []struct {
		name        string
		day         int
		wantBlocked bool
	}{
		{"1日前は回復期間内", 15, true},
		{"2日前は解禁される", 14, false},
		{"3日前は解禁される", 13, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := setlog.NewHistory([]*setlog.SetLog{
				mkLog(t, "past", c.day, "incline", 30, 10, 2),
			})
			got := s.Select(residual, accessoryPool(t), h, today(), nil)

			blocked := len(got) == 0
			if blocked != c.wantBlocked {
				t.Errorf("塞がれ=%v（期待 %v）: %v", blocked, c.wantBlocked, got)
			}
		})
	}
}

func TestAccessorySelector_PrefersLeastRecentlyUsed(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "a", 10, "incline", 30, 10, 2),
		mkLog(t, "b", 1, "incline_db", 30, 10, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 3},
		accessoryPool(t), h, today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("incline_db") {
		t.Errorf("間隔が空いている種目が選ばれていない: %v", got)
	}
}

func TestAccessorySelector_NoDuplicatesInOneSession(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	got := s.Select(
		map[training.MuscleRegion]float64{
			training.ChestUpper: 20, training.Biceps: 20, training.Calf: 20,
		},
		accessoryPool(t), setlog.NewHistory(nil), today(), nil,
	)

	seen := map[exercise.ExerciseID]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("同じ種目が二度選ばれた: %v", got)
		}
		seen[id] = true
	}
}

func TestAccessorySelector_EmptyInputs(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	pool := accessoryPool(t)
	h := setlog.NewHistory(nil)
	residual := map[training.MuscleRegion]float64{training.ChestUpper: 6}

	if got := s.Select(nil, pool, h, today(), nil); len(got) != 0 {
		t.Errorf("残差が無いのに選ばれた: %v", got)
	}
	if got := s.Select(residual, nil, h, today(), nil); len(got) != 0 {
		t.Errorf("種目プールが空なのに選ばれた: %v", got)
	}
	if got := s.Select(residual, pool, h, training.Date{}, nil); len(got) != 0 {
		t.Errorf("基準日が無いのに選ばれた: %v", got)
	}

	var zero planning.AccessorySelector
	if got := zero.Select(residual, pool, h, today(), nil); len(got) != 0 {
		t.Errorf("ゼロ値のセレクタが選んだ: %v", got)
	}
}

// 除外していない種目は、種別に関わらず候補になる。
//
// 元は「種別が MAIN のものは補助として選ばれない」を検査していた。
// メイン/補助は種目マスタの属性ではなく利用者の目標だった、というのが
// D-117 の結論で、目標は Program.declared が持つ。除くのは今日メインで
// 処方した種目だけ。
//
// 除きすぎると、脚の日にスクワットがどこにも出なくなる。宣言は
// 「伸ばしたい」であって「ヘビーでしかやらない」ではない。
func TestAccessorySelector_PicksFromEveryExerciseNotExcluded(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	bench := mustExercise(t, benchParams())

	p := benchParams()
	p.ID = "squat"
	squat := mustExercise(t, p)

	pool := append(accessoryPool(t), bench, squat)
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestMid: 10, training.TricepsLateral: 5},
		pool, setlog.NewHistory(nil), today(), nil,
	)

	if !slices.Contains(got, exercise.ExerciseID("bench")) &&
		!slices.Contains(got, exercise.ExerciseID("squat")) {
		t.Errorf("除外していない種目が候補から外れている: %v", got)
	}
}

func TestAccessorySelector_SkipsNilExercises(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	pool := append([]*exercise.Exercise{nil}, accessoryPool(t)...)
	pool = append(pool, nil)

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 3},
		pool, setlog.NewHistory(nil), today(), nil,
	)
	if len(got) != 1 {
		t.Errorf("nil が混ざると選べない: %v", got)
	}
}

func TestAccessorySelector_IsDeterministic(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 5, training.ChestLower: 5, training.Biceps: 5,
	}
	pool := accessoryPool(t)
	h := setlog.NewHistory(nil)

	first := s.Select(residual, pool, h, today(), nil)
	for range 50 {
		got := s.Select(residual, pool, h, today(), nil)
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
	s := planning.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 6, training.Biceps: 3,
	}
	before := len(residual)
	beforeChest := residual[training.ChestUpper]

	s.Select(residual, accessoryPool(t), setlog.NewHistory(nil), today(), nil)

	if len(residual) != before || residual[training.ChestUpper] != beforeChest {
		t.Errorf("残差マップが書き換わっている: %v", residual)
	}
}

func TestNewAccessorySelector_RejectsBadParams(t *testing.T) {
	cases := []struct{ recovery, sets, maxSlots int }{
		{-1, 3, 8}, {2, 0, 8}, {2, -1, 8}, {2, 3, 0}, {2, 3, -1},
		// セット数の上限は SetCount の制約に従う。独自判定だと、
		// 上限超が通ったあと消費側で無言の no-op になり残差が減らない。
		{2, 101, 8}, {2, 1000, 8},
	}
	for _, c := range cases {
		if _, err := planning.NewAccessorySelector(c.recovery, c.sets, c.maxSlots); err == nil {
			t.Errorf("不正なパラメータが通ってしまう: %+v", c)
		}
	}
	if _, err := planning.NewAccessorySelector(0, 1, 1); err != nil {
		t.Errorf("境界値が弾かれた: %v", err)
	}
}

func TestDefaultAccessorySelector_Constants(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	if s.RecoveryDays() != 2 {
		t.Errorf("回復日数が誤り: %d", s.RecoveryDays())
	}
	if s.SetsPerAccessory().Int() != 3 {
		t.Errorf("セット数が誤り: %d", s.SetsPerAccessory().Int())
	}
	if s.MaxSlots() != 8 {
		t.Errorf("スロット上限が誤り: %d", s.MaxSlots())
	}
}

// 1種目で埋まらない残差には、同じ区分の別の種目を続けて充てること。
//
// 選んだ時点で区分ごと消すと、残差6セットの区分に3セットしか充てられない。
func TestAccessorySelector_ConsumesResidualPartially(t *testing.T) {
	s := planning.DefaultAccessorySelector()

	// 大胸筋上部に6セット必要。補助は3セットずつなので2種目要る。
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 6},
		accessoryPool(t), setlog.NewHistory(nil), today(), nil,
	)

	if len(got) != 2 {
		t.Fatalf("残差6セットに対し %d 種目しか選ばれない: %v", len(got), got)
	}
	for _, id := range got {
		if id != exercise.ExerciseID("incline") && id != exercise.ExerciseID("incline_db") {
			t.Errorf("大胸筋上部を埋めない種目が選ばれた: %v", got)
		}
	}
}

// 部分的に埋まった区分は、残りぶんだけ残差に残ること。
func TestAccessorySelector_LeavesRemainderForOtherRegions(t *testing.T) {
	s := planning.DefaultAccessorySelector()

	// 上部に4セット（3セットでは埋まらない）、二頭に3セット。
	// 上部→二頭→上部 の順に3種目選ばれるはず。
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 4, training.Biceps: 3},
		accessoryPool(t), setlog.NewHistory(nil), today(), nil,
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
	s := planning.DefaultAccessorySelector()
	residual := map[training.MuscleRegion]float64{
		training.ChestUpper: 6, training.Biceps: 3, training.Calf: 3,
	}
	h := setlog.NewHistory(nil)

	forward := accessoryPool(t)
	first := s.Select(residual, forward, h, today(), nil)

	reversed := make([]*exercise.Exercise, 0, len(forward))
	for i := len(forward) - 1; i >= 0; i-- {
		reversed = append(reversed, forward[i])
	}
	got := s.Select(residual, reversed, h, today(), nil)

	if len(got) != len(first) {
		t.Fatalf("プール順で件数が変わる: %v vs %v", first, got)
	}
	for i := range got {
		if got[i] != first[i] {
			t.Fatalf("プール順で結果が変わる: %v vs %v", first, got)
		}
	}
}

// 週目標の小さい区分が飢餓に陥らないこと。
//
// 残差の大きい順に選ぶと、目標の大きい区分（大腿四頭筋16セット）が常に勝ち、
// 小さい区分（僧帽筋上部6セット）にスロットが一度も回らない。
// 上限に張り付く低頻度では、その区分が永久に0セットのままになる。
func TestAccessorySelector_DoesNotStarveSmallRegions(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 2) // 上限2スロット
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}

	pool := []*exercise.Exercise{
		mkAccessory(t, "leg_ext", map[training.MuscleRegion]float64{training.Quad: 1.0}),
		mkAccessory(t, "leg_press", map[training.MuscleRegion]float64{training.Quad: 1.0}),
		mkAccessory(t, "shrug", map[training.MuscleRegion]float64{training.TrapUpper: 1.0}),
	}

	// 大腿四頭筋の残差が圧倒的に大きいが、僧帽筋上部は一度もやっていない。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "q1", 10, "leg_ext", 60, 10, 2),
	})
	residual := map[training.MuscleRegion]float64{
		training.Quad: 16, training.TrapUpper: 3,
	}

	got := s.Select(residual, pool, h, today(), nil)

	found := false
	for _, id := range got {
		if id == exercise.ExerciseID("shrug") {
			found = true
		}
	}
	if !found {
		t.Errorf("一度も刺激していない区分にスロットが回らない: %v", got)
	}
}

// 回復期間中の区分を主働筋として使う種目は避けること。
//
// 残差の判定にしか回復期間を使わないと、多関節種目が塞がれた区分を
// 平気で叩く。「48時間ルール」という名前が実態と合わなくなる。
func TestAccessorySelector_AvoidsRecoveringPrimaryMovers(t *testing.T) {
	s := planning.DefaultAccessorySelector()

	sideRaise := mkAccessory(t, "side_raise", map[training.MuscleRegion]float64{
		training.SideDelt: 1.0,
	})
	ohp := mkAccessory(t, "overhead_press", map[training.MuscleRegion]float64{
		training.FrontDelt: 1.0, training.SideDelt: 1.0,
	})
	frontRaise := mkAccessory(t, "front_raise", map[training.MuscleRegion]float64{
		training.FrontDelt: 1.0,
	})

	// 昨日サイドレイズをやったので、三角筋中部は回復期間中。
	// フロントレイズは今日から見て2日前に使ったので、間隔の観点では
	// オーバーヘッドプレス（未使用）の方が優先される。
	// つまり主働筋の回避が無ければ、オーバーヘッドプレスが選ばれる。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "y", 15, "side_raise", 10, 12, 2),
		mkLog(t, "f", 14, "front_raise", 8, 12, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.FrontDelt: 3},
		[]*exercise.Exercise{sideRaise, ohp, frontRaise}, h, today(), nil,
	)

	for _, id := range got {
		if id == exercise.ExerciseID("overhead_press") {
			t.Errorf("回復期間中の区分を主働筋として使う種目が選ばれた: %v", got)
		}
	}
	if len(got) != 1 || got[0] != exercise.ExerciseID("front_raise") {
		t.Errorf("回復期間を避けた種目が選ばれていない: %v", got)
	}
}

// 補助的な関与（主働筋でない）は許すこと。
// これも避けると多関節種目がほとんど選べなくなる。
func TestAccessorySelector_AllowsSecondaryInvolvementOfRecoveringRegions(t *testing.T) {
	s := planning.DefaultAccessorySelector()

	curl := mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0})
	row := mkAccessory(t, "row", map[training.MuscleRegion]float64{
		training.TrapMid: 1.0, training.Biceps: 0.3,
	})

	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "y", 15, "curl", 20, 10, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.TrapMid: 3},
		[]*exercise.Exercise{curl, row}, h, today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("row") {
		t.Errorf("補助的な関与まで避けている: %v", got)
	}
}

// 残差に異常値が混ざっても落ちないこと。
//
// +Inf の残差は常に最優先になったうえ、有限値を引いても減らない。
// 事前にスロット数を計算する実装では、int への変換で負の値になり panic した。
func TestAccessorySelector_SurvivesAbnormalResidual(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	pool := accessoryPool(t)
	h := setlog.NewHistory(nil)

	for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN(), -5} {
		got := s.Select(
			map[training.MuscleRegion]float64{training.ChestUpper: v},
			pool, h, today(), nil,
		)
		if len(got) != 0 {
			t.Errorf("異常な残差 %v が処理された: %v", v, got)
		}
	}

	// 極端に大きい有限値は「異常」ではないので処理してよいが、
	// スロット数の上限は必ず守ること。
	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 1e30},
		pool, h, today(), nil,
	)
	if len(got) > s.MaxSlots() {
		t.Errorf("巨大な残差でスロット上限を超えた: %d", len(got))
	}

	// 正常な区分と混ざっていても、正常なぶんだけ処理すること。
	got = s.Select(
		map[training.MuscleRegion]float64{
			training.ChestUpper: math.Inf(1), training.Biceps: 3,
		},
		pool, h, today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("curl") {
		t.Errorf("正常な区分が処理されていない: %v", got)
	}
}

// 未来日のログがある種目が永久に選ばれなくならないこと。
// 時計のずれで1件入るだけで、その区分の唯一の担当種目が消える。
func TestAccessorySelector_HandlesFutureLogs(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "future", 20, "calf_raise", 60, 15, 2), // 4日後
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.Calf: 3},
		accessoryPool(t), h, today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("calf_raise") {
		t.Errorf("未来日のログで種目が消えている: %v", got)
	}
}

// 同じ残差・同じ刺激間隔の区分は、名前の昇順で決めること。
// 順序が定まらないと、同じ入力から違うセッションが生成される。
func TestAccessorySelector_TieBreaksRegionsByName(t *testing.T) {
	s := planning.DefaultAccessorySelector()

	// どちらも一度も刺激しておらず、残差も同じ。
	pool := []*exercise.Exercise{
		mkAccessory(t, "for_biceps", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
		mkAccessory(t, "for_calf", map[training.MuscleRegion]float64{training.Calf: 1.0}),
	}
	s2, err := planning.NewAccessorySelector(2, 3, 1) // 1スロットだけ
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}
	_ = s

	got := s2.Select(
		map[training.MuscleRegion]float64{training.Biceps: 3, training.Calf: 3},
		pool, setlog.NewHistory(nil), today(), nil,
	)
	// BICEPS < CALF なので、BICEPS を埋める種目が選ばれる。
	if len(got) != 1 || got[0] != exercise.ExerciseID("for_biceps") {
		t.Errorf("同値時に名前の昇順で決まっていない: %v", got)
	}
}

// 残差がちょうど0の区分にスロットを使わないこと。
// 使うと、埋める必要のない区分のために1種目ぶんのボリュームが積まれる。
func TestAccessorySelector_IgnoresZeroResidual(t *testing.T) {
	s := planning.DefaultAccessorySelector()

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestUpper: 0, training.Biceps: 3},
		accessoryPool(t), setlog.NewHistory(nil), today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("curl") {
		t.Errorf("残差0の区分にスロットを使っている: %v", got)
	}
}

// 区分の優先度は「最後に刺激してからの日数」で決まること。
//
// 両方とも刺激した経験がある場合に、より長く放置している方を先に埋める。
// 日数を見ないと、残差の大きい区分だけが延々と選ばれ続ける。
func TestAccessorySelector_PrefersLeastRecentlyStimulatedRegion(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 1) // 1スロットだけ
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}

	pool := []*exercise.Exercise{
		mkAccessory(t, "for_biceps", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
		mkAccessory(t, "for_calf", map[training.MuscleRegion]float64{training.Calf: 1.0}),
	}
	// 二頭は3日前、カーフは10日前。残差は二頭の方が大きい。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "b", 13, "for_biceps", 20, 10, 2),
		mkLog(t, "c", 6, "for_calf", 40, 15, 2),
	})

	got := s.Select(
		map[training.MuscleRegion]float64{training.Biceps: 9, training.Calf: 3},
		pool, h, today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("for_calf") {
		t.Errorf("最も長く放置している区分が選ばれていない: %v", got)
	}
}

// 宣言した種目も、ヘビー枠でなければ補助として残差を埋める。
//
// 宣言は「伸ばしたい」という目標であって、「ヘビーでしかやらない」では
// ない。除くのは今日のヘビー枠だけ。除きすぎると、脚の日にスクワットが
// どこにも出なくなる。
func TestAccessorySelector_DeclaredExercisesCanStillFillResidual(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	bench := mustExercise(t, benchParams())
	pool := append(accessoryPool(t), bench)

	got := s.Select(
		map[training.MuscleRegion]float64{training.ChestMid: 3},
		pool, setlog.NewHistory(nil), today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("bench") {
		t.Errorf("宣言した種目が補助として残差を埋めていない: %v", got)
	}
}

// 今日メインで処方した種目は、補助にも出さない。
//
// 出すと同じ種目が今日のリストに2回並び、セット数も二重に積まれる。
func TestAccessorySelector_ExcludesTodaysMainLifts(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	pool := append(accessoryPool(t), mustExercise(t, benchParams()))
	residual := map[training.MuscleRegion]float64{training.ChestMid: 3}

	got := s.Select(residual, pool, setlog.NewHistory(nil), today(),
		[]exercise.ExerciseID{"bench"})

	if slices.Contains(got, exercise.ExerciseID("bench")) {
		t.Errorf("メインで処方した種目が補助にも出ている: %v", got)
	}
}

// 除外した種目の履歴も、区分の放置日数の計算には効く。
//
// 除外は候補リストからだけで、履歴のIDから種目を引く辞書（byID）からは
// 抜かない。抜くと「昨日ベンチで大胸筋を刺激した」が見えなくなり、
// 翌日も胸の補助が最優先で選ばれる。
func TestAccessorySelector_ExcludedExerciseStillCountsAsStimulus(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	pool := append(accessoryPool(t), mustExercise(t, benchParams()))

	// 同じ区分を狙う補助を足す。これが無いと大胸筋中部の残差を埋める
	// 手段が存在せず、byID から抜けても結果が変わらない。
	pool = append(pool, mkAccessory(t, "pec_fly",
		map[training.MuscleRegion]float64{training.ChestMid: 1.0}))

	// 昨日ベンチで大胸筋中部を刺激し、二頭は長く放置している。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "b1", today().AddDays(-1), "bench", 80, 8, 2),
	})
	residual := map[training.MuscleRegion]float64{
		training.ChestMid: 3, training.Biceps: 3,
	}

	got := s.Select(residual, pool, h, today(), []exercise.ExerciseID{"bench"})

	// 大胸筋中部は昨日刺激したばかりなので回復中。狙う補助は出ない。
	// byID からベンチを抜くと、この記録が見えなくなって pec_fly が出る。
	if slices.Contains(got, exercise.ExerciseID("pec_fly")) {
		t.Errorf("除外した種目の刺激が回復判定に反映されていない: %v", got)
	}
	if !slices.Contains(got, exercise.ExerciseID("curl")) {
		t.Errorf("放置している区分の補助が選ばれていない: %v", got)
	}
}
