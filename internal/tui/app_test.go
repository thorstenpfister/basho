package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

var testNow = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)

func testContext() *Context {
	cfg := config.Default()
	cfg.UI.Theme = "dark"
	return &Context{Cfg: cfg, Theme: theme.New(true, false), Clock: clock.NewFake(testNow)}
}

// fake is a screen that records what reaches it.
type fake struct {
	name   string
	got    []string
	back   bool
	prov   *service.Provenance
	inited int
}

func (f *fake) Init(*Context) tea.Cmd { f.inited++; return nil }

func (f *fake) Update(_ *Context, msg tea.Msg) (Screen, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		f.got = append(f.got, k.String())
	}
	return f, nil
}

func (f *fake) View(_ *Context, w, h int) string { return f.name + " view" }
func (f *fake) Status(*Context) Status           { return Status{Left: f.name + " status", Prov: f.prov} }

func (f *fake) Keys() []key.Binding {
	return []key.Binding{key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "do x"))}
}
func (f *fake) QuitGoesBack() bool { return f.back }

func press(s string) tea.KeyPressMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func send(t *testing.T, m Model, msgs ...tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, msg := range msgs {
		var next tea.Model
		next, cmd = m.Update(msg)
		m = next.(Model)
	}
	return m, cmd
}

func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestLayouts(t *testing.T) {
	for w, want := range map[int]Layout{60: Compact, 89: Compact, 90: Normal, 129: Normal, 130: Wide, 200: Wide} {
		if got := LayoutOf(w); got != want {
			t.Errorf("width %d: %v, want %v", w, got, want)
		}
	}
	m := New(testContext(), []Screen{&fake{name: "a"}}, nil)
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 59, Height: 30})
	if !strings.Contains(m.render(), "Terminal too small (need 60×20)") {
		t.Errorf("59 columns:\n%s", m.render())
	}
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 19})
	if !strings.Contains(m.render(), "too small") {
		t.Error("19 rows")
	}
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	out := m.render()
	lines := strings.Split(out, "\n")
	if len(lines) != 24 || !strings.Contains(out, "a view") {
		t.Errorf("80×24: %d lines\n%s", len(lines), out)
	}
	for i, l := range lines {
		if w := len([]rune(stripANSI(l))); w > 80 {
			t.Errorf("line %d is %d wide", i, w)
		}
	}
}

func TestStatusBar(t *testing.T) {
	snap := &service.Provenance{Source: service.SnapshotSource, FetchedAt: testNow}
	m := New(testContext(), []Screen{&fake{name: "a", prov: snap}}, nil)
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	bar := stripANSI(m.statusBar())
	if !strings.Contains(bar, "a status") || !strings.Contains(bar, "snapshot (2026-10-14)") || !strings.Contains(bar, "? help") {
		t.Errorf("status bar %q", bar)
	}
	if w := len([]rune(bar)); w != 100 {
		t.Errorf("status bar is %d wide", w)
	}
	m, cmd := send(t, m, MessageMsg{"No bouts published for day 14 yet"})
	if !strings.Contains(stripANSI(m.statusBar()), "No bouts published") || cmd == nil {
		t.Error("message not shown")
	}
	m, _ = send(t, m, clearMsg{id: m.messageID - 1}) // an older message's timer
	if !strings.Contains(stripANSI(m.statusBar()), "No bouts") {
		t.Error("an older timer cleared the message")
	}
	m, _ = send(t, m, clearMsg{id: m.messageID})
	if strings.Contains(stripANSI(m.statusBar()), "No bouts") {
		t.Error("message not cleared")
	}
	cached := service.Provenance{Source: "cache:sumo-api", FetchedAt: testNow.Add(-90 * time.Minute), Stale: true}
	if got := provenance(cached, testNow); got != "cache:sumo-api · updated 10:30 JST (1h ago)" {
		t.Errorf("provenance %q", got)
	}
}

func TestKeysAndStack(t *testing.T) {
	a, b := &fake{name: "a"}, &fake{name: "b"}
	m := New(testContext(), []Screen{a}, func() []Screen { return []Screen{&fake{name: "today"}} })
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30}, PushMsg{b}, press("x"))
	if len(m.stack) != 2 || b.inited != 1 || len(b.got) != 1 || len(a.got) != 0 {
		t.Fatalf("push and forward: %d screens, %v / %v", len(m.stack), a.got, b.got)
	}
	m, _ = send(t, m, press("?"))
	if !strings.Contains(m.render(), "do x") || !strings.Contains(m.render(), "Legend") {
		t.Errorf("help:\n%s", m.render())
	}
	m, _ = send(t, m, press("x"), press("?"))
	if m.help || len(b.got) != 1 {
		t.Error("keys reached the screen under the help overlay")
	}
	m, _ = send(t, m, press("esc"))
	if len(m.stack) != 1 {
		t.Error("esc did not go back")
	}
	if m, _ = send(t, m, press("esc")); len(m.stack) != 1 || len(a.got) != 1 {
		t.Error("esc on the last screen goes to that screen")
	}
	m, _ = send(t, m, PushMsg{&fake{name: "vs", back: true}})
	m, cmd := send(t, m, press("q"))
	if quits(cmd) || len(m.stack) != 1 {
		t.Error("q on a QuitBacker did not go back")
	}
	if _, cmd := send(t, m, press("q")); !quits(cmd) {
		t.Error("q did not quit")
	}
	if _, cmd := send(t, m, PushMsg{&fake{back: true}}, press("ctrl+c")); !quits(cmd) {
		t.Error("ctrl+c did not quit")
	}
	m, _ = send(t, m, press("t"))
	if len(m.stack) != 1 || m.top().(*fake).name != "today" {
		t.Error("t did not reset the stack")
	}
	m, _ = send(t, m, press("L"))
	if !strings.Contains(stripANSI(m.statusBar()), "Phase 10") {
		t.Error("L placeholder")
	}
}

func TestBackground(t *testing.T) {
	ctx := testContext()
	ctx.Cfg.UI.Theme = "auto"
	m := New(ctx, []Screen{&fake{}}, nil)
	if m.Init() == nil {
		t.Error("auto theme without a background request")
	}
	_, _ = send(t, m, tea.BackgroundColorMsg{Color: lipglossWhite})
	if ctx.Theme.Dark {
		t.Error("a white background kept the dark theme")
	}
}

// capturing is a fake with a focused input.
type capturing struct{ fake }

func (c *capturing) CapturesKeys() bool { return true }

func (c *capturing) Update(ctx *Context, msg tea.Msg) (Screen, tea.Cmd) {
	_, cmd := c.fake.Update(ctx, msg)
	return c, cmd
}

func TestKeyCapture(t *testing.T) {
	c := &capturing{fake{name: "filter"}}
	m := New(testContext(), []Screen{c}, func() []Screen { return []Screen{&fake{name: "today"}} })
	m, cmd := send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30}, press("t"), press("q"), press("?"))
	if quits(cmd) || m.help || len(c.got) != 3 || m.top() != Screen(c) {
		t.Errorf("captured keys: %v, help %v", c.got, m.help)
	}
	if _, cmd := send(t, m, press("ctrl+c")); !quits(cmd) {
		t.Error("ctrl+c must stay global")
	}
}
