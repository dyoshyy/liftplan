package training_test

import (
	"testing"
	"time"

	"github.com/dyoshyy/liftplan-server/internal/domain/training"
)

func TestDate_AddDays(t *testing.T) {
	d := training.NewDate(2026, time.August, 31)
	if got, want := d.AddDays(1), training.NewDate(2026, time.September, 1); !got.Equal(want) {
		t.Errorf("月をまたぐ加算が誤り: got %v, want %v", got, want)
	}
	if got, want := d.AddDays(-31), training.NewDate(2026, time.July, 31); !got.Equal(want) {
		t.Errorf("月をまたぐ減算が誤り: got %v, want %v", got, want)
	}
	if got, want := training.NewDate(2026, time.December, 31).AddDays(1), training.NewDate(2027, time.January, 1); !got.Equal(want) {
		t.Errorf("年をまたぐ加算が誤り: got %v, want %v", got, want)
	}
}

func TestDate_AddDaysAcrossDSTBoundary(t *testing.T) {
	// UTC 固定なので夏時間の影響を受けないこと。
	// ローカルタイムゾーンで実装すると 23時間/25時間の日が生まれ、DaysSince が狂う。
	d := training.NewDate(2026, time.March, 28)
	for i := 0; i < 5; i++ {
		if got := d.AddDays(i).DaysSince(d); got != i {
			t.Errorf("%d日後の差分が誤り: got %d", i, got)
		}
	}
}

func TestDate_DaysSince(t *testing.T) {
	a := training.NewDate(2026, time.August, 10)
	b := training.NewDate(2026, time.August, 17)
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

func TestDate_WeekStartIsMonday(t *testing.T) {
	monday := training.NewDate(2026, time.August, 17)
	if monday.Weekday() != time.Monday {
		t.Fatalf("前提が誤り: 2026-08-17 は %v", monday.Weekday())
	}

	for i := range 7 {
		d := monday.AddDays(i)
		if got := d.WeekStart(); !got.Equal(monday) {
			t.Errorf("%v(%v) の週初が誤り: got %v, want %v", d, d.Weekday(), got, monday)
		}
	}

	next := monday.AddDays(7)
	if got := next.WeekStart(); !got.Equal(next) {
		t.Errorf("翌週の週初が誤り: got %v, want %v", got, next)
	}
}

func TestDate_WeekStartIsIdempotent(t *testing.T) {
	d := training.NewDate(2026, time.August, 20)
	once := d.WeekStart()
	if twice := once.WeekStart(); !once.Equal(twice) {
		t.Errorf("週初は冪等であるべき: %v vs %v", once, twice)
	}
}

func TestDate_Ordering(t *testing.T) {
	a := training.NewDate(2026, time.August, 16)
	b := training.NewDate(2026, time.August, 17)

	if !a.Before(b) {
		t.Error("Before が誤り")
	}
	if !b.After(a) {
		t.Error("After が誤り")
	}
	if a.Equal(b) {
		t.Error("Equal が誤り")
	}
	if a.Before(a) || a.After(a) {
		t.Error("同値に対する Before/After が誤り")
	}
	if !a.Equal(training.NewDate(2026, time.August, 16)) {
		t.Error("同じ日付が等しくない")
	}
}

func TestDate_ComparableAsMapKey(t *testing.T) {
	// 内部に time.Time を持つため、== 比較とマップキーの挙動を保証しておく。
	// time.Time は wall clock / monotonic / location を含むので、正規化されていないと壊れる。
	m := map[training.Date]int{}
	m[training.NewDate(2026, time.August, 16)]++
	m[training.NewDate(2026, time.August, 16)]++

	if len(m) != 1 {
		t.Errorf("同じ日付が別キーになっている: %d件", len(m))
	}

	parsed, err := training.ParseDate("2026-08-16")
	if err != nil {
		t.Fatalf("パースに失敗: %v", err)
	}
	m[parsed]++
	if len(m) != 1 {
		t.Errorf("パース由来の日付が別キーになっている: %d件", len(m))
	}
}

func TestParseDate(t *testing.T) {
	got, err := training.ParseDate("2026-08-16")
	if err != nil {
		t.Fatalf("パースに失敗: %v", err)
	}
	if want := training.NewDate(2026, time.August, 16); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseDate_Invalid(t *testing.T) {
	for _, in := range []string{
		"", "2026/08/16", "2026-8-16", "20260816",
		"2026-08-16T00:00:00Z", "2026-13-01", "2026-02-30", " 2026-08-16",
	} {
		if _, err := training.ParseDate(in); err == nil {
			t.Errorf("不正な形式が通ってしまう: %q", in)
		}
	}
}

func TestDate_StringRoundTrips(t *testing.T) {
	d := training.NewDate(2026, time.August, 6)
	if got := d.String(); got != "2026-08-06" {
		t.Errorf("got %q, want %q", got, "2026-08-06")
	}

	back, err := training.ParseDate(d.String())
	if err != nil {
		t.Fatalf("往復に失敗: %v", err)
	}
	if !back.Equal(d) {
		t.Errorf("往復で値が変わった: %v → %v", d, back)
	}
}

func TestDate_StringIsSortable(t *testing.T) {
	// 履歴のグループ化で文字列キーを日付順に並べるため、辞書順＝日付順である必要がある。
	a := training.NewDate(2026, time.August, 9).String()
	b := training.NewDate(2026, time.August, 10).String()
	if !(a < b) {
		t.Errorf("辞書順が日付順になっていない: %q < %q であるべき", a, b)
	}
}

func TestDate_IsZero(t *testing.T) {
	var zero training.Date
	if !zero.IsZero() {
		t.Error("ゼロ値が IsZero で判定できない")
	}
	if training.NewDate(2026, time.August, 16).IsZero() {
		t.Error("有効な日付が IsZero になっている")
	}
}

func TestDate_NormalizesOutOfRangeInput(t *testing.T) {
	// time.Date と同じ正規化に従う。0月32日のような入力を黙って受けるが、
	// 生成元は必ず ParseDate か明示的な定数なので実害は無い。挙動を固定しておく。
	if got, want := training.NewDate(2026, time.August, 32), training.NewDate(2026, time.September, 1); !got.Equal(want) {
		t.Errorf("正規化の挙動が変わった: got %v, want %v", got, want)
	}
}
