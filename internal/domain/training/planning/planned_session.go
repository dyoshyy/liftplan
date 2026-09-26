package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// PlannedSet はその日にやることの1単位。
//
// Weight が (Weight, false) を返すのは、履歴が足りず推定できない場合。
// 数字を捏造せず「未確定」を返し、初回だけユーザーが決める。
type PlannedSet struct {
	exerciseID exercise.ExerciseID
	weight     training.Weight
	hasWeight  bool
	sets       training.SetCount
	targetRIR  training.RIR
}

func (s PlannedSet) ExerciseID() exercise.ExerciseID { return s.exerciseID }
func (s PlannedSet) Weight() (training.Weight, bool) { return s.weight, s.hasWeight }
func (s PlannedSet) Sets() training.SetCount         { return s.sets }
func (s PlannedSet) TargetRIR() training.RIR         { return s.targetRIR }

func (s PlannedSet) IsZero() bool { return s == PlannedSet{} }

// PlannedSession は導出されたセッション。保存はしない。
type PlannedSession struct {
	date        training.Date
	split       program.Split
	hasSplit    bool
	main        []PlannedSet
	variation   []PlannedSet // バリエーションレーンの種目。nilなら出ない
	accessories []PlannedSet
}

func (s PlannedSession) Date() training.Date { return s.date }

// Split はこのセッションの分割の日。分割が無ければ2番目の戻り値が false
// （全区分を狙う）。Forecast の各回が「n回後・日の名前」を組み立てるのに
// 使う（設計書「split はその回の分割の日の名前。分割なしは null」）。
func (s PlannedSession) Split() (program.Split, bool) { return s.split, s.hasSplit }

func (s PlannedSession) Main() []PlannedSet {
	out := make([]PlannedSet, len(s.main))
	copy(out, s.main)
	return out
}

func (s PlannedSession) Variation() []PlannedSet {
	out := make([]PlannedSet, len(s.variation))
	copy(out, s.variation)
	return out
}

func (s PlannedSession) Accessories() []PlannedSet {
	out := make([]PlannedSet, len(s.accessories))
	copy(out, s.accessories)
	return out
}
