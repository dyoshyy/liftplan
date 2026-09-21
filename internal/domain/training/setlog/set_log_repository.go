package setlog

import (
	"context"

	"errors"
	"github.com/dyoshyy/liftplan/internal/domain/account"
)

// ErrConflictingSetLog は同じIDで内容の異なるログが送られたことを表す。
//
// 再送は同じ内容なら黙って受け入れる（冪等）。内容が違うなら、
// クライアントのID採番が壊れているか、別のセッションのログを
// 上書きしようとしている。黙って上書きすると、どちらが正しいか
// 分からないまま推定1RMが動く。
var ErrConflictingSetLog = errors.New("同じIDで内容の異なるセットログがある")

// Reader は実績ログの取得口。
//
// FindAll が返す History は、リポジトリ内部の可変状態を
// エイリアスしてはならない。Save と並行に呼ばれる。
//
// 所有者は引数で受け取る。context に入れないのは、口の形を見ても
// 「誰のデータか」が読めなくなるため。渡し忘れがコンパイルで落ちず、
// 実行時に他人のデータを返す形で出る。
type Reader interface {
	FindAll(ctx context.Context, userID account.UserID) (History, error)
}

// Writer は実績ログの書き込み口。
//
// Save は次を満たすこと。
//
//   - 冪等である。同じIDで同じ内容のログを二度送っても重複が生まれない。
//     クライアントはオフラインで記録して後からまとめて送るので、
//     タイムアウト後の再送は日常的に起きる。
//   - 同じIDで内容が異なる場合は ErrConflictingSetLog を返す。
//     どちらを採用するかを実装が勝手に決めると、リポジトリが返す順序
//     次第で推定1RMが変わり、同じ入力から違う計画が出る。
//   - 全か無かで書く。途中まで保存された状態を残さない。
//     1セッション25セットの半分だけが残ると、その週の刺激量が
//     実態と食い違ったまま計画に効き続ける。
//
// Delete は打ち間違いの訂正のためにある。
//
// SetLog は「確定した実績1セット」で、生成後は変更しない。削除はその
// 前提と衝突するように見えるが、意味が違う。値を書き換えるのではなく
// 「これは起きなかった」と言っている。
//
// 訂正の手段が無いほうが害が大きい。同じIDで直そうとすると衝突として
// 弾かれ（D-060）、別IDで正しい値を入れても間違った記録は残り続けて
// 推定1RMを汚す。単一ユーザーの個人アプリで、消せない記録に価値は無い。
//
// 存在しないIDの削除は成功として扱う。再送で二度目が来ることがあり、
// そこでエラーにすると「消えているのに消せない」という状態になる。
//
// # 所有者
//
// IDはクライアントが採番するので、利用者をまたぐと衝突しうる。実装は
// 次の2つを守ること。破ると、他人の記録の存在が分かり、他人の記録を消せる。
//
//   - 別の利用者が同じIDを使っても ErrConflictingSetLog にしない
//   - Delete は自分の記録にしか届かない
type Writer interface {
	Save(ctx context.Context, userID account.UserID, logs []*SetLog) error
	Delete(ctx context.Context, userID account.UserID, id SetLogID) error
}
