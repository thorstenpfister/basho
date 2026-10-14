package bout

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

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

var svc = sync.OnceValue(func() *service.Service {
	return service.New(service.Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
})

func testContext(s *service.Service) *tui.Context {
	return &tui.Context{Svc: s, Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers}
}

// open shows bout match of a day of the embedded snapshot.
func open(t *testing.T, id domain.BashoID, div domain.Division, day, match, width, height int) *tuitest.Driver {
	t.Helper()
	ctx := testContext(svc())
	res, err := svc().Day(t.Context(), id, div, day)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range append(res.Value.Bouts, res.Value.Playoffs...) {
		if r.MatchNo == match {
			return tuitest.New(tui.New(ctx, []tui.Screen{New(id, div, day, r)}, nil), width, height)
		}
	}
	t.Fatalf("no bout %d", match)
	return nil
}

func screen(d *tuitest.Driver) VS { return d.M.(tui.Model).Top().(VS) }

func TestTape(t *testing.T) {
	d := open(t, aki2026, domain.DivMakuuchi, 13, 19, 100, 40)
	v := d.View()
	for _, want := range []string{
		"Aki 2026 · Day 13 · Bout 19/19 · MUSUBI-NO-ICHIBAN",
		"ONOSATO      VS      KIRISHIMA", "大の里", "Yokozuna 1 East     RANK     Ozeki 1 East",
		"Nishonoseki     HEYA     Otowayama", "11-1    RECORD    6-6", "LAST 5", "190cm    HEIGHT    186cm",
		"KINBOSHI", "5    YUSHO     3", "H2H  10", "oshidashi ×5 · yorikiri ×2",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("lacks %q:\n%s", want, v)
		}
	}
	if !strings.Contains(v, "████ 190cm") {
		t.Error("no height bar")
	}
	if strings.Contains(v, "Last meetings") {
		t.Error("history shown before v")
	}
	d.Keys("v")
	if v := d.View(); !strings.Contains(v, "Last meetings") || strings.Count(v, " Onosato ") < 5 {
		t.Errorf("v:\n%s", v)
	}
	if !screen(d).QuitGoesBack() {
		t.Error("q must go back")
	}
}

func TestLayouts(t *testing.T) {
	wide := open(t, aki2026, domain.DivMakuuchi, 13, 19, 140, 40).View()
	if !strings.Contains(wide, "Head-to-head") || !strings.Contains(wide, "202607 12 Y1w  O1e  Kirishima tottari") {
		t.Errorf("wide:\n%s", wide)
	}
	compact := open(t, aki2026, domain.DivMakuuchi, 13, 19, 70, 30).View()
	if strings.Contains(compact, "█") || !strings.Contains(compact, "190cm") {
		t.Errorf("compact has bars or lacks values:\n%s", compact)
	}
	for _, v := range []string{wide, compact} {
		lines := strings.Split(v, "\n")
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > len([]rune(lines[len(lines)-1]))+1 {
				t.Errorf("line %d is %d wide", i, w)
			}
		}
	}
}

func TestFromTheDayView(t *testing.T) {
	ctx := testContext(svc())
	res, _ := svc().Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	d := tuitest.New(tui.New(ctx, []tui.Screen{tuiDayStub{}, New(aki2026, domain.DivMakuuchi, 13, res.Value.Bouts[0])}, nil), 100, 40)
	d.Keys("q")
	if d.M.(tui.Model).Depth() != 1 {
		t.Error("q did not go back")
	}
}

func TestProgressive(t *testing.T) {
	ctx := testContext(svc())
	res, _ := svc().Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	v := New(aki2026, domain.DivMakuuchi, 13, res.Value.Bouts[18]) // before Init: nothing loaded
	out := ansi.Strip(v.View(ctx, 100, 39))
	for _, want := range []string{"ONOSATO      VS      KIRISHIMA", "大の里", "Yokozuna 1 East     RANK     Ozeki 1 East", "11-1    RECORD    6-6", "…    HEIGHT    …", "H2H  …"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if st := v.Status(ctx); !strings.Contains(st.Left, "Bout 19") {
		t.Errorf("status %+v", st)
	}
}
