package program_test

import (
	"strings"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// このパッケージのテストだけで使う小さなヘルパー。
//
// 分割前は1つのテストパッケージに置いていた。共有用のパッケージを
// 立てるより、数行の重複のほうが安い。

func mustFrequency(t *testing.T, n int) program.Frequency {
	t.Helper()
	f, err := program.NewFrequency(n)
	if err != nil {
		t.Fatalf("NewFrequency(%d): %v", n, err)
	}
	return f
}

// decimalPlaces は文字列表現の小数点以下の桁数。
func decimalPlaces(s string) int {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		return 0
	}
	return len(s) - dot - 1
}
