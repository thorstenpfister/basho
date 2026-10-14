package cache

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/thorstenpfister/basho/internal/httpx"
)

// source is a scripted upstream: it answers with resp/err and records the
// validators it was sent.
type source struct {
	resp  *httpx.Response
	err   error
	calls int
	sent  httpx.Validators
}

func (s *source) Get(_ context.Context, _ string, v httpx.Validators) (*httpx.Response, error) {
	s.calls++
	s.sent = v
	return s.resp, s.err
}

func hourly(string, int, []byte, time.Time) (time.Duration, bool) { return time.Hour, false }
func pinned(string, int, []byte, time.Time) (time.Duration, bool) { return 0, true }

const url = "https://api.test/x"

func TestGetterFreshStaleAndRevalidate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		up := &source{resp: &httpx.Response{Status: 200, Body: []byte("v1"), Validators: httpx.Validators{ETag: `"1"`}}}
		g := &Getter{Cache: open(t, 0), Next: up, Policy: hourly}

		get := func() *httpx.Response {
			t.Helper()
			resp, err := g.Get(t.Context(), url, httpx.Validators{})
			if err != nil {
				t.Fatal(err)
			}
			return resp
		}
		start := time.Now()
		if r := get(); string(r.Body) != "v1" || up.calls != 1 || r.Cached {
			t.Fatalf("first: %q after %d requests, cached %v", r.Body, up.calls, r.Cached)
		}
		time.Sleep(30 * time.Minute)
		if r := get(); string(r.Body) != "v1" || up.calls != 1 || !r.Cached || !r.FetchedAt.Equal(start) {
			t.Fatalf("fresh: %q after %d requests, cached %v at %v; want no request, fetched at the start", r.Body, up.calls, r.Cached, r.FetchedAt)
		}

		// Stale: a conditional request; a 304 keeps the body and renews it.
		time.Sleep(time.Hour)
		up.resp = &httpx.Response{Status: 304}
		if r := get(); string(r.Body) != "v1" || r.Stale || up.calls != 2 || up.sent.ETag != `"1"` || !r.FetchedAt.Equal(time.Now()) {
			t.Fatalf("revalidated: %q stale=%v, %d requests, sent %+v, fetched %v", r.Body, r.Stale, up.calls, up.sent, r.FetchedAt)
		}
		if get(); up.calls != 2 {
			t.Error("a 304 did not renew the entry")
		}

		// Stale again, and changed upstream: the new body replaces the old.
		time.Sleep(2 * time.Hour)
		up.resp = &httpx.Response{Status: 200, Body: []byte("v2")}
		if r := get(); string(r.Body) != "v2" {
			t.Errorf("changed: %q", r.Body)
		}
	})
}

func TestGetterServesStaleWhenTheSourceFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		up := &source{resp: &httpx.Response{Status: 200, Body: []byte("v1")}}
		g := &Getter{Cache: open(t, 0), Next: up, Policy: hourly}
		if _, err := g.Get(t.Context(), url, httpx.Validators{}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Hour)
		for name, fail := range map[string]*source{
			"transport error": {err: errors.New("connection refused")},
			"server error":    {resp: &httpx.Response{Status: 503}},
		} {
			g.Next = fail
			r, err := g.Get(t.Context(), url, httpx.Validators{})
			if err != nil || string(r.Body) != "v1" || !r.Stale {
				t.Errorf("%s: %v, %v; want the stale copy", name, r, err)
			}
		}
		// Our own cancellation is not a source failure: no stale copy.
		g.Next = &source{err: context.Canceled}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := g.Get(ctx, url, httpx.Validators{}); err == nil {
			t.Error("a cancelled request returned the stale copy")
		}
	})
}

func TestGetterOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		up := &source{resp: &httpx.Response{Status: 200, Body: []byte("v1")}}
		g := &Getter{Cache: open(t, 0), Next: up, Policy: hourly}
		if _, err := g.Get(t.Context(), url, httpx.Validators{}); err != nil {
			t.Fatal(err)
		}
		g.Offline = true
		time.Sleep(48 * time.Hour)
		if r, err := g.Get(t.Context(), url, httpx.Validators{}); err != nil || string(r.Body) != "v1" || !r.Stale {
			t.Errorf("offline, cached: %v, %v", r, err)
		}
		if _, err := g.Get(t.Context(), url+"/other", httpx.Validators{}); !errors.Is(err, ErrOffline) {
			t.Errorf("offline, not cached: err = %v, want ErrOffline", err)
		}
		if up.calls != 1 {
			t.Errorf("requests = %d, want none while offline", up.calls)
		}
	})
}

func TestGetterPinnedIsFreshForever(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		up := &source{resp: &httpx.Response{Status: 200, Body: []byte("final")}}
		c := open(t, 0)
		g := &Getter{Cache: c, Next: up, Policy: pinned}
		_, _ = g.Get(t.Context(), url, httpx.Validators{})
		time.Sleep(10 * 365 * 24 * time.Hour)
		if r, _ := g.Get(t.Context(), url, httpx.Validators{}); string(r.Body) != "final" || up.calls != 1 {
			t.Errorf("after ten years: %q, %d requests", r.Body, up.calls)
		}
		if e, _ := c.Get(url); !e.Pinned {
			t.Error("entry not stored as pinned")
		}
	})
}

func TestGetterPinsEntriesThatBecomeFinal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		final := time.Now().Add(time.Hour)
		policy := func(string, int, []byte, time.Time) (time.Duration, bool) {
			return 10 * time.Minute, !time.Now().Before(final)
		}
		c := open(t, 0)
		g := &Getter{Cache: c, Next: &source{resp: &httpx.Response{Status: 200, Body: []byte("b")}}, Policy: policy}
		_, _ = g.Get(t.Context(), url, httpx.Validators{})
		if e, _ := c.Get(url); e.Pinned {
			t.Fatal("pinned before it was final")
		}
		time.Sleep(2 * time.Hour)
		_, _ = g.Get(t.Context(), url, httpx.Validators{})
		if e, _ := c.Get(url); !e.Pinned {
			t.Error("not pinned once final")
		}
	})
}

func TestGetterCachesNotFoundButNotOtherErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		up := &source{resp: &httpx.Response{Status: 404}}
		g := &Getter{Cache: open(t, 0), Next: up, Policy: hourly}
		get := func(u string, want int) {
			t.Helper()
			if r, err := g.Get(t.Context(), u, httpx.Validators{}); err != nil || r.Status != want {
				t.Fatalf("%s: %v, %v; want status %d", u, r, err, want)
			}
		}
		get(url, 404)
		get(url, 404)
		if up.calls != 1 {
			t.Errorf("requests = %d, want a cached 404", up.calls)
		}
		up.resp = &httpx.Response{Status: 400}
		get(url+"/bad", 400)
		get(url+"/bad", 400)
		if up.calls != 3 {
			t.Errorf("requests = %d, want every 400 to go upstream", up.calls)
		}
		// The 404 expires and the resource appears.
		time.Sleep(2 * time.Hour)
		up.resp = &httpx.Response{Status: 200, Body: []byte("now")}
		get(url, 200)
	})
}
