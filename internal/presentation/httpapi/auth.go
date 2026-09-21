package httpapi

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// unauthenticatedPaths は認証を通さない経路。
//
// ヘルスチェックだけ。画面は別オリジンに移したので、
// 「トークンを入力する画面そのものが出せなくなる」問題は無くなった。前段のロードバランサが叩けなくなると、
// 認証が正しくても「起動していない」と判定されてトラフィックが来なくなる。
//
// パスが /healthz ではなく /health なのは、Cloud Run のフロントエンドが
// /healthz を完全一致で横取りするため（D-073）。
var unauthenticatedPaths = map[string]bool{healthPath: true}

// RequireBearerToken は Bearer トークンによる認証を要求する。
//
// **「誰が」を決めるのはここ。**通した要求の context に利用者を載せ、
// ハンドラはそれを取り出してユースケースへ引数で渡す。ここから内側は
// 全て利用者を明示して動く。
//
// いまはトークン1本なので、載せるのは常に既定ユーザー（マイグレーション
// 0007 が既存の行を寄せた先）。トークンは「本人かどうか」しか言えず、
// 「誰か」を言えない。OAuth へ移ったらセッションから引いた利用者に
// 変わるが、内側の形は変わらない。差し替えるのはこのミドルウェアだけ。
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

			// 利用者を context に載せるのはここだけ。ハンドラが
			// 取り出したら終わりで、その先へは引数で渡す（user.go）。
			next.ServeHTTP(w, r.WithContext(
				withUser(r.Context(), account.DefaultUserID())))
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
