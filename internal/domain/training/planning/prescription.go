package planning

import (
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// Intent はその回で何を狙うか。
//
//	IntentLight     … 同じ種目を軽く回して回復と技術に充てる日
//	IntentStandard  … 通常フォームでボリュームを積む日
//	IntentHeavy     … 高強度で神経系に効かせる日
type Intent string

const (
	IntentLight    Intent = "LIGHT"
	IntentStandard Intent = "STANDARD"
	IntentHeavy    Intent = "HEAVY"
)

var (
	allIntents   = sortedValues(IntentLight, IntentStandard, IntentHeavy)
	validIntents = lookup(allIntents)
)

// AllIntents は全役割を文字列値の昇順で返す。
func AllIntents() []Intent { return clone(allIntents) }

func (r Intent) Valid() bool { return validIntents[r] }

// Prescription は「その回をどう当たるか」の処方。不変。
//
// 種目を含まない。何をやるかは軸の選定が決め、どうやるかをここが決める。
// だからどの種目にも同じ処方を当てられる。
//
// TargetRIR は止め時の指示であり、レップ数は指示しない。
// レップ数はその日の状態が決める。
type Prescription struct {
	intent    Intent
	intensity training.IntensityPct
	sets      training.SetCount
	targetRIR training.RIR
}

func (s Prescription) Intent() Intent                   { return s.intent }
func (s Prescription) Intensity() training.IntensityPct { return s.intensity }
func (s Prescription) Sets() training.SetCount          { return s.sets }
func (s Prescription) TargetRIR() training.RIR          { return s.targetRIR }

func (s Prescription) IsZero() bool { return s == Prescription{} }

// newPrescription はカタログ定義用。
//
// 渡すのはこのファイル内の定数だけなので、失敗はプログラムの誤りであり
// 実行時の入力では起こりえない。起動時に必ず気づけるよう panic する。
// 他のファイルから呼ぶことは TestDomain_PanickingFunctionsStayWhereTheyBelong が禁止する。
//
// 役割の妥当性は検査しない。カタログの各要素が正当な役割を持つことは
// TestPrescriptionCatalog_ValuesAreRealistic が確かめており、到達しない分岐は残さない。
func newPrescription(intent Intent, intensity float64, sets, rir int) Prescription {
	i, err := training.NewIntensityPct(intensity)
	if err != nil {
		panic(fmt.Sprintf("処方の定義が不正: %v", err))
	}
	s, err := training.NewSetCount(sets)
	if err != nil {
		panic(fmt.Sprintf("処方の定義が不正: %v", err))
	}
	r, err := training.NewRIR(rir)
	if err != nil {
		panic(fmt.Sprintf("処方の定義が不正: %v", err))
	}
	return Prescription{intent: intent, intensity: i, sets: s, targetRIR: r}
}

// 週の頻度ごとの処方構成。
//
// 現行のベンチ（1RM 105kg 想定で 80 / 85 / 90〜95kg）が概ね
// 76% / 81% / 88% に対応する。RIR は調整ダイヤルではなくガードレールなので
// 2で固定し、高強度の回だけ1にする。動かすのは強度帯とバリエーションの有無。
//
// 並び順は「重要な役割ほど先」にしている。強度の昇順ではない。
//
// これは、設定した頻度より実際に通う回数が少ないときの破綻を防ぐため。
// 処方はその種目の週何本目かで決まるので、週4回の設定で週2回しか通わないと
// 先頭2つしか使われない。軽い日ばかりが当たると、通常フォームの
// 高い強度がいつまでも記録されず、推定1RMが実力より低いまま固定される。
//
// 標準の処方を必ず先頭に置くことで、週に一度でも通えば通常の強度で
// 実施することが保証される。
var prescriptionsByFrequency = map[int][]Prescription{
	1: {
		newPrescription(IntentStandard, 0.81, 4, 2),
	},
	2: {
		newPrescription(IntentStandard, 0.81, 4, 2),
		newPrescription(IntentHeavy, 0.88, 3, 1),
	},
	3: {
		newPrescription(IntentStandard, 0.81, 4, 2),
		newPrescription(IntentHeavy, 0.88, 3, 1),
		newPrescription(IntentLight, 0.76, 4, 2),
	},
	4: {
		newPrescription(IntentStandard, 0.81, 4, 2),
		newPrescription(IntentHeavy, 0.88, 3, 1),
		newPrescription(IntentLight, 0.76, 4, 2),
		newPrescription(IntentLight, 0.78, 4, 2),
	},
}

// sortedValues / lookup / clone は taxonomy 側と同じ小さなヘルパー。
//
// 共有せずに複製しているのは、公開すると「ドメインの語彙」に
// 意味を持たない汎用関数が並ぶため。10行の重複のほうが安い。
func sortedValues[T ~string](values ...T) []T {
	out := make([]T, len(values))
	copy(out, values)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func clone[T ~string](values []T) []T {
	out := make([]T, len(values))
	copy(out, values)
	return out
}

func lookup[T ~string](values []T) map[T]bool {
	out := make(map[T]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}
