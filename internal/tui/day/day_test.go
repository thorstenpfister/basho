package day

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui"
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
	if !strings.Contains(d.View(), "Phase 8") {
		t.Error("enter placeholder")
	}
	d.Keys("d")
	if s := screen(d); s.div != domain.DivJuryo || len(s.data.Bouts) != 14 || s.cursor != 0 {
		t.Errorf("d: %v with %d bouts", s.div, len(s.data.Bouts))
	}
	if !strings.Contains(d.View(), "[d] Makuuchi") {
		t.Error("division header")
	}
}

func TestErrorAndLateAnswers(t *testing.T) {
	_, d := open(t, domain.BashoID{Year: 2026, Month: time.November}, 1, 100, 30)
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
