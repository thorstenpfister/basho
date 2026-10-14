package derive

import (
	"cmp"
	"slices"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Band groups the standings by wins behind the leader (SPEC §6.5).
type Band int

const (
	Leaders Band = iota // the most wins
	OneBack             // −1
	TwoBack             // −2
	Rest                // everyone else, collapsed
)

// Standings is a division's table at a cutoff.
type Standings struct {
	Rows         []Standing // wins descending, losses and absences ascending, then banzuke rank
	Today        int        // the latest day begun at the cutoff; 0 before the tournament
	YushoDecided bool       // a sole leader nobody can catch any more
	Playoff      bool       // the top is shared and nobody in contention has a day left

	KachiKoshi, MakeKoshi, Kyujo int
}

// Standing is one wrestler's row.
type Standing struct {
	RikishiID    domain.RikishiID
	Shikona      domain.Name
	Rank         domain.Rank
	Record       Record
	Remaining    int // days still to fight
	Band         Band
	InContention bool // wins + remaining ≥ the leader's wins
	NewToday     bool // kachi-koshi or make-koshi reached on Today
	Kyujo        bool // absent on the latest day counted: shown as 休

	// Results is the hoshitori grid row: days not yet counted are zero
	// (pending).
	Results [domain.BashoDays]domain.DayResult
}

// StandingsAt ranks a division's banzuke entries by their records before the
// cutoff; d holds the tournament's bouts.
func StandingsAt(entries []domain.BanzukeEntry, d DayBouts, c AsOf) Standings {
	s := Standings{Today: c.daysStarted(d.basho), Rows: make([]Standing, 0, len(entries))}
	for _, e := range entries {
		row := Standing{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank, Remaining: domain.BashoDays}
		var beforeToday Record
		for i, res := range e.Results {
			day := i + 1
			if !d.counts(e, day, c) {
				continue
			}
			row.Results[i] = res
			row.Record.add(res.Outcome)
			row.Remaining--
			row.Kyujo = res.Outcome == domain.Absent
			if day < s.Today {
				beforeToday.add(res.Outcome)
			}
		}
		r := row.Record
		row.NewToday = r.KachiKoshi() && !beforeToday.KachiKoshi() || r.MakeKoshi() && !beforeToday.MakeKoshi()
		s.KachiKoshi += b2i(r.KachiKoshi())
		s.MakeKoshi += b2i(r.MakeKoshi())
		s.Kyujo += b2i(row.Kyujo)
		s.Rows = append(s.Rows, row)
	}
	slices.SortFunc(s.Rows, func(a, b Standing) int {
		return cmp.Or(
			cmp.Compare(b.Record.Wins, a.Record.Wins),
			cmp.Compare(a.Record.Losses+a.Record.Absences, b.Record.Losses+b.Record.Absences),
			a.Rank.Compare(b.Rank),
		)
	})
	if len(s.Rows) == 0 {
		return s
	}

	lead := s.Rows[0].Record.Wins
	var atTop, catchable, toFight int // catchable: others who can still reach the leader
	for i := range s.Rows {
		row := &s.Rows[i]
		row.Band = Band(min(lead-row.Record.Wins, int(Rest)))
		row.InContention = row.Record.Wins+row.Remaining >= lead
		switch {
		case row.Record.Wins == lead:
			atTop++
		case row.InContention:
			catchable++
		}
		if row.InContention && row.Remaining > 0 {
			toFight++
		}
	}
	s.YushoDecided = atTop == 1 && catchable == 0
	s.Playoff = atTop > 1 && toFight == 0
	return s
}

// ByBanzuke returns the rows in banzuke order, for the hoshitori grid.
func (s Standings) ByBanzuke() []Standing {
	rows := slices.Clone(s.Rows)
	slices.SortFunc(rows, func(a, b Standing) int { return a.Rank.Compare(b.Rank) })
	return rows
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
