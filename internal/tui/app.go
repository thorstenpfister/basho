package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/internal/asciifold"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/spoiler"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

// Context is what every screen shares (SPEC §8.1).
type Context struct {
	Svc   *service.Service
	Cfg   config.Config
	Theme theme.Theme
	Clock clock.Clock
	// Timers, such as clearing a status message, go through Tick; nil is
	// tea.Tick. Tests turn timers off with NoTimers.
	Tick func(time.Duration, func(time.Time) tea.Msg) tea.Cmd
	// TTY reports that the output is a terminal; animations run only then.
	TTY bool
	// Rand returns a number in [0, 1) for live mode's jitter; nil is
	// math/rand. Tests fix it.
	Rand func() float64
	// Spoilers holds the reveal horizons; Hide is spoiler-free mode for this
	// session (spoilers.mode, S). Only the root changes Hide.
	Spoilers *spoiler.State
	Hide     bool
}

// Data returns the context for a screen's service calls: spoiler-free when
// Hide is on (SPEC §9). Screens take it when they build a command, not inside
// it.
func (c *Context) Data() context.Context {
	ctx := context.Background()
	if c.Hide && c.Spoilers != nil {
		ctx = service.Hiding(ctx, c.Spoilers)
	}
	return ctx
}

// Animate reports whether animations run at this width: ui.animations on,
// a terminal, and not the compact layout (SPEC §8.5).
func (c *Context) Animate(width int) bool {
	return c.Cfg.UI.Animations && c.TTY && LayoutOf(width) != Compact
}

// After returns a command that delivers fn's message after d.
func (c *Context) After(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	if c.Tick != nil {
		return c.Tick(d, fn)
	}
	return tea.Tick(d, fn)
}

// NoTimers is a Tick that never fires, for tests.
func NoTimers(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }

// Layout is the breakpoint of the terminal width (SPEC §8.1).
type Layout int

const (
	Compact Layout = iota // 60–89 columns
	Normal                // 90–129
	Wide                  // 130 and more
)

// Minimum terminal size.
const (
	MinWidth  = 60
	MinHeight = 20
)

// LayoutOf returns the layout of a terminal width.
func LayoutOf(width int) Layout {
	switch {
	case width >= 130:
		return Wide
	case width >= 90:
		return Normal
	default:
		return Compact
	}
}

// Screen is one entry of the screen stack. Screens are values: Update returns
// the next state. All I/O runs in commands.
type Screen interface {
	Init(ctx *Context) tea.Cmd
	Update(ctx *Context, msg tea.Msg) (Screen, tea.Cmd)
	// View renders the screen into width×height cells (the status bar is
	// drawn by the root).
	View(ctx *Context, width, height int) string
	Status(ctx *Context) Status
	Keys() []key.Binding // for the help overlay and the footer
}

// Status is a screen's part of the status bar.
type Status struct {
	Left    string              // e.g. "Aki 2026 · Makuuchi · Day 13"
	Prov    *service.Provenance // the data shown; nil while there is none
	Loading bool                // a request is running: ↻
}

// QuitBacker is a screen on which q goes back one level instead of quitting
// (the VS screen, SPEC §8.8).
type QuitBacker interface{ QuitGoesBack() bool }

// KeyCapturer is a screen that takes every key while it reports true, such
// as a focused filter input; only ctrl+c stays global.
type KeyCapturer interface{ CapturesKeys() bool }

// Messages screens send to the root.
type (
	// PushMsg opens a screen on top.
	PushMsg struct{ Screen Screen }
	// PopMsg closes the top screen.
	PopMsg struct{}
	// ResetMsg replaces the whole stack.
	ResetMsg struct{ Stack []Screen }
	// MessageMsg shows a short-lived status message (3 s).
	MessageMsg struct{ Text string }
	clearMsg   struct{ id int }
)

// SpoilersMsg tells every screen that the spoiler horizon or spoiler-free
// mode changed: they reload from the cache (SPEC §9).
type SpoilersMsg struct{}

// Push, Pop and Message are the commands for those messages.
func Push(s Screen) tea.Cmd       { return func() tea.Msg { return PushMsg{s} } }
func Pop() tea.Cmd                { return func() tea.Msg { return PopMsg{} } }
func Message(text string) tea.Cmd { return func() tea.Msg { return MessageMsg{text} } }

// Spoilers is the command that sends SpoilersMsg to every screen.
func Spoilers() tea.Cmd { return func() tea.Msg { return SpoilersMsg{} } }

// messageTTL is how long a status message stays.
const messageTTL = 3 * time.Second

// Global keys (SPEC §8.8).
var (
	keyQuit     = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))
	keyForce    = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit"))
	keyHelp     = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	keyBack     = key.NewBinding(key.WithKeys("esc", "b"), key.WithHelp("b/esc", "back"))
	keyLive     = key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "live mode"))
	keySpoilers = key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "spoiler-free"))
	keyToday    = key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "today"))
	keyRefresh  = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh (live: poll now)"))
)

var globalKeys = []key.Binding{keyBack, keyToday, keyLive, keySpoilers, keyHelp, keyQuit, keyForce}

// Model is the root: screen stack, global keys, status bar and help.
type Model struct {
	ctx           *Context
	stack         []Screen
	today         func() []Screen // the stack `t` goes to
	width, height int
	help          bool
	message       string
	messageID     int
	live          liveState
}

// New returns the root over a starting stack (bottom first); today builds
// the stack for `t`.
func New(ctx *Context, stack []Screen, today func() []Screen) Model {
	return Model{ctx: ctx, stack: stack, today: today}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.top().Init(m.ctx), m.loadLive()}
	if m.ctx.Cfg.UI.Theme == "auto" {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

func (m Model) top() Screen { return m.stack[len(m.stack)-1] }

// Top returns the screen on top of the stack.
func (m Model) Top() Screen { return m.top() }

// Depth returns the number of screens on the stack.
func (m Model) Depth() int { return len(m.stack) }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if next, cmd, ok := m.liveUpdate(msg); ok {
		return next, cmd
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m.forward(msg)
	case tea.BackgroundColorMsg:
		mono := m.ctx.Theme.Mono
		m.ctx.Theme = theme.New(msg.IsDark(), m.ctx.Theme.ASCII)
		m.ctx.Theme.Mono = mono
		return m, nil
	case tea.ColorProfileMsg:
		m.ctx.Theme.Mono = msg.Profile <= colorprofile.ASCII
		return m, nil
	case PushMsg:
		m.stack = append(m.stack, msg.Screen)
		return m.resized(msg.Screen.Init(m.ctx))
	case PopMsg:
		return m.pop()
	case SpoilersMsg:
		return m.broadcast(msg)
	case screenMsg:
		return m.deliver(msg)
	case ResetMsg:
		m.stack = msg.Stack
		return m.resized(m.top().Init(m.ctx))
	case MessageMsg:
		m.messageID++
		m.message = msg.Text
		id := m.messageID
		return m, m.ctx.After(messageTTL, func(time.Time) tea.Msg { return clearMsg{id} })
	case clearMsg:
		if msg.id == m.messageID {
			m.message = ""
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.MouseWheelMsg:
		return m.wheel(msg)
	}
	return m.forward(msg)
}

// wheel scrolls with the mouse wheel (ui.mouse) as ↑ and ↓ do, so every
// list and overlay moves the way its keys move it.
func (m Model) wheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseWheelUp:
		return m.key(tea.KeyPressMsg{Code: tea.KeyUp})
	case tea.MouseWheelDown:
		return m.key(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if c, ok := m.top().(KeyCapturer); ok && c.CapturesKeys() && !key.Matches(msg, keyForce) {
		return m.forward(msg)
	}
	switch {
	case key.Matches(msg, keyForce):
		return m, tea.Quit
	case m.help:
		if key.Matches(msg, keyHelp, keyBack, keyQuit) {
			m.help = false
		}
		return m, nil
	case key.Matches(msg, keyHelp):
		m.help = true
		return m, nil
	case key.Matches(msg, keyQuit):
		if b, ok := m.top().(QuitBacker); ok && b.QuitGoesBack() && len(m.stack) > 1 {
			return m.pop()
		}
		return m, tea.Quit
	case key.Matches(msg, keyBack) && len(m.stack) > 1:
		return m.pop()
	case key.Matches(msg, keyToday) && m.today != nil:
		return m.Update(ResetMsg{Stack: m.today()})
	case key.Matches(msg, keyLive):
		return m.toggleLive()
	case key.Matches(msg, keyRefresh) && m.livePolls():
		return m.refreshLive()
	case key.Matches(msg, keySpoilers):
		return m.toggleSpoilers()
	}
	return m.forward(msg)
}

// toggleSpoilers is S: spoiler-free mode on or off for this session.
func (m Model) toggleSpoilers() (tea.Model, tea.Cmd) {
	if m.ctx.Spoilers == nil {
		return m.Update(MessageMsg{"Spoiler-free mode is unavailable: no state file"})
	}
	m.ctx.Hide = !m.ctx.Hide
	text := "Spoiler-free mode off: all results shown"
	if m.ctx.Hide {
		text = "Spoiler-free mode on: results up to the horizon; space on a bout reveals it"
	}
	next, cmd := m.broadcast(SpoilersMsg{})
	return next, tea.Batch(cmd, Message(text))
}

// pop closes the top screen, keeping the last one.
func (m Model) pop() (tea.Model, tea.Cmd) {
	if len(m.stack) > 1 {
		m.stack = m.stack[:len(m.stack)-1]
	}
	return m.resized(nil)
}

// resized tells the screen now on top the terminal size, which it may have
// missed while below or before it existed.
func (m Model) resized(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if m.width == 0 {
		return m, cmd
	}
	next, sizeCmd := m.forward(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return next, tea.Batch(cmd, sizeCmd)
}

// forward hands a message to the top screen.
func (m Model) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	s, cmd := m.top().Update(m.ctx, msg)
	m.stack[len(m.stack)-1] = s
	return m, cmd
}

// View implements tea.Model.
func (m Model) View() tea.View {
	frame := m.render()
	if m.ctx.Theme.ASCII {
		frame = asciifold.Fold(frame)
	}
	v := tea.NewView(frame)
	v.AltScreen = true
	if m.ctx.Cfg.UI.Mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// render is the screen as text, for View and the tests.
func (m Model) render() string {
	t := m.ctx.Theme
	if m.width < MinWidth || m.height < MinHeight {
		msg := fmt.Sprintf("Terminal too small (need %d×%d)", MinWidth, MinHeight)
		return lipgloss.Place(max(m.width, 1), max(m.height, 1), lipgloss.Center, lipgloss.Center, t.Dim.Render(msg))
	}
	body := m.height - 1
	var content string
	if m.help {
		content = m.helpView(m.width, body)
	} else {
		content = m.top().View(m.ctx, m.width, body)
	}
	content = lipgloss.NewStyle().Width(m.width).Height(body).MaxHeight(body).MaxWidth(m.width).Render(content)
	return content + "\n" + m.statusBar()
}

// statusBar is the bottom line (SPEC §8.2).
func (m Model) statusBar() string {
	t := m.ctx.Theme
	st := m.top().Status(m.ctx)
	left := st.Left
	if m.message != "" {
		left = t.Message.Render(m.message)
	}
	var right []string
	switch {
	case st.Loading:
		right = append(right, t.Accent.Render(t.Refresh))
	case st.Prov != nil && st.Prov.Stale:
		right = append(right, t.Gold.Render(t.Stale+" stale"))
	}
	if l := m.liveStatus(); l != "" {
		right = append(right, l)
	} else if st.Prov != nil && st.Prov.Source != "" {
		if LayoutOf(m.width) == Compact {
			right = append(right, st.Prov.Source)
		} else {
			right = append(right, provenance(*st.Prov, m.ctx.Clock.Now()))
		}
	}
	if m.ctx.Hide {
		right = append(right, t.Accent.Render(t.Spoiler+" spoiler-free"))
	}
	right = append(right, t.StatusKey.Render("?")+" help")
	r := strings.Join(right, " · ")
	// " left rule right ": four spaces around the rule.
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(r) - 4
	if gap < 1 {
		left = Truncate(left, max(m.width-lipgloss.Width(r)-5, 0))
		gap = max(m.width-lipgloss.Width(left)-lipgloss.Width(r)-4, 1)
	}
	return t.StatusBar.Render(" " + left + " " + t.Rule.Render(strings.Repeat(t.Line, gap)) + " " + r + " ")
}

// provenance is the status bar's source part: `source · updated <time> (<age>)`.
func provenance(p service.Provenance, now time.Time) string {
	if p.Source == service.SnapshotSource {
		return "snapshot (" + p.FetchedAt.Format("2006-01-02") + ")"
	}
	at := p.FetchedAt
	if p.SourceUpdatedAt != nil {
		at = *p.SourceUpdatedAt
	}
	src := p.Source
	if p.Forced {
		src += " (forced)"
	}
	return fmt.Sprintf("%s · updated %s (%s)", src, at.In(domain.JST).Format("15:04 JST"), age(now.Sub(at)))
}

func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// Truncate shortens s to w cells, ending in "…" when it had to cut.
func Truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	if w <= 1 {
		return ansi.Truncate(s, w, "")
	}
	return ansi.Truncate(s, w, "…")
}

// Spread puts left and right at the ends of a width, a cell in from each
// edge.
func Spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	return " " + left + strings.Repeat(" ", max(gap, 1)) + right
}

// ErrorText explains a load's error on an error screen.
func ErrorText(err error) string {
	switch {
	case service.NotFound(err):
		return "Not available: " + err.Error()
	case service.Unavailable(err):
		return "Every source failed and nothing is cached."
	default:
		return err.Error()
	}
}

// helpView is the help overlay (SPEC §8.7): the keys of this screen and the
// global ones, the glyph legend and the data-source notes.
func (m Model) helpView(width, height int) string {
	t := m.ctx.Theme
	h := help.New()
	h.ShowAll = true
	h.SetWidth(width - 4)
	g := t.Glyphs
	legend := strings.Join([]string{
		fmt.Sprintf("%s win  %s loss  %s fusen win  %s fusen loss  %s absent  %s pending  %s kinboshi", g.Win, g.Loss, g.FusenWin, g.FusenLoss, g.Absent, g.Pending, g.Kinboshi),
		fmt.Sprintf("Day bar: %s complete  %s in progress  %s scheduled", g.Complete, g.InProgress, g.Scheduled),
		fmt.Sprintf("LIVE: %s polling  %s waiting  %s day complete · NEW: decided since the last poll", g.Live, g.LiveWait, g.Complete),
		"KK kachi-koshi (8 wins) · MK make-koshi (8 losses and absences) · " + g.Kyujo + " kyujo (absent)",
		"Ranks: Y yokozuna · O ozeki · S sekiwake · K komusubi · M maegashira · J juryo · Ms makushita; e/w east/west",
		"Height and weight are the latest known; * marks them on tournaments other than the current one",
	}, "\n")
	notes := strings.Join([]string{
		"Data: sumo-api.com (history, profiles; run by a volunteer) and osada.us (live; its policy",
		"allows search and AI input, not AI training). Derived statistics count sekitori bouts only;",
		"heya and hometown are today's values. The rating is an estimate, not betting advice.",
	}, "\n")
	body := lipgloss.JoinVertical(lipgloss.Left,
		t.Title.Render("Keys"),
		h.FullHelpView([][]key.Binding{m.top().Keys(), globalKeys}),
		"",
		t.Title.Render("Legend"),
		legend,
		"",
		t.Title.Render("About the data"),
		t.Dim.Render(notes),
		"",
		t.Dim.Render("? or esc to close"),
	)
	return lipgloss.NewStyle().Padding(1, 2).MaxHeight(height).Render(body)
}
