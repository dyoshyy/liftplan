package seed_test

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/seed"
)

// プリセットが実シードで成立すること。
//
// 分割を選んだのに補助の候補が1つも無い日があると、その日は軸だけで
// 終わる。区分の割り当てを変えたときに気づけるよう、候補数を固定する。
func TestSplitPresets_AreUsableWithTheSeed(t *testing.T) {
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("プリセットが不正: %v", err)
	}
	pool, err := seed.Exercises()
	if err != nil {
		t.Fatalf("シードが不正: %v", err)
	}

	// 軸の候補がゼロになる日があるのは 5分割 の肩と腕だけ。
	// BIG3＋チンニングに肩・腕を主働にする種目が無いため。
	declared := map[exercise.ExerciseID]bool{
		"bench": true, "squat": true, "deadlift": true, "pull_up": true,
	}
	wantNoAxis := map[string]bool{"肩": true, "腕": true}

	for _, p := range presets {
		if p.Key == "" || p.Name == "" {
			t.Errorf("キーか名前が空: %+v", p)
		}
		if len(p.Cycle) == 0 {
			t.Errorf("%s の周期が空", p.Key)
		}

		for _, s := range p.Cycle {
			axes, candidates := 0, 0
			for _, e := range pool {
				primary := false
				for _, r := range e.Stimulus().Regions() {
					c, ok := e.Stimulus().Contribution(r)
					if ok && c.Float() >= 1.0 && s.Includes(r) {
						primary = true
					}
				}
				if !primary {
					continue
				}
				candidates++
				if declared[e.ID()] {
					axes++
				}
			}

			// 補助の候補は最低3種目。1セッション最大8枠なので、
			// 3を割ると9セットを下回って通し検証の下限に触れる。
			if candidates < 3 {
				t.Errorf("%s の %s: 補助候補が %d 種目しかない", p.Key, s.Name(), candidates)
			}
			if axes == 0 && !wantNoAxis[s.Name()] {
				t.Errorf("%s の %s: 軸の候補が無い。想定外", p.Key, s.Name())
			}
			if axes > 0 && wantNoAxis[s.Name()] {
				t.Errorf("%s の %s: 軸の候補ができた。想定と違う（良い変化なら期待値を直すこと）",
					p.Key, s.Name())
			}
		}
	}
}

// 5分割だけが最小頻度を持つこと。
//
// 補助の割り振り（PR #185・docs/specs/2026-09-26-accessory-allocation-design.md）
// の計測で、週2回では five_way の各日が4週目標を100%のスロット効率でも
// 満たせない（胸0.88 背中0.52 肩1.04 腕0.63 脚0.37）。週4回からは帯域内に
// 収まる。他のプリセットにこの制約は無い（必要になるまで作らない）。
func TestSplitPresets_OnlyFiveWayHasAMinimumFrequency(t *testing.T) {
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("プリセットが不正: %v", err)
	}

	want := map[string]int{"upper_lower": 0, "ppl": 0, "five_way": 4}
	seen := map[string]bool{}
	for _, p := range presets {
		seen[p.Key] = true
		if w, ok := want[p.Key]; ok && p.MinFrequencyPerWeek != w {
			t.Errorf("%s の最小頻度が %d。%d のはず", p.Key, p.MinFrequencyPerWeek, w)
		}
	}
	for key := range want {
		if !seen[key] {
			t.Errorf("プリセット %q が無い", key)
		}
	}
}

// MatchPreset は周期の中身（名前と区分）でプリセットを引き当てる。
//
// プリセットは選んだ時点で展開して保存するので、保存された周期に
// 「どれを選んだか」は残らない（SplitPreset のコメント）。頻度の下限を
// 適用してよいかは、いま定義されているプリセットと突き合わせて判定する。
// 突き合わせの基準は web/src/features/settings/split.ts の
// matchingPresetKey と揃える（名前と区分の両方。区分だけだと、同じ割り当てに
// 別の名前を付けたプリセットが増えたときに取り違える）。
func TestMatchPreset(t *testing.T) {
	presets, err := seed.SplitPresets()
	if err != nil {
		t.Fatalf("プリセットが不正: %v", err)
	}
	var fiveWay seed.SplitPreset
	for _, p := range presets {
		if p.Key == "five_way" {
			fiveWay = p
		}
	}
	if fiveWay.Key == "" {
		t.Fatal("five_way プリセットが見つからない")
	}

	renamed, err := program.NewSplit("胸筋", fiveWay.Cycle[0].Regions())
	if err != nil {
		t.Fatalf("分割の生成に失敗: %v", err)
	}
	renamedCycle := append([]program.Split{renamed}, fiveWay.Cycle[1:]...)

	leg, err := program.NewSplit("脚", []training.MuscleRegion{training.Quad})
	if err != nil {
		t.Fatalf("分割の生成に失敗: %v", err)
	}

	cases := []struct {
		name    string
		cycle   []program.Split
		wantKey string
		wantOK  bool
	}{
		{
			name: "five_way の周期そのものは一致する", cycle: fiveWay.Cycle,
			wantKey: "five_way", wantOK: true,
		},
		{
			// 区分は同じでも1日目の名前だけ違う。取り違えないこと。
			name: "名前が1つでも違えば一致しない", cycle: renamedCycle, wantOK: false,
		},
		{
			name:  "どのプリセットとも違う周期は一致しない",
			cycle: []program.Split{leg}, wantOK: false,
		},
		{
			name: "分割なしはどれとも一致しない", cycle: nil, wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok, err := seed.MatchPreset(c.cycle)
			if err != nil {
				t.Fatalf("MatchPreset が失敗: %v", err)
			}
			if ok != c.wantOK {
				t.Fatalf("一致が %v。%v のはず", ok, c.wantOK)
			}
			if c.wantOK && got.Key != c.wantKey {
				t.Errorf("一致したキーが %q。%q のはず", got.Key, c.wantKey)
			}
		})
	}
}
