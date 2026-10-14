package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/thorstenpfister/basho/internal/httpx"
)

// Result is every use case's answer: the value and where it came from
// (SPEC §5.2).
type Result[T any] struct {
	Value T
	Prov  Provenance
}

// Provenance says where data came from and how fresh it is.
type Provenance struct {
	Source          string     // "snapshot", "sumo-api", "cache:sumo-api"
	FetchedAt       time.Time  // the snapshot's build time, or the oldest response used
	SourceUpdatedAt *time.Time // e.g. osada's resultUpdatedAt (Phase 9)
	Stale           bool       // served from an expired cache entry because the source failed
}

// SnapshotSource is the provenance label of embedded data.
const SnapshotSource = "snapshot"

// merge combines the provenance of two parts of one answer: the liveliest
// source names it (network, then cache, then snapshot), staleness spreads,
// and the older fetch time counts.
func merge(a, b Provenance) Provenance {
	switch {
	case a.Source == "":
		return b
	case b.Source == "":
		return a
	}
	out := a
	if liveliness(b.Source) > liveliness(a.Source) {
		out.Source = b.Source
	}
	if b.FetchedAt.Before(out.FetchedAt) {
		out.FetchedAt = b.FetchedAt
	}
	if out.SourceUpdatedAt == nil {
		out.SourceUpdatedAt = b.SourceUpdatedAt
	}
	out.Stale = a.Stale || b.Stale
	return out
}

func liveliness(source string) int {
	switch {
	case source == SnapshotSource:
		return 0
	case strings.HasPrefix(source, "cache:"):
		return 1
	default:
		return 2
	}
}

// recorder collects the responses of one remote call.
type recorder struct {
	mu     sync.Mutex
	n      int
	cached int
	stale  bool
	oldest time.Time
}

type recorderKey struct{}

// Recording wraps the getter under a source adapter so that the service
// learns, per call, whether the answer came from the cache, how old it is
// and whether it is stale.
func Recording(g httpx.Getter, now func() time.Time) httpx.Getter {
	return recordingGetter{next: g, now: now}
}

type recordingGetter struct {
	next httpx.Getter
	now  func() time.Time
}

func (g recordingGetter) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	resp, err := g.next.Get(ctx, url, v)
	if rec, ok := ctx.Value(recorderKey{}).(*recorder); ok && err == nil {
		at := resp.FetchedAt
		if at.IsZero() {
			at = g.now()
		}
		rec.mu.Lock()
		rec.n++
		if resp.Cached {
			rec.cached++
		}
		rec.stale = rec.stale || resp.Stale
		if rec.oldest.IsZero() || at.Before(rec.oldest) {
			rec.oldest = at
		}
		rec.mu.Unlock()
	}
	return resp, err
}

// fromRemote runs fn against a remote source and returns its provenance.
func fromRemote(ctx context.Context, name string, now time.Time, fn func(context.Context) error) (Provenance, error) {
	rec := &recorder{}
	err := fn(context.WithValue(ctx, recorderKey{}, rec))
	rec.mu.Lock()
	defer rec.mu.Unlock()
	p := Provenance{Source: name, FetchedAt: now, Stale: rec.stale}
	if rec.n > 0 {
		p.FetchedAt = rec.oldest
		if rec.cached == rec.n {
			p.Source = "cache:" + name
		}
	}
	return p, err
}
