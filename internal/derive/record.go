package derive

import (
	"strconv"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Record is a wrestler's tally in one tournament (SPEC §6.1). Fusen wins and
// losses count as wins and losses; playoffs never count.
type Record struct {
	Wins, Losses, Absences, Draws int
}

// majority is the number of wins (or of losses and absences) that settles a
// 15-day tournament: kachi-koshi or make-koshi.
const majority = domain.BashoDays/2 + 1

func (r *Record) add(o domain.Outcome) {
	switch {
	case o.IsWin():
		r.Wins++
	case o.IsLoss():
		r.Losses++
	case o == domain.Absent:
		r.Absences++
	case o == domain.Draw:
		r.Draws++
	}
}

// String prints the record as "5-7", or "5-7-3" with absences; draws (old
// era only) follow as "1d".
func (r Record) String() string {
	s := strconv.Itoa(r.Wins) + "-" + strconv.Itoa(r.Losses)
	if r.Absences > 0 {
		s += "-" + strconv.Itoa(r.Absences)
	}
	if r.Draws > 0 {
		s += " " + strconv.Itoa(r.Draws) + "d"
	}
	return s
}

// KachiKoshi reports a winning record: 8 wins.
func (r Record) KachiKoshi() bool { return r.Wins >= majority }

// MakeKoshi reports a losing record: 8 losses and absences together.
func (r Record) MakeKoshi() bool { return r.Losses+r.Absences >= majority }

// DayBouts finds a tournament's regular bouts by wrestler and day, to place
// banzuke day results against a cutoff.
type DayBouts struct {
	basho domain.BashoID
	bouts map[dayKey]*domain.Bout
}

type dayKey struct {
	rikishi domain.RikishiID
	day     int
}

// NewDayBouts indexes the bouts of tournament basho.
func NewDayBouts(basho domain.BashoID, bouts []domain.Bout) DayBouts {
	d := DayBouts{basho: basho, bouts: make(map[dayKey]*domain.Bout, 2*len(bouts))}
	for i := range bouts {
		if b := &bouts[i]; !b.IsPlayoff {
			d.bouts[dayKey{b.East.RikishiID, b.Day}] = b
			d.bouts[dayKey{b.West.RikishiID, b.Day}] = b
		}
	}
	return d
}

// Bout returns a wrestler's regular bout on day, or nil.
func (d DayBouts) Bout(id domain.RikishiID, day int) *domain.Bout { return d.bouts[dayKey{id, day}] }

// counts reports whether e's result on day counts before the cutoff.
func (d DayBouts) counts(e domain.BanzukeEntry, day int, c AsOf) bool {
	return c.IncludesDay(d.basho, day, d.Bout(e.RikishiID, day))
}

// RecordAt tallies e's banzuke record before the cutoff. The record going
// into a bout is RecordAt(…, BeforeBout(b)); the record after it,
// RecordAt(…, ThroughBout(b)).
func RecordAt(e domain.BanzukeEntry, d DayBouts, c AsOf) Record {
	var r Record
	for i, res := range e.Results {
		if d.counts(e, i+1, c) {
			r.add(res.Outcome)
		}
	}
	return r
}

// RecordThrough is a record from a tournament's day outcomes up to cutoff c:
// a day with a bout counts when c includes the bout, a day without one
// (an absence, a day off) when c has reached that day.
func RecordThrough(basho domain.BashoID, outcomes [domain.BashoDays]domain.Outcome, boutOf func(day int) *domain.Bout, c AsOf) Record {
	var r Record
	for i, o := range outcomes {
		if c.IncludesDay(basho, i+1, boutOf(i+1)) {
			r.add(o)
		}
	}
	return r
}
