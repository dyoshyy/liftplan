package exercise

import (
	"context"
	"errors"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// ErrExerciseNotFound は種目マスタに存在しない種目IDを指したことを表す。
//
// Program は種目マスタを知らないので、選択された種目IDが実在するかを
// 自分で検証できない。突合しないと、シードから種目を1つ消したり
// リネームしたりした瞬間、既存ユーザーのメニューからその種目が
// 理由の説明なく消える。
var ErrExerciseNotFound = errors.New("種目が見つからない")

// Reader は種目の取得口。
//
// 共通の種目（シード）に、その利用者が足した種目を加えて返す。所有者を
// 引数で受けるのは program.Reader と同じ理由で、他人の種目が見えては
// いけない。
//
// FindAll が返すスライスと要素は、呼び出し側が自由に扱ってよい。
// リポジトリ内部の可変状態をエイリアスして返してはならない。
type Reader interface {
	FindAll(ctx context.Context, user account.UserID) ([]*Exercise, error)
}
