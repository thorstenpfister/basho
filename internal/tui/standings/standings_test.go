package standings

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

var (
	testNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)
	aki2026 = domain.BashoID{Year: 2026, Month: time.September}
)

var embedded = sync.OnceValue(func() *service.Service {
	return service.New(service.Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
})

// fixture is the fixture snapshot (196001, 202607, 202609), stable across
// snapshot updates, for golden files.
func fixture() *service.Service {
	return service.New(service.Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../../testdata/snapshot")) },
		Params:   func() (rating.File, error) { return rating.File{}, nil },
		Clock:    clock.NewFake(testNow),
	})
}

func open(svc *service.Service, div domain.Division, day, width, height int) *tuitest.Driver {
	ctx := &tui.Context{Svc: svc, Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers}
	return tuitest.New(tui.New(ctx, []tui.Screen{New(aki2026, div, day)}, nil), width, height)
}

func screen(d *tuitest.Driver) View { return d.M.(tui.Model).Top().(View) }

func TestGolden(t *testing.T) {
	for _, tt := range []struct {
		name          string
		div           domain.Division
		day           int
		width, height int
		keys          []string
	}{
		{"race", domain.DivMakuuchi, 13, 120, 30, nil},
		{"race_expanded", domain.DivMakuuchi, 13, 120, 40, []string{"enter"}},
		{"race_final", domain.DivMakuuchi, 15, 100, 30, nil},
		{"race_playoff", domain.DivJuryo, 15, 100, 30, nil},
		{"race_compact", domain.DivMakuuchi, 13, 70, 24, nil},
		{"grid", domain.DivMakuuchi, 13, 120, 30, nil},
		{"grid_compact", domain.DivMakuuchi, 13, 70, 24, nil},
		{"popup", domain.DivMakuuchi, 13, 100, 30, []string{"j", "j", "enter"}},
		{"day1", domain.DivMakuuchi, 1, 100, 30, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := open(fixture(), tt.div, tt.day, tt.width, tt.height)
			if strings.HasPrefix(tt.name, "grid") || tt.name == "popup" {
				d.Keys("tab")
			}
			d.Keys(tt.keys...)
			golden.RequireEqual(t, d.View())
		})
	}
}

func TestCutoff(t *testing.T) {
	d := open(embedded(), domain.DivMakuuchi, 13, 100, 30)
	d.Keys("right")
	if v := d.View(); !strings.Contains(v, "Standings after Day 14") || !strings.Contains(v, "11-3") {
		t.Fatalf("after →:\n%s", v)
	}
	d.Keys("left", "left", "left")
	if s := screen(d); s.day != 11 || s.data.Day != 11 {
		t.Errorf("day %d, data for %d; want 11", s.day, s.data.Day)
	}
	d.Keys("right", "right", "right", "right", "right")
	if got := screen(d).day; got != 15 {
		t.Fatalf("day %d", got)
	}
	d.Keys("right")
	if got := screen(d).day; got != 15 {
		t.Errorf("past day 15: %d", got)
	}
}

func TestDivision(t *testing.T) {
	d := open(embedded(), domain.DivMakuuchi, 15, 100, 30)
	d.Keys("d")
	if v := d.View(); !strings.Contains(v, "Juryo") || !strings.Contains(v, "Playoff") && !strings.Contains(v, "Yusho:") {
		t.Fatalf("juryo:\n%s", v)
	}
}

// The popup takes esc and q; the screen stays.
func TestPopupKeys(t *testing.T) {
	d := open(embedded(), domain.DivMakuuchi, 13, 100, 30)
	d.Keys("tab", "enter")
	if !screen(d).popup {
		t.Fatal("no popup")
	}
	d.Keys("tab", "right")
	if s := screen(d); !s.popup || s.day != 13 || s.tab != Grid {
		t.Errorf("keys reached the screen under the popup: %+v", s)
	}
	for _, k := range []string{"esc", "q"} {
		d.Keys("enter", k)
		if screen(d).popup {
			t.Errorf("%s left the popup open", k)
		}
	}
}

// Every wrestler's opponent has a name in the popup, Makushita visitors
// included.
func TestOpponents(t *testing.T) {
	res, err := embedded().Standings(t.Context(), aki2026, domain.DivJuryo, 15)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res.Value.Rows {
		for i, day := range r.Results {
			if day.OpponentID != 0 && res.Value.Opponents[day.OpponentID].En == "" {
				t.Errorf("%s day %d: opponent %d has no name", r.Shikona.En, i+1, day.OpponentID)
			}
		}
	}
}

func TestScroll(t *testing.T) {
	d := open(embedded(), domain.DivMakuuchi, 13, 100, 20)
	d.Keys("tab")
	for range 30 {
		d.Keys("j")
	}
	v := d.View()
	if !strings.Contains(v, "▶") {
		t.Fatalf("cursor scrolled out of view:\n%s", v)
	}
	if lines := strings.Split(v, "\n"); len(lines) != 20 {
		t.Errorf("%d lines", len(lines))
	}
}
