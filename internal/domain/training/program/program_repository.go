package program

import (
	"context"
	"errors"
)

// ErrProgramNotConfigured はユーザーがまだプログラムを設定していないことを表す。
//
// 「エラー」ではなく状態なので、呼び出し側が errors.Is で判別して
// 初期設定へ誘導できるようセンチネルにする。
var ErrProgramNotConfigured = errors.New("プログラムが未設定である")

// ErrNoDeclaredExercise は伸ばしたい種目が1つも選ばれていないことを表す。
//
// SessionPlanner はヘビー枠ゼロを致命エラーにする。設定の時点で弾かないと、
// 保存は成功するのに以後すべてのセッション導出が失敗する。
//
// 以前は ErrNoMainExercise という名前で、判定はアプリケーション層にあった。
// Program が種目の Kind を知らず、どれがメインかを自分で言えなかったため。
// 宣言を Program が持つようになって、集約が自分で守れるようになった（D-117）。
var ErrNoDeclaredExercise = errors.New("伸ばしたい種目が1つも選ばれていない")

// Reader はユーザー設定の取得口。
//
// Get は未設定の場合 ErrProgramNotConfigured を返す。(nil, nil) を
// 返してはならない。呼び出し側が nil を「未設定」と「取得成功」の
// どちらとも解釈できてしまう。
type Reader interface {
	Get(ctx context.Context) (*Program, error)
}

// Writer はユーザー設定の保存口。
//
// Save は冪等であること。プログラムはユーザーごとに1つで、
// 保存は常に全体の置き換えになる。
type Writer interface {
	Save(ctx context.Context, p *Program) error
}
