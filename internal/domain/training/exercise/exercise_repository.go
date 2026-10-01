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
// 種目は利用者ごとの一覧で、共通/個人には分かれていない（Reader のコメント）
// ので、比べる相手はその利用者の一覧のうち、まだ消していない種目全部。
// 一覧で見分けられなくなるのと、二度押しで同じ種目が2つできるのを止める。
var ErrDuplicateExerciseName = errors.New("同じ名前の種目がある")

// Reader は種目の取得口。
//
// 種目は利用者ごとの一覧で、共通/個人には分かれていない。プリセットは
// その一覧の初期値としてその人の行に一度だけコピーされ、以後はその人の
// ものとして FindAll がそのまま返す。所有者を引数で受けるのは
// program.Reader と同じ理由で、他人の種目が見えてはいけない。
//
// FindAll が返すスライスと要素は、呼び出し側が自由に扱ってよい。
// リポジトリ内部の可変状態をエイリアスして返してはならない。
type Reader interface {
	FindAll(ctx context.Context, user account.UserID) ([]*Exercise, error)
}

// Writer は種目の保存口。
//
// プリセット由来かどうかで扱いを変えない。共通の種目（シード）も、利用者が
// 足した種目も同じ経路で受け取る。
//
// 同じ ID を二度渡したら上書きする（消すときは Delete した値を渡す）。
// 所有者を引数で受ける理由は Reader と同じ。
type Writer interface {
	Save(ctx context.Context, user account.UserID, e *Exercise) error
}
