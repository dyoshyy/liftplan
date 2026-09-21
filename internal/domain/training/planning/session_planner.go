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

	// 今日の分割。周期は暦ではなく出席回数で進む。休んだ日に飛ぶと、
	// 通っていないのに分割だけが回る。
	today, hasSplit := req.Program.SplitOn(historyBefore(req).SessionCount())

	// 宣言がプールに1つも残っていないのは設定の破れ。分割で絞られて
	// ゼロになるのとは別物で、こちらは計画を出さずに止める。
	declared := declaredExercises(pool, req.Program)
	if len(declared) == 0 {
		// 到達しない。NewProgram が宣言ゼロを弾き、declared ⊂ selected なので
		// pool に必ず1つ以上残る。集約の不変条件が破れたときの最後の砦として残す。
		return PlannedSession{}, errors.New("伸ばしたい種目が1つも選ばれていない")
	}

	// 軸は宣言のうち、今日の分割の区分を主働に含むもので最も古いもの。
	//
	// 該当が無ければ軸は空。5分割の肩・腕には BIG3 の中に主働を持つ
	// 種目が無く、そういう日が実際にできる。0.88 のスクワットを肩の日に
	// 出すより、軸の枠が無いほうが正直（2026-09-19 の仕様書）。
	heavy, heavyPct := p.axis(req, pool, declared, today, hasSplit)

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
	//
	// 数えるのはマスタ全件（req.Pool）で、選択された種目だけではない。
	// やったセットは、いま選択しているかに関係なく、やったセット。pool で
	// 数えると、種目を選択から外した瞬間にその記録が読み飛ばされ、区分の
	// 残差が最大1週間ふくらむ。画面の「今週の充足」もマスタ全件で数えて
	// いるので、そちらとも食い違う（#133）。
	coverage := CoverageBetween(req.History, req.Pool, req.Date.AddDays(-6), req.Date.AddDays(-1))

	main := make([]PlannedSet, 0, 1)
	thisSession := StimulusCoverage{}
	if heavy != nil {
		set := p.planHeavy(req, estHistory, heavy, heavyPct, rirBump)
		main = append(main, set)
		thisSession = thisSession.Plus(heavy.Stimulus(), set.Sets())
	}

	variation := make([]PlannedSet, 0, 1)
	exclude := accessoryExcluded(pool, req.Program)
	if v := p.variationLift(req, pool, heavy, today, hasSplit); v != nil {
		vs := p.planVariation(req, estHistory, v, rirBump)
		variation = append(variation, vs)
		thisSession = thisSession.Plus(v.Stimulus(), vs.Sets())
		exclude = append(exclude, v.ID())
	}

	// 分割があるときだけ天井を掛ける。理由は SessionResidual に書いた。
	var active ActiveCount
	if hasSplit {
		active = p.activeCount(req)
	}
	gaps := SessionResidual(req.Program.WeeklyTarget(), coverage, thisSession, active)

	// 今日の分割に属さない区分は狙わない。残差から落とすのは補助の
	// 選択に効かせるためで、週目標そのものは変えない。窓が1週なので、
	// 落とした分は次にその分割が来た日に残ったまま出てくる。
	//
	// ただし**どの日にも属さない区分は毎日活かす**。腹はどの日にやっても
	// よい部位で、どのプリセットにも入っていない。素直に落とすと永久に
	// 埋まらない（実測で腹斜筋が全プリセット・全頻度で 0%）。
	if hasSplit {
		cycle := req.Program.Cycle()
		for region := range gaps {
			if affiliated(cycle, region) && !today.Includes(region) {
				delete(gaps, region)
			}
		}
	}

	// Select にもマスタ全件を渡し、選択されていない種目は exclude で候補から
	// 落とす。Select は除外した種目も履歴を読む辞書には残すので、外した種目を
	// 前日にやっていれば、その区分は回復中と判定される。pool を渡すと辞書から
	// も消え、前日にやった区分の補助が今日も出る（#133）。
	//
	// 候補と辞書を別の引数に分けなかったのは、「候補にはしないが記録は読む」
	// が exclude の既にある意味そのものだから。
	for _, e := range req.Pool {
		if e != nil && !req.Program.Includes(e.ID()) {
			exclude = append(exclude, e.ID())
		}
	}
	chosen := p.accessory.Select(gaps, req.Pool, historyBefore(req), req.Date, exclude)
	accessories := make([]PlannedSet, 0, len(chosen))
	for _, id := range chosen {
		accessories = append(accessories, p.planAccessory(req, pool, estHistory, id, rirBump))
	}

	return PlannedSession{
		date:        req.Date,
		main:        main,
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

// historyBefore は当日より前の履歴。重量の推定に使う。
//
// 残差も推定も当日を含めない。今日の計画はその日の始まりに確定させると
// 決めてある（D-116）。当日の結果が入ると、1セット記録するたびに目標も
// リストも自分の下で動く。
//
// 当日を含めるのは画面の「今週の充足」だけで、あれは query 側の別経路。
// 表示は「今週どれだけやったか」、計画は「今日やると決めたこと」。
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

// variationLift は今日バリエーションとしてやる種目を返す。出さない日は nil
//
// 出さないのは、重点種目が未指定・軸が系統に含まれる・前回やってから十分に日数がアイていない・派生が選択されていない
// のいずれか。
func (p SessionPlanner) variationLift(
	req PlanRequest, pool []*exercise.Exercise, heavy *exercise.Exercise,
	today program.Split, hasSplit bool,
) *exercise.Exercise {
	focus, ok := req.Program.FocusExercise()
	if !ok {
		return nil
	}

	// 分割があれば、重点種目の主働が今日の集合に含まれる日だけ出す。
	//
	// 止めないと、型が「今日は脚の日」と言いながらベンチの派生が出る。
	// 型の意味を自分で否定することになる。
	//
	// 代償は系統の頻度が下がること。重点ベンチ＋上下2分割なら、上の日の
	// 数がそのまま上限になる。ベンチを週3回やりたいなら上の日を3つ置く
	// 周期を組む、が正しい答えで、順序付きの周期ならそれができる。
	if hasSplit {
		e := findExercise(pool, focus)
		if e == nil || !isPrimaryIn(e, today) {
			return nil
		}
	}

	// 今日の軸が重点種目の系統に含まれる場合、バリエーションは出さない。
	//
	// 軸が空の日がある（分割に該当する宣言が無い日）。そのときは
	// 系統の重複が起きようがないので、この門は素通しする。
	family := lineage(pool, focus)
	if heavy != nil && containsExercise(family, heavy.ID()) {
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
// 消える（D-116 系）。
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
