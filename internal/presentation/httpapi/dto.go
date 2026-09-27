// Package httpapi は HTTP のプレゼンテーション層。
//
// DTO をドメインモデルとして使わない。JSON の形が変わってもドメインが
// 揺れないよう、この層で必ず変換する。
package httpapi

import (
	"github.com/dyoshyy/liftplan/internal/application/query"
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
	return dto
}

// toPlannedSetDTOs は1レーンぶんの変換。sessionDTO と forecastSessionDTO の
// 両方が使う。ここを分けていないと、レーンごとの変換が2箇所に別々の
// ループとして存在し、片方だけ直して他方を直し忘れる余地ができる。
func toPlannedSetDTOs(sets []planning.PlannedSet) []plannedSetDTO {
	out := make([]plannedSetDTO, 0, len(sets))
	for _, v := range sets {
		out = append(out, toPlannedSetDTO(v))
	}
	return out
}

func toSessionDTO(s planning.PlannedSession) sessionDTO {
	return sessionDTO{
		Date:        s.Date().String(),
		Main:        toPlannedSetDTOs(s.Main()),
		Variation:   toPlannedSetDTOs(s.Variation()),
		Accessories: toPlannedSetDTOs(s.Accessories()),
	}
}

// forecastSessionDTO は見込みの1回ぶん。sessionDTO と違い日付を持たない
// （設計書「日付は返さない」）。index が 0=今日、1=次の回、…
type forecastSessionDTO struct {
	Index       int             `json:"index"`
	Split       *string         `json:"split"`
	Main        []plannedSetDTO `json:"main"`
	Variation   []plannedSetDTO `json:"variation"`
	Accessories []plannedSetDTO `json:"accessories"`
}

type forecastResponse struct {
	Sessions []forecastSessionDTO `json:"sessions"`
}

func toForecastResponse(sessions []planning.PlannedSession) forecastResponse {
	out := make([]forecastSessionDTO, 0, len(sessions))
	for i, s := range sessions {
		var split *string
		if sp, ok := s.Split(); ok {
			name := sp.Name()
			split = &name
		}
		out = append(out, forecastSessionDTO{
			Index:       i,
			Split:       split,
			Main:        toPlannedSetDTOs(s.Main()),
			Variation:   toPlannedSetDTOs(s.Variation()),
			Accessories: toPlannedSetDTOs(s.Accessories()),
		})
	}
	return forecastResponse{Sessions: out}
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
	// Code は分類の識別子。文言を変えてもクライアントの分岐が壊れない
	// ようにするため、メッセージとは別に載せる。
	Code  string `json:"code"`
	Error string `json:"error"`
}

// programDTO はプログラム設定の出力（GET /api/program の応答）。
//
// 書き込みには使わない。全置換の口は、フィールドを足すたびに写し忘れた
// 設定を黙って消したので無くした（#123）。書くのは1フィールドずつの DTO。
//
// 週目標は含まない。週目標は設定（頻度と1回の量）から導く値であって
// プログラムの持ち物ではなく（D-139）、利用者が編集する項目でもないため、
// 応答に含める理由が無い（#176）。
type programDTO struct {
	PerWeek             int        `json:"per_week"`
	ExercisesPerSession int        `json:"exercises_per_session"`
	SetsPerExercise     int        `json:"sets_per_exercise"`
	Selected            []string   `json:"selected_exercises"`
	Declared            []string   `json:"declared_exercises"`
	Focus               *string    `json:"focus_exercise"`
	Splits              []splitDTO `json:"splits"`
}

// sessionVolumeDTO は1回の量だけの書き込み。
//
// 0 は NewSessionVolume が弾くので、欠落と「0種目」を区別する必要が無い。
// 片方だけ送られても、欠けた側が 0 になって断られる。
type sessionVolumeDTO struct {
	ExercisesPerSession int `json:"exercises_per_session"`
	SetsPerExercise     int `json:"sets_per_exercise"`
}

// focusDTO は重点種目だけの書き込み。
//
// ポインタなのは programDTO.Focus と同じ理由で、null（指定なし）と
// フィールドの欠落を区別するため。欠落は DisallowUnknownFields では
// 弾かれないので、明示的に null を送ってもらう運用にする。
type focusDTO struct {
	Focus *string `json:"focus_exercise"`
}

// declaredDTO は伸ばしたい種目だけの書き込み。
//
// focusDTO と違ってポインタにしないのは、宣言が空のプログラムは存在
// しないため（NewProgram が弾く）。null と欠落を区別する必要が無い。
type declaredDTO struct {
	Declared []string `json:"declared_exercises"`
}

// frequencyDTO は週の頻度だけの書き込み。
//
// 0 は NewFrequency が弾くので、欠落と「0回」を区別する必要が無い。
type frequencyDTO struct {
	PerWeek int `json:"per_week"`
}

// selectedDTO は使う種目だけの書き込み。
type selectedDTO struct {
	Selected []string `json:"selected_exercises"`
}

// splitDTO は分割1件。順序が周期そのものなので、配列の並びに意味がある。
type splitDTO struct {
	Name    string   `json:"name"`
	Regions []string `json:"regions"`
}

// splitCycleDTO は分割の周期だけの書き込み。空なら分割なし。
type splitCycleDTO struct {
	Splits []splitDTO `json:"splits"`
}

type splitPresetDTO struct {
	Key    string     `json:"key"`
	Name   string     `json:"name"`
	Splits []splitDTO `json:"splits"`
	// MinFrequencyPerWeek はこのプリセットを選ぶために必要な週の最小頻度。
	// 0 は下限なし。画面が押す前に選べない理由を出せるよう、選択の可否を
	// サーバーの応答にだけ書く（seed.SplitPreset.MinFrequencyPerWeek）。
	MinFrequencyPerWeek int `json:"min_frequency_per_week"`
}

type splitPresetsResponse struct {
	Presets []splitPresetDTO `json:"presets"`
}

func toSplitDTOs(cycle []program.Split) []splitDTO {
	out := make([]splitDTO, 0, len(cycle))
	for _, s := range cycle {
		regions := make([]string, 0, len(s.Regions()))
		for _, r := range s.Regions() {
			regions = append(regions, string(r))
		}
		out = append(out, splitDTO{Name: s.Name(), Regions: regions})
	}
	return out
}

func toProgramDTO(p *program.Program) programDTO {
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
		PerWeek:             p.Frequency().PerWeek(),
		ExercisesPerSession: p.SessionVolume().Exercises(),
		SetsPerExercise:     p.SessionVolume().Sets(),
		Selected:            selected,
		Declared:            declared,
		Focus:               focus,
		Splits:              toSplitDTOs(p.Cycle()),
	}
}

// --- 読み取り経路の DTO ---

type exerciseDTO struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	IncrementKg float64 `json:"increment_kg"`
	// Stimulus はその種目が各筋区分へ与える刺激。画面が種目の一覧を
	// 部位ごとにまとめるのに使う。どれを代表に選ぶかは表示の判断なので、
	// ここでは分布のまま渡す。
	Stimulus map[string]float64 `json:"stimulus"`
	// Deleted は消した種目か。履歴の名前のために一覧には残す。設定の
	// 一覧には出さない（画面が落とす）。
	Deleted bool `json:"deleted"`
}

// exerciseDTOFrom は query.Exercise から exerciseDTO を作る。
//
// GET /api/exercises（read.go）と POST/PUT /api/exercises（handler.go）の
// 両方がここを通る。同じ変換を2箇所に書くと、どちらかが Deleted の
// 詰め忘れで食い違う。
func exerciseDTOFrom(e query.Exercise) exerciseDTO {
	stimulus := make(map[string]float64, len(e.Stimulus))
	for region, c := range e.Stimulus {
		stimulus[string(region)] = c
	}
	return exerciseDTO{
		ID:          string(e.ID),
		Name:        e.Name,
		IncrementKg: e.IncrementKg,
		Stimulus:    stimulus,
		Deleted:     e.Deleted,
	}
}

type exercisesResponse struct {
	Exercises []exerciseDTO `json:"exercises"`
}

// exerciseInputDTO は種目を足す・直す入力（POST /api/exercises・
// PUT /api/exercises/{id} で共通。設計書「PUT は同じ本文」）。
//
// Stimulus は区分ごとの寄与度の生の値。本人には主・副の2値しか選ばせない
// という決めはドメイン側にも画面側にも置かない。API はドメインが検証する
// 生の分布をそのまま運ぶだけにする。
type exerciseInputDTO struct {
	Name        string             `json:"name"`
	Stimulus    map[string]float64 `json:"stimulus"`
	IncrementKg float64            `json:"increment_kg"`
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
