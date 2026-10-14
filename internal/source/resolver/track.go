package resolver

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/thorstenpfister/basho/internal/httpx"
)

// Track records, for one call through the resolver, which sources answered
// and how fresh their responses were (SPEC §5.4: "records which one
// answered"). The service turns it into the status bar's provenance.
type Track struct {
	mu        sync.Mutex
	responses map[string]*responses // by source name, every response seen
	answered  []string              // the sources whose data is in the answer, in order
	updatedAt time.Time             // the source's own update stamp (osada's resultUpdatedAt)
	forced    bool                  // --source turned the resolver off
	bouts     map[int]string        // Torikumi: each bout's source by match number
}

type responses struct {
	n, cached int
	stale     bool
	oldest    time.Time
}

type trackKey struct{}

// Tracking returns ctx carrying a new Track.
func Tracking(ctx context.Context) (context.Context, *Track) {
	t := &Track{}
	return context.WithValue(ctx, trackKey{}, t), t
}

func trackOf(ctx context.Context) *Track {
	t, _ := ctx.Value(trackKey{}).(*Track)
	return t
}

// Record wraps an adapter's getter so that its responses are recorded under
// the source's name in the call's Track.
func Record(g httpx.Getter, name string, now func() time.Time) httpx.Getter {
	return recording{next: g, name: name, now: now}
}

type recording struct {
	next httpx.Getter
	name string
	now  func() time.Time
}

func (g recording) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	resp, err := g.next.Get(ctx, url, v)
	if t := trackOf(ctx); t != nil && err == nil {
		at := resp.FetchedAt
		if at.IsZero() {
			at = g.now()
		}
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.responses == nil {
			t.responses = map[string]*responses{}
		}
		r := t.responses[g.name]
		if r == nil {
			r = &responses{}
			t.responses[g.name] = r
		}
		r.n++
		if resp.Cached {
			r.cached++
		}
		r.stale = r.stale || resp.Stale
		if r.oldest.IsZero() || at.Before(r.oldest) {
			r.oldest = at
		}
	}
	return resp, err
}

// answer marks a source as part of the answer.
func (t *Track) answer(name string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !slices.Contains(t.answered, name) {
		t.answered = append(t.answered, name)
	}
}

func (t *Track) setUpdated(at time.Time) {
	if t == nil || at.IsZero() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if at.After(t.updatedAt) {
		t.updatedAt = at
	}
}

func (t *Track) setForced() {
	if t != nil {
		t.mu.Lock()
		t.forced = true
		t.mu.Unlock()
	}
}

func (t *Track) setBouts(bouts map[int]string) {
	if t != nil {
		t.mu.Lock()
		t.bouts = bouts
		t.mu.Unlock()
	}
}

// stale reports whether a source's responses so far include a stale one.
func (t *Track) stale(name string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.responses[name]
	return r != nil && r.stale
}

// Answered returns the sources whose data is in the answer, in the order
// they were used; empty when nothing went through the resolver.
func (t *Track) Answered() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return slices.Clone(t.answered)
}

// Responses summarises the responses of the given sources (all of them
// when none are given): how many, how many from the cache, whether any was
// stale, and the oldest fetch time.
func (t *Track) Responses(names ...string) (n, cached int, stale bool, oldest time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(names) == 0 {
		names = slices.Collect(maps.Keys(t.responses))
	}
	for _, name := range names {
		r := t.responses[name]
		if r == nil {
			continue
		}
		n += r.n
		cached += r.cached
		stale = stale || r.stale
		if !r.oldest.IsZero() && (oldest.IsZero() || r.oldest.Before(oldest)) {
			oldest = r.oldest
		}
	}
	return n, cached, stale, oldest
}

// UpdatedAt returns the source's own update stamp; zero when unknown.
func (t *Track) UpdatedAt() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.updatedAt
}

// Forced reports that --source chose the source.
func (t *Track) Forced() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.forced
}

// BoutSources returns each bout's source by match number, for a Torikumi
// call that merged sources; nil otherwise.
func (t *Track) BoutSources() map[int]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return maps.Clone(t.bouts)
}
