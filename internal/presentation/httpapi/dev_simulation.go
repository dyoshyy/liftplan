package httpapi

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/dyoshyy/liftplan/internal/application/devsim"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
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

type devScheduleDTO struct {
	Exercises int `json:"exercises_per_session"`
	Sets      int `json:"sets_per_exercise"`
	// Weekdays は頻度（"1"〜"7"）ごとの既定の曜日（開始日からの日数）。
	Weekdays map[string][]int `json:"weekdays_by_frequency"`
	Start    string           `json:"start"`
}

type devOptionsDTO struct {
	Exercises []devExerciseDTO `json:"exercises"`
	Presets   []devPresetDTO   `json:"presets"`
	Athlete   devAthleteDTO    `json:"athlete_defaults"`
	Schedule  devScheduleDTO   `json:"schedule_defaults"`
}

type devSetDTO struct {
	ExerciseID string   `json:"exercise_id"`
	Name       string   `json:"name"`
	WeightKg   *float64 `json:"weight_kg"`
	Sets       int      `json:"sets"`
	TargetRIR  int      `json:"target_rir"`
	// TargetReps は目標レップ数。
	TargetReps int `json:"target_reps"`
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
	// Weekdays は通った曜日（開始日からの日数）。指定が無ければ頻度ごとの既定。
	Weekdays  []int `json:"weekdays"`
	Exercises int   `json:"exercises_per_session"`
	Sets      int   `json:"sets_per_exercise"`
	// Custom は足した自分の種目。ID は orm= で1RM を上書きするときに使う。
	Custom []devCustomDTO `json:"custom"`
	// Reps は宣言ごとの軸のレップ数。指定した宣言だけを返す。
	Reps map[string]repTargetsDTO `json:"reps"`
	// Edits は上書きしたプリセット（ID ごと）。上書きしたものだけを返す。
	Edits map[string]devEditDTO `json:"edits"`
	// Unused は使わない種目の ID（ID の順）。
	Unused []string `json:"unused"`
}

type devEditDTO struct {
	Stimulus    map[string]float64 `json:"stimulus"`
	IncrementKg float64            `json:"increment_kg"`
}

type devCustomDTO struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Stimulus    map[string]float64 `json:"stimulus"`
	IncrementKg float64            `json:"increment_kg"`
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
	out.Schedule = devScheduleDTO{
		Exercises: seed.DefaultExercisesPerSession,
		Sets:      seed.DefaultSetsPerExercise,
		Weekdays:  map[string][]int{},
		Start:     devDefaultStart.String(),
	}
	for f := 1; f <= 7; f++ {
		if days, ok := devsim.DefaultWeekdays(f); ok {
			out.Schedule.Weekdays[strconv.Itoa(f)] = days
		}
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
	// 応答の1RM と自分の種目は、足した種目を含む一覧から埋める。Run が
	// 通ったので、ここで一覧が組めないことは無い。
	pool, err := d.sim.PoolFor(req)
	if err != nil {
		respondError(w, invalidInput(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, toDevResultDTO(req, pool, got))
}

// devDefaults は指定が無いときの既定。1ヶ月ぶんを週4で見る。
const (
	devDefaultFrequency = 4
	devDefaultWeeks     = 4
	devMaxWeeks         = 12
)

// devDefaultStart は開始日の既定（月曜）。
//
// 固定の月曜から始めるのは、同じ設定なら同じ結果を出すため。今日から
// 始めると曜日で結果が変わり、画面を見ながらの比較にならない。
var devDefaultStart = training.MustDate(2026, 8, 3)

// devQueryKeys は /api/dev/simulate が読むキー。画面の buildQuery
// （web/src/dev/simulate.ts）が送るキーと同じ顔ぶれ。
var devQueryKeys = map[string]bool{
	"declared": true, "focus": true, "split": true,
	"frequency": true, "weeks": true, "days": true, "start": true,
	"exercises": true, "sets": true,
	"growth": true, "first_pct": true, "body_weight": true, "orm": true,
	"custom": true, "reps": true, "edit": true, "unused": true,
}

func parseDevRequest(r *http.Request) (devsim.Request, error) {
	q := r.URL.Query()

	// 知らないキーと2回指定は弾く。読み飛ばすと、書き間違えたキー
	// （growt=0）は既定値（伸び 0.5%/週）のまま 200 で走り、別の条件の
	// 結果を読むことになる。2回指定は q.Get が先頭だけを取り、後ろが消える。
	// 複数あるときに毎回同じキーを名指しするよう、キーの順で見る。
	for _, key := range slices.Sorted(maps.Keys(q)) {
		if !devQueryKeys[key] {
			return devsim.Request{}, fmt.Errorf("クエリ %s は知らないキーである", key)
		}
		if len(q[key]) > 1 {
			return devsim.Request{}, fmt.Errorf("クエリ %s が2回以上指定されている", key)
		}
	}

	out := devsim.Request{
		Focus:     exercise.ExerciseID(q.Get("focus")),
		SplitKey:  q.Get("split"),
		Frequency: devDefaultFrequency,
		Weeks:     devDefaultWeeks,
		Start:     devDefaultStart,
		Athlete:   devsim.DefaultAthlete(),

		ExercisesPerSession: seed.DefaultExercisesPerSession,
		SetsPerExercise:     seed.DefaultSetsPerExercise,
	}
	if v := q.Get("start"); q.Has("start") {
		d, err := training.ParseDate(v)
		if err != nil {
			return devsim.Request{}, errDevQuery("start", v)
		}
		out.Start = d
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

	for _, f := range []struct {
		name string
		into *int
	}{
		{"exercises", &out.ExercisesPerSession},
		{"sets", &out.SetsPerExercise},
	} {
		if !q.Has(f.name) {
			continue
		}
		v := q.Get(f.name)
		n, err := strconv.Atoi(v)
		if err != nil {
			return devsim.Request{}, errDevQuery(f.name, v)
		}
		*f.into = n
	}

	// 曜日を指定したら、頻度はその数。frequency も渡されて数が違えば、
	// どちらかが書き間違いなので devsim がエラーにする。
	if v := q.Get("days"); q.Has("days") {
		for _, part := range strings.Split(v, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil {
				return devsim.Request{}, errDevQuery("days", v)
			}
			out.Weekdays = append(out.Weekdays, n)
		}
		if !q.Has("frequency") {
			out.Frequency = len(out.Weekdays)
		}
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

	custom, err := parseDevCustom(q.Get("custom"))
	if err != nil {
		return devsim.Request{}, err
	}
	out.Custom = custom

	edits, err := parseDevEdits(q.Get("edit"))
	if err != nil {
		return devsim.Request{}, err
	}
	out.Edits = edits
	for _, id := range strings.Split(q.Get("unused"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			out.Unused = append(out.Unused, exercise.ExerciseID(id))
		}
	}

	reps, err := parseDevReps(q.Get("reps"))
	if err != nil {
		return devsim.Request{}, err
	}
	out.Reps = reps
	return out, nil
}

// parseDevCustom は "名前|区分:寄与,区分:寄与|刻み;..." を自分の種目の並びにする。
//
// 効き方は本番の POST /api/exercises と同じ、区分ごとの寄与度の生の値。
// URL の1行で書けるようにした（Claude がクエリで条件を変えて読むため）。
// 寄与の範囲・寄与1.0の区分の有無・名前の重複は devsim（exercise.NewExercise）が見る。
func parseDevCustom(v string) ([]devsim.CustomExercise, error) {
	var out []devsim.CustomExercise
	for _, item := range strings.Split(v, ";") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		fields := strings.Split(item, "|")
		if len(fields) != 3 {
			return nil, errDevQuery("custom", item)
		}
		stimulus, err := parseDevStimulus(fields[1])
		if err != nil {
			// 理由を出さず item だけ返すと、区分の書き間違いと桁の書き間違いが
			// 見分けられない。parseDevStimulus の理由をそのまま本文に出す。
			return nil, errDevQuery("custom", fmt.Sprintf("%s（%s）", item, err))
		}
		inc, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			return nil, errDevQuery("custom", item)
		}
		out = append(out, devsim.CustomExercise{
			Name:        strings.TrimSpace(fields[0]),
			Stimulus:    stimulus,
			IncrementKg: inc,
		})
	}
	return out, nil
}

// parseDevEdits は "ID|区分:寄与,区分:寄与|刻み;..." をプリセットの上書きにする。
//
// custom= と同じ書式で、先頭が名前ではなくプリセットの ID（名前は変えない。
// 本番で種目を直すときも名前はそのまま）。ID がプリセットにあるか・効き方の
// 範囲は devsim（exercise.NewExercise）が見る。同じ ID の2回指定は弾く
// （後ろが黙って勝つと、どちらの条件の結果か分からない）。
func parseDevEdits(v string) (map[exercise.ExerciseID]devsim.EditedExercise, error) {
	out := map[exercise.ExerciseID]devsim.EditedExercise{}
	for _, item := range strings.Split(v, ";") {
		if strings.TrimSpace(item) == "" {
			continue
		}
		fields := strings.Split(item, "|")
		if len(fields) != 3 {
			return nil, errDevQuery("edit", item)
		}
		id := exercise.ExerciseID(strings.TrimSpace(fields[0]))
		if _, dup := out[id]; dup {
			return nil, errDevQuery("edit", fmt.Sprintf("%s（同じ種目の2回指定）", item))
		}
		stimulus, err := parseDevStimulus(fields[1])
		if err != nil {
			return nil, errDevQuery("edit", fmt.Sprintf("%s（%s）", item, err))
		}
		inc, err := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
		if err != nil {
			return nil, errDevQuery("edit", item)
		}
		out[id] = devsim.EditedExercise{Stimulus: stimulus, IncrementKg: inc}
	}
	return out, nil
}

// parseDevStimulus は "TRAP_MID:1,LAT:0.5" を区分ごとの寄与度にする。
// 区分の妥当性と範囲は devsim（exercise.NewExercise）が見るので、ここでは
// 形（コロンの有無・数値として読めるか・同じ区分の2回指定）だけを見る。
func parseDevStimulus(v string) (map[training.MuscleRegion]float64, error) {
	raw := map[string]float64{}
	for _, pair := range strings.Split(v, ",") {
		if pair = strings.TrimSpace(pair); pair == "" {
			continue
		}
		region, kg, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, fmt.Errorf("寄与の形式が不正: %s", pair)
		}
		region = strings.TrimSpace(region)
		// 黙って後勝ちにすると、書き間違い（同じ区分の2回指定）が
		// 「寄与1.0の区分が1つも無い」のような別の理由で弾かれ、
		// 何が悪いのか本文から読めなくなる。
		if _, dup := raw[region]; dup {
			return nil, fmt.Errorf("区分 %s が2回指定されている", region)
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(kg), 64)
		if err != nil {
			return nil, fmt.Errorf("寄与の値が数値でない: %s", pair)
		}
		raw[region] = n
	}
	return exerciseStimulusFrom(raw), nil
}

// parseDevReps は "bench:8:12,pull_up:6:10" を宣言ごとのレップ数にする。
// 範囲は program.NewRepTargets が見る。宣言に含まれるかは devsim が見る。
func parseDevReps(v string) (map[exercise.ExerciseID]program.RepTargets, error) {
	out := map[exercise.ExerciseID]program.RepTargets{}
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		fields := strings.Split(item, ":")
		if len(fields) != 3 {
			return nil, errDevQuery("reps", item)
		}
		heavy, err1 := strconv.Atoi(strings.TrimSpace(fields[1]))
		light, err2 := strconv.Atoi(strings.TrimSpace(fields[2]))
		if err1 != nil || err2 != nil {
			return nil, errDevQuery("reps", item)
		}
		r, err := program.NewRepTargets(heavy, light)
		if err != nil {
			return nil, fmt.Errorf("クエリ reps の %s が不正: %w", item, err)
		}
		out[exercise.ExerciseID(strings.TrimSpace(fields[0]))] = r
	}
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
	out.Exercises, out.Sets = req.ExercisesPerSession, req.SetsPerExercise
	out.Weekdays = append([]int(nil), req.Weekdays...)
	slices.Sort(out.Weekdays)
	if len(out.Weekdays) == 0 {
		out.Weekdays, _ = devsim.DefaultWeekdays(req.Frequency)
	}
	// 自分の種目かどうかは、devsim が並び順で振る ID（CustomExerciseID）で見分ける。
	// IsCustom のような区分は無い（プリセット由来かどうかで扱いを変えないため）。
	customIDs := make(map[exercise.ExerciseID]bool, len(req.Custom))
	for i := range req.Custom {
		customIDs[devsim.CustomExerciseID(i)] = true
	}
	out.Reps = make(map[string]repTargetsDTO, len(req.Reps))
	for id, r := range req.Reps {
		out.Reps[string(id)] = repTargetsDTO{Heavy: r.Heavy(), Light: r.Light()}
	}
	out.Edits = make(map[string]devEditDTO, len(req.Edits))
	for id, ed := range req.Edits {
		stim := make(map[string]float64, len(ed.Stimulus))
		for r, v := range ed.Stimulus {
			stim[string(r)] = v
		}
		out.Edits[string(id)] = devEditDTO{Stimulus: stim, IncrementKg: ed.IncrementKg}
	}
	out.Unused = make([]string, 0, len(req.Unused))
	for _, id := range req.Unused {
		out.Unused = append(out.Unused, string(id))
	}
	slices.Sort(out.Unused)
	out.Custom = []devCustomDTO{}
	for _, e := range pool {
		out.Athlete.OneRepMaxKg[string(e.ID())] = req.Athlete.OneRepMax(e.ID())
		if !customIDs[e.ID()] {
			continue
		}
		c := devCustomDTO{
			ID: string(e.ID()), Name: e.Name(), IncrementKg: e.Increment().Kg(),
			Stimulus: map[string]float64{},
		}
		for _, r := range e.Stimulus().Regions() {
			if v, ok := e.Stimulus().Contribution(r); ok {
				c.Stimulus[string(r)] = v.Float()
			}
		}
		out.Custom = append(out.Custom, c)
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
			TargetReps: s.TargetReps,
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
