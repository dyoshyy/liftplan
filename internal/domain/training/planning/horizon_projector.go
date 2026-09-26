package planning

import (
	"errors"
	"fmt"
	"math"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// ProjectedSession は先の回1つぶんの予測。割り振り器（次のPR）が読む形だけを持つ。
//
// 重量は付けない。Plan の2段（何をやるか→何kgでやるか）のうち、予測が
// 要るのは前段だけ。後段（prescribeSet）は当日の推定1RMに依存し、実際に
// その日が来るまで確定させない値なので、先取りしても割り振り器は使わない。
type ProjectedSession struct {
	date          training.Date
	split         program.Split
	hasSplit      bool
	axis          *exercise.Exercise
	axisRole      laneRole
	axisSets      training.SetCount
	variation     *exercise.Exercise
	variationSets training.SetCount
	stimulus      StimulusCoverage
}

func (s ProjectedSession) Date() training.Date { return s.date }

// Split はその回の分割の日。分割が無ければ2番目の戻り値が false（全区分を狙う）。
func (s ProjectedSession) Split() (program.Split, bool) { return s.split, s.hasSplit }

// Axis はその回の軸と、その処方のセット数。軸が無い日は3番目の戻り値が false。
//
// laneRole（heavyRole・focusVolumeRole）は公開しない。役割が外へ与える
// 効果はレーン（Axis から返るか Variation から返るか）とセット数だけで、
// 両方ともここで既に表現している。役割そのものが要る日が来たら、そのとき
// 公開する（必要になるまで作らない）。
func (s ProjectedSession) Axis() (*exercise.Exercise, training.SetCount, bool) {
	return s.axis, s.axisSets, s.axis != nil
}

// axisLaneRole はその回の軸が担う役割（重い日・重点種目の一巡の
// ボリュームの日）。パッケージの外には出さない。Forecast がその回を
// prescribe するときに、軸を heavyRole 固定ではなく実際の役割で処方する
// ために要る（設計書「役割（重い日・ボリュームの日・派生）も使う」）。
// 役割が外へ与える効果は Axis() が既に表現しているので、公開はしない
// （必要になるまで作らない）。
func (s ProjectedSession) axisLaneRole() laneRole { return s.axisRole }

// Variation はその回のバリエーションと、その処方のセット数。出ない日は
// 3番目の戻り値が false。
func (s ProjectedSession) Variation() (*exercise.Exercise, training.SetCount, bool) {
	return s.variation, s.variationSets, s.variation != nil
}

// Stimulus はその回に軸・バリエーションがすでに入れる刺激。割り振り器が
// 損失を計算する材料の一部になる（補助はまだ載っていない。回0だけが
// 補助込みの実際の刺激を Plan から得る）。
func (s ProjectedSession) Stimulus() StimulusCoverage { return s.stimulus }

// ProjectHorizon は今日を含めて頻度ぶんの先の回を予測する。まだ Plan からは
// 使わない（設計書 PR2）。
//
// 日付は今日から 7/頻度 日ごとの等間隔を仮定し、整数日へ丸める
// （horizonDates）。回ごとの分割の日・軸・バリエーションは、Plan が今日に
// 使うのと同じ関数（axis・heavyLift・focusCyclePosition・variationLift・
// prescriptionFor(...).setCount()）で決める。予測と実際の決め方がずれようが
// ない（TestProjectHorizon_MatchesPlanWhenFollowedExactly が守る）。
//
// 前の回の軸・バリエーションを実施したものとして履歴を仮に進めてから
// 次の回を決める。進めないと、2回目以降の予測が「1回目をやらなかった
// 世界線」のまま決まり、周期の位置（focusCyclePosition）も最終実施日
// （stalest・LastPerformed）もずれる。
//
// 【設計書の難所】軸を決める関数は History を受け取るが、実際に読むのは
// 最終実施日（LastPerformed）とセッション数（SessionCount・Sessions）だけで、
// 重量やレップは読まない。仮の記録として組み立てる SetLog には重量・
// レップが要る（コンストラクタが検証する）が、値そのものはどの関数の
// 判断にも使われない。
//
// 選んだのは「仮の記録を組み立てて History に足す」側（設計書の構成(a)）。
// もう一方（(b) 軸を決める関数を「最終実施日の辞書と出席回数」を受け取る
// 形に割る）は heavyLift・focusCyclePosition・variationLift・stalest・
// recentlyPerformed の5関数の型を変え、Plan からの呼び出しも道連れにする。
// (a) はこのファイル1つで完結し、既存の関数も Plan も1行も変えない。
// このPRは「動かない」PR（設計書のPR分け）なので、触る面積が小さい
// ほうを選ぶ。
//
// history は前日まで（呼び出し側が切る。Plan と同じ規約）。
func (p SessionPlanner) ProjectHorizon(
	history setlog.History, prog *program.Program, pool []*exercise.Exercise, date training.Date,
) ([]ProjectedSession, error) {
	if p.IsZero() {
		return nil, errors.New("セッション生成器が未設定である")
	}
	if prog == nil {
		return nil, errors.New("プログラムが指定されていない")
	}
	if date.IsZero() {
		return nil, errors.New("対象日が指定されていない")
	}

	usable := usablePool(pool, prog)
	declared := declaredExercises(usable, prog)

	// 1種目あたりのセット数は利用者の設定（SessionVolume）で、役割によらず
	// 共通（D-126 の理由がそのまま保たれる。Forecast と同じ値を使う）。
	sets := prog.SessionVolume().Sets()

	dates := horizonDates(date, prog.Frequency().PerWeek())
	logs := history.Logs()

	out := make([]ProjectedSession, 0, len(dates))
	for k, d := range dates {
		h := setlog.NewHistory(logs)

		today, hasSplit := prog.SplitOn(h.SessionCount())
		heavy, axisRole := axis(h, prog, usable, declared, today, hasSplit)
		variation := variationLift(h, prog, usable, heavy, d, today, hasSplit)

		session := ProjectedSession{date: d, split: today, hasSplit: hasSplit}
		if heavy != nil {
			session.axis = heavy
			session.axisRole = axisRole
			session.axisSets = p.prescriptionFor(axisRole, sets).setCount()
			session.stimulus = session.stimulus.Plus(heavy.Stimulus(), session.axisSets)
			logs = append(logs, projectedLog(k, "axis", d, heavy.ID()))
		}
		if variation != nil {
			session.variation = variation
			session.variationSets = p.prescriptionFor(variationRole, sets).setCount()
			session.stimulus = session.stimulus.Plus(variation.Stimulus(), session.variationSets)
			logs = append(logs, projectedLog(k, "variation", d, variation.ID()))
		}
		out = append(out, session)
	}
	return out, nil
}

// toHorizonSessions は ProjectHorizon の出力を、割り振り器
// （accessory_allocator.go）の入力へ変換する。
//
// 空き枠は「1回の種目数 − 軸があれば1 − バリエーションがあれば1」。
// Forecast はこの式をそのまま全回（回0を含む）で使う。以前
// （selectLineup）は回0だけ、その場で確定した lineup から求めた
// slots で上書きしていたが、Forecast が軸・バリエーションの決定を
// ProjectHorizon 側の1本に統一したので、上書きは無くなった。
func toHorizonSessions(sessions []ProjectedSession, exercisesPerSession int) []HorizonSession {
	out := make([]HorizonSession, len(sessions))
	for i, s := range sessions {
		axis, _, hasAxis := s.Axis()
		variation, _, hasVariation := s.Variation()
		slots := exercisesPerSession
		if hasAxis {
			slots--
		}
		if hasVariation {
			slots--
		}
		if slots < 0 {
			slots = 0
		}
		split, hasSplit := s.Split()
		out[i] = HorizonSession{
			Date: s.Date(), Split: split, HasSplit: hasSplit,
			Axis: axis, Variation: variation,
			Stimulus: s.Stimulus(), Slots: slots,
		}
	}
	return out
}

// horizonDates は今日を含めて f 回ぶんの日付を返す。
//
// 頻度から 7/f 日ごとの等間隔を仮定する（設計書「今日から 7/f 日ごとの
// 等間隔を整数に丸める」）。k 回目の今日からの経過日数は round(k × 7 / f)。
// 丸めは0.5をゼロから遠いほうへ（Go の math.Round と同じ、例：3.5 → 4）。
// k=0 は必ず round(0)=0、つまり今日。
//
// 割り切れる頻度（1・7）以外は間隔が1日前後する。作り直すのは毎回なので
// （設計書「毎回作り直すので、外れても誤差は1回ぶんで止まる」）、これで足りる。
// 頻度1〜7のいずれでも f 個の日付が重複しないことは
// TestHorizonDates_SpacingByFrequency が守る。
func horizonDates(today training.Date, f int) []training.Date {
	out := make([]training.Date, 0, f)
	for k := 0; k < f; k++ {
		offset := int(math.Round(float64(k) * 7 / float64(f)))
		out = append(out, today.AddDays(offset))
	}
	return out
}

// projectedLog は仮の記録を1件組み立てる。
//
// 重量・レップ・RIRは0・1・0の固定値で、値そのものに意味は無い。軸・
// バリエーションを決める関数（heavyLift・focusCyclePosition・
// variationLift・stalest・recentlyPerformed）が読むのは実施日と種目IDだけで、
// この3つの値を読む経路が無い。ID は (回, レーン, 種目) で一意にし、
// 同じ種目を軸とバリエーションの双方に同日書き込む事態（起こらない。
// variationLift は軸の系統を除く）でも衝突しない。
func projectedLog(k int, lane string, date training.Date, id exercise.ExerciseID) *setlog.SetLog {
	l, err := setlog.NewSetLog(setlog.SetLogParams{
		ID:          fmt.Sprintf("horizon-%d-%s-%s", k, lane, id),
		PerformedOn: date,
		ExerciseID:  string(id),
		WeightKg:    0,
		Reps:        1,
		RIR:         0,
	})
	if err != nil {
		// 到達しない。種目IDは *exercise.Exercise から来ていて既に検証済み、
		// 日付は呼び出し元（ProjectHorizon の date ガード＋horizonDates）が
		// 保証し、他の値は固定の有効値。NewHistory は nil を読み飛ばすので、
		// 万一到達しても panic にはならない。
		return nil
	}
	return l
}
