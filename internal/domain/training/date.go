package training

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date は時刻を持たない日付。不変。
// トレーニングの記録に必要なのは日付だけなので、時刻とタイムゾーンを閉じ込める。
type Date struct {
	t time.Time
}

// NewDate は UTC の午前0時に正規化した日付を返す。
func NewDate(year int, month time.Month, day int) Date {
	return Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate は "2006-01-02" 形式の文字列を解釈する。
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("日付として解釈できない %q: %w", s, err)
	}
	return Date{t: t.UTC()}, nil
}

func (d Date) AddDays(n int) Date { return Date{t: d.t.AddDate(0, 0, n)} }

func (d Date) Before(o Date) bool { return d.t.Before(o.t) }
func (d Date) After(o Date) bool  { return d.t.After(o.t) }
func (d Date) Equal(o Date) bool  { return d.t.Equal(o.t) }

// DaysSince は o から見た経過日数。o より前なら負になる。
func (d Date) DaysSince(o Date) int {
	return int(d.t.Sub(o.t).Hours() / 24)
}

// Weekday は曜日。週初の算出とテストの前提確認に使う。
func (d Date) Weekday() time.Weekday { return d.t.Weekday() }

// WeekStart はその日が属する週の月曜日を返す。
//
// 週の開始を月曜に固定するのは、セッションが週内で何本目かを数えるため。
// 日曜開始にすると土日のトレーニングが別の週に割れる。
func (d Date) WeekStart() Date {
	offset := (int(d.t.Weekday()) + 6) % 7 // 月曜を0にする
	return d.AddDays(-offset)
}

func (d Date) String() string { return d.t.Format(dateLayout) }

func (d Date) IsZero() bool { return d.t.IsZero() }
