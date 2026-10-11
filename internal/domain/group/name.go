package group

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidName はグループ名・表示名として使えない値を表す。
var ErrInvalidName = errors.New("名前が不正である")

// maxNameLength は名前の長さの上限（文字数）。分割の名前と同じ20文字。
// 画面の1行に収まる長さ。
const maxNameLength = 20

// Name はグループの名前（「ジム仲間」など）。
type Name struct {
	v string
}

// NewName はグループ名を作る。前後の空白を除いてから長さを見る。
func NewName(s string) (Name, error) {
	v, err := normalizeName(s)
	if err != nil {
		return Name{}, fmt.Errorf("グループ名: %w", err)
	}
	return Name{v: v}, nil
}

func (n Name) String() string { return n.v }

// DisplayName はグループの中での自分の呼ばれ方。
//
// グループごとに持つ。ジムではあだ名、同僚には本名、と場所で呼ばれ方が
// 違う。一意にはしない。同じグループに「たろう」が2人いても、困るのは
// 見分けにくいことだけで、データは壊れない。
type DisplayName struct {
	v string
}

// NewDisplayName は表示名を作る。
func NewDisplayName(s string) (DisplayName, error) {
	v, err := normalizeName(s)
	if err != nil {
		return DisplayName{}, fmt.Errorf("表示名: %w", err)
	}
	return DisplayName{v: v}, nil
}

func (n DisplayName) String() string { return n.v }

// normalizeName は空白を除いてから長さを検査する。
//
// 正規化を先にするのは、検査を通った値が正規化で別物になるのを避けるため
// （CLAUDE.md の quantize と同じ順序）。
func normalizeName(s string) (string, error) {
	v := strings.TrimSpace(s)
	if v == "" {
		return "", fmt.Errorf("%w: 空である", ErrInvalidName)
	}
	if n := len([]rune(v)); n > maxNameLength {
		return "", fmt.Errorf("%w: %d文字（%d文字まで）", ErrInvalidName, n, maxNameLength)
	}
	return v, nil
}
