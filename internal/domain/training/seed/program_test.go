package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// 初期プログラムが使う種目は DefaultSelected だけ。
//
// カタログの全種目を使うことにすると、プリセットを足すたびに、新規の利用者の
// 使う種目と計画が動く。伸ばしたい種目（BIG3）は使う種目に含まれること
// （含まれないと NewProgram が弾く）。
func TestDefaultProgram_SelectsOnlyTheDefaultSelected(t *testing.T) {
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	prog, err := seed.DefaultProgram(pool)
	if err != nil {
		t.Fatalf("初期プログラムが不正: %v", err)
	}

	want := map[exercise.ExerciseID]bool{}
	for _, id := range seed.DefaultSelected() {
		want[id] = true
		if !prog.Includes(id) {
			t.Errorf("%s が使う種目に含まれていない", id)
		}
	}
	for _, e := range pool {
		if !want[e.ID()] && prog.Includes(e.ID()) {
			t.Errorf("%s は既定で使う種目ではないのに含まれている", e.ID())
		}
	}
	if got := len(prog.SelectedExercises()); got != len(want) {
		t.Errorf("使う種目が %d 件。%d 件のはず", got, len(want))
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
