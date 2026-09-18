package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"

	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

type Handler struct {
	getSession       *usecase.GetSession
	recordSets       *usecase.RecordSets
	recordConditions *usecase.RecordConditions
	configureProgram *usecase.ConfigureProgram
	setFocus         *usecase.SetFocusExercise
	setDeclared      *usecase.SetDeclaredExercises
	setFrequency     *usecase.SetFrequency
	setSelected      *usecase.SetSelectedExercises
	setTarget        *usecase.SetWeeklyTarget
	getProgram       *usecase.GetProgram
	deleteSetLog     *usecase.DeleteSetLog
	exercises        *query.Exercises
	history          *query.History
	stats            *query.Stats
}

func NewHandler(
	getSession *usecase.GetSession,
	recordSets *usecase.RecordSets,
	recordConditions *usecase.RecordConditions,
	configureProgram *usecase.ConfigureProgram,
	setFocus *usecase.SetFocusExercise,
	setDeclared *usecase.SetDeclaredExercises,
	setFrequency *usecase.SetFrequency,
	setSelected *usecase.SetSelectedExercises,
	setTarget *usecase.SetWeeklyTarget,
	getProgram *usecase.GetProgram,
	deleteSetLog *usecase.DeleteSetLog,
	exercises *query.Exercises,
	history *query.History,
	stats *query.Stats,
) *Handler {
	return &Handler{
		getSession:       getSession,
		recordSets:       recordSets,
		recordConditions: recordConditions,
		configureProgram: configureProgram,
		setFocus:         setFocus,
		setDeclared:      setDeclared,
		setFrequency:     setFrequency,
		setSelected:      setSelected,
		setTarget:        setTarget,
		getProgram:       getProgram,
		deleteSetLog:     deleteSetLog,
		exercises:        exercises,
		history:          history,
		stats:            stats,
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
		// ボディを返さないのは、読む相手がもう居ないから。
		w.WriteHeader(clientClosedRequest)
	case errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusGatewayTimeout, "処理が時間内に終わらなかった")
	case errors.Is(err, errBodyTooLarge):
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, usecase.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, program.ErrProgramNotConfigured):
		writeError(w, http.StatusConflict, "プログラムが未設定である")
	case errors.Is(err, setlog.ErrConflictingSetLog):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, training.ErrRepositoryUnavailable):
		// 後で送り直せば通る。500 と混ぜるとクライアントが諦める。
		slog.Error("保存先に到達できない", "error", err)
		writeError(w, http.StatusServiceUnavailable, "一時的に利用できない")
	default:
		// クライアントから隠すことと、記録に残さないことは別。
		// 記録しないと、障害時に運用者へ残るのは「内部エラーが発生した」だけで
		// 原因が完全に消える。
		slog.Error("リクエストの処理に失敗", "error", err)
		writeError(w, http.StatusInternalServerError, "内部エラーが発生した")
	}
}

// invalidInput は入力の不正を、分類できる形で作る。
func invalidInput(message string) error {
	return fmt.Errorf("%w: %s", usecase.ErrInvalidInput, message)
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

	session, err := h.getSession.Execute(r.Context(), usecase.GetSessionInput{Date: date})
	if err != nil {
		respondError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toSessionDTO(session))
}

func (h *Handler) handlePostSetLogs(w http.ResponseWriter, r *http.Request) {
	var req setLogsRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	logs := make([]*setlog.SetLog, 0, len(req.Logs))
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
		log, err := setlog.NewSetLog(setlog.SetLogParams{
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
		respondError(w, err)
		return
	}

	items := make([]condition.DailyCondition, 0, len(req.Conditions))
	for i, dto := range req.Conditions {
		date, err := training.ParseDate(dto.Date)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("conditions[%d]: %v", i, err))
			return
		}
		// ドメインは範囲外の値を「無かったこと」にする。1日ぶんの異常値で
		// 取り込み全体を止めないための判断で、集めた記録を扱う場面では正しい。
		// だが保存要求では別で、黙って捨てるとクライアントは成功したと
		// 受け取ったまま記録が消える。しかも取得口が無いので検知できない。
		c := condition.NewDailyCondition(date)
		if dto.BodyWeightKg != nil {
			c = c.WithBodyWeight(*dto.BodyWeightKg)
			if _, ok := c.BodyWeightKg(); !ok {
				writeError(w, http.StatusBadRequest,
					fmt.Sprintf("conditions[%d]: 体重が範囲外である: %v", i, *dto.BodyWeightKg))
				return
			}
		}
		if dto.SleepHours != nil {
			c = c.WithSleepHours(*dto.SleepHours)
			if _, ok := c.SleepHours(); !ok {
				writeError(w, http.StatusBadRequest,
					fmt.Sprintf("conditions[%d]: 睡眠時間が範囲外である: %v", i, *dto.SleepHours))
				return
			}
		}
		if dto.BodyWeightKg == nil && dto.SleepHours == nil {
			writeError(w, http.StatusBadRequest,
				fmt.Sprintf("conditions[%d]: body_weight_kg か sleep_hours のどちらかが必要である", i))
			return
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
	prog, err := h.getProgram.Execute(r.Context())
	if err != nil {
		// 取得の文脈では 404。まだ存在しないという意味であって、
		// 状態の衝突ではない。
		if errors.Is(err, program.ErrProgramNotConfigured) {
			writeError(w, http.StatusNotFound, "プログラムが未設定である")
			return
		}
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProgramDTO(prog))
}

func (h *Handler) handlePutProgram(w http.ResponseWriter, r *http.Request) {
	var req programDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}
	if err := h.configureProgram.Execute(r.Context(), req.toInput()); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramFocus は重点種目だけを差し替える。
//
// 全置換の PUT /api/program とは別の口にしてある。クライアントが週目標や
// 選択種目を持ち回らずに済むので、契約がずれて 400 になる面も、正しく
// 通ったまま他の設定を上書きする面も無い（D-127）。
//
// 未設定は 409。GET /api/program の 404 と違い、ここは「前提が満たされて
// いない」という状態の衝突なので（D-042 の分類）。
func (h *Handler) handlePutProgramFocus(w http.ResponseWriter, r *http.Request) {
	var req focusDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	// null と空文字はどちらも「指定なし」。NewProgram が空を素通しする。
	var focus exercise.ExerciseID
	if req.Focus != nil {
		focus = exercise.ExerciseID(*req.Focus)
	}

	if err := h.setFocus.Execute(r.Context(), focus); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramDeclared は伸ばしたい種目だけを差し替える。
//
// 重点種目が新しい宣言から外れる場合は 400。黙って重点を解除すると、
// 口を分けた意味（他のフィールドを触らない）が自分で崩れる。
func (h *Handler) handlePutProgramDeclared(w http.ResponseWriter, r *http.Request) {
	var req declaredDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	ids := make([]exercise.ExerciseID, 0, len(req.Declared))
	for _, id := range req.Declared {
		ids = append(ids, exercise.ExerciseID(id))
	}

	if err := h.setDeclared.Execute(r.Context(), ids); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramFrequency は週の頻度を差し替える。週目標も道連れに
// 置き直る。他の口と違って2フィールド動くので、名前を frequency のままに
// せず応答でも隠さない（GET で両方見える）。
func (h *Handler) handlePutProgramFrequency(w http.ResponseWriter, r *http.Request) {
	var req frequencyDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}
	if err := h.setFrequency.Execute(r.Context(), req.PerWeek); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramSelected は使う種目だけを差し替える。
//
// 伸ばしたい種目が外れる選択は 400。黙って宣言を削ると、軸の顔ぶれが
// 変わったことに次のセッションまで気づけない。
func (h *Handler) handlePutProgramSelected(w http.ResponseWriter, r *http.Request) {
	var req selectedDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	ids := make([]exercise.ExerciseID, 0, len(req.Selected))
	for _, id := range req.Selected {
		ids = append(ids, exercise.ExerciseID(id))
	}

	if err := h.setSelected.Execute(r.Context(), ids); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramTarget は週目標だけを差し替える。
func (h *Handler) handlePutProgramTarget(w http.ResponseWriter, r *http.Request) {
	var req targetDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	sets := make(map[training.MuscleRegion]float64, len(req.Target))
	for k, v := range req.Target {
		sets[training.MuscleRegion(k)] = v
	}

	if err := h.setTarget.Execute(r.Context(), sets); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// maxBodyBytes はリクエストボディの上限。
//
// 1セッション25セットとして、1件200バイト弱でも5KB程度。1MBは
// 数週間ぶんをまとめて同期しても足りる。上限が無いと、無認証の
// エンドポイントに巨大なボディを投げるだけでメモリを食い潰せる。
const maxBodyBytes = 1 << 20

// errBodyTooLarge はボディが上限を超えたことを表す。413 に翻訳する。
var errBodyTooLarge = errors.New("リクエストボディが大きすぎる")

// decodeJSON はボディを1つの JSON ドキュメントとして読む。
//
// 末尾に続くトークンを検査するのは、`{"logs":[...]} {"logs":[...]}` のような
// 2つ目以降を黙って捨てて 204 を返さないため。クライアントは保存に
// 成功したと受け取ったまま実績が消える。
func decodeJSON(r *http.Request, dst any) error {
	if err := requireJSONContentType(r); err != nil {
		return err
	}

	body := http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%w: 上限 %dバイト", errBodyTooLarge, maxBodyBytes)
		}
		// 標準ライブラリのエラーには Go の型名・フィールド名が載る。
		// ユーザーが直せる情報ではないので外に出さない。
		return fmt.Errorf("%w: リクエストボディを解釈できない", usecase.ErrInvalidInput)
	}
	if dec.More() {
		return fmt.Errorf("%w: リクエストボディに余分なデータがある", usecase.ErrInvalidInput)
	}
	return nil
}

// requireJSONContentType は Content-Type を検証する。
//
// 検証しないと、text/plain や form-urlencoded でも受け付けてしまう。
// これらはブラウザがプリフライト無しで送れるので、認証を入れた時点で
// CSRF がそのまま通る。境界で閉じておく。
func requireJSONContentType(r *http.Request) error {
	raw := r.Header.Get("Content-Type")
	if raw == "" {
		return fmt.Errorf("%w: Content-Type が無い", usecase.ErrInvalidInput)
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("%w: Content-Type は application/json である必要がある", usecase.ErrInvalidInput)
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
