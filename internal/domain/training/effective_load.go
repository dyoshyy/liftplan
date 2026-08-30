package training

// EffectiveLoad returns the effective load of a set log,
// considering the bodyweight factor of the exercise and the user's bodyweight at the time of the set.
func EffectiveLoad(log *SetLog, e *Exercise, c ConditionLog) (Weight, bool) {
	if e == nil || log == nil {
		return Weight{}, false
	}

	if e.bodyweightFactor.Float() == 0 {
		return log.Weight(), true
	}

	bw, ok := c.BodyWeightAsOf(log.PerformedOn())
	if !ok {
		return Weight{}, false
	}

	adjusted, err := NewWeight(log.Weight().Kg() + e.bodyweightFactor.Float()*bw)
	if err != nil {
		return Weight{}, false
	}
	return adjusted, true
}

// AddedWeight は実効負荷の目標から、実際に付ける加重を返す。EffectiveLoad の逆。
//
// 画面に出すのは「プレートを何kg付けるか」。体重込みの 82.5kg と言われても
// 何をすればいいか分からない。体重が引けなければ false を返し、呼び出し側は
// 「自分で決める」を出す。
//
// 引いたあとの数字は刻みに乗らない。丸めは総負荷に対して既にかかっており、
// ここで丸め直すと総負荷が目標からずれ、どちらを信じる数字か分からなくなる。
func AddedWeight(total Weight, e *Exercise, c ConditionLog, on Date) (Weight, bool) {
	if e == nil {
		return Weight{}, false
	}

	factor := e.bodyweightFactor.Float()
	if factor == 0 {
		return total, true
	}

	bw, ok := c.BodyWeightAsOf(on)
	if !ok {
		return Weight{}, false
	}

	// 自重だけで目標を超えるなら、付けるものは無い。負の重量は存在しないので
	// 0 に丸める（＝「自重」）。目標そのものを下げるのはデロードの仕事。
	added := max(total.Kg()-factor*bw, 0)

	w, err := NewWeight(added)
	if err != nil {
		return Weight{}, false
	}
	return w, true
}

// effectiveHistory returns a history with effective loads
//
// should be only used for calculating estimated 1RM.
// for coverage or session counting, use the original history.
// Set without available bodyweight will be ignored.
func effectiveHistory(h History, pool []*Exercise, c ConditionLog) History {
	logs := h.Logs()
	out := make([]*SetLog, 0, len(logs))
	for _, l := range logs {
		effectiveWeight, ok := EffectiveLoad(l, findExercise(pool, l.ExerciseID()), c)
		if !ok {
			continue
		}
		newl, err := NewSetLog(SetLogParams{
			ID:          string(l.ID()),
			PerformedOn: Date(l.PerformedOn()),
			ExerciseID:  string(l.ExerciseID()),
			WeightKg:    effectiveWeight.Kg(),
			Reps:        l.Reps().Int(),
			RIR:         l.RIR().Int(),
		})
		if err != nil {
			continue
		}
		out = append(out, newl)
	}
	return NewHistory(out)
}
