package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// SlotCatalog は週の頻度に対する強度配分を持つドメインサービス。無状態。
type SlotCatalog struct{}

func NewSlotCatalog() SlotCatalog { return SlotCatalog{} }

// For は週の頻度に応じたスロット構成を返す。
//
// 返るのは週1周期ぶんのスロットで、要素数は頻度と一致する。
func (c SlotCatalog) For(f program.Frequency) []SlotTemplate {
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
func (c SlotCatalog) Select(f program.Frequency, sessionIndex int) (SlotTemplate, bool) {
	slots := c.For(f)
	if len(slots) == 0 {
		return SlotTemplate{}, false
	}
	if sessionIndex < 0 {
		sessionIndex = 0
	}
	return slots[sessionIndex%len(slots)], true
}
