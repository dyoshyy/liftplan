package httpapi

import (
	"net/http"
	"strconv"
	"time"
)

// preflightMaxAge は preflight の結果をブラウザが覚えていてよい時間。
//
// 付けないと、記録を送るたびに往復が2回になる。ジムの電波では効く。
// 2時間にしているのは Chrome の上限がそこだから。それより長く書いても
// 切り下げられるだけで、読む人に誤解を与える。
const preflightMaxAge = 2 * time.Hour

// allowedHeaders と allowedMethods は preflight への回答。
//
// Authorization を落とすと Bearer が送れない。Content-Type を落とすと
// JSON の POST が送れない。どちらも「なぜか記録だけ送れない」になる。
const (
	allowedHeaders = "Authorization, Content-Type"
	allowedMethods = "GET, POST, PUT, DELETE, OPTIONS"
)

// AllowOrigins は指定したオリジンからのクロスオリジン要求を許す。
//
// 画面は Cloudflare Workers から配られ、API はここにある。別オリジンなので
// ブラウザは CORS を要求する。
//
// ワイルドカードは受け付けない。この API は Bearer トークンで守られており、
// `*` を返すと任意のサイトが利用者のトークン付き要求の結果を読めるようになる。
// 許可する相手は環境変数で明示する。
//
// **このミドルウェアは認証の外側に置くこと。** preflight の OPTIONS には
// ブラウザが Authorization を付けないので、認証の内側だと必ず 401 になる。
// またルータはメソッド付きパターンで登録しているため、OPTIONS が
// ルータまで届くと 405 になる。どちらもここで完結させる。
func AllowOrigins(origins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		if o != "" {
			allowed[o] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Vary は許否によらず必ず付ける。付けないと、途中のキャッシュが
			// 別オリジン向けの応答を使い回す。
			w.Header().Add("Vary", "Origin")

			origin := r.Header.Get("Origin")
			isPreflight := r.Method == http.MethodOptions &&
				r.Header.Get("Access-Control-Request-Method") != ""

			// オリジンが無いのは同一オリジンか、ブラウザ以外（curl や
			// ヘルスチェック）。CORS の対象ではないのでそのまま通す。
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			if !allowed[origin] {
				// 許していない相手には応答ヘッダを付けない。preflight は
				// ここで断る。通しても本番の要求はブラウザが捨てるが、
				// 403 にしておくとログで設定漏れに気づける。
				if isPreflight {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)

			if isPreflight {
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
				w.Header().Set("Access-Control-Max-Age",
					strconv.Itoa(int(preflightMaxAge.Seconds())))
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
