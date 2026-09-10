package planning

import (
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
)

// PrescriptionCatalog は週の頻度に対する強度配分を持つドメインサービス。無状態。
type PrescriptionCatalog struct{}

func NewPrescriptionCatalog() PrescriptionCatalog { return PrescriptionCatalog{} }

// For は週の頻度に応じた処方の並びを返す。
//
// 返るのは週1周期ぶんで、要素数は頻度と一致する。
func (c PrescriptionCatalog) For(f program.Frequency) []Prescription {
	list, ok := prescriptionsByFrequency[f.PerWeek()]
	if !ok {
		return nil
	}
	out := make([]Prescription, len(list))
	copy(out, list)
	return out
}

// Select は index 本目の処方を返す。
//
// 要素数で剰余を取るため、頻度の設定を超えて回っても破綻しない。
// 週3回の設定で4回目を実施したら、1本目と同じ構成に戻る。
//
// 逆に設定より少ない回数しか通わなかった場合は先頭だけが使われる。
// 先頭を標準の処方にしてあるのは、そのときでも通常の強度で実施され、
// 推定1RMが実力より低いまま固定されないようにするため。
func (c PrescriptionCatalog) Select(f program.Frequency, index int) (Prescription, bool) {
	list := c.For(f)
	if len(list) == 0 {
		return Prescription{}, false
	}
	if index < 0 {
		index = 0
	}
	return list[index%len(list)], true
}
