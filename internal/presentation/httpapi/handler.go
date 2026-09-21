package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/application/query"
	"github.com/dyoshyy/liftplan/internal/application/usecase"
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

type Handler struct {
	getSession       *usecase.GetSession
	recordSets       *usecase.RecordSets
	recordConditions *usecase.RecordConditions
	setFocus         *usecase.SetFocusExercise
	setDeclared      *usecase.SetDeclaredExercises
	setFrequency     *usecase.SetFrequency
	setSelected      *usecase.SetSelectedExercises
	setTarget        *usecase.SetWeeklyTarget
	setSplit         *usecase.SetSplitCycle
	getProgram       *usecase.GetProgram
	deleteSetLog     *usecase.DeleteSetLog
	exercises        *query.Exercises
	history          *query.History
	stats            *query.Stats
}

// Dependencies は Handler を組むための材料。
//
// 位置引数で受けていたときは、口を足す・消すたびに呼び出し5箇所の
// 並びを書き直していた。名前で渡せば、触るのは足した1行だけで済む。
type Dependencies struct {
	GetSession       *usecase.GetSession
	RecordSets       *usecase.RecordSets
	RecordConditions *usecase.RecordConditions
	SetFocus         *usecase.SetFocusExercise
	SetDeclared      *usecase.SetDeclaredExercises
	SetFrequency     *usecase.SetFrequency
	SetSelected      *usecase.SetSelectedExercises
	SetTarget        *usecase.SetWeeklyTarget
	SetSplit         *usecase.SetSplitCycle
	GetProgram       *usecase.GetProgram
	DeleteSetLog     *usecase.DeleteSetLog
	Exercises        *query.Exercises
	History          *query.History
	Stats            *query.Stats
}

// NewHandler は依存を受け取って Handler を組む。
//
// 欠けた依存があれば、ここで止める。位置引数なら渡し忘れはコンパイル
// エラーだったが、構造体はゼロ値で通る。検査しないと、起動は成功して、
// 欠けた口が初めて叩かれたときに nil 参照で落ちる。
//
// リフレクションで回さず1行ずつ並べているのは、将来「無くてもよい依存」が
// 入ったときに、その行を書かないだけで済むようにするため。並べ忘れは
// TestNewHandler_RejectsMissingDependency が全フィールドを回して捕まえる。
func NewHandler(d Dependencies) (*Handler, error) {
	switch {
	case d.GetSession == nil:
		return nil, errMissingDependency("GetSession")
	case d.RecordSets == nil:
		return nil, errMissingDependency("RecordSets")
	case d.RecordConditions == nil:
		return nil, errMissingDependency("RecordConditions")
	case d.SetFocus == nil:
		return nil, errMissingDependency("SetFocus")
	case d.SetDeclared == nil:
		return nil, errMissingDependency("SetDeclared")
	case d.SetFrequency == nil:
		return nil, errMissingDependency("SetFrequency")
	case d.SetSelected == nil:
		return nil, errMissingDependency("SetSelected")
	case d.SetTarget == nil:
		return nil, errMissingDependency("SetTarget")
	case d.SetSplit == nil:
		return nil, errMissingDependency("SetSplit")
	case d.GetProgram == nil:
		return nil, errMissingDependency("GetProgram")
	case d.DeleteSetLog == nil:
		return nil, errMissingDependency("DeleteSetLog")
	case d.Exercises == nil:
		return nil, errMissingDependency("Exercises")
	case d.History == nil:
		return nil, errMissingDependency("History")
	case d.Stats == nil:
		return nil, errMissingDependency("Stats")
	}

	return &Handler{
		getSession:       d.GetSession,
		recordSets:       d.RecordSets,
		recordConditions: d.RecordConditions,
		setFocus:         d.SetFocus,
		setDeclared:      d.SetDeclared,
		setFrequency:     d.SetFrequency,
		setSelected:      d.SetSelected,
		setTarget:        d.SetTarget,
		setSplit:         d.SetSplit,
		getProgram:       d.GetProgram,
		deleteSetLog:     d.DeleteSetLog,
		exercises:        d.Exercises,
		history:          d.History,
		stats:            d.Stats,
	}, nil
}

// errMissingDependency は欠けた依存の名前をエラーに載せる。
// 名前が無いと、14個のどれを直せばよいかが分からない。
func errMissingDependency(field string) error {
	return fmt.Errorf("httpapi.Dependencies.%s が設定されていない", field)
}

// clientClosedRequest はクライアントが応答を待たずに切断したことを表す。
// 標準ライブラリに定数が無い。サーバーの障害ではないので、
// 500 と混ぜるとログと警報がクライアントの都合で汚れる。
const clientClosedRequest = 499

// respondError は失敗を HTTP に翻訳する。
//
// 見るのは apperror.Error 1つだけ。ドメインのセンチネルをここで並べると、
// センチネルを足すたびに presentation が動き、拾い漏らした分類が黙って
// 500 になる。翻訳はユースケース層の classify が持つ。
//
// context の2つだけは別扱い。ユースケースを通らずに決まる転送層の事情で、
// 応答の形も違う（切断はボディを返さない）。
//
// 4xx は詳細を返し、5xx は隠す。前者は送り主が直せるもので、隠すと直せない。
// 後者にはドメインの内部（種目IDや閾値）が載っており、外に出す理由がない。
func respondError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		// ボディを返さないのは、読む相手がもう居ないから。
		w.WriteHeader(clientClosedRequest)
		return
	case errors.Is(err, context.DeadlineExceeded):
		respondCoded(w, apperror.ErrTimeout, err)
		return
	}

	var coded *apperror.Error
	if !errors.As(err, &coded) {
		// 分類できないものは 500。クライアントから隠すことと、記録に
		// 残さないことは別。記録しないと、障害時に運用者へ残るのは
		// 「内部エラーが発生した」だけで原因が完全に消える。
		slog.Error("分類できないエラー", "error", err, "type", fmt.Sprintf("%T", err))
		respondCoded(w, apperror.ErrInternal, nil)
		return
	}
	respondCoded(w, coded, err)
}

// respondCoded は分類済みのエラーを応答とログにする。
//
// ログの高さを分けるのは、鳴らす相手が違うから。4xx は送り主の問題で
// サーバーは正しく動いているので Info。503 は一時障害なので Warn。
// 500 だけが調べるべきもので Error。ここを揃えると、4xx が並ぶだけで
// 警報が鳴り、本当の障害が埋もれる。
func respondCoded(w http.ResponseWriter, coded *apperror.Error, cause error) {
	message := coded.Error()
	switch {
	case coded.HTTPStatus() < 500:
		if cause != nil {
			message = cause.Error()
		}
		slog.Info("リクエストを拒否", "code", coded.Code(), "error", cause)
	case coded.HTTPStatus() == http.StatusServiceUnavailable:
		slog.Warn("一時的に利用できない", "code", coded.Code(), "error", cause)
	default:
		if cause != nil {
			slog.Error("リクエストの処理に失敗", "code", coded.Code(), "error", cause)
		}
	}
	writeJSON(w, coded.HTTPStatus(), errorResponse{Code: coded.Code(), Error: message})
}

// invalidInput は入力の不正を、分類できる形で作る。
func invalidInput(message string) error {
	return fmt.Errorf("%w: %s", apperror.ErrInvalidInput, message)
}

func (h *Handler) handleGetSession(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
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

	session, err := h.getSession.Execute(r.Context(), user, usecase.GetSessionInput{Date: date})
	if err != nil {
		respondError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toSessionDTO(session))
}

func (h *Handler) handlePostSetLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
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

	if err := h.recordSets.Execute(r.Context(), user, logs); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handlePostConditions(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
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

	if err := h.recordConditions.Execute(r.Context(), user, items); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetProgram(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	prog, err := h.getProgram.Execute(r.Context(), user)
	if err != nil {
		respondError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProgramDTO(prog))
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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
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

	if err := h.setFocus.Execute(r.Context(), user, focus); err != nil {
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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req declaredDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	ids := make([]exercise.ExerciseID, 0, len(req.Declared))
	for _, id := range req.Declared {
		ids = append(ids, exercise.ExerciseID(id))
	}

	if err := h.setDeclared.Execute(r.Context(), user, ids); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramFrequency は週の頻度を差し替える。週目標も道連れに
// 置き直る。他の口と違って2フィールド動くので、名前を frequency のままに
// せず応答でも隠さない（GET で両方見える）。
func (h *Handler) handlePutProgramFrequency(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req frequencyDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}
	if err := h.setFrequency.Execute(r.Context(), user, req.PerWeek); err != nil {
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
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req selectedDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	ids := make([]exercise.ExerciseID, 0, len(req.Selected))
	for _, id := range req.Selected {
		ids = append(ids, exercise.ExerciseID(id))
	}

	if err := h.setSelected.Execute(r.Context(), user, ids); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramTarget は週目標だけを差し替える。
func (h *Handler) handlePutProgramTarget(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req targetDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	sets := make(map[training.MuscleRegion]float64, len(req.Target))
	for k, v := range req.Target {
		sets[training.MuscleRegion(k)] = v
	}

	if err := h.setTarget.Execute(r.Context(), user, sets); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handlePutProgramSplit は分割の周期を差し替える。空なら分割なし。
func (h *Handler) handlePutProgramSplit(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	var req splitCycleDTO
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, err)
		return
	}

	cycle := make([]program.Split, 0, len(req.Splits))
	for _, d := range req.Splits {
		regions := make([]training.MuscleRegion, 0, len(d.Regions))
		for _, x := range d.Regions {
			regions = append(regions, training.MuscleRegion(x))
		}
		s, err := program.NewSplit(d.Name, regions)
		if err != nil {
			respondError(w, invalidInput(err.Error()))
			return
		}
		cycle = append(cycle, s)
	}

	if err := h.setSplit.Execute(r.Context(), user, cycle); err != nil {
		respondError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetSplitPresets は選べる分割の一覧を返す。
//
// 画面が自前で持たないのは、区分の割り当てがドメインの知識だから。
// 画面に置くと、シードの区分が増えたときに黙ってずれる。
func (h *Handler) handleGetSplitPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := seed.SplitPresets()
	if err != nil {
		respondError(w, err)
		return
	}

	out := make([]splitPresetDTO, 0, len(presets))
	for _, p := range presets {
		out = append(out, splitPresetDTO{
			Key: p.Key, Name: p.Name, Splits: toSplitDTOs(p.Cycle),
		})
	}
	writeJSON(w, http.StatusOK, splitPresetsResponse{Presets: out})
}

// maxBodyBytes はリクエストボディの上限。
//
// 1セッション25セットとして、1件200バイト弱でも5KB程度。1MBは
// 数週間ぶんをまとめて同期しても足りる。上限が無いと、無認証の
// エンドポイントに巨大なボディを投げるだけでメモリを食い潰せる。
const maxBodyBytes = 1 << 20

// errBodyTooLarge はボディが上限を超えたことを表す。
//
// 転送層の事情なのでユースケースを通らない。apperror を直接組んで、
// respondError が他と同じ経路で扱えるようにする。
var errBodyTooLarge = fmt.Errorf("%w", apperror.ErrTooLarge)

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
		return fmt.Errorf("%w: リクエストボディを解釈できない", apperror.ErrInvalidInput)
	}
	if dec.More() {
		return fmt.Errorf("%w: リクエストボディに余分なデータがある", apperror.ErrInvalidInput)
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
		return fmt.Errorf("%w: Content-Type が無い", apperror.ErrInvalidInput)
	}
	mediaType, _, err := mime.ParseMediaType(raw)
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("%w: Content-Type は application/json である必要がある", apperror.ErrInvalidInput)
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
