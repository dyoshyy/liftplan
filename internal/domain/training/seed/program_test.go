package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// 初期プログラムにバリエーションを含めないこと。
// 含めると、バリエーションがメイン扱いで独立したスロットを持つ。
func TestDefaultProgram_ExcludesVariations(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	prog, err := seed.DefaultProgram(pool)
	if err != nil {
		t.Fatalf("初期プログラムが不正: %v", err)
	}

	// 全種目が選ばれていること。かつてバリエーションは選択に入れずとも
	// 自動で回っていたが、その抜け道を塞いだので明示的に選ぶ必要がある。
	for _, e := range pool {
		if !prog.Includes(e.ID()) {
			t.Errorf("%s が選択に含まれていない", e.ID())
		}
	}
}

// 初期プログラムの頻度が README の記述と一致すること。
func TestDefaultProgram_MatchesTheDocumentedFrequency(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	prog, err := seed.DefaultProgram(pool)
	if err != nil {
		t.Fatalf("初期プログラムが不正: %v", err)
	}
	if got := prog.Frequency().PerWeek(); got != 3 {
		t.Errorf("既定の頻度が誤り: %d（README は週3回と書いている）", got)
	}
}

// 空のプールから初期プログラムを組もうとしたら失敗すること。
//
// 失敗しないなら、種目が1つも無いプログラムが保存でき、シードが空でも
// サーバーが起動してしまう。元は cmd/api/main_test.go にあった検査で、
// defaultProgram がここへ移ったのに合わせて移した。
func TestDefaultProgram_FailsOnEmptyPool(t *testing.T) {
	if _, err := seed.DefaultProgram(nil); err == nil {
		t.Error("空のプールで初期プログラムが組めてしまう")
	}
}
