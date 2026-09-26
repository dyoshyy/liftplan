package planning

import (
	"errors"
	"fmt"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
	"github.com/dyoshyy/liftplan/internal/domain/training/program"
	"github.com/dyoshyy/liftplan/internal/domain/training/setlog"
)

// overAttainmentWeight は損失の α（超過の重み）。
//
// 0 < α < 1。超過（ρ≥1）を不足（ρ<1）より軽く罰する。
//
// 指数を2から3に直したあと、α∈{1/8,1/4,1/2}×β∈{0.8,0.9,0.95} の9通りを
// 再度測り直した（PR 本文にグリッドの表がある）。α=1/4, β=0.9 を選んだ。
// 全身法（TestSimulation_WeeklyTargetIsAttainableAtEveryFrequency）を崩さない
// 組み合わせの中で、通し検証全体の失敗が最少（`TestSimulation_SplitWeeklyTargetIsAttainable`
// のうち five_way の週2・3回のみが残る）。設計書の初期値でもある。
//
// 定数にして利用者の設定にしないのは設計書の決定（「どちらも定数」）。
const overAttainmentWeight = 1.0 / 4.0

// defaultRecoveryDays は既定の回復日数。
const defaultRecoveryDays = 2

// primaryContribution はこの値以上の寄与を「主働筋として使う」とみなす境界。
//
// 回復期間中の筋区分に対しては、主働筋として使う種目を避ける。
// 補助的に軽く関与するぶん（三頭が 0.4 など）まで避けると、
// 多関節種目がほとんど選べなくなる。
const primaryContribution = 1.0

// similarityBand は損失の β（「ほぼ同じ」の幅）。
//
// 0 < β < 1。最良の減り幅の β 倍以上を「ほぼ同じ」として多様性の選定に
// 回す。0.9 を選んだ（グリッド探索の結果は overAttainmentWeight の
// コメントと PR 本文を参照）。設計書の初期値でもある。
const similarityBand = 0.9

// HorizonSession は割り振り器が読む、先の回1つぶんの入力。
//
// horizon_projector.go の ProjectedSession から作る値だが、型として
// 直接は使わない。ProjectedSession は非公開フィールドしか持たず、
// このパッケージの外（テストを含む）からは ProjectHorizon 経由でしか
// 組み立てられない。割り振り器の単体テストは分割・回復・多様性を
// 個別に作り込んだ入力で確かめる必要があるため、フィールドをそのまま
// 公開する専用の入力型を割り振り器の側に置く。PlanRequest と同じ理由。
type HorizonSession struct {
	// Date はその回の日付。
	Date training.Date
	// Split と HasSplit はその回の分割の日。分割が無ければ HasSplit は false
	// （全区分を狙う）。
	Split    program.Split
	HasSplit bool
	// Axis と Variation はその回の軸・バリエーション（無ければ nil）。
	// 回復の判定（前後 recoveryDays 以内に刺激していないか）の材料になる。
	Axis      *exercise.Exercise
	Variation *exercise.Exercise
	// Stimulus は軸・バリエーションがその回にすでに入れる刺激。
	Stimulus StimulusCoverage
	// Slots はその回の補助に使える空き枠の数。
	Slots int
}

// AccessoryAllocator は1週ぶんの補助の枠を、目標からのずれ（損失）の
// 最小化でまとめて埋めるドメインサービス。無状態。
type AccessoryAllocator struct {
	recoveryDays int
}

func NewAccessoryAllocator(recoveryDays int) (AccessoryAllocator, error) {
	if recoveryDays < 0 {
		return AccessoryAllocator{}, fmt.Errorf("回復日数は0以上である必要がある: %d", recoveryDays)
	}
	return AccessoryAllocator{recoveryDays: recoveryDays}, nil
}

func DefaultAccessoryAllocator() AccessoryAllocator {
	a, err := NewAccessoryAllocator(defaultRecoveryDays)
	if err != nil {
		panic(fmt.Sprintf("既定の回復日数が不正: %v", err))
	}
	return a
}

func (a AccessoryAllocator) RecoveryDays() int { return a.recoveryDays }

func (a AccessoryAllocator) IsZero() bool { return a == AccessoryAllocator{} }

// AllocationRequest は Allocate への入力。
type AllocationRequest struct {
	// Target は週目標。
	Target program.WeeklyVolumeTarget
	// Baseline は評価日 E 時点の4週の窓に入る、前日までの実際の記録の刺激。
	Baseline StimulusCoverage
	// Sessions は先の回。今日を含めて頻度ぶん、日付の昇順。
	Sessions []HorizonSession
	// Cycle は分割の周期。空なら分割なし。未所属の区分の判定に使う
	// （どの日にも書かれていない区分は毎日活きる。D-113 と同じ規則）。
	Cycle []program.Split
	// SetsPerAccessory は補助1種目あたりのセット数。全回・全種目で共通。
	SetsPerAccessory training.SetCount
	// Pool は候補種目。除外（宣言種目、重点種目の系統、選択されていない
	// 種目）はすでに引いてある。
	Pool []*exercise.Exercise
	// Master は種目マスタ全件。実際の記録が指す種目を引く辞書として使う。
	// Pool が除外済みでも、除外した種目の記録は回復の判定に要る。
	// 除外しても、その種目が刺激した筋区分が回復中かどうかの判定には
	// 引き続き使うため、履歴を辿る辞書からは抜けない。
	Master []*exercise.Exercise
	// History は実際の記録。前日まで（呼び出し側が切る）。
	History setlog.History
}

// stimulusEvent は「この日に、この種目ぶんの刺激があった（あるいは、
// あることにする）」という1件。実際の記録、予測した軸・バリエーション、
// すでに割り振った補助のいずれかから来る。
type stimulusEvent struct {
	date training.Date
	ex   *exercise.Exercise
}

// Allocate は1週ぶんの補助種目の枠を貪欲に埋める。戻り値は回ごとの
// 補助種目IDの列（回の個数ぶん）。Plan は回0だけを使う。
//
// 手順は設計書のとおり。
//
//  1. 全候補の組（回, 種目）について ΔL（足したときの損失の変化）を出す
//  2. 最良の ΔL* ≥ 0 なら止める
//  3. ΔL ≤ β・ΔL* の組を残す
//  4. (a)最終実施日が一番古い種目 (b)空き枠が一番多い回 (c)日付が早い回
//     (d)種目ID昇順 の順で1つに絞り、確定して1へ
//
// 候補が尽きるか、改善が無くなれば止まる。枠が残っていても止まる
// （1回の量は上限であってノルマではない）。
func (a AccessoryAllocator) Allocate(req AllocationRequest) ([][]exercise.ExerciseID, error) {
	if a.IsZero() {
		return nil, errors.New("割り振り器が未設定である")
	}
	if req.Target.IsEmpty() {
		return nil, errors.New("週目標が指定されていない")
	}
	if len(req.Sessions) == 0 {
		return nil, errors.New("先の回が予測されていない")
	}

	pool := make([]*exercise.Exercise, 0, len(req.Pool))
	for _, e := range req.Pool {
		if e != nil {
			pool = append(pool, e)
		}
	}
	// ID昇順に固定する。同点処理（手順4d）がこの並びに依存するので、
	// 呼び出し側の並びに関わらずここで揃える。
	sort.Slice(pool, func(i, j int) bool { return pool[i].ID() < pool[j].ID() })

	byID := make(map[exercise.ExerciseID]*exercise.Exercise, len(req.Master))
	for _, e := range req.Master {
		if e != nil {
			byID[e.ID()] = e
		}
	}

	// current はいまの区分ごとの積み上げ（C_r）。窓の実績＋1週ぶんの
	// 軸・バリエーションの予測から始め、補助を確定するたびに足していく。
	// 対象は週目標がある区分だけ（目標0の区分は分母にしないので追わない）。
	current := make(map[training.MuscleRegion]float64, len(req.Target.Regions()))
	for _, r := range req.Target.Regions() {
		v := req.Baseline.Sets(r)
		for _, s := range req.Sessions {
			v += s.Stimulus.Sets(r)
		}
		current[r] = v
	}

	// fixed は動かない刺激イベント（実際の記録＋予測した軸・バリエーション）。
	// 回復の判定に使う。割り振った補助（allocated）だけがループ中に増える。
	var fixed []stimulusEvent
	for _, l := range req.History.Logs() {
		if e, ok := byID[l.ExerciseID()]; ok {
			fixed = append(fixed, stimulusEvent{date: l.PerformedOn(), ex: e})
		}
	}
	for _, s := range req.Sessions {
		if s.Axis != nil {
			fixed = append(fixed, stimulusEvent{date: s.Date, ex: s.Axis})
		}
		if s.Variation != nil {
			fixed = append(fixed, stimulusEvent{date: s.Date, ex: s.Variation})
		}
	}
	var allocated []stimulusEvent

	// touching はその日の前後 recoveryDays 未満（両方向、当日は含まない）に
	// 主働として刺激されている区分。
	//
	// 刺激源の側も主働（寄与1.0以上）だけを数える。候補の側（recoveryBlocks）
	// はすでに主働だけで判定しているので、刺激源の側だけ副次まで拾うと
	// 非対称になる。副次まで拾っていたときは、隣接する日の候補が互いを
	// 締め出す症状が出た：five_way の「背中」「肩」は隣接日で、
	// barbell_row（RearDelt 副次0.4）が背中の日に選ばれると肩の日の
	// RearDelt が「回復中」になり rear_delt_fly を締め出し、逆に
	// rear_delt_fly（TrapMid 副次0.3）が肩の日に選ばれると背中の日の
	// TrapMid を締め出して barbell_row・seated_row を締め出していた
	// （TestSimulation_SplitWeeklyTargetIsAttainable の REAR_DELT・
	// TRAP_MID 未達）。主働だけに絞ると、この相互ブロックは起きない
	// （TestAccessoryAllocator_RecoverySourceCountsOnlyPrimaryMovers が守る）。
	//
	// 主働が回復中であることは引き続き候補を締め出す
	// （TestAccessoryAllocator_RecoveryLooksBothWays が守る）。
	touching := func(d training.Date) map[training.MuscleRegion]bool {
		out := map[training.MuscleRegion]bool{}
		add := func(events []stimulusEvent) {
			for _, ev := range events {
				diff := d.DaysSince(ev.date)
				if diff < 0 {
					diff = -diff
				}
				if diff == 0 || diff >= a.recoveryDays {
					continue
				}
				for _, r := range primaryRegions(ev.ex) {
					out[r] = true
				}
			}
		}
		add(fixed)
		add(allocated)
		return out
	}

	// lastPerformed は種目ごとの最終実施日。未実施は hasLast が false。
	// 割り振りが確定するたびに、その回の日付で更新する
	// （設計書「予測の中で先の回に割り振ったものはその日にやったとして数える」）。
	lastPerformed := make(map[exercise.ExerciseID]training.Date, len(pool))
	hasLast := make(map[exercise.ExerciseID]bool, len(pool))
	for _, e := range pool {
		if d, ok := req.History.LastPerformed(e.ID()); ok {
			lastPerformed[e.ID()] = d
			hasLast[e.ID()] = true
		}
	}

	slots := make([]int, len(req.Sessions))
	chosenSet := make([]map[exercise.ExerciseID]bool, len(req.Sessions))
	out := make([][]exercise.ExerciseID, len(req.Sessions))
	for k, s := range req.Sessions {
		slots[k] = s.Slots
		chosenSet[k] = map[exercise.ExerciseID]bool{}
	}

	type candidate struct {
		k     int
		e     *exercise.Exercise
		delta float64
	}

	for {
		var candidates []candidate
		for k, s := range req.Sessions {
			if slots[k] <= 0 {
				continue
			}
			recovering := touching(s.Date)
			for _, e := range pool {
				if chosenSet[k][e.ID()] {
					continue
				}
				primary := primaryRegions(e)
				if s.HasSplit && !splitAllowsAccessory(primary, s.Split, req.Cycle) {
					continue
				}
				if recoveryBlocks(primary, recovering) {
					continue
				}
				candidates = append(candidates, candidate{
					k: k, e: e, delta: deltaLoss(current, req.Target, e, req.SetsPerAccessory),
				})
			}
		}
		if len(candidates) == 0 {
			break
		}

		best := candidates[0].delta
		for _, c := range candidates[1:] {
			if c.delta < best {
				best = c.delta
			}
		}
		if best >= 0 {
			break
		}

		threshold := similarityBand * best
		near := make([]candidate, 0, len(candidates))
		for _, c := range candidates {
			if c.delta <= threshold {
				near = append(near, c)
			}
		}

		sort.SliceStable(near, func(i, j int) bool {
			ci, cj := near[i], near[j]

			// (a) 最終実施日が一番古い種目。未実施が最優先。
			hi, hj := hasLast[ci.e.ID()], hasLast[cj.e.ID()]
			if hi != hj {
				return !hi
			}
			if hi && hj {
				di, dj := lastPerformed[ci.e.ID()], lastPerformed[cj.e.ID()]
				if !di.Equal(dj) {
					return di.Before(dj)
				}
			}
			// (b) 空き枠が一番多い回。
			if slots[ci.k] != slots[cj.k] {
				return slots[ci.k] > slots[cj.k]
			}
			// (c) 日付が早い回。
			di, dj := req.Sessions[ci.k].Date, req.Sessions[cj.k].Date
			if !di.Equal(dj) {
				return di.Before(dj)
			}
			// (d) 種目IDの昇順。
			return ci.e.ID() < cj.e.ID()
		})

		chosen := near[0]
		out[chosen.k] = append(out[chosen.k], chosen.e.ID())
		chosenSet[chosen.k][chosen.e.ID()] = true
		slots[chosen.k]--

		for _, r := range chosen.e.Stimulus().Regions() {
			t := req.Target.Sets(r)
			if t <= 0 {
				continue
			}
			c, ok := chosen.e.Stimulus().Contribution(r)
			if !ok {
				continue
			}
			current[r] = training.Quantize(current[r] + c.TimesSets(req.SetsPerAccessory))
		}

		d := req.Sessions[chosen.k].Date
		allocated = append(allocated, stimulusEvent{date: d, ex: chosen.e})
		if !hasLast[chosen.e.ID()] || d.After(lastPerformed[chosen.e.ID()]) {
			lastPerformed[chosen.e.ID()] = d
			hasLast[chosen.e.ID()] = true
		}
	}

	return out, nil
}

// candidateAccessories は補助の候補プール。exclude に挙がった種目
// （宣言種目、重点種目の系統、選択されていない種目）を master から引く。
//
// ID順のソートは Allocate の側で行う（呼び出し側の並びに依存しないため）。
func candidateAccessories(master []*exercise.Exercise, exclude []exercise.ExerciseID) []*exercise.Exercise {
	excluded := make(map[exercise.ExerciseID]bool, len(exclude))
	for _, id := range exclude {
		excluded[id] = true
	}
	out := make([]*exercise.Exercise, 0, len(master))
	for _, e := range master {
		if e == nil || excluded[e.ID()] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// primaryRegions はその種目の主働（寄与1.0以上）の区分。
func primaryRegions(e *exercise.Exercise) []training.MuscleRegion {
	out := make([]training.MuscleRegion, 0, 2)
	for _, r := range e.Stimulus().Regions() {
		if c, ok := e.Stimulus().Contribution(r); ok && c.Float() >= primaryContribution {
			out = append(out, r)
		}
	}
	return out
}

// splitAllowsAccessory は S1（分割の日の種目）。種目の主働の区分がすべて、
// 今日の区分か、どの日にも属さない区分に入っていれば許す。
//
// 主働を持たない種目（該当の寄与が無い）は素通しする。そのような種目は
// 実シードには無いが、契約としては「主働で縛る」の対象外というだけで
// 誤りではない。
func splitAllowsAccessory(primary []training.MuscleRegion, today program.Split, cycle []program.Split) bool {
	for _, r := range primary {
		if today.Includes(r) {
			continue
		}
		if !affiliated(cycle, r) {
			continue
		}
		return false
	}
	return true
}

// recoveryBlocks は候補の主働のどれかが回復中（前後 recoveryDays 未満に
// 刺激されている）か。
func recoveryBlocks(primary []training.MuscleRegion, recovering map[training.MuscleRegion]bool) bool {
	for _, r := range primary {
		if recovering[r] {
			return true
		}
	}
	return false
}

// regionLoss は区分1つぶんの損失。週目標 T で重み付けし、遅れの指数は
// 3（2ではない。理由は下）。
//
//	C < 4T： T×|1−ρ|³   = |4T−C|³/(64T²)
//	C ≥ 4T： T×α×|ρ−1|³ = α×|C−4T|³/(64T²)   （ρ = C/(4T)）
//
// ρ を経由せず C・T から直接計算するのは、実装として楽になるからだけ
// ではない。ρ で書くと「達成率にした時点で目標の大小は打ち消される」と
// 誤解しやすいが、それは損失の**値**の話であって**1手あたりの変化**の
// 話ではない（下の重み付けの理由）。C・T のままの式なら、重み T が
// 消えていないことが読める。
//
// **T で重み付けする理由。**重み無し（regionLoss(ρ)=|1−ρ|ⁿ のような形）
// だと、ρ=0 付近での1手あたりの ΔL はおよそ T の逆数〜二乗の逆数に比例して
// 薄まり、達成率がまったく同じでも、週目標の大きい区分（TrapMid・Glute 等）
// は1手の効きが薄く見え、割り振り器の貪欲が系統的に後回しにする
// （実測：TestSimulation_EveryAccessoryGetsUsedInSomeSetup で hip_thrust が
// 一度も選ばれない。Glute は軸の副次寄与だけで達成率100%超に達するのに、
// 割り振り器の側はそれを大きな目標のせいで「まだ効きが薄い」としか
// 見ていなかった）。T を掛けると、1手あたりの ΔL の主要項が T に依存
// しなくなり（TestAccessoryAllocator_DeltaLossIsNotBiasedByTargetSize が、
// 同じ相対的な遅れなら T の大小で有利不利が付かないことを守る）、優先度を
// 決めるのは週目標の絶対値ではなく達成率（相対的な遅れ）になる。これは
// 旧 nextRegion の「欠けている割合で並べる」という発想と同じで、貪欲法の
// 下で保つには重みが要る、という結論だった。
//
// **指数を2ではなく3にする理由（PR 3 で見つかった、Tの重み付けだけでは
// 直らなかった別の症状）。**Tで重み付けしても、「複数区分に中程度効く
// 種目」が「1区分に大きく効く種目」に、後者の区分の方がずっと遅れている
// のに勝つケースが残っていた。指数2の損失は下がり方が遅れの大きさに
// 比例して急になるが、急峻さが2乗どまりだと、複数区分ぶんの中程度の
// 改善を**足し合わせた**値が、1区分の大きな改善に追いついてしまう。
//
// 週目標 T=10 で揃えた例（TestAccessoryAllocator_CubedLossFavorsTheMostBehindRegion
// が固定する）。孤立種目（区分A、達成率0%）と複合種目（区分B・C、それぞれ
// 達成率30%）に同じセット数を足すと、
//
//	指数2： 孤立 ΔL=-1.44375  複合 ΔL=-1.9875（B+C合計） → 複合が勝つ（誤り）
//	指数3： 孤立 ΔL=-2.08547  複合 ΔL=-1.97719（B+C合計） → 孤立が勝つ（正しい）
//
// 分割の通し検証で実際に起きていた症状：five_way の肩の日、`side_raise`
// （SideDeltのみ、達成率がほぼ0%）が `barbell_row`・`rear_delt_fly` のような
// 複数区分の種目に負け続け、TRAP_UPPER・CALF・SIDE_DELTが約54%に張り付いて
// いた（TestSimulation_SplitWeeklyTargetIsAttainable）。指数3にすると
// upper_lower・ppl は全頻度で緑になり、five_way も週4回以降が緑になった
// （週2・3回は残る。原因・数値は PR 本文）。
func regionLoss(c, t float64) float64 {
	d := c - 4*t
	if d < 0 {
		return -(d * d * d) / (64 * t * t)
	}
	return overAttainmentWeight * (d * d * d) / (64 * t * t)
}

// deltaLoss は種目 e を setsPerAccessory ぶん足したときの、損失 L の変化。
//
// e が寄与する区分の合計だけを見る（設計書「種目の減り方は効く全区分の
// 変化の合計」）。target に無い区分（週目標0）は分母を作らないので飛ばす。
func deltaLoss(
	current map[training.MuscleRegion]float64, target program.WeeklyVolumeTarget,
	e *exercise.Exercise, sets training.SetCount,
) float64 {
	total := 0.0
	for _, r := range e.Stimulus().Regions() {
		t := target.Sets(r)
		if t <= 0 {
			continue
		}
		c, ok := e.Stimulus().Contribution(r)
		if !ok {
			continue
		}
		before := regionLoss(current[r], t)
		after := regionLoss(current[r]+c.TimesSets(sets), t)
		total += after - before
	}
	return total
}
