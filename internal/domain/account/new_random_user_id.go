package account

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// uuidBytes は UUID の長さ（128ビット）。
const uuidBytes = 16

// UUID の version と variant を書き込む位置。
const (
	versionByte = 6
	variantByte = 8
)

// NewRandomUserID は新しい利用者の識別子を採番する。
//
// 採番をドメインに置いたのは、SessionToken と同じ理由。「UserID とは
// UUID である」はこのパッケージが決めていること（NewUserID が正規形の
// UUID しか受け付けない）で、どう作るかはその裏返しでしかない。
// アプリケーション層に置くと、UUID の版と乱数源が呼び出し側ごとに
// 決まることになり、NewUserID の検証は通るが version も variant も
// 立っていない値が混じりうる。標準ライブラリしか使わないので、
// 「DBもHTTPも立てずに全機能をテストできる」は壊れない（D-118）。
//
// crypto/rand が失敗したらエラーを返す。math/rand へ落とさない。
// 落とすと、推測できる UserID を「採番できた」として返すことになり、
// 他人の記録を当てにいける形になる。
func NewRandomUserID() (UserID, error) {
	var b [uuidBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return UserID{}, fmt.Errorf("%w: 乱数を取得できない: %w", ErrInvalidUserID, err)
	}

	// version 4（乱数由来）と RFC 4122 の variant。
	// 立てないと「16進を並べただけの32文字」になり、uuid 型を持つ
	// 他のツールから見て不正な UUID になる。
	b[versionByte] = (b[versionByte] & 0x0f) | 0x40
	b[variantByte] = (b[variantByte] & 0x3f) | 0x80

	s := hex.EncodeToString(b[:])
	// 8-4-4-4-12 に区切る。NewUserID を通すのは、ここで組んだ文字列が
	// 「読み戻せる形」であることを1箇所で担保するため。組み方を間違えれば
	// 採番の時点で落ちる。書けるのに読めない値が DB に入るより早い。
	return NewUserID(fmt.Sprintf("%s-%s-%s-%s-%s",
		s[0:8], s[8:12], s[12:16], s[16:20], s[20:32]))
}
