package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// laneRole は種目が今日のセッションで担う役割。
//
// 種目を選ぶ側（axis・variationLift・AccessorySelector）はこの役割までを
// 決め、強度・セット数・RIR は決めない。役割から処方の定数を引くのは
// prescriptionFor の表だけで、強度の値はあの表にしか無い。
type laneRole int

const (
	// heavyRole は軸。3レップ相当で、3レーンで最も重い。
	heavyRole laneRole = iota
	// focusVolumeRole は重点種目の一巡の2番目。同じ軸を6レップ相当で出す。
	focusVolumeRole
	// variationRole はバリエーションレーン。軸より軽く、補助より重い。
	variationRole
	// accessoryRole は補助。RIR2 で10レップ前後を狙う位置。
	accessoryRole
)

const (
	// focusCycleLength は重点種目の番に回す一巡の長さ。
	// 3レップ相当 → 6レップ相当 → 派生 の3つ。
	focusCycleLength = 3

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

// lanePrescription は役割ごとの処方の定数。
//
// 値オブジェクトへ通すのは prescribeSet の中で実行時に行う。ここで通さないのは、
// DefaultSessionPlanner がエラーを返せず、panic も使えないため
// （TestDomain_PanickingFunctionsStayWhereTheyBelong）。
type lanePrescription struct {
	intensityPct float64
	sets         int
	targetRIR    int
}

// prescriptionFor は役割から強度・セット数・RIR を引く。3レーン分の定数は
// ここにしか無い。
//
// 割合は Epley の逆算に合わせる（1 / (1 + (レップ + RIR) / 30)）。
// 0.88 は3レップ RIR1、0.81 は6レップ RIR1。外部の強度表から刻みだけを
// 借りると、推定（Epley）と処方が別の式で動く。
//
// 軸の強度を1つの定数にしていたのは、散らす相手がいなかったため。
// 宣言種目は「最後にやったのが最も古いもの」で回るので、宣言が3つ
// あれば各種目は週1回しか軸に来ない（D-117）。分割が入ると前提が
// 変わる。上下2分割で宣言がBIG3なら、上半身の日に立てる宣言はベンチ
// だけになり、同じ種目を同じ強度で週2回やることになる。そこで重点種目の
// 番だけ 0.88 と 0.81 を回す（D-128）。
//
// バリエーションを表から引かず定数にしているのは、派生が週に何回出ようと
// 強度を変える理由が無いため。同じ種目の中で強度を回すのは「同じ種目を
// 週に何回もやる」ことが前提で、派生は別種目として自分の推定1RMを持つ
// （D-113）。種目が違えば重量は自然に違う。
// 表から持ってきた値は 0.81 / 4セットだったが、0.81 は表の中で
// 0.88 や 0.76 と並んで初めて意味を持つ刻みで、単独では半端。
// 軸 0.88 と補助 0.71 の間に置く一つの値としては 0.80 でいい。
// 4セットは軸と合わせて胸の実測が週目標の134%まで出ていたので3に落とす。
//
// 補助のセット数だけ定数ではなく AccessorySelector から来る（D-126）。
// 選択器が残差を消し込むときに使う数と同じでなければ、選んだ本数と
// 出す本数が食い違う。
func (p SessionPlanner) prescriptionFor(role laneRole) lanePrescription {
	switch role {
	case heavyRole:
		return lanePrescription{intensityPct: 0.88, sets: 3, targetRIR: 1}
	case focusVolumeRole:
		return lanePrescription{intensityPct: 0.81, sets: 3, targetRIR: 1}
	case variationRole:
		return lanePrescription{intensityPct: 0.80, sets: 3, targetRIR: 2}
	case accessoryRole:
		return lanePrescription{
			intensityPct: 0.71, sets: p.accessory.SetsPerAccessory().Int(), targetRIR: 2,
		}
	}
	// 到達しない。役割は上の4つしか無い。ゼロ値を返すと prescribeSet が
	// 値オブジェクトの検証で止まり、種目だけの set になる。
	return lanePrescription{}
}

// setCount はセット数を値オブジェクトにする。残差に「今日積む分」を足すときに
// 使う。定数が範囲外ならゼロ値（0セット）で、prescribeSet がセット数を
// 付けずに返すのと同じ量になる。
func (l lanePrescription) setCount() training.SetCount {
	sets, err := training.NewSetCount(l.sets)
	if err != nil {
		return training.SetCount{}
	}
	return sets
}

// prescribe は並びの各種目に、役割の表から引いた定数と推定1RMで重量を付ける。
// 種目の選び方には触れない。
//
// 役割からレーンへの振り分けもここ。heavyRole と focusVolumeRole は同じ
// 軸レーンで、違いは強度だけ。
//
// estimable は前日まで・実効負荷の履歴（Plan が作る）。記録のままの履歴を
// 渡すと自重種目の重量がずれる。
func (p SessionPlanner) prescribe(
	lineup []lineupEntry, estimable setlog.History,
	conditions condition.ConditionLog, date training.Date,
) PlannedSession {
	rirBump := p.analyzer.RIRAdjustment(conditions, date)

	session := PlannedSession{
		date:        date,
		main:        make([]PlannedSet, 0, 1),
		variation:   make([]PlannedSet, 0, 1),
		accessories: make([]PlannedSet, 0, len(lineup)),
	}
	for _, entry := range lineup {
		set := p.prescribeSet(estimable, conditions, date, entry.exercise, entry.role, rirBump)
		switch entry.role {
		case heavyRole, focusVolumeRole:
			session.main = append(session.main, set)
		case variationRole:
			session.variation = append(session.variation, set)
		case accessoryRole:
			session.accessories = append(session.accessories, set)
		}
	}
	return session
}

// prescribeSet は「この種目をこの強度で何セット」を1件ぶん組み立てる。
// レーンごとの違いは役割から引く定数と、軸にだけ効く上乗せ（overload）。
//
// 定数を値オブジェクトへ通すのは実行時で、失敗しても種目だけの set に
// 落とす。重量が付かなければ本人が決める。定数が正しい限り発火しないが、
// panic は使わない（TestDomain_PanickingFunctionsStayWhereTheyBelong）。
func (p SessionPlanner) prescribeSet(
	estimable setlog.History, conditions condition.ConditionLog, date training.Date,
	target *exercise.Exercise, role laneRole, rirBump int,
) PlannedSet {
	set := PlannedSet{exerciseID: target.ID()}
	lane := p.prescriptionFor(role)

	baseRIR, err := training.NewRIR(lane.targetRIR)
	if err != nil {
		return set
	}
	set.targetRIR = baseRIR.Plus(rirBump)

	set.sets, err = training.NewSetCount(lane.sets)
	if err != nil {
		return set
	}

	intensity, err := training.NewIntensityPct(lane.intensityPct)
	if err != nil {
		return set
	}

	if orm, ok := p.estimator.Estimate(estimable, target.ID(), date); ok {
		if w, err := orm.WorkWeight(intensity, target.Increment()); err == nil {
			if role == heavyRole || role == focusVolumeRole {
				w = p.overload(estimable, target, lane, intensity, w)
			}
			// 推定も処方も実効負荷（体重込み）で通し、出口で加重に戻す。
			set.weight, set.hasWeight = AddedWeight(w, target, conditions, date), true
		}
	}
	return set
}

// overloadSessions は「推定が刻み単位で動いていない」と見なすのに要る、
// その種目を実施したセッションの数。
//
// 推定器の追随の速さから決めた。EWMA（α=0.3）は持続した変化の半分を
// 2セッションで（1−0.7² ≒ 0.51）、3分の2を3セッションで（≒ 0.66）拾う。
// 本人が刻み1つぶん強くなっていれば、推定は3セッション以内に処方を刻み
// 1つ動かす（模擬ユーザーで測った。4本目で 100 → 102.5）。3セッション
// 動かないなら、推定の側からは上がらない。短くすると、推定が上げている
// 途中の人に規則が先回りし、どちらで上がったのか区別が付かなくなる。
const overloadSessions = 3

// overload は軸の処方に進行の規則を1つ挟む（D-138）。
//
// 推定1RMのとおりの実力で目標 RIR ちょうどで止める人には、処方と推定が
// 固定点に落ちて重量が二度と動かない（D-014）。「何kgでやるか」はアプリの
// 担当なのに、上に行く判断だけ本人に残る。そこで、直近 overloadSessions
// セッション、この役割の処方が刻み単位で同じで、かつ一度も目標 RIR を
// 割っていなければ、刻みを1つ乗せる。
//
// 判定は重量の記録ではなく「その日の始まりの推定からこの役割で出る処方」で
// 見る。記録のままの重量で比べると、重点種目の一巡で 0.88 と 0.81 が交互に
// 来るので同じ重量が並ばず、永久に発火しない。履歴は役割を持たないので、
// 「同じ役割で出た前回」も引けない。推定から役割の強度で引き直せば、どの
// 役割でやった日でも同じ物差しで比べられ、差は刻み単位で出る（D-124）。
//
// 上乗せは「前回の重量 + 刻み」ではなく base + 刻み。上げた日も推定は
// 刻み単位で平坦なまま（窓が切れない）なので、前回の重量から積むと毎回
// さらに刻みが乗る。base から積めば、推定が支える重量より刻み1つ上にしか
// 出ない。本人が実際に強くなっていれば推定が base を動かし、窓が切れて
// base に戻る（そのときの base が上げた重量以上になる）。
//
// 推定と同じく実効負荷（体重込み）で判定し、処方の出口で加重に戻す。
// 記録の加重で比べると、自重種目は体重が変わっただけで同じ負荷が違う
// 数字になる。
//
// RIR は睡眠不足の上乗せ（rirBump）を足す前の、役割の目標と比べる。その日の
// 目標は履歴に残っていないので、上乗せのあった日に役割の目標ちょうどで
// 止めても割ったことにはしない。
//
// 下げる規則は置かない。上げた重量で目標 RIR を割れば、RIR の条件が
// 窓を抜けるまで外れて base に戻る。推定もその記録で下がる。
//
// 対象は heavyRole と focusVolumeRole（prescribeSet の呼び分け）。派生・補助が
// 対象から外れているのではない。派生も、一巡の「派生の番」に heavyRole で
// 軸へ立てば対象になる（axis_rotation.go の case 2）。線を引いているのは
// 種目ではなく役割。派生が focusVolumeRole（一巡の2番目）に立つ経路は無い
// （case 1 は必ず重点種目そのものを返す）ので、いま効くのは heavyRole
// （0.88）とバリエーション（0.80）の差だけ。丸めると 87.5 と 80 で別の
// 2.5kg グリッドに乗るので、後段の performedAtLeast が区別できる。もし
// 将来 focusVolumeRole（0.81）に派生が立つ経路ができると、0.81 と
// variationRole の 0.80 は丸めた結果が同じグリッド値になりうるため、
// この判定はすり抜ける。そのときはこの前提を見直すこと。
//
// バリエーションの日は窓の証拠に使わない。履歴は役割を持たないので、
// 派生がバリエーションレーン（variationRole・0.80・RIR2）で出た日も
// ForExercise には同じ種目として並ぶ。その日の記録RIR（2）はここで比べる
// 目標RIR（heavyRole なら1）を割っていないため素通りし、「推定が平坦」の
// 判定もその日の始まりの推定を今日の役割の強度で引き直すだけなので、実際に
// 軽い重量でやったことと無関係に成立してしまう。窓の3セッションのうち
// 重い処方で実施したのが1日しかなくても発火する不具合になる。
//
// 本来はバリエーションの日も換算して証拠に使うべきだが、その日の強度
// （0.80・RIR2）を軸の強度へどう換算するかが決まっていない。無理に決めると
// ロジックが複雑になるので、今は重い処方で実施した日だけに絞る
// （必要になるまで作らない）。軽い日は窓を消費せず素通りし、その分さらに
// 古い日まで遡って overloadSessions 件集める。推定できない日（履歴の最初の
// セッション、ブランク明け）に当たれば、重い・軽いを判定するまでもなく
// 打ち切る。窓を無限に遡るわけではないのはこのためで、42日の鮮度判定は
// ここに新しく足すのではなく、推定器の側の既存の判定にそのまま乗る。
func (p SessionPlanner) overload(
	estimable setlog.History, target *exercise.Exercise, lane lanePrescription,
	intensity training.IntensityPct, base training.Weight,
) training.Weight {
	sessions := estimable.ForExercise(target.ID()).Sessions()

	heavy := 0
	for i := len(sessions) - 1; i >= 0 && heavy < overloadSessions; i-- {
		s := sessions[i]

		// その日の始まりに、この役割で出ていたはずの処方。推定できない
		// 日（履歴の最初のセッション、ブランク明け）に当たれば判定しない。
		// バリエーションの日かどうかを見るにも同じ推定が要るので、
		// 重い・軽いを選り分ける前に確かめる。
		orm, ok := p.estimator.Estimate(estimable.Before(s.Date()), target.ID(), s.Date())
		if !ok {
			return base
		}
		w, err := orm.WorkWeight(intensity, target.Increment())
		if err != nil {
			return base
		}

		if !performedAtLeast(s, w) {
			// この役割の処方に届かない重量でやった日（バリエーションの
			// 日など）は、窓に数えず素通りする。
			continue
		}
		heavy++

		for _, l := range s.Logs() {
			if l.RIR().Int() < lane.targetRIR {
				return base
			}
		}
		if w != base {
			return base
		}
	}
	if heavy < overloadSessions {
		return base
	}

	raised, err := training.NewWeight(base.Kg() + target.Increment().Kg())
	if err != nil {
		return base
	}
	return raised
}

// performedAtLeast は、そのセッションにこの役割の処方 w 以上の重量で
// 実施したセットが1つでもあるか。
//
// 「以上」にするのは、上乗せで base より重い日（overload が既に発火した
// 翌日など）を弾かないため。1つでもあれば十分で、全セットを求めない。
// ウォームアップのような軽いセットが混ざっていても、トップセットが
// 届いていれば「この役割でやった日」と見なす。
func performedAtLeast(s setlog.TrainingSession, w training.Weight) bool {
	for _, l := range s.Logs() {
		if l.Weight().Kg() >= w.Kg() {
			return true
		}
	}
	return false
}
