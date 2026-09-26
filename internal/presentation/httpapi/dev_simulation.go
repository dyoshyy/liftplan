package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/dyoshyy/liftplan/internal/application/devsim"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// DevSimulation は設定を変えたときに計画がどう変わるかを見る口。
//
// 本番にも生やす。認証の内側に入るので、叩けるのはログイン済みの人だけ
// （取り付けは cmd の mountSimulation 1箇所）。
//
// `Handler` に混ぜない。あちらは記録と設定を扱う経路で、こちらは捏造した
// 設定から計画を作るだけ。保存先も利用者の記録も触らない。混ぜると、
// 消すときに本番の配線を触ることになる。
type DevSimulation struct {
	sim *devsim.Simulator
}

func NewDevSimulation(sim *devsim.Simulator) *DevSimulation {
	return &DevSimulation{sim: sim}
}

// DevSimulatePath は計画を作る経路。
const DevSimulatePath = "GET /api/dev/simulate"

// DevOptionsPath は画面が選択肢を取る経路。
const DevOptionsPath = "GET /api/dev/options"

// Mount は開発用の経路を mux に足す。
func (d *DevSimulation) Mount(mux *http.ServeMux) {
	mux.HandleFunc(DevSimulatePath, d.handleSimulate)
	mux.HandleFunc(DevOptionsPath, d.handleOptions)
}

type devExerciseDTO struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Derived string `json:"derived_from,omitempty"`
	// DefaultOneRepMax は模擬ユーザーの初日の実力の既定値（加重の1RM）。
	DefaultOneRepMax float64 `json:"default_1rm_kg"`
	// Bodyweight は自重が負荷に乗る種目か。1RM は加重の分だけで表す。
	Bodyweight bool `json:"bodyweight"`
}

type devPresetDTO struct {
	Key  string   `json:"key"`
	Name string   `json:"name"`
	Days []string `json:"days"`
}

type devAthleteDTO struct {
	GrowthPctPerWeek float64 `json:"growth_pct_per_week"`
	FirstSessionPct  float64 `json:"first_session_pct"`
	BodyWeightKg     float64 `json:"body_weight_kg"`
	// OneRepMaxKg は種目ごとの初日の実力。応答では全種目を既定値で埋めて返す。
	OneRepMaxKg map[string]float64 `json:"one_rep_max_kg,omitempty"`
}

type devOptionsDTO struct {
	Exercises []devExerciseDTO `json:"exercises"`
	Presets   []devPresetDTO   `json:"presets"`
	Athlete   devAthleteDTO    `json:"athlete_defaults"`
}

type devSetDTO struct {
	ExerciseID string   `json:"exercise_id"`
	Name       string   `json:"name"`
	WeightKg   *float64 `json:"weight_kg"`
	Sets       int      `json:"sets"`
	TargetRIR  int      `json:"target_rir"`
	// PctOf1RM は推定1RMに対する比。推定が立たない初出の日は null。
	PctOf1RM *float64 `json:"pct_of_1rm"`
	// Athlete1RM はその日の模擬ユーザーの実力（加重の1RM）。
	Athlete1RM float64 `json:"athlete_1rm_kg"`
	// Performed は模擬ユーザーが記録した値（全セット同じ）。
	Performed devPerformedDTO `json:"performed"`
}

type devPerformedDTO struct {
	WeightKg float64 `json:"weight_kg"`
	Reps     int     `json:"reps"`
	RIR      int     `json:"rir"`
}

// devSettingsDTO は結果を作った設定。既定値を解決したあとの値を返し、
// 応答だけでどの仮定から出た数字かが分かるようにする。
type devSettingsDTO struct {
	Declared  []string      `json:"declared"`
	Focus     string        `json:"focus"`
	Split     string        `json:"split"`
	Frequency int           `json:"frequency"`
	Weeks     int           `json:"weeks"`
	Start     string        `json:"start"`
	Athlete   devAthleteDTO `json:"athlete"`
}

type devDayDTO struct {
	Date        string      `json:"date"`
	Split       string      `json:"split"`
	TotalSets   int         `json:"total_sets"`
	Main        []devSetDTO `json:"main"`
	Variation   []devSetDTO `json:"variation"`
	Accessories []devSetDTO `json:"accessories"`
}

type devRegionDTO struct {
	Region string  `json:"region"`
	Target float64 `json:"target"`
	Done   float64 `json:"done"`
}

type devWeekDTO struct {
	Index   int            `json:"index"`
	Regions []devRegionDTO `json:"regions"`
}

type devResultDTO struct {
	Settings devSettingsDTO `json:"settings"`
	Days     []devDayDTO    `json:"days"`
	Weeks    []devWeekDTO   `json:"weeks"`
}

func (d *DevSimulation) handleOptions(w http.ResponseWriter, _ *http.Request) {
	out := devOptionsDTO{}
	for _, e := range d.sim.Pool() {
		dto := devExerciseDTO{
			ID: string(e.ID()), Name: e.Name(),
			DefaultOneRepMax: devsim.DefaultOneRepMax(e.ID()),
			Bodyweight:       e.BodyweightFactor().Float() > 0,
		}
		if from, ok := e.DerivedFrom(); ok {
			dto.Derived = string(from)
		}
		out.Exercises = append(out.Exercises, dto)
	}
	for _, p := range d.sim.Presets() {
		dto := devPresetDTO{Key: p.Key, Name: p.Name}
		for _, day := range p.Cycle {
			dto.Days = append(dto.Days, day.Name())
		}
		out.Presets = append(out.Presets, dto)
	}
	a := devsim.DefaultAthlete()
	out.Athlete = devAthleteDTO{
		GrowthPctPerWeek: a.GrowthPctPerWeek,
		FirstSessionPct:  a.FirstSessionPct,
		BodyWeightKg:     a.BodyWeightKg,
	}
	writeJSON(w, http.StatusOK, out)
}

func (d *DevSimulation) handleSimulate(w http.ResponseWriter, r *http.Request) {
	req, err := parseDevRequest(r)
	if err != nil {
		respondError(w, invalidInput(err.Error()))
		return
	}

	got, err := d.sim.Run(req)
	if err != nil {
		// 入力の組み合わせが成り立たないことは開発中の日常なので、
		// 500 ではなく 400 で理由をそのまま返す。
		respondError(w, invalidInput(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, toDevResultDTO(req, d.sim.Pool(), got))
}

// devDefaults は指定が無いときの既定。1ヶ月ぶんを週4で見る。
const (
	devDefaultFrequency = 4
	devDefaultWeeks     = 4
	devMaxWeeks         = 12
)

func parseDevRequest(r *http.Request) (devsim.Request, error) {
	q := r.URL.Query()

	out := devsim.Request{
		Focus:     exercise.ExerciseID(q.Get("focus")),
		SplitKey:  q.Get("split"),
		Frequency: devDefaultFrequency,
		Weeks:     devDefaultWeeks,
		Start:     devStartDate(q.Get("start")),
		Athlete:   devsim.DefaultAthlete(),
	}

	for _, id := range strings.Split(q.Get("declared"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			out.Declared = append(out.Declared, exercise.ExerciseID(id))
		}
	}

	if v := q.Get("frequency"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return devsim.Request{}, errDevQuery("frequency", v)
		}
		out.Frequency = n
	}
	if v := q.Get("weeks"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > devMaxWeeks {
			return devsim.Request{}, errDevQuery("weeks", v)
		}
		out.Weeks = n
	}

	// 模擬ユーザー。形だけ見て、範囲は devsim が見る（400 の理由も向こうが書く）。
	for _, f := range []struct {
		name string
		into *float64
	}{
		{"growth", &out.Athlete.GrowthPctPerWeek},
		{"first_pct", &out.Athlete.FirstSessionPct},
		{"body_weight", &out.Athlete.BodyWeightKg},
	} {
		if !q.Has(f.name) {
			continue
		}
		v := q.Get(f.name)
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return devsim.Request{}, errDevQuery(f.name, v)
		}
		*f.into = n
	}
	orm, err := parseDevOneRepMax(q.Get("orm"))
	if err != nil {
		return devsim.Request{}, err
	}
	out.Athlete.OneRepMaxKg = orm
	return out, nil
}

// parseDevOneRepMax は "bench:100,squat:140" を種目ごとの1RMにする。
func parseDevOneRepMax(v string) (map[exercise.ExerciseID]float64, error) {
	out := map[exercise.ExerciseID]float64{}
	for _, pair := range strings.Split(v, ",") {
		if pair = strings.TrimSpace(pair); pair == "" {
			continue
		}
		// ":" が無ければ kg が空になり、数値の変換で落ちる。
		id, kg, _ := strings.Cut(pair, ":")
		n, err := strconv.ParseFloat(strings.TrimSpace(kg), 64)
		if err != nil {
			return nil, errDevQuery("orm", pair)
		}
		out[exercise.ExerciseID(strings.TrimSpace(id))] = n
	}
	return out, nil
}

// devStartDate は開始日。指定が無ければ 2026-08-03（月曜）。
//
// 固定の月曜から始めるのは、同じ設定なら同じ結果を出すため。今日から
// 始めると曜日で結果が変わり、画面を見ながらの比較にならない。
func devStartDate(v string) training.Date {
	if d, err := training.ParseDate(v); err == nil {
		return d
	}
	return training.MustDate(2026, 8, 3)
}

type devQueryError struct{ name, value string }

func (e devQueryError) Error() string {
	return "クエリ " + e.name + " が不正である: " + e.value
}

func errDevQuery(name, value string) error { return devQueryError{name: name, value: value} }

func toDevResultDTO(req devsim.Request, pool []*exercise.Exercise, in devsim.Result) devResultDTO {
	out := devResultDTO{
		Settings: toDevSettingsDTO(req, pool),
		Days:     make([]devDayDTO, 0, len(in.Days)),
		Weeks:    make([]devWeekDTO, 0, len(in.Weeks)),
	}
	for _, d := range in.Days {
		out.Days = append(out.Days, devDayDTO{
			Date:        d.Date.String(),
			Split:       d.SplitName,
			TotalSets:   d.TotalSets,
			Main:        toDevSetDTOs(d.Main),
			Variation:   toDevSetDTOs(d.Variation),
			Accessories: toDevSetDTOs(d.Accessories),
		})
	}
	for _, w := range in.Weeks {
		week := devWeekDTO{Index: w.Index, Regions: make([]devRegionDTO, 0, len(w.Regions))}
		for _, r := range w.Regions {
			week.Regions = append(week.Regions, devRegionDTO{
				Region: string(r.Region), Target: r.Target, Done: r.Done,
			})
		}
		out.Weeks = append(out.Weeks, week)
	}
	return out
}

func toDevSettingsDTO(req devsim.Request, pool []*exercise.Exercise) devSettingsDTO {
	out := devSettingsDTO{
		Declared:  make([]string, 0, len(req.Declared)),
		Focus:     string(req.Focus),
		Split:     req.SplitKey,
		Frequency: req.Frequency,
		Weeks:     req.Weeks,
		Start:     req.Start.String(),
		Athlete: devAthleteDTO{
			GrowthPctPerWeek: req.Athlete.GrowthPctPerWeek,
			FirstSessionPct:  req.Athlete.FirstSessionPct,
			BodyWeightKg:     req.Athlete.BodyWeightKg,
			OneRepMaxKg:      make(map[string]float64, len(pool)),
		},
	}
	for _, id := range req.Declared {
		out.Declared = append(out.Declared, string(id))
	}
	for _, e := range pool {
		out.Athlete.OneRepMaxKg[string(e.ID())] = req.Athlete.OneRepMax(e.ID())
	}
	return out
}

func toDevSetDTOs(in []devsim.Set) []devSetDTO {
	out := make([]devSetDTO, 0, len(in))
	for _, s := range in {
		dto := devSetDTO{
			ExerciseID: string(s.ExerciseID),
			Name:       s.Name,
			Sets:       s.Sets,
			TargetRIR:  s.TargetRIR,
			Athlete1RM: s.AthleteOneRepMaxKg,
			Performed: devPerformedDTO{
				WeightKg: s.Performed.WeightKg, Reps: s.Performed.Reps, RIR: s.Performed.RIR,
			},
		}
		if s.HasWeight {
			kg := s.WeightKg
			dto.WeightKg = &kg
		}
		if s.PctOfOneRM > 0 {
			pct := s.PctOfOneRM
			dto.PctOf1RM = &pct
		}
		out = append(out, dto)
	}
	return out
}
