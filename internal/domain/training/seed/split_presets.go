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
		{"full_body", "全身法", [][2]any{
			// 区分を持たない分割は全区分を狙う。分割なしと同じ挙動だが、
			// 「全身法を選んだ」という意思が設定に残る。
			{"全身", R()},
		}},
		{"upper_lower", "上下2分割", [][2]any{
			{"上半身", upper},
			{"下半身", lower},
		}},
		{"ppl", "PPL（押す・引く・脚）", [][2]any{
			{"押す", R(training.ChestUpper, training.ChestMid, training.ChestLower,
				training.FrontDelt, training.SideDelt,
				training.TricepsLong, training.TricepsLateral)},
			{"引く", R(training.Lat, training.TrapMid, training.TrapUpper,
				training.RearDelt, training.Biceps, training.Forearm, training.Erector)},
			{"脚", R(training.Quad, training.Hamstring, training.Glute,
				training.Adductor, training.Calf)},
		}},
		{"five_way", "5分割（胸・背・肩・腕・脚）", [][2]any{
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
		out = append(out, SplitPreset{Key: sp.key, Name: sp.name, Cycle: cycle})
	}
	return out, nil
}
