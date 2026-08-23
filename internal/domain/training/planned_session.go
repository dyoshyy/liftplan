package training

// PlannedSet はその日にやることの1単位。
//
// Weight が (Weight, false) を返すのは、履歴が足りず推定できない場合。
// 数字を捏造せず「未確定」を返し、初回だけユーザーが決める。
type PlannedSet struct {
	exerciseID ExerciseID
	weight     Weight
	hasWeight  bool
	sets       SetCount
	targetRIR  RIR
	role       SlotRole
	hasRole    bool
}

func (s PlannedSet) ExerciseID() ExerciseID { return s.exerciseID }
func (s PlannedSet) Weight() (Weight, bool) { return s.weight, s.hasWeight }
func (s PlannedSet) Sets() SetCount         { return s.sets }
func (s PlannedSet) TargetRIR() RIR         { return s.targetRIR }
func (s PlannedSet) Role() (SlotRole, bool) { return s.role, s.hasRole }

func (s PlannedSet) IsZero() bool { return s == PlannedSet{} }

// PlannedSession は導出されたセッション。保存はしない。
type PlannedSession struct {
	date        Date
	main        []PlannedSet
	accessories []PlannedSet
	proposal    DeloadProposal
	hasProposal bool
}

func (s PlannedSession) Date() Date { return s.date }

func (s PlannedSession) Main() []PlannedSet {
	out := make([]PlannedSet, len(s.main))
	copy(out, s.main)
	return out
}

func (s PlannedSession) Accessories() []PlannedSet {
	out := make([]PlannedSet, len(s.accessories))
	copy(out, s.accessories)
	return out
}

func (s PlannedSession) DeloadProposal() (DeloadProposal, bool) {
	return s.proposal, s.hasProposal
}
