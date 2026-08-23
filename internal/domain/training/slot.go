package training

import "fmt"

// maxFrequencyPerWeek は週の頻度の上限。
//
// 4に抑えているのは、毎セッションで BIG3 すべてにスロットを割り当てる設計だから。
// 週5回にすると1種目あたり19セット/週、週7回なら27セット/週になり、
// デッドリフトを週27セットこなす前提のプログラムになってしまう。
// それより多く通う場合は、頻度を上げるのではなく補助種目の日を自分で足す。
const maxFrequencyPerWeek = 4

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
//	RoleLight     … 同じ種目を軽く回して回復と技術に充てる日
//	RoleStandard  … 通常フォームでボリュームを積む日
//	RoleHeavy     … 高強度で神経系に効かせる日
type SlotRole string

const (
	RoleLight    SlotRole = "LIGHT"
	RoleStandard SlotRole = "STANDARD"
	RoleHeavy    SlotRole = "HEAVY"
)

var (
	allSlotRoles   = sortedValues(RoleLight, RoleStandard, RoleHeavy)
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
// 他のファイルから呼ぶことは TestDomain_PanickingFunctionsStayWhereTheyBelong が禁止する。
//
// 役割の妥当性は検査しない。カタログの各要素が正当な役割を持つことは
// TestSlotCatalog_ValuesAreRealistic が確かめており、到達しない分岐は残さない。
func newSlotTemplate(role SlotRole, intensity float64, sets, rir int) SlotTemplate {
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
//
// 並び順は「重要な役割ほど先」にしている。強度の昇順ではない。
//
// これは、設定した頻度より実際に通う回数が少ないときの破綻を防ぐため。
// スロットはその週の何本目かで決まるので、週4回の設定で週2回しか通わないと
// 先頭2つしか使われない。軽い日ばかりが当たると、通常フォームの
// 高い強度がいつまでも記録されず、推定1RMが実力より低いまま固定される。
//
// 標準スロットを必ず先頭に置くことで、週に一度でも通えば通常の強度で
// 実施することが保証される。
var slotsByFrequency = map[int][]SlotTemplate{
	1: {
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
	},
	2: {
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
		newSlotTemplate(RoleHeavy, 0.88, 3, 1),
	},
	3: {
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
		newSlotTemplate(RoleHeavy, 0.88, 3, 1),
		newSlotTemplate(RoleLight, 0.76, 4, 2),
	},
	4: {
		newSlotTemplate(RoleStandard, 0.81, 4, 2),
		newSlotTemplate(RoleHeavy, 0.88, 3, 1),
		newSlotTemplate(RoleLight, 0.76, 4, 2),
		newSlotTemplate(RoleLight, 0.78, 4, 2),
	},
}

// SlotCatalog は週の頻度に対する強度配分を持つドメインサービス。無状態。
type SlotCatalog struct{}

func NewSlotCatalog() SlotCatalog { return SlotCatalog{} }

// For は週の頻度に応じたスロット構成を返す。
//
// 返るのは週1周期ぶんのスロットで、要素数は頻度と一致する。
func (c SlotCatalog) For(f Frequency) []SlotTemplate {
	slots, ok := slotsByFrequency[f.PerWeek()]
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
//
// 逆に設定より少ない回数しか通わなかった場合は先頭のスロットだけが使われる。
// 先頭を標準スロットにしてあるのは、そのときでもメインリフト本体が
// 実施されるようにするため。
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
