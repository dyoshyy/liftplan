package planning

import (
	"github.com/dyoshyy/liftplan-server/internal/domain/training"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan-server/internal/domain/training/setlog"
)

// EffectiveLoad は記録を推定に使う負荷へ読み替える。記録に入っているのは
// 加重だけで、体重を足すのは推定するときだけ。
//
// 失敗しない。負荷が出せなければ 0kg を返し、除外は下流に委ねる。0kg は
// NewOneRepMax の下限に弾かれるので推定に混ざらない。手前で間引くと履歴の
// 件数が変わり、やったはずのセットが残差から消える。
//
// 実測があるのに引けない日を既定体重に倒さないのは、既定値由来の点が混ざると
// 推定が実測と既定値のあいだで揺れるため。
func EffectiveLoad(log *setlog.SetLog, e *exercise.Exercise, c condition.ConditionLog) training.Weight {
	if e == nil || log == nil {
		return training.Weight{}
	}

	factor := e.BodyweightFactor().Float()
	if factor == 0 {
		return log.Weight()
	}

	bw, ok := c.BodyWeightAsOf(log.PerformedOn())
	if !ok {
		if c.HasBodyWeight() {
			return training.Weight{}
		}
		bw = condition.DefaultBodyWeightKg
	}

	adjusted, err := training.NewWeight(log.Weight().Kg() + factor*bw)
	if err != nil {
		return training.Weight{}
	}
	return adjusted
}

// AddedWeight は実効負荷の目標から、実際に付ける加重を返す。EffectiveLoad の逆。
// 体重込みの数字を見せられても何をすればいいか分からないため。
//
// 引いたあとは丸めない。丸めは総負荷に既にかかっており、乗せ直すと総負荷が
// 目標からずれる。
//
// EffectiveLoad と違い 0kg に倒す道が無い。引き算が消えて added = total となり、
// 自重種目に総負荷をそのまま処方してしまう。当日で引く以上、記録が一件でも
// あれば必ず過去にあるので、無ければ既定体重で常に値を返す。
func AddedWeight(total training.Weight, e *exercise.Exercise, c condition.ConditionLog, on training.Date) training.Weight {
	if e == nil {
		return total
	}

	factor := e.BodyweightFactor().Float()
	if factor == 0 {
		return total
	}

	bw, ok := c.BodyWeightAsOf(on)
	if !ok {
		bw = condition.DefaultBodyWeightKg
	}

	// 自重だけで目標を超えるなら、付けるものは無い。負の重量は存在しないので
	// 0 に倒す（＝「自重」）。目標そのものを下げるのはデロードの仕事。
	added := max(total.Kg()-factor*bw, 0)

	w, err := training.NewWeight(added)
	if err != nil {
		return training.Weight{}
	}
	return w
}

// effectiveHistory は推定に渡す履歴。数える・日付を見る・記録を見せる経路には
// 渡さないこと。重量が違う。
//
// セットは一つも落とさない。落とすと件数が変わり、やったはずのセットが残差から
// 消えて、同じ区分の補助が何度でも提示される。
func effectiveHistory(h setlog.History, pool []*exercise.Exercise, c condition.ConditionLog) setlog.History {
	logs := h.Logs()
	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		byID[e.ID()] = e
	}

	out := make([]*setlog.SetLog, 0, len(logs))
	for _, l := range logs {
		w := EffectiveLoad(l, byID[l.ExerciseID()], c)
		newl, err := setlog.NewSetLog(setlog.SetLogParams{
			ID:          string(l.ID()),
			PerformedOn: training.Date(l.PerformedOn()),
			ExerciseID:  string(l.ExerciseID()),
			WeightKg:    w.Kg(),
			Reps:        l.Reps().Int(),
			RIR:         l.RIR().Int(),
		})
		if err != nil {
			// EffectiveLoad は必ず有効な重量を返すのでここには来ないが、
			// 来たときに件数を保つため元の記録を残す。落とすと残差が狂う。
			out = append(out, l)
			continue
		}
		out = append(out, newl)
	}
	return setlog.NewHistory(out)
}
