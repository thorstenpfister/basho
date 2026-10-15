package tui_test

import (
	"fmt"
	"testing"
	"unicode"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/day"
	"github.com/thorstenpfister/basho/internal/tui/picker"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// Phase 12 step 1: with --ascii no screen draws a symbol outside ASCII.
// Letters are data and may stay: names and techniques in kanji, places
// like Töv.
func TestASCIIScreens(t *testing.T) {
	if testing.Short() || raceDetector {
		t.Skip("walks every screen; make check runs it")
	}
	ctx := &tui.Context{Svc: embedded(), Cfg: config.Default(), Theme: theme.New(true, true), Clock: clock.NewFake(testNow), Tick: tui.NoTimers}
	ctx.Cfg.UI.ASCII = true
	shown := screens(t, ctx, []int{1, 13, 15})
	open := func(id domain.BashoID) tui.Screen { return day.New(ctx, id, 0) }
	d := tuitest.New(tui.New(ctx, []tui.Screen{picker.New(ctx, open), day.New(ctx, aki2026, 13)}, nil), 140, 44)
	d.Keys("?")
	for i := range 5 {
		shown[fmt.Sprintf("help page %d", i+1)] = d.View()
		d.Keys("tab")
	}
	for name, view := range shown {
		for _, r := range view {
			if r > unicode.MaxASCII && !unicode.IsLetter(r) {
				t.Errorf("%s draws %q:\n%s", name, r, view)
				break
			}
		}
	}
}
