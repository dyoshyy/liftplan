package group

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidInviteToken は招待トークンとして使えない値を表す。
var ErrInvalidInviteToken = errors.New("招待トークンが不正である")

// inviteTokenBytes は招待トークンの乱数バイト数。128ビット。
const inviteTokenBytes = 16

// InviteToken は招待リンクの鍵。知っている人はグループに入れる。
//
// セッショントークンと違ってハッシュにせず、そのまま保存する。メンバーは
// いつでも同じリンクを見せ直すので、元の値を持っていないと共有のたびに
// 作り直すことになる。DB が漏れて失うのは「入れば日と種目名が見える権利」で、
// その中身は set_logs と一緒に既に漏れている（仕様「保存」）。
type InviteToken struct {
	v string
}

// NewInviteToken は新しいトークンを作る。
//
// URL のクエリに載るので、URL 安全な base64 でパディングを付けない。
func NewInviteToken() (InviteToken, error) {
	b := make([]byte, inviteTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return InviteToken{}, fmt.Errorf("%w: 乱数を取得できない: %w", ErrInvalidInviteToken, err)
	}
	return InviteToken{v: base64.RawURLEncoding.EncodeToString(b)}, nil
}

// ParseInviteToken は送られてきた文字列をトークンにする。
//
// 形は検査しない（当てずっぽうは引けないことで表れる）。空だけを弾く。
// 空を通すと「空のトークン」で引く経路ができる。
func ParseInviteToken(s string) (InviteToken, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return InviteToken{}, fmt.Errorf("%w: 空である", ErrInvalidInviteToken)
	}
	return InviteToken{v: v}, nil
}

// String はトークンの文字列を返す。
func (t InviteToken) String() string { return t.v }
