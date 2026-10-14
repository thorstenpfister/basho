package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/source"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/resolver"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// pending is osada.us with some of day 13's Makuuchi bouts not decided yet.
type pending struct {
	next  source.CurrentSource
	after int // bouts after this match number are undecided; 0: all decided
}

func (p *pending) Current(ctx context.Context) (source.Current, error) {
	cur, err := p.next.Current(ctx)
	if err != nil || p.after == 0 {
		return cur, err
	}
	bouts := cur.Days[12].Bouts[domain.DivMakuuchi]
	for i := range bouts {
		if bouts[i].MatchNo > p.after {
			bouts[i].Winner, bouts[i].Kimarite = 0, ""
		}
	}
	return cur, nil
}

// noBoutCache is sumo-api with a cache that lacks the bout lists, so it does
// not fill in what osada has undecided.
type noBoutCache struct{ next httpx.Getter }

func (n noBoutCache) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	if cache.IsCachedOnly(ctx) && strings.Contains(url, "/torikumi/") {
		return nil, cache.ErrOffline
	}
	return n.next.Get(ctx, url, v)
}

type activeWrestlers struct{}

func (activeWrestlers) ActiveRikishi(context.Context) ([]domain.Rikishi, error) {
	sn, err := data.Snapshot()
	if err != nil {
		return nil, err
	}
	return sn.Rikishi()
}

// watchDeps is Aki 2026 as if day 13 were running at 16:00, without the
// snapshot; every sleep advances the clock and lets osada publish two more
// bouts.
func watchDeps(t *testing.T, at time.Time, after int) (Deps, *clock.Fake, *[]time.Duration) {
	t.Helper()
	clk := clock.NewFake(at)
	src := &pending{next: osada.New(fixture.Getter{Dir: "../../testdata/osada", Base: osada.DefaultBaseURL}, osada.DefaultBaseURL), after: after}
	res := &resolver.Resolver{
		Historical: sumoapi.New(noBoutCache{fixture.Getter{Dir: "../../testdata/sumoapi", Base: sumoapi.DefaultBaseURL}}, sumoapi.DefaultBaseURL),
		Current:    src, Now: clk.Now,
	}
	svc := service.New(service.Options{Remote: res, Wrestlers: activeWrestlers{}, Clock: clk})
	res.IDs = svc.IDs
	deps := fixtureDeps(t)
	deps.Open = func(config.Config) (*service.Service, error) { return svc, nil }
	deps.Now = clk.Now
	var sleeps []time.Duration
	deps.Sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		clk.Advance(d)
		if src.after > 0 {
			src.after = min(src.after+2, 19)
			if src.after == 19 {
				src.after = 0
			}
		}
		return nil
	}
	return deps, clk, &sleeps
}

var day13At16 = time.Date(2026, time.September, 25, 16, 0, 0, 0, domain.JST)

func TestWatch(t *testing.T) {
	deps, _, sleeps := watchDeps(t, day13At16, 15)
	out, code := run(t, deps, "watch", "--division", "makuuchi")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// 15 bouts at once, then 17, 19 (two per poll), and the end.
	if len(lines) != 20 || !strings.HasSuffix(lines[19], "Day complete") {
		t.Fatalf("%d lines:\n%s", len(lines), out)
	}
	if want := "16:00 JST  Makuuchi #1    M16w Toshinofuji  ○ oshidashi ●  M13w Abi"; lines[0] != want {
		t.Errorf("first line %q\nwant       %q", lines[0], want)
	}
	if !strings.Contains(lines[15], " JST  Makuuchi #16") || !strings.Contains(lines[18], "#19   Y1e Onosato  ● shitatenage ○  O1e Kirishima") {
		t.Errorf("later lines:\n%s", strings.Join(lines[15:], "\n"))
	}
	if len(*sleeps) != 2 || (*sleeps)[0] < 2*time.Minute {
		t.Errorf("sleeps %v", *sleeps)
	}
}

func TestWatchJSON(t *testing.T) {
	deps, _, _ := watchDeps(t, day13At16, 0)
	out, code := run(t, deps, "watch", "--json")
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 14+19+1 {
		t.Fatalf("%d lines", len(lines))
	}
	var first, last watchEvent
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatal(err)
	}
	if first.Event != "bout" || first.Division != "juryo" || first.Bout == nil || first.Bout.Winner == nil || first.Day != 13 {
		t.Errorf("first %+v", first)
	}
	if last.Event != "complete" {
		t.Errorf("last %+v", last)
	}
}

func TestWatchOutsideTheWindow(t *testing.T) {
	// Before the window: watch waits for it, then prints.
	deps, _, sleeps := watchDeps(t, time.Date(2026, time.September, 25, 9, 0, 0, 0, domain.JST), 0)
	out, code := run(t, deps, "watch", "--division", "juryo")
	if code != 0 || !strings.Contains(out, "14:00 JST  Juryo") || (*sleeps)[0] != 5*time.Hour {
		t.Errorf("exit %d, sleeps %v:\n%s", code, *sleeps, out)
	}
	// Between tournaments: exit 3 with the next tournament.
	deps = fixtureDeps(t)
	if out, code := run(t, deps, "watch"); code != exitNotFound {
		t.Errorf("between tournaments: exit %d\n%s", code, out)
	}
}
