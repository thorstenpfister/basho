package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

// ErrNoSnapshot is returned by snapshot-only operations without a snapshot.
var ErrNoSnapshot = errors.New("no snapshot")

// Options configure a Service.
type Options struct {
	Snapshot   func() (*snapshot.Snapshot, error) // nil: --no-snapshot
	Params     func() (rating.File, error)
	Remote     TournamentSource // disk cache over the network; nil: snapshot only
	RemoteName string           // provenance label, e.g. "sumo-api"
	Clock      clock.Clock      // nil: the real clock
}

// Service is the façade the TUI and CLI talk to (SPEC §5.1). Everything heavy
// (the snapshot, the bout index, the ratings) is loaded the first time it is
// needed, so commands such as `basho version` stay instant. It is safe for
// concurrent use.
type Service struct {
	opts      Options
	snapshot  func() (*snapshot.Snapshot, error)
	index     func() (*snapshot.Index, error)
	ratings   func() (*Ratings, error)
	wrestlers func() (map[domain.RikishiID]domain.Rikishi, error)

	mu    sync.Mutex
	later map[domain.BashoID]*laterTournament // after the snapshot: the days loaded so far
}

// laterTournament collects what the remote layers returned for a tournament
// newer than the snapshot, so the bout index and ratings can include it.
type laterTournament struct {
	basho domain.Basho
	days  map[dayKey][]domain.Bout
}

type dayKey struct {
	div domain.Division
	day int
}

// New returns a service over the given layers.
func New(opts Options) *Service {
	if opts.Clock == nil {
		opts.Clock = clock.Real{}
	}
	if opts.RemoteName == "" {
		opts.RemoteName = "sumo-api"
	}
	s := &Service{opts: opts, later: map[domain.BashoID]*laterTournament{}}
	s.snapshot = sync.OnceValues(func() (*snapshot.Snapshot, error) {
		if opts.Snapshot == nil {
			return nil, nil
		}
		return opts.Snapshot()
	})
	s.index = sync.OnceValues(func() (*snapshot.Index, error) {
		sn, err := s.snapshot()
		if err != nil || sn == nil {
			return snapshot.NewIndex(), err
		}
		return sn.BuildIndex()
	})
	s.ratings = sync.OnceValues(func() (*Ratings, error) {
		x, err := s.index()
		if err != nil {
			return nil, err
		}
		f := rating.File{}
		if opts.Params != nil {
			if f, err = opts.Params(); err != nil {
				return nil, err
			}
		}
		return NewRatings(x, f)
	})
	s.wrestlers = sync.OnceValues(func() (map[domain.RikishiID]domain.Rikishi, error) {
		sn, err := s.snapshot()
		if err != nil || sn == nil {
			return map[domain.RikishiID]domain.Rikishi{}, err
		}
		rs, err := sn.Rikishi()
		if err != nil {
			return nil, err
		}
		m := make(map[domain.RikishiID]domain.Rikishi, len(rs))
		for _, r := range rs {
			m[r.ID] = r
		}
		return m, nil
	})
	return s
}

func (s *Service) today() domain.Date { return domain.DateOf(s.opts.Clock.Now()) }

// tournaments returns the layer order of SPEC §5.8.
func (s *Service) tournaments() (Tournaments, error) {
	sn, err := s.snapshot()
	return Tournaments{Snapshot: sn, Remote: s.opts.Remote}, err
}

// layer runs a read against the snapshot when it holds id, otherwise against
// the remote layers, and returns where the answer came from.
func (s *Service) layer(ctx context.Context, id domain.BashoID, read func(context.Context, Tournaments) error) (Provenance, error) {
	ts, err := s.tournaments()
	if err != nil {
		return Provenance{}, err
	}
	if ts.fromSnapshot(id) {
		return Provenance{Source: SnapshotSource, FetchedAt: ts.Snapshot.Manifest().GeneratedAt}, read(ctx, ts)
	}
	if ts.Remote == nil {
		return Provenance{}, fmt.Errorf("tournament %s: not in the snapshot and no remote source: %w", id, source.ErrNotYetAvailable)
	}
	return fromRemote(ctx, s.opts.RemoteName, s.opts.Clock.Now(), func(ctx context.Context) error { return read(ctx, ts) })
}

func (s *Service) basho(ctx context.Context, id domain.BashoID) (domain.Basho, Provenance, error) {
	var b domain.Basho
	p, err := s.layer(ctx, id, func(ctx context.Context, ts Tournaments) (err error) {
		b, err = ts.Basho(ctx, id)
		return err
	})
	return b, p, err
}

func (s *Service) banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, Provenance, error) {
	var e []domain.BanzukeEntry
	p, err := s.layer(ctx, id, func(ctx context.Context, ts Tournaments) (err error) {
		e, err = ts.Banzuke(ctx, id, div)
		return err
	})
	return e, p, err
}

// torikumi returns a division's bouts of one day. Days of tournaments after
// the snapshot also go into the bout index and the ratings.
func (s *Service) torikumi(ctx context.Context, b domain.Basho, div domain.Division, day int) ([]domain.Bout, Provenance, error) {
	var bouts []domain.Bout
	p, err := s.layer(ctx, b.ID, func(ctx context.Context, ts Tournaments) (err error) {
		bouts, err = ts.Torikumi(ctx, b.ID, div, day)
		return err
	})
	if err == nil && p.Source != SnapshotSource {
		err = s.addLater(b, div, day, bouts)
	}
	return bouts, p, err
}

// addLater puts a remote day into the bout index and the ratings.
func (s *Service) addLater(b domain.Basho, div domain.Division, day int, bouts []domain.Bout) error {
	x, err := s.index()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.later[b.ID]
	if t == nil {
		t = &laterTournament{days: map[dayKey][]domain.Bout{}}
		s.later[b.ID] = t
	}
	t.basho = b
	t.days[dayKey{div, day}] = slices.Clone(bouts)
	var all []domain.Bout
	for _, k := range slices.SortedFunc(maps.Keys(t.days), func(a, b dayKey) int {
		if a.day != b.day {
			return a.day - b.day
		}
		return int(a.div - b.div)
	}) {
		all = append(all, t.days[k]...)
	}
	x.Set(b, all)
	r, err := s.ratings()
	if err != nil {
		return err
	}
	// An older tournament than the newest rated keeps its ratings as they
	// were: ratings move forward only (SPEC §6.8).
	if err := r.Set(b.ID, all); err != nil && !errors.Is(err, rating.ErrOutOfOrder) {
		return err
	}
	return nil
}

// Ratings returns the rating model, built on first use.
func (s *Service) Ratings() (*Ratings, error) { return s.ratings() }

// Newest returns the newest tournament in the snapshot.
func (s *Service) Newest() (domain.BashoID, error) {
	sn, err := s.snapshot()
	if err != nil {
		return domain.BashoID{}, err
	}
	if sn == nil {
		return domain.BashoID{}, ErrNoSnapshot
	}
	return sn.Manifest().Newest(), nil
}

// CurrentBasho returns the current or most recent tournament by the calendar
// (D18): the one in progress, or the latest that has started.
func (s *Service) CurrentBasho() domain.BashoID {
	id, _ := domain.ParseTournament("current", s.today())
	return id
}
