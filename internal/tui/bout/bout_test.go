package bout

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
	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
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
	rows := append(res.Value.Bouts, res.Value.Playoffs...)
	for i, r := range rows {
		if r.MatchNo == match {
			return tuitest.New(tui.New(ctx, []tui.Screen{New(id, div, day, res.Value.Date, rows, i)}, nil), width, height)
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
	if strings.Contains(lineWith(compact, "HEIGHT"), "█") || !strings.Contains(compact, "190cm") {
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
	d := tuitest.New(tui.New(ctx, []tui.Screen{tuiDayStub{}, New(aki2026, domain.DivMakuuchi, 13, res.Value.Date, res.Value.Bouts, 0)}, nil), 100, 40)
	d.Keys("q")
	if d.M.(tui.Model).Depth() != 1 {
		t.Error("q did not go back")
	}
}

func TestProgressive(t *testing.T) {
	ctx := testContext(svc())
	res, _ := svc().Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	v := New(aki2026, domain.DivMakuuchi, 13, res.Value.Date, res.Value.Bouts, 18) // before Init: nothing loaded
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

func TestResultPanel(t *testing.T) {
	for _, tt := range []struct {
		name  string
		id    domain.BashoID
		day   int
		match int
		want  []string
	}{
		{"regular", aki2026, 13, 19, []string{"KIRISHIMA wins by SHITATENAGE  下手投げ", "underarm throw", "Onosato 11-2   ·   Kirishima 7-6"}},
		{"fusen", aki2026, 13, 15, []string{"HAKUNOFUJI wins by FUSEN", "FUSEN (default)"}},
		{"kinboshi", domain.BashoID{Year: 1960, Month: time.January}, 1, 21, []string{"★ KINBOSHI", "SUKUINAGE"}},
	} {
		v := open(t, tt.id, domain.DivMakuuchi, tt.day, tt.match, 100, 40).View()
		for _, w := range tt.want {
			if !strings.Contains(v, w) {
				t.Errorf("%s: lacks %q:\n%s", tt.name, w, v)
			}
		}
		lines := strings.Split(v, "\n")
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimLeft(l, " "), "│") && !strings.HasPrefix(l, " │") {
				t.Errorf("%s: box line %d not aligned: %q", tt.name, i, l)
			}
		}
	}
}

func TestUndecided(t *testing.T) {
	ctx := testContext(svc())
	row := service.BoutRow{MatchNo: 3, East: service.BoutSide{Wrestler: service.Wrestler{Shikona: domain.Name{En: "A"}}}, West: service.BoutSide{Wrestler: service.Wrestler{Shikona: domain.Name{En: "B"}}}}
	later := New(aki2026, domain.DivMakuuchi, 13, domain.NewDate(2026, time.September, 25), []service.BoutRow{row}, 0)
	later.tale = &service.Tale{}
	if v := ansi.Strip(later.resultPanel(ctx, measure(ctx, 100, 40))); !strings.Contains(v, "Scheduled — Day 13") {
		t.Errorf("scheduled:\n%s", v)
	}
	today := New(aki2026, domain.DivMakuuchi, 13, domain.DateOf(testNow), []service.BoutRow{row}, 0)
	today.tale = &service.Tale{}
	if v := ansi.Strip(today.resultPanel(ctx, measure(ctx, 100, 40))); !strings.Contains(v, "Waiting for result…") {
		t.Errorf("waiting:\n%s", v)
	}
}

func TestTechniqueRows(t *testing.T) {
	d := open(t, aki2026, domain.DivMakuuchi, 13, 19, 100, 40)
	v := d.View()
	for _, want := range []string{"All-rounder", "Belt fighter", "STYLE", "yorikiri 34%  SIGNATURE   yorikiri 22%"} {
		if !strings.Contains(v, want) {
			t.Errorf("lacks %q:\n%s", want, v)
		}
	}
	d.Keys("m")
	v = d.View()
	if !strings.Contains(v, "MOVESET · last 6 basho") || !strings.Contains(v, "Matchup:") || !strings.Contains(v, "Wins by") {
		t.Errorf("moveset panel:\n%s", v)
	}
	d.Keys("w")
	if v := d.View(); !strings.Contains(v, "MOVESET · career") || !strings.Contains(v, "187 W · 65 L") {
		t.Errorf("career window:\n%s", v)
	}
	d.Keys("w", "w", "m")
	if v := d.View(); strings.Contains(v, "MOVESET") || screen(d).window != derive.LastSix {
		t.Error("closing the panel")
	}
	d.Keys("w")
	if screen(d).window != derive.LastSix {
		t.Error("w changed the window with the panel closed")
	}

	// Day 1 of 1960: nobody has 10 wins yet.
	early := open(t, domain.FirstBasho, domain.DivMakuuchi, 1, 21, 100, 40).View()
	if strings.Count(early, "— not enough data —") != 4 {
		t.Errorf("not enough data:\n%s", early)
	}
}

func lineWith(s, sub string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			return l
		}
	}
	return ""
}

func TestRatingRows(t *testing.T) {
	v := open(t, aki2026, domain.DivMakuuchi, 13, 19, 100, 40).View()
	r, wc := lineWith(v, "RATING"), lineWith(v, "WIN CHANCE")
	if !strings.Contains(r, "1681 ▲23") || !strings.Contains(r, "1633 ▼26") || !strings.Contains(wc, "57%") || !strings.Contains(wc, "43%") {
		t.Errorf("rating rows:\n%s\n%s", r, wc)
	}
	// Hidden by the config.
	ctx := testContext(svc())
	ctx.Cfg.UI.WinChance = false
	res, _ := svc().Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	d := tuitest.New(tui.New(ctx, []tui.Screen{New(aki2026, domain.DivMakuuchi, 13, res.Value.Date, res.Value.Bouts, 18)}, nil), 100, 40)
	if v := d.View(); strings.Contains(v, "RATING") || strings.Contains(v, "WIN CHANCE") {
		t.Error("ui.win_chance = false shows the rating")
	}
	// Hidden when calibration failed (V9).
	hidden := service.New(service.Options{Snapshot: data.Snapshot, Params: func() (rating.File, error) {
		f, err := data.RatingParams()
		f.Show = false
		return f, err
	}, Clock: clock.NewFake(testNow)})
	ctx = testContext(hidden)
	d = tuitest.New(tui.New(ctx, []tui.Screen{New(aki2026, domain.DivMakuuchi, 13, res.Value.Date, res.Value.Bouts, 18)}, nil), 100, 40)
	if v := d.View(); strings.Contains(v, "RATING") {
		t.Error("a hidden rating is shown")
	}
}

func TestNavigation(t *testing.T) {
	d := open(t, aki2026, domain.DivMakuuchi, 13, 18, 100, 40)
	d.Keys("]")
	if s := screen(d); s.match != 19 || s.tale == nil || s.tale.MatchNo != 19 {
		t.Fatalf("]: bout %d", s.match)
	}
	d.Keys("]")
	if s := screen(d); s.day != 14 || s.match != 1 || s.tale.Day != 14 {
		t.Errorf("] past the musubi: day %d bout %d", s.day, s.match)
	}
	d.Keys("[")
	if s := screen(d); s.day != 13 || s.match != 19 {
		t.Errorf("[ back: day %d bout %d", s.day, s.match)
	}
	d = open(t, aki2026, domain.DivMakuuchi, 1, 1, 100, 40)
	d.Keys("[")
	if !strings.Contains(d.View(), "first bout of the tournament") {
		t.Error("[ at the first bout")
	}
	// 196503 has no bouts on day 7: ] from day 6's last bout goes to day 8.
	gap := domain.BashoID{Year: 1965, Month: time.March}
	res, _ := svc().Day(t.Context(), gap, domain.DivMakuuchi, 6)
	last := res.Value.Bouts[len(res.Value.Bouts)-1].MatchNo
	d = open(t, gap, domain.DivMakuuchi, 6, last, 100, 40)
	d.Keys("]")
	if s := screen(d); s.day != 8 || s.match != 1 {
		t.Errorf("over the gap: day %d bout %d", s.day, s.match)
	}
}

func TestAnimation(t *testing.T) {
	ctx := testContext(svc())
	ctx.TTY = true
	res, _ := svc().Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	start := func(ctx *tui.Context, width int) VS {
		s, _ := New(aki2026, domain.DivMakuuchi, 13, res.Value.Date, res.Value.Bouts, 18).Update(ctx, tea.WindowSizeMsg{Width: width, Height: 40})
		return s.(VS)
	}
	v := start(ctx, 100)
	if !v.sliding || v.slide != 0 {
		t.Fatalf("no slide-in: %v %v", v.sliding, v.slide)
	}
	first := lineWith(ansi.Strip(v.View(ctx, 100, 39)), "VS")
	if strings.Index(first, "ONOSATO") > 5 {
		t.Errorf("the east name does not start at the edge: %q", first)
	}
	frames := 0
	for ; v.sliding && frames < 120; frames++ {
		s, _ := v.Update(ctx, frameMsg{})
		v = s.(VS)
	}
	if v.sliding || v.slide != 1 || frames > 20 { // ~250 ms at 60 fps
		t.Errorf("slide after %d frames: %v %v", frames, v.sliding, v.slide)
	}
	if got := lineWith(ansi.Strip(v.View(ctx, 100, 39)), "VS"); !strings.Contains(got, "ONOSATO      VS      KIRISHIMA") {
		t.Errorf("names not in place: %q", got)
	}

	// Off: compact layout, ui.animations, no terminal.
	if start(ctx, 70).sliding {
		t.Error("slides in the compact layout")
	}
	off := testContext(svc())
	off.TTY = true
	off.Cfg.UI.Animations = false
	if start(off, 100).sliding {
		t.Error("slides with ui.animations off")
	}
	if start(testContext(svc()), 100).sliding {
		t.Error("slides without a terminal")
	}

	s, _ := v.Update(ctx, RevealMsg{})
	if !s.(VS).flash {
		t.Error("no reveal flash")
	}
	s, _ = s.Update(ctx, flashDoneMsg{})
	if s.(VS).flash {
		t.Error("flash did not end")
	}
	if s, _ := start(off, 100).Update(off, RevealMsg{}); s.(VS).flash {
		t.Error("flash with animations off")
	}
}

func displayWidth(s string) int { return ansi.StringWidth(s) }
