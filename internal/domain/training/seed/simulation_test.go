package seed_test

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"

	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// このファイルはシードが実運用で破綻しないことを検証する通し検証。
//
// シードは「値が入っていること」を確かめても意味がない。週目標と刺激係数は
// セッション生成器を通してはじめて挙動になるので、実際に何週間か回して
// 達成率と種目の出番を見る。

var simStart = training.MustDate(2026, time.August, 3) // 月曜

// maxSimFrequency は通し検証で回す頻度の上限。program.NewFrequency の
// 上限と揃える。揃っていないと、上限を上げたのに検証されない頻度が残る。
const maxSimFrequency = 7

// 通し検証の上限が実装の上限と一致すること。
//
// ずれていても他のテストは緑のまま通るので、上限を上げたのに検証されない
// 頻度が残る。上げ忘れではなく「上げたことに気づかない」ほうが危ない。
func TestSimulation_CoversEveryAllowedFrequency(t *testing.T) {
	if _, err := program.NewFrequency(maxSimFrequency); err != nil {
		t.Errorf("通し検証の上限 %d が実装で弾かれる: %v", maxSimFrequency, err)
	}
	if _, err := program.NewFrequency(maxSimFrequency + 1); err == nil {
		t.Errorf("週%d回が通る。通し検証されない頻度が残っている", maxSimFrequency+1)
	}
	for f := 1; f <= maxSimFrequency; f++ {
		if len(weekdays[f]) != f {
			t.Errorf("週%d回の曜日が %d 個。%d 個のはず", f, len(weekdays[f]), f)
		}
	}
}

// weekdays は頻度ごとの曜日オフセット（月曜=0）。
// 実在しうるスケジュールに合わせる。
var weekdays = map[int][]int{
	1: {0},
	2: {0, 3},
	3: {0, 2, 4},
	4: {0, 2, 4, 6},
	5: {0, 1, 2, 4, 5},
	6: {0, 1, 2, 3, 4, 5},
	7: {0, 1, 2, 3, 4, 5, 6},
}

// trueOneRepMax はシミュレーション上の本人の実力。期間中は一定とする。
// 週目標が届くかどうかは重量ではなくセット数の話なので、伸びは考えない。
func trueOneRepMax(id exercise.ExerciseID) float64 {
	switch id {
	case "squat":
		return 140
	case "bench":
		return 100
	case "deadlift":
		return 180
	}
	return 50
}

type simResult struct {
	achieved  map[training.MuscleRegion]float64 // 週あたりの平均刺激量
	target    program.WeeklyVolumeTarget
	setsPer   []int                       // セッションごとの総セット数
	picked    map[exercise.ExerciseID]int // 種目ごとの選出回数
	weeks     int
	undecided int // 重量が未確定のまま提示された延べ件数
}

func (r simResult) rate(region training.MuscleRegion) float64 {
	t := r.target.Sets(region)
	if t <= 0 {
		return 0
	}
	return r.achieved[region] / t
}

// simulate は frequency 回/週で weeks 週ぶん、処方どおりに実施した場合を回す。
func simulate(t *testing.T, frequency, weeks int) simResult {
	t.Helper()
	return simulateWithout(t, frequency, weeks)
}

// simulateFocused は重点種目を指定して回す。
func simulateFocused(t *testing.T, frequency, weeks int, focus exercise.ExerciseID) simResult {
	t.Helper()
	return simulateWith(t, frequency, weeks, focus)
}

// simulateWithout は指定した種目をプログラムから外して回す。
func simulateWithout(t *testing.T, frequency, weeks int, excluded ...exercise.ExerciseID) simResult {
	t.Helper()
	return simulateWith(t, frequency, weeks, "", excluded...)
}

// simulateWith は重点種目と除外種目を指定して回す。
func simulateWith(t *testing.T, frequency, weeks int, focus exercise.ExerciseID, excluded ...exercise.ExerciseID) simResult {
	t.Helper()

	skip := make(map[exercise.ExerciseID]bool, len(excluded))
	for _, id := range excluded {
		skip[id] = true
	}

	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, err := program.NewFrequency(frequency)
	if err != nil {
		t.Fatalf("頻度が不正: %v", err)
	}
	target, err := seed.DefaultWeeklyTarget(freq, simVolume(t))
	if err != nil {
		t.Fatalf("週目標が不正: %v", err)
	}

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(all))
	ids := make([]exercise.ExerciseID, 0, len(all))
	for _, e := range all {
		byID[e.ID()] = e
		if !skip[e.ID()] {
			ids = append(ids, e.ID())
		}
	}
	// 伸ばしたい種目は、選択に残っている BIG3 だけにする。
	// 構成によってはスクワットを外すので、declared ⊂ selected を保つ。
	declared := make([]exercise.ExerciseID, 0, 3)
	for _, id := range []exercise.ExerciseID{"bench", "squat", "deadlift"} {
		if !skip[id] {
			declared = append(declared, id)
		}
	}
	program, err := program.NewProgram(freq, simVolume(t), target, ids, declared, focus)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}

	planner := planning.DefaultSessionPlanner()
	res := simResult{
		achieved: map[training.MuscleRegion]float64{},
		target:   target,
		picked:   map[exercise.ExerciseID]int{},
		weeks:    weeks,
	}

	var logs []*setlog.SetLog
	n := 0
	for w := range weeks {
		for _, off := range weekdays[frequency] {
			date := simStart.AddDays(w*7 + off)
			s, err := planner.Plan(planning.PlanRequest{
				Program: program, Pool: all,
				History:    setlog.NewHistory(logs),
				Conditions: condition.NewConditionLog(nil),
				Date:       date,
			})
			if err != nil {
				t.Fatalf("%v: Plan が失敗: %v", date, err)
			}

			total := 0
			// 3レーンすべてを消化する。Variation を落とすと、重点種目を
			// 指定したときの実測が本番と食い違う。
			done := append(s.Main(), s.Variation()...)
			for _, set := range append(done, s.Accessories()...) {
				res.picked[set.ExerciseID()]++

				kg := 0.0
				if w, ok := set.Weight(); ok {
					kg = w.Kg()
				} else {
					// 初回はユーザーが自分で決める。実力の7割を入れたとする。
					res.undecided++
					kg = trueOneRepMax(set.ExerciseID()) * 0.7
				}

				e := byID[set.ExerciseID()]
				for range set.Sets().Int() {
					n++
					total++
					l, err := setlog.NewSetLog(setlog.SetLogParams{
						ID: fmt.Sprintf("l%05d", n), PerformedOn: date,
						ExerciseID: string(set.ExerciseID()),
						WeightKg:   kg, Reps: 8, RIR: set.TargetRIR().Int(),
					})
					if err != nil {
						t.Fatalf("ログの生成に失敗: %v", err)
					}
					logs = append(logs, l)

					for _, r := range e.Stimulus().Regions() {
						c, _ := e.Stimulus().Contribution(r)
						res.achieved[r] += c.Float()
					}
				}
			}
			res.setsPer = append(res.setsPer, total)
		}
	}

	for r := range res.achieved {
		res.achieved[r] /= float64(weeks)
	}
	return res
}

// 週目標が全頻度で現実的な範囲に収まること。
//
// 届かない目標は「毎週すべての区分が赤字」の画面を出し続けるだけで、
// 何も導かない。大幅な超過も同じで、目標が挙動を説明できていない。
func TestSimulation_WeeklyTargetIsAttainableAtEveryFrequency(t *testing.T) {
	// 帯が広めなのは、供給の内訳が頻度で変わるため。1週間に供給できる
	// 総量は頻度に比例するが、限られた枠を21区分に配る形は比例しない。
	// 目標は「意図」であって実測の写しではないので、ぴったり合わせない。
	//
	// この帯に収まるのは、補助の順序が欠けている割合で決まり、残差を
	// 4週の窓で数えているから。日数を第1キーにしていた頃は、1日4種目で
	// 54〜168% に開き、52週平均でも縮まなかった（偏りであってばらつき
	// ではない）。窓が1週だと、週目標が1種目ぶんより小さい区分が抑え
	// られず 179〜202% に張り付いた。
	const (
		minRate = 0.60
		maxRate = 1.45
	)
	// 週1回は見ない。想定する利用者ではない。
	//
	// 週1回×4種目×3セットだと4週で48セットを21区分に配ることになり、
	// 小さい区分の4週ぶんの目標（約2.4セット）が1種目ぶん（3セット）より
	// 小さい。窓をどう取っても配分どおりには回りきらない（実測 54〜179%）。
	// 週1回を選ぶこと自体は妨げないが、配分の質は保証しない。
	for f := 2; f <= maxSimFrequency; f++ {
		t.Run(fmt.Sprintf("週%d回", f), func(t *testing.T) {
			res := simulate(t, f, 8)

			regions := training.AllMuscleRegions()
			sort.Slice(regions, func(i, j int) bool {
				return res.rate(regions[i]) < res.rate(regions[j])
			})
			for _, r := range regions {
				rate := res.rate(r)
				if rate < minRate || rate > maxRate {
					t.Errorf("%s の達成率が範囲外: %.0f%%（目標 %.1f、実測 %.1f）",
						r, rate*100, res.target.Sets(r), res.achieved[r])
				}
			}
		})
	}
}

// 重点種目を指定すると、その系統が週3回前後で回ること。
//
// 軸として週1回、バリエーションとして週2回。本人の実際の運用
// （ベンチだけ週3回、他は週1回）がこの形だった。
//
// 実シードで回すのは、テストが自前のデータだけで完結している罠を避けるため。
// planPool の派生だけ付けてシードに入れ忘れると、単体テストは緑のまま
// 本番で機能が丸ごと無効になる。自重係数で一度踏んだ（D-100）。
func TestSimulation_FocusLineageRunsAboutThreeTimesAWeek(t *testing.T) {
	const weeks = 8
	res := simulateFocused(t, 3, weeks, "bench")

	lineage := []exercise.ExerciseID{
		"bench", "larsen_press", "tempo_bench", "close_grip_bench",
	}
	total := 0
	for _, id := range lineage {
		total += res.picked[id]
	}

	perWeek := float64(total) / weeks
	if perWeek < 2.5 || perWeek > 3.5 {
		t.Errorf("ベンチ系が週%.1f回。3回前後のはず: %v", perWeek, pickedOf(res, lineage))
	}

	// 派生が1つも回っていなければ、レーンが動いていない。
	derived := 0
	for _, id := range lineage[1:] {
		derived += res.picked[id]
	}
	if derived == 0 {
		t.Errorf("派生が一度も出ていない。バリエーションレーンが動いていない: %v",
			pickedOf(res, lineage))
	}
}

// pickedOf は指定した種目の選出回数を、エラーメッセージ用に取り出す。
func pickedOf(res simResult, ids []exercise.ExerciseID) map[exercise.ExerciseID]int {
	out := map[exercise.ExerciseID]int{}
	for _, id := range ids {
		out[id] = res.picked[id]
	}
	return out
}

// どの補助種目も、選ぶ限りは出番があること。
//
// 出てこない種目が混ざっていると「使う種目にチェックを入れるだけ」という
// このパッケージの約束が守られない。
// どの補助種目も、選ぶ限りはどこかの構成で出番があること。
//
// BIG3 を全部やる週3回の構成だけを見ると、レッグカール（ハムのみ）と
// ヒップスラスト（臀筋のみ）は出てこない。デッドリフトが毎回その2区分を
// 主働筋として叩くので、「最も長く放置している区分」の順位で永久に負ける。
// これは選択が壊れているのではなく、デッドリフトをやる人にハムの単関節種目が
// 不要という判断が効いているだけで、デッドリフトを外した構成では出てくる。
func TestSimulation_EveryAccessoryGetsUsedInSomeSetup(t *testing.T) {
	all, _ := seed.Exercises()

	used := map[exercise.ExerciseID]bool{}
	for f := 1; f <= maxSimFrequency; f++ {
		for id := range simulate(t, f, 8).picked {
			used[id] = true
		}
	}
	// メインを外した構成も見る。メインが毎回埋める区分の補助は、
	// メインをやる人には回ってこない。それは死んでいるのではなく
	// 「その構成では要らない」だけなので、要る構成で確かめる。
	//
	//   デッドリフト … ハム・臀筋の主働筋枠が空く
	//   スクワット   … 大腿四頭・内転筋の枠が空く（レッグプレス、アダクション）
	for _, without := range []exercise.ExerciseID{"deadlift", "squat"} {
		for id := range simulateWithout(t, 3, 8, without).picked {
			used[id] = true
		}
	}

	declared := lookupIDs(seed.DefaultDeclared())
	for _, e := range all {
		if declared[e.ID()] {
			continue
		}
		if !used[e.ID()] {
			t.Errorf("%s がどの構成でも一度も選ばれない", e.ID())
		}
	}
}

// セッションの長さが1日の予算を超えないこと。
//
// 上限は「1日の種目数 × 1種目あたりのセット数」。範囲ではなく予算そのもので
// 見る。以前は 9〜36 という決め打ちの幅で見ていたが、この数字は「週1回の人が
// 1週間ぶんを1回で消化する」前提から来ていて、1日の量を決める仕組みが
// できれば意味を失う。
//
// 下限を1種目ぶんに置くのは、軸だけの日（補助が全部回復期間に当たる、
// 分割で狙う区分が尽きる）が正当にあるため。予算に届かない日を責めない。
//
// 予算は利用者の設定から来る。ここでは出荷される既定を測る。
func TestSimulation_SessionLengthIsReasonable(t *testing.T) {
	volume := simVolume(t)
	budget := volume.TotalSets()
	setsPerExercise := volume.Sets()

	for f := 1; f <= maxSimFrequency; f++ {
		res := simulate(t, f, 8)
		for i, n := range res.setsPer {
			if n > budget {
				t.Errorf("週%d回の%d本目が予算超過: %dセット（予算%d）", f, i+1, n, budget)
			}
			if n < setsPerExercise {
				t.Errorf("週%d回の%d本目が短すぎる: %dセット", f, i+1, n)
			}
		}
	}
}

// 重量の未確定は初出のときだけで、一度記録すれば次から確定すること。
//
// 以前は加えて「12本目には未確定が0件」を見ていた。1日9種目のころは12本で
// カタログが一巡していたので成り立ったが、これは推定の性質ではなく
// ローテーションの速さの話だった。1日4種目では、まれにしか選ばれない種目が
// 24本目で初めて出てくる（実測）。本数を延ばしても単調には直らない
// （16・20本は通り、24・30本で落ちる）ので、本数を選んで緑にするのはやめた。
//
// 推定の契約は、1セットずつの「記録があるのに未確定」の検査が守っている。
func TestSimulation_WeightsResolveQuickly(t *testing.T) {
	all, _ := seed.Exercises()
	freq, _ := program.NewFrequency(3)
	target, _ := seed.DefaultWeeklyTarget(freq, simVolume(t))

	ids := make([]exercise.ExerciseID, 0, len(all))
	byID := map[exercise.ExerciseID]*exercise.Exercise{}
	for _, e := range all {
		byID[e.ID()] = e
		ids = append(ids, e.ID())
	}
	program, _ := program.NewProgram(freq, simVolume(t), target, ids,
		[]exercise.ExerciseID{"bench", "squat", "deadlift"}, "")
	planner := planning.DefaultSessionPlanner()

	// 体重を一度は測っている人を想定する。自重種目の負荷は体重×係数＋加重なので、
	// 体重が無いとチンニングとディップスは推定にも処方にも乗らない
	// （その挙動は TestSessionPlanner_BodyweightExerciseNeedsABodyWeight で固定した）。
	conditions := condition.NewConditionLog([]condition.DailyCondition{
		condition.NewDailyCondition(simStart).WithBodyWeight(75),
	})

	var logs []*setlog.SetLog
	n := 0
	seen := map[exercise.ExerciseID]bool{}
	for i := range 12 {
		date := simStart.AddDays(i / 3 * 7).AddDays((i % 3) * 2)
		s, err := planner.Plan(planning.PlanRequest{
			Program: program, Pool: all,
			History:    setlog.NewHistory(logs),
			Conditions: conditions,
			Date:       date,
		})
		if err != nil {
			t.Fatalf("Plan が失敗: %v", err)
		}

		for _, set := range append(s.Main(), s.Accessories()...) {
			kg := 0.0
			if w, ok := set.Weight(); ok {
				kg = w.Kg()
			} else {
				// 未確定が許されるのは初出のときだけ。一度でも記録が
				// あるのに重量が出ないなら、推定の経路が壊れている。
				if seen[set.ExerciseID()] {
					t.Errorf("%d本目: 記録があるのに %s の重量が未確定",
						i+1, set.ExerciseID())
				}
				kg = trueOneRepMax(set.ExerciseID()) * 0.7
			}
			seen[set.ExerciseID()] = true
			for range set.Sets().Int() {
				n++
				l, _ := setlog.NewSetLog(setlog.SetLogParams{
					ID: fmt.Sprintf("r%05d", n), PerformedOn: date,
					ExerciseID: string(set.ExerciseID()),
					WeightKg:   kg, Reps: 8, RIR: set.TargetRIR().Int(),
				})
				logs = append(logs, l)
			}
		}
	}
}

// 数値を目で見るための出力。`go test -run TestSimulation_Report -v` で使う。
func TestSimulation_Report(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("-v のときだけ出力する")
	}
	for f := 1; f <= maxSimFrequency; f++ {
		res := simulate(t, f, 8)
		regions := training.AllMuscleRegions()
		sort.Slice(regions, func(i, j int) bool {
			return res.rate(regions[i]) < res.rate(regions[j])
		})
		t.Logf("=== 週%d回 セット数 %v", f, res.setsPer[:min(6, len(res.setsPer))])
		for _, r := range regions {
			t.Logf("  %-16s 目標 %5.1f 実測 %5.1f  %3.0f%%",
				r, res.target.Sets(r), res.achieved[r], res.rate(r)*100)
		}
		unused := []exercise.ExerciseID{}
		all, _ := seed.Exercises()
		for _, e := range all {
			if res.picked[e.ID()] == 0 {
				unused = append(unused, e.ID())
			}
		}
		t.Logf("  未使用: %v", unused)
	}
}

// simVolume は通し検証で使う1回の量。
//
// **出荷される既定をそのまま使う。**テスト用の値を置くと、利用者が実際に
// 受け取る構成では一度も測っていないことになる。既定を動かしたら、この
// 検証の数字も動くのが正しい。
func simVolume(t *testing.T) program.SessionVolume {
	t.Helper()
	v, err := seed.DefaultSessionVolume()
	if err != nil {
		t.Fatalf("既定の1回の量が不正: %v", err)
	}
	return v
}
