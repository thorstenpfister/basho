package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

// Career is a wrestler's career as seen from one tournament (SPEC §8.10,
// `basho wrestler`): everything up to its end, or up to its spoiler horizon.
type Career struct {
	Wrestler                       // today's name and the last rank
	Through     domain.BashoID     // the tournament it is seen from
	Profile     domain.Rikishi     // heya, hometown, birth, height and weight
	HasProfile  bool               // false: none known
	Stats       derive.CareerStats // as of the cutoff: age, yusho, special prizes
	Kinboshi    derive.Kinboshi
	Movesets    [3]derive.Profile // by derive.Window
	Tournaments []CareerBasho     // oldest first
	RatingShown bool              // V9: false means the UI hides ratings
}

// CareerBasho is one tournament of a career.
type CareerBasho struct {
	Basho   domain.BashoID
	Shikona domain.Name // as of that tournament
	Rank    domain.Rank
	Record  derive.Record
	Partial bool     // cut by the cutoff: the tournament seen from, under way or behind a horizon
	Yusho   bool     // won its division; false until the cutoff has passed the tournament
	Prizes  []string // special prizes, likewise
	Rating  float64  // after the tournament, or at the cutoff; 0 when hidden
	LastDay int      // the last day they fought, within the cutoff; 0 for none
}

// Wrestler returns a wrestler's career as seen from tournament through: every
// sekitori tournament up to it, cut at its end, or at its horizon in
// spoiler-free mode. Tournaments after the snapshot come from the remote
// layers.
func (s *Service) Wrestler(ctx context.Context, id domain.RikishiID, through domain.BashoID) (Result[Career], error) {
	x, err := s.index()
	if err != nil {
		return Result[Career]{}, err
	}
	r, err := s.ratings()
	if err != nil {
		return Result[Career]{}, err
	}
	profiles, err := s.wrestlers()
	if err != nil {
		return Result[Career]{}, err
	}
	ts, err := s.tournaments()
	if err != nil {
		return Result[Career]{}, err
	}
	c := limit(ctx, derive.EndOf(through))
	prov := Provenance{Source: SnapshotSource}
	if ts.Snapshot != nil {
		prov.FetchedAt = ts.Snapshot.Manifest().GeneratedAt
	}

	var places []snapshot.Placement
	for _, p := range x.Placements(id) {
		if p.Basho.Compare(through) <= 0 {
			places = append(places, p)
		}
	}
	// Tournaments after the snapshot: their banzuke from the remote layers.
	for _, b := range x.Tournaments() {
		if b.Compare(through) > 0 || ts.fromSnapshot(b) {
			continue
		}
		for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
			entries, p, err := s.banzuke(ctx, b, div)
			if err != nil {
				return Result[Career]{}, err
			}
			if i := slices.IndexFunc(entries, func(e domain.BanzukeEntry) bool { return e.RikishiID == id }); i >= 0 {
				places = append(places, snapshot.PlacementOf(b, entries[i]))
				prov = merge(prov, p)
			}
		}
	}
	if len(places) == 0 {
		return Result[Career]{}, fmt.Errorf("wrestler %d: no sekitori tournament up to %s: %w", id, through, source.ErrNotFound)
	}

	bouts := x.Rikishi(id)
	type dayKey struct {
		basho domain.BashoID
		day   int
	}
	byDay := make(map[dayKey]*domain.Bout, len(bouts))
	for _, b := range bouts {
		if !b.IsPlayoff {
			byDay[dayKey{b.Basho, b.Day}] = b
		}
	}
	boutOf := func(basho domain.BashoID) func(int) *domain.Bout {
		return func(day int) *domain.Bout { return byDay[dayKey{basho, day}] }
	}
	last := places[len(places)-1]
	profile, hasProfile := profiles[id]
	name := profile.Name
	if !hasProfile {
		name = last.Shikona
		profile.ID = id // CareerAt counts yusho and prizes by it
	}
	meta, _ := x.Basho(through)
	if meta.ID.IsZero() {
		meta.ID = through
	}
	car := Career{
		Wrestler: Wrestler{RikishiID: id, Shikona: name, Rank: last.Rank},
		Through:  through, Profile: profile, HasProfile: hasProfile,
		Stats:       derive.CareerAt(profile, last.Shikona, meta, bashoList(x), c, s.CurrentBasho()),
		Kinboshi:    derive.KinboshiOf(id, bouts, c),
		RatingShown: r.Shown(),
	}
	for w := range car.Movesets {
		car.Movesets[w] = derive.ProfileOf(id, bouts, c, derive.Window(w))
	}
	for _, p := range places {
		cb := CareerBasho{
			Basho: p.Basho, Shikona: p.Shikona, Rank: p.Rank,
			Record:  derive.RecordThrough(p.Basho, p.Outcomes, boutOf(p.Basho), c),
			Partial: p.Basho == c.Basho && derive.EndOf(p.Basho).After(c),
		}
		for day := domain.BashoDays; day > 0 && cb.LastDay == 0; day-- {
			if b := byDay[dayKey{p.Basho, day}]; b != nil && c.Includes(*b) {
				cb.LastDay = day
			}
		}
		if b, ok := x.Basho(p.Basho); ok && finalShown(ctx, p.Basho) && !cb.Partial {
			cb.Yusho = b.Yusho[p.Rank.Title.Division()] == id
			for _, pr := range b.Prizes {
				if pr.RikishiID == id {
					cb.Prizes = append(cb.Prizes, pr.Type)
				}
			}
		}
		if car.RatingShown {
			cb.Rating = r.At(id, p.Rank, derive.Earlier(derive.EndOf(p.Basho), c)).Value
		}
		car.Tournaments = append(car.Tournaments, cb)
	}
	return Result[Career]{Value: car, Prov: prov}, nil
}

// Match is a wrestler a name found.
type Match struct {
	Wrestler                   // today's name
	Shikona     domain.Name    // the name that matched
	First, Last domain.BashoID // their first and last sekitori tournaments
}

// FindWrestlers returns the wrestlers who fought under name in any
// tournament or carry it today, matched exactly but for case; ordered by
// their first tournament. A number is taken as a wrestler ID. No match is
// a NotFound error.
func (s *Service) FindWrestlers(name string) ([]Match, error) {
	x, err := s.index()
	if err != nil {
		return nil, err
	}
	profiles, err := s.wrestlers()
	if err != nil {
		return nil, err
	}
	if id, err := strconv.Atoi(name); err == nil {
		name = ""
		if ps := x.Placements(domain.RikishiID(id)); len(ps) > 0 {
			return []Match{match(domain.RikishiID(id), ps, ps[len(ps)-1].Shikona, profiles)}, nil
		}
		return nil, fmt.Errorf("no sekitori wrestler has the ID %d: %w", id, source.ErrNotFound)
	}
	var out []Match
	x.EachPlaced(func(id domain.RikishiID, ps []snapshot.Placement) {
		for _, p := range ps {
			if strings.EqualFold(p.Shikona.En, name) || p.Shikona.Jp != "" && p.Shikona.Jp == name {
				out = append(out, match(id, ps, p.Shikona, profiles))
				return
			}
		}
		if p, ok := profiles[id]; ok && (strings.EqualFold(p.Name.En, name) || p.Name.Jp != "" && p.Name.Jp == name) {
			out = append(out, match(id, ps, p.Name, profiles))
		}
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("no sekitori wrestler is or was called %q: %w", name, source.ErrNotFound)
	}
	slices.SortFunc(out, func(a, b Match) int {
		return cmp.Or(a.First.Compare(b.First), cmp.Compare(a.RikishiID, b.RikishiID))
	})
	return out, nil
}

func match(id domain.RikishiID, ps []snapshot.Placement, matched domain.Name, profiles map[domain.RikishiID]domain.Rikishi) Match {
	last := ps[len(ps)-1]
	name := last.Shikona
	if p, ok := profiles[id]; ok {
		name = p.Name
	}
	return Match{Wrestler: Wrestler{RikishiID: id, Shikona: name, Rank: last.Rank}, Shikona: matched, First: ps[0].Basho, Last: last.Basho}
}
