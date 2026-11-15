package httpapi

import (
	"net/http"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/runlog"
)

// runLogDTO はラン1回。数値をポインタにするのは setLogDTO と同じ理由で、
// 欠落を 0 と区別するため。0 はコンストラクタが範囲外として弾くが、
// そのエラーでは「送られなかった」ことが分からない。
type runLogDTO struct {
	ID              string   `json:"id"`
	Date            string   `json:"date"`
	DistanceKm      *float64 `json:"distance_km"`
	DurationSeconds *int     `json:"duration_seconds"`
}

// runLogsResponse の runs は空でも [] で返す（null にしない）。
type runLogsResponse struct {
	Runs []runLogDTO `json:"runs"`
}

func toRunLogDTO(l *runlog.RunLog) runLogDTO {
	km := l.Distance().Km()
	sec := l.Duration().Seconds()
	return runLogDTO{
		ID: string(l.ID()), Date: l.PerformedOn().String(),
		DistanceKm: &km, DurationSeconds: &sec,
	}
}

func (h *Handler) handlePostRunLog(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var dto runLogDTO
	if err := decodeJSON(r, &dto); err != nil {
		respondError(w, err)
		return
	}
	date, err := training.ParseDate(dto.Date)
	if err != nil {
		respondError(w, invalidInput(err.Error()))
		return
	}
	if dto.DistanceKm == nil || dto.DurationSeconds == nil {
		respondError(w, invalidInput("distance_km / duration_seconds は必須である"))
		return
	}
	l, err := runlog.NewRunLog(runlog.RunLogParams{
		ID: dto.ID, PerformedOn: date,
		DistanceKm: *dto.DistanceKm, DurationSeconds: *dto.DurationSeconds,
	})
	if err != nil {
		respondError(w, invalidInput(err.Error()))
		return
	}
	if err := h.recordRun.Execute(r.Context(), user, l); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetRunLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	from, to, err := periodOf(r)
	if err != nil {
		respondError(w, err)
		return
	}
	logs, err := h.runs.Between(r.Context(), user, from, to)
	if err != nil {
		respondError(w, err)
		return
	}
	out := make([]runLogDTO, 0, len(logs))
	for _, l := range logs {
		out = append(out, toRunLogDTO(l))
	}
	writeJSON(w, http.StatusOK, runLogsResponse{Runs: out})
}

func (h *Handler) handleDeleteRunLog(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	if err := h.deleteRunLog.Execute(r.Context(), user, runlog.RunLogID(r.PathValue("id"))); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
