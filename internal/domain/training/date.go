package training

import (
	"errors"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// 表現できる年の範囲。String() と ParseDate が "YYYY-MM-DD" で往復できる範囲に限る。
const (
	minYear = 1
	maxYear = 9999
)

// Date は時刻もタイムゾーンも持たない暦日。不変かつ自己検証。
//
// 内部を年月日の整数で持つ理由は3つある。
//   - 構造体の == が Equal と一致し、マップのキーとして安全に使える
//   - monotonic clock やロケーションが混入する余地が無い
//   - 日数差を time.Duration に頼らないため、±292年で飽和する問題が起きない
//
// ゼロ値は「未設定」を表す無効な日付であり、IsZero で判定できる。
// ゼロ値に対する演算はゼロ値を返す（後述）。受け取る側が IsZero で弾くこと。
type Date struct {
	year  int
	month time.Month
	day   int
}

// NewDate は暦上実在する日付だけを受け付ける。
// 2月30日のような存在しない日付は、正規化せずエラーにする。
func NewDate(year int, month time.Month, day int) (Date, error) {
	if year < minYear || year > maxYear {
		return Date{}, fmt.Errorf("年は%d〜%dの範囲である必要がある: %d", minYear, maxYear, year)
	}
	if month < time.January || month > time.December {
		return Date{}, fmt.Errorf("月が不正: %d", int(month))
	}
	if max := daysInMonth(year, month); day < 1 || day > max {
		return Date{}, fmt.Errorf("%d年%d月は%d日までしかない: %d", year, int(month), max, day)
	}
	return Date{year: year, month: month, day: day}, nil
}

// MustDate は NewDate のうちエラーを panic に変えたもの。
//
// コンパイル時に確定しているリテラル専用である。実行時に決まる値には決して使わない。
// 外部入力は ParseDate か FromTime を通すこと。
// この制約は TestDomain_MustDateIsTestOnly が機械的に検査する。
func MustDate(year int, month time.Month, day int) Date {
	d, err := NewDate(year, month, day)
	if err != nil {
		panic(fmt.Sprintf("MustDate に不正なリテラルが渡された: %v", err))
	}
	return d
}

// ParseDate は "2006-01-02" 形式の文字列を解釈する。
// 書式を検査したあと、不変条件の検証は NewDate に委ねる（検証の実装を一箇所に保つ）。
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("日付として解釈できない %q: %w", s, err)
	}
	d, err := NewDate(t.Year(), t.Month(), t.Day())
	if err != nil {
		return Date{}, fmt.Errorf("日付として解釈できない %q: %w", s, err)
	}
	return d, nil
}

// FromTime は loc における、その瞬間の暦日を返す。
//
// 時刻・monotonic clock・ロケーションはここで落ちる。
// ロケーションを引数で強制するのは、「どのタイムゾーンの今日か」という判断を
// 呼び出し側に明示させるため。暗黙に UTC やローカルを選ぶと、
// JST の早朝トレーニングが前日として記録されるような事故が静かに起きる。
func FromTime(t time.Time, loc *time.Location) (Date, error) {
	if loc == nil {
		return Date{}, errors.New("ロケーションが指定されていない")
	}
	year, month, day := t.In(loc).Date()
	return NewDate(year, month, day)
}

func (d Date) Year() int         { return d.year }
func (d Date) Month() time.Month { return d.month }
func (d Date) Day() int          { return d.day }

// IsZero はゼロ値（未設定）かどうか。
func (d Date) IsZero() bool { return d == Date{} }

// AddDays は n 日後（負なら n 日前）。
//
// ゼロ値に対してはゼロ値を返す。表現範囲を超える場合は端の日付で飽和する。
// 黙って別の値になるより、単調性を保ったまま端に張り付く方が検知しやすい。
func (d Date) AddDays(n int) Date {
	if d.IsZero() {
		return Date{}
	}
	year, month, day := civilFromDays(daysFromCivil(d.year, d.month, d.day) + n)
	if year < minYear {
		return Date{year: minYear, month: time.January, day: 1}
	}
	if year > maxYear {
		return Date{year: maxYear, month: time.December, day: 31}
	}
	return Date{year: year, month: month, day: day}
}

// DaysSince は o から見た経過日数。o より前なら負になる。
// どちらかがゼロ値なら 0 を返す。
func (d Date) DaysSince(o Date) int {
	if d.IsZero() || o.IsZero() {
		return 0
	}
	return daysFromCivil(d.year, d.month, d.day) - daysFromCivil(o.year, o.month, o.day)
}

// Compare は d が o より前なら負、後なら正、同じなら0を返す。
func (d Date) Compare(o Date) int {
	switch {
	case d.year != o.year:
		return d.year - o.year
	case d.month != o.month:
		return int(d.month) - int(o.month)
	default:
		return d.day - o.day
	}
}

func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }
func (d Date) After(o Date) bool  { return d.Compare(o) > 0 }

// Equal は同じ暦日かどうか。構造体の == と常に一致する。
func (d Date) Equal(o Date) bool { return d == o }

// Weekday は曜日。ゼロ値に対しては time.Sunday を返すが、意味は無い。
func (d Date) Weekday() time.Weekday {
	if d.IsZero() {
		return time.Sunday
	}
	// 1970-01-01（通日0）は木曜日。
	w := (daysFromCivil(d.year, d.month, d.day) + int(time.Thursday)) % 7
	if w < 0 {
		w += 7
	}
	return time.Weekday(w)
}

// String は "YYYY-MM-DD"。辞書順が日付順に一致するため、ソートキーに使える。
func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.year, int(d.month), d.day)
}

func daysInMonth(year int, month time.Month) int {
	switch month {
	case time.April, time.June, time.September, time.November:
		return 30
	case time.February:
		if isLeapYear(year) {
			return 29
		}
		return 28
	default:
		return 31
	}
}

func isLeapYear(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// daysFromCivil は 1970-01-01 を0とする通日。
// Howard Hinnant の days_from_civil に基づく。time.Duration を経由しないため飽和しない。
func daysFromCivil(year int, month time.Month, day int) int {
	y := year
	if month <= time.February {
		y--
	}
	era := y
	if y < 0 {
		era = y - 399
	}
	era /= 400

	yoe := y - era*400                     // [0, 399]
	mp := (int(month) + 9) % 12            // 3月を0とする
	doy := (153*mp+2)/5 + day - 1          // [0, 365]
	doe := yoe*365 + yoe/4 - yoe/100 + doy // [0, 146096]
	return era*146097 + doe - 719468
}

// civilFromDays は daysFromCivil の逆変換。
func civilFromDays(z int) (int, time.Month, int) {
	z += 719468
	era := z
	if z < 0 {
		era = z - 146096
	}
	era /= 146097

	doe := z - era*146097                                  // [0, 146096]
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365 // [0, 399]
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100) // [0, 365]
	mp := (5*doy + 2) / 153                  // [0, 11]
	day := doy - (153*mp+2)/5 + 1            // [1, 31]

	month := mp + 3
	if mp >= 10 {
		month = mp - 9
	}
	if month <= 2 {
		y++
	}
	return y, time.Month(month), day
}
