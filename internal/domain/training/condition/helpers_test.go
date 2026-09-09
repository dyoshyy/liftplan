package condition_test

import (
	"strings"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// このパッケージのテストだけで使う小さなヘルパー。
//
// 分割前は1つのテストパッケージに置いていた。共有用のパッケージを
// 立てるより、数行の重複のほうが安い。

// baseDay は履歴の起点。8月1日を1日目とする。
func baseDay(day int) training.Date {
	return training.MustDate(2026, time.August, 1).AddDays(day - 1)
}

// decimalPlaces は文字列表現の小数点以下の桁数。
func decimalPlaces(s string) int {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return 0
	}
	return len(s) - dot - 1
}
