package training

import (
	"errors"
)

// ErrRepositoryUnavailable は保存先そのものに到達できないことを表す。
//
// 「入力が悪い」でも「サーバーのバグ」でもなく、一時的に扱えない状態。
// 500 と混ぜると、クライアントは「送り直しても無駄」と読んで諦める。
// 実際には後で送り直せば通るので、区別できる形で返す。
var ErrRepositoryUnavailable = errors.New("保存先に到達できない")
