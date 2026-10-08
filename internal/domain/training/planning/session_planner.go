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
	accessory AccessoryAllocator
	analyzer  ConditionAnalyzer
}

func NewSessionPlanner(
	estimator OneRepMaxEstimator,
	accessory AccessoryAllocator,
	analyzer ConditionAnalyzer,
) (SessionPlanner, error) {
	if estimator.IsZero() {
		return SessionPlanner{}, errors.New("推定器が未設定である")
	}
	if accessory.IsZero() {
		return SessionPlanner{}, errors.New("補助の割り振り器が未設定である")
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
		accessory: DefaultAccessoryAllocator(),
		analyzer:  DefaultConditionAnalyzer(),
	}
}

func (p SessionPlanner) IsZero() bool { return p == SessionPlanner{} }

// Forecast は今日を含めて頻度ぶんの先の回を、各回に重量まで付けて返す。
// 回0が今日。Plan はその回を取り出すだけ（経路を1本にする。設計書
// 「Plan(req) = Forecast(req) の回0」）。
//
// 先の回は「このまま予定どおりこなした場合の、今日の時点の見込み」。
// 軸・バリエーション・分割の日は ProjectHorizon の予測をそのまま使い、
// 補助は割り振り器が回ごとに割り当てたものを使う。重量は前日までの
// 履歴（今日の時点の推定1RM）でその回を prescribe するだけで、先の回を
// 実際にこなした場合の伸びは見込まない（設計書「今日の実力でその回の
// 処方をしたら何kgか」）。評価日は常に req.Date（今日）で、
// sess.Date()（その回自身の日付）はここでは使わない。使うと、42日の
// 鮮度判定と上乗せの判定が「その回が来たとき」を基準に動いてしまう。
//
// 直すところ：以前（selectLineup）は今日の空き枠が0だと補助の割り振り
// を丸ごと飛ばしていた。先の回に枠があるかもしれないので、今日の枠が
// 0でも割り振りは最後まで回す（回0の補助はその場合ちょうど0件になる
// だけ）。
func (p SessionPlanner) Forecast(req PlanRequest) ([]PlannedSession, error) {
	if p.IsZero() {
		return nil, errors.New("セッション生成器が未設定である")
	}
	if req.Program == nil {
		return nil, errors.New("プログラムが指定されていない")
	}
	if req.Target.IsEmpty() {
		return nil, errors.New("週目標が指定されていない")
	}
	if req.Date.IsZero() {
		return nil, errors.New("対象日が指定されていない")
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
	//     数えられ、上の日が下の日に変わる。先の回の予測（ProjectHorizon）の
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

	declared := declaredExercises(pool, req.Program)
	if len(declared) == 0 {
		// 到達しない。NewProgram が宣言ゼロを弾き、declared ⊂ selected
		// なので pool に必ず1つ以上残る（req.Pool 自体が空の異常入力を
		// 除く。その場合はこの分岐が最後の砦になる）。
		return nil, errors.New("伸ばしたい種目が1つも選ばれていない")
	}

	sessions, err := p.ProjectHorizon(history, req.Program, pool, req.Date)
	if err != nil {
		return nil, fmt.Errorf("先の回の予測に失敗: %w", err)
	}
	exercisesPerSession := req.Program.SessionVolume().Exercises()
	horizon := toHorizonSessions(sessions, exercisesPerSession)

	// 補助の候補プールから外す種目：宣言種目と、重点種目の系統全体
	// （lineage は重点種目本体と全派生を返すので、回ごとに選ばれる
	// バリエーションがどれでもここで一括して外れる）。選択されていない
	// 種目も外す。
	exclude := accessoryExcluded(pool, req.Program)
	for _, e := range req.Pool {
		if e != nil && !req.Program.Includes(e.ID()) {
			exclude = append(exclude, e.ID())
		}
	}

	// 評価日 E は予測の最後の回の日。窓は [E-27, E]（設計書「損失」）。
	evalDate := horizon[len(horizon)-1].Date
	baseline := CoverageBetween(history, req.Pool, evalDate.AddDays(-(CoverageWindowDays - 1)), evalDate)

	setsPerAccessory, err := training.NewSetCount(req.Program.SessionVolume().Sets())
	if err != nil {
		return nil, fmt.Errorf("補助のセット数が不正: %w", err)
	}

	// 重点種目の系統が余分に入れる刺激のぶん、その区分の目標を上げる
	// （raiseForFocus）。画面の充足も同じ目標を見る（WeeklyTarget）。
	target := raiseForFocus(req.Target, sessions)

	allocations, err := p.accessory.Allocate(AllocationRequest{
		Target: target, Baseline: baseline, Sessions: horizon,
		Cycle: req.Program.Cycle(), SetsPerAccessory: setsPerAccessory,
		Pool: candidateAccessories(req.Pool, exclude), Master: req.Pool,
		History: history,
	})
	if err != nil {
		return nil, fmt.Errorf("補助の割り振りに失敗: %w", err)
	}

	out := make([]PlannedSession, len(sessions))
	for k, sess := range sessions {
		var lineup []lineupEntry
		if heavy, _, ok := sess.Axis(); ok {
			role := sess.axisLaneRole()
			lineup = append(lineup, lineupEntry{
				exercise: heavy, role: role,
				reps: axisRepTargets(req.Program, heavy, role),
			})
		}
		if v, _, ok := sess.Variation(); ok {
			lineup = append(lineup, lineupEntry{exercise: v, role: variationRole, reps: program.DefaultRepTargets()})
		}
		for _, id := range allocations[k] {
			// Allocate が返すのは Pool（= pool から exclude を引いたもの）
			// の中の種目に限るので、findExercise が nil を返す経路は無い。
			if e := findExercise(pool, id); e != nil {
				lineup = append(lineup, lineupEntry{exercise: e, role: accessoryRole, reps: program.DefaultRepTargets()})
			}
		}

		split, hasSplit := sess.Split()
		out[k] = p.prescribe(lineup, estimable, req.Conditions, req.Date,
			req.Program.SessionVolume().Sets(), split, hasSplit)
	}
	return out, nil
}

// Plan はその日のセッションを導出する。Forecast の回0を取り出すだけ。
//
// 経路を1本にする（設計書「Plan(req) = Forecast(req) の回0」）。2本の
// 経路があると、どちらかだけ直したときに今日の計画と見込みがずれる。
//
// 未来のセッションはどこにも保存しない。今日のメニューも先のメニューも
// Forecast を対象日で呼んだ結果でしかない。だから予定と実績が食い違う
// 状態が原理的に発生しない。
//
// 受け入れ条件は TestSessionPlanner_PlanIsFixedForTheWholeDay。当日の
// 記録を落とす規約（history := req.History.Before(req.Date)）は Forecast
// 側に1箇所だけ残る。
func (p SessionPlanner) Plan(req PlanRequest) (PlannedSession, error) {
	sessions, err := p.Forecast(req)
	if err != nil {
		return PlannedSession{}, err
	}
	return sessions[0], nil
}

// lineupEntry はセッション1回ぶんの種目1つと、その役割。重量はまだ
// 付いていない。
//
// reps は軸の役割で狙うレップ数。軸以外の役割では使わない（既定を入れておく）。
type lineupEntry struct {
	exercise *exercise.Exercise
	role     laneRole
	reps     program.RepTargets
}

// axisRepTargets は軸に立った種目のレップ数を宣言から引く。
//
// 派生が軸に立つ日（focusVariationRole）は、派生自身ではなく重点種目の
// 値を使う。派生を宣言していなくても、重点種目の軽い番で出すため。
func axisRepTargets(prog *program.Program, axis *exercise.Exercise, role laneRole) program.RepTargets {
	if role == focusVariationRole {
		if focus, ok := prog.FocusExercise(); ok {
			return prog.RepTargetsFor(focus)
		}
	}
	return prog.RepTargetsFor(axis.ID())
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
