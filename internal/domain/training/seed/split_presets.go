package seed

import (
	"fmt"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SplitPreset は分割のプリセット1件。
//
// 週目標のプリセットと同じ扱いで、選んだ時点で展開して Program に保存する。
// 名前を保存して毎回引き直すのではない。ここの定義を後で変えても、既に
// 選んだ人の設定は動かない。
type SplitPreset struct {
	Key   string
	Name  string
	Cycle []program.Split

	// MinFrequencyPerWeek はこのプリセットを選ぶために必要な週の最小頻度。
	// 0 は下限なし。
	//
	// 補助の割り振り（PR #185・docs/specs/2026-09-26-accessory-allocation-design.md）
	// の計測で、five_way は週2回だと各日が4週目標を100%のスロット効率でも
	// 満たせない（胸0.88 背中0.52 肩1.04 腕0.63 脚0.37）。週4回からは
	// 帯域内に収まる。選べる設定を残したまま本人に判断を押し戻すのではなく、
	// 選べる選択肢から外す（CLAUDE.md「答えるべき問いと、答えるべきでない
	// 問いを分ける」）。
	//
	// いま下限を持つのは five_way だけなので、頻度と区分数からの一般式には
	// せず、プリセットごとの値として持たせるだけにする（必要になるまで
	// 作らない）。
	MinFrequencyPerWeek int
}

// SplitPresets は選べる分割の一覧を、簡単な順に返す。
//
// 実シードの29種目で成立することを測ってある（2026-09-19 の仕様書）。
// 5分割の肩・腕には BIG3＋チンニングの中に主働を持つ種目が無いので、
// その日は軸が空になる。伸ばしたい肩種目を宣言に足せば軸が入る。
func SplitPresets() ([]SplitPreset, error) {
	type spec struct {
		key, name string
		days      [][2]any // [名前, 区分]
		minFreq   int      // 0 は下限なし
	}

	R := func(rs ...training.MuscleRegion) []training.MuscleRegion { return rs }

	upper := R(
		training.ChestUpper, training.ChestMid, training.ChestLower,
		training.Lat, training.TrapMid, training.TrapUpper,
		training.FrontDelt, training.SideDelt, training.RearDelt,
		training.TricepsLong, training.TricepsLateral,
		training.Biceps, training.Forearm,
	)
	lower := R(
		training.Quad, training.Hamstring, training.Glute,
		training.Adductor, training.Calf, training.Erector,
	)

	specs := []spec{
		{key: "upper_lower", name: "上下2分割", days: [][2]any{
			{"上半身", upper},
			{"下半身", lower},
		}},
		{key: "ppl", name: "PPL（押す・引く・脚）", days: [][2]any{
			{"押す", R(training.ChestUpper, training.ChestMid, training.ChestLower,
				training.FrontDelt, training.SideDelt,
				training.TricepsLong, training.TricepsLateral)},
			{"引く", R(training.Lat, training.TrapMid, training.TrapUpper,
				training.RearDelt, training.Biceps, training.Forearm, training.Erector)},
			{"脚", R(training.Quad, training.Hamstring, training.Glute,
				training.Adductor, training.Calf)},
		}},
		{key: "five_way", name: "5分割（胸・背・肩・腕・脚）", minFreq: 4, days: [][2]any{
			{"胸", R(training.ChestUpper, training.ChestMid, training.ChestLower)},
			{"背中", R(training.Lat, training.TrapMid, training.TrapUpper, training.Erector)},
			{"肩", R(training.FrontDelt, training.SideDelt, training.RearDelt)},
			{"腕", R(training.Biceps, training.TricepsLong, training.TricepsLateral,
				training.Forearm)},
			{"脚", R(training.Quad, training.Hamstring, training.Glute,
				training.Adductor, training.Calf)},
		}},
	}

	out := make([]SplitPreset, 0, len(specs))
	for _, sp := range specs {
		cycle := make([]program.Split, 0, len(sp.days))
		for _, d := range sp.days {
			s, err := program.NewSplit(d[0].(string), d[1].([]training.MuscleRegion))
			if err != nil {
				return nil, fmt.Errorf("%s の %s: %w", sp.key, d[0], err)
			}
			cycle = append(cycle, s)
		}
		out = append(out, SplitPreset{
			Key: sp.key, Name: sp.name, Cycle: cycle, MinFrequencyPerWeek: sp.minFreq,
		})
	}
	return out, nil
}

// MatchPreset は周期がどのプリセットと一致するかを返す。一致しなければ false。
//
// プリセットは選んだ時点で展開して Program に保存するので、保存された
// Cycle に「どれを選んだか」は残らない（SplitPreset のコメント）。頻度の
// 下限をこの周期に適用してよいかは、いま定義されているプリセットと
// 突き合わせて判定する。
//
// 突き合わせは名前と区分の両方で見る（web/src/features/settings/split.ts の
// matchingPresetKey と同じ基準）。区分だけだと、同じ割り当てに別の名前を
// 付けたプリセットが増えたときに取り違える。
func MatchPreset(cycle []program.Split) (SplitPreset, bool, error) {
	presets, err := SplitPresets()
	if err != nil {
		return SplitPreset{}, false, err
	}
	for _, p := range presets {
		if splitCyclesEqual(p.Cycle, cycle) {
			return p, true, nil
		}
	}
	return SplitPreset{}, false, nil
}

// splitCyclesEqual は2つの周期が同じ名前・同じ区分を同じ並びで持つかを見る。
func splitCyclesEqual(a, b []program.Split) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name() != b[i].Name() {
			return false
		}
		ra, rb := a[i].Regions(), b[i].Regions()
		if len(ra) != len(rb) {
			return false
		}
		for j := range ra {
			if ra[j] != rb[j] {
				return false
			}
		}
	}
	return true
}
