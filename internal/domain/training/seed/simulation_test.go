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
	sessions  []simSession
}

// simSession は1セッションの内訳。分割の検証で、その日に何が出たかを見る。
type simSession struct {
	date        training.Date
	split       program.Split // 分割なしならゼロ値
	hasSplit    bool
	main        []exercise.ExerciseID
	variation   []exercise.ExerciseID
	accessories []exercise.ExerciseID
	sets        int
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
	return runSim(t, simConfig{
		frequency: frequency, weeks: weeks, focus: focus, excluded: excluded,
	})
}

// simConfig は1回の通し検証の条件。
//
// 引数を並べるのをやめたのは、分割を足した時点で位置引数が5つになり、
// 呼び出し側で何がどれか読めなくなるため。
type simConfig struct {
	frequency int
	weeks     int
	focus     exercise.ExerciseID
	excluded  []exercise.ExerciseID
	cycle     []program.Split // 空なら分割なし（全身法）
}

// runSim は条件どおりに何週間か実施した場合を回す。
func runSim(t *testing.T, cfg simConfig) simResult {
	t.Helper()

	skip := make(map[exercise.ExerciseID]bool, len(cfg.excluded))
	for _, id := range cfg.excluded {
		skip[id] = true
	}

	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	freq, err := program.NewFrequency(cfg.frequency)
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
	prog, err := program.NewProgram(freq, simVolume(t), ids, declared, cfg.focus)
	if err != nil {
		t.Fatalf("プログラムの生成に失敗: %v", err)
	}
	if len(cfg.cycle) > 0 {
		prog, err = prog.WithCycle(cfg.cycle)
		if err != nil {
			t.Fatalf("分割の設定に失敗: %v", err)
		}
	}

	planner := planning.DefaultSessionPlanner()
	res := simResult{
		achieved: map[training.MuscleRegion]float64{},
		target:   target,
		picked:   map[exercise.ExerciseID]int{},
		weeks:    cfg.weeks,
	}

	var logs []*setlog.SetLog
	n := 0
	attended := 0 // ハーネス側で数えた出席回数。周期の位置はこれで決まる。
	for w := range cfg.weeks {
		for _, off := range weekdays[cfg.frequency] {
			date := simStart.AddDays(w*7 + off)
			s, err := planner.Plan(planning.PlanRequest{
				Program: prog, Target: target, Pool: all,
				History:    setlog.NewHistory(logs),
				Conditions: condition.NewConditionLog(nil),
				Date:       date,
			})
			if err != nil {
				t.Fatalf("%v: Plan が失敗: %v", date, err)
			}

			// 今日の分割は、ハーネスが数えた出席回数から自前で引く。
			// Program.SplitOn を呼ぶと、周期の位置がずれる壊れ方を
			// ハーネスが一緒に間違えるので、検証が素通りする。
			hasSplit := len(cfg.cycle) > 0
			var today program.Split
			if hasSplit {
				today = cfg.cycle[attended%len(cfg.cycle)]
			}
			rec := simSession{date: date, split: today, hasSplit: hasSplit}

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
			rec.main = idsOf(s.Main())
			rec.variation = idsOf(s.Variation())
			rec.accessories = idsOf(s.Accessories())
			rec.sets = total
			res.sessions = append(res.sessions, rec)
			res.setsPer = append(res.setsPer, total)
			if total > 0 {
				attended++
			}
		}
	}

	for r := range res.achieved {
		res.achieved[r] /= float64(cfg.weeks)
	}
	return res
}

// idsOf は計画されたセットの種目IDを並び順のまま取り出す。
func idsOf(sets []planning.PlannedSet) []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, 0, len(sets))
	for _, s := range sets {
		out = append(out, s.ExerciseID())
	}
	return out
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
// 下限そのものを外さないのは、何も出ない日（0セット）は捕まえたいから。
// ジムに来て空のリストが出るのは、予算に届かないのとは別の壊れ方。
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
	program, _ := program.NewProgram(freq, simVolume(t), ids,
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
			Program: program, Target: target, Pool: all,
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

// ここから下は分割（スプリット）を設定した経路の通し検証。
//
// 分割なしの経路だけを回していると、プランナーが分割を読む部分が実シードで
// 一度も動かない。単体テストは作り物のプール（上半身6・下半身6）で通るが、
// 実シードの29種目では「その日に選べる補助が3種目しかない」日ができる。

// splitCycles は実シードのプリセットを返す。
//
// 全身法（分割なし）は既存の TestSimulation_* が回しているので含めない。
// seed.SplitPresets() が返すのは分割を設定する3つだけ。
func splitCycles(t *testing.T) []seed.SplitPreset {
	t.Helper()
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("分割のプリセットが不正: %v", err)
	}
	if len(presets) == 0 {
		t.Fatal("分割のプリセットが空")
	}
	return presets
}

// splitWeeks は周期が整数回まわる週数を返す。
//
// 達成率は週数で割って出すので、周期の途中で終わると「2回来た分割」と
// 「1回しか来なかった分割」が混ざり、実装ではなく週数の取り方で数字が動く。
// 5分割を週1回で回すと1周に5週かかるので、8週では割り切れない。
func splitWeeks(frequency, cycleLen int) int {
	for w := 8; w < 8+cycleLen; w++ {
		if (frequency*w)%cycleLen == 0 {
			return w
		}
	}
	return 8
}

// splitAllocatorRewritten は補助の割り振りの入れ替え（PR 3。
// docs/specs/2026-09-26-accessory-allocation-design.md）が終わったら true にする。
//
// 単一のスイッチ。true にすると、下の「既知の赤」が全部いつもの assertion に
// 戻る。PR 3 のあとにこれを true にして go test ./... が緑にならなければ、
// 割り振り器がここに挙げた赤を消しきれていない、ということ。
const splitAllocatorRewritten = true

// knownEmptySplitSession は、いまの補助選択が分割＋回復の二重制約で
// 候補を使い切り、空セッション（軸も補助も無い）を出す既知の組み合わせ。
//
// five_way の週6・7回、腕の日。肩・腕の日は軸を持たない設計だが、頻度が
// 上がるほど同じ区分の日が近接し、回復期間中の種目しか残らず補助まで
// 尽きる。#175（1回の量の予算）で1回に入る枠が9固定から4に絞られたぶん、
// 同じ現象がより浅いところ（以前は最小6セット止まりだったのが、いまは0）
// で顕在化した。docs/specs/…-accessory-allocation-design.md が
// 「1回が6セットまで落ちる」として挙げていたのと同根で、1週ぶんをまとめて
// 見る割り振り器（PR 3）が解消する見込み。
func knownEmptySplitSession(presetKey string, frequency int) bool {
	return presetKey == "five_way" && (frequency == 6 || frequency == 7)
}

// logKnownRed は既知の赤を、-v で見えるログとして残しつつ検査を通す。
func logKnownRed(t *testing.T, msg string) {
	t.Helper()
	t.Logf("既知の赤（補助の割り振りの入れ替え・PR 3 で有効化する）: %s", msg)
}

// 分割を設定しても計画が出続けること。
//
// 空のセッションが出ると、周期が出席回数で進む以上そこで止まる。記録が
// 増えないので次の日も同じ分割が来て、永久に同じ日を繰り返す。
func TestSimulation_SplitAlwaysProducesASession(t *testing.T) {
	for _, p := range splitCycles(t) {
		for f := 1; f <= maxSimFrequency; f++ {
			t.Run(fmt.Sprintf("%s/週%d回", p.Key, f), func(t *testing.T) {
				res := runSim(t, simConfig{
					frequency: f, weeks: splitWeeks(f, len(p.Cycle)), cycle: p.Cycle,
				})
				for i, s := range res.sessions {
					if s.sets == 0 {
						msg := fmt.Sprintf("%d本目（%s の日）が空。周期がここで止まる", i+1, s.split.Name())
						if !splitAllocatorRewritten && knownEmptySplitSession(p.Key, f) {
							logKnownRed(t, msg)
							continue
						}
						t.Fatalf("%s", msg)
					}
				}
			})
		}
	}
}

// 今日の分割に属さない区分は狙わないこと。
//
// 「今日は脚の日」と言いながら胸の補助が出るなら、分割の型が意味を
// 持たない。軸は主働（寄与1.0以上）が今日の区分に入っていること、補助は
// 今日の区分のどれかに寄与があること。補助の条件がゆるいのは、残差を
// 今日の区分に絞ったうえで「その区分を埋められる種目」を選ぶ実装だから
// （planning.primaryContribution は軸の側の閾値）。
//
// ただし**どの日にも属さない区分を主働に持つ補助は、どの日に出ても正しい**
// （#113）。腹はどのプリセットにも入っておらず、「その日に無い」ではなく
// 「二度と来ない」なので毎日活かす、と決めた。ここを例外にしないと、
// 仕様どおりに出ているクランチとサイドベンドを赤にしてしまう。
//
// 例外は**主働**に限る。スクワットやデッドリフトは腹に副次の寄与を持つので、
// 「どこか1つでも未所属なら通す」にすると胸の日にスクワットが通り、この検査が
// 何も守らなくなる。周期の所属は planning の非公開関数を借りず、ハーネスが
// p.Cycle から自分で数える（借りると同じ壊れ方を一緒に間違える）。
func TestSimulation_SplitKeepsTheDayInsideItsRegions(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	byID := map[exercise.ExerciseID]*exercise.Exercise{}
	for _, e := range all {
		byID[e.ID()] = e
	}

	for _, p := range splitCycles(t) {
		for f := 1; f <= maxSimFrequency; f++ {
			for _, focus := range []exercise.ExerciseID{"", "bench"} {
				t.Run(fmt.Sprintf("%s/週%d回/重点%q", p.Key, f, focus), func(t *testing.T) {
					res := runSim(t, simConfig{
						frequency: f, weeks: splitWeeks(f, len(p.Cycle)),
						focus: focus, cycle: p.Cycle,
					})
					for i, s := range res.sessions {
						for _, id := range append(s.main, s.variation...) {
							if !primaryInSplit(byID[id], s.split) {
								t.Errorf("%d本目（%s の日）の軸 %s は、その日の区分を主働に持たない",
									i+1, s.split.Name(), id)
							}
						}
						for _, id := range s.accessories {
							if touchesSplit(byID[id], s.split) {
								continue
							}
							if primaryUnaffiliated(byID[id], p.Cycle) {
								continue
							}
							msg := fmt.Sprintf("%d本目（%s の日）に無関係な補助 %s が出ている",
								i+1, s.split.Name(), id)
							if !splitAllocatorRewritten && knownRedFrontSquatLeak(id) {
								logKnownRed(t, msg)
								continue
							}
							t.Errorf("%s", msg)
						}
					}
				})
			}
		}
	}
}

// primaryInSplit はその種目の主働区分（寄与1.0以上）が分割に含まれるか。
func primaryInSplit(e *exercise.Exercise, s program.Split) bool {
	if e == nil {
		return false
	}
	for _, r := range e.Stimulus().Regions() {
		c, ok := e.Stimulus().Contribution(r)
		if ok && c.Float() >= 1.0 && s.Includes(r) {
			return true
		}
	}
	return false
}

// primaryUnaffiliated はその種目の主働区分（寄与1.0以上）のどれかが、
// 周期のどの日にも書かれていないか。
//
// 未所属の区分は毎日活きる（#113）ので、それを主働に持つ種目はどの日に
// 出てもよい。副次の寄与では通さない。
func primaryUnaffiliated(e *exercise.Exercise, cycle []program.Split) bool {
	if e == nil {
		return false
	}
	for _, r := range e.Stimulus().Regions() {
		c, ok := e.Stimulus().Contribution(r)
		if !ok || c.Float() < 1.0 {
			continue
		}
		if !affiliatedInCycle(cycle, r) {
			return true
		}
	}
	return false
}

// knownRedFrontSquatLeak は front_squat が分割の外の日に補助として漏れ出る、
// 既知の赤（docs/specs/…-accessory-allocation-design.md
// 「肩・腕・胸の日にフロントスクワットが出る」）。
//
// front_squat は Abs 0.4 の副次寄与を持つ。腹はどのプリセットにも未所属で
// #113 のとおり毎日活きるので、腹を埋める目的で選ばれて胸・肩・腕の日にも
// 出てくる。「副次の寄与では未所属の例外にしない」という判断（bf615e87）
// 自体は正しく、この検査を緩めるとその判断ごと壊れるので、帯や例外は
// 動かさず、種目名で既知の赤として保持する。1週ぶんの割り振り器（PR 3）が
// 日の外の区分への刺激を候補選定の時点で締め出す設計なので解消の見込み。
func knownRedFrontSquatLeak(id exercise.ExerciseID) bool {
	return id == "front_squat"
}

// affiliatedInCycle はその区分が周期のどこかの日に書かれているか。
func affiliatedInCycle(cycle []program.Split, r training.MuscleRegion) bool {
	for _, day := range cycle {
		if day.Includes(r) {
			return true
		}
	}
	return false
}

// touchesSplit はその種目が分割の区分に何らかの寄与を持つか。
func touchesSplit(e *exercise.Exercise, s program.Split) bool {
	if e == nil {
		return false
	}
	for _, r := range e.Stimulus().Regions() {
		if s.Includes(r) {
			return true
		}
	}
	return false
}

// 5分割の肩・腕の日は軸が空になり、それでも計画が出ること。
//
// BIG3 の中に肩・腕を主働に持つ種目が無い。0.88 のスクワットを肩の日に
// 出すより軸の枠が無いほうが正直、と決めた（2026-09-19 の仕様書）。
// 空になるのはその2つの日だけで、他のプリセットでは一度も空にならない。
func TestSimulation_FiveWayLeavesTheAxisEmptyOnShoulderAndArmDays(t *testing.T) {
	// 軸が空になってよい日の名前。プリセットの定義に合わせる。
	axisless := map[string]bool{"肩": true, "腕": true}

	for _, p := range splitCycles(t) {
		for f := 1; f <= maxSimFrequency; f++ {
			t.Run(fmt.Sprintf("%s/週%d回", p.Key, f), func(t *testing.T) {
				res := runSim(t, simConfig{
					frequency: f, weeks: splitWeeks(f, len(p.Cycle)), cycle: p.Cycle,
				})
				empty := 0
				for i, s := range res.sessions {
					if len(s.main) > 0 {
						if axisless[s.split.Name()] {
							t.Errorf("%d本目（%s の日）に軸 %v が出ている。宣言に主働を持つ種目は無いはず",
								i+1, s.split.Name(), s.main)
						}
						continue
					}
					empty++
					if !axisless[s.split.Name()] {
						t.Errorf("%d本目（%s の日）の軸が空。この日は宣言から軸が取れるはず",
							i+1, s.split.Name())
					}
					if len(s.accessories) == 0 {
						msg := fmt.Sprintf("%d本目（%s の日）が軸も補助も無い", i+1, s.split.Name())
						if !splitAllocatorRewritten && knownEmptySplitSession(p.Key, f) {
							logKnownRed(t, msg)
							continue
						}
						t.Errorf("%s", msg)
					}
				}
				if p.Key == "five_way" && empty == 0 {
					t.Error("5分割なのに軸が空の日が一度も無い。肩・腕の日が来ていない")
				}
			})
		}
	}
}

// knownRedObliqueShortfall は ppl・週3回で腹斜筋の達成率が帯を割る、既知の赤。
//
// PR #107（main 取り込み前）では腹（ABS/OBLIQUE）の週1回の振り切れ（超過、
// 218〜260%）が問題だった。#175 で1回の量の予算が9固定から4に絞られた
// ことで逆側に振れ、今度は一部の頻度で不足するようになった。未所属の規則
// 自体（#113）は生きていて一度も出ないわけではないので、「死んでいる」
// ではなく「量が足りない」。1週ぶんをまとめて見る割り振り器（PR 3）が
// 日の外の区分もまとめて埋め直す設計なので解消の見込み。
func knownRedObliqueShortfall(presetKey string, frequency int) bool {
	return presetKey == "ppl" && frequency == 3
}

// どの分割にも属さない区分（腹直筋・腹斜筋）が、分割を設定しても死なないこと。
//
// 仕様は「未所属の区分は毎日活きる」と決めている。全部の集合に書かせると
// 書き忘れた区分が永久に埋まらないので、書かれていない区分は毎日狙う、が
// その結論だった（2026-09-19 の仕様書）。
func TestSimulation_UnaffiliatedRegionsStayActiveUnderSplit(t *testing.T) {
	// 体幹の種目。どのプリセットの区分にも腹は入っていないので、
	// 未所属の規則が効いていなければ一度も出ない。
	core := []exercise.ExerciseID{"cable_crunch", "side_bend"}

	for _, p := range splitCycles(t) {
		for f := 1; f <= maxSimFrequency; f++ {
			t.Run(fmt.Sprintf("%s/週%d回", p.Key, f), func(t *testing.T) {
				res := runSim(t, simConfig{
					frequency: f, weeks: splitWeeks(f, len(p.Cycle)), cycle: p.Cycle,
				})
				for _, id := range core {
					if res.picked[id] == 0 {
						t.Errorf("%s が一度も出ない。未所属の区分が分割で死んでいる: %v",
							id, pickedOf(res, core))
					}
				}
				if rate := res.rate(training.Oblique); rate < 0.60 {
					msg := fmt.Sprintf("腹斜筋の達成率が %.0f%%（目標 %.1f、実測 %.1f）",
						rate*100, res.target.Sets(training.Oblique),
						res.achieved[training.Oblique])
					if !splitAllocatorRewritten && knownRedObliqueShortfall(p.Key, f) {
						logKnownRed(t, msg)
					} else {
						t.Errorf("%s", msg)
					}
				}
			})
		}
	}
}

// splitAttainmentKnownRed は TestSimulation_SplitWeeklyTargetIsAttainable で
// いま帯（60〜145%）を外れている (プリセット, 頻度, 区分) の組み合わせ。
//
// #175（1回の量の予算）で1回に入る枠が9固定から4に絞られたことで、分割の
// 「今日の区分しか狙わない」制約と組み合わさり、多くの区分が4週の窓に
// 届かなくなった。PR #107（main 取り込み前）が挙げていた ABS/OBLIQUE の
// 超過より範囲が広い。帯を緩めるのではなく、いま実際に外れている組だけを
// ここに列挙して既知の赤として保持する（測定日 2026-09-26、main は
// 19d0dd0d + このブランチ）。1週ぶんをまとめて見る割り振り器（PR 3）が
// 解消する見込み。
//
// five_way・週1回はここに含めない。1区分が5週に1度しか来ず構造的に
// 届かないので、帯の対象外として別に除外する（下記）。
var splitAttainmentKnownRed = map[string]bool{
	"upper_lower|1|SIDE_DELT":    true,
	"upper_lower|1|REAR_DELT":    true,
	"upper_lower|1|LAT":          true,
	"upper_lower|3|BICEPS":       true,
	"upper_lower|3|CHEST_UPPER":  true,
	"upper_lower|4|CHEST_LOWER":  true,
	"upper_lower|5|SIDE_DELT":    true,
	"upper_lower|5|TRICEPS_LONG": true,
	"upper_lower|6|SIDE_DELT":    true,
	"upper_lower|6|TRICEPS_LONG": true,
	"upper_lower|7|CHEST_LOWER":  true,
	"upper_lower|7|TRAP_MID":     true,
	"upper_lower|7|LAT":          true,
	"ppl|1|REAR_DELT":            true,
	"ppl|1|ADDUCTOR":             true,
	"ppl|1|TRICEPS_LONG":         true,
	"ppl|1|FOREARM":              true,
	"ppl|3|OBLIQUE":              true,
	"ppl|3|FOREARM":              true,
	"five_way|2|ADDUCTOR":        true,
	"five_way|6|ADDUCTOR":        true,
}

func knownRedSplitAttainment(presetKey string, frequency int, r training.MuscleRegion) bool {
	return splitAttainmentKnownRed[fmt.Sprintf("%s|%d|%s", presetKey, frequency, r)]
}

// 分割を設定しても週目標が現実的な範囲に収まること。
//
// 帯は分割なしと同じ 60〜145%。分割は「同じ週の量を日で割り振る型」で
// あって、量そのものを変える設定ではない。届かないなら、それは型が
// 週目標を配りきれていない。
func TestSimulation_SplitWeeklyTargetIsAttainable(t *testing.T) {
	const (
		minRate = 0.60
		maxRate = 1.45
	)

	for _, p := range splitCycles(t) {
		for f := 1; f <= maxSimFrequency; f++ {
			t.Run(fmt.Sprintf("%s/週%d回", p.Key, f), func(t *testing.T) {
				// 5分割・週1回は対象外。1区分が5週に1度しか来ず、4週の窓には
				// どう割り振っても構造的に届かない
				// （docs/specs/2026-09-26-accessory-allocation-design.md
				// 「受け入れない構成」）。帯を広げるのではなく、この構成
				// だけを外す。
				if p.Key == "five_way" && f == 1 {
					t.Skip("5分割・週1回は1区分が5週に1度しか来ず、4週の窓に構造的に届かない。達成率の帯の対象外（docs/specs/2026-09-26-accessory-allocation-design.md 受け入れない構成）")
				}

				res := runSim(t, simConfig{
					frequency: f, weeks: splitWeeks(f, len(p.Cycle)), cycle: p.Cycle,
				})

				regions := training.AllMuscleRegions()
				sort.Slice(regions, func(i, j int) bool {
					return res.rate(regions[i]) < res.rate(regions[j])
				})
				for _, r := range regions {
					rate := res.rate(r)
					if rate < minRate || rate > maxRate {
						msg := fmt.Sprintf("%s の達成率が範囲外: %.0f%%（目標 %.1f、実測 %.1f）",
							r, rate*100, res.target.Sets(r), res.achieved[r])
						if !splitAllocatorRewritten && knownRedSplitAttainment(p.Key, f, r) {
							logKnownRed(t, msg)
							continue
						}
						t.Errorf("%s", msg)
					}
				}
			})
		}
	}
}

// 分割を設定してもセッションの長さが予算の範囲に収まること。
//
// 帯は分割なしの TestSimulation_SessionLengthIsReasonable と同じ
// 「予算（種目数×1種目あたりのセット数）以内、かつ1種目ぶん以上」。
// 固定の9〜36は #175（1回の量の予算）より前の、9種目固定枠だった頃の
// 名残で、予算が可変になった時点で意味を失っている（分割なしの帯と
// 同時に消した）。
func TestSimulation_SplitSessionLengthIsReasonable(t *testing.T) {
	volume := simVolume(t)
	budget := volume.TotalSets()
	setsPerExercise := volume.Sets()

	for _, p := range splitCycles(t) {
		for f := 1; f <= maxSimFrequency; f++ {
			for _, focus := range []exercise.ExerciseID{"", "bench"} {
				t.Run(fmt.Sprintf("%s/週%d回/重点%q", p.Key, f, focus), func(t *testing.T) {
					res := runSim(t, simConfig{
						frequency: f, weeks: splitWeeks(f, len(p.Cycle)),
						focus: focus, cycle: p.Cycle,
					})
					for i, s := range res.sessions {
						if s.sets > budget {
							t.Errorf("%d本目（%s の日）が予算超過: %dセット（予算%d）",
								i+1, s.split.Name(), s.sets, budget)
						}
						if s.sets < setsPerExercise {
							msg := fmt.Sprintf("%d本目（%s の日）のセット数が現実的でない: %d",
								i+1, s.split.Name(), s.sets)
							if !splitAllocatorRewritten && knownEmptySplitSession(p.Key, f) {
								logKnownRed(t, msg)
								continue
							}
							t.Errorf("%s", msg)
						}
					}
				})
			}
		}
	}
}

// 分割ごとの数値を目で見るための出力。
// `go test -run TestSimulation_SplitReport -v` で使う。
//
// 既存の TestSimulation_Report は重点種目を指定していないのでバリエーションを
// 見ない。分割の検証にはこちらを使う。
func TestSimulation_SplitReport(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("-v のときだけ出力する")
	}

	lineage := []exercise.ExerciseID{
		"bench", "larsen_press", "tempo_bench", "close_grip_bench",
	}

	for _, p := range splitCycles(t) {
		for f := 1; f <= maxSimFrequency; f++ {
			for _, focus := range []exercise.ExerciseID{"", "bench"} {
				weeks := splitWeeks(f, len(p.Cycle))
				res := runSim(t, simConfig{
					frequency: f, weeks: weeks, focus: focus, cycle: p.Cycle,
				})

				lo, hi, emptyAxis, variations := 1<<30, 0, 0, 0
				for _, s := range res.sessions {
					lo, hi = min(lo, s.sets), max(hi, s.sets)
					if len(s.main) == 0 {
						emptyAxis++
					}
					variations += len(s.variation)
				}
				family := 0
				for _, id := range lineage {
					family += res.picked[id]
				}

				regions := training.AllMuscleRegions()
				sort.Slice(regions, func(i, j int) bool {
					return res.rate(regions[i]) < res.rate(regions[j])
				})
				out := []string{}
				for _, r := range regions {
					if rate := res.rate(r); rate < 0.60 || rate > 1.45 {
						out = append(out, fmt.Sprintf("%s=%.0f%%", r, rate*100))
					}
				}
				t.Logf("=== %s 週%d回 重点%q（%d週）セット %d..%d 空軸 %d本 バリエーション %d本 ベンチ系 週%.1f回",
					p.Key, f, focus, weeks, lo, hi, emptyAxis, variations,
					float64(family)/float64(weeks))
				t.Logf("    範囲外: %v", out)
			}
		}
	}
}
