package planning

import (
	"errors"
	"slices"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

const (
	// 軸レーンの処方。3レーンで最も重い。
	//
	// 表を引かず定数にしているのは、週の何本目かで強度を変える必要が
	// 無くなったため。表は「同じ種目を週に何度もやるなら強度を散らす」
	// ための仕組みだったが、宣言種目は「最後にやったのが最も古いもの」で
	// 回るので、宣言が3つあれば各種目は週1回しか軸に来ない（D-117）。
	// 散らす相手がいない。
	//
	// 派生を重ねたいときはバリエーションレーンが受け持つ。そちらは
	// 0.80 で、軸とは別の種目・別の推定1RMを使う。
	heavyIntensityPct = 0.88
	heavySets         = 3
	heavyTargetRIR    = 1

	// accessoryIntensityPct は補助種目の強度。RIR2 で10レップ前後を狙う位置。
	accessoryIntensityPct = 0.71
	accessoryTargetRIR    = 2

	// バリエーションの処方。軸より軽く、補助より重い。
	//
	// 表を引かず定数にしているのは、派生が週に何回出ようと強度を変える
	// 理由が無いため。同じ種目の中で強度を回すのは「同じ種目を週に何回も
	// やる」ことが前提で、派生は別種目として自分の推定1RMを持つ（D-113）。
	// 種目が違えば重量は自然に違う。
	//
	// 表から持ってきた値は 0.81 / 4セットだったが、0.81 は表の中で
	// 0.88 や 0.76 と並んで初めて意味を持つ刻みで、単独では半端。
	// 軸 0.88 と補助 0.71 の間に置く一つの値としては 0.80 でいい。
	// 4セットは軸と合わせて胸の実測が週目標の134%まで出ていたので3に落とす。
	variationIntensityPct = 0.80
	variationSets         = 3
	variationTargetRIR    = 2

	// variationRecoveryDays は同じ系統を再び出すまでに空ける日数。
	//
	// 2 は「中1日」で、月曜にやったら火曜は出さず水曜から出す。判定は
	// AccessorySelector.recovering と同じ開区間 (date - N, date)。
	//
	// recoveryDays と値が同じだが共有しない。あちらは筋区分の回復で
	// コンストラクタの引数、こちらは系統の間隔で設定にしない。共有すると
	// 片方を動かしたときにもう片方が黙って動く。
	variationRecoveryDays = 2
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
	estimator OneRepMaxEstimator
	accessory AccessorySelector
	analyzer  ConditionAnalyzer
}

func NewSessionPlanner(
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
		estimator: estimator, accessory: accessory, analyzer: analyzer,
	}, nil
}

func DefaultSessionPlanner() SessionPlanner {
	return SessionPlanner{
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

	rirBump := p.analyzer.RIRAdjustment(req.Conditions, req.Date)

	// 直近1週のカバレッジ。窓は前日までの6日ぶんで、当日を足して7日。
	//
	// date-7 にしてはいけない。先週の同じ曜日のセッションが窓に残り、
	// 同じ曜日に通う人は定常状態で不足が 0 になって補助が出なくなる。
	//
	// 暦週をやめたのは、週の先頭でリセットされるため。埋めきった週末は
	// セッションが短くなり（実測18セット）、週明けに全区分の不足が
	// 最大になって一日で使い尽くしていた。
	//
	// 当日の記録は見ない。
	// 当日を含めると、1セット記録するたびに残差が動いて選ばれる種目と並びが
	// 変わり、ジムで消化している最中にリストが自分の下で入れ替わる。
	coverage := CoverageBetween(req.History, pool, req.Date.AddDays(-6), req.Date.AddDays(-1))

	set := p.planHeavy(req, estHistory, heavy, rirBump)
	thisSession := StimulusCoverage{}.Plus(heavy.Stimulus(), set.Sets())

	variation := make([]PlannedSet, 0, 1)
	exclude := accessoryExcluded(pool, req.Program)
	if v := p.variationLift(req, pool, heavy); v != nil {
		vs := p.planVariation(req, estHistory, v, rirBump)
		variation = append(variation, vs)
		thisSession = thisSession.Plus(v.Stimulus(), vs.Sets())
		exclude = append(exclude, v.ID())
	}

	gaps := SessionResidual(req.Program.WeeklyTarget(), coverage, thisSession)

	chosen := p.accessory.Select(gaps, pool, historyBefore(req), req.Date, exclude)
	accessories := make([]PlannedSet, 0, len(chosen))
	for _, id := range chosen {
		accessories = append(accessories, p.planAccessory(req, pool, estHistory, id, rirBump))
	}

	return PlannedSession{
		date:        req.Date,
		main:        []PlannedSet{set},
		variation:   variation,
		accessories: accessories,
	}, nil
}

// usablePool はプログラムで選択された種目を ID 昇順で返す。
//
// 選択されていない種目は出てこない。以前は「バリエーションはメインに付随して
// 自動で回るため、選択に含まれていなくても候補にする」という抜け道があったが、
// D-114 で塞いだ。7種目は移行で選択に追加してある。
//
// 並びを固定するのは、同じ入力から同じ計画が出るようにするため。軸の選定が
// 同点のときにここの順序で決まる。
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

// planHeavy は軸レーンの処方を組み立てる。
//
// 以前は planMain という名前で、頻度と週の何本目かで引いた表を受け取って
// いた。軽い日にベンチをラーセンプレスへ差し替えていた頃の名残で、差し替えを
// やめた時点（D-114）から target は引数そのものに固定されている。表のほうも
// D-117 で宣言種目が順に回るようになった時点で意味を失っていた（D-126）。
func (p SessionPlanner) planHeavy(
	req PlanRequest,
	historyBefore setlog.History,
	target *exercise.Exercise,
	rirBump int,
) PlannedSet {
	return p.prescribe(req, historyBefore, target,
		heavyIntensityPct, heavySets, heavyTargetRIR, rirBump)
}

// planVariation はバリエーションレーンの処方を組み立てる。
//
// 強度・セット数・RIR は定数。軸と同じく、週の何本目かでは変えない。派生は
// それぞれ自分の推定1RMを持つので、種目が違えば重量は自然に違う。
func (p SessionPlanner) planVariation(
	req PlanRequest,
	historyBefore setlog.History,
	target *exercise.Exercise,
	rirBump int,
) PlannedSet {
	return p.prescribe(req, historyBefore, target,
		variationIntensityPct, variationSets, variationTargetRIR, rirBump)
}

// prescribe は「この種目をこの強度で何セット」を1件ぶん組み立てる。
// レーンごとの違いは渡す定数だけ。
//
// 定数を値オブジェクトへ通すのは実行時で、失敗しても種目だけの set に
// 落とす。重量が付かなければ本人が決める。定数が正しい限り発火しないが、
// panic は使わない（TestDomain_PanickingFunctionsStayWhereTheyBelong）。
func (p SessionPlanner) prescribe(
	req PlanRequest,
	historyBefore setlog.History,
	target *exercise.Exercise,
	intensityPct float64, sets, rir, rirBump int,
) PlannedSet {
	set := PlannedSet{exerciseID: target.ID()}

	baseRIR, err := training.NewRIR(rir)
	if err != nil {
		return set
	}
	set.targetRIR = baseRIR.Plus(rirBump)

	set.sets, err = training.NewSetCount(sets)
	if err != nil {
		return set
	}

	intensity, err := training.NewIntensityPct(intensityPct)
	if err != nil {
		return set
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
	return set
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

// heavyLift は今日メインでやる＝高重量を扱う種目を返す。
//
// 宣言のうち、最後に実施したのが最も古い種目を返す。未着手の種目があればそれを優先する。
func (p SessionPlanner) heavyLift(req PlanRequest, pool []*exercise.Exercise) *exercise.Exercise {
	return stalest(historyBefore(req), declaredExercises(pool, req.Program))
}

// stalest は候補のうち、最後に実施したのが最も古い種目を返す。候補が空なら nil。
//
// 未着手の種目があればそれを優先する。記録が無いのを「最も古い」と解釈する
// ため、ゼロ値の日付と比べるのではなく LastPerformed の第2返り値で分ける。
// 日付のゼロ値が何を表すかを知らなくても読める。
//
// 同点は先に見たものを残す。候補は usablePool が ID 昇順に並べているので、
// 同じ入力から同じ種目が返る。
//
// 渡す履歴は前日まで（historyBefore）。当日を含めると、ジムで1セット記録した
// 瞬間に「最も古い」が入れ替わり、今日のメニューが自分の下で変わる（D-086）。
func stalest(h setlog.History, candidates []*exercise.Exercise) *exercise.Exercise {
	var best *exercise.Exercise
	var bestDate training.Date

	for _, c := range candidates {
		last, ok := h.LastPerformed(c.ID())
		if !ok {
			return c
		}
		if best == nil || last.Before(bestDate) {
			best, bestDate = c, last
		}
	}
	return best
}

// variationLift は今日バリエーションとしてやる種目を返す。出さない日は nil
//
// 出さないのは、重点種目が未指定・軸が系統に含まれる・前回やってから十分に日数がアイていない・派生が選択されていない
// のいずれか。
func (p SessionPlanner) variationLift(req PlanRequest, pool []*exercise.Exercise, heavy *exercise.Exercise) *exercise.Exercise {
	focus, ok := req.Program.FocusExercise()
	if !ok {
		return nil
	}

	// 今日の軸が重点種目の系統に含まれる場合、バリエーションは出さない。
	family := lineage(pool, focus)
	if containsExercise(family, heavy.ID()) {
		return nil
	}

	// 前回やってから十分に日数が空いていない場合、バリエーションは出さない。
	h := historyBefore(req)
	if recentlyPerformed(h, family, req.Date) {
		return nil
	}

	return stalest(h, variationsOf(pool, focus))
}

// lineage は重点種目とその派生のうち、pool にあるものを返す。
//
// 重点種目自身を含める。含めないと、軸でベンチをやった翌日にラーセンが出る。
//
// 根まで辿らない。辿ると、重点種目に RDL を指定したとき「RDL の系統」に
// 床引きデッドリフトが入り、バリエーションとして出てしまう。床引きは
// 宣言しなければ出ない（D-117）。
// accessoryExcluded は補助の候補から外す種目を返す。
//
// 宣言種目そのものを外す理由は D-125 のとおり。これに重点種目の派生を
// 足す。派生はバリエーションレーンで出るものなので、補助にも出ると
// 同じ系統が1日に二度来る。
//
// 実害は週5で出た。脚の日に胸の残差が大きく残っていると、補助が
// ベンチの派生（ラーセンプレス・テンポベンチ）を2つ選び、脚の日の
// 上半身ボリュームが 17.1 まで膨らむ。胸を埋めるならインクラインや
// フライで埋めるほうが、系統の回復日程と衝突しない。
//
// **重点種目の系統だけ**を外す。宣言していても重点でない種目の派生
// （RDL・フロントスクワット・デフィシットデッドリフト）は、補助が
// 唯一の出口なので外すと計画から消える。実際に全部外して測ったら、
// 胸が週目標の163%まで超過し、使われない種目が出た。専用レーンを
// 持っているのは重点種目の系統だけ、というのが線引き。
func accessoryExcluded(pool []*exercise.Exercise, prog *program.Program) []exercise.ExerciseID {
	out := prog.DeclaredExercises()

	// 重点種目が未指定なら focus は空ID。lineage は空を返すので、
	// ここで分けない。分けても到達しない分岐が増えるだけ。
	focus, _ := prog.FocusExercise()
	for _, e := range lineage(pool, focus) {
		out = append(out, e.ID())
	}
	return out
}

func lineage(pool []*exercise.Exercise, focus exercise.ExerciseID) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, 4)
	for _, e := range pool {
		if e.ID() == focus {
			out = append(out, e)
			continue
		}
		if from, ok := e.DerivedFrom(); ok && from == focus {
			out = append(out, e)
		}
	}
	return out
}

// variationsOf は重点種目の派生のうち pool にあるものを返す。重点種目自身は含まない。
func variationsOf(pool []*exercise.Exercise, focus exercise.ExerciseID) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, 3)
	for _, e := range pool {
		if from, ok := e.DerivedFrom(); ok && from == focus {
			out = append(out, e)
		}
	}
	return out
}

// recentlyPerformed は系統のどれかを直近 variationRecoveryDays 日にやったか。
//
// AccessorySelector.recovering と同じ開区間 (date - N, date)。区分ではなく
// 系統で見る点だけが違う。
//
// 渡す履歴は前日まで。当日を含めると、今日ラーセンを1セット記録して
// 開き直した瞬間に系統が「最近やった」になり、バリエーションが自分の下で
// 消える（D-086 系）。
func recentlyPerformed(h setlog.History, family []*exercise.Exercise, date training.Date) bool {
	inFamily := make(map[exercise.ExerciseID]bool, len(family))
	for _, e := range family {
		inFamily[e.ID()] = true
	}

	cutoff := date.AddDays(-variationRecoveryDays)
	for _, l := range h.After(cutoff).Before(date).Logs() {
		if inFamily[l.ExerciseID()] {
			return true
		}
	}
	return false
}

// containsExercise は候補のどれかが id か。
//
// ポインタではなく ID で比べる。エンティティの同一性は ID で決まるので
// （Exercise.SameIdentity）、スライスの作り方が変わっても壊れない。
func containsExercise(candidates []*exercise.Exercise, id exercise.ExerciseID) bool {
	return slices.ContainsFunc(candidates, func(e *exercise.Exercise) bool { return e.ID() == id })
}
