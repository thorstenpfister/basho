package service

import (
	"context"
	"maps"
	"slices"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// BashoStatus is a tournament's state by the calendar and its data.
type BashoStatus int

const (
	Upcoming BashoStatus = iota
	InProgressBasho
	Finished
	Cancelled
)

var bashoStatusNames = [...]string{"upcoming", "in_progress", "finished", "cancelled"}

func (b BashoStatus) String() string { return bashoStatusNames[b] }

// MarshalText implements encoding.TextMarshaler.
func (b BashoStatus) MarshalText() ([]byte, error) { return []byte(b.String()), nil }

// Info is a tournament's metadata with its winners named (`basho info`).
type Info struct {
	Basho  domain.Basho
	Name   string
	Venue  string
	Status BashoStatus
	Yusho  []Winner // by division, Makuuchi first; lower divisions only when named
	Prizes []Winner // special prizes; Division is zero
}

// Winner is a yusho or special prize and who won it.
type Winner struct {
	Division domain.Division // for a yusho
	Prize    string          // for a special prize, e.g. "Kanto-sho"
	Wrestler
	Record *derive.Record // final record; nil outside the snapshot's divisions
}

// Info returns tournament id's metadata.
func (s *Service) Info(ctx context.Context, id domain.BashoID) (Result[Info], error) {
	if id.Held() == domain.NotHeld {
		return Result[Info]{Value: Info{Basho: domain.Basho{ID: id}, Name: id.Name(), Venue: id.Venue(), Status: Cancelled}, Prov: Provenance{Source: "calendar", FetchedAt: s.opts.Clock.Now()}}, nil
	}
	b, p, err := s.basho(ctx, id)
	if err != nil {
		return Result[Info]{}, err
	}
	names := map[domain.RikishiID]Winner{}
	for _, div := range []domain.Division{domain.DivJuryo, domain.DivMakuuchi} {
		entries, p2, err := s.banzuke(ctx, id, div)
		if err != nil {
			return Result[Info]{}, err
		}
		p = merge(p, p2)
		for _, e := range entries {
			r := derive.RecordAt(e, derive.NewDayBouts(id, nil), derive.EndOf(id))
			names[e.RikishiID] = Winner{Wrestler: Wrestler{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank}, Record: &r}
		}
	}
	profiles, err := s.wrestlers()
	if err != nil {
		return Result[Info]{}, err
	}
	name := func(id domain.RikishiID) Winner {
		if w, ok := names[id]; ok {
			return w
		}
		return Winner{Wrestler: Wrestler{RikishiID: id, Shikona: profiles[id].Name}} // a lower division's winner
	}
	out := Info{Basho: b, Name: id.Name(), Venue: venue(b), Status: s.status(b)}
	for _, div := range slices.Sorted(maps.Keys(b.Yusho)) {
		// The snapshot names sekitori only (SPEC §5.8): a lower division's
		// winner is listed when known.
		if w := name(b.Yusho[div]); w.Shikona.En != "" || div == domain.DivMakuuchi || div == domain.DivJuryo {
			w.Division = div
			out.Yusho = append(out.Yusho, w)
		}
	}
	for _, pr := range b.Prizes {
		w := name(pr.RikishiID)
		w.Prize = pr.Type
		out.Prizes = append(out.Prizes, w)
	}
	return Result[Info]{Value: out, Prov: p}, nil
}

// CalendarStatus is a tournament's status by the estimated dates alone, until
// its data says better.
func CalendarStatus(id domain.BashoID, today domain.Date) BashoStatus {
	switch {
	case id.Held() == domain.NotHeld:
		return Cancelled
	case today.DaysSince(id.EstimatedEnd()) >= domain.FinalMargin:
		return Finished
	case today.Compare(id.EstimatedStart()) >= 0:
		return InProgressBasho
	default:
		return Upcoming
	}
}

func (s *Service) status(b domain.Basho) BashoStatus {
	today := s.today()
	switch {
	case b.ID.Held() == domain.NotHeld:
		return Cancelled
	case domain.IsFinal(b.End, b.Yusho[domain.DivMakuuchi] != 0, today):
		return Finished
	case !b.Start.IsZero() && today.Compare(b.Start) >= 0:
		return InProgressBasho
	default:
		return Upcoming
	}
}

// ListEntry is one row of `basho list`.
type ListEntry struct {
	Basho      domain.BashoID
	Name       string
	Venue      string
	Status     BashoStatus
	Yusho      *Winner // the Makuuchi winner, when known
	InSnapshot bool
}

// List returns the tournaments of the years from–to (zero: all), newest
// first, up to the current one. Snapshot tournaments need no request; for
// later ones the remote layers are asked once each, and a failure only leaves
// the winner out.
func (s *Service) List(ctx context.Context, from, to int) (Result[[]ListEntry], error) {
	ts, err := s.tournaments()
	if err != nil {
		return Result[[]ListEntry]{}, err
	}
	var out []ListEntry
	prov := Provenance{}
	for id := s.CurrentBasho(); id.Compare(domain.FirstBasho) >= 0; id = id.Prev() {
		if to != 0 && id.Year > to || from != 0 && id.Year < from {
			continue
		}
		e := ListEntry{Basho: id, Name: id.Name(), Venue: id.Venue(), InSnapshot: ts.fromSnapshot(id), Status: CalendarStatus(id, s.today())}
		if id.Held() == domain.NotHeld {
			e.Status = Cancelled
			out = append(out, e)
			continue
		}
		if e.InSnapshot || ts.Remote != nil {
			if info, err := s.Info(ctx, id); err == nil {
				prov = merge(prov, info.Prov)
				e.Venue, e.Status = info.Value.Venue, info.Value.Status
				for _, y := range info.Value.Yusho {
					if y.Division == domain.DivMakuuchi {
						e.Yusho = &y
					}
				}
			}
		}
		out = append(out, e)
	}
	if prov.Source == "" {
		prov = Provenance{Source: "calendar", FetchedAt: s.opts.Clock.Now()}
	}
	return Result[[]ListEntry]{Value: out, Prov: prov}, nil
}
