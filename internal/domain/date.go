package domain

import (
	"cmp"
	"fmt"
	"time"
)

// Date is a calendar day in Japan. It is comparable with ==, works as a map
// key, and prints and parses as "2006-01-02". The zero Date means "unknown".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

const dateLayout = "2006-01-02"

// NewDate returns the date for y-m-d, normalised like time.Date
// (2026-10-32 becomes 2026-11-01).
func NewDate(y int, m time.Month, d int) Date {
	y, m, d = time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Date()
	return Date{y, m, d}
}

// DateOf returns the JST calendar day an instant falls on.
func DateOf(t time.Time) Date {
	y, m, d := t.In(JST).Date()
	return Date{y, m, d}
}

// ParseDate parses "2006-01-02".
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation(dateLayout, s, JST)
	if err != nil {
		return Date{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	return DateOf(t), nil
}

// Time returns midnight JST at the start of the day.
func (d Date) Time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, JST)
}

// AddDays returns the date n days later (or earlier, for negative n).
func (d Date) AddDays(n int) Date {
	return NewDate(d.Year, d.Month, d.Day+n)
}

// Weekday returns the day of the week.
func (d Date) Weekday() time.Weekday { return d.Time().Weekday() }

// Compare returns -1, 0 or +1 as d is before, equal to or after o.
func (d Date) Compare(o Date) int {
	return cmp.Or(cmp.Compare(d.Year, o.Year), cmp.Compare(d.Month, o.Month), cmp.Compare(d.Day, o.Day))
}

// IsZero reports whether d is the zero (unknown) date.
func (d Date) IsZero() bool { return d == Date{} }

func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Time().Format(dateLayout)
}

// MarshalText implements encoding.TextMarshaler, for JSON and TOML.
func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler; empty text is the zero Date.
func (d *Date) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(string(b))
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
