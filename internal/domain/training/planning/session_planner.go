package planning

import (
	"errors"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

const (
	// accessoryIntensityPct は補助種目の強度。RIR2 で10レップ前後を狙う位置。
	accessoryIntensityPct = 0.71
	accessoryTargetRIR    = 2
)

// PlanRequest は導出の入力すべて。ドメインは自分でデータを取りに行かない。
type PlanRequest struct {
	Program    *program.Program
	Pool       []*exercise.Exercise
	History    setlog.History
	Conditions condition.ConditionLog
	Date       training.Date
}

// SessionPlanner はドメインの入口となるドメインサービス。無状態。
type SessionPlanner struct {
	slots     SlotCatalog
	estimator OneRepMaxEstimator
	accessory AccessorySelector
	analyzer  ConditionAnalyzer
}

func NewSessionPlanner(
	slots SlotCatalog,
	estimator OneRepMaxEstimator,
	accessory AccessorySelector,
	analyzer ConditionAnalyzer,
) (SessionPlanner, error) {
	if estimator.IsZero() {
		return SessionPlanner{}, errors.New("推定器が未設定である")
	}
	if accessory.IsZero() {
		return SessionPlanner{}, errors.New("補助種目の選択器が未設定である")
	}
	if analyzer.IsZero() {
		return SessionPlanner{}, errors.New("コンディション分析器が未設定である")
	}
	return SessionPlanner{
		slots: slots, estimator: estimator,
		accessory: accessory, analyzer: analyzer,
	}, nil
}

func DefaultSessionPlanner() SessionPlanner {
	return SessionPlanner{
		slots:     NewSlotCatalog(),
		estimator: DefaultOneRepMaxEstimator(),
		accessory: DefaultAccessorySelector(),
		analyzer:  DefaultConditionAnalyzer(),
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
	estHistory := effectiveHistory(historyBefore(req), pool, req.Conditions)
	heavy := p.heavyLift(req, pool)
	if heavy == nil {
		// 到達しない。NewProgram が宣言ゼロを弾き、declared ⊂ selected なので
		// pool に必ず1つ以上残る。集約の不変条件が破れたときの最後の砦として残す。
		return PlannedSession{}, errors.New("伸ばしたい種目が1つも選ばれていない")
	}

	template, ok := p.slots.Select(req.Program.Frequency(), liftIndexInWeek(historyBefore(req), heavy.ID(), req.Date))
	if !ok {
		return PlannedSession{}, fmt.Errorf(
			"週%d回に対応するスロット構成が無い", req.Program.Frequency().PerWeek())
	}

	rirBump := p.analyzer.RIRAdjustment(req.Conditions, req.Date)

	// 週内カバレッジは前日まで。当日の記録は見ない。
	//
	// 今日のリストは、その日が始まった時点で確定していてほしい。当日を
	// 含めると、1セット記録するたびに残差が動いて選ばれる種目と並びが
	// 変わり、ジムで消化している最中にリストが自分の下で入れ替わる。
	//
	// 画面に出る「今週の充足」は当日を含む（query.Stats.WeeklyVolume）。
	// 表示は「今週どれだけやったか」、計画は「今日やると決めたこと」で
	// 意味が違うため、基準が違ってよい。
	coverage := CoverageBetween(req.History, pool, req.Date.WeekStart(), req.Date.AddDays(-1))

	set, performed := p.planMain(req, pool, estHistory, heavy, template, rirBump)
	coverage = coverage.Plus(performed.Stimulus(), set.Sets())

	// 設定より多く通った場合でも、残り1セッション分は狙えるようにする。
	// 0 以下にすると残差が空になり、補助が1つも出ないまま
	// メイン種目のフルスロットだけが積まれる。
	sessionsRemaining := max(1, req.Program.Frequency().PerWeek()-sessionIndexInWeek(req.History, req.Date))
	gaps := SessionResidual(req.Program.WeeklyTarget(), coverage, sessionsRemaining)

	// 補助の候補も並びも、その日の始まりに分かっていたことだけで決める。
	//
	// 以前はここで「今日ぶんを終えた種目を外す」「着手中は残す」「終えた
	// ぶん枠を減らす」「並びをIDの昇順で固定する」という手当てをしていた。
	// どれも当日を見ていたことの帰結で、見なくなれば要らない（D-116）。
	//
	// Select が返す順序は「最も放置している区分から」という優先度で、
	// 一日中変わらないので、そのまま画面の並びになる。
	chosen := p.accessory.Select(gaps, pool, historyBefore(req), req.Date, heavy.ID())

	accessories := make([]PlannedSet, 0, len(chosen))
	for _, id := range chosen {
		accessories = append(accessories, p.planAccessory(req, pool, estHistory, id, rirBump))
	}

	return PlannedSession{
		date:        req.Date,
		main:        []PlannedSet{set},
		accessories: accessories,
	}, nil
}

// usablePool はプログラムで選択された種目と、その派生バリエーションを返す。
//
// バリエーションはユーザーが個別に選ぶものではなく、メインに付随して
// 自動で回るため、選択に含まれていなくても候補にする。
func (p SessionPlanner) usablePool(req PlanRequest) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, len(req.Pool))
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

// declaredExercises はプログラムで宣言された種目だけを返す。派生バリエーションは含まない。
func declaredExercises(pool []*exercise.Exercise, prog *program.Program) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, len(pool))
	for _, e := range pool {
		if prog.Declares(e.ID()) {
			out = append(out, e)
		}
	}
	return out
}

// planMain は1つのメインリフトのスロットを埋める。
// 2つ目の返り値は実際に行う種目（バリエーションに差し替わることがある）。
func (p SessionPlanner) planMain(
	req PlanRequest,
	pool []*exercise.Exercise,
	historyBefore setlog.History,
	main *exercise.Exercise,
	template SlotTemplate,
	rirBump int,
) (PlannedSet, *exercise.Exercise) {
	target := main

	intensity := template.Intensity()

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
	if orm, ok := p.estimator.Estimate(historyBefore, target.ID(), req.Date); ok {
		if w, err := orm.WorkWeight(intensity, target.Increment()); err == nil {
			// 推定も処方も実効負荷（体重込み）で通し、出口で加重に戻す。
			set.weight, set.hasWeight = AddedWeight(w, target, req.Conditions, req.Date), true
		}
	}
	return set, target
}

func (p SessionPlanner) planAccessory(
	req PlanRequest,
	pool []*exercise.Exercise, historyBefore setlog.History, id exercise.ExerciseID, rirBump int,
) PlannedSet {
	baseRIR, err := training.NewRIR(accessoryTargetRIR)
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

	intensity, err := training.NewIntensityPct(accessoryIntensityPct)
	if err != nil {
		return set
	}

	if orm, ok := p.estimator.Estimate(historyBefore, id, req.Date); ok {
		if w, err := orm.WorkWeight(intensity, exercise.Increment()); err == nil {
			set.weight, set.hasWeight = AddedWeight(w, exercise, req.Conditions, req.Date), true
		}
	}
	return set
}

// historyBefore は当日より前の履歴。重量の推定に使う。
//
// 週内カバレッジ（D-021）は当日を含めるが、重量の推定は含めない。
// 前者は「今日どれだけ埋めたか」で当日が本質、後者は「今日いくつで
// やるか」で、当日の結果が入ると同じセッションの中で目標が動く。
func historyBefore(req PlanRequest) setlog.History {
	return req.History.Before(req.Date)
}

func findExercise(pool []*exercise.Exercise, id exercise.ExerciseID) *exercise.Exercise {
	for _, e := range pool {
		if e.ID() == id {
			return e
		}
	}
	return nil
}

// CoverageBetween は期間内に埋めた刺激量を数える。両端を含む。
//
// 記録1件を1セットとして数える。SetLog は「確定した実績1セット」なので、
// 件数がそのままセット数になる。
//
// 公開しているのは、週目標の充足を見せる読み取り経路が同じ数え方を
// 必要とするため。別々に実装すると、画面に出る数字とエンジンが使う数字が
// ずれる。ずれた瞬間、どちらが正しいのか誰にも分からなくなる。
func CoverageBetween(h setlog.History, pool []*exercise.Exercise, from, to training.Date) StimulusCoverage {
	coverage := StimulusCoverage{}
	one, err := training.NewSetCount(1)
	if err != nil {
		return coverage
	}

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(pool))
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

// sessionIndexInWeek はその週で対象日が何本目のセッションか（0始まり）。
// 曜日の割り当てはドメインの責務ではないため、実績から導出する。
func sessionIndexInWeek(h setlog.History, date training.Date) int {
	return h.OnOrAfter(date.WeekStart()).Before(date).SessionCount()
}

// liftIndexInWeek はその種目を、今週すでに何回やったかを返す。
//
// 週の何本目かではなく種目ごとに数えるのは、ヘビー枠が1セッションに
// 1つになったため。宣言が3つあれば、週3回通ってもベンチは週1回しか
// 出ない。週の本数で引くと、その1回に「週3本目＝軽い日」が当たる。
func liftIndexInWeek(h setlog.History, id exercise.ExerciseID, date training.Date) int {
	return h.ForExercise(id).OnOrAfter(date.WeekStart()).Before(date).SessionCount()
}

// heavyLift は今日メインでやる＝高重量を扱う種目を返す。
//
// 宣言のうち、最後に実施したのが最も古い種目を返す。未着手の種目があればそれを優先する。
func (p SessionPlanner) heavyLift(req PlanRequest, pool []*exercise.Exercise) *exercise.Exercise {
	h := historyBefore(req)

	var stalest *exercise.Exercise
	var stalestDate training.Date

	for _, c := range declaredExercises(pool, req.Program) {
		last, ok := h.LastPerformed(c.ID())
		if !ok {
			return c // 未着手の種目があればそれを優先する
		}
		if stalest == nil || last.Before(stalestDate) {
			stalest = c
			stalestDate = last
		}
	}
	return stalest

}
