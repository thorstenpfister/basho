package domain

import (
	"cmp"
	"fmt"
	"strconv"
	"time"
)

// BashoID identifies a tournament by year and month. Tournaments are held in
// the odd months; basho covers January 1960 onwards. It is comparable with ==
// and prints and parses as "YYYYMM", the form both APIs use. The zero BashoID
// means "unknown" and prints as "".
type BashoID struct {
	Year  int
	Month time.Month
}

// FirstBasho is the earliest tournament basho covers (SPEC §4.1).
var FirstBasho = BashoID{1960, time.January}

// NewBashoID validates and returns the tournament of year y, month m.
func NewBashoID(y int, m time.Month) (BashoID, error) {
	id := BashoID{y, m}
	if m < time.January || m > time.December || m%2 == 0 {
		return BashoID{}, fmt.Errorf("basho %s: tournaments are held in odd months", id)
	}
	if id.Compare(FirstBasho) < 0 {
		return BashoID{}, fmt.Errorf("basho %s: data starts with %s", id, FirstBasho)
	}
	return id, nil
}

// ParseBashoID parses "YYYYMM".
func ParseBashoID(s string) (BashoID, error) {
	if len(s) != 6 {
		return BashoID{}, fmt.Errorf("basho %q: want YYYYMM", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return BashoID{}, fmt.Errorf("basho %q: want YYYYMM", s)
	}
	return NewBashoID(n/100, time.Month(n%100))
}

// IsZero reports whether b is the zero (unknown) BashoID.
func (b BashoID) IsZero() bool { return b == BashoID{} }

func (b BashoID) String() string {
	if b.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d%02d", b.Year, int(b.Month))
}

// Compare returns -1, 0 or +1 as b is before, equal to or after o.
func (b BashoID) Compare(o BashoID) int {
	return cmp.Or(cmp.Compare(b.Year, o.Year), cmp.Compare(b.Month, o.Month))
}

// Next returns the following tournament.
func (b BashoID) Next() BashoID {
	if b.Month == time.November {
		return BashoID{b.Year + 1, time.January}
	}
	return BashoID{b.Year, b.Month + 2}
}

// Prev returns the preceding tournament.
func (b BashoID) Prev() BashoID {
	if b.Month == time.January {
		return BashoID{b.Year - 1, time.November}
	}
	return BashoID{b.Year, b.Month - 2}
}

// MarshalText implements encoding.TextMarshaler.
func (b BashoID) MarshalText() ([]byte, error) { return []byte(b.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler; empty text is the zero BashoID.
func (b *BashoID) UnmarshalText(text []byte) error {
	if len(text) == 0 {
		*b = BashoID{}
		return nil
	}
	id, err := ParseBashoID(string(text))
	if err != nil {
		return err
	}
	*b = id
	return nil
}
