package training_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

// condDate は起点（8月1日）から daysAgo 日前。
func condDate(daysAgo int) training.Date { return baseDay(30).AddDays(-daysAgo) }

func sleepLog(days int, hours func(daysAgo int) float64) training.ConditionLog {
	items := make([]training.DailyCondition, 0, days)
	for i := range days {
		items = append(items, training.NewDailyCondition(condDate(i)).WithSleepHours(hours(i)))
	}
	return training.NewConditionLog(items)
}

func weightLog(days int, kg func(daysAgo int) float64) training.ConditionLog {
	items := make([]training.DailyCondition, 0, days)
	for i := range days {
		items = append(items, training.NewDailyCondition(condDate(i)).WithBodyWeight(kg(i)))
	}
	return training.NewConditionLog(items)
}

func TestDailyCondition_OptionalFields(t *testing.T) {
	c := training.NewDailyCondition(condDate(0))
	if _, ok := c.BodyWeightKg(); ok {
		t.Error("未設定の体重が取れてしまう")
	}
	if _, ok := c.SleepHours(); ok {
		t.Error("未設定の睡眠が取れてしまう")
	}

	c2 := c.WithBodyWeight(75).WithSleepHours(7)
	if v, ok := c2.BodyWeightKg(); !ok || v != 75 {
		t.Errorf("体重が誤り: %v %v", v, ok)
	}
	if v, ok := c2.SleepHours(); !ok || v != 7 {
		t.Errorf("睡眠が誤り: %v %v", v, ok)
	}

	// With 系は元の値を書き換えない。
	if _, ok := c.BodyWeightKg(); ok {
		t.Error("元の値が書き換わっている")
	}
}

// 範囲外の値は「無かったこと」にする。
// Health Connect からの取り込みなので、デバイスの誤作動や単位の取り違えが混ざる。
func TestDailyCondition_RejectsOutOfRangeValues(t *testing.T) {
	c := training.NewDailyCondition(condDate(0))

	for _, kg := range []float64{0, -5, 19, 301, 1000, math.NaN(), math.Inf(1)} {
		if _, ok := c.WithBodyWeight(kg).BodyWeightKg(); ok {
			t.Errorf("範囲外の体重が通ってしまう: %v", kg)
		}
	}
	for _, kg := range []float64{20, 75.5, 300} {
		if _, ok := c.WithBodyWeight(kg).BodyWeightKg(); !ok {
			t.Errorf("正当な体重が弾かれた: %v", kg)
		}
	}

	for _, h := range []float64{-1, 24.1, 100, math.NaN(), math.Inf(1)} {
		if _, ok := c.WithSleepHours(h).SleepHours(); ok {
			t.Errorf("範囲外の睡眠時間が通ってしまう: %v", h)
		}
	}
	// 徹夜は0時間として正当。
	for _, h := range []float64{0, 6.5, 24} {
		if _, ok := c.WithSleepHours(h).SleepHours(); !ok {
			t.Errorf("正当な睡眠時間が弾かれた: %v", h)
		}
	}
}

func TestDailyCondition_QuantizesValues(t *testing.T) {
	c := training.NewDailyCondition(condDate(0)).
		WithBodyWeight(75.12345678901).
		WithSleepHours(7.12345678901)

	kg, _ := c.BodyWeightKg()
	if n := decimalPlaces(strconv.FormatFloat(kg, 'f', -1, 64)); n > 6 {
		t.Errorf("体重に端数が残っている: %v", kg)
	}
	h, _ := c.SleepHours()
	if n := decimalPlaces(strconv.FormatFloat(h, 'f', -1, 64)); n > 6 {
		t.Errorf("睡眠時間に端数が残っている: %v", h)
	}
}

func TestNewConditionLog_SortsAndDeduplicates(t *testing.T) {
	log := training.NewConditionLog([]training.DailyCondition{
		training.NewDailyCondition(condDate(0)).WithBodyWeight(75),
		training.NewDailyCondition(condDate(5)).WithBodyWeight(76),
		training.NewDailyCondition(condDate(0)).WithBodyWeight(74), // 同じ日の再送
		training.NewDailyCondition(training.Date{}).WithBodyWeight(99),
	})

	if log.Len() != 2 {
		t.Fatalf("重複とゼロ値が除かれていない: %d", log.Len())
	}

	items := log.Items()
	if !items[0].Date().Before(items[1].Date()) {
		t.Error("日付昇順になっていない")
	}

	// 後のものを採用する。
	got, ok := log.On(condDate(0))
	if !ok {
		t.Fatal("当日の記録が取れない")
	}
	if kg, _ := got.BodyWeightKg(); kg != 74 {
		t.Errorf("後の記録が採用されていない: %v", kg)
	}
}

func TestConditionLog_ItemsIsDefensivelyCopied(t *testing.T) {
	log := sleepLog(3, func(int) float64 { return 7 })

	items := log.Items()
	items[0] = training.DailyCondition{}
	if log.Items()[0].IsZero() {
		t.Error("返り値の書き換えが内部状態に波及している")
	}
}

func TestConditionLog_ZeroValue(t *testing.T) {
	var zero training.ConditionLog
	if !zero.IsEmpty() || zero.Len() != 0 {
		t.Error("ゼロ値が空でない")
	}
	if _, ok := zero.On(condDate(0)); ok {
		t.Error("ゼロ値から値が取れる")
	}
}

func TestConditionAnalyzer_NoAdjustmentWhenNormal(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	if got := a.RIRAdjustment(sleepLog(15, func(int) float64 { return 7 }), condDate(0)); got != 0 {
		t.Errorf("平常時に補正が入っている: %d", got)
	}
}

func TestConditionAnalyzer_AdjustsWhenSleepDeprived(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := sleepLog(15, func(daysAgo int) float64 {
		if daysAgo == 0 {
			return 4.5
		}
		return 7
	})
	if got := a.RIRAdjustment(log, condDate(0)); got != 1 {
		t.Errorf("睡眠不足で補正されていない: %d", got)
	}
}

// 閾値の境界。ちょうど閾値ぶん短ければ補正する。
func TestConditionAnalyzer_SleepDeficitBoundary(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	cases := []struct {
		name  string
		today float64
		want  int
	}{
		{"閾値ちょうど", 5.5, 1},
		{"閾値をわずかに超える不足", 5.4, 1},
		{"閾値にわずかに足りない不足", 5.6, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			log := sleepLog(15, func(daysAgo int) float64 {
				if daysAgo == 0 {
					return c.today
				}
				return 7 // 中央値7、閾値1.5 なので境界は 5.5
			})
			if got := a.RIRAdjustment(log, condDate(0)); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}

// 基準は中央値。徹夜が数日あっても基準が下がらないこと。
//
// 平均だと外れ値に引きずられて基準が下がり、本当に睡眠不足の日に
// 補正が入らなくなる。
func TestConditionAnalyzer_BaselineResistsOutliers(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	// 過去14日のうち2日が徹夜（0時間）、残りは7時間。
	// 中央値は7なので閾値は5.5。平均は6.0なので閾値は4.5。
	log := sleepLog(15, func(daysAgo int) float64 {
		switch daysAgo {
		case 0:
			return 5.0 // 今日は不足
		case 3, 7:
			return 0 // 過去の徹夜
		default:
			return 7
		}
	})

	if got := a.RIRAdjustment(log, condDate(0)); got != 1 {
		t.Errorf("過去の外れ値で基準が下がっている: %d", got)
	}
}

func TestConditionAnalyzer_NoAdjustmentWithoutData(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	// 当日のデータが無い。
	items := make([]training.DailyCondition, 0, 14)
	for i := 1; i <= 14; i++ {
		items = append(items, training.NewDailyCondition(condDate(i)).WithSleepHours(7))
	}
	if got := a.RIRAdjustment(training.NewConditionLog(items), condDate(0)); got != 0 {
		t.Errorf("当日データが無いのに補正された: %d", got)
	}

	// 基準を作るだけの履歴が無い。
	if got := a.RIRAdjustment(sleepLog(1, func(int) float64 { return 4 }), condDate(0)); got != 0 {
		t.Errorf("基準が無いのに補正された: %d", got)
	}

	// 体重だけあって睡眠が無い。
	if got := a.RIRAdjustment(weightLog(15, func(int) float64 { return 75 }), condDate(0)); got != 0 {
		t.Errorf("睡眠データが無いのに補正された: %d", got)
	}
}

// 基準は当日より前の記録だけで作ること。
//
// 当日を含めると、今日の睡眠が基準の中央値を引き下げ、
// 本当に不足している日でも補正が入らなくなる。
func TestConditionAnalyzer_BaselineExcludesToday(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	// 過去5日は [4,4,8,8,8]、中央値は8なので閾値は6.5。今日は6時間なので補正される。
	// 当日を基準に含めると [4,4,6,8,8,8] で中央値7、閾値5.5となり補正されなくなる。
	past := []float64{8, 8, 8, 4, 4}
	log := sleepLog(6, func(daysAgo int) float64 {
		if daysAgo == 0 {
			return 6
		}
		return past[daysAgo-1]
	})

	if got := a.RIRAdjustment(log, condDate(0)); got != 1 {
		t.Errorf("当日が基準に混ざっている: %d", got)
	}
}

// 当日の記録はあるが睡眠が欠損している場合は補正しないこと。
//
// 欠損を0時間として扱うと、体重だけ取り込めた日に必ず補正が入る。
func TestConditionAnalyzer_NoAdjustmentWhenTodaysSleepIsMissing(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	items := make([]training.DailyCondition, 0, 15)
	for i := 1; i <= 14; i++ {
		items = append(items, training.NewDailyCondition(condDate(i)).WithSleepHours(7))
	}
	// 当日は体重だけ。
	items = append(items, training.NewDailyCondition(condDate(0)).WithBodyWeight(75))

	if got := a.RIRAdjustment(training.NewConditionLog(items), condDate(0)); got != 0 {
		t.Errorf("睡眠が欠損しているのに補正された: %d", got)
	}
}

func TestConditionAnalyzer_TrendDetectsCutting(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	// 過去ほど重い＝減量中。
	log := weightLog(21, func(daysAgo int) float64 { return 75 + float64(daysAgo)*0.05 })

	got, ok := a.BodyWeightTrendKgPerWeek(log, condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if got >= -0.1 {
		t.Errorf("減量中と判定されていない: %v", got)
	}
	// 1日 -0.05kg なら週 -0.35kg。
	if math.Abs(got-(-0.35)) > 0.01 {
		t.Errorf("傾きが誤り: %v", got)
	}
}

func TestConditionAnalyzer_TrendDetectsBulking(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := weightLog(21, func(daysAgo int) float64 { return 75 - float64(daysAgo)*0.05 })

	got, ok := a.BodyWeightTrendKgPerWeek(log, condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if got <= 0.1 {
		t.Errorf("増量中と判定されていない: %v", got)
	}
}

func TestConditionAnalyzer_TrendFlat(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	got, ok := a.BodyWeightTrendKgPerWeek(weightLog(21, func(int) float64 { return 75 }), condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if math.Abs(got) > 0.01 {
		t.Errorf("横ばいと判定されていない: %v", got)
	}
}

func TestConditionAnalyzer_TrendNeedsEnoughSamples(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	for _, days := range []int{0, 1, 4} {
		if _, ok := a.BodyWeightTrendKgPerWeek(weightLog(days, func(int) float64 { return 75 }), condDate(0)); ok {
			t.Errorf("%d日ぶんのサンプルでトレンドが返る", days)
		}
	}
	if _, ok := a.BodyWeightTrendKgPerWeek(weightLog(5, func(int) float64 { return 75 }), condDate(0)); !ok {
		t.Error("最低サンプル数でトレンドが取れない")
	}
}

func TestConditionAnalyzer_TrendIgnoresMissingWeight(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	if _, ok := a.BodyWeightTrendKgPerWeek(sleepLog(21, func(int) float64 { return 7 }), condDate(0)); ok {
		t.Error("体重が無いのにトレンドが返る")
	}
}

// 窓の外の記録を使わないこと。
// 使うと、2ヶ月前の減量期の傾きが今の判定に混ざる。
func TestConditionAnalyzer_TrendRespectsTheWindow(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	// 直近21日は横ばい、それ以前は急激に減っている。
	log := weightLog(60, func(daysAgo int) float64 {
		if daysAgo <= 21 {
			return 75
		}
		return 75 + float64(daysAgo-21)*0.5
	})

	got, ok := a.BodyWeightTrendKgPerWeek(log, condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if math.Abs(got) > 0.05 {
		t.Errorf("窓の外の記録が混ざっている: %v", got)
	}
}

// 未来の記録を使わないこと。
func TestConditionAnalyzer_TrendIgnoresFutureRecords(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	items := make([]training.DailyCondition, 0, 30)
	for i := range 21 {
		items = append(items, training.NewDailyCondition(condDate(i)).WithBodyWeight(75))
	}
	// 基準日より後に急激な増加を置く。
	for i := 1; i <= 5; i++ {
		items = append(items, training.NewDailyCondition(condDate(-i)).WithBodyWeight(75+float64(i)*2))
	}

	got, ok := a.BodyWeightTrendKgPerWeek(training.NewConditionLog(items), condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if math.Abs(got) > 0.05 {
		t.Errorf("未来の記録が混ざっている: %v", got)
	}
}

func TestConditionAnalyzer_TrendIsQuantized(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	log := weightLog(21, func(daysAgo int) float64 { return 75 + float64(daysAgo)*0.037 })

	got, _ := a.BodyWeightTrendKgPerWeek(log, condDate(0))
	if n := decimalPlaces(strconv.FormatFloat(got, 'f', -1, 64)); n > 6 {
		t.Errorf("端数が残っている: %v", got)
	}
}

func TestNewConditionAnalyzer_RejectsBadParams(t *testing.T) {
	cases := []struct {
		baselineDays      int
		sleepDeficitHours float64
		trendWindowDays   int
	}{
		{0, 1.5, 21}, {-1, 1.5, 21},
		{14, 0, 21}, {14, -1, 21}, {14, math.NaN(), 21}, {14, math.Inf(1), 21},
		{14, 25, 21}, // 睡眠不足の閾値が1日を超える
		{14, 1.5, 0}, {14, 1.5, -1},
		// 窓が最低サンプル数を下回る設定は、通っても機能が黙って死ぬ。
		{4, 1.5, 21}, {14, 1.5, 4},
		// 桁を間違えた設定で巨大な確保を試みない。
		{366, 1.5, 21}, {14, 1.5, 366}, {1 << 30, 1.5, 21},
	}
	for _, c := range cases {
		if _, err := training.NewConditionAnalyzer(c.baselineDays, c.sleepDeficitHours, c.trendWindowDays); err == nil {
			t.Errorf("不正なパラメータが通ってしまう: %+v", c)
		}
	}
	for _, c := range []struct {
		baselineDays      int
		sleepDeficitHours float64
		trendWindowDays   int
	}{
		{5, 0.1, 5}, {365, 24, 365},
	} {
		if _, err := training.NewConditionAnalyzer(c.baselineDays, c.sleepDeficitHours, c.trendWindowDays); err != nil {
			t.Errorf("境界値が弾かれた: %+v (%v)", c, err)
		}
	}
}

func TestDefaultConditionAnalyzer_Constants(t *testing.T) {
	a := training.DefaultConditionAnalyzer()
	if a.BaselineDays() != 14 {
		t.Errorf("基準日数が誤り: %d", a.BaselineDays())
	}
	if math.Abs(a.SleepDeficitHours()-1.5) > 1e-9 {
		t.Errorf("睡眠不足の閾値が誤り: %v", a.SleepDeficitHours())
	}
	if a.TrendWindowDays() != 21 {
		t.Errorf("トレンド窓が誤り: %d", a.TrendWindowDays())
	}
}

func TestConditionAnalyzer_ZeroValueIsSafe(t *testing.T) {
	var a training.ConditionAnalyzer
	log := sleepLog(15, func(int) float64 { return 7 })

	if got := a.RIRAdjustment(log, condDate(0)); got != 0 {
		t.Errorf("ゼロ値の分析器が補正した: %d", got)
	}
	if _, ok := a.BodyWeightTrendKgPerWeek(weightLog(21, func(int) float64 { return 75 }), condDate(0)); ok {
		t.Error("ゼロ値の分析器がトレンドを返した")
	}
}

func TestConditionAnalyzer_ZeroDateIsSafe(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	if got := a.RIRAdjustment(sleepLog(15, func(int) float64 { return 7 }), training.Date{}); got != 0 {
		t.Errorf("基準日が無いのに補正された: %d", got)
	}
	if _, ok := a.BodyWeightTrendKgPerWeek(weightLog(21, func(int) float64 { return 75 }), training.Date{}); ok {
		t.Error("基準日が無いのにトレンドが返る")
	}
}

// 同じ日の記録はフィールド単位で合成すること。
//
// レコードごと置き換えると、体重だけの記録が睡眠だけの記録を消す。
// Health Connect では睡眠（起床時）と体重（体重計に乗ったとき）が
// 別のデータ型・別のタイミングで入るので、これは通常の経路。
func TestNewConditionLog_MergesPartialRecordsOfTheSameDay(t *testing.T) {
	date := condDate(0)
	log := training.NewConditionLog([]training.DailyCondition{
		training.NewDailyCondition(date).WithSleepHours(4),
		training.NewDailyCondition(date).WithBodyWeight(75),
	})

	got, ok := log.On(date)
	if !ok {
		t.Fatal("記録が取れない")
	}
	if h, ok := got.SleepHours(); !ok || h != 4 {
		t.Errorf("睡眠が消えている: %v %v", h, ok)
	}
	if kg, ok := got.BodyWeightKg(); !ok || kg != 75 {
		t.Errorf("体重が消えている: %v %v", kg, ok)
	}

	// 同じフィールドが二度来たら後のものを採用する。
	log2 := training.NewConditionLog([]training.DailyCondition{
		training.NewDailyCondition(date).WithBodyWeight(75),
		training.NewDailyCondition(date).WithBodyWeight(74),
	})
	got2, _ := log2.On(date)
	if kg, _ := got2.BodyWeightKg(); kg != 74 {
		t.Errorf("後の記録が採用されていない: %v", kg)
	}
}

// 部分レコードの合成が RIR 補正を生かすこと。
func TestConditionAnalyzer_AdjustsWhenSleepArrivesSeparately(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	items := make([]training.DailyCondition, 0, 16)
	for i := 1; i <= 14; i++ {
		items = append(items, training.NewDailyCondition(condDate(i)).WithSleepHours(7))
	}
	// 当日は睡眠が先、体重が後から届く。
	items = append(items,
		training.NewDailyCondition(condDate(0)).WithSleepHours(4),
		training.NewDailyCondition(condDate(0)).WithBodyWeight(75),
	)

	if got := a.RIRAdjustment(training.NewConditionLog(items), condDate(0)); got != 1 {
		t.Errorf("後着の体重が睡眠を消している: %d", got)
	}
}

// 体重の傾きが計測ノイズで反転しないこと。
//
// 最小二乗法だと、食後や着衣による 0.4kg 程度のずれ1点で傾きが
// 0.1kg/週 以上動き、「減量中かどうか」の判定が反転する。
// 減量中と誤判定されると、本物のオーバーリーチによる停滞で
// デロードが永久に出なくなる。
func TestConditionAnalyzer_TrendResistsMeasurementNoise(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	// 体重は完全に横ばい。週2回計測で、最も古い1点だけ +0.4kg。
	items := []training.DailyCondition{}
	for _, daysAgo := range []int{0, 5, 10, 15, 20} {
		kg := 75.0
		if daysAgo == 20 {
			kg = 75.4
		}
		items = append(items, training.NewDailyCondition(condDate(daysAgo)).WithBodyWeight(kg))
	}

	got, ok := a.BodyWeightTrendKgPerWeek(training.NewConditionLog(items), condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if got < -0.1 {
		t.Errorf("計測ノイズで減量中と誤判定される: %v kg/週", got)
	}
}

// 本当に減量しているとき、当日1点のノイズで判定が消えないこと。
func TestConditionAnalyzer_TrendSurvivesASingleOutlier(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	// 週 -0.3kg で減量中。今日だけ服を着たまま測って +1kg。
	items := []training.DailyCondition{}
	for daysAgo := 0; daysAgo <= 20; daysAgo++ {
		kg := 75 + float64(daysAgo)*0.3/7
		if daysAgo == 0 {
			kg += 1
		}
		items = append(items, training.NewDailyCondition(condDate(daysAgo)).WithBodyWeight(kg))
	}

	got, ok := a.BodyWeightTrendKgPerWeek(training.NewConditionLog(items), condDate(0))
	if !ok {
		t.Fatal("トレンドが取れない")
	}
	if got >= -0.1 {
		t.Errorf("1点の外れ値で減量判定が消えた: %v kg/週", got)
	}
}

// 基準日数が実際に窓幅として使われていること。
// ゲッターの値だけ見ても、窓に使われているかは分からない。
func TestConditionAnalyzer_BaselineDaysIsUsedAsTheWindow(t *testing.T) {
	narrow, err := training.NewConditionAnalyzer(5, 1.5, 21)
	if err != nil {
		t.Fatalf("NewConditionAnalyzer: %v", err)
	}
	wide := training.DefaultConditionAnalyzer() // 14日

	// 直近5日は4時間、それより前は8時間。今日は3時間。
	// 5日窓の中央値は4なので閾値2.5、補正されない。
	// 14日窓の中央値は8なので閾値6.5、補正される。
	log := sleepLog(15, func(daysAgo int) float64 {
		switch {
		case daysAgo == 0:
			return 3
		case daysAgo <= 5:
			return 4
		default:
			return 8
		}
	})

	if got := narrow.RIRAdjustment(log, condDate(0)); got != 0 {
		t.Errorf("5日窓が広く取られている: %d", got)
	}
	if got := wide.RIRAdjustment(log, condDate(0)); got != 1 {
		t.Errorf("14日窓が狭く取られている: %d", got)
	}
}

// トレンド窓はちょうど N 日ぶんであること。
//
// 下限と上限の扱いが非対称だと、同じ設定で21日窓と22日窓が混在し、
// 「先週と同じデータのはずなのに傾きが違う」という再現しにくい挙動になる。
//
// 窓の内外は、サンプル数が最低値に届くかどうかで判定する。
// Theil-Sen は外れ値に強いので、値の変化では境界を検出できない。
func TestConditionAnalyzer_TrendWindowIsExactlyNDays(t *testing.T) {
	a := training.DefaultConditionAnalyzer()

	// 窓の内側に4点だけ置く。最低サンプル数は5なので、
	// 5点目が窓に入るかどうかで ok が切り替わる。
	build := func(fifthDaysAgo int) training.ConditionLog {
		items := []training.DailyCondition{}
		for _, daysAgo := range []int{0, 5, 10, 15} {
			items = append(items, training.NewDailyCondition(condDate(daysAgo)).WithBodyWeight(75))
		}
		items = append(items, training.NewDailyCondition(condDate(fifthDaysAgo)).WithBodyWeight(75))
		return training.NewConditionLog(items)
	}

	// 既定の窓は21日。0〜20日前が内側。
	if _, ok := a.BodyWeightTrendKgPerWeek(build(20), condDate(0)); !ok {
		t.Error("20日前が窓の外になっている")
	}
	if _, ok := a.BodyWeightTrendKgPerWeek(build(21), condDate(0)); ok {
		t.Error("21日前が窓の内側になっている。窓が22日ぶんある")
	}
}
