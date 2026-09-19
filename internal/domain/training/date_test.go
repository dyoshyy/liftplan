package training_test

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan/internal/domain/training"
)

// baseDay は履歴の起点。8月1日を1日目とする。
func baseDay(day int) training.Date {
	return training.MustDate(2026, time.August, 1).AddDays(day - 1)
}

func TestNewDate_RejectsNonExistentDates(t *testing.T) {
	cases := []struct {
		name  string
		year  int
		month time.Month
		day   int
	}{
		{"2月30日", 2026, time.February, 30},
		{"平年の2月29日", 2026, time.February, 29},
		{"4月31日", 2026, time.April, 31},
		{"0日", 2026, time.August, 0},
		{"13月", 2026, time.Month(13), 1},
		{"0月", 2026, time.Month(0), 1},
		{"0年", 0, time.January, 1},
		{"範囲外の年", 10000, time.January, 1},
		{"負の年", -1, time.January, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := training.NewDate(c.year, c.month, c.day); err == nil {
				t.Errorf("存在しない日付が通ってしまう: %d-%d-%d", c.year, int(c.month), c.day)
			}
		})
	}
}

func TestNewDate_AcceptsValidDates(t *testing.T) {
	cases := []struct {
		name  string
		year  int
		month time.Month
		day   int
	}{
		{"閏年の2月29日", 2024, time.February, 29},
		{"400年周期の閏年", 2000, time.February, 29},
		{"月末", 2026, time.December, 31},
		{"最小年", 1, time.January, 1},
		{"最大年", 9999, time.December, 31},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := training.NewDate(c.year, c.month, c.day); err != nil {
				t.Errorf("正当な日付が弾かれた: %v", err)
			}
		})
	}
}

func TestNewDate_RejectsCentennialNonLeapYear(t *testing.T) {
	// 1900年は4で割れるが100で割れて400で割れないので閏年ではない。
	if _, err := training.NewDate(1900, time.February, 29); err == nil {
		t.Error("1900-02-29 が通ってしまう（閏年判定が 400 年ルールを見ていない）")
	}
}

func TestMustDate_PanicsOnInvalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("不正なリテラルで panic しない")
		}
	}()
	training.MustDate(2026, time.February, 30)
}

// 日付は時刻もタイムゾーンも持たない。この不変条件が壊れると、
// 夏時間のある地域で1日が23時間/25時間になり日数差が狂う。
// 内部表現が年月日の整数であることを、外から観測できる性質として固定する。
func TestDate_CarriesNoTimeOrZone(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skipf("タイムゾーン情報が無い環境: %v", err)
	}
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("タイムゾーン情報が無い環境: %v", err)
	}

	// UTC 正午は JST では同日21時。どちらのロケーションで見ても暦日は同じなので、
	// 生成された値も同一になる（時刻が残っていれば == が壊れる）。
	instant := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	fromUTC, err := training.FromTime(instant, time.UTC)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	fromTokyo, err := training.FromTime(instant, tokyo)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	if fromUTC != fromTokyo {
		t.Errorf("同じ暦日が別の値になった: %v vs %v", fromUTC, fromTokyo)
	}

	// 夏時間の切り替わりを跨いでも、1日は必ず1日として数えられる。
	// 米国の2026年の夏時間開始は3月8日。
	before := training.MustDate(2026, time.March, 7)
	after := training.MustDate(2026, time.March, 14)
	if got := after.DaysSince(before); got != 7 {
		t.Errorf("夏時間開始を跨ぐ日数差が誤り: got %d, want 7", got)
	}

	// 夏時間の切り替わり当日をローカル時刻から作っても、暦日は素直に得られる。
	dstDay := time.Date(2026, time.March, 8, 12, 0, 0, 0, newYork)
	got, err := training.FromTime(dstDay, newYork)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	if want := training.MustDate(2026, time.March, 8); got != want {
		t.Errorf("夏時間開始日が誤り: got %v, want %v", got, want)
	}
}

func TestFromTime_UsesTheGivenLocation(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Skipf("タイムゾーン情報が無い環境: %v", err)
	}

	// UTC の 2026-08-17 21:30 は JST では翌日の 06:30。
	// ジムの朝トレが前日として記録される事故は、この差から生まれる。
	instant := time.Date(2026, time.August, 17, 21, 30, 0, 0, time.UTC)

	inUTC, err := training.FromTime(instant, time.UTC)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	inTokyo, err := training.FromTime(instant, tokyo)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}

	if want := training.MustDate(2026, time.August, 17); inUTC != want {
		t.Errorf("UTC 基準が誤り: got %v, want %v", inUTC, want)
	}
	if want := training.MustDate(2026, time.August, 18); inTokyo != want {
		t.Errorf("JST 基準が誤り: got %v, want %v", inTokyo, want)
	}
}

func TestFromTime_RejectsNilLocation(t *testing.T) {
	if _, err := training.FromTime(time.Now(), nil); err == nil {
		t.Error("ロケーション未指定が通ってしまう")
	}
}

func TestFromTime_DropsMonotonicClock(t *testing.T) {
	// time.Now() は monotonic clock を含む。それが Date に残ると == が壊れる。
	now := time.Now()
	a, err := training.FromTime(now, time.UTC)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	b, err := training.FromTime(now.Round(0), time.UTC)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	if a != b {
		t.Errorf("monotonic clock の有無で値が変わる: %v vs %v", a, b)
	}
}

func TestDate_AddDays(t *testing.T) {
	cases := []struct {
		name string
		from training.Date
		n    int
		want training.Date
	}{
		{"月をまたぐ", training.MustDate(2026, time.August, 31), 1, training.MustDate(2026, time.September, 1)},
		{"月をまたいで戻る", training.MustDate(2026, time.August, 1), -1, training.MustDate(2026, time.July, 31)},
		{"年をまたぐ", training.MustDate(2026, time.December, 31), 1, training.MustDate(2027, time.January, 1)},
		{"年をまたいで戻る", training.MustDate(2027, time.January, 1), -1, training.MustDate(2026, time.December, 31)},
		{"閏日を踏む", training.MustDate(2024, time.February, 28), 1, training.MustDate(2024, time.February, 29)},
		{"平年は閏日を飛ばす", training.MustDate(2026, time.February, 28), 1, training.MustDate(2026, time.March, 1)},
		{"0日後は同じ", training.MustDate(2026, time.August, 16), 0, training.MustDate(2026, time.August, 16)},
		{"1年後", training.MustDate(2026, time.August, 16), 365, training.MustDate(2027, time.August, 16)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.from.AddDays(c.n); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestDate_AddDaysIsReversible(t *testing.T) {
	d := training.MustDate(2026, time.August, 16)
	for n := -800; n <= 800; n++ {
		if got := d.AddDays(n).AddDays(-n); got != d {
			t.Fatalf("%d日の往復で値が変わった: %v", n, got)
		}
	}
}

func TestDate_AddDaysSaturatesAtBoundaries(t *testing.T) {
	// 表現範囲を超えたら端に張り付く。黙って別の値になるより検知しやすい。
	if got, want := training.MustDate(9999, time.December, 31).AddDays(1), training.MustDate(9999, time.December, 31); got != want {
		t.Errorf("上端で飽和しない: got %v", got)
	}
	if got, want := training.MustDate(1, time.January, 1).AddDays(-1), training.MustDate(1, time.January, 1); got != want {
		t.Errorf("下端で飽和しない: got %v", got)
	}
}

func TestDate_DaysSince(t *testing.T) {
	a := training.MustDate(2026, time.August, 10)
	b := training.MustDate(2026, time.August, 17)

	if got := b.DaysSince(a); got != 7 {
		t.Errorf("日数差が誤り: got %d, want 7", got)
	}
	if got := a.DaysSince(b); got != -7 {
		t.Errorf("逆向きの日数差が誤り: got %d, want -7", got)
	}
	if got := a.DaysSince(a); got != 0 {
		t.Errorf("同日の差分が0でない: got %d", got)
	}
}

func TestDate_DaysSinceIsConsistentWithAddDays(t *testing.T) {
	// 1600年から現代まで、うるう年・世紀年をすべて跨いで整合すること。
	base := training.MustDate(1600, time.January, 1)
	for n := 0; n < 200000; n += 7 {
		if got := base.AddDays(n).DaysSince(base); got != n {
			t.Fatalf("%d日後の差分が誤り: got %d", n, got)
		}
	}
}

func TestDate_DaysSinceDoesNotSaturate(t *testing.T) {
	// time.Duration 経由だと ±106751 日（約292年）で飽和する。
	a := training.MustDate(1, time.January, 1)
	b := training.MustDate(9999, time.December, 31)
	if got := b.DaysSince(a); got <= 106751 {
		t.Errorf("日数差が飽和している: got %d", got)
	}
}

func TestDate_WeekdayKnownValues(t *testing.T) {
	cases := []struct {
		date training.Date
		want time.Weekday
	}{
		{training.MustDate(1970, time.January, 1), time.Thursday},
		{training.MustDate(2000, time.January, 1), time.Saturday},
		{training.MustDate(2026, time.August, 16), time.Sunday},
		{training.MustDate(2026, time.August, 17), time.Monday},
		{training.MustDate(1900, time.January, 1), time.Monday},
	}
	for _, c := range cases {
		if got := c.date.Weekday(); got != c.want {
			t.Errorf("%v の曜日が誤り: got %v, want %v", c.date, got, c.want)
		}
	}
}

func TestDate_Ordering(t *testing.T) {
	a := training.MustDate(2026, time.August, 16)
	b := training.MustDate(2026, time.August, 17)

	if !a.Before(b) || !b.After(a) {
		t.Error("前後関係が誤り")
	}
	if a.Before(a) || a.After(a) {
		t.Error("同値に対する Before/After が誤り")
	}
	if a.Compare(b) >= 0 || b.Compare(a) <= 0 || a.Compare(a) != 0 {
		t.Error("Compare が誤り")
	}
}

func TestDate_OrderingAcrossFields(t *testing.T) {
	// 年・月・日それぞれの桁で正しく比較できること。
	cases := []struct{ earlier, later training.Date }{
		{training.MustDate(2025, time.December, 31), training.MustDate(2026, time.January, 1)},
		{training.MustDate(2026, time.January, 31), training.MustDate(2026, time.February, 1)},
		{training.MustDate(2026, time.August, 16), training.MustDate(2026, time.August, 17)},
	}
	for _, c := range cases {
		if !c.earlier.Before(c.later) {
			t.Errorf("%v が %v より前と判定されない", c.earlier, c.later)
		}
	}
}

// == と Equal が食い違わないこと。これが内部表現を整数にした主目的。
func TestDate_EqualMatchesStructEquality(t *testing.T) {
	values := []training.Date{
		training.MustDate(2026, time.August, 16),
		training.MustDate(2026, time.August, 17),
		training.MustDate(2027, time.August, 16),
		{},
	}
	parsed, err := training.ParseDate("2026-08-16")
	if err != nil {
		t.Fatalf("パースに失敗: %v", err)
	}
	fromTime, err := training.FromTime(time.Date(2026, time.August, 16, 23, 59, 59, 0, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	values = append(values, parsed, fromTime)

	for _, a := range values {
		for _, b := range values {
			if a.Equal(b) != (a == b) {
				t.Errorf("Equal と == が食い違う: %v vs %v", a, b)
			}
		}
	}
}

func TestDate_ComparableAsMapKey(t *testing.T) {
	m := map[training.Date]int{}
	m[training.MustDate(2026, time.August, 16)]++
	m[training.MustDate(2026, time.August, 16)]++

	parsed, err := training.ParseDate("2026-08-16")
	if err != nil {
		t.Fatalf("パースに失敗: %v", err)
	}
	m[parsed]++

	fromTime, err := training.FromTime(time.Date(2026, time.August, 16, 12, 34, 56, 789, time.UTC), time.UTC)
	if err != nil {
		t.Fatalf("FromTime に失敗: %v", err)
	}
	m[fromTime]++

	if len(m) != 1 {
		t.Errorf("生成経路によってキーが割れている: %d件 %v", len(m), m)
	}
	if m[training.MustDate(2026, time.August, 16)] != 4 {
		t.Errorf("集計が誤り: %v", m)
	}
}

func TestParseDate(t *testing.T) {
	got, err := training.ParseDate("2026-08-16")
	if err != nil {
		t.Fatalf("パースに失敗: %v", err)
	}
	if want := training.MustDate(2026, time.August, 16); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDate_Invalid(t *testing.T) {
	for _, in := range []string{
		"", "2026/08/16", "2026-8-16", "20260816",
		"2026-08-16T00:00:00Z", "2026-13-01", "2026-02-30", "2026-02-29",
		" 2026-08-16", "2026-08-16 ", "2026-08-16x", "0000-01-01", "10000-01-01",
	} {
		if got, err := training.ParseDate(in); err == nil {
			t.Errorf("不正な形式が通ってしまう: %q → %v", in, got)
		}
	}
}

func TestParseDate_FailureReturnsZeroValue(t *testing.T) {
	// 失敗時に中途半端な値を返すと、呼び出し側が err を見落としたとき静かに壊れる。
	got, err := training.ParseDate("garbage")
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !got.IsZero() {
		t.Errorf("失敗時にゼロ値でない値が返る: %v", got)
	}
}

func TestDate_StringRoundTrips(t *testing.T) {
	if got := training.MustDate(2026, time.August, 6).String(); got != "2026-08-06" {
		t.Errorf("got %q, want %q", got, "2026-08-06")
	}

	d := training.MustDate(1, time.January, 1)
	for i := 0; i < 200000; i += 997 {
		v := d.AddDays(i)
		back, err := training.ParseDate(v.String())
		if err != nil {
			t.Fatalf("%v の往復に失敗: %v", v, err)
		}
		if back != v {
			t.Fatalf("往復で値が変わった: %v → %v", v, back)
		}
	}
}

func TestDate_StringIsSortable(t *testing.T) {
	// 履歴を日付でグループ化する際、文字列キーの辞書順が日付順である必要がある。
	d := training.MustDate(1, time.January, 1)
	prev := d.String()
	for i := 1; i < 200000; i += 331 {
		cur := d.AddDays(i).String()
		if !(prev < cur) {
			t.Fatalf("辞書順が日付順になっていない: %q < %q であるべき", prev, cur)
		}
		prev = cur
	}
}

func TestDate_ZeroValueIsInert(t *testing.T) {
	var zero training.Date

	if !zero.IsZero() {
		t.Error("ゼロ値が IsZero で判定できない")
	}
	if training.MustDate(2026, time.August, 16).IsZero() {
		t.Error("有効な日付が IsZero になっている")
	}

	// ゼロ値に対する演算はゼロ値に閉じる。混入しても別の日付に化けない。
	if got := zero.AddDays(5); !got.IsZero() {
		t.Errorf("ゼロ値の AddDays が有効な日付を返した: %v", got)
	}
	if got := zero.DaysSince(training.MustDate(2026, time.August, 16)); got != 0 {
		t.Errorf("ゼロ値との日数差が0でない: %d", got)
	}
	if got := training.MustDate(2026, time.August, 16).DaysSince(zero); got != 0 {
		t.Errorf("ゼロ値との日数差が0でない: %d", got)
	}
}

func TestDate_ZeroValueIsNotAValidDay(t *testing.T) {
	// ゼロ値の String() が正常な日付に見えると、履歴に混入したとき気づけない。
	var zero training.Date
	if _, err := training.ParseDate(zero.String()); err == nil {
		t.Errorf("ゼロ値の文字列 %q が正当な日付として解釈できてしまう", zero.String())
	}

	// 逆に、最小の正当な日付はゼロ値ではない。
	min, err := training.ParseDate("0001-01-01")
	if err != nil {
		t.Fatalf("0001-01-01 のパースに失敗: %v", err)
	}
	if min.IsZero() {
		t.Error("0001-01-01 が未設定と誤判定される")
	}
}
