package program

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/exercise"
)

// WeeklyVolumeTarget は筋区分ごとの週あたり目標セット数。不変。
type WeeklyVolumeTarget struct {
	m map[training.MuscleRegion]float64
}

func NewWeeklyVolumeTarget(m map[training.MuscleRegion]float64) (WeeklyVolumeTarget, error) {
	if len(m) == 0 {
		return WeeklyVolumeTarget{}, errors.New("週目標が空である")
	}

	out := make(map[training.MuscleRegion]float64, len(m))
	for region, v := range m {
		if !region.Valid() {
			return WeeklyVolumeTarget{}, fmt.Errorf("未知の筋区分: %q", region)
		}
		// 正で有限であることだけを見る。
		//
		// 以前は 0.5〜40 に収めていた。利用者が手で入力していた頃の防波堤で、
		// 「0を入れるくらいなら区分ごと外せ」「40はどんなプログラムでも過剰」
		// という理由だった。週目標は利用者の設定（頻度 × 種目数 × セット数）
		// から導く値になり、入力ではなくなった。範囲を残すと、選べる設定の
		// うち10通り（週1回の少量設定で0.5未満、週7回6種目6セットで臀筋41）
		// で週目標が組めず、その設定を選んだ瞬間に保存が落ちる。
		//
		// 量子化してから見るので、0に潰れるほど小さい値は弾かれる。
		q := training.Quantize(v)
		name := fmt.Sprintf("筋区分 %s の目標セット数", region)
		if err := training.ValidateRange(name, q, training.SmallestPositive, math.MaxFloat64); err != nil {
			return WeeklyVolumeTarget{}, err
		}
		out[region] = q
	}
	return WeeklyVolumeTarget{m: out}, nil
}

// Sets は設定されていない区分に対して0を返す。
//
// 0は「その区分を狙わない」という意味であり、エラーではない。
// 全21区分を必ず設定させると、シードを触りたくないユーザーが詰まる。
func (t WeeklyVolumeTarget) Sets(r training.MuscleRegion) float64 { return t.m[r] }

// Regions はソート済みの筋区分を返す。再現性のため順序を固定する。
func (t WeeklyVolumeTarget) Regions() []training.MuscleRegion {
	out := make([]training.MuscleRegion, 0, len(t.m))
	for r := range t.m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (t WeeklyVolumeTarget) IsEmpty() bool { return len(t.m) == 0 }

// Program はユーザーの設定を保持する集約ルート。
//
//ddd:aggregate
type Program struct {
	frequency Frequency
	volume    SessionVolume                      // 1セッションの量。利用者の設定
	selected  []exercise.ExerciseID              // 実施可能な種目
	declared  []exercise.ExerciseID              // 重量を伸ばしたい種目
	focus     exercise.ExerciseID                // 重点的に伸ばしたい種目。空なら指定なし
	cycle     []Split                            // 分割の周期。空なら分割なし（全身法）
	reps      map[exercise.ExerciseID]RepTargets // 宣言ごとのレップ数。無い宣言は既定
}

// programParams は Program を組み立てる材料。Program と同じ7つを持つ。
//
// フィールドを足すときに触るのは、Program とここ、newProgram と params。
// ほかの With* は触らない。With* がそれぞれ全フィールドを手で並べ直して
// いたときは、1本でも書き忘れるとコンパイルもテストも通ったまま、その
// 設定だけが黙って消えた（declared と focus で D-127、cycle で #122）。
type programParams struct {
	frequency Frequency
	volume    SessionVolume
	selected  []exercise.ExerciseID
	declared  []exercise.ExerciseID
	focus     exercise.ExerciseID
	cycle     []Split
	reps      map[exercise.ExerciseID]RepTargets
}

// NewProgram は分割なしのプログラムを組み立てる。分割は WithCycle で足す。
//
// 週目標は引数に持たない。週目標は「設定（頻度と1回の量）から導く値」で
// あって集約の持ち物ではない（seed.DefaultWeeklyTarget、D-139、#176）。
// 導いた値が要る側（計画・画面の充足・種目選択の検証）は、都度
// Frequency() と SessionVolume() から組み直す。
func NewProgram(freq Frequency, volume SessionVolume, selected, declared []exercise.ExerciseID, focus exercise.ExerciseID) (*Program, error) {
	return newProgram(programParams{
		frequency: freq,
		volume:    volume,
		selected:  selected,
		declared:  declared,
		focus:     focus,
	})
}

// newProgram は不変条件を検証して Program を組み立てる。検証はここだけ。
//
// スライスは必ず写してから持つ（normalizeExerciseIDs と normalizeCycle が
// 新しいスライスを作る）。params が中身を写さずに渡せるのはこのため。
func newProgram(x programParams) (*Program, error) {
	if x.frequency.IsZero() {
		return nil, errors.New("週の頻度が設定されていない")
	}
	if x.volume.IsZero() {
		return nil, errors.New("1回の量が設定されていない")
	}
	if len(x.selected) == 0 {
		return nil, errors.New("使用する種目が1つも選ばれていない")
	}
	if len(x.declared) == 0 {
		return nil, ErrNoDeclaredExercise
	}

	selected, err := normalizeExerciseIDs(x.selected)
	if err != nil {
		return nil, fmt.Errorf("選択種目が不正: %w", err)
	}
	declared, err := normalizeExerciseIDs(x.declared)
	if err != nil {
		return nil, fmt.Errorf("伸ばしたい種目が不正: %w", err)
	}

	// declared ⊂ selected であることを確認する。
	for _, id := range declared {
		if !slices.Contains(selected, id) {
			return nil, fmt.Errorf("伸ばしたい種目 %q が選択種目に含まれていない", id)
		}
	}
	// 重点種目が伸ばしたい種目に含まれていることを確認する。
	//
	// declared ⊂ selected なので、これが通れば選択にも含まれる。宣言して
	// いない種目を重点にできると、「伸ばしたい種目の中でさらに重点」という
	// 意味が崩れる。
	//
	// 空は素通しする。指定なしが正当な既定値で、ここで NewExerciseID に
	// 渡すと全プログラムが「種目IDが空である」で落ちる。
	focus := x.focus
	if focus != "" {
		id, err := exercise.NewExerciseID(string(focus))
		if err != nil {
			return nil, fmt.Errorf("重点種目が不正: %w", err)
		}
		if !slices.Contains(declared, id) {
			return nil, fmt.Errorf("重点種目 %q が伸ばしたい種目に含まれていない", id)
		}
		focus = id
	}

	cycle, err := normalizeCycle(x.cycle)
	if err != nil {
		return nil, err
	}

	// 宣言ごとのレップ数は、宣言に含まれる種目だけが持てる。宣言から外す
	// ときの掃除は WithDeclared がする。ここで黙って落とすと、宣言して
	// いない種目に立てた呼び出し（WithRepTargets）が成功に見える。
	reps := make(map[exercise.ExerciseID]RepTargets, len(x.reps))
	for id, r := range x.reps {
		if !slices.Contains(declared, id) {
			return nil, fmt.Errorf("レップ数を持つ %q が伸ばしたい種目に含まれていない", id)
		}
		if r.IsZero() {
			return nil, fmt.Errorf("%q のレップ数が設定されていない", id)
		}
		reps[id] = r
	}

	return &Program{
		frequency: x.frequency,
		volume:    x.volume,
		selected:  selected,
		declared:  declared,
		focus:     focus,
		cycle:     cycle,
		reps:      reps,
	}, nil
}

// params は全フィールドを写す。写す場所はここだけ。
//
// スライスと対応表の中身は写さない。newProgram が必ず新しいスライスと対応表を作るので、
// 元の Program と共有されたまま残ることがない。
func (p *Program) params() programParams {
	return programParams{
		frequency: p.frequency,
		volume:    p.volume,
		selected:  p.selected,
		declared:  p.declared,
		focus:     p.focus,
		cycle:     p.cycle,
		reps:      p.reps,
	}
}

// with は「写して、1つだけ変えて、検証する」を行う。With* は全部ここを通る。
func (p *Program) with(change func(*programParams)) (*Program, error) {
	x := p.params()
	change(&x)
	return newProgram(x)
}

// WithCycle は分割の周期だけを差し替えた新しいプログラムを返す。
//
// 空を渡すと分割なしに戻る。全身法はこれで表す。
//
// 周期のどの位置が今日かは持たない。履歴の出席回数から導くので、
// 集約が「いま何日目か」を覚える必要がない。覚えると、記録を消した
// ときに周期だけが進んだままになる。
func (p *Program) WithCycle(cycle []Split) (*Program, error) {
	return p.with(func(x *programParams) { x.cycle = cycle })
}

// Cycle は分割の周期。空なら分割なし。
func (p *Program) Cycle() []Split {
	out := make([]Split, len(p.cycle))
	copy(out, p.cycle)
	return out
}

// SplitOn はその日の分割を返す。分割なしなら false。
//
// 周期の位置は「これまでの出席回数」で決まる。暦では進めない。
// 休んだ日に周期が飛ぶと、通っていないのに分割だけが回る。
//
// sessionsBefore はその日より前のセッション数。当日は数えない（D-116）。
func (p *Program) SplitOn(sessionsBefore int) (Split, bool) {
	if len(p.cycle) == 0 {
		return Split{}, false
	}
	if sessionsBefore < 0 {
		sessionsBefore = 0
	}
	return p.cycle[sessionsBefore%len(p.cycle)], true
}

// WithFocus は重点種目だけを差し替えた新しいプログラムを返す。元は変えない。
//
// 空の ID を渡すと「指定なし」に戻る。newProgram が空を素通しするので、
// 解除のための分岐は要らない。
//
// with に委譲するのは、focus ⊂ declared の検証を2箇所に書かない
// ため。ここで自前に検査すると、片方だけ直したときに黙ってずれる。
func (p *Program) WithFocus(id exercise.ExerciseID) (*Program, error) {
	return p.with(func(x *programParams) { x.focus = id })
}

// WithDeclared は伸ばしたい種目だけを差し替えた新しいプログラムを返す。
//
// 重点種目が新しい宣言に含まれなくなる場合はエラーになる。黙って解除は
// しない。解除するかどうかは本人が決めることで、宣言を変えた副作用として
// 重点が消えると、次に画面を開くまで気づけない。
//
// 外した種目のレップ数は捨てる。入れ直したら既定に戻る（設計書
// 2026-10-03-declared-rep-targets）。重点と違って黙って捨ててよいのは、
// 外した種目は計画に出ないので、値が残っていても使われないため。
func (p *Program) WithDeclared(ids []exercise.ExerciseID) (*Program, error) {
	return p.with(func(x *programParams) {
		x.declared = ids
		kept := make(map[exercise.ExerciseID]RepTargets, len(x.reps))
		for id, r := range x.reps {
			if slices.Contains(ids, id) {
				kept[id] = r
			}
		}
		x.reps = kept
	})
}

// WithFrequency は週の頻度だけを差し替えた新しいプログラムを返す。
//
// 週目標を道連れにしていたのはやめた（#176）。週目標はもう集約の持ち物
// ではなく、頻度と1回の量から都度導く値なので、頻度を差し替えれば
// 導いた先の値も自動でついてくる。持ち物のときのように別の値を渡して
// 揃え忘れる経路自体が無くなった。
func (p *Program) WithFrequency(freq Frequency) (*Program, error) {
	return p.with(func(x *programParams) { x.frequency = freq })
}

// WithSessionVolume は1回の量だけを差し替えた新しいプログラムを返す。
// 週目標を道連れにしない理由は WithFrequency と同じ。
func (p *Program) WithSessionVolume(volume SessionVolume) (*Program, error) {
	return p.with(func(x *programParams) { x.volume = volume })
}

// WithSelected は使う種目だけを差し替えた新しいプログラムを返す。
//
// 伸ばしたい種目が新しい選択から外れる場合はエラーになる（declared ⊂
// selected）。黙って宣言を削らないのは WithDeclared と同じ理由で、
// 選択を変えた副作用として軸の顔ぶれが変わると気づけない。
//
// 種目がマスタに実在するかはここでは見ない。集約は種目マスタを持たない。
// 確認はユースケースの verifySelection が行う。
func (p *Program) WithSelected(ids []exercise.ExerciseID) (*Program, error) {
	return p.with(func(x *programParams) { x.selected = ids })
}

func (p *Program) Frequency() Frequency         { return p.frequency }
func (p *Program) SessionVolume() SessionVolume { return p.volume }

// normalizeExerciseIDs は種目IDの正規化を行う。重複と存在しない種目はエラーになる。昇順にソートする。
func normalizeExerciseIDs(ids []exercise.ExerciseID) ([]exercise.ExerciseID, error) {
	seen := make(map[exercise.ExerciseID]bool, len(ids))
	out := make([]exercise.ExerciseID, 0, len(ids))
	for _, id := range ids {
		validID, err := exercise.NewExerciseID(string(id))
		if err != nil {
			return nil, err
		}
		if seen[validID] {
			return nil, fmt.Errorf("種目 %q が重複している", validID)
		}
		seen[validID] = true
		out = append(out, validID)
	}

	slices.Sort(out)
	return out, nil
}

func (p *Program) SelectedExercises() []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, len(p.selected))
	copy(out, p.selected)
	return out
}

func (p *Program) DeclaredExercises() []exercise.ExerciseID {
	out := make([]exercise.ExerciseID, len(p.declared))
	copy(out, p.declared)
	return out
}

// FocusExercise は重点種目。指定が無ければ false。
//
// ポインタではなく (値, bool) を返すのは、Weight() と同じ
// 「任意項目」の規約に揃えるため。ポインタだと呼び出し側が nil 判定と
// 逆参照の2手を踏むうえ、集約の内部フィールドのアドレスが外へ出る。
func (p *Program) FocusExercise() (exercise.ExerciseID, bool) {
	return p.focus, p.focus != ""
}

func (p *Program) Includes(id exercise.ExerciseID) bool {
	return slices.Contains(p.selected, id)
}

func (p *Program) Declares(id exercise.ExerciseID) bool {
	return slices.Contains(p.declared, id)
}

// WithRepTargets は1つの宣言のレップ数だけを差し替えた新しいプログラムを返す。
// 宣言していない種目はエラー。
func (p *Program) WithRepTargets(id exercise.ExerciseID, t RepTargets) (*Program, error) {
	return p.with(func(x *programParams) {
		next := make(map[exercise.ExerciseID]RepTargets, len(x.reps)+1)
		for k, v := range x.reps {
			next[k] = v
		}
		next[id] = t
		x.reps = next
	})
}

// RepTargetsFor はその種目を軸で出すときのレップ数。設定していなければ既定。
//
// 宣言していない種目にも既定を返す。計画の側は軸に立った種目の値を引くだけで、
// 宣言かどうかを確かめさせない。
func (p *Program) RepTargetsFor(id exercise.ExerciseID) RepTargets {
	if r, ok := p.reps[id]; ok {
		return r
	}
	return DefaultRepTargets()
}

// DeclaredRepTargets は設定されたレップ数の写し。保存に使う。設定していない
// 宣言は含まない。
func (p *Program) DeclaredRepTargets() map[exercise.ExerciseID]RepTargets {
	out := make(map[exercise.ExerciseID]RepTargets, len(p.reps))
	for k, v := range p.reps {
		out[k] = v
	}
	return out
}
