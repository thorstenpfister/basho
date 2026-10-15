package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/idmap"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
	"github.com/thorstenpfister/basho/internal/source/resolver"
)

// ErrNoSnapshot is returned by snapshot-only operations without a snapshot.
var ErrNoSnapshot = errors.New("no snapshot")

// Options configure a Service.
type Options struct {
	Snapshot   func() (*snapshot.Snapshot, error) // nil: --no-snapshot
	Params     func() (rating.File, error)
	Remote     TournamentSource // disk cache over the network; nil: snapshot only
	RemoteName string           // provenance label, e.g. "sumo-api"
	Wrestlers  WrestlerSource   // the active wrestlers, for the index refresh; nil: none
	Clock      clock.Clock      // nil: the real clock
}

// WrestlerSource returns the active wrestlers; *sumoapi.Client implements it.
type WrestlerSource interface {
	ActiveRikishi(ctx context.Context) ([]domain.Rikishi, error)
}

// Service is the façade the TUI and CLI talk to (SPEC §5.1). Everything heavy
// (the snapshot, the bout index, the ratings) is loaded the first time it is
// needed, so commands such as `basho version` stay instant. It is safe for
// concurrent use.
type Service struct {
	opts     Options
	snapshot func() (*snapshot.Snapshot, error)
	index    func() (*snapshot.Index, error)
	params   func() (rating.File, error)
	ratings  func() (*Ratings, error)
	profiles func() (*profiles, error)

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
	s.params = sync.OnceValues(func() (rating.File, error) {
		if opts.Params == nil {
			return rating.File{}, nil
		}
		return opts.Params()
	})
	s.ratings = sync.OnceValues(func() (*Ratings, error) {
		x, err := s.index()
		if err != nil {
			return nil, err
		}
		f, err := s.params()
		if err != nil {
			return nil, err
		}
		return NewRatings(x, f)
	})
	s.profiles = sync.OnceValues(func() (*profiles, error) {
		sn, err := s.snapshot()
		if err != nil || sn == nil {
			return s.withRefresh(newProfiles(nil)), err
		}
		rs, err := sn.Rikishi()
		if err != nil {
			return nil, err
		}
		return s.withRefresh(newProfiles(rs)), nil
	})
	return s
}

// profiles holds the wrestler index: the snapshot's, overlaid with the active
// wrestlers once refreshed. The map is replaced, never changed, so readers
// use it without a lock.
type profiles struct {
	mu   sync.RWMutex
	byID map[domain.RikishiID]domain.Rikishi
	ids  *idmap.Map
}

func newProfiles(rs []domain.Rikishi) *profiles {
	p := &profiles{byID: make(map[domain.RikishiID]domain.Rikishi, len(rs)), ids: idmap.New(rs)}
	for _, r := range rs {
		p.byID[r.ID] = r
	}
	return p
}

// withRefresh lets the JSA ID mapping look up an unknown ID among the active
// wrestlers (SPEC §5.5): revalidated with sumo-api, since the cache keeps the
// index for 7 days, and overlaid on the profiles too.
func (s *Service) withRefresh(p *profiles) *profiles {
	if w := s.opts.Wrestlers; w != nil {
		p.ids.SetRefresh(func(ctx context.Context) ([]domain.Rikishi, error) {
			rs, err := w.ActiveRikishi(Revalidate(ctx))
			if err == nil {
				p.overlay(rs)
			}
			return rs, err
		}, s.opts.Clock.Now)
	}
	return p
}

func (p *profiles) all() map[domain.RikishiID]domain.Rikishi {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.byID
}

func (p *profiles) overlay(rs []domain.Rikishi) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m := maps.Clone(p.byID)
	for _, r := range rs {
		m[r.ID] = r
	}
	p.byID = m
	p.ids.Add(rs)
}

// wrestlers returns the wrestler index as it stands.
func (s *Service) wrestlers() (map[domain.RikishiID]domain.Rikishi, error) {
	p, err := s.profiles()
	if err != nil {
		return nil, err
	}
	return p.all(), nil
}

// IDs returns the JSA ID mapping (SPEC §5.5), for the osada.us adapter.
func (s *Service) IDs() (*idmap.Map, error) {
	p, err := s.profiles()
	if err != nil {
		return nil, err
	}
	return p.ids, nil
}

// indexMaxAge is how old the snapshot's wrestler index may get before the
// active wrestlers are fetched (SPEC §7.3).
const indexMaxAge = 7 * 24 * time.Hour

// RefreshWrestlers overlays the active wrestlers' current profiles (heya,
// height, weight, retirement) when the snapshot's index is older than 7 days.
// It makes one request at most, and none while the disk cache holds the
// answer (also 7 days). It reports whether it refreshed.
func (s *Service) RefreshWrestlers(ctx context.Context) (bool, error) {
	if s.opts.Wrestlers == nil {
		return false, nil
	}
	sn, err := s.snapshot()
	if err != nil {
		return false, err
	}
	if sn != nil && s.opts.Clock.Now().Sub(sn.Manifest().GeneratedAt) <= indexMaxAge {
		return false, nil
	}
	p, err := s.profiles()
	if err != nil {
		return false, err
	}
	rs, err := s.opts.Wrestlers.ActiveRikishi(ctx)
	if err != nil {
		return false, err
	}
	p.overlay(rs)
	return true, nil
}

// StartBackground starts the background work of a long-running session (the
// TUI): the wrestler index refresh. Errors are dropped; the snapshot's index
// stays in use.
func (s *Service) StartBackground(ctx context.Context) {
	go func() { _, _ = s.RefreshWrestlers(ctx) }()
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
	p, _, err := s.layerTracked(ctx, id, read)
	return p, err
}

// layerTracked is layer that also returns the resolver's record of a remote
// call; nil for the snapshot.
func (s *Service) layerTracked(ctx context.Context, id domain.BashoID, read func(context.Context, Tournaments) error) (Provenance, *resolver.Track, error) {
	ts, err := s.tournaments()
	if err != nil {
		return Provenance{}, nil, err
	}
	if ts.fromSnapshot(id) {
		return Provenance{Source: SnapshotSource, FetchedAt: ts.Snapshot.Manifest().GeneratedAt}, nil, read(ctx, ts)
	}
	if ts.Remote == nil {
		return Provenance{}, nil, fmt.Errorf("tournament %s: not in the snapshot and no remote source: %w", id, source.ErrNotYetAvailable)
	}
	return tracked(ctx, s.opts.RemoteName, s.opts.Clock.Now(), func(ctx context.Context) error { return read(ctx, ts) })
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

// torikumi returns a division's bouts of one day, and each bout's source by
// match number when the resolver merged sources. Days of tournaments after
// the snapshot also go into the bout index and the ratings.
func (s *Service) torikumi(ctx context.Context, b domain.Basho, div domain.Division, day int) ([]domain.Bout, Provenance, map[int]string, error) {
	var bouts []domain.Bout
	p, t, err := s.layerTracked(ctx, b.ID, func(ctx context.Context, ts Tournaments) (err error) {
		bouts, err = ts.Torikumi(ctx, b.ID, div, day)
		return err
	})
	if err != nil {
		return nil, p, nil, err
	}
	var sources map[int]string
	if t != nil {
		sources = t.BoutSources()
	}
	if p.Source != SnapshotSource {
		err = s.addLater(b, div, day, bouts)
	}
	return bouts, p, sources, err
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

// CachedOnly marks ctx so that the service answers from the snapshot and the
// disk cache only, never the network: browsing such as the picker (SPEC
// §8.3). Uncached data then fails as Unavailable.
func CachedOnly(ctx context.Context) context.Context { return cache.CachedOnly(ctx) }

// Revalidate marks ctx so that cached answers are revalidated with the
// source even while fresh: the manual refresh R. Finished tournaments stay
// as they are.
func Revalidate(ctx context.Context) context.Context { return cache.Revalidate(ctx) }
