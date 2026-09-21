package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

const (
	// 軸レーンの処方。3レーンで最も重い。
	//
	// 割合は Epley の逆算に合わせる（1 / (1 + (レップ + RIR) / 30)）。
	// 0.88 は3レップ RIR1、0.81 は6レップ RIR1。外部の強度表から刻みだけを
	// 借りると、推定（Epley）と処方が別の式で動く。
	//
	// 軸の強度を1つの定数にしていたのは、散らす相手がいなかったため。
	// 宣言種目は「最後にやったのが最も古いもの」で回るので、宣言が3つ
	// あれば各種目は週1回しか軸に来ない（D-117）。分割が入ると前提が
	// 変わる。上下2分割で宣言がBIG3なら、上半身の日に立てる宣言はベンチ
	// だけになり、同じ種目を同じ強度で週2回やることになる。
	heavyIntensityPct       = 0.88
	focusVolumeIntensityPct = 0.81
	heavySets               = 3
	heavyTargetRIR          = 1

	// focusCycleLength は重点種目の番に回す一巡の長さ。
	// 3レップ相当 → 6レップ相当 → 派生 の3つ。
	focusCycleLength = 3

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
	intensityPct float64,
	rirBump int,
) PlannedSet {
	return p.prescribe(req, historyBefore, target,
		intensityPct, heavySets, heavyTargetRIR, rirBump)
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

	// 当日の記録は使わない（D-116）。含めると、1セット目を記録した瞬間に
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
