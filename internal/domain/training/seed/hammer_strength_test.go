package seed_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// Hammer Strength のマシンがプリセットに入っていること。
//
// 公式カタログ（Plate-Loaded・MTS・Select の3系列）のうち、筋区分に寄与を
// 付けられるマシンを、1台1種目で入れている。同じ動作でも、系列やマシンが違えば
// 重量の刻みも感触も違い、推定1RMは種目ごとに持つので、別の種目にする。
func TestHammerStrengthPresets_AreInTheCatalog(t *testing.T) {
	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	hs := 0
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, e := range all {
		id, name := string(e.ID()), e.Name()
		if ids[id] {
			t.Errorf("ID が重複している: %s", id)
		}
		if names[name] {
			t.Errorf("名前が重複している: %s", name)
		}
		ids[id], names[name] = true, true

		if strings.HasPrefix(id, "hs_") {
			hs++
			// 一覧で、汎用の種目（レッグプレスなど）と見分けがつくこと。
			if !strings.HasPrefix(name, "HS ") {
				t.Errorf("%s の名前が「HS 」で始まっていない: %s", id, name)
			}
			// 系列（プレート・MTS・セレクト）が名前の末尾に付いていること。同じ動作の
			// マシンが系列ごとにあり、名前だけでは区別できなくなるため。
			if !strings.HasSuffix(name, "）") {
				t.Errorf("%s の名前に系列が付いていない: %s", id, name)
			}
			if n := utf8.RuneCountInString(name); n > 40 {
				t.Errorf("%s の名前が長すぎる（%d字）: %s", id, n, name)
			}
		}
	}
	if hs < 60 {
		t.Errorf("Hammer Strength のプリセットが %d 種目しか無い。3系列で80種目前後のはず", hs)
	}
}

// Hammer Strength のマシンは、新規の利用者が最初から使う種目には入らないこと。
//
// 入ると、足すたびに新規の利用者の計画が動き、使わない種目が一覧に並ぶ。
// 使うには種目の管理で「使う」にする。
func TestHammerStrengthPresets_AreNotUsedByDefault(t *testing.T) {
	for _, id := range seed.DefaultSelected() {
		if strings.HasPrefix(string(id), "hs_") {
			t.Errorf("%s が既定で使う種目に入っている", id)
		}
	}

	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}
	prog, err := seed.DefaultProgram(all)
	if err != nil {
		t.Fatalf("初期プログラムが不正: %v", err)
	}
	for _, e := range all {
		if strings.HasPrefix(string(e.ID()), "hs_") && prog.Includes(e.ID()) {
			t.Errorf("%s が初期プログラムの使う種目に入っている", e.ID())
		}
	}
}
