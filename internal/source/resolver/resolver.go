package resolver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/idmap"
	"github.com/thorstenpfister/basho/internal/source"
)

// Source names, as the status bar shows them.
const (
	SumoAPI = "sumo-api"
	Osada   = "osada"
)

// Mode is the --source setting (SPEC §5.4 rule 3).
type Mode int

const (
	Auto         Mode = iota // the policy table
	ForceSumoAPI             // sumo-api for everything
	ForceOsada               // osada for the current tournament, nothing else
)

// ParseMode reads --source: auto, sumoapi or osada.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "", "auto":
		return Auto, nil
	case "sumoapi":
		return ForceSumoAPI, nil
	case "osada":
		return ForceOsada, nil
	}
	return 0, fmt.Errorf("source %q: want auto, sumoapi or osada", s)
}

// Historical is sumo-api: every tournament, by request (SPEC §5.3).
type Historical interface {
	Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error)
	Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error)
	Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error)
}

// Resolver chooses the source per request and tournament state, falls back
// between them, and merges them during the current tournament (SPEC §5.4).
// It answers the same three calls as Historical, so the service uses it in
// sumo-api's place.
type Resolver struct {
	Historical Historical
	Current    source.CurrentSource // nil: osada disabled
	// IDs maps osada's JSA IDs to sumo-api IDs; the service provides it once
	// its wrestler index is loaded.
	IDs  func() (*idmap.Map, error)
	Mode Mode
	Now  func() time.Time // nil: time.Now
	Log  *slog.Logger     // disagreements between the sources, at debug level; nil: none
}

// route is how one request is served.
type route int

const (
	sumoOnly  route = iota // finished tournaments, or forced
	osadaOnly              // forced
	current                // the policy table's current-tournament column
)

func (r *Resolver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Resolver) route(ctx context.Context, id domain.BashoID) route {
	switch r.Mode {
	case ForceSumoAPI:
		trackOf(ctx).setForced()
		return sumoOnly
	case ForceOsada:
		trackOf(ctx).setForced()
		return osadaOnly
	}
	if r.Current == nil || !IsCurrent(id, domain.DateOf(r.now())) {
		return sumoOnly
	}
	return current
}

// rankingLead is how long before day 1 a tournament counts as current: its
// banzuke is out about 13 days before (SPEC Appendix C).
const rankingLead = 14

// IsCurrent reports whether osada.us may serve a tournament as of today:
// from its banzuke until its results are final (SPEC §5.4's "in progress
// (or ranking just published)").
func IsCurrent(id domain.BashoID, today domain.Date) bool {
	return today.Compare(id.EstimatedStart().AddDays(-rankingLead)) >= 0 && today.DaysSince(id.EstimatedEnd()) < domain.FinalMargin
}

// Basho returns a tournament's metadata: sumo-api, then osada's dates.
func (r *Resolver) Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error) {
	sumo := step[domain.Basho]{name: SumoAPI, fn: func(ctx context.Context) (domain.Basho, error) { return r.Historical.Basho(ctx, id) }}
	osada := step[domain.Basho]{name: Osada, fn: func(ctx context.Context) (domain.Basho, error) {
		cur, err := r.osada(ctx, id)
		if err != nil {
			return domain.Basho{}, err
		}
		return osadaBasho(cur)
	}}
	switch r.route(ctx, id) {
	case sumoOnly:
		return try(ctx, sumo)
	case osadaOnly:
		return try(ctx, osada)
	}
	return try(ctx, settled(ctx, sumo), osada)
}

// Banzuke returns a division's banzuke: sumo-api with osada's newer results
// laid over it, or osada's alone when sumo-api fails (rule 2).
func (r *Resolver) Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	sumo := step[[]domain.BanzukeEntry]{name: SumoAPI, fn: func(ctx context.Context) ([]domain.BanzukeEntry, error) {
		return r.Historical.Banzuke(ctx, id, div)
	}}
	osada := step[[]domain.BanzukeEntry]{name: Osada, fn: func(ctx context.Context) ([]domain.BanzukeEntry, error) {
		cur, err := r.osada(ctx, id)
		if err != nil {
			return nil, err
		}
		m, err := r.mapper(ctx, cur)
		if err != nil {
			return nil, err
		}
		return m.banzuke(div), nil
	}}
	switch r.route(ctx, id) {
	case sumoOnly:
		return try(ctx, sumo)
	case osadaOnly:
		return try(ctx, osada)
	}
	sumo = settled(ctx, sumo)
	merged := step[[]domain.BanzukeEntry]{name: SumoAPI, staleOK: sumo.staleOK, fn: func(ctx context.Context) ([]domain.BanzukeEntry, error) {
		es, err := sumo.fn(ctx)
		if err != nil {
			return nil, err
		}
		cur, err := r.osada(ctx, id)
		if err != nil {
			return es, nil // osada adds nothing
		}
		m, err := r.mapper(ctx, cur)
		if err != nil {
			return es, nil
		}
		out, used := r.overlay(es, m.banzuke(div), m.today(r.now()))
		if used {
			trackOf(ctx).answer(Osada)
			trackOf(ctx).setUpdated(cur.ResultUpdatedAt)
		}
		return out, nil
	}}
	return try(ctx, merged, osada)
}

// Torikumi returns a division's bouts of one day: osada for the current
// tournament, with sumo-api's cached copy filling in what osada lacks, then
// sumo-api.
func (r *Resolver) Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	sumo := step[[]domain.Bout]{name: SumoAPI, fn: func(ctx context.Context) ([]domain.Bout, error) {
		return r.Historical.Torikumi(polled(ctx, SumoAPI), id, div, day)
	}}
	osada := func(merge bool) step[[]domain.Bout] {
		return step[[]domain.Bout]{name: Osada, fn: func(ctx context.Context) ([]domain.Bout, error) {
			cur, err := r.osadaPolled(ctx, id)
			if err != nil {
				return nil, err
			}
			m, err := r.mapper(ctx, cur)
			if err != nil {
				return nil, err
			}
			bouts, err := m.torikumi(div, day)
			if err != nil {
				return nil, err
			}
			trackOf(ctx).setUpdated(cur.ResultUpdatedAt)
			if !merge {
				return bouts, nil
			}
			// "Behind" without a request: sumo-api's copy only if cached.
			theirs, err := r.Historical.Torikumi(cache.CachedOnly(ctx), id, div, day)
			if err != nil {
				return bouts, nil
			}
			out, sources := r.mergeBouts(bouts, theirs, day == m.today(r.now()))
			if fromSumoAPI(sources) {
				trackOf(ctx).answer(SumoAPI)
				trackOf(ctx).setBouts(sources)
			}
			return out, nil
		}}
	}
	switch r.route(ctx, id) {
	case sumoOnly:
		return try(ctx, sumo)
	case osadaOnly:
		return try(ctx, osada(false))
	}
	return try(ctx, osada(true), sumo)
}

// osada returns osada.us's tournament when it is id.
//
// In a live poll it answers from the cache, even expired: only the bout
// list's read, osadaPolled, revalidates.
func (r *Resolver) osada(ctx context.Context, id domain.BashoID) (source.Current, error) {
	if live(ctx) == nil {
		return r.current(ctx, id)
	}
	cur, err := r.current(cache.CachedOnly(ctx), id)
	if errors.Is(err, cache.ErrOffline) { // never fetched: fetched now, so the poll need not revalidate
		live(ctx).osada.Store(false)
		return r.current(ctx, id)
	}
	trackOf(ctx).clearStale(Osada) // expired on purpose
	return cur, err
}

// osadaPolled is the bout list's read: revalidated once per live poll.
func (r *Resolver) osadaPolled(ctx context.Context, id domain.BashoID) (source.Current, error) {
	if live(ctx) == nil {
		return r.current(ctx, id)
	}
	return r.current(polled(ctx, Osada), id)
}

func (r *Resolver) current(ctx context.Context, id domain.BashoID) (source.Current, error) {
	if r.Current == nil {
		return source.Current{}, fmt.Errorf("osada.us is disabled: %w", source.ErrNotFound)
	}
	cur, err := r.Current.Current(ctx)
	if err != nil {
		return source.Current{}, err
	}
	if cur.Basho != id {
		return source.Current{}, fmt.Errorf("osada.us serves %s, not %s: %w", cur.Basho, id, source.ErrNotYetAvailable)
	}
	return cur, nil
}

func osadaBasho(cur source.Current) (domain.Basho, error) {
	if cur.Start.IsZero() {
		return domain.Basho{}, fmt.Errorf("osada.us %s: no schedule yet: %w", cur.Basho, source.ErrNotYetAvailable)
	}
	return domain.Basho{ID: cur.Basho, Start: cur.Start, End: domain.DayDate(cur.Start, domain.BashoDays)}, nil
}

// ErrStale is a source that could answer only from an expired cache copy:
// the source itself failed and the cache stood in.
var ErrStale = errors.New("unreachable, only a cached copy")

// step is one source to try for a request.
type step[T any] struct {
	name    string
	fn      func(context.Context) (T, error)
	staleOK bool // an expired cache copy is good enough: live polling
}

type liveKey struct{}

// tick is one live poll: each source is revalidated once in it.
type tick struct{ osada, sumo atomic.Bool }

// Live marks ctx as one live poll (SPEC §10.2). The bout list is what the
// poll is for: the first osada request of the poll revalidates (osada.us's
// banzuke never does), and so does the first sumo-api bout list, should
// osada fail. sumo-api's metadata and banzuke come from the cache, even
// expired, with osada's results laid over them. A poll thus costs one
// conditional GET to osada.
func Live(ctx context.Context) context.Context {
	t := &tick{}
	t.osada.Store(true)
	t.sumo.Store(true)
	return context.WithValue(ctx, liveKey{}, t)
}

func live(ctx context.Context) *tick {
	t, _ := ctx.Value(liveKey{}).(*tick)
	return t
}

// polled is the context of a live poll's request to a source: revalidated
// the first time in the poll.
func polled(ctx context.Context, name string) context.Context {
	t := live(ctx)
	if t == nil {
		return ctx
	}
	flag := &t.sumo
	if name == Osada {
		flag = &t.osada
	}
	if flag.CompareAndSwap(true, false) {
		return cache.Revalidate(ctx)
	}
	return ctx
}

// settled makes a sumo-api step of a live poll answer from the cache, even
// expired; only what was never fetched is fetched.
func settled[T any](ctx context.Context, s step[T]) step[T] {
	if live(ctx) == nil {
		return s
	}
	fn := s.fn
	s.fn = func(ctx context.Context) (T, error) {
		v, err := fn(cache.CachedOnly(ctx))
		if errors.Is(err, cache.ErrOffline) { // never fetched: once for real
			return fn(ctx)
		}
		return v, err
	}
	s.staleOK = true
	return s
}

// try asks the sources in order (rule 1). It moves on when a source fails
// in a way another may not, or answers only from a stale cache copy; when
// every source fails, a stale answer is better than none (rule 4).
func try[T any](ctx context.Context, steps ...step[T]) (T, error) {
	parent := trackOf(ctx)
	var (
		firstErr   error
		stale      T
		staleTrack *Track
	)
	for i, s := range steps {
		sctx, t := Tracking(ctx)
		t.answer(s.name) // first: a merge adds the other source after it
		v, err := s.fn(sctx)
		if err == nil {
			if s.staleOK {
				t.clearStale(s.name)
			}
			if t.stale(s.name) && i < len(steps)-1 {
				if staleTrack == nil {
					stale, staleTrack = v, t
				}
				parent.fail(s.name, ErrStale)
				continue
			}
			parent.absorb(t)
			return v, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if !errors.Is(err, cache.ErrOffline) { // not cached is no outage
			parent.fail(s.name, err)
		}
		if !fallsBack(err) {
			break
		}
	}
	if staleTrack != nil {
		parent.absorb(staleTrack)
		return stale, nil
	}
	var zero T
	return zero, firstErr
}

// fallsBack reports errors another source may not have: the source failed
// or is paused, or it does not have the data yet (it is behind).
func fallsBack(err error) bool {
	for _, e := range []error{source.ErrUpstream, source.ErrRateLimited, httpx.ErrCircuitOpen, cache.ErrOffline, source.ErrNotYetAvailable, source.ErrNotFound} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

// absorb adds a finished step's record to the call's.
func (t *Track) absorb(c *Track) {
	if t == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.responses == nil {
		t.responses = map[string]*responses{}
	}
	for name, r := range c.responses {
		mine := t.responses[name]
		if mine == nil {
			mine = &responses{}
			t.responses[name] = mine
		}
		mine.n += r.n
		mine.cached += r.cached
		mine.stale = mine.stale || r.stale
		if !r.oldest.IsZero() && (mine.oldest.IsZero() || r.oldest.Before(mine.oldest)) {
			mine.oldest = r.oldest
		}
	}
	for _, a := range c.answered {
		if !slices.Contains(t.answered, a) {
			t.answered = append(t.answered, a)
		}
	}
	if c.updatedAt.After(t.updatedAt) {
		t.updatedAt = c.updatedAt
	}
	if c.bouts != nil {
		t.bouts = c.bouts
	}
	t.failed = append(t.failed, c.failed...)
}

// fromSumoAPI reports whether a merge took any bout from sumo-api.
func fromSumoAPI(sources map[int]string) bool {
	for _, s := range sources {
		if s == SumoAPI {
			return true
		}
	}
	return false
}
