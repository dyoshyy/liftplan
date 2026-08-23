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

// allMuscleRegions は全筋区分。文字列値の昇順で保持する。
//
// 順序を固定するのは、この一覧を走査するコードが常に同じ結果を返すようにするため。
// 定義順のまま持つと、定数を並べ替えただけで走査順が変わる。
// なお補助種目の貪欲選択における同点処理は、選択側が自前でソートして担保している。
// この一覧の順序はそこには届かない。
var allMuscleRegions = sortedValues(
	ChestUpper, ChestMid, ChestLower,
	Lat, TrapMid, TrapUpper, Erector,
	FrontDelt, SideDelt, RearDelt,
	TricepsLong, TricepsLateral,
	Biceps, Forearm,
	Quad, Hamstring, Glute, Adductor, Calf,
	Abs, Oblique,
)

// validMuscleRegions は allMuscleRegions から導出する。
// 手書きの switch と併存させると、一覧に足し忘れても Valid だけ通る非対称が生まれる。
var validMuscleRegions = lookup(allMuscleRegions)

// AllMuscleRegions は全筋区分を文字列値の昇順で返す。呼び出し側が書き換えても影響しない。
func AllMuscleRegions() []MuscleRegion { return clone(allMuscleRegions) }

func (r MuscleRegion) Valid() bool { return validMuscleRegions[r] }

// ExerciseKind は種目の役割。
//
//	KindMain      … その日の軸になる種目
//	KindAccessory … 補助種目。残差を埋めるために選ばれる
type ExerciseKind string

const (
	KindMain      ExerciseKind = "MAIN"
	KindAccessory ExerciseKind = "ACCESSORY"
)

var (
	allExerciseKinds   = sortedValues(KindMain, KindAccessory)
	validExerciseKinds = lookup(allExerciseKinds)
)

// AllExerciseKinds は全種別を文字列値の昇順で返す。
func AllExerciseKinds() []ExerciseKind { return clone(allExerciseKinds) }

func (k ExerciseKind) Valid() bool { return validExerciseKinds[k] }

// sortedValues は可変長で受けた値を昇順に並べた新しいスライスを返す。
//
// 引数のスライスをその場でソートすると、`sortedValues(s...)` と書かれたときに
// 呼び出し側の s を破壊する。必ずコピーしてから並べ替える。
func sortedValues[T ~string](values ...T) []T {
	out := clone(values)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func clone[T ~string](values []T) []T {
	out := make([]T, len(values))
	copy(out, values)
	return out
}

func lookup[T ~string](values []T) map[T]bool {
	m := make(map[T]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}
