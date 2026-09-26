package account

import (
	"errors"
	"fmt"
)

// ErrInvalidIdentity は認可先から受け取った身元が使えないことを表す。
var ErrInvalidIdentity = errors.New("認可先から受け取った身元が不正である")

// Identity は認可先から受け取った「誰か」。
//
// Subject はそのプロバイダの中で不変な識別子で、**これだけが必須**。
// Email は確認済みのメールアドレスで、取れないことがある。
//
// # なぜ Email を持つか
//
// GitHub の数値IDと Google の sub は別の名前空間で、「同じ人だ」と
// 言える情報がそれ以外に無い。確認済みアドレスだけが、2つのプロバイダを
// またいで同じ人を指せる唯一の手がかりになる。
//
// **確認済みでないアドレスを入れてはいけない。**未確認のアドレスで
// 結ぶと、他人のアドレスを登録したアカウントを作るだけでその人の記録を
// 乗っ取れる。どちらのプロバイダも確認の済んだものだけを verified として
// 返すので、そこを通った値だけをここへ入れる（取り出しは
// internal/infrastructure/oauth）。
type Identity struct {
	provider Provider
	subject  string
	email    Email
}

// NewIdentity は認可先から受け取った身元を組む。
//
// subject が空なら失敗する。空を通すと、**識別子を持たない全員が
// 同一人物になる**。アドレスは空でよい。
func NewIdentity(provider Provider, subject string, email Email) (Identity, error) {
	if subject == "" {
		return Identity{}, fmt.Errorf("%w: 識別子が空である", ErrInvalidIdentity)
	}
	if provider == (Provider{}) {
		return Identity{}, fmt.Errorf("%w: 認可先が空である", ErrInvalidIdentity)
	}
	return Identity{provider: provider, subject: subject, email: email}, nil
}

func (i Identity) Provider() Provider { return i.provider }
func (i Identity) Subject() string    { return i.subject }
func (i Identity) Email() Email       { return i.email }
