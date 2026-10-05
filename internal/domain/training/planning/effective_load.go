package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
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

// LoadOffset は、自重種目で記録した加重に足すと実効負荷になる量。
// 自重係数 × on 時点の体重。自重を使わない種目は 0。
//
// 体重を一度も記録していなければ既定体重で足す。0 を返すと、足す側が
// 「加重だけ」で比べ始めて、自重種目の負荷が体重ぶん軽く見える。
//
// EffectiveLoad と違い、体重を記録した人の on より前に体重が無い場合も
// 既定体重に倒す。こちらは「いま上げるセット」の負荷を出すための量で、
// 値が無いことは許されない。
func LoadOffset(e *exercise.Exercise, c condition.ConditionLog, on training.Date) float64 {
	if e == nil {
		return 0
	}
	factor := e.BodyweightFactor().Float()
	if factor == 0 {
		return 0
	}

	bw, ok := c.BodyWeightAsOf(on)
	if !ok {
		bw = condition.DefaultBodyWeightKg
	}
	return factor * bw
}

// AddedWeight は実効負荷の目標から、実際に付ける加重を返す。EffectiveLoad の逆。
// 体重込みの数字を見せられても何をすればいいか分からないため。
//
// 引いたあとに種目の刻みへ丸める。付けるのはプレートなので、刻みの倍数でない
// 加重は組めない。総負荷の丸めは体重×係数を引く前にかかっており、72.37kg の
// 人の差分は 8.7485kg のような半端な数になる。丸めた結果の総負荷は目標から
// 刻みの半分までずれるが、載せられる重量が刻みの倍数だけである以上避けられない。
//
// EffectiveLoad と違い 0kg に倒す道が無い。引き算が消えて added = total となり、
// 自重種目に総負荷をそのまま処方してしまう。当日で引く以上、記録が一件でも
// あれば必ず過去にあるので、無ければ既定体重で常に値を返す。
func AddedWeight(total training.Weight, e *exercise.Exercise, c condition.ConditionLog, on training.Date) training.Weight {
	if e == nil {
		return total
	}

	// 自重だけで目標を超えるなら、付けるものは無い。負の重量は存在しないので
	// 0 に倒す（＝「自重」）。目標そのものを下げるのはデロードの仕事。
	added := max(total.Kg()-LoadOffset(e, c, on), 0)

	w, err := training.NewWeight(added)
	if err != nil {
		return training.Weight{}
	}
	rounded, err := w.RoundTo(e.Increment())
	if err != nil {
		return training.Weight{}
	}
	return rounded
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

// SessionEstimate はセッション1回ぶんの推定1RM。
type SessionEstimate struct {
	Date      training.Date
	OneRepMax training.OneRepMax
}

// SessionEstimates は種目 e の履歴から、セッションごとの推定1RMを古い順に返す。
//
// 履歴は記録のまま渡す。体重込みへの読み替えはここで済ませる。呼び出し側に
// 任せると、通し忘れた経路だけ自重種目が推定できなくなる（#67：推移グラフに
// 自重種目の線が引かれなかった）。
//
// 推定できないセッション（体重を引けない日など）は含めない。0 として混ぜると、
// 線が床まで落ちて推移が読めなくなる。
func SessionEstimates(h setlog.History, e *exercise.Exercise, c condition.ConditionLog) []SessionEstimate {
	if e == nil {
		return nil
	}
	converted := effectiveHistory(h.ForExercise(e.ID()), []*exercise.Exercise{e}, c)

	out := []SessionEstimate{}
	for _, s := range converted.Sessions() {
		v, ok := s.MedianOneRepMax()
		if !ok {
			continue
		}
		out = append(out, SessionEstimate{Date: s.Date(), OneRepMax: v})
	}
	return out
}
