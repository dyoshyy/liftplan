package planning

import (
	"fmt"

	"github.com/dyoshyy/liftplan/internal/domain/training"
	"github.com/dyoshyy/liftplan/internal/domain/training/condition"
)

// 窓の既定値。
const (
	defaultBaselineDays      = 14
	defaultSleepDeficitHours = 1.5

	// 基準を求めるのに最低限必要なサンプル数。
	// これを下回るときは「判定できない」として補正しない。推測で軽くしない。
	minBaselineSamples = 5

	// 窓の上限。設定を外部から与えるようになったとき、桁を間違えた値で
	// 巨大な確保を試みたり演算が破綻したりしないようにする。
	maxWindowDays = 365
)

// ConditionAnalyzer はコンディションから補正を導くドメインサービス。無状態。
type ConditionAnalyzer struct {
	baselineDays      int
	sleepDeficitHours float64
}

func NewConditionAnalyzer(baselineDays int, sleepDeficitHours float64) (ConditionAnalyzer, error) {
	// 窓が最低サンプル数を下回ると、標本が集まらず機能が黙って死ぬ。
	// 「通るのに永久に効かない設定」を作らせない。
	if baselineDays < minBaselineSamples || baselineDays > maxWindowDays {
		return ConditionAnalyzer{}, fmt.Errorf(
			"基準日数は%d〜%dの範囲である必要がある: %d", minBaselineSamples, maxWindowDays, baselineDays)
	}
	if err := training.ValidateRange("睡眠不足の閾値", training.Quantize(sleepDeficitHours), training.SmallestPositive, condition.MaxSleepHours); err != nil {
		return ConditionAnalyzer{}, err
	}
	return ConditionAnalyzer{
		baselineDays:      baselineDays,
		sleepDeficitHours: sleepDeficitHours,
	}, nil
}

func DefaultConditionAnalyzer() ConditionAnalyzer {
	return ConditionAnalyzer{
		baselineDays:      defaultBaselineDays,
		sleepDeficitHours: defaultSleepDeficitHours,
	}
}

func (a ConditionAnalyzer) BaselineDays() int          { return a.baselineDays }
func (a ConditionAnalyzer) SleepDeficitHours() float64 { return a.sleepDeficitHours }

func (a ConditionAnalyzer) IsZero() bool { return a == ConditionAnalyzer{} }

// RIRAdjustment は睡眠不足の日に目標RIRへ加える補正。
//
// 基準は直近 baselineDays 日の睡眠の中央値。そこから閾値以上短ければ +1。
// データが無い場合は補正しない。推測で軽くしない。
//
// 中央値を使うのは、1日だけの徹夜や計測ミスで基準が下がらないようにするため。
func (a ConditionAnalyzer) RIRAdjustment(log condition.ConditionLog, date training.Date) int {
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

	if todaySleep <= training.Median(samples)-a.sleepDeficitHours {
		return 1
	}
	return 0
}

// baselineSleep は基準となる睡眠時間の標本。当日は含めない。
func (a ConditionAnalyzer) baselineSleep(log condition.ConditionLog, date training.Date) []float64 {
	from := date.AddDays(-a.baselineDays)
	out := make([]float64, 0, log.Len())

	for _, c := range log.Items() {
		if c.Date().Before(from) || !c.Date().Before(date) {
			continue
		}
		if h, ok := c.SleepHours(); ok {
			out = append(out, h)
		}
	}
	return out
}
