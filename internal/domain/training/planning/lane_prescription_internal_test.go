package planning

import (
	"math"
	"testing"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
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
		{"重点種目の派生の番", focusVariationRole},
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
			// セット数は強度と RIR の組に効かないので、既定の3で引く。
			row := planner.prescriptionFor(lane.role, 3, program.DefaultRepTargets())
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

// 軸の強度は Epley の逆算を小数2桁に丸めた値。
//
// 丸めた値は今の表と一致する（3 → 0.88、6 → 0.81）。一致しないと、
// レップ数を設定していない全員の重量が動く。1〜15 のどれでも、丸めた
// 強度から Epley で逆算したレップ数が元に戻る（TestSessionPlanner_TargetReps
// の式の一致がそのまま通る）。
//
// 非公開関数の契約なので内部テストで書く。公開の入口（Plan）からは、
// 丸めの有無は重量が刻みに丸まった後の差としてしか見えず、刻みをまたがない
// 限り観測できない。
func TestAxisPrescription_FollowsEpley(t *testing.T) {
	for _, c := range []struct {
		reps int
		want float64
	}{
		{1, 0.94}, {3, 0.88}, {6, 0.81}, {8, 0.77}, {10, 0.73}, {12, 0.70}, {15, 0.65},
	} {
		got := axisPrescription(c.reps, 3)
		if got.intensityPct != c.want {
			t.Errorf("%dレップの強度が %v。%v のはず", c.reps, got.intensityPct, c.want)
		}
	}

	for reps := 1; reps <= 15; reps++ {
		got := axisPrescription(reps, 3)
		back := int(math.Round(30*(1/got.intensityPct-1))) - got.targetRIR
		if back != reps || got.targetReps != reps || got.targetRIR != 1 {
			t.Errorf("%dレップ: 強度 %v・RIR%d・目標 %d、逆算 %d", reps,
				got.intensityPct, got.targetRIR, got.targetReps, back)
		}
	}
}
