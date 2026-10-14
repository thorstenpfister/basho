package service

import (
	"context"
	"errors"
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
	Basho     domain.BashoID
	Day       int
	Days      map[domain.Division]Result[Day]
	UpdatedAt time.Time // the source's stamp: osada's resultUpdatedAt; zero without one
	Complete  bool      // every bout decided in both divisions, no playoff pending
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

// LivePoll refreshes today's bouts of both divisions (SPEC §10.2): one
// conditional GET to osada, with sumo-api as the fallback on the same tick.
// The answer also lands in the cache, the bout index and the ratings, like
// any day that is not in the snapshot.
func (s *Service) LivePoll(ctx context.Context, id domain.BashoID, day int) (LivePoll, error) {
	out := LivePoll{Basho: id, Day: day, Days: map[domain.Division]Result[Day]{}}
	ctx = resolver.Live(ctx)
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		res, err := s.Day(ctx, id, div, day)
		if err != nil {
			return out, err
		}
		out.Days[div] = res
		out.Prov = merge(out.Prov, res.Prov)
	}
	if at := out.Prov.SourceUpdatedAt; at != nil {
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
	return out, nil
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
