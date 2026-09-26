package planning

import (
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// 処方どおり完遂したとき、推定1RMがどう動くかを固定する。
//
// 3レーンの強度と目標RIRの組は、いずれも Epley の逆算より少ないレップに
// 丸まるため、推定1RMは上がらない。往復だけでは、処方どおりやり続けると
// 重量が停滞する（D-014）。
//
// 停滞は推定器ではなく処方の側で破る（軸の overload、D-138）。この往復は
// 今も収縮するので、定数を変えたときに挙動の変化が見えるよう、数値で
// 固定しておく。
//
// 表を消した（D-126）ので、対象は頻度の行ではなくレーンの定数になった。
// 定数は役割ごとの表（prescriptionFor）にしか無いので、役割で引く。
func TestLanePrescriptions_RoundTripIsCurrentlyContractive(t *testing.T) {
	planner := DefaultSessionPlanner()
	lanes := []struct {
		name string
		role laneRole
	}{
		{"軸", heavyRole},
		{"重点種目の6レップ相当", focusVolumeRole},
		{"バリエーション", variationRole},
		{"補助", accessoryRole},
	}

	baseline, err := training.NewOneRepMax(105)
	if err != nil {
		t.Fatalf("NewOneRepMax: %v", err)
	}
	inc, err := training.NewIncrement(2.5)
	if err != nil {
		t.Fatalf("NewIncrement: %v", err)
	}

	for _, lane := range lanes {
		t.Run(lane.name, func(t *testing.T) {
			row := planner.prescriptionFor(lane.role)
			pct, err := training.NewIntensityPct(row.intensityPct)
			if err != nil {
				t.Fatalf("NewIntensityPct: %v", err)
			}
			rir, err := training.NewRIR(row.targetRIR)
			if err != nil {
				t.Fatalf("NewRIR: %v", err)
			}
			w, err := baseline.WorkWeight(pct, inc)
			if err != nil {
				t.Fatalf("処方に失敗: %v", err)
			}

			// 強度から Epley で逆算した限界レップ数。目標RIRぶん手前で止める。
			limit := 30 * (1/pct.Float() - 1)
			performed := int(limit) - rir.Int()
			if performed < 1 {
				t.Fatalf("実施レップが1未満になる（強度 %v・目標RIR %d）",
					pct.Float(), rir.Int())
			}

			reps, err := training.NewReps(performed)
			if err != nil {
				t.Fatalf("NewReps: %v", err)
			}
			got, ok := training.EstimateOneRepMax(w, reps, rir)
			if !ok {
				t.Fatal("推定できない")
			}

			if got.Kg() > baseline.Kg() {
				t.Errorf("処方どおり完遂で推定1RMが上がった（%v → %v）。"+
					"往復が収縮しなくなったなら、D-138 の上乗せと二重に上がっていないか確かめること",
					baseline.Kg(), got.Kg())
			}
		})
	}
}
