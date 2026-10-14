package service

import (
	"fmt"
	"slices"
	"sync"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

// Service is the façade the TUI and CLI talk to. Everything heavy (the
// snapshot, the bout index, the ratings) is loaded the first time it is
// needed, so commands such as `basho version` stay instant. Phase 6 adds the
// remote layers and the remaining use cases.
type Service struct {
	snapshot func() (*snapshot.Snapshot, error)
	index    func() (*snapshot.Index, error)
	ratings  func() (*Ratings, error)
}

// New returns a service over the snapshot and rating parameters the loaders
// provide (data.Snapshot and data.RatingParams in the binary).
func New(snap func() (*snapshot.Snapshot, error), params func() (rating.File, error)) *Service {
	s := &Service{snapshot: sync.OnceValues(snap)}
	s.index = sync.OnceValues(func() (*snapshot.Index, error) {
		sn, err := s.snapshot()
		if err != nil {
			return nil, err
		}
		return sn.BuildIndex()
	})
	s.ratings = sync.OnceValues(func() (*Ratings, error) {
		x, err := s.index()
		if err != nil {
			return nil, err
		}
		f, err := params()
		if err != nil {
			return nil, err
		}
		return NewRatings(x, f)
	})
	return s
}

// Ratings returns the rating model, built on first use.
func (s *Service) Ratings() (*Ratings, error) { return s.ratings() }

// Newest returns the newest tournament the service holds.
func (s *Service) Newest() (domain.BashoID, error) {
	sn, err := s.snapshot()
	if err != nil {
		return domain.BashoID{}, err
	}
	return sn.Manifest().Newest(), nil
}

// RatingTable is the top-rated wrestlers of a tournament (`basho ratings`).
type RatingTable struct {
	Basho domain.BashoID
	Shown bool // V9: false means the UI hides ratings
	Rows  []Rated
}

// TopRatings rates both divisions' wrestlers of tournament id as of its end
// (as far as it has been fought), highest first, and keeps the first n.
func (s *Service) TopRatings(id domain.BashoID, n int) (RatingTable, error) {
	sn, err := s.snapshot()
	if err != nil {
		return RatingTable{}, err
	}
	if !sn.Has(id) {
		// Tournaments after the snapshot come from the remote layers (Phase 6).
		return RatingTable{}, fmt.Errorf("tournament %s: %w", id, source.ErrNotYetAvailable)
	}
	r, err := s.ratings()
	if err != nil {
		return RatingTable{}, err
	}
	var entries []domain.BanzukeEntry
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		e, err := sn.Banzuke(id, div)
		if err != nil {
			return RatingTable{}, err
		}
		entries = slices.Concat(entries, e)
	}
	return RatingTable{Basho: id, Shown: r.Shown(), Rows: r.Top(entries, derive.EndOf(id), n)}, nil
}
