package exercise

import (
	"context"
	"errors"
)

// ErrExerciseNotFound は種目マスタに存在しない種目IDを指したことを表す。
//
// Program は種目マスタを知らないので、選択された種目IDが実在するかを
// 自分で検証できない。突合しないと、シードから種目を1つ消したり
// リネームしたりした瞬間、既存ユーザーのメニューからその種目が
// 理由の説明なく消える。
var ErrExerciseNotFound = errors.New("種目が見つからない")

// Reader は種目マスタの取得口。
//
// 種目マスタはシードから流し込まれるだけで、実行時に書き換わらない。
// 書き手が居ないので Writer は無い。必要になってから足す。
//
// FindAll が返すスライスと要素は、呼び出し側が自由に扱ってよい。
// リポジトリ内部の可変状態をエイリアスして返してはならない。
type Reader interface {
	FindAll(ctx context.Context) ([]*Exercise, error)
}
