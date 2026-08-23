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
	accessory AccessorySelector,
	deload DeloadPolicy,
) (SessionPlanner, error) {
	if estimator.IsZero() {
		return SessionPlanner{}, errors.New("推定器が未設定である")
	}
	if accessory.IsZero() {
		return SessionPlanner{}, errors.New("補助種目の選択器が未設定である")
	}
	if deload.IsZero() {
		return SessionPlanner{}, errors.New("デロードのポリシーが未設定である")
	}
	return SessionPlanner{
		slots: slots, estimator: estimator,
		accessory: accessory, deload: deload,
	}, nil
}

func DefaultSessionPlanner() SessionPlanner {
	return SessionPlanner{
		slots:     NewSlotCatalog(),
		estimator: DefaultOneRepMaxEstimator(),
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
		if doneToday[performed.ID()] == 0 {
			coverage = coverage.Plus(performed.Stimulus(), set.Sets())
		}
	}

	// 設定より多く通った場合でも、残り1セッション分は狙えるようにする。
	// 0 以下にすると残差が空になり、補助が1つも出ないまま
	// メイン種目のフルスロットだけが積まれる。
	sessionsRemaining := max(1, req.Program.Frequency().PerWeek()-sessionIndexInWeek(req.History, req.Date))
	gaps := SessionResidual(req.Program.WeeklyTarget(), coverage, sessionsRemaining)

	// 今日ぶんを終えた種目だけをプールから外す。
	//
	// 当たり前に見えて、両側に落とし穴がある。1セットでも記録したら
	// 外すと、その種目が今日のメニューから消えて残り2セットを記録
	// できない。逆に一切外さないと、3セット終えた種目がまた提示されて
	// セッションが終わらない（D-021 で踏んだ）。境目は「今日の予定分を
	// こなしたか」であって「今日やったか」ではない（D-087）。
	// 選択に渡す履歴からも当日を外す。残差（gaps）には当日を含めるが、
	// 「どの区分を長く放置しているか」と「どの種目を最近やったか」は
	// 当日を見ない。
	//
	// 含めると、1セット記録した瞬間にその区分が「たった今刺激した」に
	// なって優先順位が最下位へ落ち、種目ごとリストから消える。ジムで
	// 使っている最中に、こなしているリストが自分の下で入れ替わる。
	// 何を選ぶかは、その日が始まる前に分かっていたことで決める（D-087）。
	chosen := p.accessory.Select(
		gaps,
		unfinished(pool, doneToday, p.accessory.SetsPerAccessory()),
		historyBefore(req),
		req.Date)

	chosen = p.fitAccessories(chosen, pool, doneToday)
	// 並びは種目IDの昇順に固定する。
	//
	// Select が返す順序は「最も放置している区分から」という優先度だが、
	// 残差は当日の記録で動くので、1セット記録するたびに並びが入れ替わる。
	// ジムで消化している最中にリストが自分の下で動く。
	//
	// 優先度は「8枠に選ばれたかどうか」に既に表れている。その中での
	// 並びは情報ではないので、安定を取る（D-087）。
	sort.Slice(chosen, func(i, j int) bool { return chosen[i] < chosen[j] })

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
		if req.Program.Includes(e.ID()) {
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

	// 当日の記録は使わない（D-086）。含めると、1セット目を記録した瞬間に
	// 推定1RMが動いて2セット目の提示重量が変わる。しかも RIR を守って
	// きついセットをこなすほど推定が上がるので、**追い込むほど次が重くなる**。
	// その日にやることは、その日が始まる前に分かっていたことから決める。
	if orm, ok := p.estimator.Estimate(historyBefore(req), target.ID(), req.Date); ok {
		if w, err := orm.WorkWeight(intensity, target.Increment()); err == nil {
			set.weight, set.hasWeight = w, true
		}
	}
	return set, target
}

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

	if orm, ok := p.estimator.Estimate(historyBefore(req), id, req.Date); ok {
		if w, err := orm.WorkWeight(intensity, exercise.Increment()); err == nil {
			set.weight, set.hasWeight = w, true
		}
	}
	return set
}

// historyBefore は当日より前の履歴。重量の推定に使う。
//
// 週内カバレッジ（D-021）は当日を含めるが、重量の推定は含めない。
// 前者は「今日どれだけ埋めたか」で当日が本質、後者は「今日いくつで
// やるか」で、当日の結果が入ると同じセッションの中で目標が動く。
func historyBefore(req PlanRequest) History {
	return req.History.Before(req.Date)
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
	return CoverageBetween(h, pool, date.WeekStart(), date)
}

// CoverageBetween は期間内に埋めた刺激量を数える。両端を含む。
//
// 記録1件を1セットとして数える。SetLog は「確定した実績1セット」なので、
// 件数がそのままセット数になる。
//
// 公開しているのは、週目標の充足を見せる読み取り経路が同じ数え方を
// 必要とするため。別々に実装すると、画面に出る数字とエンジンが使う数字が
// ずれる。ずれた瞬間、どちらが正しいのか誰にも分からなくなる。
func CoverageBetween(h History, pool []*Exercise, from, to Date) StimulusCoverage {
	coverage := StimulusCoverage{}
	one, err := NewSetCount(1)
	if err != nil {
		return coverage
	}

	byID := make(map[ExerciseID]*Exercise, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		byID[e.ID()] = e
	}

	for _, l := range h.OnOrAfter(from).OnOrBefore(to).Logs() {
		e, ok := byID[l.ExerciseID()]
		if !ok {
			continue
		}
		coverage = coverage.Plus(e.Stimulus(), one)
	}
	return coverage
}

// performedOn はその日に記録があるセット数を種目ごとに数える。
func performedOn(h History, date Date) map[ExerciseID]int {
	out := map[ExerciseID]int{}
	for _, l := range h.OnOrAfter(date).OnOrBefore(date).Logs() {
		out[l.ExerciseID()]++
	}
	return out
}

// fitAccessories は、その日の補助の枠に収める。
//
// 3つの規則が要る。どれが欠けても実運用で壊れる。
//
//  1. 着手して途中の種目は必ず残す。落とすと、1セットやった種目が
//     リストから消えて残りを記録できない。
//  2. こなし終えた種目のぶんは枠を減らす。減らさないと、終えるたびに
//     新しい種目が補充されて種目マスタが尽きるまでセッションが
//     終わらない（通しで消化して踏んだ。60手やっても8件出続けた）。
//  3. 残った枠を新規で埋める。
//
// 「補助は最大8枠」は、その日にやる補助が最大8種目という意味であって、
// 常時8件を提示し続けるという意味ではない（D-088）。
func (p SessionPlanner) fitAccessories(
	chosen []ExerciseID,
	pool []*Exercise,
	doneToday map[ExerciseID]int,
) []ExerciseID {
	per := p.accessory.SetsPerAccessory().Int()

	var started, finished []ExerciseID
	for _, e := range pool {
		if e == nil || e.Kind() != KindAccessory {
			continue
		}
		switch n := doneToday[e.ID()]; {
		case n >= per:
			finished = append(finished, e.ID())
		case n > 0:
			started = append(started, e.ID())
		}
	}

	budget := p.accessory.MaxSlots() - len(finished)
	if budget <= 0 {
		return nil
	}

	out := make([]ExerciseID, 0, budget)
	seen := make(map[ExerciseID]bool, budget)
	for _, id := range started {
		if len(out) >= budget {
			break
		}
		out = append(out, id)
		seen[id] = true
	}
	for _, id := range chosen {
		if len(out) >= budget {
			break
		}
		if seen[id] {
			continue
		}
		out = append(out, id)
		seen[id] = true
	}
	return out
}

// unfinished は今日ぶんを終えていない種目のプール。
func unfinished(pool []*Exercise, doneToday map[ExerciseID]int, perAccessory SetCount) []*Exercise {
	out := make([]*Exercise, 0, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		if doneToday[e.ID()] >= perAccessory.Int() {
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
