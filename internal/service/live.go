package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/source/resolver"
)

// LiveTournament returns the tournament in progress today and its day 1,
// for live mode (SPEC §10.1); ok is false between tournaments. It costs one
// metadata request at most, cached.
func (s *Service) LiveTournament(ctx context.Context) (id domain.BashoID, start domain.Date, ok bool, err error) {
	id = s.CurrentBasho()
	if CalendarStatus(id, s.today()) != InProgressBasho {
		return id, domain.Date{}, false, nil
	}
	b, _, err := s.basho(ctx, id)
	if err != nil {
		return id, domain.Date{}, false, err
	}
	end := b.End
	if end.IsZero() {
		end = domain.DayDate(b.Start, domain.BashoDays)
	}
	today := s.today()
	return id, b.Start, today.Compare(b.Start) >= 0 && today.Compare(end) <= 0, nil
}

// LivePoll is one live tick's answer: today's bouts of both divisions.
type LivePoll struct {
	Basho domain.BashoID
	Day   int
	Days  map[domain.Division]Result[Day]
	// UpdatedAt is when the results last changed: the poll that first
	// showed them, or osada's resultUpdatedAt when that is later. The
	// scheduler slows down while it stays the same.
	UpdatedAt time.Time
	Complete  bool // every bout decided in both divisions, no playoff pending
	Osada     bool // the poll revalidated osada (OsadaEvery, the playoff watch)
	Prov      Provenance
}

// Err returns the failure the scheduler backs off for: the primary source's,
// even when the fallback answered.
func (p LivePoll) Err() error {
	if p.Prov.Failed != "" {
		return errors.New(p.Prov.Failed + ": " + p.Prov.FailedErr)
	}
	return nil
}

// OsadaEvery is how often live mode revalidates osada.us (SPEC §10.2):
// sumo-api has most results first, osada publishes in batches about 13
// minutes apart (V1, docs/v1-latency.md).
const OsadaEvery = 15 * time.Minute

// liveState is what one tournament day's polls remember.
type liveState struct {
	basho   domain.BashoID
	day     int
	osadaAt time.Time // the last poll that revalidated osada
	// watch: day 15 with every regular bout decided. Only osada lists the
	// playoff, so it is revalidated each poll.
	watch   bool
	results string    // the last poll's decided bouts
	changed time.Time // the poll that first showed them
}

// LivePoll refreshes today's bouts of both divisions (SPEC §10.2):
// sumo-api's two bout lists, and osada's every OsadaEvery, during the
// playoff watch, or when sumo-api fails. osada's bouts, names and playoffs
// are the base, with sumo-api's newer results laid over them. The answer
// also lands in the cache, the bout index and the ratings, like any day that
// is not in the snapshot.
func (s *Service) LivePoll(ctx context.Context, id domain.BashoID, day int) (LivePoll, error) {
	out := LivePoll{Basho: id, Day: day, Days: map[domain.Division]Result[Day]{}}
	now := s.opts.Clock.Now()
	s.liveMu.Lock()
	defer s.liveMu.Unlock()
	st := &s.live
	if st.basho != id || st.day != day {
		*st = liveState{basho: id, day: day}
	}
	out.Osada = st.osadaAt.IsZero() || now.Sub(st.osadaAt) >= OsadaEvery || st.watch
	ctx = resolver.Live(ctx, out.Osada)
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		res, err := s.Day(ctx, id, div, day)
		if err != nil {
			return out, err
		}
		out.Days[div] = res
		out.Prov = merge(out.Prov, res.Prov)
	}
	if out.Osada {
		st.osadaAt = now
	}
	if r := decidedResults(out.Days); r != st.results {
		st.results, st.changed = r, now
	}
	out.UpdatedAt = st.changed
	if at := out.Prov.SourceUpdatedAt; at != nil && at.After(out.UpdatedAt) {
		out.UpdatedAt = *at
	}
	out.Complete = true
	for div, res := range out.Days {
		done, err := s.dayComplete(ctx, id, div, day, res.Value)
		if err != nil {
			return out, err
		}
		out.Complete = out.Complete && done
	}
	st.watch = day == domain.BashoDays && !out.Complete && allRegularDecided(out.Days)
	return out, nil
}

// decidedResults is a poll's decided bouts as text, to see a change.
func decidedResults(days map[domain.Division]Result[Day]) string {
	var b strings.Builder
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		d := days[div].Value
		for _, r := range append(append([]BoutRow{}, d.Bouts...), d.Playoffs...) {
			if r.Winner != 0 {
				fmt.Fprintf(&b, "%s %d %d %s;", div, r.MatchNo, r.Winner, r.Kimarite.Kimarite)
			}
		}
	}
	return b.String()
}

// allRegularDecided reports a day whose regular bouts are all decided.
func allRegularDecided(days map[domain.Division]Result[Day]) bool {
	for _, res := range days {
		if len(res.Value.Bouts) == 0 {
			return false
		}
		for _, r := range res.Value.Bouts {
			if r.Winner == 0 {
				return false
			}
		}
	}
	return len(days) > 0
}

// dayComplete reports a day with every bout decided and, on day 15, no
// playoff to come: a shared lead nobody can still change needs one, which
// osada lists once it is scheduled.
func (s *Service) dayComplete(ctx context.Context, id domain.BashoID, div domain.Division, day int, d Day) (bool, error) {
	rows := append(append([]BoutRow{}, d.Bouts...), d.Playoffs...)
	if len(rows) == 0 {
		return false, nil
	}
	for _, r := range rows {
		if r.Winner == 0 {
			return false, nil
		}
	}
	if day < domain.BashoDays || len(d.Playoffs) > 0 {
		return true, nil
	}
	st, err := s.Standings(ctx, id, div, day) // the poll just filled the cache
	if err != nil {
		return false, err
	}
	return !st.Value.Playoff, nil
}
