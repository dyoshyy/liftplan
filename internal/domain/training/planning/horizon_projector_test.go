package planning_test

import (
	"fmt"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// horizonFixture は1ケースぶんの入力。分割・重点種目の有無で軸・
// バリエーションの決まり方が変わるので、ケースごとに別々に組み立てる。
//
// history は今日より前の実際の記録（あれば）。空だと ProjectHorizon が
// 受け取った history を無視しても（回0は常に周期の先頭・宣言の先頭になる
// だけなので）気づけない。少なくとも一部のケースは空でない履歴で
// 「渡された履歴の続きから予測する」ことを検査する。
type horizonFixture struct {
	name    string
	prog    *program.Program
	pool    []*exercise.Exercise
	history []*setlog.SetLog
}

// pplFixture は seed.SplitPresets の実物の "ppl" 周期を使う。
//
// 自前で区分を組むと、実シードとの食い違いに気づけない。頻度6にして
// 周期（3日）を2周させ、同じ日区分が週内に2回来る場合も検査に含める。
func pplFixture(t *testing.T) horizonFixture {
	t.Helper()
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("SplitPresets: %v", err)
	}
	var cycle []program.Split
	for _, p := range presets {
		if p.Key == "ppl" {
			cycle = p.Cycle
		}
	}
	if cycle == nil {
		t.Fatal("ppl プリセットが見つからない")
	}

	pool := []*exercise.Exercise{
		mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mainExercise(t, "row", map[training.MuscleRegion]float64{training.Lat: 1.0}),
		mainExercise(t, "squat", map[training.MuscleRegion]float64{training.Quad: 1.0}),
		mkAccessory(t, "fly", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
		mkAccessory(t, "legcurl", map[training.MuscleRegion]float64{training.Hamstring: 1.0}),
	}
	ids := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		ids = append(ids, e.ID())
	}

	prog, err := program.NewProgram(mustFrequency(t, 6), planVolume(t),
		mustTarget(t, map[training.MuscleRegion]float64{
			training.ChestMid: 12, training.Lat: 12, training.Quad: 12, training.Biceps: 6, training.Hamstring: 6,
		}),
		ids, []exercise.ExerciseID{"bench", "row", "squat"}, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	prog, err = prog.WithCycle(cycle)
	if err != nil {
		t.Fatalf("WithCycle: %v", err)
	}
	return horizonFixture{name: "ppl", prog: prog, pool: pool}
}

// rotationFixture は分割なし・重点種目つき。頻度7で1週間ぶん予測すると、
// 重点種目の一巡（3レップ相当 → 6レップ相当 → 派生）をちょうど3回踏み、
// かつ他の宣言種目が軸の日にバリエーション（派生）が出る条件も踏む。
func rotationFixture(t *testing.T) horizonFixture {
	t.Helper()
	derived := func(id string) *exercise.Exercise {
		return mustExercise(t, exercise.ExerciseParams{
			ID: id, Name: id,
			Stimulus:    map[training.MuscleRegion]float64{training.ChestMid: 1.0},
			IncrementKg: 2.5,
			DerivedFrom: "bench",
		})
	}
	pool := []*exercise.Exercise{
		mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mainExercise(t, "squat", map[training.MuscleRegion]float64{training.Quad: 1.0}),
		mainExercise(t, "deadlift", map[training.MuscleRegion]float64{training.Hamstring: 1.0}),
		derived("larsen"),
		derived("tempo"),
		mkAccessory(t, "curl", map[training.MuscleRegion]float64{training.Biceps: 1.0}),
	}
	prog, err := program.NewProgram(mustFrequency(t, 7), planVolume(t),
		mustTarget(t, map[training.MuscleRegion]float64{
			training.ChestMid: 21, training.Quad: 21, training.Hamstring: 21, training.Biceps: 7,
		}),
		[]exercise.ExerciseID{"bench", "squat", "deadlift", "larsen", "tempo", "curl"},
		big3(), "bench")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	return horizonFixture{name: "rotation", prog: prog, pool: pool}
}

func horizonFixtures(t *testing.T) []horizonFixture {
	t.Helper()
	return []horizonFixture{
		// 3週ぶんの実績つき。ProjectHorizon が受け取った history をそのまま
		// 使わず、空の履歴として扱っても（軸は毎回宣言の先頭に戻るだけで）
		// 気づけないケースが混ざらないようにする。
		{name: "no-split", prog: planProgram(t), pool: planPool(t), history: planHistory(t)},
		// 出席1回ぶんの実績つき。周期の先頭（上）から1つ進めた「下」が
		// 回0の分割になる。履歴を無視すると回0が「上」のまま出て気づける。
		{
			name: "upper-lower",
			prog: splitProgram(t, mkSplit(t, "上", training.ChestMid), mkSplit(t, "下", training.Quad)),
			pool: splitPool(t),
			history: []*setlog.SetLog{
				mkLogOn(t, "upper-lower-seed", planMonday.AddDays(-2), "bench", 60, 8, 2),
			},
		},
		pplFixture(t),
		rotationFixture(t),
	}
}

// TestProjectHorizon_MatchesPlanWhenFollowedExactly は設計書の受け入れ条件。
//
// 模擬の利用者が予測どおりの日に、予測どおりの軸・バリエーション・補助を
// こなしたとき、回 k について予測した（分割の日、軸ID、軸のセット数、
// バリエーションID、そのセット数）は、実際にその日を迎えて Plan を呼んだ
// 結果と一致する。ずれると、次のPR（割り振り器）が架空の週を根拠に
// 損失を計算することになる。
//
// laneRole（heavy/focusVolume）は Plan の出力（Main/Variation）に現れない
// ので比較しない。役割の効果はレーンとセット数に既に現れている。
//
// 分割の日の期待値は、予測(sess.Split())とは独立に、実際に積み上がった
// 履歴(before)から prog.SplitOn で出す。ProjectHorizon が受け取った
// history を無視して空から予測しても（"upper-lower" フィクスチャの
// history が効かなければ）ここで気づける。
func TestProjectHorizon_MatchesPlanWhenFollowedExactly(t *testing.T) {
	planner := planning.DefaultSessionPlanner()

	for _, fx := range horizonFixtures(t) {
		t.Run(fx.name, func(t *testing.T) {
			date := planMonday
			simulated := append([]*setlog.SetLog(nil), fx.history...)

			got, err := planner.ProjectHorizon(setlog.NewHistory(simulated), fx.prog, fx.pool, date)
			if err != nil {
				t.Fatalf("ProjectHorizon が失敗: %v", err)
			}
			f := fx.prog.Frequency().PerWeek()
			if len(got) != f {
				t.Fatalf("予測した回数が %d。頻度 %d のはず", len(got), f)
			}

			sawVariation := false
			axisIDs := map[exercise.ExerciseID]bool{}

			for k, sess := range got {
				before := setlog.NewHistory(simulated)

				wantSplit, wantHasSplit := fx.prog.SplitOn(before.SessionCount())
				gotSplit, gotHasSplit := sess.Split()
				if gotHasSplit != wantHasSplit || gotSplit.Name() != wantSplit.Name() {
					t.Fatalf("回%d: 分割の日が %v(%v)。%v(%v) のはず",
						k, gotSplit.Name(), gotHasSplit, wantSplit.Name(), wantHasSplit)
				}

				actual, err := planner.Plan(planning.PlanRequest{
					Program: fx.prog, Pool: fx.pool, History: before,
					Conditions: condition.NewConditionLog(nil), Date: sess.Date(),
				})
				if err != nil {
					t.Fatalf("回%d: Plan が失敗: %v", k, err)
				}

				axisEx, axisSets, axisOK := sess.Axis()
				switch {
				case axisOK && (len(actual.Main()) != 1 || actual.Main()[0].ExerciseID() != axisEx.ID()):
					t.Errorf("回%d: 予測した軸が %v。実際は %v", k, axisEx.ID(), actual.Main())
				case !axisOK && len(actual.Main()) != 0:
					t.Errorf("回%d: 軸は無い予測だが、実際は %v", k, actual.Main())
				case axisOK && actual.Main()[0].Sets() != axisSets:
					t.Errorf("回%d: 軸のセット数が %v。%v のはず", k, actual.Main()[0].Sets(), axisSets)
				}
				if axisOK {
					axisIDs[axisEx.ID()] = true
				}

				varEx, varSets, varOK := sess.Variation()
				switch {
				case varOK && (len(actual.Variation()) != 1 || actual.Variation()[0].ExerciseID() != varEx.ID()):
					t.Errorf("回%d: 予測したバリエーションが %v。実際は %v", k, varEx.ID(), actual.Variation())
				case !varOK && len(actual.Variation()) != 0:
					t.Errorf("回%d: バリエーションは無い予測だが、実際は %v", k, actual.Variation())
				case varOK && actual.Variation()[0].Sets() != varSets:
					t.Errorf("回%d: バリエーションのセット数が %v。%v のはず", k, actual.Variation()[0].Sets(), varSets)
				}
				if varOK {
					sawVariation = true
				}

				// 模擬の利用者：計画どおりに軸・バリエーション・補助を
				// その日にこなす。重量・レップ・RIRの値は軸・バリエーション・
				// 分割の判定に使われないので、記録として成立する適当な値でよい。
				n := 0
				for _, set := range append(append(actual.Main(), actual.Variation()...), actual.Accessories()...) {
					simulated = append(simulated, mkLogOn(t, fmt.Sprintf("%s-k%d-%d", fx.name, k, n),
						sess.Date(), string(set.ExerciseID()), 40, 8, 2))
					n++
				}
			}

			// フィクスチャが意図した経路を本当に踏んだことの確認。踏んで
			// いなければ、この節が何も守らずに緑になる。
			if fx.name == "rotation" {
				if !sawVariation {
					t.Error("このフィクスチャはバリエーションが出る回を含むはずが、1回も出なかった")
				}
				if len(axisIDs) < 2 {
					t.Errorf("軸が回っていない（宣言種目間の一巡を検査できていない）: %v", axisIDs)
				}
			}
		})
	}
}

// TestHorizonDates_SpacingByFrequency は日付の等間隔・丸めの契約を固定する。
//
// horizonDates は非公開なので、ProjectHorizon(...).Date() 経由で外側から
// 検査する。期待値は「今日からの経過日数 = round(k × 7 ÷ 頻度)」を手計算
// したもの（0.5はゼロから遠いほうへ丸める。例：頻度2のk=1は 7/2=3.5→4）。
func TestHorizonDates_SpacingByFrequency(t *testing.T) {
	cases := []struct {
		freq        int
		wantOffsets []int // 今日からの経過日数、k=0..freq-1
	}{
		{1, []int{0}},                   // 7/1=7 だが k=0 だけなので回数分しか出ない
		{2, []int{0, 4}},                // k=1: 7/2=3.5 → 4
		{3, []int{0, 2, 5}},             // k=1: 7/3≈2.33→2, k=2: 14/3≈4.67→5
		{4, []int{0, 2, 4, 5}},          // k=2: 14/4=3.5→4, k=3: 21/4=5.25→5
		{5, []int{0, 1, 3, 4, 6}},       // k=2: 14/5=2.8→3, k=4: 28/5=5.6→6
		{6, []int{0, 1, 2, 4, 5, 6}},    // k=3: 21/6=3.5→4
		{7, []int{0, 1, 2, 3, 4, 5, 6}}, // 割り切れるので毎日
	}

	pool := []*exercise.Exercise{mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0})}

	for _, c := range cases {
		t.Run(fmt.Sprintf("週%d回", c.freq), func(t *testing.T) {
			target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestMid: 10})
			prog, err := program.NewProgram(mustFrequency(t, c.freq), planVolume(t), target,
				[]exercise.ExerciseID{"bench"}, []exercise.ExerciseID{"bench"}, "")
			if err != nil {
				t.Fatalf("プログラムの生成に失敗: %v", err)
			}

			got, err := planning.DefaultSessionPlanner().ProjectHorizon(
				setlog.NewHistory(nil), prog, pool, planMonday)
			if err != nil {
				t.Fatalf("ProjectHorizon が失敗: %v", err)
			}
			if len(got) != len(c.wantOffsets) {
				t.Fatalf("回数が %d。%d のはず", len(got), len(c.wantOffsets))
			}

			seen := map[training.Date]bool{}
			for k, want := range c.wantOffsets {
				wantDate := planMonday.AddDays(want)
				if got[k].Date() != wantDate {
					t.Errorf("k=%d: 日付が %v。%v のはず", k, got[k].Date(), wantDate)
				}
				if seen[got[k].Date()] {
					t.Errorf("k=%d: 日付 %v が別の回と重複している", k, got[k].Date())
				}
				seen[got[k].Date()] = true
			}
		})
	}
}
