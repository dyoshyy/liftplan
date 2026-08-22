package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed web
var webFS embed.FS

// staticPaths は認証を通さない静的資産。
//
// 画面の殻を認証の内側に置くと、トークンを入力する画面そのものが
// 出せなくなる。殻に秘密は入っていないので公開してよい。
//
// 明示的な一覧にするのは、フォールバックで開かないため。
// 「/api 以外は公開」にすると、後から足した経路が既定で公開になる。
var staticPaths = map[string]string{
	"/":                "web/index.html",
	"/app.js":          "web/app.js",
	"/app.webmanifest": "web/app.webmanifest",
	"/sw.js":           "web/sw.js",
	"/icon.svg":        "web/icon.svg",
}

// contentTypes は拡張子から Content-Type を決める。
//
// http.ServeContent に任せると .webmanifest を認識しないので自前で持つ。
var contentTypes = map[string]string{
	"web/index.html":      "text/html; charset=utf-8",
	"web/app.js":          "text/javascript; charset=utf-8",
	"web/app.webmanifest": "application/manifest+json",
	"web/sw.js":           "text/javascript; charset=utf-8",
	"web/icon.svg":        "image/svg+xml",
}

func (h *Handler) handleStatic(w http.ResponseWriter, r *http.Request) {
	name, ok := staticPaths[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}

	body, err := fs.ReadFile(webFS, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "画面を読み込めない")
		return
	}

	w.Header().Set("Content-Type", contentTypes[name])
	// 画面は更新のたびに変わる。キャッシュされると、直したのに
	// 古い画面が出続ける。1つのファイルなので毎回取り直してよい。
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}
