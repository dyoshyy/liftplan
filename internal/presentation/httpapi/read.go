package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// defaultHistoryDays は期間を指定しなかったときに遡る日数。
//
// 8週間。もとはデロード判定の窓（8セッション）に合わせた値で、その機能は
// 消えた（D-124）。推移を見るには足りているので据え置いている。
// 既定と上限を置く理由そのものは D-083。
const defaultHistoryDays = 56

// maxHistoryDays は一度に遡れる上限。
//
// 上限が無いと、3年ぶんを1回で要求されたときに応答が数MBになる。
// 画面が固まれば使えないことに変わりはないし、その1本を組み立てている間の
// メモリは同じインスタンスに居るほかの利用者と分け合っている。
const maxHistoryDays = 400

func (h *Handler) handleGetExercises(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	items, err := h.exercises.All(r.Context(), user)
	if err != nil {
		respondError(w, err)
		return
	}

	out := make([]exerciseDTO, 0, len(items))
	for _, e := range items {
		stimulus := make(map[string]float64, len(e.Stimulus))
		for region, c := range e.Stimulus {
			stimulus[string(region)] = c
		}

		out = append(out, exerciseDTO{
			ID:          string(e.ID),
			Name:        e.Name,
			IncrementKg: e.IncrementKg,
			Stimulus:    stimulus,
		})
	}
	writeJSON(w, http.StatusOK, exercisesResponse{Exercises: out})
}

func (h *Handler) handleGetSetLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	from, to, err := periodOf(r)
	if err != nil {
		respondError(w, err)
		return
	}

	days, err := h.history.Days(r.Context(), user, from, to)
	if err != nil {
		respondError(w, err)
		return
	}

	// 直近の実績も同じ応答で返す。画面は「今日のメニュー」と並べて
	// 「前回どうだったか」を出すので、別の往復にすると表示が揃わない。
	last, err := h.history.LastPerformances(r.Context(), user, to)
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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := h.deleteSetLog.Execute(r.Context(), user, setlog.SetLogID(id)); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetStats(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	from, to, err := periodOf(r)
	if err != nil {
		respondError(w, err)
		return
	}

	trends, err := h.stats.Trends(r.Context(), user, from, to)
	if err != nil {
		respondError(w, err)
		return
	}
	volume, err := h.stats.WeeklyVolume(r.Context(), user, to)
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
		// to が無いときだけサーバーの日付（UTC）を使う。
		//
		// UTC の「今日」は利用者の「今日」と一致するとは限らない（日本では
		// 朝9時まで前日になる）。それでも利用者ごとのタイムゾーンを
		// 持たないのは、画面が必ず端末の日付で to を付けてくるので
		// （web の useLiftplan.ts と useStats.ts）、この既定が誰かの
		// 「今日」を決める経路が無いため。ここに来るのは curl で手で
		// 叩いたときだけで、窓の端が1日ずれるだけで済む。
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

func toLastDTOs(last map[exercise.ExerciseID]query.LastPerformance) map[string]lastDTO {
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

// loginDTO はログイン方法の1つ。
//
// email がポインタなのは、取っていないアドレスを null で返すため。空文字に
// すると、画面が「空という値のアドレス」と「取っていない」を区別できない。
type loginDTO struct {
	Provider string  `json:"provider"`
	Email    *string `json:"email"`
}

// handleGetAccount は認証した本人のログイン方法を返す。
//
// 設定画面の「アカウント」に、どのアドレスで入っているかを出すのに使う。
// アカウントを持たない利用者（開発用のセッション）には空の配列を返す。
func (h *Handler) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	logins, err := h.accounts.Of(r.Context(), user)
	if err != nil {
		respondError(w, err)
		return
	}

	out := make([]loginDTO, 0, len(logins))
	for _, l := range logins {
		dto := loginDTO{Provider: l.Provider}
		if l.Email != "" {
			email := l.Email
			dto.Email = &email
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, struct {
		Accounts []loginDTO `json:"accounts"`
	}{out})
}
