package planning_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
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
	got := sel(t, s,
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
			if got := sel(t, s, c.residual, pool, h, today(), nil); len(got) != c.want {
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
	got := sel(t, s,
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
	if got := sel(t, s, residual, accessoryPool(t), setlog.NewHistory(nil), today(), nil); len(got) > 2 {
		t.Errorf("スロット数の上限が効いていない: %d (%v)", len(got), got)
	}
}

func TestAccessorySelector_SkipsRecentlyStimulatedRegion(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "recent", 15, "incline", 30, 10, 2),
	})

	got := sel(t, s,
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

	got := sel(t, s,
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
			got := sel(t, s, residual, accessoryPool(t), h, today(), nil)

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

	got := sel(t, s,
		map[training.MuscleRegion]float64{training.ChestUpper: 3},
		accessoryPool(t), h, today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("incline_db") {
		t.Errorf("間隔が空いている種目が選ばれていない: %v", got)
	}
}

func TestAccessorySelector_NoDuplicatesInOneSession(t *testing.T) {
	s := planning.DefaultAccessorySelector()
	got := sel(t, s,
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

	if got := sel(t, s, nil, pool, h, today(), nil); len(got) != 0 {
		t.Errorf("残差が無いのに選ばれた: %v", got)
	}
	if got := sel(t, s, residual, nil, h, today(), nil); len(got) != 0 {
		t.Errorf("種目プールが空なのに選ばれた: %v", got)
	}
	if got := sel(t, s, residual, pool, h, training.Date{}, nil); len(got) != 0 {
		t.Errorf("基準日が無いのに選ばれた: %v", got)
	}

	var zero planning.AccessorySelector
	if got := sel(t, zero, residual, pool, h, today(), nil); len(got) != 0 {
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
	got := sel(t, s,
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

	got := sel(t, s,
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

	first := sel(t, s, residual, pool, h, today(), nil)
	for range 50 {
		got := sel(t, s, residual, pool, h, today(), nil)
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

	sel(t, s, residual, accessoryPool(t), setlog.NewHistory(nil), today(), nil)

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
	got := sel(t, s,
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
	got := sel(t, s,
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
	first := sel(t, s, residual, forward, h, today(), nil)

	reversed := make([]*exercise.Exercise, 0, len(forward))
	for i := len(forward) - 1; i >= 0; i-- {
		reversed = append(reversed, forward[i])
	}
	got := sel(t, s, residual, reversed, h, today(), nil)

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

	got := sel(t, s, residual, pool, h, today(), nil)

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

	got := sel(t, s,
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

	calfRaise := mkAccessory(t, "calf_raise", map[training.MuscleRegion]float64{training.Calf: 1.0})

	// 昨日カールをやったので、二頭は回復期間中。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "y", 15, "curl", 20, 10, 2),
	})

	cases := []struct {
		name     string
		residual map[training.MuscleRegion]float64
		want     exercise.ExerciseID
	}{
		{
			// 僧帽筋中部を埋めるためのロウ。二頭への 0.3 は巻き添えなので許す。
			name:     "別の区分を埋める種目が、回復中の区分に副次的に効くのは許す",
			residual: map[training.MuscleRegion]float64{training.TrapMid: 3},
			want:     "row",
		},
		{
			// 許すのは巻き添えまで。回復中の二頭そのものを狙いに行くと、
			// 主働筋の回避をすり抜けるロウ（二頭 0.3）が二頭のために選ばれる。
			// 回復中の区分を残差から外していなければ、カーフレイズの後に
			// ロウが足されて2種目になる。
			name:     "回復中の区分そのものは、副次的に効く種目でも埋めに行かない",
			residual: map[training.MuscleRegion]float64{training.Biceps: 3, training.Calf: 3},
			want:     "calf_raise",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sel(t, s, c.residual, []*exercise.Exercise{curl, row, calfRaise}, h, today(), nil)

			if len(got) != 1 || got[0] != c.want {
				t.Errorf("選ばれたのは %v。%v だけのはず", got, c.want)
			}
		})
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
		got := sel(t, s,
			map[training.MuscleRegion]float64{training.ChestUpper: v},
			pool, h, today(), nil,
		)
		if len(got) != 0 {
			t.Errorf("異常な残差 %v が処理された: %v", v, got)
		}
	}

	// 極端に大きい有限値は「異常」ではないので処理してよいが、
	// スロット数の上限は必ず守ること。
	got := sel(t, s,
		map[training.MuscleRegion]float64{training.ChestUpper: 1e30},
		pool, h, today(), nil,
	)
	if len(got) > s.MaxSlots() {
		t.Errorf("巨大な残差でスロット上限を超えた: %d", len(got))
	}

	// 正常な区分と混ざっていても、正常なぶんだけ処理すること。
	got = sel(t, s,
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

	got := sel(t, s,
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

	got := sel(t, s2,
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

	got := sel(t, s,
		map[training.MuscleRegion]float64{training.ChestUpper: 0, training.Biceps: 3},
		accessoryPool(t), setlog.NewHistory(nil), today(), nil,
	)
	if len(got) != 1 || got[0] != exercise.ExerciseID("curl") {
		t.Errorf("残差0の区分にスロットを使っている: %v", got)
	}
}

// 欠けている割合が同じなら、長く放置している区分を先に埋めること。
//
// 元は「日数が第1キー」を検査していた。日数ではどの区分にも均等に順番が
// 回り、配分表が効かなかったので、第1キーを欠けている割合に替えた
// （TestAccessorySelector_PrefersLargestRelativeShortfall）。日数は同点処理
// として残る。低容量では欠けている割合が1種目ぶんの刻みで揃いやすく、
// 同点は珍しくない。
func TestAccessorySelector_PrefersLeastRecentlyStimulatedRegion(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 1) // 1スロットだけ
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}

	pool := []*exercise.Exercise{
		mkAccessory(t, "for_biceps", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
		mkAccessory(t, "for_calf", map[training.MuscleRegion]float64{training.Calf: 1.0}),
	}
	// どのケースも、欠けている割合は同じ。日数で差をつける。
	residual := map[training.MuscleRegion]float64{training.Biceps: 3, training.Calf: 3}

	cases := []struct {
		name string
		logs []*setlog.SetLog
	}{
		{
			// 二頭は3日前、カーフは10日前。
			name: "長く放置している区分を先に埋める",
			logs: []*setlog.SetLog{
				mkLog(t, "b", 13, "for_biceps", 20, 10, 2),
				mkLog(t, "c", 6, "for_calf", 40, 15, 2),
			},
		},
		{
			// 二頭は15日前と3日前、カーフは10日前。
			// 「最後に刺激してから」なので二頭は3日で、カーフ（10日）が先。
			// 古い方の15日で測ると、3日前にやったばかりの二頭が
			// カーフより放置されていることになって先に選ばれる。
			name: "同じ区分に記録が複数あれば、最も新しい記録から数える",
			logs: []*setlog.SetLog{
				mkLog(t, "b_old", 1, "for_biceps", 20, 10, 2),
				mkLog(t, "b_new", 13, "for_biceps", 20, 10, 2),
				mkLog(t, "c", 6, "for_calf", 40, 15, 2),
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sel(t, s, residual, pool, setlog.NewHistory(c.logs), today(), nil)

			if len(got) != 1 || got[0] != exercise.ExerciseID("for_calf") {
				t.Errorf("最も長く放置している区分が選ばれていない: %v", got)
			}
		})
	}
}

// 渡された種目は1つ残らず候補から外れること。
//
// 元は「宣言した種目も、ヘビー枠でなければ補助として残差を埋める」を
// 検査していた。除くのは今日のヘビー枠だけで、根拠は「除きすぎると脚の日に
// スクワットがどこにも出なくなる」だった。
//
// これは的外れだった。スクワットが軸でない日に脚のボリュームを埋めるのは
// レッグプレスやレッグカールであって、スクワットである必要が無い。宣言は
// 軸レーンで扱うものと割り切ると、レーンの境界がはっきりする。
//
// 除外が1つだけだった頃の ExcludesTodaysMainLifts も、ここに畳んだ。
// 単数のままだと、ヘビー枠以外の宣言がメインと補助の両方に出る。
func TestAccessorySelector_ExcludesEveryListedExercise(t *testing.T) {
	pool := append(accessoryPool(t), mustExercise(t, benchParams()))
	residual := map[training.MuscleRegion]float64{
		training.ChestMid: 3, training.ChestUpper: 3, training.Biceps: 3,
	}

	cases := []struct {
		name    string
		exclude []exercise.ExerciseID
		// 候補に出てはいけない種目。
		absent []exercise.ExerciseID
		// 候補に出るべき種目。除外が広がりすぎていないことを見る。
		present []exercise.ExerciseID
	}{
		{
			name:    "除外が空なら全種目が候補になる",
			exclude: nil,
			present: []exercise.ExerciseID{"bench", "incline"},
		},
		{
			name:    "1つ渡せばそれだけ外れる",
			exclude: []exercise.ExerciseID{"bench"},
			absent:  []exercise.ExerciseID{"bench"},
			present: []exercise.ExerciseID{"incline"},
		},
		{
			name:    "複数渡せばすべて外れる",
			exclude: []exercise.ExerciseID{"bench", "incline"},
			absent:  []exercise.ExerciseID{"bench", "incline"},
			present: []exercise.ExerciseID{"curl"},
		},
		{
			name:    "候補に無い種目を渡しても壊れない",
			exclude: []exercise.ExerciseID{"nonexistent"},
			present: []exercise.ExerciseID{"bench"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := planning.DefaultAccessorySelector()
			got := sel(t, s, residual, pool, setlog.NewHistory(nil), today(), c.exclude)

			for _, id := range c.absent {
				if slices.Contains(got, id) {
					t.Errorf("除外した %s が候補に出ている: %v", id, got)
				}
			}
			for _, id := range c.present {
				if !slices.Contains(got, id) {
					t.Errorf("除外していない %s が候補から消えている: %v", id, got)
				}
			}
		})
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

	got := sel(t, s, residual, pool, h, today(), []exercise.ExerciseID{"bench"})

	// 大胸筋中部は昨日刺激したばかりなので回復中。狙う補助は出ない。
	// byID からベンチを抜くと、この記録が見えなくなって pec_fly が出る。
	if slices.Contains(got, exercise.ExerciseID("pec_fly")) {
		t.Errorf("除外した種目の刺激が回復判定に反映されていない: %v", got)
	}
	if !slices.Contains(got, exercise.ExerciseID("curl")) {
		t.Errorf("放置している区分の補助が選ばれていない: %v", got)
	}
}

// sel は全区分に同じ週目標を置いて Select を呼ぶ。
//
// 目標が全区分で同じなら、相対不足（残り ÷ 目標）の順は残差の絶対値の順と
// 一致する。順序そのものを見ないテストは、これで目標を意識せずに書ける。
// 順序を見るテストは Select を直接呼び、区分ごとに目標を置く。
//
// 40 は週目標の上限。残差がこれを超える区分（異常値の検査）でも目標は
// 組める。
func sel(
	t *testing.T, s planning.AccessorySelector,
	residual map[training.MuscleRegion]float64,
	pool []*exercise.Exercise, h setlog.History, date training.Date,
	exclude []exercise.ExerciseID,
) []exercise.ExerciseID {
	t.Helper()
	m := make(map[training.MuscleRegion]float64, len(residual))
	for r := range residual {
		m[r] = 40
	}
	var target program.WeeklyVolumeTarget
	if len(m) > 0 {
		var err error
		target, err = program.NewWeeklyVolumeTarget(m)
		if err != nil {
			t.Fatalf("週目標が組めない: %v", err)
		}
	}
	return s.Select(target, residual, pool, h, date, exclude)
}

// 区分の優先度は「週目標に対してどれだけ足りていないか」の割合で決まること。
//
// 残差の絶対値で比べると、週目標の大きい区分が常に勝つ。絶対値に直すと
// 大腿四頭筋の2セット不足はカーフの1.6セット不足より大きいが、目標に対しては
// 大腿四頭筋は2割しか欠けておらず、カーフは8割欠けている。配分表の割合が
// 配分を支配するには、この「欠けている割合」で並べる必要がある。
//
// 以前は最後に刺激してからの日数が第1キーだった。どの区分にもほぼ均等に
// 順番が回るので、目標の小さい区分は必ず超過し、大きい区分は必ず不足した
// （1日4種目で達成率 54〜168%、52週平均でも縮まない）。
//
// ここでは日数を揃えて、割合だけが効くようにする。
func TestAccessorySelector_PrefersLargestRelativeShortfall(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 1) // 1スロットだけ
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}
	pool := []*exercise.Exercise{
		mkAccessory(t, "for_calf", map[training.MuscleRegion]float64{training.Calf: 1.0}),
		mkAccessory(t, "for_quad", map[training.MuscleRegion]float64{training.Quad: 1.0}),
	}
	// 両方とも同じ日にやっている。日数では差がつかない。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "c", 6, "for_calf", 40, 15, 2),
		mkLog(t, "q", 6, "for_quad", 60, 10, 2),
	})
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Calf: 2, training.Quad: 10,
	})
	// カーフは目標2に対して1.6不足（8割欠け）、大腿四頭筋は10に対して2不足（2割欠け）。
	residual := map[training.MuscleRegion]float64{training.Calf: 1.6, training.Quad: 2}

	got := s.Select(target, residual, pool, h, today(), nil)
	if len(got) != 1 || got[0] != exercise.ExerciseID("for_calf") {
		t.Errorf("欠けている割合の大きい区分が選ばれていない: %v", got)
	}
}

// 一度も刺激していない区分は、欠けている割合によらず先に埋めること。
//
// 割合で並べても、一度もやっていない区分が後回しになると、分割で狙う日が
// 少ない区分や目標の小さい区分がいつまでも初回を迎えない。最初の1回は
// 割合の比較より先に回す。
func TestAccessorySelector_NeverStimulatedComesFirst(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 1) // 1スロットだけ
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}
	pool := []*exercise.Exercise{
		mkAccessory(t, "shrug", map[training.MuscleRegion]float64{training.TrapUpper: 1.0}),
		mkAccessory(t, "for_quad", map[training.MuscleRegion]float64{training.Quad: 1.0}),
	}
	// 大腿四頭筋は10日前にやった。僧帽筋上部は一度もやっていない。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "q", 6, "for_quad", 60, 10, 2),
	})
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.TrapUpper: 10, training.Quad: 10,
	})
	// 割合では大腿四頭筋が大きい（10割欠け）が、僧帽筋上部は初回。
	residual := map[training.MuscleRegion]float64{training.TrapUpper: 1, training.Quad: 10}

	got := s.Select(target, residual, pool, h, today(), nil)
	if len(got) != 1 || got[0] != exercise.ExerciseID("shrug") {
		t.Errorf("一度も刺激していない区分が先に選ばれていない: %v", got)
	}
}

// 欠けている割合は、最後に刺激してからの日数より優先されること。
//
// TestAccessorySelector_PrefersLargestRelativeShortfall は日数を揃えているので、
// 「割合が絶対値に勝つ」は見ているが「割合が日数に勝つ」は見ていない。
// 順序を日数第1キーに戻しても、あちらは緑のまま通る（変異で確認した）。
// ここでは両者を食い違わせる。
func TestAccessorySelector_ShortfallOutranksStaleness(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 1) // 1スロットだけ
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}
	pool := []*exercise.Exercise{
		mkAccessory(t, "for_calf", map[training.MuscleRegion]float64{training.Calf: 1.0}),
		mkAccessory(t, "for_quad", map[training.MuscleRegion]float64{training.Quad: 1.0}),
	}
	// カーフは3日前（回復期間は明けている）、大腿四頭筋は10日前。
	// 日数で並べれば大腿四頭筋が先。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "c", 13, "for_calf", 40, 15, 2),
		mkLog(t, "q", 6, "for_quad", 60, 10, 2),
	})
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Calf: 10, training.Quad: 10,
	})
	// 割合ではカーフが大きい（窓ぶん40に対して36不足）、大腿四頭筋は小さい（8不足）。
	residual := map[training.MuscleRegion]float64{training.Calf: 36, training.Quad: 8}

	got := s.Select(target, residual, pool, h, today(), nil)
	if len(got) != 1 || got[0] != exercise.ExerciseID("for_calf") {
		t.Errorf("欠けている割合より日数が優先されている: %v", got)
	}
}

// 週目標に無い区分は、残差があっても Select が狙わないこと。
//
// target と residual は Select が受け取る別々の引数であり、両者の対応
// （区分ごとの目標と残差が同じ集合を指すこと）は呼び出し側の責務でしかない。
// いまはプランナーが SessionResidual と Select に同じ週目標を渡しているから
// 一致しているだけで、Select 自身の入力契約として構造的に保証されてはいない。
// trackable の「目標が正でない区分を落とす」ガードを消すと、nextRegion の
// shortfall = remaining[r] / windowSets(target, r) が 0 除算で +Inf になり、
// 目標の無い区分が常に最優先で選ばれる。
func TestAccessorySelector_IgnoresResidualForRegionWithoutTarget(t *testing.T) {
	s, err := planning.NewAccessorySelector(2, 3, 1) // 1スロットだけ
	if err != nil {
		t.Fatalf("NewAccessorySelector: %v", err)
	}
	pool := []*exercise.Exercise{
		mkAccessory(t, "for_calf", map[training.MuscleRegion]float64{training.Calf: 1.0}),
		mkAccessory(t, "for_quad", map[training.MuscleRegion]float64{training.Quad: 1.0}),
	}
	// 両方とも同じ日にやっている。日数でも割合でも差がつかない。差は
	// 0除算だけが作る。
	h := setlog.NewHistory([]*setlog.SetLog{
		mkLog(t, "c", 6, "for_calf", 40, 15, 2),
		mkLog(t, "q", 6, "for_quad", 60, 10, 2),
	})
	// 週目標には大腿四頭筋しかない。カーフは目標が無いのに残差だけが残っている
	// ——target と residual を別々に組んで渡せば起こる形（Select の入力契約の話であり、
	// いまのプランナーの呼び出し方でこれが起きると主張するものではない）。
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Quad: 10})
	residual := map[training.MuscleRegion]float64{training.Calf: 1, training.Quad: 40}

	got := s.Select(target, residual, pool, h, today(), nil)
	if len(got) != 1 || got[0] != exercise.ExerciseID("for_quad") {
		t.Errorf("目標の無い区分が選ばれた（0除算で最優先になった疑い）: %v", got)
	}
}
