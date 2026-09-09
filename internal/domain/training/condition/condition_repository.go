package condition

import (
	"context"
)

// Reader は日次コンディションの取得口。
//
// FindAll が返す ConditionLog は、リポジトリ内部の可変状態を
// エイリアスしてはならない。
type Reader interface {
	FindAll(ctx context.Context) (ConditionLog, error)
}

// Writer は日次コンディションの書き込み口。
//
// Save は冪等であること。同じ日付を二度送ったら、後から来た値で
// 項目ごとに上書きする（DailyCondition.Merge と同じ規則）。
// 日付ごと置き換えると、体重だけを送ったときに睡眠時間が消える。
//
// 全か無かで書くこと。
type Writer interface {
	Save(ctx context.Context, items []DailyCondition) error
}
