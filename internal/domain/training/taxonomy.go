package training

import "sort"

// MuscleRegion は筋区分。
//
// 粗い「部位」ではなくこの粒度で週ボリュームを管理する。「ベンチが埋めない
// 大胸筋上部を補助で埋める」という判断を表現するには、胸を上部・中部・下部に
// 割る必要があるため。胸だけ細かくて他が雑という歪みを避けるべく、全身を
// 同じ解像度で持つ。
type MuscleRegion string

const (
	ChestUpper MuscleRegion = "CHEST_UPPER"
	ChestMid   MuscleRegion = "CHEST_MID"
	ChestLower MuscleRegion = "CHEST_LOWER"

	Lat       MuscleRegion = "LAT"
	TrapMid   MuscleRegion = "TRAP_MID"
	TrapUpper MuscleRegion = "TRAP_UPPER"
	Erector   MuscleRegion = "ERECTOR"

	FrontDelt MuscleRegion = "FRONT_DELT"
	SideDelt  MuscleRegion = "SIDE_DELT"
	RearDelt  MuscleRegion = "REAR_DELT"

	TricepsLong    MuscleRegion = "TRICEPS_LONG"
	TricepsLateral MuscleRegion = "TRICEPS_LATERAL"

	Biceps  MuscleRegion = "BICEPS"
	Forearm MuscleRegion = "FOREARM"

	Quad      MuscleRegion = "QUAD"
	Hamstring MuscleRegion = "HAMSTRING"
	Glute     MuscleRegion = "GLUTE"
	Adductor  MuscleRegion = "ADDUCTOR"
	Calf      MuscleRegion = "CALF"

	Abs     MuscleRegion = "ABS"
	Oblique MuscleRegion = "OBLIQUE"
)

// allMuscleRegions は全筋区分。識別子の昇順で保持する。
//
// 順序を固定するのは、週ボリュームの走査順が補助種目の選択結果に影響しうるため。
// 定義順のまま持つと、定数を並べ替えただけで生成されるセッションが変わる。
var allMuscleRegions = sortedRegions(
	ChestUpper, ChestMid, ChestLower,
	Lat, TrapMid, TrapUpper, Erector,
	FrontDelt, SideDelt, RearDelt,
	TricepsLong, TricepsLateral,
	Biceps, Forearm,
	Quad, Hamstring, Glute, Adductor, Calf,
	Abs, Oblique,
)

func sortedRegions(regions ...MuscleRegion) []MuscleRegion {
	sort.Slice(regions, func(i, j int) bool { return regions[i] < regions[j] })
	return regions
}

// validMuscleRegions は Valid の定数時間判定用。
var validMuscleRegions = func() map[MuscleRegion]bool {
	m := make(map[MuscleRegion]bool, len(allMuscleRegions))
	for _, r := range allMuscleRegions {
		m[r] = true
	}
	return m
}()

// AllMuscleRegions は全筋区分を識別子の昇順で返す。呼び出し側が書き換えても影響しない。
func AllMuscleRegions() []MuscleRegion {
	out := make([]MuscleRegion, len(allMuscleRegions))
	copy(out, allMuscleRegions)
	return out
}

func (r MuscleRegion) Valid() bool { return validMuscleRegions[r] }

// ExerciseKind は種目の役割。
//
//	KindMain      … 通常フォームのメイン種目
//	KindVariation … メインの派生（ラーセン、テンポなど）。対メイン係数を持つ
//	KindAccessory … 補助種目。残差を埋めるために選ばれる
type ExerciseKind string

const (
	KindMain      ExerciseKind = "MAIN"
	KindVariation ExerciseKind = "VARIATION"
	KindAccessory ExerciseKind = "ACCESSORY"
)

func (k ExerciseKind) Valid() bool {
	switch k {
	case KindMain, KindVariation, KindAccessory:
		return true
	default:
		return false
	}
}

// AllExerciseKinds は全種別。網羅性を検査するテストのために公開する。
func AllExerciseKinds() []ExerciseKind {
	return []ExerciseKind{KindAccessory, KindMain, KindVariation}
}

// MainLift は週内スロットで強度帯を振り分ける対象。
type MainLift string

const (
	LiftSquat    MainLift = "SQUAT"
	LiftBench    MainLift = "BENCH"
	LiftDeadlift MainLift = "DEADLIFT"
)

func (l MainLift) Valid() bool {
	switch l {
	case LiftSquat, LiftBench, LiftDeadlift:
		return true
	default:
		return false
	}
}

// AllMainLifts は全メインリフト。網羅性を検査するテストのために公開する。
func AllMainLifts() []MainLift {
	return []MainLift{LiftBench, LiftDeadlift, LiftSquat}
}
