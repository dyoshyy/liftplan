package httpapi

// テストからだけ見える口。
//
// context に載った利用者は、外から詰められないよう鍵の型を非公開に
// している（user.go）。取り出す側も非公開なので、外部テストが
// 「載っているか」を確かめる手段がこれしか無い。
var UserForTest = userOf
