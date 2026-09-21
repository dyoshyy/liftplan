package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dyoshyy/liftplan/internal/application/apperror"
	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// userContextKey は利用者を context に載せるための鍵。
//
// 型を非公開にしているのは、httpapi の外から詰められないようにするため。
// 公開すると、どのパッケージからでも「誰か」を context に入れられる。
// 認証を通っていない値がハンドラに届く経路をそもそも作らない。
//
// 空の構造体を型として使うのは、文字列の鍵だと他のパッケージが同じ
// 文字列で上書きできるため（context.WithValue の doc が言う衝突）。
type userContextKey struct{}

// withUser は利用者を context に載せる。
//
// **設計では「UserID は引数で渡す。context に入れない」と決めている。
// 例外はここ1つだけで、理由は他に渡す手段が無いこと。**net/http の
// ハンドラは func(http.ResponseWriter, *http.Request) で形が決まっており、
// ミドルウェアからハンドラへ値を渡す口は r.Context() しかない。
//
// **例外の範囲は httpapi の中の1ホップだけ。**ハンドラが userOf で
// 取り出したら、その先（ユースケース・リポジトリ）へは引数で渡す。
// context のまま流すと、口を見ても「誰のデータか」が読めなくなり、
// 渡し忘れがコンパイルで落ちず、実行時に他人のデータを返す形で出る。
func withUser(ctx context.Context, user account.UserID) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

// userOf は context から利用者を取り出す。
func userOf(ctx context.Context) (account.UserID, bool) {
	user, ok := ctx.Value(userContextKey{}).(account.UserID)
	return user, ok
}

// requireUser はハンドラの入口で利用者を取り出す。
// 取り出せなければ応答を書いて false を返す。
//
// **取り出せなかったときに既定ユーザーで続行しない。**認証を通っていない
// のに処理が進む経路になる。ミドルウェアを外した配線ミスや、認証の外に
// 置いた経路が、そのまま「誰のものでもない要求が他人の記録を読み書きする」
// になり、応答は 204 なので誰も気づけない。
//
// 401 ではなく 500 にするのは、原因が送り主ではなくこちらの配線にあるため。
// ミドルウェアを通っていればここには来ない。401 を返すとクライアントは
// 認証をやり直すが、何度やっても通らない。500 なら記録に残り、調べられる。
func requireUser(w http.ResponseWriter, r *http.Request) (account.UserID, bool) {
	user, ok := userOf(r.Context())
	if !ok {
		respondCoded(w, apperror.ErrInternal,
			fmt.Errorf("利用者が context に無い: %s %s。認証ミドルウェアを通っていない",
				r.Method, r.URL.Path))
		return account.UserID{}, false
	}
	return user, true
}
