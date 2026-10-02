// Package devsim は開発用のシミュレーションを回す。
//
// 設定を変えたときに計画がどう変わるかを、人が目で見るためのもの。
// 通し検証（`seed` のテスト）が数字で守るのに対して、こちらは**何が
// 起きているか**を見せる。どちらか一方では足りない。
//
// 本番の経路には入らない。DB も認証も使わず、履歴はその場で捏造する。
// 要らなくなったらこのディレクトリと、cmd の取り付け1箇所を消せばよい。
package devsim

import (
	"fmt"
	"slices"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// Request は1回ぶんのシミュレーションの入力。
type Request struct {
	Declared  []exercise.ExerciseID
	Focus     exercise.ExerciseID
	SplitKey  string // 分割プリセットのキー。空なら分割なし
	Frequency int
	Weeks     int
	Start     training.Date
	Athlete   AthleteParams

	// ExercisesPerSession と SetsPerExercise は1回の量。
	ExercisesPerSession int
	SetsPerExercise     int
	// Weekdays は通う曜日（開始日からの日数 0〜6）。空なら頻度ごとの既定。
	Weekdays []int

	// Custom は利用者が足した種目。共通の一覧に加えて全部「使う種目」に入る。
	// ID は並び順で CustomExerciseID が振る。
	Custom []CustomExercise
}

// Set は計画された1種目。
type Set struct {
	ExerciseID exercise.ExerciseID
	Name       string
	WeightKg   float64
	HasWeight  bool
	Sets       int
	TargetRIR  int
	// PctOfOneRM は推定1RMに対する比。一巡が回っているかはこれで見る。
	// 推定が立たない初出の日は 0。
	PctOfOneRM float64
	// Performed は模擬ユーザーが実際に記録した値。処方の重量が無い日も
	// 本人が選んだ重さで埋まる。全セット同じ値で記録する。
	Performed Performed
	// AthleteOneRepMaxKg はその日の模擬ユーザーの実力（加重の1RM）。
	// 処方が実力を追えているかは、これと WeightKg を比べて見る。
	AthleteOneRepMaxKg float64
}

// Day は1セッション。
type Day struct {
	Date        training.Date
	SplitName   string
	Main        []Set
	Variation   []Set
	Accessories []Set
	TotalSets   int
}

// RegionVolume は週ごと・筋区分ごとの充足。
type RegionVolume struct {
	Region training.MuscleRegion
	Target float64
	Done   float64
}

// Week は1週間の結果。
type Week struct {
	Index   int // 1始まり
	Regions []RegionVolume
}

// Result はシミュレーションの結果。
type Result struct {
	Days  []Day
	Weeks []Week
}

// weekdays は頻度ごとの曜日オフセット（月曜=0）。
//
// `seed` の通し検証と同じ並びにしてある。別の並びにすると、画面で見た
// 数字とテストが守っている数字が食い違う。
var weekdays = map[int][]int{
	1: {0},
	2: {0, 3},
	3: {0, 2, 4},
	4: {0, 2, 4, 6},
	5: {0, 1, 2, 4, 5},
	6: {0, 1, 2, 3, 4, 5},
	7: {0, 1, 2, 3, 4, 5, 6},
}

// DefaultWeekdays は頻度ごとの既定の曜日（開始日からの日数）。
func DefaultWeekdays(frequency int) ([]int, bool) {
	days, ok := weekdays[frequency]
	return append([]int(nil), days...), ok
}

// offsets は通う曜日。指定が無ければ頻度ごとの既定。
//
// 指定するときは頻度と数を揃える。頻度は週目標の割り付けに効くので、
// 曜日の数と食い違うと「週4の目標を週2でこなす」ことになる。
func (r Request) offsets() ([]int, error) {
	if len(r.Weekdays) == 0 {
		days, ok := DefaultWeekdays(r.Frequency)
		if !ok {
			return nil, fmt.Errorf("頻度が範囲外である: %d", r.Frequency)
		}
		return days, nil
	}
	if len(r.Weekdays) != r.Frequency {
		return nil, fmt.Errorf("曜日の数（%d）が頻度（%d）と違う", len(r.Weekdays), r.Frequency)
	}
	seen := map[int]bool{}
	for _, d := range r.Weekdays {
		if d < 0 || d > 6 || seen[d] {
			return nil, fmt.Errorf("曜日は 0〜6 を重複なく指定する: %v", r.Weekdays)
		}
		seen[d] = true
	}
	days := append([]int(nil), r.Weekdays...)
	slices.Sort(days)
	return days, nil
}

// Simulator は処方どおり実施し続けた場合の計画を作る。
type Simulator struct {
	pool      []*exercise.Exercise
	planner   planning.SessionPlanner
	estimator planning.OneRepMaxEstimator
	presets   []seed.SplitPreset
}

func NewSimulator(pool []*exercise.Exercise) (*Simulator, error) {
	presets, err := seed.SplitPresets()
	if err != nil {
		return nil, fmt.Errorf("分割プリセットが不正: %w", err)
	}
	return &Simulator{
		pool:      pool,
		planner:   planning.DefaultSessionPlanner(),
		estimator: planning.DefaultOneRepMaxEstimator(),
		presets:   presets,
	}, nil
}

// Presets は選べる分割の一覧。画面がそのまま出す。
func (s *Simulator) Presets() []seed.SplitPreset { return s.presets }

// Pool は種目マスタ。画面が宣言と重点種目の候補に使う。
func (s *Simulator) Pool() []*exercise.Exercise { return s.pool }

// CustomExercise は模擬ユーザーが足した自分の種目。本番の POST /api/exercises と
// 同じ入力（名前・区分ごとの寄与・刻み）で表す。
type CustomExercise struct {
	Name        string
	Stimulus    map[training.MuscleRegion]float64
	IncrementKg float64
}

// CustomExerciseID は i 番目（0始まり）の自分の種目の ID。
//
// 本番は乱数で振るが、ここでは同じ設定から同じ結果を出したいので並び順で振る。
// orm=u-sim01:80 のように、1RM の上書きもこの ID で指す。
func CustomExerciseID(i int) exercise.ExerciseID {
	return exercise.ExerciseID(fmt.Sprintf("%ssim%02d", exercise.CustomExerciseIDPrefix, i+1))
}

// poolFor は共通の一覧に req の自分の種目を足した一覧を返す。
//
// 名前の重複は本番と同じく弾く（共通の種目と、自分の種目どうし）。
// 週目標はどの一覧からも出ない（seed.DefaultWeeklyTarget は頻度と1回の量だけで決まる）。
func (s *Simulator) poolFor(req Request) ([]*exercise.Exercise, error) {
	if len(req.Custom) == 0 {
		return s.pool, nil
	}
	out := make([]*exercise.Exercise, 0, len(s.pool)+len(req.Custom))
	out = append(out, s.pool...)
	names := make(map[string]bool, cap(out))
	for _, e := range s.pool {
		names[e.Name()] = true
	}
	for i, c := range req.Custom {
		e, err := exercise.NewExercise(exercise.ExerciseParams{
			ID: string(CustomExerciseID(i)), Name: c.Name,
			Stimulus: c.Stimulus, IncrementKg: c.IncrementKg,
		})
		if err != nil {
			return nil, fmt.Errorf("自分の種目 %d 番目（%s）が不正: %w", i+1, c.Name, err)
		}
		if names[e.Name()] {
			return nil, fmt.Errorf("自分の種目 %d 番目: %w: %s", i+1, exercise.ErrDuplicateExerciseName, e.Name())
		}
		names[e.Name()] = true
		out = append(out, e)
	}
	return out, nil
}

// PoolFor は req で使う種目の一覧（共通の一覧＋自分の種目）。画面が1RM の
// 既定値と名前を出すのに使う。
func (s *Simulator) PoolFor(req Request) ([]*exercise.Exercise, error) { return s.poolFor(req) }

// Run は req の設定で Weeks 週ぶんの計画を作る。
//
// 処方どおり全部こなしたことにして履歴を進める。**やめどきを本人が決める**
// のが本来の姿（D-116）だが、途中でやめる量を仮定すると、その仮定のほうが
// 結果を決めてしまう。全部こなした場合を見る。
func (s *Simulator) Run(req Request) (Result, error) {
	pool, err := s.poolFor(req)
	if err != nil {
		return Result{}, err
	}
	prog, err := s.buildProgram(req, pool)
	if err != nil {
		return Result{}, err
	}
	// 週目標はプログラムの持ち物ではなく、設定（頻度と1回の量）から導く
	// 値（D-139、#176）。頻度も量も週の途中で動かないので、ループの外で
	// 一度組めば足りる。
	target, err := seed.DefaultWeeklyTarget(prog.Frequency(), prog.SessionVolume())
	if err != nil {
		return Result{}, fmt.Errorf("週目標が組めない: %w", err)
	}

	offsets, err := req.offsets()
	if err != nil {
		return Result{}, err
	}

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	for _, e := range pool {
		byID[e.ID()] = e
	}
	if err := req.Athlete.validate(byID); err != nil {
		return Result{}, err
	}
	who := athlete{params: req.Athlete, start: req.Start}

	conditions := condition.NewConditionLog([]condition.DailyCondition{
		condition.NewDailyCondition(req.Start).WithBodyWeight(req.Athlete.BodyWeightKg),
	})

	out := Result{}
	var logs []*setlog.SetLog
	// effective は同じ記録を体重込みの負荷に直したもの。推定比を出すのに
	// だけ使う。プランナーも推定は体重込みで行う（planning.effectiveHistory）。
	// 記録のままで推定すると、自重種目の比が意味の無い値になる。
	var effective []*setlog.SetLog
	n := 0

	for w := range req.Weeks {
		week := Week{Index: w + 1}
		done := map[training.MuscleRegion]float64{}

		for _, off := range offsets {
			date := req.Start.AddDays(w*7 + off)
			history := setlog.NewHistory(logs)
			estimable := setlog.NewHistory(effective)

			planned, err := s.planner.Plan(planning.PlanRequest{
				Program:    prog,
				Target:     target,
				Pool:       pool,
				History:    history,
				Conditions: conditions,
				Date:       date,
			})
			if err != nil {
				return Result{}, fmt.Errorf("%v の計画に失敗: %w", date, err)
			}

			day := Day{Date: date, SplitName: s.splitNameOn(prog, history, date)}
			for _, lane := range []struct {
				sets []planning.PlannedSet
				into *[]Set
			}{
				{planned.Main(), &day.Main},
				{planned.Variation(), &day.Variation},
				{planned.Accessories(), &day.Accessories},
			} {
				for _, set := range lane.sets {
					day.TotalSets += set.Sets().Int()

					// 処方の重量が無い回（履歴の無い初回）は本人が選ぶ。
					e := byID[set.ExerciseID()]
					weightKg := who.firstWeight(e, date)
					if kg, ok := set.Weight(); ok {
						weightKg = kg.Kg()
					}
					did := who.perform(e, date, weightKg, set.TargetRIR().Int())

					described := s.describe(set, e, estimable, date, who.bodyLoad(e))
					described.Performed = did
					described.AthleteOneRepMaxKg = who.strength(e, date) - who.bodyLoad(e)
					*lane.into = append(*lane.into, described)

					// 1セットずつ積む。まとめて1件にすると、残差が
					// 「1セットしかやっていない」と見て補助が増える。
					asLoad := did
					asLoad.WeightKg += who.bodyLoad(e)
					for range set.Sets().Int() {
						if l := s.log(&n, date, set.ExerciseID(), did); l != nil {
							logs = append(logs, l)
						}
						if l := s.log(&n, date, set.ExerciseID(), asLoad); l != nil {
							effective = append(effective, l)
						}
					}
					addStimulus(done, byID[set.ExerciseID()], set.Sets().Int())
				}
			}
			out.Days = append(out.Days, day)
		}

		for _, r := range target.Regions() {
			week.Regions = append(week.Regions, RegionVolume{
				Region: r, Target: target.Sets(r), Done: done[r],
			})
		}
		out.Weeks = append(out.Weeks, week)
	}
	return out, nil
}

// buildProgram は入力からプログラムを組み立てる。
func (s *Simulator) buildProgram(req Request, pool []*exercise.Exercise) (*program.Program, error) {
	freq, err := program.NewFrequency(req.Frequency)
	if err != nil {
		return nil, fmt.Errorf("頻度が不正: %w", err)
	}
	volume, err := program.NewSessionVolume(req.ExercisesPerSession, req.SetsPerExercise)
	if err != nil {
		return nil, fmt.Errorf("1回の量が不正: %w", err)
	}

	// 使う種目は全件。外したときの挙動を見たいときは宣言と重点種目で足りる。
	selected := make([]exercise.ExerciseID, 0, len(pool))
	for _, e := range pool {
		selected = append(selected, e.ID())
	}

	prog, err := program.NewProgram(freq, volume, selected, req.Declared, req.Focus)
	if err != nil {
		return nil, fmt.Errorf("プログラムが不正: %w", err)
	}
	if req.SplitKey == "" {
		return prog, nil
	}

	for _, p := range s.presets {
		if p.Key != req.SplitKey {
			continue
		}
		prog, err = prog.WithCycle(p.Cycle)
		if err != nil {
			return nil, fmt.Errorf("分割が不正: %w", err)
		}
		return prog, nil
	}
	return nil, fmt.Errorf("分割プリセットが見つからない: %s", req.SplitKey)
}

// splitNameOn はその日の分割の名前。分割なしなら空。
func (s *Simulator) splitNameOn(
	prog *program.Program, history setlog.History, date training.Date,
) string {
	split, ok := prog.SplitOn(history.Before(date).SessionCount())
	if !ok {
		return ""
	}
	return split.Name()
}

// describe は処方を画面に出す形に直す。
//
// estimable は体重込みの負荷の履歴。推定比は、処方（加重）に体重の分
// （bodyLoad）を足して、体重込みの推定1RMで割る。プランナーと同じ物差し。
func (s *Simulator) describe(
	set planning.PlannedSet, e *exercise.Exercise,
	estimable setlog.History, date training.Date, bodyLoad float64,
) Set {
	out := Set{
		ExerciseID: set.ExerciseID(),
		Name:       e.Name(),
		Sets:       set.Sets().Int(),
		TargetRIR:  set.TargetRIR().Int(),
	}
	w, ok := set.Weight()
	if !ok {
		return out
	}
	out.WeightKg, out.HasWeight = w.Kg(), true

	if orm, ok := s.estimator.Estimate(estimable, set.ExerciseID(), date); ok && orm.Kg() > 0 {
		out.PctOfOneRM = (w.Kg() + bodyLoad) / orm.Kg()
	}
	return out
}

// log は模擬ユーザーの記録を1セットぶん作る。
func (s *Simulator) log(n *int, date training.Date, id exercise.ExerciseID, did Performed) *setlog.SetLog {
	*n++
	l, err := setlog.NewSetLog(setlog.SetLogParams{
		ID:          fmt.Sprintf("sim-%06d", *n),
		PerformedOn: date,
		ExerciseID:  string(id),
		WeightKg:    did.WeightKg,
		Reps:        did.Reps,
		RIR:         did.RIR,
	})
	if err != nil {
		// 処方と実力から作った値なので、ここは通らない。通ったら記録が
		// 積まれず推定1RMが立たないので、画面が「ずっと未確定」になって気づく。
		return nil
	}
	return l
}

// addStimulus はセット数を筋区分ごとの刺激量に足す。
func addStimulus(into map[training.MuscleRegion]float64, e *exercise.Exercise, sets int) {
	if e == nil {
		return
	}
	for _, r := range e.Stimulus().Regions() {
		c, ok := e.Stimulus().Contribution(r)
		if !ok {
			continue
		}
		into[r] += c.Float() * float64(sets)
	}
}
