package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/thorstenpfister/basho/internal/httpx"
)

// ErrOffline is returned in offline mode for a request that is not cached.
var ErrOffline = errors.New("offline and not cached")

// Policy decides how long a response stays fresh as of now, and whether it is
// pinned (finished tournaments: fresh forever, never evicted). It sees the
// status and body because the data class can depend on them, e.g. whether a
// yusho winner is present. The source adapters provide it (SPEC §7.2).
type Policy func(url string, status int, body []byte, now time.Time) (ttl time.Duration, pinned bool)

// Getter is an httpx.Getter that answers from the cache while fresh,
// revalidates stale entries with conditional requests, and falls back to a
// stale entry when the source fails (SPEC §7.2).
//
// Stale entries are refreshed synchronously here; showing them first and
// refreshing in the background is the service layer's job.
type Getter struct {
	Cache   *Cache
	Next    httpx.Getter
	Policy  Policy
	Offline bool // never use the network: --offline
}

// Get implements httpx.Getter. 200 and 404 answers are cached; Stale marks an
// entry served because the source could not be reached (or offline).
func (g *Getter) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	entry, cached := g.Cache.Get(url)
	if cached {
		ttl, pinned := g.Policy(url, entry.Status, entry.Body, time.Now())
		if pinned && !entry.Pinned { // became final since it was stored
			if err := g.Cache.Pin(url); err != nil {
				return nil, err
			}
		}
		if entry.Pinned || pinned || time.Since(entry.FetchedAt) <= ttl {
			return fromEntry(entry, false), nil
		}
	}
	if g.Offline {
		if cached {
			return fromEntry(entry, true), nil
		}
		return nil, fmt.Errorf("%s: %w", url, ErrOffline)
	}

	if cached && v == (httpx.Validators{}) {
		v = entry.Validators
	}
	resp, err := g.Next.Get(ctx, url, v)
	switch {
	case cached && ctx.Err() == nil && (err != nil || resp.Status >= 500):
		return fromEntry(entry, true), nil // the source failed: the stale copy is better
	case err != nil:
		return nil, err
	case resp.NotModified() && cached:
		if err := g.Cache.Touch(url, time.Now(), keep(resp.Validators, entry.Validators)); err != nil {
			return nil, err
		}
		return fromEntry(entry, false), nil
	case resp.OK() || resp.Status == 404:
		now := time.Now()
		_, pinned := g.Policy(url, resp.Status, resp.Body, now)
		e := Entry{Key: url, Status: resp.Status, FetchedAt: now, Validators: resp.Validators, Pinned: pinned, Body: resp.Body}
		if err := g.Cache.Put(e); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func fromEntry(e Entry, stale bool) *httpx.Response {
	return &httpx.Response{Status: e.Status, Body: e.Body, Validators: e.Validators, Stale: stale}
}

// keep prefers fresh validators, but a 304 may omit them.
func keep(fresh, old httpx.Validators) httpx.Validators {
	if fresh == (httpx.Validators{}) {
		return old
	}
	return fresh
}
