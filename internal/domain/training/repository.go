package training

import (
	"context"
	"errors"
)

// ErrProgramNotConfigured はユーザーがまだプログラムを設定していないことを表す。
//
// 「エラー」ではなく状態なので、呼び出し側が errors.Is で判別して
// 初期設定へ誘導できるようセンチネルにする。
var ErrProgramNotConfigured = errors.New("プログラムが未設定である")

// ErrExerciseNotFound は種目マスタに存在しない種目IDを指したことを表す。
//
// Program は種目マスタを知らないので、選択された種目IDが実在するかを
// 自分で検証できない。突合しないと、シードから種目を1つ消したり
// リネームしたりした瞬間、既存ユーザーのメニューからその種目が
// 理由の説明なく消える。
var ErrExerciseNotFound = errors.New("種目が見つからない")

// ErrNoDeclaredExercise は伸ばしたい種目が1つも選ばれていないことを表す。
//
// SessionPlanner はヘビー枠ゼロを致命エラーにする。設定の時点で弾かないと、
// 保存は成功するのに以後すべてのセッション導出が失敗する。
//
// 以前は ErrNoMainExercise という名前で、判定はアプリケーション層にあった。
// Program が種目の Kind を知らず、どれがメインかを自分で言えなかったため。
// 宣言を Program が持つようになって、集約が自分で守れるようになった（D-117）。
var ErrNoDeclaredExercise = errors.New("伸ばしたい種目が1つも選ばれていない")

// ErrRepositoryUnavailable は保存先そのものに到達できないことを表す。
//
// 「入力が悪い」でも「サーバーのバグ」でもなく、一時的に扱えない状態。
// 500 と混ぜると、クライアントは「送り直しても無駄」と読んで諦める。
// 実際には後で送り直せば通るので、区別できる形で返す。
var ErrRepositoryUnavailable = errors.New("保存先に到達できない")

// ErrConflictingSetLog は同じIDで内容の異なるログが送られたことを表す。
//
// 再送は同じ内容なら黙って受け入れる（冪等）。内容が違うなら、
// クライアントのID採番が壊れているか、別のセッションのログを
// 上書きしようとしている。黙って上書きすると、どちらが正しいか
// 分からないまま推定1RMが動く。
var ErrConflictingSetLog = errors.New("同じIDで内容の異なるセットログがある")

// ExerciseRepository は種目マスタの取得口。
//
// FindAll が返すスライスと要素は、呼び出し側が自由に扱ってよい。
// リポジトリ内部の可変状態をエイリアスして返してはならない。
type ExerciseRepository interface {
	FindAll(ctx context.Context) ([]*Exercise, error)
}

// SetLogRepository は実績ログの永続化口。
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
// FindAll が返す History は、リポジトリ内部の可変状態を
// エイリアスしてはならない。Save と並行に呼ばれる。
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
type SetLogRepository interface {
	FindAll(ctx context.Context) (History, error)
	Save(ctx context.Context, logs []*SetLog) error
	Delete(ctx context.Context, id SetLogID) error
}

// ConditionRepository は日次コンディションの永続化口。
//
// Save は冪等であること。同じ日付を二度送ったら、後から来た値で
// 項目ごとに上書きする（DailyCondition.Merge と同じ規則）。
// 日付ごと置き換えると、体重だけを送ったときに睡眠時間が消える。
//
// 全か無かで書くこと。FindAll が返す ConditionLog は、リポジトリ内部の
// 可変状態をエイリアスしてはならない。
type ConditionRepository interface {
	FindAll(ctx context.Context) (ConditionLog, error)
	Save(ctx context.Context, items []DailyCondition) error
}

// ProgramRepository はユーザー設定の取得・保存口。
//
// Get は未設定の場合 ErrProgramNotConfigured を返す。(nil, nil) を
// 返してはならない。呼び出し側が nil を「未設定」と「取得成功」の
// どちらとも解釈できてしまう。
//
// Save は冪等であること。プログラムはユーザーごとに1つで、
// 保存は常に全体の置き換えになる。
type ProgramRepository interface {
	Get(ctx context.Context) (*Program, error)
	Save(ctx context.Context, p *Program) error
}
