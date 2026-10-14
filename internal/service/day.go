package service

import (
	"context"
	"fmt"
	"slices"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/source"
)

// DayStatus is how far a day has got (SPEC §8.4 day bar).
type DayStatus int

const (
	NotPublished DayStatus = iota // no bout list yet
	Scheduled                     // bouts known, none decided
	InProgress                    // some decided
	Complete                      // all decided
)

var dayStatusNames = [...]string{"not_published", "scheduled", "in_progress", "complete"}

func (d DayStatus) String() string { return dayStatusNames[d] }

// MarshalText implements encoding.TextMarshaler: stable lowercase names.
func (d DayStatus) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// Day is one division's bout list of one day (SPEC §8.4, `basho day`).
type Day struct {
	Basho    domain.BashoID
	Name     string // "Aki"
	Venue    string
	Division domain.Division
	Day      int
	Date     domain.Date
	Status   DayStatus
	Bouts    []BoutRow // regular bouts in match order
	Playoffs []BoutRow // kettei-sen, after the regular bouts
	Kyujo    []Wrestler
}

// Wrestler names someone as of a tournament.
type Wrestler struct {
	RikishiID domain.RikishiID
	Shikona   domain.Name
	Rank      domain.Rank
}

// BoutRow is one bout of a day.
type BoutRow struct {
	MatchNo    int
	East, West BoutSide
	Winner     domain.Side // zero: undecided
	Kimarite   Technique
	Fusen      bool
	Kinboshi   bool
	Hidden     bool     // after the spoiler horizon, decided or not: shown as ?
	Musubi     bool     // the day's last regular bout (結び)
	WinChance  *float64 // the east side's chance before the bout; nil when the rating is hidden
	Source     string   // the bout's source when the day merged sources (SPEC §5.4); empty otherwise
}

// BoutSide is one wrestler of a bout row: the record before and after it.
type BoutSide struct {
	Wrestler
	Before, After derive.Record
	Visitor       bool // from a lower division: no record here
}

// Technique is a kimarite with its display names.
type Technique struct {
	Kimarite domain.Kimarite
	Kanji    string
	English  string
}

func technique(k domain.Kimarite) Technique {
	info, _ := k.Info()
	return Technique{Kimarite: k, Kanji: info.Kanji, English: info.English}
}

// dayData is what the day, bout, standings and recap views read.
type dayData struct {
	basho   domain.Basho
	entries []domain.BanzukeEntry
	bouts   []domain.Bout // this day's, this division
	days    derive.DayBouts
	prov    Provenance
	sources map[int]string // each bout's source when the resolver merged sources
}

func checkDay(day int) error {
	if day < 1 || day > domain.BashoDays {
		return fmt.Errorf("day %d: days run from 1 to %d: %w", day, domain.BashoDays, source.ErrNotFound)
	}
	return nil
}

func (s *Service) loadDay(ctx context.Context, id domain.BashoID, div domain.Division, day int) (dayData, error) {
	if err := checkDay(day); err != nil {
		return dayData{}, err
	}
	b, p1, err := s.basho(ctx, id)
	if err != nil {
		return dayData{}, err
	}
	entries, p2, err := s.banzuke(ctx, id, div)
	if err != nil {
		return dayData{}, err
	}
	bouts, p3, sources, err := s.torikumi(ctx, b, div, day)
	if err != nil {
		return dayData{}, err
	}
	known := bouts
	// A spoiler horizon inside an earlier day cuts through that day's
	// results: its bouts tell which ones count.
	if h, ok := horizon(ctx, id); ok && h.Basho == id && h.MatchNo > 0 && h.Day < day {
		hb, _, _, err := s.torikumi(ctx, b, div, h.Day)
		if err != nil {
			return dayData{}, err
		}
		known = append(slices.Clone(bouts), hb...)
	}
	return dayData{basho: b, entries: entries, bouts: bouts, days: derive.NewDayBouts(id, known), prov: merge(merge(p1, p2), p3), sources: sources}, nil
}

// Day returns a division's bouts of one day, with records as of before each
// bout and win chances when the rating is shown.
func (s *Service) Day(ctx context.Context, id domain.BashoID, div domain.Division, day int) (Result[Day], error) {
	d, err := s.loadDay(ctx, id, div, day)
	if err != nil {
		return Result[Day]{}, err
	}
	r, err := s.ratings()
	if err != nil {
		return Result[Day]{}, err
	}
	out := Day{
		Basho: id, Name: id.Name(), Venue: venue(d.basho), Division: div, Day: day,
		Date: domain.DayDate(d.basho.Start, day), Status: status(d.bouts),
	}
	entries := byID(d.entries)
	lastRegular := 0
	var last domain.Bout
	for _, b := range d.bouts {
		if !b.IsPlayoff && b.MatchNo >= lastRegular {
			lastRegular, last = b.MatchNo, b
		}
	}
	for _, b := range d.bouts {
		if b.IsPlayoff && !visible(ctx, last) {
			continue // that there is a playoff gives the day's results away
		}
		row := s.boutRow(ctx, b, entries, d.days, r)
		row.Source = d.sources[b.MatchNo]
		row.Musubi = !b.IsPlayoff && b.MatchNo == lastRegular && div == domain.DivMakuuchi
		if b.IsPlayoff {
			out.Playoffs = append(out.Playoffs, row)
		} else {
			out.Bouts = append(out.Bouts, row)
		}
	}
	for _, e := range d.entries {
		if e.Results[day-1].Outcome == domain.Absent {
			out.Kyujo = append(out.Kyujo, Wrestler{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank})
		}
	}
	return Result[Day]{Value: out, Prov: d.prov}, nil
}

// boutRow is one bout of a day. In spoiler-free mode a bout after the
// horizon keeps its result to itself: undecided, Hidden, records frozen at
// the horizon.
func (s *Service) boutRow(ctx context.Context, b domain.Bout, entries map[domain.RikishiID]domain.BanzukeEntry, days derive.DayBouts, r *Ratings) BoutRow {
	shown := visible(ctx, b)
	before := limit(ctx, derive.BeforeBout(b))
	side := func(bs domain.BoutSide) BoutSide {
		out := BoutSide{Wrestler: Wrestler{RikishiID: bs.RikishiID, Shikona: bs.Shikona, Rank: bs.Rank}, Visitor: true}
		if e, ok := entries[bs.RikishiID]; ok { // a Makushita visitor has no record here
			out.Visitor = false
			out.Shikona = e.Shikona // with kanji, which bout sides may lack
			out.Before = derive.RecordAt(e, days, before)
			out.After = out.Before
			if shown {
				out.After = derive.RecordAt(e, days, derive.ThroughBout(b))
			}
		}
		return out
	}
	if !shown {
		b.Winner, b.Kimarite = 0, ""
	}
	row := BoutRow{
		MatchNo: b.MatchNo, East: side(b.East), West: side(b.West), Winner: b.Winner,
		Kimarite: technique(b.Kimarite), Fusen: b.Kimarite == "fusen",
	}
	_, row.Kinboshi = derive.IsKinboshi(&b)
	row.Hidden = !shown
	if r.Shown() {
		p := r.WinChanceAt(b, before)
		row.WinChance = &p
	}
	return row
}

func status(bouts []domain.Bout) DayStatus {
	if len(bouts) == 0 {
		return NotPublished
	}
	decided := 0
	for _, b := range bouts {
		if b.Winner != 0 {
			decided++
		}
	}
	switch decided {
	case 0:
		return Scheduled
	case len(bouts):
		return Complete
	default:
		return InProgress
	}
}

func byID(entries []domain.BanzukeEntry) map[domain.RikishiID]domain.BanzukeEntry {
	m := make(map[domain.RikishiID]domain.BanzukeEntry, len(entries))
	for _, e := range entries {
		m[e.RikishiID] = e
	}
	return m
}

// venue prefers the source's location and falls back to the calendar's.
func venue(b domain.Basho) string {
	if b.Location != "" {
		return b.Location
	}
	return b.ID.Venue()
}

// DefaultDay is the day `basho day` shows without N (SPEC §11): today during
// the tournament, otherwise its last day once over, or day 1 before it.
func (s *Service) DefaultDay(ctx context.Context, id domain.BashoID) (int, error) {
	b, _, err := s.basho(ctx, id)
	if err != nil {
		return 0, err
	}
	today := s.today()
	switch {
	case b.Start.IsZero() || today.Compare(b.End) > 0:
		return domain.BashoDays, nil
	case today.Compare(b.Start) < 0:
		return 1, nil
	default:
		return min(today.DaysSince(b.Start)+1, domain.BashoDays), nil
	}
}

// DayStatuses returns the status of each day of a division for the day bar
// (SPEC §8.4). Snapshot tournaments count their bouts; later ones go by
// the dates (one metadata request at most): days before today complete,
// today in progress, the next scheduled, later ones not published.
func (s *Service) DayStatuses(ctx context.Context, id domain.BashoID, div domain.Division) (Result[[domain.BashoDays]DayStatus], error) {
	var out [domain.BashoDays]DayStatus
	ts, err := s.tournaments()
	if err != nil {
		return Result[[domain.BashoDays]DayStatus]{}, err
	}
	if ts.fromSnapshot(id) {
		x, err := s.index()
		if err != nil {
			return Result[[domain.BashoDays]DayStatus]{}, err
		}
		var byDay [domain.BashoDays][]domain.Bout
		for _, b := range x.Tournament(id) {
			if b.Division == div {
				byDay[b.Day-1] = append(byDay[b.Day-1], b)
			}
		}
		for i := range out {
			out[i] = status(byDay[i])
		}
		return Result[[domain.BashoDays]DayStatus]{Value: out, Prov: Provenance{Source: SnapshotSource, FetchedAt: ts.Snapshot.Manifest().GeneratedAt}}, nil
	}
	b, p, err := s.basho(ctx, id)
	if err != nil {
		return Result[[domain.BashoDays]DayStatus]{}, err
	}
	today := s.today()
	for i := range out {
		date := domain.DayDate(b.Start, i+1)
		switch c := date.Compare(today); {
		case b.Start.IsZero():
			out[i] = NotPublished
		case c < 0:
			out[i] = Complete
		case c == 0:
			out[i] = InProgress
		case date.DaysSince(today) == 1:
			out[i] = Scheduled // the next day's bouts come out the day before
		default:
			out[i] = NotPublished
		}
	}
	if s.status(b) == Finished {
		for i := range out {
			out[i] = Complete
		}
	}
	return Result[[domain.BashoDays]DayStatus]{Value: out, Prov: p}, nil
}
