package tui_test

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/day"
	"github.com/thorstenpfister/basho/internal/tui/picker"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// Screen tests over the whole app as basho starts it (SPEC §13, Phase 7 AC).

var testNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)

// fixtureService is the fixture snapshot (196001, 202607, 202609): stable
// across snapshot updates, for golden files.
func fixtureService() *service.Service {
	return service.New(service.Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../testdata/snapshot")) },
		Params: func() (rating.File, error) {
			return rating.File{Schema: rating.FileSchema, Params: rating.Params{K: 10, KNew: 24, NewBouts: 45, R: 0.3, WarmupScale: 0.5}, Show: true}, nil
		},
		Clock: clock.NewFake(testNow),
	})
}

// embedded is the binary's snapshot without remote layers: no network.
var embedded = sync.OnceValue(func() *service.Service {
	return service.New(service.Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
})

// app builds the model like cmd/basho: the picker under the current
// tournament's latest day.
func app(svc *service.Service) tui.Model {
	return appWith(&tui.Context{Svc: svc, Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers})
}

func appWith(ctx *tui.Context) tui.Model {
	open := func(id domain.BashoID) tui.Screen { return day.New(ctx, id, 0) }
	today := func() []tui.Screen {
		return []tui.Screen{picker.New(ctx, open), day.New(ctx, ctx.Svc.CurrentBasho(), 0)}
	}
	return tui.New(ctx, today(), today)
}

func TestGolden(t *testing.T) {
	for _, size := range []struct{ w, h int }{{80, 24}, {120, 40}} {
		for _, tt := range []struct {
			name string
			keys []string
		}{
			{"start", nil},
			{"juryo", []string{"d"}},
			{"day13", []string{"g", "1", "3"}},
			{"picker", []string{"esc"}},
			{"picker_filter", []string{"esc", "/", "a", "k", "i", "enter"}},
			{"help", []string{"?"}},
			{"help_legend", []string{"?", "tab"}},
			{"help_rating", []string{"?", "tab", "tab"}},
			{"help_data", []string{"?", "tab", "tab", "tab"}},
			{"help_limits", []string{"?", "shift+tab"}},
		} {
			t.Run(fmt.Sprintf("%s_%dx%d", tt.name, size.w, size.h), func(t *testing.T) {
				d := tuitest.New(app(fixtureService()), size.w, size.h)
				d.Keys(tt.keys...)
				golden.RequireEqual(t, d.View())
			})
		}
	}
	t.Run("too_small", func(t *testing.T) {
		golden.RequireEqual(t, tuitest.New(app(fixtureService()), 50, 15).View())
	})
}

// The navigation flows through a real Bubble Tea program. The renderer
// writes only changed cells after the first frame, so the test waits for the
// first frame in the output, then lets each step's loads finish and checks
// the final model.
func TestFlow(t *testing.T) {
	for _, tt := range []struct {
		name  string
		steps [][]string
		want  string
		depth int
	}{
		{"picker to a day in Juryo", [][]string{{"esc", "/", "a", "k", "i", " ", "2", "0", "2", "6", "enter", "enter"}, {"1"}, {"d"}}, "Aki 2026 · Juryo · Day 1", 2},
		{"t goes back to today", [][]string{{"d"}, {"esc"}, {"j", "j", "enter"}, {"t"}}, "Aki 2026 · Makuuchi · Day 15", 2},
		{"esc at the picker stays", [][]string{{"esc"}, {"esc"}}, "Basho picker · 402 tournaments", 1}, // 196001–202611,
	} {
		t.Run(tt.name, func(t *testing.T) {
			tm := teatest.NewTestModel(t, app(fixtureService()), teatest.WithInitialTermSize(120, 40))
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return strings.Contains(string(b), "Day 15 · Sun 2026-09-27") },
				teatest.WithDuration(5*time.Second), teatest.WithCheckInterval(10*time.Millisecond))
			for _, step := range tt.steps {
				for _, k := range step {
					tm.Send(tuitest.Key(k))
				}
				time.Sleep(150 * time.Millisecond) // the step's loads
			}
			tm.Send(tuitest.Key("ctrl+c"))
			final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(tui.Model)
			if got := final.Top().Status(nil).Left; got != tt.want || final.Depth() != tt.depth {
				t.Errorf("%q with %d screens; want %q with %d", got, final.Depth(), tt.want, tt.depth)
			}
		})
	}
}

// Every day since 1960 opens in both divisions from the embedded snapshot
// alone (Phase 7 AC); each tournament's last day renders in the day view.
func TestBrowseAllOffline(t *testing.T) {
	svc := embedded()
	newest, err := svc.Newest()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for id := domain.FirstBasho; id.Compare(newest) <= 0; id = id.Next() {
		if id.Held() == domain.NotHeld {
			continue
		}
		for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
			for d := 1; d <= domain.BashoDays; d++ {
				if _, err := svc.Day(t.Context(), id, div, d); err != nil {
					t.Fatalf("%s %s day %d: %v", id, div, d, err)
				}
				n++
			}
		}
		ctx := &tui.Context{Svc: svc, Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers}
		v := tuitest.New(tui.New(ctx, []tui.Screen{day.New(ctx, id, 15)}, nil), 100, 30).View()
		if !strings.Contains(v, "Day 15 ·") {
			t.Fatalf("%s day 15:\n%s", id, v)
		}
	}
	if n < 11000 {
		t.Errorf("only %d days", n)
	}
}

// Resizing never breaks the layout (Phase 7 AC): every screen fills exactly
// the terminal at any size.
func TestResize(t *testing.T) {
	d := tuitest.New(app(embedded()), 100, 30)
	screens := map[string][]string{"day": nil, "juryo": {"d"}, "picker": {"d", "esc"}, "help": {"d", "esc", "?"}}
	for _, name := range []string{"day", "juryo", "picker", "help"} {
		d.Keys(screens[name]...)
		for _, w := range []int{50, 60, 61, 75, 89, 90, 100, 129, 130, 160, 200} {
			for _, h := range []int{15, 20, 24, 30, 50} {
				d.Send(tea.WindowSizeMsg{Width: w, Height: h})
				lines := strings.Split(d.View(), "\n")
				if len(lines) != h {
					t.Errorf("%s at %d×%d: %d lines", name, w, h, len(lines))
				}
				for i, l := range lines {
					if lw := ansi.StringWidth(l); lw > w {
						t.Errorf("%s at %d×%d: line %d is %d wide", name, w, h, i, lw)
						break
					}
				}
			}
		}
		d.Keys("esc", "esc") // back to a known state: close help, filters
		d.Send(tea.WindowSizeMsg{Width: 100, Height: 30})
		d = tuitest.New(app(embedded()), 100, 30)
	}
}

// The help overlay pages with ←/→ and scrolls a page longer than the
// screen with j/k and the wheel, never past its end.
func TestHelpPages(t *testing.T) {
	d := tuitest.New(app(fixtureService()), 80, 20)
	d.Keys("?", "left") // Limitations, 15 lines in 14 rows
	if v := d.View(); !strings.Contains(v, "j/k scroll (1–14 of 15)") || !strings.Contains(v, "- Heya and hometown") {
		t.Fatalf("limitations:\n%s", v)
	}
	d.Keys("j", "j", "j", "j", "j")
	if v := d.View(); !strings.Contains(v, "(2–15 of 15)") || strings.Contains(v, "Heya") {
		t.Errorf("scrolled past the end:\n%s", v)
	}
	d.Send(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if v := d.View(); !strings.Contains(v, "(1–14 of 15)") {
		t.Errorf("wheel up:\n%s", v)
	}
	d.Keys("right") // around to Keys, from the top
	if v := d.View(); !strings.Contains(v, "previous day") || strings.Contains(v, "scroll") {
		t.Errorf("keys page:\n%s", v)
	}
	d.Keys("esc")
	if v := d.View(); strings.Contains(v, "Limitations") {
		t.Errorf("esc kept the overlay:\n%s", v)
	}
}

// Phase 12 step 4: from nothing loaded to the first screen with bouts in
// under 300 ms: the snapshot opened, the current tournament's latest day
// read and drawn as basho starts (cmd/basho measures about 110 ms in a
// terminal, process start included).
func BenchmarkStart(b *testing.B) {
	for b.Loop() {
		svc := service.New(service.Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
		d := tuitest.New(app(svc), 130, 50)
		if !strings.Contains(d.View(), "Onosato") {
			b.Fatalf("no bouts:\n%s", d.View())
		}
	}
}
