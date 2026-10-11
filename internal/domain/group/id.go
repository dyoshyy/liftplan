package group

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidID はグループの識別子として使えない値を表す。
var ErrInvalidID = errors.New("グループの識別子が不正である")

// ID はグループの同一性。中身は UUID の正規形。
//
// account.UserID と同じ理由で構造体で包む。ID("") と書けると、検証を
// 素通りした値で引く経路ができる。
//
// 連番にしないのは、URL（/api/groups/{id}）に出るから。推測で他の
// グループを叩けない形にしておく。叩いても VisibleTo が弾くが、
// 「存在するかどうか」も漏らさない（404 で返す）ための前提でもある。
type ID struct {
	v string
}

// uuidLength は UUID の正規形（8-4-4-4-12）の長さ。
const uuidLength = 36

// ParseID は文字列から ID を作る。URL と DB から読み戻す口。
//
// 正規化（空白の除去・小文字化）は検証の前に行う。
func ParseID(s string) (ID, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if !isUUID(v) {
		return ID{}, fmt.Errorf("%w: %q は UUID の形ではない", ErrInvalidID, s)
	}
	return ID{v: v}, nil
}

// NewRandomID は新しい ID を採番する（UUID version 4）。
//
// crypto/rand が失敗したらエラーを返す。推測できる ID を作らない。
func NewRandomID() (ID, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ID{}, fmt.Errorf("%w: 乱数を取得できない: %w", ErrInvalidID, err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	s := hex.EncodeToString(b[:])
	return ParseID(fmt.Sprintf("%s-%s-%s-%s-%s", s[0:8], s[8:12], s[12:16], s[16:20], s[20:32]))
}

// String は UUID の文字列を返す。ゼロ値では空文字になる。
func (id ID) String() string { return id.v }

// IsZero はゼロ値か。
func (id ID) IsZero() bool { return id.v == "" }

// isUUID は小文字化済みの v が UUID の正規形か。
//
// account.NewUserID と同じ検査の写し。共有の部品を立てるより、20行の
// 重複のほうが安い（account に依存を足すと、利用者の識別子の都合で
// グループの識別子が変わりうる）。
func isUUID(v string) bool {
	if len(v) != uuidLength {
		return false
	}
	for i := range uuidLength {
		switch i {
		case 8, 13, 18, 23:
			if v[i] != '-' {
				return false
			}
		default:
			c := v[i]
			if !('0' <= c && c <= '9') && !('a' <= c && c <= 'f') {
				return false
			}
		}
	}
	return true
}
