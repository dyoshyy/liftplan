package training

// EffectiveLoad は記録された1セットを、推定に使う実効負荷へ読み替える。
//
//	負荷 = 体重 × 係数 + 加重
//
// 記録に入っているのは「プレートを何kg付けたか」で、体重を足すのは推定する
// ときだけ。こうしないと「あの日は何kg付けたか」が記録から読めなくなる。
//
// 失敗しない。返せない場合は 0kg を返し、下流に判断を委ねる。0kg のセットは
// NewOneRepMax の下限（smallestPositive）に弾かれて EstimatedOneRepMax が
// false を返し、MedianOneRepMax が除外し、smooth が飛ばす。手前で間引くと
// 履歴の件数が変わり、やったはずのセットが残差から消える。
//
// 0kg を返すのは次の場合。
//   - 種目や記録が引けない（選択から外した種目の過去ログなど）
//   - 体重の実測はあるが、そのセットの日付より前には無い
//
// 後者で既定体重に倒さないのは、実測を持っている利用者の履歴に既定値由来の
// 点が混ざると、推定が実測と既定値のあいだで揺れるため。
func EffectiveLoad(log *SetLog, e *Exercise, c ConditionLog) Weight {
	if e == nil || log == nil {
		return Weight{}
	}

	factor := e.bodyweightFactor.Float()
	if factor == 0 {
		return log.Weight()
	}

	bw, ok := c.BodyWeightAsOf(log.PerformedOn())
	if !ok {
		if c.HasBodyWeight() {
			return Weight{}
		}
		bw = defaultBodyWeightKg
	}

	adjusted, err := NewWeight(log.Weight().Kg() + factor*bw)
	if err != nil {
		return Weight{}
	}
	return adjusted
}

// AddedWeight は実効負荷の目標から、実際に付ける加重を返す。EffectiveLoad の逆。
//
// 画面に出すのは「プレートを何kg付けるか」。体重込みの 77.5kg と言われても
// 何をすればいいか分からない。
//
// 引いたあとの数字は刻みに乗らない。丸めは総負荷に対して既にかかっており、
// ここで丸め直すと総負荷が目標からずれ、どちらを信じる数字か分からなくなる。
//
// EffectiveLoad と違い、体重が引けないケースは「一度も記録していない」しか
// 無い。処方は当日で体重を引くので、記録が一件でもあれば必ず過去にある。
// よって既定体重に倒して常に値を返す。
//
// ここで体重を0と見なしてはいけない。引き算が消えて added = total になり、
// 自重種目に総負荷をそのまま処方する（チンニングに「77.5kg 付けろ」）。
func AddedWeight(total Weight, e *Exercise, c ConditionLog, on Date) Weight {
	if e == nil {
		return total
	}

	factor := e.bodyweightFactor.Float()
	if factor == 0 {
		return total
	}

	bw, ok := c.BodyWeightAsOf(on)
	if !ok {
		bw = defaultBodyWeightKg
	}

	// 自重だけで目標を超えるなら、付けるものは無い。負の重量は存在しないので
	// 0 に倒す（＝「自重」）。目標そのものを下げるのはデロードの仕事。
	added := max(total.Kg()-factor*bw, 0)

	w, err := NewWeight(added)
	if err != nil {
		return Weight{}
	}
	return w
}

// effectiveHistory は推定に渡す履歴。全セットを実効負荷へ読み替える。
//
// 数える・日付を見る・記録を見せる経路には渡さないこと。重量が違う。
//
// セットは一つも落とさない。落とすと件数が変わり、やったはずのセットが
// 残差の計算から消えて、同じ区分の補助が何度でも提示される。負荷が
// 出せないセットは 0kg になり、推定側で除外される。
func effectiveHistory(h History, pool []*Exercise, c ConditionLog) History {
	logs := h.Logs()
	byID := make(map[ExerciseID]*Exercise, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		byID[e.ID()] = e
	}

	out := make([]*SetLog, 0, len(logs))
	for _, l := range logs {
		w := EffectiveLoad(l, byID[l.ExerciseID()], c)
		newl, err := NewSetLog(SetLogParams{
			ID:          string(l.ID()),
			PerformedOn: Date(l.PerformedOn()),
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
	return NewHistory(out)
}
