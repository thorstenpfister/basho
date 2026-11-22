package tui_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

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
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/day"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// changing is osada.us with a change applied to its tournament, and a
// switch to fail.
type changing struct {
	next source.CurrentSource
	edit func(*source.Current)
	down bool
}

func (c *changing) Current(ctx context.Context) (source.Current, error) {
	if c.down {
		return source.Current{}, source.ErrUpstream
	}
	cur, err := c.next.Current(ctx)
	if err == nil && c.edit != nil {
		c.edit(&cur)
	}
	return cur, err
}

// undecided takes the result of a day's bout away.
func undecided(day, match int) func(*source.Current) {
	return func(c *source.Current) {
		for _, bouts := range c.Days[day-1].Bouts {
			for i := range bouts {
				if bouts[i].MatchNo == match {
					bouts[i].Winner, bouts[i].Kimarite = 0, ""
				}
			}
		}
	}
}

type liveApp struct {
	*tuitest.Driver
	clk   *clock.Fake
	osada *changing
}

func newLiveApp(t *testing.T, now time.Time, auto bool, setup ...func(*changing)) liveApp {
	t.Helper()
	clk := clock.NewFake(now)
	og := resolver.Record(fixture.Getter{Dir: "../../testdata/osada", Base: osada.DefaultBaseURL}, resolver.Osada, clk.Now)
	ch := &changing{next: osada.New(og, osada.DefaultBaseURL), edit: undecided(13, 19)}
	for _, f := range setup {
		f(ch)
	}
	held := func() bool { return ch.edit != nil }
	sg := resolver.Record(behind{fixture.Getter{Dir: "../../testdata/sumoapi", Base: sumoapi.DefaultBaseURL}, held}, resolver.SumoAPI, clk.Now)
	res := &resolver.Resolver{Historical: sumoapi.New(sg, sumoapi.DefaultBaseURL), Current: ch, Now: clk.Now}
	svc := service.New(service.Options{Remote: res, Wrestlers: wrestlers{}, Clock: clk})
	res.IDs = svc.IDs
	cfg := config.Default()
	cfg.Live.Auto = auto
	ctx := &tui.Context{Svc: svc, Cfg: cfg, Theme: theme.New(true, false), Clock: clk, Tick: tui.NoTimers, Rand: func() float64 { return 0.5 }}
	d := tuitest.New(tui.New(ctx, []tui.Screen{day.New(ctx, aki2026, 13)}, nil), 120, 32)
	return liveApp{d, clk, ch}
}

func (a liveApp) status() string { return statusLine(a.View()) }

func (a liveApp) row(match int) string {
	for _, l := range strings.Split(a.View(), "\n") {
		if f := strings.Fields(l); len(f) > 0 && (f[0] == strconv.Itoa(match) || len(f) > 1 && f[0] == "▶" && f[1] == strconv.Itoa(match)) {
			return l
		}
	}
	return ""
}

func TestLiveMode(t *testing.T) {
	a := newLiveApp(t, day13, true)
	// live.auto in the window: on at start, one poll.
	if s := a.status(); !strings.Contains(s, "● LIVE · next 3:00 · sumo-api + osada 17:27 JST") {
		t.Fatalf("status %q", s)
	}
	if r := a.row(19); !strings.Contains(r, "—") || strings.Contains(r, "NEW") {
		t.Errorf("bout 19 before: %q", r)
	}
	a.Keys("j", "j", "j")

	// Bout 19 is decided: R polls at once, the bout gets NEW, the cursor
	// stays, and the day is complete.
	a.osada.edit = nil
	a.clk.Advance(40 * time.Second)
	a.Keys("R")
	if r := a.row(19); !strings.Contains(r, "shitatenage") || !strings.Contains(r, "NEW") {
		t.Errorf("bout 19 after: %q", r)
	}
	if r := a.row(4); !strings.HasPrefix(r, "▶") || strings.Contains(a.row(18), "NEW") {
		t.Errorf("cursor or badges moved:\n%s", a.View())
	}
	if s := a.status(); !strings.Contains(s, "✓ LIVE · day complete · final check 16:30") {
		t.Errorf("status %q", s)
	}
	a.clk.Advance(61 * time.Second)
	if strings.Contains(a.row(19), "NEW") {
		t.Error("NEW after 60 s")
	}

	// L turns it off and on.
	a.Keys("L")
	if strings.Contains(a.status(), "LIVE") {
		t.Errorf("off: %q", a.status())
	}
	a.Keys("L")
	if !strings.Contains(a.status(), "LIVE") {
		t.Errorf("on again: %q", a.status())
	}
}

func TestLiveRefreshCooldown(t *testing.T) {
	a := newLiveApp(t, day13, true)
	a.clk.Advance(40 * time.Second)
	a.Keys("R", "R")
	if s := a.status(); !strings.Contains(s, "Refreshed a moment ago") {
		t.Errorf("status %q", s)
	}
}

// osada down on day 15: sumo-api answers on the same tick, but its bout
// lists lack the Juryo playoff, so the day is not complete and live mode
// backs off.
func TestLiveBackoff(t *testing.T) {
	a := newLiveApp(t, time.Date(2026, time.September, 27, 16, 0, 0, 0, domain.JST), true, func(c *changing) { c.down = true })
	if s := a.status(); !strings.Contains(s, "● LIVE · ⚠ osada unreachable, using sumo-api · retry 2:00") {
		t.Errorf("status %q", s)
	}
}

func TestLiveStates(t *testing.T) {
	for _, tt := range []struct {
		name string
		now  time.Time
		want string
	}{
		{"before the window", time.Date(2026, time.September, 25, 10, 0, 0, 0, domain.JST), "◌ LIVE · window opens 14:00 JST (in 4h00m)"},
		{"after the window", time.Date(2026, time.September, 25, 19, 0, 0, 0, domain.JST), "✓ LIVE · window closed"},
		{"between tournaments", time.Date(2026, time.October, 14, 15, 0, 0, 0, domain.JST), "LIVE unavailable · next basho Kyushu, Day 1 2026-11-08"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := newLiveApp(t, tt.now, true)
			if strings.Contains(a.status(), "LIVE") {
				t.Errorf("auto outside the window: %q", a.status())
			}
			a.Keys("L")
			if s := a.status(); !strings.Contains(s, tt.want) {
				t.Errorf("status %q; want %q", s, tt.want)
			}
		})
	}
}

// Without live.auto, live mode waits for L.
func TestLiveManual(t *testing.T) {
	a := newLiveApp(t, day13, false)
	if strings.Contains(a.status(), "LIVE") {
		t.Errorf("status %q", a.status())
	}
	a.Keys("L")
	if !strings.Contains(a.status(), "● LIVE") {
		t.Errorf("status %q", a.status())
	}
}

// behind is sumo-api without day 13's Makuuchi bout 19 decided while held,
// like osada's edit: live polls ask sumo-api's bout lists first.
type behind struct {
	next httpx.Getter
	held func() bool
}

func (b behind) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	resp, err := b.next.Get(ctx, url, v)
	if err != nil || !b.held() || !strings.HasSuffix(url, "/torikumi/Makuuchi/13") {
		return resp, err
	}
	var t map[string]any
	if err := json.Unmarshal(resp.Body, &t); err != nil {
		return nil, err
	}
	for _, m := range t["torikumi"].([]any) {
		if bout := m.(map[string]any); bout["matchNo"] == float64(19) {
			bout["winnerId"], bout["winnerEn"], bout["kimarite"] = 0, "", ""
		}
	}
	held := *resp
	held.Body, err = json.Marshal(t)
	return &held, err
}
