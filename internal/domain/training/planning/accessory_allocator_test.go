package planning_test

import (
	"slices"
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/planning"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// このファイルは設計書
// docs/specs/2026-09-26-accessory-allocation-design.md の「割り振り器
// （テーブルドリブン、全部変異で赤を確認）」の表をそのままテストにする。
//
// 各ケースの変異は go doc コメントに手順を書く。手で sed して
// go test を回し、期待した理由で落ちることを確認したうえで戻した
// （writing-tests スキルの手順）。

// regionOnly は1区分だけに寄与する種目を作る。
func regionOnly(t *testing.T, id string, r training.MuscleRegion, contribution float64) *exercise.Exercise {
	t.Helper()
	return mkAccessory(t, id, map[training.MuscleRegion]float64{r: contribution})
}

// coverage は基準となる刺激（窓の実績＋予測の代わり）を組み立てる。
// StimulusCoverage はゼロ値から Plus で積み上げる（他に組み立て方が無い）。
func coverage(t *testing.T, e *exercise.Exercise, sets int) planning.StimulusCoverage {
	t.Helper()
	return planning.StimulusCoverage{}.Plus(e.Stimulus(), mustSetCount(t, sets))
}

func idsOf(exs []exercise.ExerciseID) []string {
	out := make([]string, 0, len(exs))
	for _, id := range exs {
		out = append(out, string(id))
	}
	return out
}

func contains(ids []exercise.ExerciseID, id exercise.ExerciseID) bool {
	return slices.Contains(ids, id)
}

// noSplitSession は分割なしの回。空き枠は slots。
func noSplitSession(d training.Date, slots int) planning.HorizonSession {
	return planning.HorizonSession{Date: d, Slots: slots}
}

var allocatorDay = training.MustDate(2026, time.August, 16)

// TestAccessoryAllocator_DeficitIsPunishedMoreSteeply は「遅れの大きい区分から埋まる」を守る。
//
// 損失を2乗にしているのは、遅れが大きいほど1セットあたりの減り方が
// 大きくなるようにするため（設計書「遅れの大きい区分ほど先に埋まる」）。
// 区分A（達成率0%）と区分B（達成率80%）を同じ週目標・同じ寄与度・同じ
// セット数で埋められる候補を1つずつ用意すると、2乗ではAが選ばれる差が
// 大きい（ΔL がAの方がはっきり小さい）ので、Aが選ばれる。
//
// 【変異】regionLoss の `d * d` を `math.Abs(d)` に変える（1乗にする）。
// 1乗だと、どちらの区分も1セットあたりの寄与度（Δρ）が同じなら ΔL が
// 完全に一致し、A・Bの候補は同点処理（手順4a・最終実施日）に落ちる。
// Aの候補に「実施済み」、Bの候補に「未実施」を割り当てておくと、同点処理は
// 未実施を優先するBを選ぶため、Aが選ばれなくなり本テストが落ちる。
func TestAccessoryAllocator_DeficitIsPunishedMoreSteeply(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Quad: 10, training.Hamstring: 10,
	})
	// 4週の目標 4*10=40。区分A(Quad)は実績0（達成率0%）、
	// 区分B(Hamstring)は実績32（達成率80%）。
	baseline := coverage(t, regionOnly(t, "hist_ham", training.Hamstring, 1.0), 32)

	candA := regionOnly(t, "candidate_a", training.Quad, 1.0)
	candB := regionOnly(t, "candidate_b", training.Hamstring, 1.0)

	// Aは実施済み（10日前）、Bは未実施。1乗の変異が入って同点になったとき
	// 「未実施優先」でBが勝つようにしておく（変異の効果を確実に検知するため）。
	history := setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "l1", allocatorDay.AddDays(-10), "candidate_a", 40, 8, 2),
	})

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 1)},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{candA, candB},
		Master:           []*exercise.Exercise{candA, candB},
		History:          history,
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !contains(got[0], "candidate_a") {
		t.Errorf("回0の割り当てが %v。達成率0%%の区分Aを埋める candidate_a が選ばれるはず", idsOf(got[0]))
	}
}

// TestAccessoryAllocator_DeltaLossIsNotBiasedByTargetSize は M1 の再現・
// 回帰検査。週目標 T で損失を重み付けしないと、1手あたりの ΔL が
// およそ 1/T に比例して薄まるため、達成率が同じでも週目標の大きい区分が
// 系統的に後回しになる（TestSimulation_EveryAccessoryGetsUsedInSomeSetup
// で hip_thrust が一度も選ばれなかった実測がこれ）。
//
// 区分S（週目標2・小さい）は達成率50%、区分L（週目標40・Sの20倍）は
// 達成率30%にする。相対的な遅れは L の方が深刻なので、L 専用の候補が
// 選ばれるべきである。
//
// T の比を大きく取る（20倍）のは、変異後（重み無し）でも本テストが
// 同点処理に落ちずに逆転して見えるようにするため。比が小さいと、重み
// 無しでも両候補が「ほぼ同じ」の帯に入り、同点処理（未実施優先）が
// たまたま ID 順で正しい方を選んでしまい、変異の効果を検知できない
// （手を動かして確認した）。
//
// 【変異】regionLoss の重み付け（T を掛ける・16Tで割る）を外し、
// (c/(4t)-1)² のような重み無しの形に戻す。週目標が20倍大きい L は
// 1手のΔρが1/20になるぶん ΔL が大きく薄まり、S の方が「遅れが浅いのに」
// 選ばれてしまう（達成率の大小と選択が逆転する）。
func TestAccessoryAllocator_DeltaLossIsNotBiasedByTargetSize(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Calf: 2, training.Erector: 40,
	})
	// 4週目標: Calf=8、Erector=160。
	// Calf 4/8=50%、Erector 48/160=30%（Lの方が相対的に大きく遅れている）。
	baseline := planning.StimulusCoverage{}.
		Plus(regionOnly(t, "hist_s", training.Calf, 1.0).Stimulus(), mustSetCount(t, 4)).
		Plus(regionOnly(t, "hist_l", training.Erector, 1.0).Stimulus(), mustSetCount(t, 48))

	small := regionOnly(t, "small_target", training.Calf, 1.0)
	large := regionOnly(t, "large_target", training.Erector, 1.0)

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 1)},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{small, large},
		Master:           []*exercise.Exercise{small, large},
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !contains(got[0], "large_target") {
		t.Errorf("回0の割り当てが %v。達成率30%%で大きく遅れている large_target が選ばれるはず", idsOf(got[0]))
	}
	if contains(got[0], "small_target") {
		t.Errorf("回0の割り当てが %v。達成率50%%の small_target が先に選ばれている（週目標の小ささが有利に働いている）", idsOf(got[0]))
	}
}

// TestAccessoryAllocator_CubedLossFavorsTheMostBehindRegion は指数3の
// 存在理由そのものを固定する。T による重み付け（M1）だけでは、「複数区分に
// 中程度効く種目」が「1区分に大きく効く種目」に、後者の区分の方がずっと
// 遅れているのに勝ってしまうことがある。
//
// side_raise 相当（SideDelt のみ、達成率0%）と barbell_row 相当（TrapMid・
// RearDelt の2区分、それぞれ達成率30%）を、週目標10で揃えて比べる。
// 相対的な遅れは side_raise 側がはるかに深刻（0% 対 30%）なので、
// side_raise 相当が選ばれるべきである。
//
// 手計算（週目標T=10、寄与1.0・3セット）：
//
//	指数2： 孤立 ΔL=-1.44375  複合 ΔL=-1.9875（2区分合計） → 複合が勝つ（誤り）
//	指数3： 孤立 ΔL=-2.08547  複合 ΔL=-1.97719（2区分合計） → 孤立が勝つ（正しい）
//
// side_raise 相当を未実施、barbell_row 相当を実施済みにしておくのは、
// 指数3でも近い値になる（同点処理の帯に両方入りうる）ため、同点処理
// （手順4a・未実施優先）でも side_raise 相当が勝つよう固定するため
// （margin だけに頼らない）。
//
// 【変異】regionLoss の指数を3から2に戻す（accessory_allocator.go の
// コメントに書いた式）。複合（barbell_row 相当）が選ばれてしまい、
// side_raise 相当を実施済みでも未実施でもない中間の状態にしなくても、
// 複合の ΔL が孤立を上回って本テストが落ちる。
func TestAccessoryAllocator_CubedLossFavorsTheMostBehindRegion(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.SideDelt: 10, training.TrapMid: 10, training.RearDelt: 10,
	})
	// SideDelt は実績0（達成率0%）。TrapMid・RearDelt は実績12（達成率30%）。
	baseline := planning.StimulusCoverage{}.
		Plus(regionOnly(t, "hist_trap", training.TrapMid, 1.0).Stimulus(), mustSetCount(t, 12)).
		Plus(regionOnly(t, "hist_rear", training.RearDelt, 1.0).Stimulus(), mustSetCount(t, 12))

	isolation := regionOnly(t, "side_raise_like", training.SideDelt, 1.0)
	compound, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "barbell_row_like", Name: "barbell_row_like",
		Stimulus: map[training.MuscleRegion]float64{
			training.TrapMid: 1.0, training.RearDelt: 1.0,
		},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("NewExercise(barbell_row_like): %v", err)
	}

	history := setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "l1", allocatorDay.AddDays(-10), "barbell_row_like", 40, 8, 2),
	})

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 1)},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{isolation, compound},
		Master:           []*exercise.Exercise{isolation, compound},
		History:          history,
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !contains(got[0], "side_raise_like") {
		t.Errorf("回0の割り当てが %v。達成率0%%で最も遅れている side_raise_like が選ばれるはず", idsOf(got[0]))
	}
	if contains(got[0], "barbell_row_like") {
		t.Errorf("回0の割り当てが %v。2区分ぶんの中程度の改善を足し合わせた barbell_row_like が"+
			"勝ってしまっている", idsOf(got[0]))
	}
}

// TestAccessoryAllocator_DoesNotBlowUpRegionsAtTarget は「目標に届いた
// 区分には足さない（腹が振り切れない）」を守る。
//
// 区分D（達成率0%）と区分S（達成率325%・大幅な超過）を用意する。D専用の
// 候補 "pure" と、Dに加えてSも刺激する候補 "combo" を比べると、combo は
// 超過中のSへさらに積む分だけ損失が増える（α>0 の罰）ので pure が勝つ。
// Sの超過を大きく取るのは、浅い超過だと罰が弱く combo が「ほぼ同じ」の帯に
// 入って同点処理に落ちてしまうため（実測で確認済み。手を動かして数値を
// 決めた。α・指数を測り直すたびに崩れていないか確認すること）。
//
// 【変異】overAttainmentWeight（α）を0にする。超過の罰が消えると combo の
// Sへの追加がタダになり、combo と pure の ΔL が Dの項だけで完全に一致する。
// pure に「実施済み」、combo に「未実施」を割り当てておくと、同点処理が
// 未実施の combo を選んでしまい、Sへ積んだ（振り切れた）ことを示す。
func TestAccessoryAllocator_DoesNotBlowUpRegionsAtTarget(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Abs: 10, training.Oblique: 10,
	})
	// Abs(D) は実績0。Oblique(S) は実績130（4週目標40に対し325%、大幅な超過）。
	// SetCount は1回のPlusにつき100までなので、2回に分けて積む。
	baseline := planning.StimulusCoverage{}.
		Plus(regionOnly(t, "hist_s1", training.Oblique, 1.0).Stimulus(), mustSetCount(t, 100)).
		Plus(regionOnly(t, "hist_s2", training.Oblique, 1.0).Stimulus(), mustSetCount(t, 30))

	pure := regionOnly(t, "pure", training.Abs, 1.0)
	combo, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "combo", Name: "combo",
		Stimulus: map[training.MuscleRegion]float64{
			training.Abs: 1.0, training.Oblique: 1.0,
		},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("NewExercise(combo): %v", err)
	}

	history := setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "l1", allocatorDay.AddDays(-10), "pure", 20, 8, 2),
	})

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 1)},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{pure, combo},
		Master:           []*exercise.Exercise{pure, combo},
		History:          history,
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if contains(got[0], "combo") {
		t.Errorf("回0の割り当てが %v。超過中の区分Sへさらに積む combo ではなく、"+
			"不足を埋める pure が選ばれるはず", idsOf(got[0]))
	}
	if !contains(got[0], "pure") {
		t.Errorf("回0の割り当てが %v。pure が選ばれるはず", idsOf(got[0]))
	}
}

// TestAccessoryAllocator_SecondaryContributionsCountToo は「副次の寄与も
// 損失に数える（二頭と前腕の状況でカールが入れ替わる）」を守る。
//
// ハンマーカール相当（前腕が主働1.0、二頭が副次0.8）とバーベルカール相当
// （二頭が主働1.0、前腕が副次0.4）を、前腕の状況を変えて比べる。
//
//   - 二頭・前腕とも達成率50%（同じ出発点）: ハンマーは前腕（自分の主働）を
//     大きく埋め、二頭への寄与はバーベルの主働寄与よりわずかに小さいだけ
//     なので、ハンマーの合計 ΔL の方が大きく減り、ハンマーが選ばれる
//   - 前腕がすでに超過（達成率170%）: ハンマーは前腕にさらに積んで α の罰を
//     受けるので、バーベルが選ばれる
//
// 【変異】ΔL の計算を主働（寄与1.0以上）の区分だけに絞る。1つめのケースは
// 主働どうし（前腕の項 vs 二頭の項）を比べると完全に一致するように数値を
// 組んであるので、同点処理が種目ID昇順（barbell が先）に落ちてバーベルが
// 選ばれてしまい、本テストが落ちる。
func TestAccessoryAllocator_SecondaryContributionsCountToo(t *testing.T) {
	hammer, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "hammer", Name: "ハンマーカール",
		Stimulus:    map[training.MuscleRegion]float64{training.Forearm: 1.0, training.Biceps: 0.8},
		IncrementKg: 2.0,
	})
	if err != nil {
		t.Fatalf("NewExercise(hammer): %v", err)
	}
	barbell, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "barbell", Name: "バーベルカール",
		Stimulus:    map[training.MuscleRegion]float64{training.Biceps: 1.0, training.Forearm: 0.4},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("NewExercise(barbell): %v", err)
	}

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Biceps: 10, training.Forearm: 10,
	})

	cases := []struct {
		name        string
		forearmSets int // 実績セット数（Forearmのみ）
		bicepsSets  int // 実績セット数（Bicepsのみ）
		want        exercise.ExerciseID
	}{
		{
			// 4週目標40。二頭・前腕とも 20/40=50%（同じ達成率から出発）。
			name: "前腕はまだ余裕がある", bicepsSets: 20, forearmSets: 20,
			want: "hammer",
		},
		{
			// 二頭 20/40=50%、前腕 68/40=170%（超過）。
			name: "前腕はすでに超過している", bicepsSets: 20, forearmSets: 68,
			want: "barbell",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			baseline := planning.StimulusCoverage{}.
				Plus(regionOnly(t, "hist_b", training.Biceps, 1.0).Stimulus(), mustSetCount(t, c.bicepsSets)).
				Plus(regionOnly(t, "hist_f", training.Forearm, 1.0).Stimulus(), mustSetCount(t, c.forearmSets))

			req := planning.AllocationRequest{
				Target:           target,
				Baseline:         baseline,
				Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 1)},
				SetsPerAccessory: mustSetCount(t, 3),
				Pool:             []*exercise.Exercise{hammer, barbell},
				Master:           []*exercise.Exercise{hammer, barbell},
				History:          setlog.NewHistory(nil),
			}

			got, err := planning.DefaultAccessoryAllocator().Allocate(req)
			if err != nil {
				t.Fatalf("Allocate: %v", err)
			}
			if !contains(got[0], c.want) {
				t.Errorf("回0の割り当てが %v。%s が選ばれるはず", idsOf(got[0]), c.want)
			}
		})
	}
}

// TestAccessoryAllocator_NoFrontSquatOnShoulderDay は「分割の日の種目
// （S1）：肩の日にフロントスクワットが出ない」を守る。
//
// 周期は「肩」（SideDelt）と「脚」（Quad）の2日。今日は肩の日。
// フロントスクワット相当（Quad 主働1.0）は Quad の残差が非常に大きいので
// 分割の制約が無ければ最有力候補になるが、Quad は脚の日に属し（未所属では
// ない）、今日の区分ではないので選ばれてはならない。
//
// 【変異】S1（splitAllowsAccessory）の判定を外す（常に true にする）。
// フロントスクワット相当が選ばれてしまい、本テストが落ちる。
func TestAccessoryAllocator_NoFrontSquatOnShoulderDay(t *testing.T) {
	shoulderDay, err := program.NewSplit("肩", []training.MuscleRegion{training.SideDelt})
	if err != nil {
		t.Fatalf("NewSplit(肩): %v", err)
	}
	legDay, err := program.NewSplit("脚", []training.MuscleRegion{training.Quad})
	if err != nil {
		t.Fatalf("NewSplit(脚): %v", err)
	}
	cycle := []program.Split{shoulderDay, legDay}

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.SideDelt: 10, training.Quad: 10,
	})
	// SideDelt 32/40=80%、Quad 0/40=0%（Quadの方がずっと不足）。
	baseline := coverage(t, regionOnly(t, "hist_shoulder", training.SideDelt, 1.0), 32)

	frontSquat := regionOnly(t, "front_squat_like", training.Quad, 1.0)
	sideRaise := regionOnly(t, "side_raise_like", training.SideDelt, 1.0)

	session := planning.HorizonSession{
		Date: allocatorDay, Split: shoulderDay, HasSplit: true, Slots: 1,
	}
	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         []planning.HorizonSession{session},
		Cycle:            cycle,
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{frontSquat, sideRaise},
		Master:           []*exercise.Exercise{frontSquat, sideRaise},
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if contains(got[0], "front_squat_like") {
		t.Errorf("肩の日の割り当てが %v。脚の区分だけに効く front_squat_like が出てはいけない", idsOf(got[0]))
	}
	if !contains(got[0], "side_raise_like") {
		t.Errorf("肩の日の割り当てが %v。side_raise_like が選ばれるはず", idsOf(got[0]))
	}
}

// TestAccessoryAllocator_RecoveryLooksBothWays は「回復を前後両方で守る」を
// 守る。主働（寄与1.0以上）が回復中なら、引き続き候補を締め出すことも
// 確かめる（M2・刺激源も主働だけで数える、に切り替えたあとに残るべき
// 挙動）。
//
// 回0（today）と回1（today+1日）の2回。回1の軸が二頭にも主働で触れる
// （デッドリフトのように複数区分を1.0で持つ種目を模す）。回復日数2なら、
// 回0からの差は1日（前後どちらでも）で回復窓に入る。候補は二頭が主働の
// 種目1つだけ。回0の視点では「1日先（未来）に二頭を主働で刺激する回が
// ある」ため回0では選べず、回1では「同じ日（差0）」は回復窓に入らないので
// 選べる。結果、回0の割り当ては空になり、回1に割り当てられる。
//
// 【変異】touching の判定を「差が正（過去方向）のときだけ」に絞る
// （前方向を見なくする）。回0が二頭の候補を選べるようになり、しかも
// 同じ種目・同じ ΔL で回1より日付が早い分だけ優先されるため、回0に
// 誤って割り当てられ、本テストが落ちる。
func TestAccessoryAllocator_RecoveryLooksBothWays(t *testing.T) {
	rows, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "rows_like", Name: "rows_like",
		Stimulus:    map[training.MuscleRegion]float64{training.Lat: 1.0, training.Biceps: 1.0},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("NewExercise(rows_like): %v", err)
	}
	curl := regionOnly(t, "curl", training.Biceps, 1.0)

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Biceps: 10, training.Lat: 10,
	})

	sessions := []planning.HorizonSession{
		{Date: allocatorDay, Slots: 1}, // 回0：今日
		{ // 回1：明日。軸が二頭に副次で触れる。
			Date: allocatorDay.AddDays(1), Slots: 1,
			Axis:     rows,
			Stimulus: coverage(t, rows, 3),
		},
	}

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         planning.StimulusCoverage{},
		Sessions:         sessions,
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{curl},
		Master:           []*exercise.Exercise{curl, rows},
		History:          setlog.NewHistory(nil),
	}

	allocator, err := planning.NewAccessoryAllocator(2)
	if err != nil {
		t.Fatalf("NewAccessoryAllocator: %v", err)
	}
	got, err := allocator.Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if contains(got[0], "curl") {
		t.Errorf("回0の割り当てが %v。翌日の軸が二頭を刺激するので回0では選べないはず", idsOf(got[0]))
	}
	if !contains(got[1], "curl") {
		t.Errorf("回1の割り当てが %v。curl はここに割り当てられるはず", idsOf(got[1]))
	}
}

// TestAccessoryAllocator_RecoverySourceCountsOnlyPrimaryMovers は M2 の
// 再現・回帰検査。回復の刺激源も主働（寄与1.0以上）だけを数えることを守る。
//
// five_way の隣接日（背中・肩）を模す。barbell_row 相当（TrapMid 主働1.0・
// RearDelt 副次0.4）と rear_delt_fly 相当（RearDelt 主働1.0・TrapMid
// 副次0.3）は、互いの主働ではなく副次にしか触れない。副次まで回復の
// 刺激源に数えると、前日にどちらかが出た瞬間にもう片方が翌日締め出され、
// 実際に通し検証で REAR_DELT・TRAP_MID の未達として出た
// （TestSimulation_SplitWeeklyTargetIsAttainable）。
//
// 【変異】刺激源側の判定を primaryRegions ではなく
// e.Stimulus().Regions()（副次も含む）に戻す。前日に固定した種目の副次が
// 翌日の候補の主働を回復中にしてしまい、締め出されて本テストが落ちる。
func TestAccessoryAllocator_RecoverySourceCountsOnlyPrimaryMovers(t *testing.T) {
	barbellRow, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "barbell_row_like", Name: "barbell_row_like",
		Stimulus:    map[training.MuscleRegion]float64{training.TrapMid: 1.0, training.RearDelt: 0.4},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("NewExercise(barbell_row_like): %v", err)
	}
	rearDeltFly, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "rear_delt_fly_like", Name: "rear_delt_fly_like",
		Stimulus:    map[training.MuscleRegion]float64{training.RearDelt: 1.0, training.TrapMid: 0.3},
		IncrementKg: 1.0,
	})
	if err != nil {
		t.Fatalf("NewExercise(rear_delt_fly_like): %v", err)
	}

	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.TrapMid: 10, training.RearDelt: 10,
	})

	cases := []struct {
		name      string
		fixed     *exercise.Exercise // 前日に軸として固定で刺激される種目
		candidate *exercise.Exercise // 当日の唯一の候補
	}{
		{
			name:  "barbell_rowの副次(RearDelt)がrear_delt_flyを締め出さない",
			fixed: barbellRow, candidate: rearDeltFly,
		},
		{
			name:  "rear_delt_flyの副次(TrapMid)がbarbell_rowを締め出さない",
			fixed: rearDeltFly, candidate: barbellRow,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sessions := []planning.HorizonSession{
				{ // 前日：fixed が軸として出る（枠は無い＝それ自体は候補にならない）。
					Date: allocatorDay, Axis: c.fixed, Stimulus: coverage(t, c.fixed, 3), Slots: 0,
				},
				{Date: allocatorDay.AddDays(1), Slots: 1}, // 当日
			}

			req := planning.AllocationRequest{
				Target:           target,
				Baseline:         planning.StimulusCoverage{},
				Sessions:         sessions,
				SetsPerAccessory: mustSetCount(t, 3),
				Pool:             []*exercise.Exercise{c.candidate},
				Master:           []*exercise.Exercise{c.candidate, c.fixed},
				History:          setlog.NewHistory(nil),
			}

			allocator, err := planning.NewAccessoryAllocator(2)
			if err != nil {
				t.Fatalf("NewAccessoryAllocator: %v", err)
			}
			got, err := allocator.Allocate(req)
			if err != nil {
				t.Fatalf("Allocate: %v", err)
			}
			if !contains(got[1], c.candidate.ID()) {
				t.Errorf("当日の割り当てが %v。%s が前日の副次の重複で締め出されずに選ばれるはず",
					idsOf(got[1]), c.candidate.ID())
			}
		})
	}
}

// TestAccessoryAllocator_IdenticalProfilesAlternate は「寄与が同じ組は
// 交互に出る（インクラインのダンベルとバーベル）」を守る。
//
// 2種目とも ChestUpper に同じ寄与（1.0）を持ち、ΔL は常に一致する。
// 未実施の種目を優先する同点処理（手順4a）が効けば、一度も選ばれていない
// 方（incline_db）が選ばれる。
//
// 【変異】同点処理の (a) を外し、種目ID昇順だけで決める。
// "incline_barbell" < "incline_db"（辞書順）なので、実施済みのはずの
// incline_barbell が選ばれてしまい、本テストが落ちる。
func TestAccessoryAllocator_IdenticalProfilesAlternate(t *testing.T) {
	inclineDB := regionOnly(t, "incline_db", training.ChestUpper, 1.0)
	inclineBarbell := regionOnly(t, "incline_barbell", training.ChestUpper, 1.0)

	target := mustTarget(t, map[training.MuscleRegion]float64{training.ChestUpper: 10})
	baseline := coverage(t, regionOnly(t, "hist", training.ChestUpper, 1.0), 10) // 25%、余地は十分

	history := setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "l1", allocatorDay.AddDays(-5), "incline_barbell", 40, 8, 2),
	})

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 1)},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{inclineDB, inclineBarbell},
		Master:           []*exercise.Exercise{inclineDB, inclineBarbell},
		History:          history,
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !contains(got[0], "incline_db") {
		t.Errorf("回0の割り当てが %v。未実施の incline_db が選ばれるはず", idsOf(got[0]))
	}
}

// TestAccessoryAllocator_TiesSpreadToTheSessionWithMoreFreeSlots は
// 「同じくらい効くなら空き枠の多い回へ散る」を守る。
//
// 同じ候補が回0（空き枠1・日付が早い）と回1（空き枠3・日付が遅い）の
// 両方で選べる。数値は「1回の追加でちょうど不足が解消し、2回目の追加は
// 損失を悪化させる」ように組んであるので、確定は1回だけ起きる。
// 手順4bが手順4cより先に効けば、日付ではなく空き枠の多い回1が選ばれる。
//
// 【変異】同点処理から (b) を外し、(c) 日付だけで決める。
// 日付の早い回0が選ばれてしまい、本テストが落ちる。
func TestAccessoryAllocator_TiesSpreadToTheSessionWithMoreFreeSlots(t *testing.T) {
	e := regionOnly(t, "candidate", training.Calf, 1.0)
	target := mustTarget(t, map[training.MuscleRegion]float64{training.Calf: 10})
	// 4週目標40。実績37（92.5%）。3セット追加でちょうど100%に届き、
	// もう3セット追加すると超過側に振れて2回目の ΔL が正になる。
	baseline := coverage(t, e, 37)

	sessions := []planning.HorizonSession{
		{Date: allocatorDay, Slots: 1},            // 回0：空き枠が少ない・日付は早い
		{Date: allocatorDay.AddDays(2), Slots: 3}, // 回1：空き枠が多い・日付は遅い
	}

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         sessions,
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{e},
		Master:           []*exercise.Exercise{e},
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if len(got[0]) != 0 {
		t.Errorf("回0の割り当てが %v。空き枠の多い回1へ割り当てられるはず", idsOf(got[0]))
	}
	if !contains(got[1], "candidate") {
		t.Errorf("回1の割り当てが %v。candidate が選ばれるはず", idsOf(got[1]))
	}
}

// TestAccessoryAllocator_IgnoresRegionsWithoutATarget は「目標0の区分は
// 数えない」を守る。
//
// combo は目標のある区分Aと、週目標を設定していない区分Zの両方に寄与1.0を
// 持つ。onlyA はAだけに同じ寄与を持つ。Zが分母を作らなければ、combo と
// onlyA の ΔL はAの項だけで完全に一致し、同点処理（未実施優先）で
// combo が選ばれる（onlyA は実施済みにしておく）。
//
// 【変異】ΔL の計算で「週目標が0以下の区分は飛ばす」ガードを外す
// （分母に0を入れる）。0除算で NaN・Inf が伝播し、比較が壊れて
// combo が選ばれなくなる（多くの場合 near のスライスが空になり
// index out of range で panic する）。
func TestAccessoryAllocator_IgnoresRegionsWithoutATarget(t *testing.T) {
	// training.Oblique は週目標を設定しない（未所属＝目標0）。
	regionA := training.Calf
	regionZ := training.Oblique

	combo, err := exercise.NewExercise(exercise.ExerciseParams{
		ID: "combo", Name: "combo",
		Stimulus:    map[training.MuscleRegion]float64{regionA: 1.0, regionZ: 1.0},
		IncrementKg: 2.5,
	})
	if err != nil {
		t.Fatalf("NewExercise(combo): %v", err)
	}
	onlyA := regionOnly(t, "only_a", regionA, 1.0)

	target := mustTarget(t, map[training.MuscleRegion]float64{regionA: 10})
	baseline := planning.StimulusCoverage{} // Aは実績0（達成率0%）

	history := setlog.NewHistory([]*setlog.SetLog{
		mkLogOn(t, "l1", allocatorDay.AddDays(-10), "only_a", 20, 8, 2),
	})

	req := planning.AllocationRequest{
		Target:           target,
		Baseline:         baseline,
		Sessions:         []planning.HorizonSession{noSplitSession(allocatorDay, 1)},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{combo, onlyA},
		Master:           []*exercise.Exercise{combo, onlyA},
		History:          history,
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !contains(got[0], "combo") {
		t.Errorf("回0の割り当てが %v。目標の無い区分は分母にならず onlyA と同点になるので、"+
			"未実施の combo が選ばれるはず", idsOf(got[0]))
	}
}

// TestAccessoryAllocator_CountsTheHorizonStimulus は、先の回の軸・
// バリエーションが入れる刺激（HorizonSession.Stimulus）が積み上げに
// 入ることを守る。
//
// 以前は計画の側（TestSessionPlanner_VariationCoverageFreesSlotsForOtherRegions）
// で「重点種目の派生がある日は胸の遅れが減り、二頭筋に枠が回る」として
// 見ていた。重点種目の系統ぶん週目標を上げる（raiseForFocus）ようにして
// からは、派生の刺激と目標の上げ幅が相殺して胸の遅れは減らないので、
// 計画の側ではこの性質を観測できない。割り振り器の入力で直接見る。
//
// 区分A（Quad）とB（Hamstring）は同じ週目標で実績ゼロ。回0の Stimulus で
// Aを3セットぶん埋めておくと、Bのほうが遅れているのでBの候補が選ばれる。
//
// 【変異】Allocate の `v += s.Stimulus.Sets(r)` を外す。A・Bが同点になり、
// 同点処理（ID昇順）で candidate_a が選ばれて落ちる。
func TestAccessoryAllocator_CountsTheHorizonStimulus(t *testing.T) {
	target := mustTarget(t, map[training.MuscleRegion]float64{
		training.Quad: 10, training.Hamstring: 10,
	})
	candA := regionOnly(t, "candidate_a", training.Quad, 1.0)
	candB := regionOnly(t, "candidate_b", training.Hamstring, 1.0)

	session := noSplitSession(allocatorDay, 1)
	session.Stimulus = coverage(t, regionOnly(t, "axis_quad", training.Quad, 1.0), 3)

	req := planning.AllocationRequest{
		Target:           target,
		Sessions:         []planning.HorizonSession{session},
		SetsPerAccessory: mustSetCount(t, 3),
		Pool:             []*exercise.Exercise{candA, candB},
		Master:           []*exercise.Exercise{candA, candB},
		History:          setlog.NewHistory(nil),
	}

	got, err := planning.DefaultAccessoryAllocator().Allocate(req)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if !contains(got[0], "candidate_b") {
		t.Errorf("回0の割り当てが %v。先の回の刺激で Quad は埋まっているので、"+
			"遅れている Hamstring の candidate_b が選ばれるはず", idsOf(got[0]))
	}
}
