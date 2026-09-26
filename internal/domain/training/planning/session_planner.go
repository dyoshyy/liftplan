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
	heavy, axisRole := axis(history, prog, pool, declared, today, hasSplit, date)
	if heavy != nil {
		lineup = append(lineup, lineupEntry{exercise: heavy, role: axisRole})
	}

	exclude := accessoryExcluded(pool, prog)
	if v := variationLift(history, prog, pool, heavy, date, today, hasSplit); v != nil {
		lineup = append(lineup, lineupEntry{exercise: v, role: variationRole})
		exclude = append(exclude, v.ID())
	}

	// 補助の候補プールに残さない種目。マスタ全件のうち選択されていない
	// ものも足す（AccessorySelector.Select が以前していたのと同じ理由：
	// 除外した種目の記録は回復の判定に要るので、辞書には残しつつ候補からは
	// 落とす。候補プールと辞書を別の引数に分けないのは AllocationRequest の
	// Pool／Master がその2役をそのまま引き継いでいるため）。
	for _, e := range master {
		if e != nil && !prog.Includes(e.ID()) {
			exclude = append(exclude, e.ID())
		}
	}

	// 補助に割ける今日の枠は、1回の種目数から、すでに並んだ軸と
	// バリエーションを引いた残り。取り分を先に決め打ちしないのは以前と同じ
	// 理由（軸が立たない日・バリエーションが出ない日がある）。
	slots := prog.SessionVolume().Exercises() - len(lineup)
	if slots <= 0 {
		return lineup, nil
	}

	// 先の回（今日を含めて頻度ぶん）を予測し、割り振り器に渡す形へ変換する。
	// 回0の空き枠は、いま確定した lineup から求めた slots で上書きする
	// （ProjectHorizon が計算し直す軸・バリエーションの有無と一致するはずだが
	// 一致は horizon_projector.go の TestProjectHorizon_MatchesPlanWhenFollowedExactly
	// が守っている契約であって、ここでは Plan 自身が確定した値を優先する）。
	sessions, err := p.ProjectHorizon(history, prog, pool, date)
	if err != nil {
		return nil, fmt.Errorf("先の回の予測に失敗: %w", err)
	}
	horizon := toHorizonSessions(sessions, prog.SessionVolume().Exercises())
	if len(horizon) > 0 {
		horizon[0].Slots = slots
	}

	// 評価日 E は予測の最後の回の日。窓は [E-27, E]（設計書「損失」）。
	// 前日までの実際の記録は date より先に伸びないので、上端を E に
	// 取っても実害は無い（History に未来の記録は無い）。
	evalDate := horizon[len(horizon)-1].Date
	baseline := CoverageBetween(history, master, evalDate.AddDays(-(CoverageWindowDays - 1)), evalDate)

	setsPerAccessory, err := training.NewSetCount(prog.SessionVolume().Sets())
	if err != nil {
		return nil, fmt.Errorf("補助のセット数が不正: %w", err)
	}

	allocations, err := p.accessory.Allocate(AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         horizon,
		Cycle:            prog.Cycle(),
		SetsPerAccessory: setsPerAccessory,
		Pool:             candidateAccessories(master, exclude),
		Master:           master,
		History:          history,
	})
	if err != nil {
		return nil, fmt.Errorf("補助の割り振りに失敗: %w", err)
	}

	// 回0（今日）だけを採用する。Allocate が返すのは pool の中の種目に
	// 限るので findExercise が nil を返す経路は無い。
	for _, id := range allocations[0] {
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
