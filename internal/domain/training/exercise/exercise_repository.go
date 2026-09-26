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

// ErrDuplicateExerciseName は同じ名前の種目が既にあることを表す。
//
// 比べる相手は、共通の種目と、その利用者のまだ消していない種目。
// 一覧で見分けられなくなるのと、二度押しで同じ種目が2つできるのを止める。
var ErrDuplicateExerciseName = errors.New("同じ名前の種目がある")

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

// Writer は利用者が足した種目の保存口。
//
// 共通の種目（IsCustom が false）は渡さない。共通の種目はバイナリ同梱で、
// 実行時に書き換わらない。渡されたら実装はエラーを返す。
//
// 同じ ID を二度渡したら上書きする（消すときは Delete した値を渡す）。
// 所有者を引数で受ける理由は Reader と同じ。
type Writer interface {
	Save(ctx context.Context, user account.UserID, e *Exercise) error
}
