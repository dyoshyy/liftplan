package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// unauthenticatedPaths は認証を通さない経路。
//
// ヘルスチェックと、**ログインの入口と戻りだけ。**前段のロードバランサが
// ヘルスチェックを叩けなくなると、認証が正しくても「起動していない」と
// 判定されてトラフィックが来なくなる。ログインの経路は、通す前の人が
// 通るための経路なので、認証を要求すると誰もログインできない。
//
// **ログアウト（DELETE /auth/session）はここに入れない。**誰でも叩けると、
// トークンのハッシュを総当たりする口になる。「/auth で始まる経路は通す」と
// まとめて書くとここが静かに開くので、経路を1つずつ並べる。
//
// パスが /healthz ではなく /health なのは、Cloud Run のフロントエンドが
// /healthz を完全一致で横取りするため（D-073）。
var unauthenticatedPaths = map[string]bool{
	healthPath:              true,
	"/auth/github/start":    true,
	"/auth/github/callback": true,
	"/auth/google/start":    true,
	"/auth/google/callback": true,
}

// RequireSession はセッショントークンによる認証を要求する。
//
// **「誰が」を決めるのはここ。**トークンをハッシュにしてセッションを引き、
// そのセッションの利用者を context に載せる。ハンドラはそれを取り出して
// ユースケースへ引数で渡す（user.go）。ここから内側は全て利用者を
// 明示して動く。
//
// 以前はトークン1本を定数時間で比べていた（D-068/D-069）。あれは
// 「本人かどうか」しか言えず「誰か」を言えないので、載せる利用者は常に
// 既定ユーザーだった。**差し替えたのはこのミドルウェアだけで、内側の形は
// 変わっていない。**D-068 が「OAuth へはミドルウェアの差し替えで移れる」と
// 書いていたのがこれ。
//
// トークンそのものは保存されていないので、比較は起きない。ハッシュで
// 引くだけなので、定数時間比較も要らない。
//
// now を引数で受けるのは、期限切れのテストが書けなくなるため。
// 期限の判定はセッションの店に委ねる（規則を2箇所に置かない）。
func RequireSession(
	sessions account.SessionReader, now func() time.Time,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if unauthenticatedPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			user, ok := resolveSession(w, r, sessions, now())
			if !ok {
				return
			}

			// 利用者を context に載せるのはここだけ。ハンドラが
			// 取り出したら終わりで、その先へは引数で渡す（user.go）。
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
		})
	}
}

// resolveSession はトークンから利用者を引く。引けなければ応答を書く。
func resolveSession(
	w http.ResponseWriter, r *http.Request,
	sessions account.SessionReader, now time.Time,
) (account.UserID, bool) {
	raw, ok := bearerToken(r)
	if !ok {
		// WWW-Authenticate を付けるのは、クライアントが
		// 「認証が要る」と「壊れている」を区別できるようにするため。
		unauthorized(w, "認証が必要である")
		return account.UserID{}, false
	}

	token, err := account.ParseSessionToken(raw)
	if err != nil {
		// 形が違えば、そのトークンのセッションは存在しない。
		// 保存先に問い合わせるまでもない。
		unauthorized(w, "認証に失敗した")
		return account.UserID{}, false
	}

	session, err := sessions.Find(r.Context(), token.Hash(), now)
	switch {
	case errors.Is(err, account.ErrSessionNotFound):
		// 知らない・消された・期限切れ。どれも本人がログインし直せば直る。
		// 区別して返さないのは、存在するハッシュを教えないため。
		unauthorized(w, "認証に失敗した")
		return account.UserID{}, false
	case errors.Is(err, training.ErrRepositoryUnavailable):
		// **401 にしない。**クライアントは 401 を「ログインし直せ」と読み、
		// 通るはずのトークンを捨てる。保存先が戻れば通るので 503。
		respondCoded(w, apperror.ErrUnavailable, err)
		return account.UserID{}, false
	case err != nil:
		respondCoded(w, apperror.ErrInternal, err)
		return account.UserID{}, false
	}
	return session.UserID(), true
}

func unauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="liftplan"`)
	writeError(w, http.StatusUnauthorized, message)
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
