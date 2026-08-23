package training

import (
	"sort"
)

const (
	defaultBaselineDays      = 14
	defaultSleepDeficitHours = 1.5
	defaultTrendWindowDays   = 21

	// 基準や傾きを求めるのに最低限必要なサンプル数。
	// これを下回るときは「判定できない」として補正しない。推測で軽くしない。
	minBaselineSamples = 5
	minTrendSamples    = 5

	// 窓の上限。設定を外部から与えるようになったとき、桁を間違えた値で
	// 巨大な確保を試みたり演算が破綻したりしないようにする。
	maxWindowDays = 365

	// 体重と睡眠の現実的な範囲。Health Connect から取り込む値なので、
	// デバイスの誤作動が混ざりうる。
	//
	// なお単位の取り違え（ポンド）はこの範囲では捕まらない。
	// 実在する成人のポンド値（100〜300lb）は全て kg の範囲に収まる。
	minBodyWeightKg = 20
	maxBodyWeightKg = 300
	maxSleepHours   = 24
)

// DailyCondition は Health Connect から取り込んだ日次スナップショット。
//
// 体重と睡眠はどちらも欠損しうる。不変で、With 系は新しい値を返す。
// 範囲外の値は「無かったこと」にする。エラーを返さないのは、
// 1日ぶんの異常値で取り込み全体を止めるより、その日を欠損として
// 扱う方が運用として素直だから。
//
//ddd:aggregate
type DailyCondition struct {
	date          Date
	bodyWeightKg  float64
	hasBodyWeight bool
	sleepHours    float64
	hasSleepHours bool
}

func NewDailyCondition(date Date) DailyCondition {
	return DailyCondition{date: date}
}

func (c DailyCondition) WithBodyWeight(kg float64) DailyCondition {
	q := quantize(kg)
	if validateRange("体重", q, minBodyWeightKg, maxBodyWeightKg) != nil {
		return c
	}
	c.bodyWeightKg = q
	c.hasBodyWeight = true
	return c
}

func (c DailyCondition) WithSleepHours(h float64) DailyCondition {
	q := quantize(h)
	if validateRange("睡眠時間", q, 0, maxSleepHours) != nil {
		return c
	}
	c.sleepHours = q
	c.hasSleepHours = true
	return c
}

func (c DailyCondition) Date() Date { return c.date }

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
	byDate := make(map[Date]DailyCondition, len(items))
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
func (l ConditionLog) On(date Date) (DailyCondition, bool) {
	for _, c := range l.items {
		if c.Date().Equal(date) {
			return c, true
		}
	}
	return DailyCondition{}, false
}
