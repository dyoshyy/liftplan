package httpapi

import "net/http"

// Routes は Go 1.22 以降のメソッド付きパターンでルーティングする。
// 外部のルータライブラリは不要。
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.handleHealthz)
	mux.HandleFunc("GET /api/sessions", h.handleGetSession)
	mux.HandleFunc("POST /api/set-logs", h.handlePostSetLogs)
	mux.HandleFunc("POST /api/conditions", h.handlePostConditions)
	mux.HandleFunc("GET /api/program", h.handleGetProgram)
	mux.HandleFunc("PUT /api/program", h.handlePutProgram)
	return mux
}
