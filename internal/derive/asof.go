package derive

import (
	"cmp"
	"math"
	"sort"

	"github.com/thorstenpfister/basho/internal/domain"
)

// AsOf is a cutoff in time (SPEC §6): a computation sees only what happened
// before it. It is a point in the order of play: by tournament and day; within
// a day Juryo before Makuuchi (lower divisions fight first), a division's
// playoffs after its regular bouts, then by match number.
//
// The zero value cuts off before everything; NoCutoff lets everything through.
type AsOf struct {
	Basho     domain.BashoID
	Day       int             // 0: before the tournament
	Division  domain.Division // the division MatchNo counts in
	MatchNo   int             // 0: the whole day
	Playoff   bool            // MatchNo counts playoff bouts
	Inclusive bool            // the bout or day at the cutoff counts too
}

// NoCutoff includes everything there is.
var NoCutoff = AsOf{Basho: domain.BashoID{Year: math.MaxInt}}

// BeforeDay cuts off at the start of day (day 1: before the tournament).
func BeforeDay(basho domain.BashoID, day int) AsOf { return AsOf{Basho: basho, Day: day} }

// AfterDay cuts off at the end of day, playoffs included.
func AfterDay(basho domain.BashoID, day int) AsOf {
	return AsOf{Basho: basho, Day: day, Inclusive: true}
}

// EndOf cuts off after a whole tournament, playoffs included.
func EndOf(basho domain.BashoID) AsOf { return AfterDay(basho, domain.BashoDays) }

// BeforeBout cuts off just before b: what both wrestlers bring into it.
func BeforeBout(b domain.Bout) AsOf {
	return AsOf{Basho: b.Basho, Day: b.Day, Division: b.Division, MatchNo: b.MatchNo, Playoff: b.IsPlayoff}
}

// ThroughBout cuts off just after b, like a spoiler horizon at the last bout
// seen (SPEC §9).
func ThroughBout(b domain.Bout) AsOf {
	c := BeforeBout(b)
	c.Inclusive = true
	return c
}

// Includes reports whether b took place before the cutoff.
func (c AsOf) Includes(b domain.Bout) bool {
	if n := b.Basho.Compare(c.Basho); n != 0 {
		return n < 0
	}
	if b.Day != c.Day {
		return b.Day < c.Day
	}
	if c.MatchNo == 0 {
		return c.Inclusive
	}
	n := ComparePlay(b, domain.Bout{Basho: c.Basho, Day: c.Day, Division: c.Division, MatchNo: c.MatchNo, IsPlayoff: c.Playoff})
	return n < 0 || n == 0 && c.Inclusive
}

// ComparePlay orders bouts as they are fought: by tournament and day; within
// a day Juryo before Makuuchi, a division's playoffs after its regular bouts,
// then by match number. It is the order AsOf cuts.
func ComparePlay(a, b domain.Bout) int {
	return cmp.Or(
		a.Basho.Compare(b.Basho),
		cmp.Compare(a.Day, b.Day),
		cmp.Compare(b.Division, a.Division), // Juryo (the larger value) fights first
		boolCompare(a.IsPlayoff, b.IsPlayoff),
		cmp.Compare(a.MatchNo, b.MatchNo),
	)
}

// IncludesDay reports whether a banzuke day result counts before the cutoff:
// by its bout when it has one, otherwise (absence, a gap in the source) once
// its day has started.
func (c AsOf) IncludesDay(basho domain.BashoID, day int, bout *domain.Bout) bool {
	if bout != nil {
		return c.Includes(*bout)
	}
	return day <= c.daysStarted(basho)
}

// daysStarted returns how many days of basho have begun at the cutoff: the
// latest one is "today".
func (c AsOf) daysStarted(basho domain.BashoID) int {
	switch n := basho.Compare(c.Basho); {
	case n < 0:
		return domain.BashoDays
	case n > 0:
		return 0
	case c.Inclusive || c.MatchNo > 0:
		return min(c.Day, domain.BashoDays)
	default:
		return max(c.Day-1, 0)
	}
}

// before returns the bouts of a chronological career before the cutoff. A
// wrestler fights once a day, apart from playoffs after the regular bouts, so
// the career's order agrees with the cutoff's and a prefix is the answer.
func before(career []*domain.Bout, c AsOf) []*domain.Bout {
	return career[:sort.Search(len(career), func(i int) bool { return !c.Includes(*career[i]) })]
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}

// Earlier returns whichever cutoff comes first in the order of play: a
// cutoff further limited by a spoiler horizon (SPEC §9).
func Earlier(a, b AsOf) AsOf {
	if a.point().compare(b.point()) <= 0 {
		return a
	}
	return b
}

// After reports whether c lies after o in the order of play.
func (c AsOf) After(o AsOf) bool { return c.point().compare(o.point()) > 0 }

// point places a cutoff on one scale: the tournament, the day, then within
// the day its start, a bout (before or just after it) or its end.
type point struct {
	basho     domain.BashoID
	day       int
	phase     int // 0: start of the day, 1: at a bout, 2: end of the day
	bout      domain.Bout
	inclusive bool
}

func (c AsOf) point() point {
	p := point{basho: c.Basho, day: c.Day}
	switch {
	case c.MatchNo > 0:
		p.phase, p.inclusive = 1, c.Inclusive
		p.bout = domain.Bout{Basho: c.Basho, Day: c.Day, Division: c.Division, MatchNo: c.MatchNo, IsPlayoff: c.Playoff}
	case c.Inclusive:
		p.phase = 2
	}
	return p
}

func (p point) compare(o point) int {
	if n := cmp.Or(p.basho.Compare(o.basho), cmp.Compare(p.day, o.day), cmp.Compare(p.phase, o.phase)); n != 0 || p.phase != 1 {
		return n
	}
	return cmp.Or(ComparePlay(p.bout, o.bout), boolCompare(p.inclusive, o.inclusive))
}
