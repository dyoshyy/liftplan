package account

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidUserID は利用者の識別子として使えない値を受け取ったことを表す。
var ErrInvalidUserID = errors.New("利用者の識別子が不正である")

// UserID は利用者の同一性。
//
// 定義型（type UserID string）にしないのは、UserID("") と誰でも書けて
// コンストラクタを素通りするため。構造体で包めば、正規化と検証を通った
// 値しか存在しない。
//
// 中身は UUID の文字列。数値の連番にしないのは、他人の記録を推測で
// 叩ける形になるため。URL にもレスポンスにも出さない前提だが、
// 「出しても困らない」ほうを選んでおく。
type UserID struct {
	v string
}

// 正規の UUID 文字列（8-4-4-4-12）の形。
const (
	userIDLength = 36
	// ハイフンが入る位置。
	hyphenAt1 = 8
	hyphenAt2 = 13
	hyphenAt3 = 18
	hyphenAt4 = 23
)

// NewUserID は文字列から UserID を作る。
//
// 空白の除去と小文字への正規化は、検証の前に行う。後にすると、検証を
// 通った値が正規化で別物になる（training の quantize と同じ理由）。
//
// 受け付けるのは UUID の正規形だけ。ハイフンを抜いた32文字や波括弧付きの
// 表記を受け入れると、同じ人が表記違いで複数の UserID を持ちうる。
// 入り口は Postgres の uuid 型と OAuth の受け入れ処理の2つしかなく、
// どちらも正規形で寄こす。
func NewUserID(s string) (UserID, error) {
	v := strings.ToLower(strings.TrimSpace(s))

	if len(v) != userIDLength {
		return UserID{}, fmt.Errorf("%w: %q は UUID の形ではない", ErrInvalidUserID, s)
	}
	for i := range userIDLength {
		switch i {
		case hyphenAt1, hyphenAt2, hyphenAt3, hyphenAt4:
			if v[i] != '-' {
				return UserID{}, fmt.Errorf(
					"%w: %q は UUID の形ではない", ErrInvalidUserID, s)
			}
		default:
			if !isHexDigit(v[i]) {
				return UserID{}, fmt.Errorf(
					"%w: %q は UUID の形ではない", ErrInvalidUserID, s)
			}
		}
	}
	return UserID{v: v}, nil
}

// isHexDigit は16進1桁かを返す。小文字化済みの前提。
func isHexDigit(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f')
}

// String は UUID の文字列を返す。ゼロ値では空文字になる。
func (id UserID) String() string { return id.v }

// defaultUserIDText はマイグレーション 0007 が既存の行に埋めた UUID。
//
// マルチユーザー化より前の記録には持ち主が無い。誰のものでもない状態を
// 作らないために、1人分の UUID を決めて全行をそこに寄せている。
//
// **この定数は仮の足場である。**OAuth を入れたら、本人のアカウントを
// この UUID に結ぶ1行を流し（手順は docs/deploy.md）、認証が本物の UserID を
// 渡すようになる。そうなれば DefaultUserID を呼ぶ場所は無くなるので、
// このファイルから消す。いま呼んでいるのは認証ミドルウェア
// （httpapi.RequireBearerToken）と起動時の初期プログラム投入
// （cmd/api の seedProgramIfMissing）の2箇所だけ。
const defaultUserIDText = "8d5e743e-f1b0-4430-9998-89d313e89da8"

// DefaultUserID は既定ユーザー。マイグレーション 0007 の既定値と同じ値を返す。
//
// 値オブジェクトなので定数にはできない。var で公開すると誰でも書き換えられ、
// 「既定ユーザーが実行時に変わる」という説明のつかない状態を作れてしまう。
// 関数にして、返した値の書き換えが呼び手に閉じるようにする。
func DefaultUserID() UserID {
	// ここで失敗しうるのは、上の定数を壊したときだけ。壊れたまま
	// 起動して「誰のものでもない記録」を作るより、その場で落ちるほうがよい。
	id, err := NewUserID(defaultUserIDText)
	if err != nil {
		panic(fmt.Sprintf("既定ユーザーの UUID が不正: %v", err))
	}
	return id
}
