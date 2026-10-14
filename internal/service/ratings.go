package service

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

// upsetChance is the winner's win chance before the bout below which a
// result is an upset (SPEC §6.8, §8.4).
const upsetChance = 0.35

// Ratings answers rating questions as of any cutoff (SPEC §6.8). It is safe
// for concurrent use.
type Ratings struct {
	mu    sync.RWMutex
	model *rating.Ratings
	shown bool
}

// NewRatings runs the model over every tournament of the bout index with the
// calibrated parameters.
func NewRatings(x *snapshot.Index, f rating.File) (*Ratings, error) {
	r := &Ratings{model: rating.New(f.Params), shown: f.Show}
	for _, id := range x.Tournaments() {
		if err := r.model.Set(id, x.Tournament(id)); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Shown reports whether the rating passed calibration (V9); when false, the
// UI hides rating and win chance.
func (r *Ratings) Shown() bool { return r.shown }

// Set adds a tournament newer than those held, or replaces the newest one as
// live results come in.
func (r *Ratings) Set(basho domain.BashoID, bouts []domain.Bout) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.model.Set(basho, bouts)
}

// Rating is a wrestler's rating as of a cutoff.
type Rating struct {
	Value       float64
	Change      float64 // since the start of the cutoff's tournament: ▲12 / ▼8
	Provisional bool    // no rated bout yet: the starting rating of their rank
}

// At returns id's rating before the cutoff, ranked rank in its tournament.
func (r *Ratings) At(id domain.RikishiID, rank domain.Rank, c derive.AsOf) Rating {
	r.mu.RLock()
	defer r.mu.RUnlock()
	v, prov := r.model.Before(id, rank, c)
	start, _ := r.model.Before(id, rank, derive.BeforeDay(c.Basho, 1))
	return Rating{Value: v, Change: v - start, Provisional: prov}
}

// WinChance returns the east side's chance in b, from both ratings before it.
func (r *Ratings) WinChance(b domain.Bout) float64 {
	c := derive.BeforeBout(b)
	return rating.WinChance(r.At(b.East.RikishiID, b.East.Rank, c).Value, r.At(b.West.RikishiID, b.West.Rank, c).Value)
}

// IsUpset reports a win by technique whose winner had less than a 35% chance
// before the bout.
func (r *Ratings) IsUpset(b *domain.Bout) bool {
	if b.Winner == 0 || b.Kimarite.Category() == domain.NoTechnique {
		return false
	}
	east := r.WinChance(*b)
	if b.Winner == domain.West {
		east = 1 - east
	}
	return east < upsetChance
}

// Rated is one row of a ratings table.
type Rated struct {
	RikishiID domain.RikishiID
	Shikona   domain.Name
	Rank      domain.Rank
	Rating
}

// Top rates the wrestlers of entries as of the cutoff, highest first, and
// returns the first n (all for n ≤ 0).
func (r *Ratings) Top(entries []domain.BanzukeEntry, c derive.AsOf, n int) []Rated {
	rows := make([]Rated, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, Rated{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank, Rating: r.At(e.RikishiID, e.Rank, c)})
	}
	slices.SortStableFunc(rows, func(a, b Rated) int { return cmp.Or(cmp.Compare(b.Value, a.Value), a.Rank.Compare(b.Rank)) })
	if n > 0 && n < len(rows) {
		rows = rows[:n]
	}
	return rows
}

// RatingTable is the top-rated wrestlers of a tournament (`basho ratings`).
type RatingTable struct {
	Basho domain.BashoID
	Shown bool // V9: false means the UI hides ratings
	Rows  []Rated
}

// RatingTable rates both divisions' wrestlers of tournament id as of its end
// (as far as it has been fought), highest first, and keeps the first n.
func (s *Service) RatingTable(ctx context.Context, id domain.BashoID, n int) (Result[RatingTable], error) {
	r, err := s.ratings()
	if err != nil {
		return Result[RatingTable]{}, err
	}
	var entries []domain.BanzukeEntry
	var prov Provenance
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		e, p, err := s.banzuke(ctx, id, div)
		if err != nil {
			return Result[RatingTable]{}, err
		}
		entries, prov = slices.Concat(entries, e), merge(prov, p)
	}
	return Result[RatingTable]{Value: RatingTable{Basho: id, Shown: r.Shown(), Rows: r.Top(entries, derive.EndOf(id), n)}, Prov: prov}, nil
}
