package runlog

import (
	"context"
	"errors"

	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// ErrConflictingRunLog は同じIDで内容の異なる記録が送られたことを表す。
//
// 同じ内容の再送は黙って受け入れる（冪等）。内容が違うなら、クライアントの
// 採番が壊れている。黙って上書きすると、どちらが正しいか分からなくなる。
var ErrConflictingRunLog = errors.New("同じIDで内容の異なるランの記録がある")

// Reader はランの記録の取得口。
//
// FindAll は実施日の昇順、同じ日の中は ID の昇順で返す。順序が揺れると、
// 同じ記録から違う一覧が出る。
type Reader interface {
	FindAll(ctx context.Context, userID account.UserID) ([]*RunLog, error)
}

// Writer はランの記録の書き込み口。
//
// Save は冪等。同じIDで同じ内容なら重複を作らず成功し、内容が違えば
// ErrConflictingRunLog を返す。クライアントはオフラインで記録して後から
// 送るので、再送は日常的に起きる。
//
// Delete は打ち間違いの訂正のためにある。存在しないIDの削除は成功として
// 扱う（再送で二度目が来ても「消えているのに消せない」にしない）。
//
// 所有者をまたがないこと。別の利用者が同じIDを使っても衝突にせず、
// Delete は自分の記録にしか届かない（setlog.Writer と同じ）。
type Writer interface {
	Save(ctx context.Context, userID account.UserID, log *RunLog) error
	Delete(ctx context.Context, userID account.UserID, id RunLogID) error
}
