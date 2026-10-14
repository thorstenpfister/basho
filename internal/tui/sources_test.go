package tui_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/resolver"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/day"
	"github.com/thorstenpfister/basho/internal/tui/standings"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// Phase 9 AC: Aki 2026 as if it were running (day 13, 16:00 JST), without
// the snapshot, from the recorded files of both sources.

var (
	aki2026 = domain.BashoID{Year: 2026, Month: time.September}
	day13   = time.Date(2026, time.September, 25, 16, 0, 0, 0, domain.JST)
)

type down struct{}

func (down) Get(context.Context, string, httpx.Validators) (*httpx.Response, error) {
	return nil, errors.New("connection refused")
}

// wrestlers is sumo-api's active-wrestler list, for the JSA ID mapping.
type wrestlers struct{}

func (wrestlers) ActiveRikishi(context.Context) ([]domain.Rikishi, error) {
	sn, err := data.Snapshot()
	if err != nil {
		return nil, err
	}
	return sn.Rikishi()
}

func liveContext(t *testing.T, sumoUp, osadaUp bool) *tui.Context {
	t.Helper()
	var sumo, os httpx.Getter = down{}, down{}
	if sumoUp {
		sumo = fixture.Getter{Dir: "../../testdata/sumoapi", Base: sumoapi.DefaultBaseURL}
	}
	if osadaUp {
		os = fixture.Getter{Dir: "../../testdata/osada", Base: osada.DefaultBaseURL}
	}
	clk := clock.NewFake(day13)
	res := &resolver.Resolver{
		Historical: sumoapi.New(resolver.Record(sumo, resolver.SumoAPI, clk.Now), sumoapi.DefaultBaseURL),
		Current:    osada.New(resolver.Record(os, resolver.Osada, clk.Now), osada.DefaultBaseURL),
		Now:        clk.Now,
	}
	svc := service.New(service.Options{Remote: res, Wrestlers: wrestlers{}, Clock: clk})
	res.IDs = svc.IDs
	return &tui.Context{Svc: svc, Cfg: config.Default(), Theme: theme.New(true, false), Clock: clk, Tick: tui.NoTimers}
}

func statusLine(v string) string {
	lines := strings.Split(v, "\n")
	return lines[len(lines)-1]
}

func TestSources(t *testing.T) {
	for _, tt := range []struct {
		name           string
		sumoUp, osadUp bool
		source         string
	}{
		{"sumo-api down: osada alone", false, true, " osada · updated 17:27 JST"},
		{"osada down: sumo-api alone", true, false, " sumo-api · updated "},
		{"both: each for its part", true, true, " sumo-api + osada · updated 17:27 JST"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := liveContext(t, tt.sumoUp, tt.osadUp)
			d := tuitest.New(tui.New(ctx, []tui.Screen{day.New(ctx, aki2026, 13)}, nil), 120, 32)
			v := d.View()
			for _, want := range []string{"Aki Basho 2026", "Day 13 · Fri 2026-09-25", "Onosato", "Kirishima", "shitatenage"} {
				if !strings.Contains(v, want) {
					t.Errorf("day view: no %q:\n%s", want, v)
				}
			}
			if s := statusLine(v); !strings.Contains(s, tt.source) {
				t.Errorf("day view status %q; want %q", s, tt.source)
			}

			d.Keys("s")
			v = d.View()
			for _, want := range []string{"Standings after Day 13", "Onosato", "11-2"} {
				if !strings.Contains(v, want) {
					t.Errorf("standings: no %q:\n%s", want, v)
				}
			}
			if s := statusLine(v); !strings.Contains(s, tt.source) {
				t.Errorf("standings status %q; want %q", s, tt.source)
			}
			if _, ok := d.M.(tui.Model).Top().(standings.View); !ok {
				t.Error("not on the standings")
			}
		})
	}
}

// Every name has its English form through the ID mapping: none is a
// transliteration.
func TestOsadaNames(t *testing.T) {
	ctx := liveContext(t, false, true)
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		res, err := ctx.Svc.Day(t.Context(), aki2026, div, 13)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range res.Value.Bouts {
			for _, s := range []service.BoutSide{b.East, b.West} {
				if s.Shikona.Approx || s.Shikona.En == "" {
					t.Errorf("%s bout %d: %+v", div, b.MatchNo, s.Shikona)
				}
			}
		}
	}
}
