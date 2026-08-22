package httpapi

import "net/http"

// HealthPath はヘルスチェックの経路。
//
// /healthz にしないのは、Cloud Run のフロントエンドが完全一致で
// 横取りして Google の 404 を返すため。アプリまで届かない（D-073）。
const HealthPath = "/health"

// healthPath は同じ値の内部名。
const healthPath = HealthPath

// Routes は Go 1.22 以降のメソッド付きパターンでルーティングする。
// 外部のルータライブラリは不要。
func (h *Handler) Routes() *http.ServeMux {
	// /healthz はここに置かない。保存先への疎通を含める必要があり、
	// プレゼンテーション層は保存先を知らない。cmd がルータの前に被せる。
	// 両方に置くと、どちらが応答しているのか読んで分からなくなる。
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sessions", h.handleGetSession)
	mux.HandleFunc("POST /api/set-logs", h.handlePostSetLogs)
	mux.HandleFunc("POST /api/conditions", h.handlePostConditions)
	mux.HandleFunc("GET /api/program", h.handleGetProgram)
	mux.HandleFunc("PUT /api/program", h.handlePutProgram)
	return mux
}
