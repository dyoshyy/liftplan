package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// unauthenticatedPaths は認証を通さない経路。
//
// ヘルスチェックだけ。Cloud Run の起動プローブや前段のロードバランサが
// 叩けなくなると、認証が正しくても「起動していない」と判定されて
// トラフィックが来なくなる。
var unauthenticatedPaths = map[string]bool{"/healthz": true}

// RequireBearerToken は Bearer トークンによる認証を要求する。
//
// 単一ユーザー向けの最小構成。ユーザーという概念はここから内側へ
// 持ち込まない。ドメインもユースケースも「誰が」を知らないままにする。
//
// OAuth へ移るときは、このミドルウェアを差し替えるだけで済む。
func RequireBearerToken(token string) func(http.Handler) http.Handler {
	want := []byte(token)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if unauthenticatedPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			got, ok := bearerToken(r)
			if !ok {
				// WWW-Authenticate を付けるのは、クライアントが
				// 「認証が要る」と「壊れている」を区別できるようにするため。
				w.Header().Set("WWW-Authenticate", `Bearer realm="liftplan"`)
				writeError(w, http.StatusUnauthorized, "認証が必要である")
				return
			}

			// 定数時間で比べる。素朴な == は、一致する接頭辞が長いほど
			// 応答が遅くなるので、トークンを1バイトずつ推測できる。
			if subtle.ConstantTimeCompare([]byte(got), want) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="liftplan"`)
				writeError(w, http.StatusUnauthorized, "認証に失敗した")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// bearerToken は Authorization ヘッダからトークンを取り出す。
//
// 方式名の比較を大文字小文字を無視して行うのは RFC 7235 の要求。
// "bearer" と送ってくるクライアントを弾くと、原因の分からない 401 になる。
func bearerToken(r *http.Request) (string, bool) {
	const prefix = "bearer "

	raw := r.Header.Get("Authorization")
	if len(raw) < len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(raw[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}
