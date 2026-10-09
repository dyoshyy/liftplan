package seed_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// Hammer Strength のプリセットは、本人が通うジムにあるマシンだけであること。
//
// 公式カタログを全部入れると多すぎ、使わないニッチなマシンが並ぶので、
// エニタイムフィットネス 品川中延店のマシンラインナップ（2026-10-03 時点）に
// 載っていて、機種名が Hammer Strength のカタログと一致するものに絞った
// （hammerStrengthSpecs のコメント）。増やすときは、この一覧と出典を一緒に直す。
func TestHammerStrengthPresets_AreOnlyTheMachinesAtTheGym(t *testing.T) {
	want := map[string]bool{
		"hs_pl_iso_incline_press": true, "hs_pl_iso_decline_chest_press": true,
		"hs_pl_iso_shoulder_press": true, "hs_pl_iso_row": true, "hs_pl_iso_high_row": true,
		"hs_pl_iso_low_row": true, "hs_pl_iso_dy_row": true, "hs_pl_iso_wide_pulldown": true,
		"hs_pl_iso_front_pulldown": true,
		"hs_pl_t_bar_row":          true, "hs_pl_lateral_raise": true, "hs_pl_seated_biceps": true,
		"hs_pl_linear_leg_press": true, "hs_pl_hack_squat": true, "hs_pl_reverse_v_squat": true,
		"hs_pl_glute_drive": true,
	}

	all, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	got := map[string]bool{}
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

		if !strings.HasPrefix(id, "hs_") {
			continue
		}
		got[id] = true
		// 一覧で、汎用の種目（レッグプレスなど）と見分けがつくこと。見分けるのは
		// 頭の「HS 」で足りる。末尾の「（プレート）」は長いだけなので付けない
		// （本人の依頼で外した。2026-10-09）。
		if !strings.HasPrefix(name, "HS ") || strings.Contains(name, "プレート）") {
			t.Errorf("%s の名前が「HS ＋ 機種名」でない: %s", id, name)
		}
		if n := utf8.RuneCountInString(name); n > 40 {
			t.Errorf("%s の名前が長すぎる（%d字）: %s", id, n, name)
		}
	}
	for id := range want {
		if !got[id] {
			t.Errorf("%s がプリセットに無い", id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("%s は通うジムにあるマシンの一覧に無い", id)
		}
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
