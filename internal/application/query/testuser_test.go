package query_test

import "github.com/dyoshyy/liftplan/internal/domain/account"

// testUser はテストが使う利用者。**値そのものに意味は無い。**
//
// 以前はここに testUser を書いていた。あれは
// マイグレーション 0007 が既存の行を寄せた先で、認証が「誰が」を
// 言えるようになった時点で役目が終わっている。テストが
// 「適当な1人」を要るだけなら、その1人をここで作る。
var testUser = mustTestUserID("11111111-1111-4111-8111-111111111111")

func mustTestUserID(s string) account.UserID {
	id, err := account.NewUserID(s)
	if err != nil {
		panic(err)
	}
	return id
}
