package picker

import (
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

var testNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)

func testContext() *tui.Context {
	c := clock.NewFake(testNow)
	svc := service.New(service.Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../../testdata/snapshot")) },
		Params:   func() (rating.File, error) { return rating.File{}, nil },
		Clock:    c,
	})
	return &tui.Context{Svc: svc, Cfg: config.Default(), Theme: theme.New(true, false), Clock: c}
}

// opened is the screen open returns, carrying the tournament.
type opened struct {
	tui.Screen
	id domain.BashoID
}

func newPicker(t *testing.T) (*tui.Context, Picker) {
	t.Helper()
	ctx := testContext()
	p := New(ctx, func(id domain.BashoID) tui.Screen { return opened{id: id} })
	s, _ := p.Update(ctx, tea.WindowSizeMsg{Width: 80, Height: 24})
	s, _ = s.Update(ctx, p.Init(ctx)())
	return ctx, s.(Picker)
}

func keys(t *testing.T, ctx *tui.Context, p Picker, ks ...string) (Picker, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range ks {
		msg := tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		}
		var s tui.Screen
		s, cmd = p.Update(ctx, msg)
		p = s.(Picker)
	}
	return p, cmd
}

func TestRows(t *testing.T) {
	ctx, p := newPicker(t)
	first, last := p.rows[0], p.rows[len(p.rows)-1]
	if first.id != (domain.BashoID{Year: 2026, Month: time.November}) || first.status != service.Upcoming || last.id != domain.FirstBasho {
		t.Errorf("rows %v … %v", first, last)
	}
	byID := map[domain.BashoID]row{}
	for _, r := range p.rows {
		byID[r.id] = r
	}
	if r := byID[domain.BashoID{Year: 2020, Month: time.May}]; r.status != service.Cancelled {
		t.Errorf("202005 %v", r.status)
	}
	aki := byID[domain.BashoID{Year: 2026, Month: time.September}]
	if aki.yusho == nil || aki.yusho.Shikona.En != "Onosato" || aki.yusho.Record.String() != "12-3" {
		t.Errorf("Aki 2026 yusho %+v", aki.yusho)
	}
	view := ansi.Strip(p.View(ctx, 80, 23))
	for _, want := range []string{"BASHO", "2026  Kyushu  11  upcoming · banzuke 2026-10-26", "2026  Aki     09  Yusho: Onosato (12-3)", "── 2025 ──", "enter open"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if n := strings.Count(view, "\n") + 1; n != 23 {
		t.Errorf("view is %d lines, want 23", n)
	}
}

func TestFilterAndOpen(t *testing.T) {
	ctx, p := newPicker(t)
	p, _ = keys(t, ctx, p, "/")
	if !p.CapturesKeys() {
		t.Fatal("the filter does not capture keys")
	}
	p, _ = keys(t, ctx, p, "a", "k", "i", " ", "2", "0", "1", "9", "enter")
	if p.CapturesKeys() || len(p.shown) != 1 {
		t.Fatalf("aki 2019: %d rows", len(p.shown))
	}
	_, cmd := keys(t, ctx, p, "enter")
	push, ok := cmd().(tui.PushMsg)
	if !ok || push.Screen.(opened).id != (domain.BashoID{Year: 2019, Month: time.September}) {
		t.Errorf("enter: %v", cmd())
	}
	p, _ = keys(t, ctx, p, "esc")
	if len(p.shown) != len(p.rows) {
		t.Error("esc did not clear the filter")
	}
	p, _ = keys(t, ctx, p, "/", "n", "a", "t", "s", "u", " ", "2", "0", "2", "0", "enter")
	if _, cmd := keys(t, ctx, p, "enter"); cmd == nil {
		t.Error("no message for a cancelled tournament")
	} else if _, ok := cmd().(tui.MessageMsg); !ok {
		t.Errorf("cancelled: %v", cmd())
	}
	p, _ = keys(t, ctx, p, "esc", "/", "x", "x", "enter")
	if len(p.shown) != 0 || !strings.Contains(p.View(ctx, 80, 23), "no tournament matches") {
		t.Error("no match")
	}
}

func TestScrolling(t *testing.T) {
	ctx, p := newPicker(t)
	p, _ = keys(t, ctx, p, "G")
	if id, _ := p.Selected(); id != domain.FirstBasho {
		t.Fatalf("G: %v", id)
	}
	view := ansi.Strip(p.View(ctx, 80, 23))
	if !strings.Contains(view, "▶ 1960  Hatsu") || strings.Contains(view, "2026") {
		t.Errorf("bottom:\n%s", view)
	}
	p, _ = keys(t, ctx, p, "k")
	if v := ansi.Strip(p.View(ctx, 80, 23)); !strings.Contains(v, "▶ 1960  Haru") || !strings.Contains(v, "1960  Hatsu") {
		t.Errorf("moving up from the bottom scrolled away:\n%s", v)
	}
	p, _ = keys(t, ctx, p, "g")
	if id, _ := p.Selected(); id.Year != 2026 {
		t.Errorf("g: %v", id)
	}
	if len(p.Keys()) == 0 || !key.Matches(tea.KeyPressMsg{Code: '/', Text: "/"}, keyFilter) {
		t.Error("keys")
	}
	if st := p.Status(ctx); st.Loading || st.Prov == nil || !strings.Contains(st.Left, "tournaments") {
		t.Errorf("status %+v", st)
	}
}
