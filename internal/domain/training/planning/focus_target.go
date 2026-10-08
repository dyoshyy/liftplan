package planning

import (
	"fmt"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// WeeklyTarget は設定から組んだ週目標（seed.DefaultWeeklyTarget）に、
// 重点種目の系統が先の回で余分に入れる刺激を足して返す。
//
// 計画（Forecast）と画面の充足・開発用シミュレーションが同じ目標を使う
// ための入口。計画だけ上げると、画面が「胸は足りている」と言いながら
// 計画は胸に補助を足し続ける。
//
// history は当日を含んでよい（ここで前日までに切る。Forecast と同じ規約）。
func (p SessionPlanner) WeeklyTarget(
	base program.WeeklyVolumeTarget, history setlog.History,
	prog *program.Program, pool []*exercise.Exercise, date training.Date,
) (program.WeeklyVolumeTarget, error) {
	if prog == nil {
		return program.WeeklyVolumeTarget{}, fmt.Errorf("プログラムが指定されていない")
	}
	if _, ok := prog.FocusExercise(); !ok {
		return base, nil
	}
	sessions, err := p.ProjectHorizon(history.Before(date), prog, usablePool(pool, prog), date)
	if err != nil {
		return program.WeeklyVolumeTarget{}, fmt.Errorf("先の回の予測に失敗: %w", err)
	}
	return raiseForFocus(base, sessions), nil
}

// raiseForFocus は週目標に、重点種目を指定したから出る回の刺激を足す。
//
// 重点種目は一巡（重い番・軽い番・派生）とバリエーションレーンで、他の
// 宣言種目より多く出る。目標が他と同じままだと、その区分は毎週目標を
// 大きく超え、割り振り器の超過の罰則（regionLoss の α）が、その区分に
// 副次で効く補助まで締め出す。胸を重点にすると、胸に 0.3 で効くだけの
// 肩や三頭の種目が選ばれなくなり、補助の枠が余る。
//
// 足すのは重い番以外。重い番は重点でなくても宣言種目の番として来る回で、
// 設定から組んだ目標が既にその分を見ている。軽い番・派生が軸に立つ回・
// バリエーションレーンは、重点を指定したから出る回。
//
// 先の回から数えるのは、系統の出方が分割と頻度で週 0.75〜3 回まで動く
// ため（上下2分割は上の日の数が上限、ppl は押す日だけ）。定数で上げると
// 合わない構成が必ず残り、上げすぎた構成では胸の補助が流れ込む。
//
// 目標に無い区分（0 = 狙わない）には足さない。
func raiseForFocus(base program.WeeklyVolumeTarget, sessions []ProjectedSession) program.WeeklyVolumeTarget {
	extra := StimulusCoverage{}
	for _, s := range sessions {
		if s.axis != nil && s.axisRole != heavyRole {
			extra = extra.Plus(s.axis.Stimulus(), s.axisSets)
		}
		if s.variation != nil {
			extra = extra.Plus(s.variation.Stimulus(), s.variationSets)
		}
	}
	if extra.IsEmpty() {
		return base
	}

	raised := make(map[training.MuscleRegion]float64, len(base.Regions()))
	for _, r := range base.Regions() {
		raised[r] = base.Sets(r) + extra.Sets(r)
	}
	out, err := program.NewWeeklyVolumeTarget(raised)
	if err != nil {
		// 到達しない。base の区分は検証済みで、足す量は非負。
		return base
	}
	return out
}
