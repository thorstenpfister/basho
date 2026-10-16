package tui_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/spoiler"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/bout"
	"github.com/thorstenpfister/basho/internal/tui/day"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// spoilerApp is the day view of Aki 2026 day 13 in spoiler-free mode, with
// the horizon after Makuuchi bout 4 and the state saved under a temporary
// directory.
func spoilerApp(t *testing.T) (*tuitest.Driver, *spoiler.State, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	st, err := spoiler.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Reveal(domain.Bout{Basho: aki2026, Day: 13, Division: domain.DivMakuuchi, MatchNo: 4}); err != nil {
		t.Fatal(err)
	}
	ctx := &tui.Context{Svc: embedded(), Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers, Spoilers: st, Hide: true}
	return tuitest.New(tui.New(ctx, []tui.Screen{day.New(ctx, aki2026, 13)}, nil), 120, 40), st, path
}

// row returns the day view's line of a bout.
func row(v string, match int) string {
	for _, l := range strings.Split(v, "\n") {
		f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(l), "▶"))
		if len(f) > 1 && f[0] == itoaN(match) {
			return l
		}
	}
	return ""
}

func itoaN(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// S names how far the tournament on screen is seen, so a tournament seen to
// the end doesn't look as if S did nothing (QA run 1).
func TestSpoilersMessage(t *testing.T) {
	d, st, _ := spoilerApp(t)
	d.Keys("S", "S") // off and on again
	if v := d.View(); !strings.Contains(v, "Spoiler-free: Aki 2026 seen through Day 13 · Makuuchi bout 4") {
		t.Errorf("S with a horizon:\n%s", v)
	}
	if err := st.Set(aki2026, spoiler.Horizon{}); err != nil {
		t.Fatal(err)
	}
	d.Keys("S", "S")
	if v := d.View(); !strings.Contains(v, "Spoiler-free: Aki 2026 nothing seen yet") {
		t.Errorf("S before Day 1:\n%s", v)
	}
}

// SPEC §9: space on the VS screen reveals that bout, ] walks on.
func TestRevealOnVS(t *testing.T) {
	d, st, _ := spoilerApp(t)
	if r := row(d.View(), 5); !strings.Contains(r, "?") {
		t.Fatalf("bout 5 before: %q", r)
	}
	d.Keys("j", "j", "j", "j", "enter") // bout 5
	if _, ok := d.M.(tui.Model).Top().(bout.VS); !ok {
		t.Fatal("not on the VS screen")
	}
	if v := d.View(); !strings.Contains(v, "Result hidden (spoiler-free)") || strings.Contains(v, " wins by ") {
		t.Fatalf("VS before:\n%s", v)
	}
	d.Keys("space")
	if v := d.View(); strings.Contains(v, "Result hidden") || !strings.Contains(v, " wins by ") {
		t.Errorf("VS after space:\n%s", v)
	}
	if h := st.Horizon(aki2026); h.MatchNo != 5 || h.Day != 13 {
		t.Errorf("horizon %+v", h)
	}
	d.Keys("]")
	if v := d.View(); !strings.Contains(v, "Result hidden") {
		t.Errorf("bout 6 shown:\n%s", v)
	}
	d.Keys("space", "esc")
	// Back on the day view, which reloaded: bouts 5 and 6 are shown, 7 not.
	v := d.View()
	if strings.Contains(row(v, 5), "?") || strings.Contains(row(v, 6), "?") || !strings.Contains(row(v, 7), "?") {
		t.Errorf("day view after the reveals:\n%s", v)
	}
}

func TestRevealUpToHere(t *testing.T) {
	d, st, path := spoilerApp(t)
	d.Keys("j", "j", "j", "j", "j", "j", "j", "j", "j", "ctrl+r") // bout 10
	if v := d.View(); !strings.Contains(v, "Reveal every result up to Day 13 bout 10? [y/N]") {
		t.Fatalf("no prompt:\n%s", v)
	}
	d.Keys("n")
	if st.Horizon(aki2026).MatchNo != 4 || !strings.Contains(row(d.View(), 10), "?") {
		t.Error("n revealed")
	}
	d.Keys("ctrl+r", "y")
	v := d.View()
	if st.Horizon(aki2026).MatchNo != 10 || strings.Contains(row(v, 10), "?") || !strings.Contains(row(v, 11), "?") {
		t.Errorf("after y:\n%s", v)
	}
	// The horizon persists between runs.
	again, err := spoiler.Load(path)
	if err != nil || again.Horizon(aki2026).MatchNo != 10 {
		t.Errorf("reloaded: %+v, %v", again.Horizon(aki2026), err)
	}
	// q and esc answer the prompt instead of leaving.
	d.Keys("j", "ctrl+r", "esc")
	if d.M.(tui.Model).Depth() != 1 || st.Horizon(aki2026).MatchNo != 10 {
		t.Error("esc at the prompt")
	}
}

func TestSpoilerToggle(t *testing.T) {
	d, _, _ := spoilerApp(t)
	if !strings.Contains(statusLine(d.View()), "spoiler-free") {
		t.Errorf("status %q", statusLine(d.View()))
	}
	d.Keys("S")
	v := d.View()
	if strings.Contains(row(v, 19), "?") || strings.Contains(statusLine(v), "🙈") {
		t.Errorf("S off:\n%s", v)
	}
	d.Keys("S")
	if !strings.Contains(row(d.View(), 19), "?") {
		t.Error("S on again")
	}
	// ctrl+r with spoiler-free mode off says so.
	d.Keys("S", "ctrl+r")
	if s := statusLine(d.View()); !strings.Contains(s, "Spoiler-free mode is off") {
		t.Errorf("status %q", s)
	}
}
