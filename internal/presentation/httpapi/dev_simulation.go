package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/dyoshyy/liftplan/internal/application/devsim"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// DevSimulation は開発用のシミュレーションの口。
//
// `Handler` に混ぜない。あちらは本番の経路で、こちらは開発中にだけ
// 取り付ける（取り付けは cmd が env で決める）。混ぜると、消すときに
// 本番の配線を触ることになる。
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
}

type devPresetDTO struct {
	Key  string   `json:"key"`
	Name string   `json:"name"`
	Days []string `json:"days"`
}

type devOptionsDTO struct {
	Exercises []devExerciseDTO `json:"exercises"`
	Presets   []devPresetDTO   `json:"presets"`
}

type devSetDTO struct {
	ExerciseID string   `json:"exercise_id"`
	Name       string   `json:"name"`
	WeightKg   *float64 `json:"weight_kg"`
	Sets       int      `json:"sets"`
	TargetRIR  int      `json:"target_rir"`
	// PctOf1RM は推定1RMに対する比。推定が立たない初出の日は null。
	PctOf1RM *float64 `json:"pct_of_1rm"`
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
	Days  []devDayDTO  `json:"days"`
	Weeks []devWeekDTO `json:"weeks"`
}

func (d *DevSimulation) handleOptions(w http.ResponseWriter, _ *http.Request) {
	out := devOptionsDTO{}
	for _, e := range d.sim.Pool() {
		dto := devExerciseDTO{ID: string(e.ID()), Name: e.Name()}
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
	writeJSON(w, http.StatusOK, out)
}

func (d *DevSimulation) handleSimulate(w http.ResponseWriter, r *http.Request) {
	req, err := parseDevRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	got, err := d.sim.Run(req)
	if err != nil {
		// 入力の組み合わせが成り立たないことは開発中の日常なので、
		// 500 ではなく 400 で理由をそのまま返す。
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDevResultDTO(got))
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

func toDevResultDTO(in devsim.Result) devResultDTO {
	out := devResultDTO{
		Days:  make([]devDayDTO, 0, len(in.Days)),
		Weeks: make([]devWeekDTO, 0, len(in.Weeks)),
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

func toDevSetDTOs(in []devsim.Set) []devSetDTO {
	out := make([]devSetDTO, 0, len(in))
	for _, s := range in {
		dto := devSetDTO{
			ExerciseID: string(s.ExerciseID),
			Name:       s.Name,
			Sets:       s.Sets,
			TargetRIR:  s.TargetRIR,
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
