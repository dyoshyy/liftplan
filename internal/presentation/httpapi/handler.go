package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/dyoshyy/liftplan-server/internal/application/usecase"
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

type Handler struct {
	getSession       *usecase.GetSession
	recordSets       *usecase.RecordSets
	recordConditions *usecase.RecordConditions
	configureProgram *usecase.ConfigureProgram
	programs         training.ProgramRepository
}

func NewHandler(
	getSession *usecase.GetSession,
	recordSets *usecase.RecordSets,
	recordConditions *usecase.RecordConditions,
	configureProgram *usecase.ConfigureProgram,
	programs training.ProgramRepository,
) *Handler {
	return &Handler{
		getSession:       getSession,
		recordSets:       recordSets,
		recordConditions: recordConditions,
		configureProgram: configureProgram,
		programs:         programs,
	}
}

// clientClosedRequest はクライアントが応答を待たずに切断したことを表す。
// 標準ライブラリに定数が無い。サーバーの障害ではないので、
// 500 と混ぜるとログと警報がクライアントの都合で汚れる。
const clientClosedRequest = 499

// respondError は失敗をステータスコードに翻訳する。
//
// 分類の根拠を1箇所に集める。ハンドラごとに errors.Is を並べると、
// 新しいセンチネルを足したときに拾い漏らすハンドラが出る。
//
// 500 のときだけ内部のエラー文を返さない。ドメインのエラーには
// 種目IDや閾値が載っており、外に出す理由がない。
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		w.WriteHeader(clientClosedRequest)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "処理が時間内に終わらなかった")
	case errors.Is(err, usecase.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, training.ErrProgramNotConfigured):
		writeError(w, http.StatusConflict, "プログラムが未設定である")
	case errors.Is(err, training.ErrConflictingSetLog):
		writeError(w, http.StatusConflict, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "内部エラーが発生した")
	}
}

// parseExerciseIDs はカンマ区切りの種目IDを分解する。
//
// 空要素は落とす。"a,,b" や末尾のカンマはクライアントの組み立てで
// 普通に生まれるので、そのたびに 400 を返す理由がない。
func parseExerciseIDs(raw string) []training.ExerciseID {
	if raw == "" {
		return nil
	}
	out := make([]training.ExerciseID, 0, strings.Count(raw, ",")+1)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, training.ExerciseID(part))
	}
	return out
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("date")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "date クエリパラメータが必要である")
		return
	}
	date, err := training.ParseDate(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	session, err := h.getSession.Execute(r.Context(), usecase.GetSessionInput{
		Date:           date,
		DeloadAccepted: parseExerciseIDs(r.URL.Query().Get("deload_accepted")),
	})
	if err != nil {
		respondError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toSessionDTO(session))
}

func (h *Handler) handlePostSetLogs(w http.ResponseWriter, r *http.Request) {
	var req setLogsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	logs := make([]*training.SetLog, 0, len(req.Logs))
	for i, dto := range req.Logs {
		date, err := training.ParseDate(dto.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("logs[%d]: %v", i, err))
			return
		}
		if dto.WeightKg == nil || dto.Reps == nil || dto.RIR == nil {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("logs[%d]: weight_kg / reps / rir は必須である", i))
			return
		}
		log, err := training.NewSetLog(training.SetLogParams{
			ID:          dto.ID,
			PerformedOn: date,
			ExerciseID:  dto.ExerciseID,
			WeightKg:    *dto.WeightKg,
			Reps:        *dto.Reps,
			RIR:         *dto.RIR,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("logs[%d]: %v", i, err))
			return
		}
		logs = append(logs, log)
	}

	if err := h.recordSets.Execute(r.Context(), logs); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handlePostConditions(w http.ResponseWriter, r *http.Request) {
	var req conditionsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	items := make([]training.DailyCondition, 0, len(req.Conditions))
	for i, dto := range req.Conditions {
		date, err := training.ParseDate(dto.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("conditions[%d]: %v", i, err))
			return
		}
		c := training.NewDailyCondition(date)
		if dto.BodyWeightKg != nil {
			c = c.WithBodyWeight(*dto.BodyWeightKg)
		}
		if dto.SleepHours != nil {
			c = c.WithSleepHours(*dto.SleepHours)
		}
		items = append(items, c)
	}

	if err := h.recordConditions.Execute(r.Context(), items); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetProgram(w http.ResponseWriter, r *http.Request) {
	program, err := h.programs.Get(r.Context())
	if err != nil {
		if errors.Is(err, training.ErrProgramNotConfigured) {
			// 取得の文脈では 404。まだ存在しないという意味であって、
			// 状態の衝突ではない。
			writeError(w, http.StatusNotFound, "プログラムが未設定である")
			return
		}
		respondError(w, err)
		return
	}
	if program == nil {
		writeError(w, http.StatusNotFound, "プログラムが未設定である")
		return
	}
	writeJSON(w, http.StatusOK, toProgramDTO(program))
}

func (h *Handler) handlePutProgram(w http.ResponseWriter, r *http.Request) {
	var req programDTO
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.configureProgram.Execute(r.Context(), req.toInput()); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("リクエストボディを解釈できない: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
