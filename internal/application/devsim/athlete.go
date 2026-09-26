package devsim

import (
	"fmt"
	"math"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// AthleteParams は模擬ユーザー（処方をこなす本人）の設定。
//
// シミュレーションの結果はこの仮定でほぼ決まるので、全部を画面から
// 変えられるようにしてある。本番の設定画面と違い、ここは挙動を観察する
// 道具なので、設定が増えることを避けない。
type AthleteParams struct {
	// GrowthPctPerWeek は実力の伸び（%/週）。負なら落ちていく。
	GrowthPctPerWeek float64
	// FirstSessionPct は履歴が無い回に本人が選ぶ重さ（実力の%）。
	FirstSessionPct float64
	// BodyWeightKg は体重。自重種目の負荷に乗る。
	BodyWeightKg float64
	// OneRepMaxKg は種目ごとの初日の実力（1RM）の上書き。無い種目は既定値。
	// 自重種目は加重の分だけ（チンニング +25kg なら 25）。
	OneRepMaxKg map[exercise.ExerciseID]float64
}

// 模擬ユーザーの設定が取りうる範囲。外れた値は画面に 400 で返す。
const (
	maxGrowthPctPerWeek = 10
	minFirstSessionPct  = 10
	maxFirstSessionPct  = 100
	minBodyWeightKg     = 30
	maxBodyWeightKg     = 250
	maxOneRepMaxKg      = 1000
)

// DefaultAthlete は画面を開いたときの模擬ユーザー。
//
// 伸び 0.5%/週は、ベンチ100kgが12週で約106kgになる幅。中級者が
// 普通に伸びる速さとして置いた。
func DefaultAthlete() AthleteParams {
	return AthleteParams{
		GrowthPctPerWeek: 0.5,
		FirstSessionPct:  70,
		BodyWeightKg:     75,
	}
}

// OneRepMax は初日の実力（加重の1RM）。上書きが無ければ既定値。
func (a AthleteParams) OneRepMax(id exercise.ExerciseID) float64 {
	if v, ok := a.OneRepMaxKg[id]; ok {
		return v
	}
	return DefaultOneRepMax(id)
}

// validate は設定が成り立つかを見る。pool は種目の実在と自重かどうかを引く。
func (a AthleteParams) validate(pool map[exercise.ExerciseID]*exercise.Exercise) error {
	if math.IsNaN(a.GrowthPctPerWeek) || math.Abs(a.GrowthPctPerWeek) > maxGrowthPctPerWeek {
		return fmt.Errorf("伸び率は ±%d%%/週 の範囲である必要がある: %v", maxGrowthPctPerWeek, a.GrowthPctPerWeek)
	}
	if !(a.FirstSessionPct >= minFirstSessionPct && a.FirstSessionPct <= maxFirstSessionPct) {
		return fmt.Errorf("初回の重さは実力の %d〜%d%% である必要がある: %v",
			minFirstSessionPct, maxFirstSessionPct, a.FirstSessionPct)
	}
	if !(a.BodyWeightKg >= minBodyWeightKg && a.BodyWeightKg <= maxBodyWeightKg) {
		return fmt.Errorf("体重は %d〜%dkg である必要がある: %v", minBodyWeightKg, maxBodyWeightKg, a.BodyWeightKg)
	}
	for id, kg := range a.OneRepMaxKg {
		e, ok := pool[id]
		if !ok {
			return fmt.Errorf("1RM を指定した種目が無い: %s", id)
		}
		// 自重種目は加重0（自重だけで1回）がありうる。それ以外の0は
		// 何も挙がらない人になり、記録が作れない。
		min := 0.0
		if e.BodyweightFactor().Float() == 0 {
			min = math.SmallestNonzeroFloat64
		}
		if math.IsNaN(kg) || kg < min || kg > maxOneRepMaxKg {
			return fmt.Errorf("%s の1RMが範囲外である: %v", id, kg)
		}
	}
	return nil
}

// Performed は模擬ユーザーが記録した1セット。重量は加重（記録のまま）。
type Performed struct {
	WeightKg float64
	Reps     int
	RIR      int
}

// athlete は設定を実行用に解いたもの。
type athlete struct {
	params AthleteParams
	start  training.Date
}

// bodyLoad はその種目で負荷に乗る体重の分。
func (a athlete) bodyLoad(e *exercise.Exercise) float64 {
	return e.BodyweightFactor().Float() * a.params.BodyWeightKg
}

// strength はその日の実力。体重込みの負荷で表す（推定1RMと同じ物差し）。
//
// 伸びは体重込みの実力に掛ける。加重に掛けると、加重0のチンニングは
// いつまでも伸びない。
func (a athlete) strength(e *exercise.Exercise, on training.Date) float64 {
	weeks := float64(on.DaysSince(a.start)) / 7
	base := a.params.OneRepMax(e.ID()) + a.bodyLoad(e)
	return base * math.Pow(1+a.params.GrowthPctPerWeek/100, weeks)
}

// firstWeight は履歴が無い回に本人が選ぶ加重。刻みに丸め、負にしない。
func (a athlete) firstWeight(e *exercise.Exercise, on training.Date) float64 {
	load := a.strength(e, on) * a.params.FirstSessionPct / 100
	inc := e.Increment().Kg()
	return math.Max(0, math.Round((load-a.bodyLoad(e))/inc)*inc)
}

// maxRepsToFailure は記録する「限界までのレップ数」の上限。推定器が
// これを超える記録を捨てる（training.maxRepsToFailure と同じ値）。
const maxRepsToFailure = 20

// perform はその日の実力で、加重 weightKg を目標 RIR ちょうどで止めた記録。
//
// レップ数は Epley を逆に解いて決める。整数に丸めるので、実力をぴったり
// 保つ記録は一般に無い。round は最も近いものを取り、推定を上にも下にも
// 寄せない（planning の steadyReps と同じ定義）。
//
//	reps + RIR = round(30 × (実力 / 負荷 − 1))
//
// 範囲の外は2つ。重すぎて目標 RIR を残せないなら、挙がるだけ挙げて
// RIR0 で記録する（1回も挙がらない重さでも1回とする。0回の記録は作れない）。
// 軽すぎて限界まで20回を超えるなら、20回で頭打ちにする。推定は
// 実力より低く出るが、実際の人も軽すぎる重さで限界は測らない。
func (a athlete) perform(e *exercise.Exercise, on training.Date, weightKg float64, rir int) Performed {
	load := weightKg + a.bodyLoad(e)
	toFailure := int(math.Round(30 * (a.strength(e, on)/load - 1)))
	toFailure = min(toFailure, maxRepsToFailure)

	if toFailure-rir < 1 {
		return Performed{WeightKg: weightKg, Reps: max(toFailure, 1), RIR: 0}
	}
	return Performed{WeightKg: weightKg, Reps: toFailure - rir, RIR: rir}
}

// DefaultOneRepMax は種目ごとの初日の実力（加重の1RM）の既定値。
//
// 体重75kgの中級者を想定した目安で、正確さは狙っていない。画面で
// 上書きできる。自重種目は加重の分だけ。知らない種目は 50kg。
//
// 週目標の充足（seed の通し検証が見ているもの）はセット数の話で、
// 重量には依存しない。ここを変えても通し検証の数字は動かない。
func DefaultOneRepMax(id exercise.ExerciseID) float64 {
	if v, ok := defaultOneRepMax[id]; ok {
		return v
	}
	return 50
}

var defaultOneRepMax = map[exercise.ExerciseID]float64{
	"squat":    140,
	"bench":    100,
	"deadlift": 180,

	"larsen_press":      90,
	"tempo_bench":       85,
	"close_grip_bench":  90,
	"pause_squat":       120,
	"front_squat":       110,
	"deficit_deadlift":  160,
	"romanian_deadlift": 140,

	"incline_db_press":      30,
	"incline_barbell_press": 85,
	"decline_press":         105,
	"pec_fly":               60,
	"dip":                   40,

	"lat_pulldown":   80,
	"pull_up":        25,
	"barbell_row":    90,
	"seated_row":     80,
	"shrug":          160,
	"back_extension": 20,

	"overhead_press": 60,
	"side_raise":     15,
	"rear_delt_fly":  15,

	"triceps_pushdown":   40,
	"overhead_extension": 30,
	"barbell_curl":       45,
	"hammer_curl":        20,

	"leg_press":        250,
	"leg_extension":    80,
	"leg_curl":         60,
	"hip_thrust":       160,
	"adductor_machine": 80,
	"calf_raise":       150,

	"cable_crunch": 60,
	"side_bend":    40,
}
