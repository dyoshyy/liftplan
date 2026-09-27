// Package apperror はアプリケーション層のエラー分類を持つ。
//
// ドメインのセンチネルは「何が起きたか」しか言わない。それを HTTP の
// どれに写すかはアプリケーションの都合なので、ドメインには置けない。
// かといって presentation に置くと、分類のためにドメインの全パッケージを
// import することになり、センチネルを足すたびに presentation が動く。
//
// 間に1つ型を挟む。Classify がドメインのセンチネルをここに翻訳し、
// presentation は errors.As でこの型だけを見る。
package apperror

import (
	"errors"
	"net/http"
)

// Error は分類済みのエラー。
//
// メッセージと HTTP ステータスとコードを持つ。コードを持つのは、
// 文言を変えてもクライアントの分岐が壊れないようにするため。
type Error struct {
	code    string
	message string
	status  int
}

func newError(code, message string, status int) *Error {
	return &Error{code: code, message: message, status: status}
}

func (e *Error) Error() string   { return e.message }
func (e *Error) Code() string    { return e.code }
func (e *Error) HTTPStatus() int { return e.status }

// Is はコードで一致を見る。
//
// 値の同一性ではなくコードで比べるのは、この型を fmt.Errorf の %w に
// 渡した先で errors.Is が効くようにするため。いまは package 変数しか
// 作らないので == でも足りるが、複製を作れるようにしたときに黙って
// 壊れる場所を先に塞いでおく。
func (e *Error) Is(target error) bool {
	var other *Error
	return errors.As(target, &other) && other.code == e.code
}

var (
	// ErrInvalidInput は入力が受け付けられないこと。
	ErrInvalidInput = newError("INVALID_INPUT", "入力が不正である", http.StatusBadRequest)

	// ErrNotConfigured はプログラムが未設定であること。
	//
	// 404 ではなく 409。「まだ存在しない」ではなく「前提が満たされて
	// いない」という状態の衝突（D-042）。GET /api/program だけは
	// 「まだ存在しない」の意味なので、そちらは presentation が 404 に写す。
	ErrNotConfigured = newError("NOT_CONFIGURED", "プログラムが未設定である", http.StatusConflict)

	// ErrNotFound は取得の文脈での「まだ無い」。
	//
	// ErrNotConfigured と中身は同じ状態を指すが、意味が違う。取得では
	// 「まだ存在しない」で 404、セッション導出では「前提が満たされて
	// いない」で 409（D-042）。同じ状態でも文脈で写し先が変わるので、
	// 分類を2つ持つ。
	ErrNotFound = newError("NOT_FOUND", "プログラムが未設定である", http.StatusNotFound)

	// ErrConflict は同じIDで内容の違う記録が既にあること。
	ErrConflict = newError("CONFLICT", "記録が衝突している", http.StatusConflict)

	// ErrDuplicateName は同じ名前の種目が既にあること。
	ErrDuplicateName = newError("DUPLICATE_NAME", "同じ名前の種目がある", http.StatusConflict)

	// ErrExerciseNotFound は消そうとした種目が無いこと。共通の種目と
	// 他人の種目もここに入る（その利用者から見て「消せる種目」が無い）。
	ErrExerciseNotFound = newError("EXERCISE_NOT_FOUND", "種目が見つからない", http.StatusNotFound)

	// ErrStillDeclared は伸ばしたい種目に入っている種目を消そうとしたこと。
	//
	// 黙って宣言から外さない。伸ばしたい種目が0個になりうるのと、軸の
	// 顔ぶれが変わっても気づけないため（Program.WithSelected と同じ理由）。
	ErrStillDeclared = newError("STILL_DECLARED", "伸ばしたい種目に入っている種目は消せない", http.StatusConflict)

	// ErrUnavailable は保存先に到達できないこと。
	//
	// 500 と分けるのは、後で送り直せば通るから。混ぜるとクライアントが
	// 「送り直しても無駄」と読んで記録を捨てる。
	ErrUnavailable = newError("UNAVAILABLE", "一時的に利用できない", http.StatusServiceUnavailable)

	// ErrTooLarge はボディが上限を超えたこと。
	ErrTooLarge = newError("TOO_LARGE", "リクエストボディが大きすぎる", http.StatusRequestEntityTooLarge)

	// ErrTimeout は処理が時間内に終わらなかったこと。
	ErrTimeout = newError("TIMEOUT", "処理が時間内に終わらなかった", http.StatusGatewayTimeout)

	// ErrInternal は分類できなかったもの。
	//
	// これを返り値に使うことはない。presentation が errors.As に失敗した
	// ときの既定として使う。
	ErrInternal = newError("INTERNAL", "内部エラーが発生した", http.StatusInternalServerError)
)
