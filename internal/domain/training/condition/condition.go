package condition

import (
	"sort"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

const (
	// 体重と睡眠の現実的な範囲。Health Connect から取り込む値なので、
	// デバイスの誤作動が混ざりうる。
	//
	// なお単位の取り違え（ポンド）はこの範囲では捕まらない。
	// 実在する成人のポンド値（100〜300lb）は全て kg の範囲に収まる。
	minBodyWeightKg = 20
	maxBodyWeightKg = 300
)

// MaxSleepHours は1日の睡眠時間として受け付ける上限。
//
// 解析側（planning）も閾値の検証に使うので公開する。
const MaxSleepHours = 24

// DefaultBodyWeightKg は体重を一度も記録していない利用者に使う既定値。
//
// かつては推定から落として「自分で決める」を出していたが、「何kgでやるか」は
// アプリが答えるべき問いなので既定値を置く。
//
// 実体からずれても出力はほとんど動かない。処方を展開すると
//
//	added = 1.333·I·w + k·B·(1.333·I − 1)
//
// で、補助種目（I = 0.71）なら体重 B の係数は −0.054。20kg ずれても処方は
// 1kg しか動かない。推定と処方の両側で同じ B を使うため打ち消し合う。
// 一度でも記録すれば実測に切り替わるので、誤差は自己修復する。
const DefaultBodyWeightKg = 70

// DailyCondition は Health Connect から取り込んだ日次スナップショット。
//
// 体重と睡眠はどちらも欠損しうる。不変で、With 系は新しい値を返す。
// 範囲外の値は「無かったこと」にする。エラーを返さないのは、
// 1日ぶんの異常値で取り込み全体を止めるより、その日を欠損として
// 扱う方が運用として素直だから。
//
//ddd:aggregate
type DailyCondition struct {
	date          training.Date
	bodyWeightKg  float64
	hasBodyWeight bool
	sleepHours    float64
	hasSleepHours bool
}

func NewDailyCondition(date training.Date) DailyCondition {
	return DailyCondition{date: date}
}

func (c DailyCondition) WithBodyWeight(kg float64) DailyCondition {
	q := training.Quantize(kg)
	if training.ValidateRange("体重", q, minBodyWeightKg, maxBodyWeightKg) != nil {
		return c
	}
	c.bodyWeightKg = q
	c.hasBodyWeight = true
	return c
}

func (c DailyCondition) WithSleepHours(h float64) DailyCondition {
	q := training.Quantize(h)
	if training.ValidateRange("睡眠時間", q, 0, MaxSleepHours) != nil {
		return c
	}
	c.sleepHours = q
	c.hasSleepHours = true
	return c
}

func (c DailyCondition) Date() training.Date { return c.date }

func (c DailyCondition) BodyWeightKg() (float64, bool) { return c.bodyWeightKg, c.hasBodyWeight }
func (c DailyCondition) SleepHours() (float64, bool)   { return c.sleepHours, c.hasSleepHours }

// IsZero はゼロ値（未設定）かどうか。
func (c DailyCondition) IsZero() bool { return c == DailyCondition{} }

// Merge は同じ日の別の記録を取り込んだ新しい値を返す。
//
// フィールド単位で合成するのが要点。レコードごと置き換えると、
// 体重と睡眠が別々のタイミングで届いたとき、後から届いた方が
// 先に届いた方を丸ごと消してしまう。
// Health Connect では睡眠（起床時）と体重（体重計に乗ったとき）が
// 別のデータ型・別のタイミングで入るので、これは例外ではなく通常。
func (c DailyCondition) Merge(other DailyCondition) DailyCondition {
	if other.hasBodyWeight {
		c.bodyWeightKg, c.hasBodyWeight = other.bodyWeightKg, true
	}
	if other.hasSleepHours {
		c.sleepHours, c.hasSleepHours = other.sleepHours, true
	}
	return c
}

// ConditionLog は日次スナップショットの集まり。日付昇順で保持する。
//
// 同じ日付が複数含まれる場合はフィールド単位で合成する。
// レコードごと置き換えると、体重だけの記録が睡眠だけの記録を消してしまう。
type ConditionLog struct {
	items []DailyCondition
}

func NewConditionLog(items []DailyCondition) ConditionLog {
	byDate := make(map[training.Date]DailyCondition, len(items))
	for _, c := range items {
		if c.Date().IsZero() {
			continue
		}
		if prev, dup := byDate[c.Date()]; dup {
			byDate[c.Date()] = prev.Merge(c)
			continue
		}
		byDate[c.Date()] = c
	}

	out := make([]DailyCondition, 0, len(byDate))
	for _, c := range byDate {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date().Before(out[j].Date()) })
	return ConditionLog{items: out}
}

func (l ConditionLog) IsEmpty() bool { return len(l.items) == 0 }

func (l ConditionLog) Len() int { return len(l.items) }

func (l ConditionLog) Items() []DailyCondition {
	out := make([]DailyCondition, len(l.items))
	copy(out, l.items)
	return out
}

// On は指定日のスナップショット。
func (l ConditionLog) On(date training.Date) (DailyCondition, bool) {
	for _, c := range l.items {
		if c.Date().Equal(date) {
			return c, true
		}
	}
	return DailyCondition{}, false
}

// BodyWeightAsOf は date 以前で最も新しい体重。
//
// 鮮度は見ない。古い体重で計算され続ける経路は残るが、推定1RM側の
// 42日判定が先に効いて「自分で決める」になるので破綻しない。
// 必要になってから足す。
func (l ConditionLog) BodyWeightAsOf(date training.Date) (float64, bool) {
	for i := len(l.items) - 1; i >= 0; i-- {
		c := l.items[i]
		if c.Date().After(date) {
			continue // 指定した日付より未来だったらスキップ
		}
		if kg, ok := c.BodyWeightKg(); ok {
			return kg, true
		}
	}
	return 0, false
}

// HasBodyWeight は体重の記録が一件でもあるか。
//
// 「一件も無い」と「その日付より前には無い」を区別するためにある。前者は
// 既定体重で計算し、後者はそのセットを推定から落とす。
func (l ConditionLog) HasBodyWeight() bool {
	for _, c := range l.items {
		if _, ok := c.BodyWeightKg(); ok {
			return true
		}
	}
	return false
}
