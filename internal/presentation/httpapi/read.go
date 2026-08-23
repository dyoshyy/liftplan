package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/application/query"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// defaultHistoryDays は期間を指定しなかったときに遡る日数。
//
// 8週間。デロードの判定が8セッション（D-020）なので、判断に使う窓を
// ひととおり含む長さにしてある。
const defaultHistoryDays = 56

// maxHistoryDays は一度に遡れる上限。
//
// 上限が無いと、3年ぶんを1回で要求されたときに応答が数MBになる。
// 単一ユーザーでも、画面が固まれば使えないことに変わりはない。
const maxHistoryDays = 400

func (h *Handler) handleGetExercises(w http.ResponseWriter, r *http.Request) {
	items, err := h.exercises.All(r.Context())
	if err != nil {
		respondError(w, err)
		return
	}

	out := make([]exerciseDTO, 0, len(items))
	for _, e := range items {
		out = append(out, exerciseDTO{
			ID:          string(e.ID),
			Name:        e.Name,
			Kind:        string(e.Kind),
			IncrementKg: e.IncrementKg,
		})
	}
	writeJSON(w, http.StatusOK, exercisesResponse{Exercises: out})
}

func (h *Handler) handleGetSetLogs(w http.ResponseWriter, r *http.Request) {
	from, to, err := periodOf(r)
	if err != nil {
		respondError(w, err)
		return
	}

	days, err := h.history.Days(r.Context(), from, to)
	if err != nil {
		respondError(w, err)
		return
	}

	// 直近の実績も同じ応答で返す。画面は「今日のメニュー」と並べて
	// 「前回どうだったか」を出すので、別の往復にすると表示が揃わない。
	last, err := h.history.LastPerformances(r.Context(), to)
	if err != nil {
		respondError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, setLogsResponse{
		From: from.String(),
		To:   to.String(),
		Days: toDayDTOs(days),
		Last: toLastDTOs(last),
	})
}

func (h *Handler) handleDeleteSetLog(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.deleteSetLog.Execute(r.Context(), training.SetLogID(id)); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetStats(w http.ResponseWriter, r *http.Request) {
	from, to, err := periodOf(r)
	if err != nil {
		respondError(w, err)
		return
	}

	trends, err := h.stats.Trends(r.Context(), from, to)
	if err != nil {
		respondError(w, err)
		return
	}
	volume, err := h.stats.WeeklyVolume(r.Context(), to)
	if err != nil {
		respondError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, statsResponse{
		From:         from.String(),
		To:           to.String(),
		Trends:       toTrendDTOs(trends),
		WeeklyVolume: toVolumeDTOs(volume),
	})
}

// periodOf は from / to を読む。省略されたら直近の既定期間。
//
// 既定を持たせるのは、画面が毎回日付を組み立てなくて済むようにするため。
// 端末の日付とサーバーの日付がずれる余地を減らす意味もある。
func periodOf(r *http.Request) (training.Date, training.Date, error) {
	q := r.URL.Query()

	to, err := dateOrToday(q.Get("to"))
	if err != nil {
		return training.Date{}, training.Date{}, err
	}

	if raw := q.Get("from"); raw != "" {
		from, err := training.ParseDate(raw)
		if err != nil {
			return training.Date{}, training.Date{},
				invalidInput("from が日付として読めない: " + err.Error())
		}
		if to.DaysSince(from) > maxHistoryDays {
			return training.Date{}, training.Date{},
				invalidInput("期間が長すぎる（上限 " + strconv.Itoa(maxHistoryDays) + "日）")
		}
		if from.After(to) {
			return training.Date{}, training.Date{}, invalidInput("from が to より後である")
		}
		return from, to, nil
	}
	return to.AddDays(-defaultHistoryDays), to, nil
}

func dateOrToday(raw string) (training.Date, error) {
	if raw == "" {
		// サーバーの日付を使う。単一ユーザーで、端末とサーバーの
		// タイムゾーンが違う運用は想定していない。
		d, err := training.FromTime(time.Now(), time.UTC)
		if err != nil {
			return training.Date{}, err
		}
		return d, nil
	}
	d, err := training.ParseDate(raw)
	if err != nil {
		return training.Date{}, invalidInput("to が日付として読めない: " + err.Error())
	}
	return d, nil
}

func toDayDTOs(days []query.Day) []dayDTO {
	out := make([]dayDTO, 0, len(days))
	for _, d := range days {
		day := dayDTO{Date: d.Date.String(), TotalSets: d.TotalSets}
		for _, e := range d.Exercises {
			ex := exerciseLogDTO{
				ExerciseID: string(e.ExerciseID),
				Name:       e.Name,
				Sets:       make([]setDTO, 0, len(e.Sets)),
			}
			for _, s := range e.Sets {
				ex.Sets = append(ex.Sets, setDTO{
					ID: string(s.ID), WeightKg: s.WeightKg, Reps: s.Reps, RIR: s.RIR,
				})
			}
			day.Exercises = append(day.Exercises, ex)
		}
		out = append(out, day)
	}
	return out
}

func toLastDTOs(last map[training.ExerciseID]query.LastPerformance) map[string]lastDTO {
	out := make(map[string]lastDTO, len(last))
	for id, l := range last {
		out[string(id)] = lastDTO{
			Date:     l.Date.String(),
			WeightKg: l.WeightKg,
			Weights:  l.Weights,
			Reps:     l.Reps,
			DaysAgo:  l.DaysAgo,
		}
	}
	return out
}

func toTrendDTOs(trends []query.Trend) []trendDTO {
	out := make([]trendDTO, 0, len(trends))
	for _, t := range trends {
		item := trendDTO{
			ExerciseID: string(t.ExerciseID),
			Name:       t.Name,
			CurrentKg:  t.CurrentKg,
			ChangeKg:   t.ChangeKg,
			Points:     make([]pointDTO, 0, len(t.Points)),
		}
		for _, p := range t.Points {
			item.Points = append(item.Points, pointDTO{Date: p.Date.String(), Kg: p.Kg})
		}
		out = append(out, item)
	}
	return out
}

func toVolumeDTOs(volume []query.RegionVolume) []volumeDTO {
	out := make([]volumeDTO, 0, len(volume))
	for _, v := range volume {
		out = append(out, volumeDTO{
			Region: string(v.Region), TargetSets: v.TargetSet, DoneSets: v.DoneSet,
		})
	}
	return out
}
