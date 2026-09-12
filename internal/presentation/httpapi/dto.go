// Package httpapi は HTTP のプレゼンテーション層。
//
// DTO をドメインモデルとして使わない。JSON の形が変わってもドメインが
// 揺れないよう、この層で必ず変換する。
package httpapi

import (
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// plannedSetDTO の WeightKg が null になるのはバグではなく仕様。
// 履歴が足りず重量を推定できない場合で、クライアントは
// 「初回だけ自分で決める」UI を出す。
type plannedSetDTO struct {
	ExerciseID string   `json:"exercise_id"`
	WeightKg   *float64 `json:"weight_kg"`
	Sets       int      `json:"sets"`
	TargetRIR  int      `json:"target_rir"`
	Intent     string   `json:"intent,omitempty"`
}

// 3レーンとも配列にする。バリエーションは高々1件だが、クライアントが
// 同じコードで描けるほうがよい。出ない日は null ではなく空配列。
type sessionDTO struct {
	Date        string          `json:"date"`
	Main        []plannedSetDTO `json:"main"`
	Variation   []plannedSetDTO `json:"variation"`
	Accessories []plannedSetDTO `json:"accessories"`
}

func toPlannedSetDTO(s planning.PlannedSet) plannedSetDTO {
	dto := plannedSetDTO{
		ExerciseID: string(s.ExerciseID()),
		Sets:       s.Sets().Int(),
		TargetRIR:  s.TargetRIR().Int(),
	}
	if w, ok := s.Weight(); ok {
		kg := w.Kg()
		dto.WeightKg = &kg
	}
	if intent, ok := s.Intent(); ok {
		dto.Intent = string(intent)
	}
	return dto
}

func toSessionDTO(s planning.PlannedSession) sessionDTO {
	main := make([]plannedSetDTO, 0, len(s.Main()))
	for _, v := range s.Main() {
		main = append(main, toPlannedSetDTO(v))
	}
	variation := make([]plannedSetDTO, 0, len(s.Variation()))
	for _, v := range s.Variation() {
		variation = append(variation, toPlannedSetDTO(v))
	}
	accessories := make([]plannedSetDTO, 0, len(s.Accessories()))
	for _, v := range s.Accessories() {
		accessories = append(accessories, toPlannedSetDTO(v))
	}

	out := sessionDTO{
		Date:        s.Date().String(),
		Main:        main,
		Variation:   variation,
		Accessories: accessories,
	}
	return out
}

// ポインタなのは、フィールドの欠落を検出するため。
//
// 非ポインタだと weight_kg の欠落が 0kg（正当な自重セット）になり、
// rir の欠落が RIR 0（限界まで追い込んだ）になる。どちらも有意味な値なので、
// 「送られなかった」と区別できない。
// RIR は毎セットを1RM測定に変えるための必須情報であり、欠落を黙って
// 0 と解釈すると推定1RMが実態より低くなる。
type setLogDTO struct {
	ID         string   `json:"id"`
	Date       string   `json:"date"`
	ExerciseID string   `json:"exercise_id"`
	WeightKg   *float64 `json:"weight_kg"`
	Reps       *int     `json:"reps"`
	RIR        *int     `json:"rir"`
}

type setLogsRequest struct {
	Logs []setLogDTO `json:"logs"`
}

type conditionDTO struct {
	Date         string   `json:"date"`
	BodyWeightKg *float64 `json:"body_weight_kg"`
	SleepHours   *float64 `json:"sleep_hours"`
}

type conditionsRequest struct {
	Conditions []conditionDTO `json:"conditions"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// programDTO はプログラム設定の入出力。
//
// 週目標をマップで受けるのは、区分ごとに独立して調整するため。
// 配列だと順序に意味が生まれ、区分の追加でクライアントが壊れる。
type programDTO struct {
	PerWeek  int                `json:"per_week"`
	Target   map[string]float64 `json:"weekly_target"`
	Selected []string           `json:"selected_exercises"`
	Declared []string           `json:"declared_exercises"`
	Focus    *string            `json:"focus_exercise"`
}

func toProgramDTO(p *program.Program) programDTO {
	target := map[string]float64{}
	for _, r := range p.WeeklyTarget().Regions() {
		target[string(r)] = p.WeeklyTarget().Sets(r)
	}

	selected := make([]string, 0)
	for _, id := range p.SelectedExercises() {
		selected = append(selected, string(id))
	}

	declared := make([]string, 0)
	for _, id := range p.DeclaredExercises() {
		declared = append(declared, string(id))
	}

	// ポインタなのは weight_kg と同じ理由。非ポインタだと「指定なし」と
	// フィールドの欠落がどちらも空文字になり、区別できない。
	var focus *string
	if id, ok := p.FocusExercise(); ok {
		s := string(id)
		focus = &s
	}

	return programDTO{
		PerWeek:  p.Frequency().PerWeek(),
		Target:   target,
		Selected: selected,
		Declared: declared,
		Focus:    focus,
	}
}

func (d programDTO) toInput() usecase.ConfigureProgramInput {
	target := make(map[training.MuscleRegion]float64, len(d.Target))
	for k, v := range d.Target {
		target[training.MuscleRegion(k)] = v
	}
	selected := make([]exercise.ExerciseID, 0, len(d.Selected))
	for _, id := range d.Selected {
		selected = append(selected, exercise.ExerciseID(id))
	}
	declared := make([]exercise.ExerciseID, 0, len(d.Declared))
	for _, id := range d.Declared {
		declared = append(declared, exercise.ExerciseID(id))
	}
	var focus exercise.ExerciseID
	if d.Focus != nil {
		focus = exercise.ExerciseID(*d.Focus)
	}
	return usecase.ConfigureProgramInput{
		PerWeek: d.PerWeek, Target: target, Selected: selected, Declared: declared,
		Focus: focus,
	}
}

// --- 読み取り経路の DTO ---

type exerciseDTO struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	IncrementKg float64 `json:"increment_kg"`
}

type exercisesResponse struct {
	Exercises []exerciseDTO `json:"exercises"`
}

type setDTO struct {
	ID       string  `json:"id"`
	WeightKg float64 `json:"weight_kg"`
	Reps     int     `json:"reps"`
	RIR      int     `json:"rir"`
}

type exerciseLogDTO struct {
	ExerciseID string   `json:"exercise_id"`
	Name       string   `json:"name"`
	Sets       []setDTO `json:"sets"`
}

type dayDTO struct {
	Date      string           `json:"date"`
	Exercises []exerciseLogDTO `json:"exercises"`
	TotalSets int              `json:"total_sets"`
}

// lastDTO は種目ごとの直近の実績。
// 今日提示された重量を信じる根拠になる。
type lastDTO struct {
	Date     string    `json:"date"`
	WeightKg float64   `json:"weight_kg"`
	Weights  []float64 `json:"weights"`
	Reps     []int     `json:"reps"`
	DaysAgo  int       `json:"days_ago"`
}

type setLogsResponse struct {
	From string             `json:"from"`
	To   string             `json:"to"`
	Days []dayDTO           `json:"days"`
	Last map[string]lastDTO `json:"last_performances"`
}

type pointDTO struct {
	Date string  `json:"date"`
	Kg   float64 `json:"kg"`
}

type trendDTO struct {
	ExerciseID string     `json:"exercise_id"`
	Name       string     `json:"name"`
	Points     []pointDTO `json:"points"`
	CurrentKg  float64    `json:"current_kg"`
	ChangeKg   float64    `json:"change_kg"`
}

type volumeDTO struct {
	Region     string  `json:"region"`
	TargetSets float64 `json:"target_sets"`
	DoneSets   float64 `json:"done_sets"`
}

type statsResponse struct {
	From         string      `json:"from"`
	To           string      `json:"to"`
	Trends       []trendDTO  `json:"trends"`
	WeeklyVolume []volumeDTO `json:"weekly_volume"`
}
