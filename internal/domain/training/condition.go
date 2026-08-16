package training

import (
	"fmt"
	"math"
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

	// 体重と睡眠の現実的な範囲。Health Connect から取り込む値なので、
	// デバイスの誤作動や単位の取り違えが混ざりうる。
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

// ConditionLog は日次スナップショットの集まり。日付昇順で保持する。
//
// 同じ日付が複数含まれる場合は後のものを採用する。
// リポジトリが日付をキーに上書きする挙動と揃えている。
type ConditionLog struct {
	items []DailyCondition
}

func NewConditionLog(items []DailyCondition) ConditionLog {
	byDate := make(map[Date]DailyCondition, len(items))
	for _, c := range items {
		if c.Date().IsZero() {
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

// ConditionAnalyzer はコンディションから補正を導くドメインサービス。無状態。
type ConditionAnalyzer struct {
	baselineDays      int
	sleepDeficitHours float64
	trendWindowDays   int
}

func NewConditionAnalyzer(baselineDays int, sleepDeficitHours float64, trendWindowDays int) (ConditionAnalyzer, error) {
	if baselineDays < 1 {
		return ConditionAnalyzer{}, fmt.Errorf("基準日数は1以上である必要がある: %d", baselineDays)
	}
	if math.IsNaN(sleepDeficitHours) || sleepDeficitHours <= 0 {
		return ConditionAnalyzer{}, fmt.Errorf("睡眠不足の閾値は正の数である必要がある: %v", sleepDeficitHours)
	}
	if trendWindowDays < 1 {
		return ConditionAnalyzer{}, fmt.Errorf("トレンド窓は1以上である必要がある: %d", trendWindowDays)
	}
	return ConditionAnalyzer{
		baselineDays:      baselineDays,
		sleepDeficitHours: sleepDeficitHours,
		trendWindowDays:   trendWindowDays,
	}, nil
}

func DefaultConditionAnalyzer() ConditionAnalyzer {
	return ConditionAnalyzer{
		baselineDays:      defaultBaselineDays,
		sleepDeficitHours: defaultSleepDeficitHours,
		trendWindowDays:   defaultTrendWindowDays,
	}
}

func (a ConditionAnalyzer) BaselineDays() int          { return a.baselineDays }
func (a ConditionAnalyzer) SleepDeficitHours() float64 { return a.sleepDeficitHours }
func (a ConditionAnalyzer) TrendWindowDays() int       { return a.trendWindowDays }

func (a ConditionAnalyzer) IsZero() bool { return a == ConditionAnalyzer{} }

// RIRAdjustment は睡眠不足の日に目標RIRへ加える補正。
//
// 基準は直近 baselineDays 日の睡眠の中央値。そこから閾値以上短ければ +1。
// データが無い場合は補正しない。推測で軽くしない。
//
// 中央値を使うのは、1日だけの徹夜や計測ミスで基準が下がらないようにするため。
func (a ConditionAnalyzer) RIRAdjustment(log ConditionLog, date Date) int {
	if a.IsZero() || date.IsZero() {
		return 0
	}

	todayCondition, ok := log.On(date)
	if !ok {
		return 0
	}
	todaySleep, ok := todayCondition.SleepHours()
	if !ok {
		return 0
	}

	samples := a.baselineSleep(log, date)
	if len(samples) < minBaselineSamples {
		return 0
	}

	if todaySleep <= median(samples)-a.sleepDeficitHours {
		return 1
	}
	return 0
}

// baselineSleep は基準となる睡眠時間の標本。当日は含めない。
func (a ConditionAnalyzer) baselineSleep(log ConditionLog, date Date) []float64 {
	from := date.AddDays(-a.baselineDays)
	out := make([]float64, 0, a.baselineDays)

	for _, c := range log.items {
		if c.Date().Before(from) || !c.Date().Before(date) {
			continue
		}
		if h, ok := c.SleepHours(); ok {
			out = append(out, h)
		}
	}
	return out
}

// BodyWeightTrendKgPerWeek は体重トレンド（kg/週）。最小二乗法の傾きを週換算する。
//
// 減量中かどうかの判定に使う。データが足りなければ false を返し、
// 呼び出し側は「判定できない」として扱う。
//
// 減量中の停滞とオーバーリーチによる停滞は、トレーニング記録だけ見ると
// 同じ形をしている。この2つを見分けるためだけに体重を取り込んでいる。
func (a ConditionAnalyzer) BodyWeightTrendKgPerWeek(log ConditionLog, date Date) (float64, bool) {
	if a.IsZero() || date.IsZero() {
		return 0, false
	}

	from := date.AddDays(-a.trendWindowDays)
	type point struct{ x, y float64 }
	points := make([]point, 0, a.trendWindowDays)

	for _, c := range log.items {
		if c.Date().Before(from) || c.Date().After(date) {
			continue
		}
		if kg, ok := c.BodyWeightKg(); ok {
			points = append(points, point{x: float64(c.Date().DaysSince(from)), y: kg})
		}
	}
	if len(points) < minTrendSamples {
		return 0, false
	}

	n := float64(len(points))
	var sumX, sumY float64
	for _, p := range points {
		sumX += p.x
		sumY += p.y
	}
	meanX, meanY := sumX/n, sumY/n

	var numerator, denominator float64
	for _, p := range points {
		numerator += (p.x - meanX) * (p.y - meanY)
		denominator += (p.x - meanX) * (p.x - meanX)
	}
	if denominator == 0 {
		// 全サンプルが同じ日。傾きは定義できないが、
		// 「変化していない」とみなす方が呼び出し側にとって扱いやすい。
		return 0, true
	}
	return quantize(numerator / denominator * 7), true
}
