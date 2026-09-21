package usecase

import "github.com/dyoshyy/liftplan/internal/domain/account"

// currentUser は「いま誰の記録を扱っているか」の仮の答え。
//
// **これは足場である。**リポジトリは所有者を引数で受け取るようになったが、
// 認証はまだ Bearer トークン1本で、誰がログインしているかを言えない。
// その間だけ、既定ユーザー（マイグレーション 0007 が既存の行を寄せた先）を
// 返す。
//
// 呼び出し元に直接 account.DefaultUserID() と書かず、ここに集めているのは
// **消すときのため**。次のPRで Execute が利用者を引数に取るようになったら、
// このファイルを消せばコンパイラが残った呼び出しを全部挙げる。
func currentUser() account.UserID { return account.DefaultUserID() }
