package training

import (
	"errors"
	"fmt"
	"sort"
)

const (
	// accessoryIntensityPct は補助種目の強度。RIR2 で10レップ前後を狙う位置。
	accessoryIntensityPct = 0.71
	accessoryTargetRIR    = 2
)

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

// PlanRequest は導出の入力すべて。ドメインは自分でデータを取りに行かない。
type PlanRequest struct {
	Program    *Program
	Pool       []*Exercise
	History    History
	Conditions ConditionLog
	Date       Date

	// DeloadAccepted はユーザーがデロードを承認した種目。
	//
	// bool ではなく種目の一覧なのは、承認の粒度を提案の粒度に合わせるため。
	// 単一の bool だと、ベンチの提案を承認した状態のまま後からスクワットが
	// 停滞判定に入ったとき、新しい承認を経ずにスクワットまで下がる。
	//
	// 提案の有無とは独立に効く。提案は毎回計算し直されるので、体重の記録が
	// 数日途切れただけで消えることがある。提案が消えたら承認も無効、では
	// 「承認したのに重量が下がらない」という説明のつかない挙動になる。
	DeloadAccepted []ExerciseID
}

// SessionPlanner はドメインの入口となるドメインサービス。無状態。
type SessionPlanner struct {
	slots     SlotCatalog
	estimator OneRepMaxEstimator
	ratios    VariationRatioResolver
	accessory AccessorySelector
	deload    DeloadPolicy
}

// analyzer は RIR 補正に使うコンディション分析器。
//
// デロード判定と同じものを使う。別々にすると、同じセッションの中で
// 「デロードは減量中と判定、RIR補正は減量中でないと判定」のような
// 一貫性の破れが起きる。
func (p SessionPlanner) analyzer() ConditionAnalyzer { return p.deload.Analyzer() }

func NewSessionPlanner(
	slots SlotCatalog,
	estimator OneRepMaxEstimator,
	ratios VariationRatioResolver,
	accessory AccessorySelector,
	deload DeloadPolicy,
) (SessionPlanner, error) {
	if estimator.IsZero() {
		return SessionPlanner{}, errors.New("推定器が未設定である")
	}
	if ratios.IsZero() {
		return SessionPlanner{}, errors.New("対メイン係数の解決器が未設定である")
	}
	if accessory.IsZero() {
		return SessionPlanner{}, errors.New("補助種目の選択器が未設定である")
	}
	if deload.IsZero() {
		return SessionPlanner{}, errors.New("デロードのポリシーが未設定である")
	}
	return SessionPlanner{
		slots: slots, estimator: estimator, ratios: ratios,
		accessory: accessory, deload: deload,
	}, nil
}

func DefaultSessionPlanner() SessionPlanner {
	return SessionPlanner{
		slots:     NewSlotCatalog(),
		estimator: DefaultOneRepMaxEstimator(),
		ratios:    DefaultVariationRatioResolver(),
		accessory: DefaultAccessorySelector(),
		deload:    DefaultDeloadPolicy(),
	}
}

func (p SessionPlanner) IsZero() bool { return p == SessionPlanner{} }

// Plan はその日のセッションを導出する。
//
// 未来のセッションはどこにも保存しない。今日のメニューも来週のメニューも
// この関数を対象日で呼んだ結果でしかない。だから予定と実績が食い違う状態が
// 原理的に発生しない。
func (p SessionPlanner) Plan(req PlanRequest) (PlannedSession, error) {
	if p.IsZero() {
		return PlannedSession{}, errors.New("セッション生成器が未設定である")
	}
	if req.Program == nil {
		return PlannedSession{}, errors.New("プログラムが指定されていない")
	}
	if req.Date.IsZero() {
		return PlannedSession{}, errors.New("対象日が指定されていない")
	}

	pool := p.usablePool(req)
	mains := mainExercises(pool)
	if len(mains) == 0 {
		// 集約は種目マスタを知らないので、メイン種目の有無を検証できない。
		// ここが唯一の検出点。空のセッションを黙って返すと、ユーザーには
		// 中身の無いメニューが出てどこにもエラーが立たない。
		return PlannedSession{}, errors.New("メイン種目が1つも選ばれていない")
	}

	template, ok := p.slots.Select(req.Program.Frequency(), sessionIndexInWeek(req.History, req.Date))
	if !ok {
		return PlannedSession{}, fmt.Errorf(
			"週%d回に対応するスロット構成が無い", req.Program.Frequency().PerWeek())
	}

	mainIDs := make([]ExerciseID, 0, len(mains))
	for _, e := range mains {
		mainIDs = append(mainIDs, e.ID())
	}
	proposal, hasProposal := p.deload.Propose(req.History, mainIDs, req.Conditions, req.Date)

	// デロードは承認された種目にだけ適用する。伸びている種目まで一律に下げると、
	// 本人の実感と噛み合わない。
	deloadTargets := make(map[ExerciseID]bool, len(req.DeloadAccepted))
	for _, id := range req.DeloadAccepted {
		deloadTargets[id] = true
	}

	rirBump := p.analyzer().RIRAdjustment(req.Conditions, req.Date)

	// 週内カバレッジには当日の実績も含める。含めないと、セッション中に
	// 補助をこなして計画を開き直したとき残差が減らず、同じ補助が
	// 何度でも提示されてセッションが終わらない。
	coverage := coveredThisWeek(req.History, pool, req.Date)
	doneToday := performedOn(req.History, req.Date)

	main := make([]PlannedSet, 0, len(mains))
	for _, e := range mains {
		set, performed := p.planMain(req, pool, e, template, deloadTargets[e.ID()], rirBump)
		main = append(main, set)

		// 当日すでに記録済みのメインは、カバレッジに二重計上しない。
		if !doneToday[performed.ID()] {
			coverage = coverage.Plus(performed.Stimulus(), set.Sets())
		}
	}

	// 設定より多く通った場合でも、残り1セッション分は狙えるようにする。
	// 0 以下にすると残差が空になり、補助が1つも出ないまま
	// メイン種目のフルスロットだけが積まれる。
	sessionsRemaining := max(1, req.Program.Frequency().PerWeek()-sessionIndexInWeek(req.History, req.Date))
	gaps := SessionResidual(req.Program.WeeklyTarget(), coverage, sessionsRemaining)

	chosen := p.accessory.Select(gaps, remaining(pool, doneToday), req.History, req.Date)
	accessories := make([]PlannedSet, 0, len(chosen))
	for _, id := range chosen {
		accessories = append(accessories, p.planAccessory(req, pool, id, rirBump))
	}

	return PlannedSession{
		date:        req.Date,
		main:        main,
		accessories: accessories,
		proposal:    proposal,
		hasProposal: hasProposal,
	}, nil
}

// usablePool はプログラムで選択された種目と、その派生バリエーションを返す。
//
// バリエーションはユーザーが個別に選ぶものではなく、メインに付随して
// 自動で回るため、選択に含まれていなくても候補にする。
func (p SessionPlanner) usablePool(req PlanRequest) []*Exercise {
	out := make([]*Exercise, 0, len(req.Pool))
	for _, e := range req.Pool {
		if e == nil {
			continue
		}
		if req.Program.Includes(e.ID()) || e.Kind() == KindVariation {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

func mainExercises(pool []*Exercise) []*Exercise {
	out := make([]*Exercise, 0, len(pool))
	for _, e := range pool {
		if e.Kind() == KindMain {
			out = append(out, e)
		}
	}
	return out
}

// planMain は1つのメインリフトのスロットを埋める。
// 2つ目の返り値は実際に行う種目（バリエーションに差し替わることがある）。
func (p SessionPlanner) planMain(
	req PlanRequest,
	pool []*Exercise,
	main *Exercise,
	template SlotTemplate,
	deloaded bool,
	rirBump int,
) (PlannedSet, *Exercise) {
	target := main
	ratio := unitRatio

	if template.Role() == RoleVariation {
		if v := p.pickVariation(req, pool, main); v != nil {
			target = v
			ratio = p.ratios.Resolve(req.History, v, main.ID(), req.Date)
		}
	}

	intensity := template.Intensity()
	if deloaded {
		intensity = intensity.Reduce(p.deload.IntensityDropPct())
	}

	set := PlannedSet{
		exerciseID: target.ID(),
		sets:       template.Sets(),
		targetRIR:  template.TargetRIR().Plus(rirBump),
		role:       template.Role(),
		hasRole:    true,
	}

	// 重量はメインの推定1RMを基準にし、バリエーションには係数を掛ける。
	// バリエーション自身の1RMを使うと、履歴の少ない種目で数字が暴れる。
	if orm, ok := p.estimator.Estimate(req.History, main.ID(), req.Date); ok {
		if w, err := orm.WorkWeight(intensity, ratio, target.Increment()); err == nil {
			set.weight, set.hasWeight = w, true
		}
	}
	return set, target
}

// pickVariation は同じメインリフトの派生のうち、最後に使ってから
// 最も間隔が空いているものを選ぶ。
func (p SessionPlanner) pickVariation(req PlanRequest, pool []*Exercise, main *Exercise) *Exercise {
	lift, ok := main.MainLift()
	if !ok {
		return nil
	}

	var best *Exercise
	bestDaysAgo := -1
	for _, e := range pool {
		if !e.IsVariationOf(lift) {
			continue
		}
		daysAgo := neverStimulated
		if last, ok := req.History.LastPerformed(e.ID()); ok {
			daysAgo = req.Date.DaysSince(last)
			if daysAgo < 0 {
				daysAgo = 0
			}
		}
		if daysAgo > bestDaysAgo {
			best, bestDaysAgo = e, daysAgo
		}
	}
	return best
}

// planAccessory は補助種目の1枠を埋める。
//
// デロードは適用しない。デロードの対象はメイン種目だけで、補助種目は
// そもそも停滞判定の対象になっていない。
func (p SessionPlanner) planAccessory(
	req PlanRequest,
	pool []*Exercise,
	id ExerciseID,
	rirBump int,
) PlannedSet {
	baseRIR, err := NewRIR(accessoryTargetRIR)
	if err != nil {
		return PlannedSet{}
	}
	set := PlannedSet{
		exerciseID: id,
		sets:       p.accessory.SetsPerAccessory(),
		targetRIR:  baseRIR.Plus(rirBump),
	}

	exercise := findExercise(pool, id)
	if exercise == nil {
		return set
	}

	intensity, err := NewIntensityPct(accessoryIntensityPct)
	if err != nil {
		return set
	}

	if orm, ok := p.estimator.Estimate(req.History, id, req.Date); ok {
		if w, err := orm.WorkWeight(intensity, unitRatio, exercise.Increment()); err == nil {
			set.weight, set.hasWeight = w, true
		}
	}
	return set
}

func findExercise(pool []*Exercise, id ExerciseID) *Exercise {
	for _, e := range pool {
		if e.ID() == id {
			return e
		}
	}
	return nil
}

// coveredThisWeek は週初からその日までに埋めた刺激量。当日を含む。
//
// 当日を含めるのは、セッション中にこなした補助を残差に反映するため。
// 含めないと、記録して計画を開き直しても残差が減らず、同じ補助が
// 何度でも提示されてセッションが終わらない。
//
// 当日のメイン種目については、呼び出し側が二重計上を避ける。
func coveredThisWeek(h History, pool []*Exercise, date Date) StimulusCoverage {
	coverage := StimulusCoverage{}
	one, err := NewSetCount(1)
	if err != nil {
		return coverage
	}

	byID := make(map[ExerciseID]*Exercise, len(pool))
	for _, e := range pool {
		byID[e.ID()] = e
	}

	for _, l := range h.OnOrAfter(date.WeekStart()).OnOrBefore(date).Logs() {
		e, ok := byID[l.ExerciseID()]
		if !ok {
			continue
		}
		coverage = coverage.Plus(e.Stimulus(), one)
	}
	return coverage
}

// performedOn はその日に記録がある種目。
func performedOn(h History, date Date) map[ExerciseID]bool {
	out := map[ExerciseID]bool{}
	for _, l := range h.OnOrAfter(date).OnOrBefore(date).Logs() {
		out[l.ExerciseID()] = true
	}
	return out
}

// remaining は当日すでに実施した種目を除いたプール。
//
// 除かないと、こなした補助がもう一度提示される。
func remaining(pool []*Exercise, doneToday map[ExerciseID]bool) []*Exercise {
	out := make([]*Exercise, 0, len(pool))
	for _, e := range pool {
		if doneToday[e.ID()] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// sessionIndexInWeek はその週で対象日が何本目のセッションか（0始まり）。
// 曜日の割り当てはドメインの責務ではないため、実績から導出する。
func sessionIndexInWeek(h History, date Date) int {
	return h.OnOrAfter(date.WeekStart()).Before(date).SessionCount()
}
