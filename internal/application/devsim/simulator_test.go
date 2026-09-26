package devsim_test

import (
	"fmt"
	"math"
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
		Athlete:   devsim.DefaultAthlete(),
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

// 模擬ユーザーは、その日の実力どおりに記録する。
//
// Epley で逆算したレップ数を記録するので、記録から推定した1RMはその日の
// 実力と一致する（レップ数が整数に丸まるぶん、0.5レップ以内でずれる）。
// 以前は重さに関係なく8レップを記録していて、軸（3レップ相当）の日に
// 実力の14%増しの記録が毎回付き、推定が1回ごとに約4%伸び続けていた。
//
// 自重種目は体重込みの負荷で見る。記録は加重だけなので、体重を足さずに
// 逆算するとチンニングが自重で何十回も挙がることになる。
//
// 範囲の外は2つあり、どちらも「実力以上の記録を付けない」ことだけは守る。
//   - 実力が落ちていく人には、推定が追いつかず重すぎる処方が出る。目標 RIR を
//     残せないなら RIR0 で挙がるだけ挙げる
//   - 軽すぎる重さは、限界までを20回で頭打ちにする（推定器が捨てる範囲）
func TestSimulator_RecordsMatchTheAthletesStrength(t *testing.T) {
	cases := []struct {
		name     string
		growth   float64
		firstPct float64
	}{
		{"実力が一定", 0, 70},
		{"実力が伸びる", 1.5, 70},
		// 推定が遅れて、処方が実力を超える回が出る。
		{"実力が落ちる", -8, 70},
		// 初回が実力の2割。限界まで120回ぶんの軽さ。
		{"初回が軽すぎる", 0, 20},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := baseRequest()
			req.Weeks = 8
			req.Athlete.GrowthPctPerWeek = c.growth
			req.Athlete.FirstSessionPct = c.firstPct

			s := newSimulator(t)
			got, err := s.Run(req)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			byID := map[exercise.ExerciseID]*exercise.Exercise{}
			for _, e := range s.Pool() {
				byID[e.ID()] = e
			}

			exact, failed, capped := 0, 0, 0
			for _, d := range got.Days {
				weeks := float64(d.Date.DaysSince(req.Start)) / 7
				for _, set := range allSets(d) {
					e := byID[set.ExerciseID]
					bw := e.BodyweightFactor().Float() * req.Athlete.BodyWeightKg
					strength := (req.Athlete.OneRepMax(set.ExerciseID) + bw) *
						math.Pow(1+c.growth/100, weeks)
					load := set.Performed.WeightKg + bw
					toFailure := set.Performed.Reps + set.Performed.RIR
					claimed := load * (1 + float64(toFailure)/30)
					where := func() string {
						return fmt.Sprintf("%v %s: %.1fkg×%d RIR%d（実力 %.1fkg）", d.Date, set.ExerciseID,
							set.Performed.WeightKg, set.Performed.Reps, set.Performed.RIR, strength)
					}

					if diff := math.Abs(set.AthleteOneRepMaxKg - (strength - bw)); diff > 1e-9 {
						t.Errorf("%s: 返した実力が %.2fkg。%.2fkg のはず", where(), set.AthleteOneRepMaxKg, strength-bw)
					}
					if set.Performed.Reps < 1 {
						t.Fatalf("%s: 1レップ以上を記録するはず", where())
					}
					if toFailure > 20 {
						t.Fatalf("%s: 限界まで %d 回。20回で頭打ちのはず", where(), toFailure)
					}

					switch {
					case toFailure == 20:
						// 頭打ち。実力より低く出るのは仕様。
						capped++
						if claimed > strength+load/60+1e-9 {
							t.Errorf("%s: 頭打ちなのに実力を超える", where())
						}
					case set.Performed.RIR == 0 && set.Performed.Reps == 1:
						// 挙がらない重さ。1回とする以上、実力は超えうる。
						failed++
					default:
						if set.Performed.RIR == 0 && set.TargetRIR > 0 {
							failed++ // 目標 RIR を残せず、挙がるだけ挙げた
						}
						// round で選んだ整数なので、半レップぶん（load/60）以内。
						if diff := math.Abs(claimed - strength); diff > load/60+1e-9 {
							t.Errorf("%s: 記録が実力から %.2fkg ずれる", where(), diff)
						}
						exact++
					}
				}
			}
			if exact == 0 {
				t.Fatal("検査できた記録が1件も無い")
			}
			// 端の場合を本当に踏んでいるか。踏んでいなければ、この行は何も守らない。
			if c.growth < 0 && failed == 0 {
				t.Error("実力が落ちるのに、目標 RIR を残せなかった回が1つも無い")
			}
			if c.firstPct < 30 && capped == 0 {
				t.Error("初回が軽すぎるのに、20回の頭打ちを1度も踏まない")
			}
		})
	}
}

// 初回は実力の決まった割合の重さを本人が選ぶ。処方が無い日の記録がこれ。
func TestSimulator_FirstSessionUsesTheChosenShareOfStrength(t *testing.T) {
	req := baseRequest()
	req.Athlete.FirstSessionPct = 60
	// 既定値（100kg）と違う値にする。同じだと、上書きを無視しても通る。
	req.Athlete.OneRepMaxKg = map[exercise.ExerciseID]float64{"bench": 150}

	for _, d := range mustRun(t, req).Days {
		for _, set := range allSets(d) {
			if set.ExerciseID != "bench" {
				continue
			}
			if set.HasWeight {
				t.Fatalf("最初のベンチに処方の重量がある（%vkg）。履歴が無いはず", set.WeightKg)
			}
			if set.Performed.WeightKg != 90 {
				t.Errorf("初回の記録が %vkg。実力150kgの60%%で 90kg のはず", set.Performed.WeightKg)
			}
			return
		}
	}
	t.Fatal("ベンチが1度も出ない")
}

// 実力が変わらなければ、軸の重量は実力の0.88倍の近くで頭打ちになる。
//
// 伸びるのは上乗せ（刻み1つ）の分だけ。記録が実力どおりなら推定は実力に
// 収束するので、それより上に積み上がり続けるなら、記録が実力を超えている。
func TestSimulator_ConstantStrengthPlateaus(t *testing.T) {
	req := baseRequest()
	req.Weeks = 12
	req.Athlete.GrowthPctPerWeek = 0
	req.Athlete.OneRepMaxKg = map[exercise.ExerciseID]float64{"bench": 100}

	top := topAxisWeight(mustRun(t, req), "bench")
	// 0.88 × 100 を 2.5kg に丸めて 87.5、上乗せで +2.5。
	if top > 90 {
		t.Errorf("実力100kgのベンチが %vkg まで処方された。90kg で頭打ちのはず", top)
	}
	if top < 85 {
		t.Errorf("軸の重量が %vkg にしか届かない。推定が実力に追いついていない", top)
	}
}

// 実力が伸びれば、処方もそれを追って伸びる。
func TestSimulator_GrowthRaisesThePrescription(t *testing.T) {
	flat := baseRequest()
	flat.Weeks = 12
	flat.Athlete.GrowthPctPerWeek = 0

	growing := flat
	growing.Athlete.GrowthPctPerWeek = 1.5

	a := topAxisWeight(mustRun(t, flat), "bench")
	b := topAxisWeight(mustRun(t, growing), "bench")
	// 12週で実力は約1.2倍。処方の頭打ちより十分上に出る。
	if b < a+10 {
		t.Errorf("伸び 1.5%%/週で %vkg、伸びなしで %vkg。伸ばした実力を処方が追っていない", b, a)
	}
}

// 模擬ユーザーの設定が成り立たないときはエラーにする。画面に 400 を返すため。
func TestSimulator_RejectsBadAthlete(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*devsim.AthleteParams)
	}{
		{"伸び率が大きすぎる", func(a *devsim.AthleteParams) { a.GrowthPctPerWeek = 20 }},
		{"伸び率が小さすぎる", func(a *devsim.AthleteParams) { a.GrowthPctPerWeek = -20 }},
		{"初回の割合が0", func(a *devsim.AthleteParams) { a.FirstSessionPct = 0 }},
		{"初回の割合が100超", func(a *devsim.AthleteParams) { a.FirstSessionPct = 120 }},
		{"体重が0", func(a *devsim.AthleteParams) { a.BodyWeightKg = 0 }},
		{"1RMが負", func(a *devsim.AthleteParams) {
			a.OneRepMaxKg = map[exercise.ExerciseID]float64{"bench": -1}
		}},
		{"自重でない種目の1RMが0", func(a *devsim.AthleteParams) {
			a.OneRepMaxKg = map[exercise.ExerciseID]float64{"bench": 0}
		}},
		{"知らない種目の1RM", func(a *devsim.AthleteParams) {
			a.OneRepMaxKg = map[exercise.ExerciseID]float64{"nope": 100}
		}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := baseRequest()
			c.mutate(&req.Athlete)
			if _, err := newSimulator(t).Run(req); err == nil {
				t.Error("エラーにならない")
			}
		})
	}
}

// 自重種目は加重0（自重だけで1回）を実力として受け付ける。
func TestSimulator_AcceptsBodyweightOnlyStrength(t *testing.T) {
	req := baseRequest()
	req.Athlete.OneRepMaxKg = map[exercise.ExerciseID]float64{"pull_up": 0}
	if _, err := newSimulator(t).Run(req); err != nil {
		t.Errorf("エラーになった: %v", err)
	}
}

func allSets(d devsim.Day) []devsim.Set {
	out := append([]devsim.Set{}, d.Main...)
	out = append(out, d.Variation...)
	return append(out, d.Accessories...)
}

// topAxisWeight は軸レーンで処方された、その種目の最も重い重量。
func topAxisWeight(r devsim.Result, id exercise.ExerciseID) float64 {
	top := 0.0
	for _, d := range r.Days {
		for _, m := range d.Main {
			if m.ExerciseID == id && m.HasWeight && m.WeightKg > top {
				top = m.WeightKg
			}
		}
	}
	return top
}

// 推定比（処方 ÷ 推定1RM）は、自重種目でも体重込みの負荷で出す。
//
// プランナーは体重込みで推定して処方し、出口で加重に戻す。比を加重だけで
// 出すと、バックエクステンション（加重 1.25kg）で 1.41 のような意味の無い
// 値になる。体重込みで見れば、どの種目も役割の強度（補助 0.71・軸 0.81〜
// 0.88）の近くに並ぶ。加重0に倒した回（自重だけで強度を超える）は除く。
func TestSimulator_PctOfOneRMUsesEffectiveLoad(t *testing.T) {
	// 画面で 1.41 が出た設定。自重種目に加重が付く回を踏む。
	req := baseRequest()
	req.SplitKey = "upper_lower"
	req.Focus = "bench"
	req.Weeks = 12

	checked := map[exercise.ExerciseID]bool{}
	for _, d := range mustRun(t, req).Days {
		lanes := []struct {
			sets     []devsim.Set
			lo, hi   float64
			laneName string
		}{
			{d.Main, 0.76, 0.95, "軸"},
			{d.Accessories, 0.64, 0.80, "補助"},
		}
		for _, l := range lanes {
			for _, s := range l.sets {
				if s.PctOfOneRM == 0 || s.WeightKg == 0 {
					continue
				}
				if s.PctOfOneRM < l.lo || s.PctOfOneRM > l.hi {
					t.Errorf("%v %s（%s）: 推定比 %.2f。%.2f〜%.2f のはず",
						d.Date, s.ExerciseID, l.laneName, s.PctOfOneRM, l.lo, l.hi)
				}
				checked[s.ExerciseID] = true
			}
		}
	}
	// 自重種目を1つも踏んでいなければ、この検査は何も守らない。
	if !checked["back_extension"] && !checked["dip"] && !checked["pull_up"] {
		t.Fatal("自重種目の推定比を1つも検査していない")
	}
}
