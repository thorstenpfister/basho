package day

import (
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/standings"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

var (
	testNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)
	aki2026 = domain.BashoID{Year: 2026, Month: time.September}
)

// svc is the embedded snapshot without remote layers, built once.
var svc = sync.OnceValue(func() *service.Service {
	return service.New(service.Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
})

func open(t *testing.T, id domain.BashoID, day, width, height int) (*tui.Context, *tuitest.Driver) {
	t.Helper()
	ctx := &tui.Context{Svc: svc(), Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers}
	return ctx, tuitest.New(tui.New(ctx, []tui.Screen{New(ctx, id, day)}, nil), width, height)
}

func screen(d *tuitest.Driver) Day {
	return d.M.(tui.Model).Top().(Day)
}

func TestDefaultDayAndJumps(t *testing.T) {
	_, d := open(t, aki2026, 0, 100, 30)
	if s := screen(d); s.day != 15 || s.data == nil || s.data.Day != 15 {
		t.Fatalf("a finished tournament opens on day 15, got %d", s.day)
	}
	for _, tt := range []struct {
		keys []string
		want int
	}{
		{[]string{"left"}, 14},
		{[]string{"h", "h"}, 12},
		{[]string{"right"}, 13},
		{[]string{"5"}, 5},
		{[]string{"0"}, 10},
		{[]string{"!"}, 11},
		{[]string{"%"}, 15},
		{[]string{"g", "0", "3"}, 3},
		{[]string{"g", "1", "4"}, 14},
		{[]string{"g", "x", "7"}, 7}, // a broken prefix is dropped
	} {
		d.Keys(tt.keys...)
		if s := screen(d); s.day != tt.want || s.data.Day != tt.want {
			t.Errorf("%v: day %d (data %d), want %d", tt.keys, s.day, s.data.Day, tt.want)
		}
	}
	d.Keys("g", "1", "9")
	if s := screen(d); s.day != 7 || !strings.Contains(d.View(), "Days run from 1 to 15") {
		t.Errorf("g19: day %d\n%s", s.day, d.View())
	}
}

func TestGapDay(t *testing.T) {
	_, d := open(t, domain.BashoID{Year: 1965, Month: time.March}, 6, 100, 30)
	d.Keys("right")
	if s := screen(d); s.day != 8 {
		t.Errorf("right from day 6 over the gap: day %d, want 8", s.day)
	}
	d.Keys("7")
	if s := screen(d); s.day != 8 || !strings.Contains(d.View(), "No bouts published for day 7") {
		t.Errorf("jump to the gap: day %d\n%s", s.day, d.View())
	}
	_, d = open(t, aki2026, 15, 100, 30)
	d.Keys("right")
	if screen(d).day != 15 {
		t.Error("right on day 15")
	}
}

func TestTableAndKeys(t *testing.T) {
	_, d := open(t, aki2026, 13, 100, 24)
	v := d.View()
	for _, want := range []string{
		"Aki Basho 2026 · Ryogoku Kokugikan · Makuuchi", "[d] Juryo", "[13]", "Day 13 · Fri 2026-09-25", "19 bouts",
		"▶   1  M16w Toshinofuji   7-5  ○  oshidashi  ●  7-5  Abi", "Kyujo: Kotoshoho (M1e)", "Aki 2026 · Makuuchi · Day 13",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "結び") {
		t.Error("the musubi fits on 24 rows?")
	}
	d.Keys("G")
	for range 18 {
		d.Keys("j")
	}
	if v := d.View(); !strings.Contains(v, "▶  19  Y1e  Onosato      11-1  ● shitatenage ○  6-6  Kirishima    O1e  結び") {
		t.Errorf("scrolled to the musubi:\n%s", v)
	}
	if sel, _ := screen(d).Selected(); sel.MatchNo != 19 {
		t.Errorf("selected %d", sel.MatchNo)
	}
	d.Keys("p")
	if v := d.View(); !strings.Contains(v, "% E") && !strings.Contains(v, "% W") {
		t.Errorf("no win chance column:\n%s", v)
	}
	d.Keys("enter")
	if !strings.Contains(d.View(), "MUSUBI-NO-ICHIBAN") {
		t.Errorf("enter did not open the VS screen:\n%s", d.View())
	}
	d.Keys("esc")
	d.Keys("d")
	if s := screen(d); s.div != domain.DivJuryo || len(s.data.Bouts) != 14 || s.cursor != 0 {
		t.Errorf("d: %v with %d bouts", s.div, len(s.data.Bouts))
	}
	if !strings.Contains(d.View(), "[d] Makuuchi") {
		t.Error("division header")
	}
}

func TestErrorAndLateAnswers(t *testing.T) {
	b, err := service.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	_, d := open(t, b.Newest.Next(), 1, 100, 30)
	if v := d.View(); !strings.Contains(v, "Not available") {
		t.Errorf("a tournament after the snapshot without remote:\n%s", v)
	}
	ctx, d := open(t, aki2026, 13, 100, 30)
	s := screen(d)
	late := dayMsg{req: s.req - 1, res: service.Result[service.Day]{Value: service.Day{Day: 2}}}
	next, _ := s.Update(ctx, late)
	if next.(Day).data.Day != 13 {
		t.Error("a late answer replaced the day")
	}
}

func TestCompact(t *testing.T) {
	_, d := open(t, aki2026, 13, 60, 20)
	for i, l := range strings.Split(d.View(), "\n") {
		if w := len([]rune(l)); w > 60 {
			t.Errorf("line %d is %d wide: %q", i, w, l)
		}
	}
	if v := d.View(); !strings.Contains(v, "Toshinofu…") || !strings.Contains(v, "Aki 2026 · Makuuchi · Day 13") {
		t.Errorf("compact:\n%s", v)
	}
}

func TestWidePanel(t *testing.T) {
	_, d := open(t, aki2026, 13, 140, 32)
	v := d.View()
	if !strings.Contains(v, "Yusho race · after Day 13") || !strings.Contains(v, "11-2  Y1e  Onosato") || !strings.Contains(v, "── −1 ──") {
		t.Errorf("wide:\n%s", v)
	}
	d.Keys("right")
	if v := d.View(); !strings.Contains(v, "after Day 14") {
		t.Errorf("the panel did not follow the day:\n%s", v)
	}
	d.Keys("%")
	if v := d.View(); !strings.Contains(v, "✓ yusho") {
		t.Errorf("day 15: no decided mark:\n%s", v)
	}

	_, d = open(t, aki2026, 13, 100, 32)
	if strings.Contains(d.View(), "Yusho race") {
		t.Error("a panel at 100 columns")
	}
	d.Send(tea.WindowSizeMsg{Width: 140, Height: 32})
	if !strings.Contains(d.View(), "Yusho race · after Day 13") {
		t.Error("no panel after widening")
	}
	for i, l := range strings.Split(d.View(), "\n") {
		if w := len([]rune(l)); w > 140 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}

// s opens the standings after the shown day and division; esc comes back.
func TestStandingsKey(t *testing.T) {
	_, d := open(t, aki2026, 13, 100, 30)
	d.Keys("d", "s")
	if _, ok := d.M.(tui.Model).Top().(standings.View); !ok {
		t.Fatalf("top is %T", d.M.(tui.Model).Top())
	}
	if v := d.View(); !strings.Contains(v, "Standings after Day 13 · Aki 2026 · Juryo") {
		t.Errorf("standings:\n%s", v)
	}
	d.Keys("esc")
	if s := screen(d); s.day != 13 || s.div != domain.DivJuryo {
		t.Errorf("back on day %d %s", s.day, s.div)
	}
}

func TestRecap(t *testing.T) {
	_, d := open(t, aki2026, 13, 100, 30)
	d.Keys("r")
	v := d.View()
	for _, want := range []string{
		"Day 13 recap · Makuuchi",
		"Leaders     Onosato (Y1e) 11-2",
		"Make-koshi  Kotoshoho (M1e), Roga (M5e)",
		"Upsets      Asanoyama (M6e) over Atamifuji (S1e) · yorikiri",
		"katasukashi 肩透かし (under-shoulder swing down) by Ura",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("no %q:\n%s", want, v)
		}
	}
	d.Keys("right") // the recap follows the day
	if v := d.View(); !strings.Contains(v, "Day 14 recap") || !strings.Contains(v, "Kotozakura (O1w) over") {
		t.Errorf("day 14:\n%s", v)
	}
	for _, k := range []string{"esc", "q", "r"} {
		d.Keys(k)
		if s := screen(d); s.recapOpen || d.M.(tui.Model).Depth() != 1 {
			t.Errorf("%s: open %v, depth %d", k, s.recapOpen, d.M.(tui.Model).Depth())
		}
		d.Keys("r")
	}
}

// Early days have many leaders; the recap names a few and fits a small
// terminal.
func TestRecapCompact(t *testing.T) {
	_, d := open(t, aki2026, 1, 70, 24)
	d.Keys("r")
	v := d.View()
	if !strings.Contains(v, "and 13 more") || !strings.Contains(v, "r/esc close") {
		t.Errorf("compact recap:\n%s", v)
	}
	for i, l := range strings.Split(v, "\n") {
		if w := ansi.StringWidth(l); w > 70 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}

func TestRecapKinboshi(t *testing.T) {
	// 196001 day 1: the fixture's kinboshi (the VS screen's golden).
	_, d := open(t, domain.BashoID{Year: 1960, Month: time.January}, 1, 100, 30)
	d.Keys("r")
	if v := d.View(); !strings.Contains(v, "Kinboshi    "+theme.Unicode.Kinboshi) {
		t.Errorf("no kinboshi:\n%s", v)
	}
}
