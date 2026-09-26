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

// PlanRequest は導出の入力すべて。ドメインは自分でデータを取りに行かない。
//
// Target は週目標。Program から取らないのは、週目標が「設定（頻度と
// 1回の量）から導く値」であって、集約の持ち物ではなくなったため
// （呼び出し側が seed.DefaultWeeklyTarget で組む。理由は seed パッケージの
// DefaultWeeklyTarget のコメント）。
type PlanRequest struct {
	Program    *program.Program
	Target     program.WeeklyVolumeTarget
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
	if req.Target.IsEmpty() {
		return PlannedSession{}, errors.New("週目標が指定されていない")
	}
	if req.Date.IsZero() {
		return PlannedSession{}, errors.New("対象日が指定されていない")
	}

	pool := usablePool(req.Pool, req.Program)

	// 当日の記録を落とすのはここ1箇所。これより下で req.History と書かない。
	//
	// 今日の計画はその日の始まりに確定させると決めてある（D-116）。重量の
	// 推定も、残差も、軸も、補助の選択も、全て前日までの履歴から決める。
	// 以前は使う側が毎回「前日までに切る」を書いていて、1箇所でも忘れると
	// ジムで1セット記録するたびに計画が自分の下で動く。どこで忘れても別の
	// 形で出るので、症状から原因に辿りにくい。実際に出た・出うる形：
	//
	//   - 残差：1セット記録するたびに残差が動いて、選ばれる補助と並びが
	//     変わる。消化している最中にリストが入れ替わる
	//   - 重量の推定：1セット目を記録した瞬間に推定1RMが動いて、2セット目の
	//     提示重量が変わる。しかも RIR を守ってきついセットをこなすほど
	//     推定が上がるので、**追い込むほど次が重くなる**
	//   - 軸（stalest）：1セット記録した瞬間に「最後にやったのが最も古い
	//     種目」が入れ替わり、今日の軸が別の種目になる
	//   - バリエーション（recentlyPerformed）：今日ラーセンを1セット記録して
	//     開き直した瞬間に系統が「最近やった」になり、バリエーションが消える
	//   - 分割：周期は出席回数で進むので、1セット記録した瞬間に今日が1回に
	//     数えられ、上の日が下の日に変わる。1回ぶんの天井（activeCount）の
	//     起点も1つずれる
	//   - 重点種目の一巡：今日のセッションが1回に数えられて位置が進み、
	//     軸の強度か種目が変わる
	//
	// かつて別々に手当てしていた不具合（終えた補助が再提示される、記録すると
	// 種目が消える、並びが入れ替わる、枠が補充されて終わらない）は、すべて
	// この1点の派生だった。
	//
	// 当日を含めるのは画面の「充足」だけで、あれは query 側の別経路。
	// 表示は「どれだけやったか」、計画は「今日やると決めたこと」。
	//
	// 受け入れ条件は TestSessionPlanner_PlanIsFixedForTheWholeDay。
	//
	// 履歴は2種類ある。history は記録のまま（数える・日付を見る）。
	// estimable は実効負荷（体重込み）に直したもので、推定にだけ渡す。
	// 重量が違うので、数える・日付を見る・記録を見せる経路には渡さない。
	history := req.History.Before(req.Date)
	estimable := effectiveHistory(history, pool, req.Conditions)

	// 2段。何をやるか（種目と役割）を決めてから、何kgでやるかを付ける。
	//
	// 重量の側から種目の側への依存は無い。逆向きは残差に使うセット数だけで、
	// それは役割の表から引くので処方の結果を待たない。種目の決め方を変える
	// PR と重量の決め方を変える PR が同じ流れを触らずに済む。
	lineup, err := p.selectLineup(history, req.Program, req.Target, pool, req.Pool, req.Date)
	if err != nil {
		return PlannedSession{}, err
	}
	return p.prescribe(lineup, estimable, req.Conditions, req.Date, req.Program.SessionVolume().Sets()), nil
}

// lineupEntry は今日やる種目1つと、その役割。重量はまだ付いていない。
type lineupEntry struct {
	exercise *exercise.Exercise
	role     laneRole
}

// selectLineup は今日やる種目とその役割の並びを決める。軸 → バリエーション →
// 補助の順。強度・セット数・RIR はここでは決めない。
//
// history は前日まで（Plan が切る）。pool は選択された種目、master は
// マスタ全件。カバレッジと補助の選択にはマスタ全件を渡す（理由は各所）。
func (p SessionPlanner) selectLineup(
	history setlog.History, prog *program.Program, target program.WeeklyVolumeTarget,
	pool, master []*exercise.Exercise, date training.Date,
) ([]lineupEntry, error) {
	// 今日の分割。周期は暦ではなく出席回数で進む。休んだ日に飛ぶと、
	// 通っていないのに分割だけが回る。
	today, hasSplit := prog.SplitOn(history.SessionCount())

	// 宣言がプールに1つも残っていないのは設定の破れ。分割で絞られて
	// ゼロになるのとは別物で、こちらは計画を出さずに止める。
	declared := declaredExercises(pool, prog)
	if len(declared) == 0 {
		// 到達しない。NewProgram が宣言ゼロを弾き、declared ⊂ selected なので
		// pool に必ず1つ以上残る。集約の不変条件が破れたときの最後の砦として残す。
		return nil, errors.New("伸ばしたい種目が1つも選ばれていない")
	}

	// 軸は宣言のうち、今日の分割の区分を主働に含むもので最も古いもの。
	//
	// 該当が無ければ軸は空。5分割の肩・腕には BIG3 の中に主働を持つ
	// 種目が無く、そういう日が実際にできる。0.88 のスクワットを肩の日に
	// 出すより、軸の枠が無いほうが正直（2026-09-19 の仕様書）。
	var lineup []lineupEntry
	heavy, axisRole := axis(history, prog, pool, declared, today, hasSplit)

	// 直近4週のカバレッジ。窓は前日までの27日ぶんで、当日を足して28日。
	// 長さの理由は CoverageWindowWeeks に書いた。
	//
	// date-28 にしてはいけない。4週前の同じ曜日のセッションが窓に残り、
	// 同じ曜日に通う人は定常状態で不足が 0 になって補助が出なくなる。
	//
	// 暦週をやめたのは、週の先頭でリセットされるため。埋めきった週末は
	// セッションが短くなり（実測18セット）、週明けに全区分の不足が
	// 最大になって一日で使い尽くしていた。
	//
	// 当日の記録は見ない。history が前日までなのに加えて、窓の上端も
	// 前日で切る。CoverageBetween は画面の「充足」が当日込みで使う
	// 公開関数なので、当日を外すのは呼ぶ側の窓で言う。
	//
	// 数えるのはマスタ全件（master）で、選択された種目だけではない。
	// やったセットは、いま選択しているかに関係なく、やったセット。pool で
	// 数えると、種目を選択から外した瞬間にその記録が読み飛ばされ、区分の
	// 残差が窓の長さのあいだふくらむ。画面の「充足」もマスタ全件で数えて
	// いるので、そちらとも食い違う（#133）。
	coverage := CoverageBetween(history, master, date.AddDays(-(CoverageWindowDays - 1)), date.AddDays(-1))

	// 今日すでに積む分（軸とバリエーション）。セット数は役割の表から引く。
	// 処方を待たないのは、重量の側へ依存を作らないため。
	sets := prog.SessionVolume().Sets()
	thisSession := StimulusCoverage{}
	if heavy != nil {
		lineup = append(lineup, lineupEntry{exercise: heavy, role: axisRole})
		thisSession = thisSession.Plus(heavy.Stimulus(), p.prescriptionFor(axisRole, sets).setCount())
	}

	exclude := accessoryExcluded(pool, prog)
	if v := variationLift(history, prog, pool, heavy, date, today, hasSplit); v != nil {
		lineup = append(lineup, lineupEntry{exercise: v, role: variationRole})
		thisSession = thisSession.Plus(v.Stimulus(), p.prescriptionFor(variationRole, sets).setCount())
		exclude = append(exclude, v.ID())
	}

	// 分割があるときだけ天井を掛ける。理由は SessionResidual に書いた。
	var active ActiveCount
	if hasSplit {
		active = activeCount(history, prog)
	}
	gaps := SessionResidual(target, coverage, thisSession, active)

	// 今日の分割に属さない区分は狙わない。残差から落とすのは補助の
	// 選択に効かせるためで、週目標そのものは変えない。窓が1週なので、
	// 落とした分は次にその分割が来た日に残ったまま出てくる。
	//
	// ただし**どの日にも属さない区分は毎日活かす**。腹はどの日にやっても
	// よい部位で、どのプリセットにも入っていない。素直に落とすと永久に
	// 埋まらない（実測で腹斜筋が全プリセット・全頻度で 0%）。
	if hasSplit {
		cycle := prog.Cycle()
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
	for _, e := range master {
		if e != nil && !prog.Includes(e.ID()) {
			exclude = append(exclude, e.ID())
		}
	}
	// 補助に割ける枠は、1回の種目数から、すでに並んだ軸とバリエーションを
	// 引いた残り。
	//
	// 取り分を先に決め打ちしない。軸が立たない日（分割で狙う区分に宣言種目が
	// 無い）もバリエーションが出ない日もあるので、実際に並んだぶんを引く。
	// 決め打ちにすると、軸が空の日に予算が余ったまま終わる。
	//
	// 以前は枠が AccessorySelector の maxSlots = 8 という定数で、軸を足した
	// 9種目27セットが全頻度・全セッションで固定的に出ていた。
	//
	// 選択器を毎回組み直すのは、枠数とセット数が利用者の設定だから。回復
	// 日数だけが方針で、組み立て時のものをそのまま使う。
	slots := prog.SessionVolume().Exercises() - len(lineup)
	if slots <= 0 {
		return lineup, nil
	}
	selector, err := NewAccessorySelector(p.accessory.RecoveryDays(), sets, slots)
	if err != nil {
		return nil, fmt.Errorf("補助の枠が組めない: %w", err)
	}

	// Select が返すのは pool の中の種目に限る。候補は master から exclude を
	// 引いたもので、pool（選択された種目）に無いものは全て exclude に入れて
	// あるので、findExercise が nil を返す経路は無い。
	for _, id := range selector.Select(target, gaps, master, history, date, exclude) {
		if e := findExercise(pool, id); e != nil {
			lineup = append(lineup, lineupEntry{exercise: e, role: accessoryRole})
		}
	}
	return lineup, nil
}

// usablePool はプログラムで選択された種目を ID 昇順で返す。
//
// 選択されていない種目は出てこない。以前は「バリエーションはメインに付随して
// 自動で回るため、選択に含まれていなくても候補にする」という抜け道があったが、
// D-114 で塞いだ。7種目は移行で選択に追加してある。
//
// 並びを固定するのは、同じ入力から同じ計画が出るようにするため。軸の選定が
// 同点のときにここの順序で決まる。
func usablePool(pool []*exercise.Exercise, prog *program.Program) []*exercise.Exercise {
	out := make([]*exercise.Exercise, 0, len(pool))
	for _, e := range pool {
		if e == nil {
			continue
		}
		if prog.Includes(e.ID()) {
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

func findExercise(pool []*exercise.Exercise, id exercise.ExerciseID) *exercise.Exercise {
	for _, e := range pool {
		if e.ID() == id {
			return e
		}
	}
	return nil
}
