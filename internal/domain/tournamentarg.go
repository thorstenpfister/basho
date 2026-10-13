package domain

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseTournament parses a tournament argument (SPEC D18): "202609",
// "2026-09", "aki2026", "aki-2026" (any case), or "current", "prev", "next"
// relative to the most recent tournament as of today.
func ParseTournament(s string, today Date) (BashoID, error) {
	in := strings.ToLower(strings.TrimSpace(s))
	switch in {
	case "current":
		return MostRecentTournament(today), nil
	case "prev":
		return MostRecentTournament(today).Prev(), nil
	case "next":
		return MostRecentTournament(today).Next(), nil
	}
	if len(in) == 7 && in[4] == '-' {
		in = in[:4] + in[5:] // 2026-09
	}
	if len(in) == 6 {
		return ParseBashoID(in)
	}
	// "aki2026", "aki-2026": a tournament name, then the year.
	bad := fmt.Errorf("tournament %q: use 202609, 2026-09, aki2026, current, prev or next", s)
	if len(in) < 5 {
		return BashoID{}, bad
	}
	year, err := strconv.Atoi(in[len(in)-4:])
	if err != nil {
		return BashoID{}, bad
	}
	name := strings.TrimSuffix(in[:len(in)-4], "-")
	for month, info := range months {
		if name == strings.ToLower(info.name) {
			return NewBashoID(year, month)
		}
	}
	return BashoID{}, bad
}
