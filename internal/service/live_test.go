package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/resolver"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// server serves recorded files with ETags and counts what it answered.
type server struct {
	dir string

	mu       sync.Mutex
	requests []string // "200 /api/v1/torikumi.json", "304 …"
	down     bool
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := func(status int) { s.requests = append(s.requests, http.StatusText(status)+" "+r.URL.Path) }
	if s.down { // 429: no retries, the host pauses
		record(http.StatusTooManyRequests)
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	path := r.URL.Path
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	body, err := os.ReadFile(filepath.Join(s.dir, fixture.Name(path)))
	if err != nil {
		record(http.StatusNotFound)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		record(http.StatusNotModified)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	record(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *server) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.requests
	s.requests = nil
	return r
}

type liveWrestlers struct{}

func (liveWrestlers) ActiveRikishi(ctx context.Context) ([]domain.Rikishi, error) {
	sn, err := data.Snapshot()
	if err != nil {
		return nil, err
	}
	return sn.Rikishi()
}

// liveStack is service.Open's stack over test servers on day 13 of Aki
// 2026, without the snapshot.
func liveStack(t *testing.T) (*Service, *server, *server, *clock.Fake) {
	t.Helper()
	os, sumo := &server{dir: "../../testdata/osada"}, &server{dir: "../../testdata/sumoapi"}
	osSrv, sumoSrv := httptest.NewServer(os), httptest.NewServer(sumo)
	t.Cleanup(osSrv.Close)
	t.Cleanup(sumoSrv.Close)
	c, err := cache.Open(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	client := httpx.New(httpx.Options{UserAgent: "basho-test"})
	clk := clock.NewFake(time.Date(2026, time.September, 25, 16, 0, 0, 0, domain.JST))
	cached := func(name string, p cache.Policy) httpx.Getter {
		return resolver.Record(&cache.Getter{Cache: c, Next: client, Policy: p}, name, clk.Now)
	}
	api := sumoapi.New(cached(resolver.SumoAPI, sumoapi.Freshness{}.Policy), sumoSrv.URL)
	res := &resolver.Resolver{Historical: api, Current: osada.New(cached(resolver.Osada, osada.Freshness{}.Policy), osSrv.URL), Now: clk.Now}
	svc := New(Options{Remote: res, Wrestlers: liveWrestlers{}, Clock: clk})
	res.IDs = svc.IDs
	return svc, os, sumo, clk
}

// SPEC §10.2 and §7.3: a live poll is one conditional GET to osada, a 304
// while nothing changed.
func TestLivePollConditional(t *testing.T) {
	svc, os, sumo, clk := liveStack(t)
	first, err := svc.LivePoll(t.Context(), aki2026, 13)
	if err != nil {
		t.Fatal(err)
	}
	if got := os.take(); len(got) != 2 { // torikumi.json and banzuke.json
		t.Errorf("first poll, osada: %v", got)
	}
	if got := sumo.take(); len(got) != 3 { // once: the metadata and both banzuke
		t.Errorf("first poll, sumo-api: %v", got)
	}
	if !first.Complete || first.UpdatedAt.IsZero() || first.Err() != nil || len(first.Days[domain.DivJuryo].Value.Bouts) != 14 {
		t.Errorf("first poll: complete %v, updated %s, err %v", first.Complete, first.UpdatedAt, first.Err())
	}
	for range 3 {
		clk.Advance(3 * time.Minute)
		p, err := svc.LivePoll(t.Context(), aki2026, 13)
		if err != nil {
			t.Fatal(err)
		}
		if got := os.take(); len(got) != 1 || got[0] != "Not Modified /api/v1/torikumi.json" {
			t.Errorf("osada: %v; want one 304", got)
		}
		if got := sumo.take(); len(got) != 0 {
			t.Errorf("sumo-api: %v", got)
		}
		if !p.UpdatedAt.Equal(first.UpdatedAt) || !strings.Contains(p.Prov.Source, "osada") {
			t.Errorf("poll: %s from %s", p.UpdatedAt, p.Prov.Source)
		}
	}
}

// osada fails: sumo-api answers on the same tick, and the poll reports the
// failure for the backoff.
func TestLivePollFallback(t *testing.T) {
	svc, os, sumo, clk := liveStack(t)
	if _, err := svc.LivePoll(t.Context(), aki2026, 13); err != nil {
		t.Fatal(err)
	}
	os.take()
	sumo.take()
	os.down = true
	for range 2 {
		clk.Advance(3 * time.Minute)
		p, err := svc.LivePoll(t.Context(), aki2026, 13)
		if err != nil {
			t.Fatal(err)
		}
		if p.Err() == nil || p.Prov.Failed != resolver.Osada || !strings.Contains(p.Prov.Source, resolver.SumoAPI) {
			t.Errorf("err %v, failed %q, source %q", p.Err(), p.Prov.Failed, p.Prov.Source)
		}
		// The day's two bout lists; none once sumo-api's copies are final
		// in the cache (the cache judges by the real date).
		if got := sumo.take(); len(got) > 2 {
			t.Errorf("sumo-api: %v; want the day's bout lists", got)
		}
		if got := os.take(); len(got) > 1 {
			t.Errorf("osada: %v; want one failed revalidation at most (then the host is paused)", got)
		}
	}
}
