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
