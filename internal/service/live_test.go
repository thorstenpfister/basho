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
		return resolver.Record(&cache.Getter{Cache: c, Next: client, Policy: p, Now: clk.Now}, name, clk.Now)
	}
	api := sumoapi.New(cached(resolver.SumoAPI, sumoapi.Freshness{}.Policy), sumoSrv.URL)
	res := &resolver.Resolver{Historical: api, Current: osada.New(cached(resolver.Osada, osada.Freshness{}.Policy), osSrv.URL), Now: clk.Now}
	svc := New(Options{Remote: res, Wrestlers: liveWrestlers{}, Clock: clk})
	res.IDs = svc.IDs
	return svc, os, sumo, clk
}

// SPEC §10.2 and §7.3: a live poll asks sumo-api's two bout lists, and
// osada's torikumi.json every OsadaEvery, conditionally.
func TestLivePollRequests(t *testing.T) {
	svc, os, sumo, clk := liveStack(t)
	first, err := svc.LivePoll(t.Context(), aki2026, 13)
	if err != nil {
		t.Fatal(err)
	}
	if got := os.take(); len(got) != 2 { // torikumi.json and banzuke.json
		t.Errorf("first poll, osada: %v", got)
	}
	if got := sumo.take(); len(got) != 5 { // the metadata, both banzuke, both bout lists
		t.Errorf("first poll, sumo-api: %v", got)
	}
	if !first.Complete || !first.Osada || first.UpdatedAt.IsZero() || first.Err() != nil || len(first.Days[domain.DivJuryo].Value.Bouts) != 14 {
		t.Errorf("first poll: complete %v, osada %v, updated %s, err %v", first.Complete, first.Osada, first.UpdatedAt, first.Err())
	}
	for i := range 5 {
		clk.Advance(3 * time.Minute)
		p, err := svc.LivePoll(t.Context(), aki2026, 13)
		if err != nil {
			t.Fatal(err)
		}
		osadaDue := i == 4 // 15 minutes after the first poll
		want := []string{}
		if osadaDue {
			want = []string{"Not Modified /api/v1/torikumi.json"}
		}
		if got := os.take(); strings.Join(got, ",") != strings.Join(want, ",") || p.Osada != osadaDue {
			t.Errorf("poll %d, osada: %v (due %v); want %v", i+2, got, p.Osada, want)
		}
		// 304s here; sumo-api itself sends no validators, so 200s for real.
		if got := sumo.take(); len(got) != 2 || !strings.HasSuffix(got[0], "/torikumi/Makuuchi/13") || !strings.HasSuffix(got[1], "/torikumi/Juryo/13") {
			t.Errorf("poll %d, sumo-api: %v; want the two bout lists", i+2, got)
		}
		if !p.UpdatedAt.Equal(first.UpdatedAt) || !strings.Contains(p.Prov.Source, "osada") || p.Err() != nil {
			t.Errorf("poll %d: %s from %s, err %v", i+2, p.UpdatedAt, p.Prov.Source, p.Err())
		}
	}
}

// One source fails: the other answers on the same poll, and the poll
// reports the failure for the backoff.
func TestLivePollFallback(t *testing.T) {
	for _, c := range []struct {
		name   string
		failed string
		down   func(os, sumo *server)
	}{
		{"sumo-api down", resolver.SumoAPI, func(_, sumo *server) { sumo.down = true }},
		{"osada down", resolver.Osada, func(os, _ *server) { os.down = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			svc, os, sumo, clk := liveStack(t)
			if _, err := svc.LivePoll(t.Context(), aki2026, 13); err != nil {
				t.Fatal(err)
			}
			os.take()
			sumo.take()
			c.down(os, sumo)
			// osada is due 15 minutes on, or at once when sumo-api fails.
			clk.Advance(15 * time.Minute)
			p, err := svc.LivePoll(t.Context(), aki2026, 13)
			if err != nil {
				t.Fatal(err)
			}
			if p.Err() == nil || p.Prov.Failed != c.failed || len(p.Days[domain.DivMakuuchi].Value.Bouts) == 0 {
				t.Errorf("err %v, failed %q, source %q", p.Err(), p.Prov.Failed, p.Prov.Source)
			}
			if got := os.take(); len(got) != 1 {
				t.Errorf("osada: %v; want one revalidation", got)
			}
			if got := sumo.take(); len(got) > 2 {
				t.Errorf("sumo-api: %v; want the two bout lists at most", got)
			}
		})
	}
}

// A sumo-api failure brings osada in before it is due.
func TestLivePollSumoDown(t *testing.T) {
	svc, os, sumo, clk := liveStack(t)
	if _, err := svc.LivePoll(t.Context(), aki2026, 13); err != nil {
		t.Fatal(err)
	}
	os.take()
	sumo.down = true
	clk.Advance(3 * time.Minute)
	p, err := svc.LivePoll(t.Context(), aki2026, 13)
	if err != nil {
		t.Fatal(err)
	}
	if got := os.take(); len(got) != 1 || got[0] != "Not Modified /api/v1/torikumi.json" {
		t.Errorf("osada: %v; want it revalidated in sumo-api's place", got)
	}
	if p.Prov.Failed != resolver.SumoAPI || !p.Complete {
		t.Errorf("failed %q, complete %v", p.Prov.Failed, p.Complete)
	}
}
