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

type cachedOnlyKey struct{}

// CachedOnly marks ctx so that requests are answered from the cache only,
// like Offline, for one call: browsing that must never trigger requests
// (the TUI's picker, SPEC §8.3).
func CachedOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, cachedOnlyKey{}, true)
}

func cachedOnly(ctx context.Context) bool { return IsCachedOnly(ctx) }

// IsCachedOnly reports whether ctx is marked by CachedOnly.
func IsCachedOnly(ctx context.Context) bool {
	v, _ := ctx.Value(cachedOnlyKey{}).(bool)
	return v
}

type revalidateKey struct{}

// Revalidate marks ctx so that fresh entries are revalidated with a
// conditional request anyway: a manual refresh (R, SPEC §8.8). Pinned
// entries are final and still answer from the cache.
func Revalidate(ctx context.Context) context.Context {
	return context.WithValue(ctx, revalidateKey{}, true)
}

func revalidate(ctx context.Context) bool { return Revalidating(ctx) }

// WithoutRevalidate undoes Revalidate for one request: a file that changes
// rarely beside one that is polled (osada.us's banzuke).
func WithoutRevalidate(ctx context.Context) context.Context {
	return context.WithValue(ctx, revalidateKey{}, false)
}

// Revalidating reports whether ctx is marked by Revalidate.
func Revalidating(ctx context.Context) bool {
	v, _ := ctx.Value(revalidateKey{}).(bool)
	return v
}

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
		if entry.Pinned || pinned || time.Since(entry.FetchedAt) <= ttl && !revalidate(ctx) {
			return fromEntry(entry, false), nil
		}
	}
	if g.Offline || cachedOnly(ctx) {
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
		entry.FetchedAt = time.Now()
		if err := g.Cache.Touch(url, entry.FetchedAt, keep(resp.Validators, entry.Validators)); err != nil {
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
	return &httpx.Response{Status: e.Status, Body: e.Body, Validators: e.Validators, Stale: stale, Cached: true, FetchedAt: e.FetchedAt}
}

// keep prefers fresh validators, but a 304 may omit them.
func keep(fresh, old httpx.Validators) httpx.Validators {
	if fresh == (httpx.Validators{}) {
		return old
	}
	return fresh
}
