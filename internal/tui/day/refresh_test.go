package day

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// counting counts the requests that reach the "network" (the fixtures).
type counting struct {
	next  httpx.Getter
	calls atomic.Int32
}

func (c *counting) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	c.calls.Add(1)
	return c.next.Get(ctx, url, v)
}

// remoteContext is the production stack without the snapshot: sumo-api over
// the disk cache over the recorded fixtures.
func remoteContext(t *testing.T, net httpx.Getter) *tui.Context {
	t.Helper()
	c, err := cache.Open(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	fake := clock.NewFake(testNow)
	g := service.Recording(&cache.Getter{Cache: c, Next: net, Policy: sumoapi.Freshness{}.Policy}, fake.Now)
	api := sumoapi.New(g, sumoapi.DefaultBaseURL)
	svc := service.New(service.Options{Remote: api, Clock: fake})
	return &tui.Context{Svc: svc, Cfg: config.Default(), Theme: theme.New(true, false), Clock: fake, Tick: tui.NoTimers}
}

func TestStaleWhileRevalidate(t *testing.T) {
	net := &counting{next: fixture.Getter{Dir: "../../../testdata/sumoapi", Base: sumoapi.DefaultBaseURL}}
	ctx := remoteContext(t, net)
	d := tuitest.New(tui.New(ctx, []tui.Screen{New(ctx, aki2026, 13)}, nil), 100, 30)
	s := screen(d)
	if s.data == nil || s.prov.Source != "sumo-api" || s.loading {
		t.Fatalf("first open: %+v loading %v", s.prov, s.loading)
	}
	first := net.calls.Load()
	if first == 0 {
		t.Fatal("no requests on a cold cache")
	}
	// Day 1 (cold), then day 13 again: the cache's copy, then a
	// revalidation that the cache answers itself (finished: pinned).
	d.Keys("1")
	warm := net.calls.Load()
	d.Keys("g", "1", "3")
	if s := screen(d); s.day != 13 || s.prov.Source != "cache:sumo-api" || net.calls.Load() != warm {
		t.Errorf("warm: %+v after %d more requests", s.prov, net.calls.Load()-warm)
	}
	d.Keys("R")
	if !strings.Contains(d.View(), "Day 13") || net.calls.Load() != warm {
		t.Error("R refetched a final day")
	}
	d.Keys("R")
	if !strings.Contains(d.View(), "again in 30s") {
		t.Errorf("no cooldown:\n%s", d.View())
	}
}

// broken fails every request, like an unreachable source.
type broken struct{}

func (broken) Get(context.Context, string, httpx.Validators) (*httpx.Response, error) {
	return nil, context.DeadlineExceeded
}

func TestErrorState(t *testing.T) {
	ctx := remoteContext(t, broken{})
	d := tuitest.New(tui.New(ctx, []tui.Screen{New(ctx, aki2026, 13)}, nil), 100, 30)
	if v := d.View(); !strings.Contains(v, "Every source failed and nothing is cached. R retries.") {
		t.Errorf("error state:\n%s", v)
	}
	d.Keys("R")
	if s := screen(d); s.loading || s.err == nil {
		t.Error("R from the error state")
	}
}

func TestLoadingState(t *testing.T) {
	ctx := remoteContext(t, broken{})
	s := New(ctx, aki2026, 13)
	s.day, s.loading = 13, true
	if v := ansi.Strip(s.View(ctx, 100, 29)); !strings.Contains(v, "⠋ Loading day 13…") {
		t.Errorf("loading:\n%s", v)
	}
	next, cmd := s.Update(ctx, spinMsg{})
	if next.(Day).frame != 1 || cmd != nil { // NoTimers: no next frame scheduled
		t.Error("spinner frame")
	}
}
