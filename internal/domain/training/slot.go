package training

import "fmt"

const maxFrequencyPerWeek = 7

// Frequency は週あたりのトレーニング回数。
type Frequency struct {
	perWeek int
}

func NewFrequency(perWeek int) (Frequency, error) {
	if perWeek < 1 || perWeek > maxFrequencyPerWeek {
		return Frequency{}, fmt.Errorf("週の頻度は1〜%d回である必要がある: %d", maxFrequencyPerWeek, perWeek)
	}
	return Frequency{perWeek: perWeek}, nil
}

func (f Frequency) PerWeek() int { return f.perWeek }

func (f Frequency) IsZero() bool { return f == Frequency{} }

// SlotRole は週内スロットの役割。
//
//	RoleVariation … バリエーション種目で技術と弱点を突く日
//	RoleStandard  … 通常フォームでボリュームを積む日
//	RoleHeavy     … 高強度で神経系に効かせる日
type SlotRole string

const (
	RoleVariation SlotRole = "VARIATION"
	RoleStandard  SlotRole = "STANDARD"
	RoleHeavy     SlotRole = "HEAVY"
)

var (
	allSlotRoles   = sortedValues(RoleVariation, RoleStandard, RoleHeavy)
	validSlotRoles = lookup(allSlotRoles)
)

// AllSlotRoles は全役割を文字列値の昇順で返す。
func AllSlotRoles() []SlotRole { return clone(allSlotRoles) }

func (r SlotRole) Valid() bool { return validSlotRoles[r] }

// SlotTemplate は1スロットの設計。不変。
//
// TargetRIR は止め時の指示であり、レップ数は指示しない。
// レップ数はその日の状態が決める。
type SlotTemplate struct {
	role      SlotRole
	intensity IntensityPct
	sets      SetCount
	targetRIR RIR
}

func (s SlotTemplate) Role() SlotRole          { return s.role }
func (s SlotTemplate) Intensity() IntensityPct { return s.intensity }
func (s SlotTemplate) Sets() SetCount          { return s.sets }
func (s SlotTemplate) TargetRIR() RIR          { return s.targetRIR }

func (s SlotTemplate) IsZero() bool { return s == SlotTemplate{} }

// newSlotTemplate はカタログ定義用。
//
// 渡すのはこのファイル内の定数だけなので、失敗はプログラムの誤りであり
// 実行時の入力では起こりえない。起動時に必ず気づけるよう panic する。
func newSlotTemplate(role SlotRole, intensity float64, sets, rir int) SlotTemplate {
	if !role.Valid() {
		panic(fmt.Sprintf("スロット定義の役割が不正: %q", role))
	}
	i, err := NewIntensityPct(intensity)
	if err != nil {
		panic(fmt.Sprintf("スロット定義が不正: %v", err))
	}
	s, err := NewSetCount(sets)
	if err != nil {
		panic(fmt.Sprintf("スロット定義が不正: %v", err))
	}
	r, err := NewRIR(rir)
	if err != nil {
		panic(fmt.Sprintf("スロット定義が不正: %v", err))
	}
	return SlotTemplate{role: role, intensity: i, sets: s, targetRIR: r}
}

// 週の頻度ごとのスロット構成。
//
// 現行のベンチ（1RM 105kg 想定で 80 / 85 / 90〜95kg）が概ね
// 76% / 81% / 88% に対応する。RIR は調整ダイヤルではなくガードレールなので
// 2で固定し、高強度スロットのみ1にする。動かすのは強度帯とバリエーションの有無。
var slotsByFrequency = map[int][]SlotTemplate{
	1: {
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
	},
	2: {
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
		newSlotTemplate(RoleHeavy, 0.88, 3, 1),
	},
	3: {
		newSlotTemplate(RoleVariation, 0.76, 4, 2),
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
		newSlotTemplate(RoleHeavy, 0.88, 3, 1),
	},
	4: {
		newSlotTemplate(RoleVariation, 0.76, 4, 2),
		newSlotTemplate(RoleVariation, 0.78, 4, 2),
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
		newSlotTemplate(RoleHeavy, 0.88, 3, 1),
	},
}

// SlotCatalog は週の頻度に対する強度配分を持つドメインサービス。無状態。
type SlotCatalog struct{}

func NewSlotCatalog() SlotCatalog { return SlotCatalog{} }

// For は週の頻度に応じたスロット構成を返す。
//
// 週5回以上は4スロット構成を巡回させる。週の後半で同じ強度帯が二度来るが、
// 強度配分そのものを崩すよりは良い。
func (c SlotCatalog) For(f Frequency) []SlotTemplate {
	n := f.PerWeek()
	if n > 4 {
		n = 4
	}
	slots, ok := slotsByFrequency[n]
	if !ok {
		return nil
	}
	out := make([]SlotTemplate, len(slots))
	copy(out, slots)
	return out
}

// Select は週内 sessionIndex 本目のスロットを返す。
//
// 要素数で剰余を取るため、頻度の設定を超えて回っても破綻しない。
// 週3回の設定で4回目を実施したら、1本目と同じ構成に戻る。
func (c SlotCatalog) Select(f Frequency, sessionIndex int) (SlotTemplate, bool) {
	slots := c.For(f)
	if len(slots) == 0 {
		return SlotTemplate{}, false
	}
	if sessionIndex < 0 {
		sessionIndex = 0
	}
	return slots[sessionIndex%len(slots)], true
}
