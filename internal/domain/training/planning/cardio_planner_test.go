package planning_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// presetCycle は seed.SplitPresets の実物の周期を返す。
//
// 自前で区分を組むと、PPL の「引く」に脊柱起立筋が入っていることを
// 再現できない。脚の日の判定で脊柱起立筋を除くかどうかは、この実物で
// 初めて効く。
func presetCycle(t *testing.T, key string) []program.Split {
	t.Helper()
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("SplitPresets: %v", err)
	}
	for _, p := range presets {
		if p.Key == key {
			return p.Cycle
		}
	}
	t.Fatalf("%s プリセットが見つからない", key)
	return nil
}

// cardioHorizon は ProjectHorizon を実際に通して、PlaceCardio の入力を作る。
//
// attended は今日より前の出席回数。周期の位置は出席回数で進むので、
// これで今日がどの分割の日から始まるかを決める。
func cardioHorizon(
	t *testing.T, freq, attended int, declared []exercise.ExerciseID, cycle []program.Split,
) []planning.ProjectedSession {
	t.Helper()
	pool := []*exercise.Exercise{
		mainExercise(t, "bench", map[training.MuscleRegion]float64{training.ChestMid: 1.0}),
		mainExercise(t, "row", map[training.MuscleRegion]float64{training.Lat: 1.0}),
		mainExercise(t, "squat", map[training.MuscleRegion]float64{training.Quad: 1.0}),
		mainExercise(t, "backext", map[training.MuscleRegion]float64{training.Erector: 1.0}),
	}
	ids := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		ids = append(ids, e.ID())
	}
	prog, err := program.NewProgram(mustFrequency(t, freq), planVolume(t), ids, declared, "")
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	if len(cycle) > 0 {
		if prog, err = prog.WithCycle(cycle); err != nil {
			t.Fatalf("WithCycle: %v", err)
		}
	}

	// 出席は1日1件で数える（SessionCount は日付の数）。
	logs := make([]*setlog.SetLog, 0, attended)
	for i := range attended {
		logs = append(logs, mkLogOn(t, fmt.Sprintf("past-%d", i), planMonday.AddDays(-30+i), "bench", 60, 5, 2))
	}

	got, err := planning.DefaultSessionPlanner().ProjectHorizon(
		setlog.NewHistory(logs), prog, pool, planMonday)
	if err != nil {
		t.Fatalf("ProjectHorizon が失敗: %v", err)
	}
	return got
}

// TestPlaceCardio は有酸素を置く休みの日の選び方を固定する。
//
// 練習の日付は horizonDates の仮置き（今日から round(k × 7 ÷ 頻度) 日後）。
// 期待値はその日付から休みの日を書き出し、
//
//  1. 前日が脚の日でない休み
//  2. 前日が脚の日の休み
//
// の順に、それぞれ早い日から2日取ったもの。日付は返さない（仮置きなので、
// 返すといずれ画面に出る）。早い順は (回, 休みの何日目) の順で表す。
func TestPlaceCardio(t *testing.T) {
	type want struct {
		afterSession int // 直前の練習の回
		restDay      int // その回の後の休みの何日目か
	}
	big := []exercise.ExerciseID{"bench", "row", "squat"}

	cases := []struct {
		name     string
		freq     int
		attended int
		declared []exercise.ExerciseID
		cycle    []program.Split
		want     []want
	}{
		{
			// 練習は0(上)・2(下)・5(上)。休みは1(上の翌日)・3(下の翌日)・4・6。
			// 3を飛ばして1と4を取る。前日を見なければ1と3になる。
			name: "脚の日の翌日を避けて、その次の休みに置く",
			freq: 3, declared: big, cycle: presetCycle(t, "upper_lower"),
			want: []want{{0, 1}, {1, 2}},
		},
		{
			// 練習は0(押す)・2(引く)・4(脚)・5(押す)。休みは1・3(引くの翌日)・6。
			// 「引く」は脊柱起立筋を含むが脚の日ではないので、3は最優先のまま。
			// LowerBody() で判定すると3が後回しになり、1と6になる。
			name: "脊柱起立筋だけの日は脚の日にしない",
			freq: 4, declared: big, cycle: presetCycle(t, "ppl"),
			want: []want{{0, 1}, {1, 1}},
		},
		{
			// 出席1回ぶん周期が進み、引く(0)・脚(2)・押す(4)・引く(5)。
			// 休みは1・3(脚の翌日)・6。6は最後の回の後で、来週の今日の前日まで。
			// 最後の回の後を数えないと、3しか残らず脚の翌日に置いてしまう。
			name: "最後の回の後の休みも候補に入れる",
			freq: 4, attended: 1, declared: big, cycle: presetCycle(t, "ppl"),
			want: []want{{0, 1}, {3, 1}},
		},
		{
			// 練習は0・1・2(脚)・4・5・6(脚)。休みは脚の翌日の3だけ。
			// 最優先が無くても、休みがあれば置く。休みの数より多くは置かない。
			name: "休みが脚の翌日しか無ければ、そこに置く",
			freq: 6, declared: big, cycle: presetCycle(t, "ppl"),
			want: []want{{2, 1}},
		},
		{
			// 出席1回ぶん周期が進み、引く(0)・脚(1)・押す(3)・引く(4)・脚(6)。
			// 休みは2(脚の翌日)・5。優先度では5が先に取られるが、返すのは早い順。
			name: "後回しにした休みが先の日付でも、早い順に返す",
			freq: 5, attended: 1, declared: big, cycle: presetCycle(t, "ppl"),
			want: []want{{1, 1}, {3, 1}},
		},
		{
			name: "毎日練習するなら置かない",
			freq: 7, declared: big, cycle: presetCycle(t, "ppl"),
			want: nil,
		},
		{
			// 分割なし。宣言がスクワットだけなので軸は毎回スクワット。
			// 練習は0・4。休みは1(脚の翌日)・2・3・5(脚の翌日)・6。
			name: "分割なしの日は、軸が脚の種目なら脚の日",
			freq: 2, declared: []exercise.ExerciseID{"squat"},
			want: []want{{0, 2}, {0, 3}},
		},
		{
			// 分割なしでも、軸が上半身なら翌日に置ける。
			// 全区分の日を一律に脚の日にすると、上と同じ 2・3 になる。
			name: "分割なしの日は、軸が上半身なら脚の日ではない",
			freq: 2, declared: []exercise.ExerciseID{"bench"},
			want: []want{{0, 1}, {0, 2}},
		},
		{
			// 軸の判定も分割と同じく脊柱起立筋を除く。
			name: "分割なしの日に、軸が脊柱起立筋だけの種目でも脚の日ではない",
			freq: 2, declared: []exercise.ExerciseID{"backext"},
			want: []want{{0, 1}, {0, 2}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := planning.PlaceCardio(cardioHorizon(t, c.freq, c.attended, c.declared, c.cycle))

			if len(got) != len(c.want) {
				t.Fatalf("有酸素の日が %d 日。%d 日のはず: %+v", len(got), len(c.want), got)
			}
			for i, w := range c.want {
				g := got[i]
				if g.AfterSession() != w.afterSession || g.RestDay() != w.restDay {
					t.Errorf("%d件目が %d回目の後の休み%d日目。%d回目の後の%d日目のはず",
						i, g.AfterSession(), g.RestDay(), w.afterSession, w.restDay)
				}
			}
		})
	}
}

// 有酸素の日は、当日の記録で動かない。
//
// 周期は出席回数で進むので、当日を1回と数えると今日の分割が1つずれ、
// 脚の日の翌日がどこになるかも変わる。今日の計画と同じく、前日までの
// 履歴から決める（D-116。当日を落とすのは planWeek の1箇所だけ）。
//
// fixedDaySplitRequest は週3回・上下の周期。練習は0・2・5日後。
// 出席0回なら 上・下・上 で、有酸素は (0,1)・(1,2)。
// 当日を数えると 下・上・下 になり、(1,1)・(1,2) に変わる。
func TestSessionPlanner_ForecastWeek_CardioIsFixedForTheWholeDay(t *testing.T) {
	req := fixedDaySplitRequest(t)
	planner := planning.DefaultSessionPlanner()

	first, err := planner.ForecastWeek(req)
	if err != nil {
		t.Fatalf("ForecastWeek が失敗: %v", err)
	}
	want := [][2]int{{0, 1}, {1, 2}}
	if got := cardioPositions(first.Cardio()); !slices.Equal(got, want) {
		t.Fatalf("前提: 記録の無い朝の有酸素の日が %v。%v のはず", got, want)
	}

	req.History = setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "today", req.Date, "bench", 60, 8, 2),
	})
	after, err := planner.ForecastWeek(req)
	if err != nil {
		t.Fatalf("ForecastWeek が失敗: %v", err)
	}
	if got := cardioPositions(after.Cardio()); !slices.Equal(got, want) {
		t.Errorf("今日1セット記録したら有酸素の日が %v に動いた。%v のまま", got, want)
	}
}

// ForecastWeek の予定は Forecast と同じ。Forecast はこれの予定だけを返す。
//
// 別々に組み立てると、片方だけ直したときに予定と有酸素の日がずれる。
func TestSessionPlanner_ForecastWeek_SessionsMatchForecast(t *testing.T) {
	req := fixedDaySplitRequest(t)
	planner := planning.DefaultSessionPlanner()

	week, err := planner.ForecastWeek(req)
	if err != nil {
		t.Fatalf("ForecastWeek が失敗: %v", err)
	}
	sessions, err := planner.Forecast(req)
	if err != nil {
		t.Fatalf("Forecast が失敗: %v", err)
	}
	if len(week.Sessions()) != len(sessions) {
		t.Fatalf("回数が %d。Forecast は %d", len(week.Sessions()), len(sessions))
	}
	for k := range sessions {
		if diff := planDiff(sessions[k], week.Sessions()[k]); len(diff) > 0 {
			t.Errorf("回%d が Forecast と違う\n  %s", k, strings.Join(diff, "\n  "))
		}
	}
}

func cardioPositions(days []planning.CardioDay) [][2]int {
	out := make([][2]int, 0, len(days))
	for _, d := range days {
		out = append(out, [2]int{d.AfterSession(), d.RestDay()})
	}
	return out
}
