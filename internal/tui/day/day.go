package day

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/spoiler"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/bout"
	"github.com/thorstenpfister/basho/internal/tui/standings"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

// Day is the day view (SPEC §8.4): one division's bouts of one day.
type Day struct {
	id   domain.BashoID
	div  domain.Division
	day  int // 0 until the default day is known
	want int // the day asked for, 0: the default

	statuses     [domain.BashoDays]service.DayStatus
	haveStatuses bool

	data    *service.Day
	prov    *service.Provenance
	err     error
	loading bool // a request for the shown day is running: ↻
	req     int  // the current request; older answers are dropped

	race    *service.Standings // the wide layout's side panel
	raceFor refreshKey         // the day race belongs to

	fresh   map[int]time.Time // NEW: bouts a live poll found decided, by match number
	confirm *domain.Bout      // ctrl+r: the bout to reveal up to, awaiting y

	recapOpen bool           // r: the recap overlay
	recap     *service.Recap // nil with recapErr
	recapErr  error          // the recap failed
	recapFor  refreshKey     // the day recap and recapErr belong to

	spinning  bool
	frame     int
	refreshed map[refreshKey]time.Time // when R last refreshed a day

	cursor, offset int
	width, height  int
	winChance      bool
	pending        string // digits after the g prefix
}

var (
	keyPrev      = key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous day"))
	keyNext      = key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next day"))
	keyUp        = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "previous bout"))
	keyDown      = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "next bout"))
	keyJump      = key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9", "0", "!", "@", "#", "$", "%"), key.WithHelp("1–0 !–%", "day 1–15"))
	keyG         = key.NewBinding(key.WithKeys("g"), key.WithHelp("g13", "go to day 13"))
	keyDivision  = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "Makuuchi ⇄ Juryo"))
	keyWinChance = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "win chance column"))
	keyVS        = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "VS screen"))
	keyStandings = key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "standings"))
	keyRecap     = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "recap"))
	keyRefresh   = key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "refresh (30 s cooldown)"))

	keyRevealHere = key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reveal up to here"))
	keyRecapClose = key.NewBinding(key.WithKeys("r", "esc", "q", "b", "enter"))
)

// shifted maps the shifted digits to days 11–15 (SPEC §8.8).
var shifted = map[string]int{"!": 11, "@": 12, "#": 13, "$": 14, "%": 15}

// New returns the day view of tournament id; day 0 opens the default day
// (today during the tournament, else its last day).
func New(ctx *tui.Context, id domain.BashoID, day int) Day {
	div := domain.DivMakuuchi
	if ctx.Cfg.UI.Division == "juryo" {
		div = domain.DivJuryo
	}
	return Day{id: id, div: div, want: day, winChance: ctx.Cfg.UI.WinChanceColumn}
}

type (
	statusesMsg struct {
		id  domain.BashoID
		div domain.Division
		res service.Result[[domain.BashoDays]service.DayStatus]
		err error
	}
	// defaultDayMsg starts the view on a day: the one asked for, or the
	// service's default.
	defaultDayMsg struct {
		day int
		err error
	}
	dayMsg struct {
		req   int
		phase phase
		res   service.Result[service.Day]
		err   error
	}
	spinMsg struct{}
	raceMsg struct {
		key refreshKey
		res service.Result[service.Standings]
		err error
	}
)

// phase is a step of stale-while-revalidate (SPEC §7.2): the cache's copy
// first, even expired, then the sources.
type phase int

const (
	cachedPhase  phase = iota // snapshot and disk cache only
	freshPhase                // through the sources (revalidating)
	refreshPhase              // R: revalidate even fresh entries
	settlePhase               // after a live poll in spoiler-free mode: the cache only, no follow-up
)

type refreshKey struct {
	id  domain.BashoID
	div domain.Division
	day int
}

// refreshCooldown is how often R may refresh one day (SPEC §8.8).
const refreshCooldown = 30 * time.Second

var (
	spinner      = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	asciiSpinner = []string{"|", "/", "-", "\\"}
)

const spinInterval = 100 * time.Millisecond

// Init implements tui.Screen.
func (d Day) Init(ctx *tui.Context) tea.Cmd {
	svc, id, want := ctx.Svc, d.id, d.want
	start := func() tea.Msg { return defaultDayMsg{day: want} }
	if want == 0 {
		start = func() tea.Msg {
			n, err := svc.DefaultDay(context.Background(), id)
			return defaultDayMsg{n, err}
		}
	}
	return tea.Batch(d.loadStatuses(ctx), start)
}

func (d Day) loadStatuses(ctx *tui.Context) tea.Cmd {
	svc, id, div := ctx.Svc, d.id, d.div
	return func() tea.Msg {
		res, err := svc.DayStatuses(context.Background(), id, div)
		return statusesMsg{id, div, res, err}
	}
}

// load fetches the current day, the cache's copy first.
func (d Day) load(ctx *tui.Context) (Day, tea.Cmd) {
	d.req++
	d.loading = true
	d, spin := d.spin(ctx)
	return d, tea.Batch(d.fetch(ctx, cachedPhase), spin)
}

// fetch runs one phase of the load.
func (d Day) fetch(ctx *tui.Context, ph phase) tea.Cmd {
	svc, id, div, day, req, base := ctx.Svc, d.id, d.div, d.day, d.req, ctx.Data()
	return func() tea.Msg {
		c := base
		switch ph {
		case cachedPhase, settlePhase:
			c = service.CachedOnly(c)
		case refreshPhase:
			c = service.Revalidate(c)
		}
		res, err := svc.Day(c, id, div, day)
		return dayMsg{req, ph, res, err}
	}
}

// spin starts the spinner if it is not running.
func (d Day) spin(ctx *tui.Context) (Day, tea.Cmd) {
	if d.spinning {
		return d, nil
	}
	d.spinning = true
	return d, ctx.After(spinInterval, func(time.Time) tea.Msg { return spinMsg{} })
}

// Update implements tui.Screen.
func (d Day) Update(ctx *tui.Context, msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height-1
		if d.data != nil {
			return d, d.loadRace(ctx)
		}
	case raceMsg:
		if msg.err == nil && msg.key == (refreshKey{d.id, d.div, d.day}) {
			d.race, d.raceFor = &msg.res.Value, msg.key
		}
	case tui.SpoilersMsg:
		d.race, d.recap = nil, nil
		next, cmd := d.load(ctx)
		return next, tea.Batch(cmd, next.loadRace(ctx), next.loadRecap(ctx))
	case tui.LiveMsg:
		return d.live(ctx, msg.Poll)
	case recapMsg:
		if msg.key == (refreshKey{d.id, d.div, d.day}) {
			d.recap, d.recapErr, d.recapFor = nil, msg.err, msg.key
			if msg.err == nil {
				d.recap = &msg.res.Value
			}
		}
	case statusesMsg:
		if msg.id == d.id && msg.div == d.div && msg.err == nil {
			d.statuses, d.haveStatuses = msg.res.Value, true
		}
	case defaultDayMsg:
		if msg.err != nil {
			d.err = msg.err
			return d, nil
		}
		d.day = msg.day
		return d.load(ctx)
	case dayMsg:
		if msg.req != d.req {
			return d, nil // an answer for a day no longer shown
		}
		return d.answer(ctx, msg)
	case spinMsg:
		if !d.loading || d.data != nil {
			d.spinning = false
			return d, nil
		}
		d.frame++
		return d, ctx.After(spinInterval, func(time.Time) tea.Msg { return spinMsg{} })
	case tea.KeyPressMsg:
		return d.key(ctx, msg)
	}
	return d, nil
}

// answer takes a load's result. A cached copy shows at once; unless it is
// the snapshot's (final), the sources are asked next, and their answer
// replaces it when it comes (stale-while-revalidate).
func (d Day) answer(ctx *tui.Context, msg dayMsg) (tui.Screen, tea.Cmd) {
	if msg.err != nil && msg.phase == settlePhase {
		d.loading = false
		return d, nil // the shown copy stays
	}
	if msg.err != nil {
		if msg.phase == cachedPhase { // not cached: ask the sources
			return d, d.fetch(ctx, freshPhase)
		}
		d.loading = false
		if d.data != nil {
			return d, tui.Message("Refresh failed: showing the cached copy")
		}
		d.err = msg.err
		return d, nil
	}
	d.err = nil
	d.data, d.prov = &msg.res.Value, &msg.res.Prov
	d.cursor = min(d.cursor, max(len(d.rows())-1, 0))
	d.scroll()
	race := tea.Batch(d.loadRace(ctx), d.loadRecap(ctx))
	if msg.phase == cachedPhase && msg.res.Prov.Source != service.SnapshotSource {
		return d, tea.Batch(d.fetch(ctx, freshPhase), race) // revalidate in the background: ↻
	}
	d.loading = false
	return d, race
}

// loadRace fetches the standings for the side panel of the wide layout; the
// day's data is cached by now, so it costs no request.
func (d Day) loadRace(ctx *tui.Context) tea.Cmd {
	if tui.LayoutOf(d.width) != tui.Wide || d.day == 0 {
		return nil
	}
	k := refreshKey{d.id, d.div, d.day}
	if d.race != nil && d.raceFor == k {
		return nil
	}
	svc, base := ctx.Svc, ctx.Data()
	return func() tea.Msg {
		res, err := svc.Standings(service.CachedOnly(base), k.id, k.div, k.day)
		return raceMsg{k, res, err}
	}
}

// refresh is R: revalidate the shown day with its source, at most every
// 30 s; snapshot days are final.
func (d Day) refresh(ctx *tui.Context) (tui.Screen, tea.Cmd) {
	if d.day == 0 {
		return d, nil
	}
	if d.err == nil && d.prov != nil && d.prov.Source == service.SnapshotSource {
		return d, tui.Message("Finished tournaments in the snapshot do not change")
	}
	k := refreshKey{d.id, d.div, d.day}
	now := ctx.Clock.Now()
	if at, ok := d.refreshed[k]; ok && now.Sub(at) < refreshCooldown {
		wait := (refreshCooldown - now.Sub(at)).Round(time.Second)
		return d, tui.Message(fmt.Sprintf("Refreshed a moment ago; again in %s", wait))
	}
	if d.refreshed == nil {
		d.refreshed = map[refreshKey]time.Time{}
	}
	d.refreshed[k] = now
	d.req++
	d.loading = true
	return d, d.fetch(ctx, refreshPhase)
}

func (d Day) key(ctx *tui.Context, msg tea.KeyPressMsg) (tui.Screen, tea.Cmd) {
	s := msg.String()
	if d.recapOpen {
		return d.recapKey(ctx, msg)
	}
	if d.confirm != nil {
		return d.confirmKey(ctx, s)
	}
	if d.pending != "" || s == "g" {
		return d.gPrefix(ctx, s)
	}
	switch {
	case key.Matches(msg, keyPrev):
		return d.step(ctx, -1)
	case key.Matches(msg, keyNext):
		return d.step(ctx, 1)
	case key.Matches(msg, keyUp):
		d.cursor = max(d.cursor-1, 0)
		d.scroll()
	case key.Matches(msg, keyDown):
		d.cursor = min(d.cursor+1, max(len(d.rows())-1, 0))
		d.scroll()
	case key.Matches(msg, keyJump):
		n, ok := shifted[s]
		if !ok {
			n = int(s[0] - '0')
			if n == 0 {
				n = 10
			}
		}
		return d.goTo(ctx, n)
	case key.Matches(msg, keyDivision):
		d.div = domain.DivJuryo + domain.DivMakuuchi - d.div
		d.haveStatuses, d.cursor, d.offset, d.data, d.err = false, 0, 0, nil, nil
		next, cmd := d.load(ctx)
		return next, tea.Batch(cmd, next.loadStatuses(ctx))
	case key.Matches(msg, keyWinChance):
		if d.data != nil && len(d.data.Bouts) > 0 && d.data.Bouts[0].WinChance == nil {
			return d, tui.Message("Win chances are hidden: the rating did not pass calibration")
		}
		d.winChance = !d.winChance
	case key.Matches(msg, keyRefresh):
		return d.refresh(ctx)
	case key.Matches(msg, keyVS):
		if _, ok := d.Selected(); ok {
			return d, tui.Push(bout.New(d.id, d.div, d.day, d.data.Date, d.rows(), d.cursor))
		}
	case key.Matches(msg, keyStandings):
		if d.day > 0 {
			return d, tui.Push(standings.New(d.id, d.div, d.day))
		}
	case key.Matches(msg, keyRevealHere):
		return d.revealHere(ctx)
	case key.Matches(msg, keyRecap):
		if d.day > 0 {
			d.recapOpen = true
			return d, d.loadRecap(ctx)
		}
	}
	return d, nil
}

// freshFor is how long a NEW badge stays at most (SPEC §8.4).
const freshFor = 60 * time.Second

// live takes a live poll's answer for the shown day: the bouts that are
// newly decided get a NEW badge until the next poll or for 60 s, and the
// cursor stays where it is.
func (d Day) live(ctx *tui.Context, p service.LivePoll) (tui.Screen, tea.Cmd) {
	if p.Basho != d.id {
		return d, nil
	}
	statuses := d.loadStatuses(ctx)
	res, ok := p.Days[d.div]
	if !ok || p.Day != d.day {
		return d, statuses
	}
	if ctx.Hide { // the poll's answer shows everything: reload behind the horizon
		d.req++
		return d, tea.Batch(statuses, d.fetch(ctx, settlePhase))
	}
	decided := map[int]bool{}
	for _, r := range d.rows() {
		decided[r.MatchNo] = r.Winner != 0
	}
	had := d.data != nil
	now := ctx.Clock.Now()
	d.data, d.prov, d.err, d.fresh = &res.Value, &res.Prov, nil, map[int]time.Time{}
	d.req++ // an answer still on its way is older
	d.loading = false
	for _, r := range d.rows() {
		if had && r.Winner != 0 && !decided[r.MatchNo] {
			d.fresh[r.MatchNo] = now
		}
	}
	d.cursor = min(d.cursor, max(len(d.rows())-1, 0))
	d.scroll()
	d.race, d.recap = nil, nil // reloaded from the cache
	return d, tea.Batch(statuses, d.loadRace(ctx), d.loadRecap(ctx))
}

func (d Day) isFresh(ctx *tui.Context, r service.BoutRow) bool {
	at, ok := d.fresh[r.MatchNo]
	return ok && ctx.Clock.Now().Sub(at) < freshFor
}

// recapKey handles the keys while the recap is open: it takes every key
// (CapturesKeys), closes on r, esc, q, b and enter, and follows ←/→.
func (d Day) recapKey(ctx *tui.Context, msg tea.KeyPressMsg) (tui.Screen, tea.Cmd) {
	switch {
	case key.Matches(msg, keyRecapClose):
		d.recapOpen = false
	case key.Matches(msg, keyPrev):
		return d.step(ctx, -1)
	case key.Matches(msg, keyNext):
		return d.step(ctx, 1)
	}
	return d, nil
}

// CapturesKeys implements tui.KeyCapturer: esc and q close the recap
// instead of leaving the day view.
func (d Day) CapturesKeys() bool { return d.recapOpen || d.confirm != nil }

// revealHere is ctrl+r in spoiler-free mode: after a confirmation, the
// horizon moves through the selected bout (SPEC §9).
func (d Day) revealHere(ctx *tui.Context) (tui.Screen, tea.Cmd) {
	r, ok := d.Selected()
	switch {
	case !ctx.Hide || ctx.Spoilers == nil:
		return d, tui.Message("Spoiler-free mode is off: S turns it on")
	case !ok:
		return d, nil
	case !r.Hidden:
		return d, tui.Message("Already revealed")
	}
	b := domain.Bout{Basho: d.id, Division: d.div, Day: d.day, MatchNo: r.MatchNo, IsPlayoff: d.cursor >= len(d.data.Bouts)}
	d.confirm = &b
	return d, nil
}

func (d Day) confirmKey(ctx *tui.Context, s string) (tui.Screen, tea.Cmd) {
	b := *d.confirm
	d.confirm = nil
	if s != "y" && s != "Y" {
		return d, tui.Message("Nothing revealed")
	}
	if _, err := ctx.Spoilers.Reveal(b); err != nil {
		return d, tui.Message("Could not save the horizon: " + err.Error())
	}
	return d, tea.Batch(tui.Spoilers(), tui.Message("Revealed up to "+spoiler.Of(b).String()))
}

// gPrefix collects the two digits of g13.
func (d Day) gPrefix(ctx *tui.Context, s string) (tui.Screen, tea.Cmd) {
	if s == "g" && d.pending == "" {
		d.pending = "g"
		return d, nil
	}
	if len(s) != 1 || s[0] < '0' || s[0] > '9' {
		d.pending = ""
		return d, nil
	}
	d.pending += s
	if len(d.pending) < 3 {
		return d, nil
	}
	n := int(d.pending[1]-'0')*10 + int(d.pending[2]-'0')
	d.pending = ""
	return d.goTo(ctx, n)
}

// step moves to the previous or next day that has data.
func (d Day) step(ctx *tui.Context, dir int) (tui.Screen, tea.Cmd) {
	for n := d.day + dir; n >= 1 && n <= domain.BashoDays; n += dir {
		if d.selectable(n) {
			return d.goTo(ctx, n)
		}
		if !d.haveStatuses {
			break
		}
	}
	if n := d.day + dir; n >= 1 && n <= domain.BashoDays {
		return d, tui.Message(fmt.Sprintf("No bouts published for day %d yet", n))
	}
	return d, nil
}

func (d Day) selectable(n int) bool {
	return !d.haveStatuses || d.statuses[n-1] != service.NotPublished
}

func (d Day) goTo(ctx *tui.Context, n int) (tui.Screen, tea.Cmd) {
	if n < 1 || n > domain.BashoDays {
		return d, tui.Message(fmt.Sprintf("Days run from 1 to %d", domain.BashoDays))
	}
	if !d.selectable(n) {
		return d, tui.Message(fmt.Sprintf("No bouts published for day %d yet", n))
	}
	if n == d.day && d.data != nil {
		return d, nil
	}
	d.day, d.cursor, d.offset, d.data, d.err = n, 0, 0, nil, nil
	return d.load(ctx)
}

// rows is the regular bouts then the playoffs, in display order.
func (d Day) rows() []service.BoutRow {
	if d.data == nil {
		return nil
	}
	return append(append([]service.BoutRow{}, d.data.Bouts...), d.data.Playoffs...)
}

// Selected returns the bout under the cursor.
func (d Day) Selected() (service.BoutRow, bool) {
	rows := d.rows()
	if d.cursor >= len(rows) {
		return service.BoutRow{}, false
	}
	return rows[d.cursor], true
}

// chrome is the number of lines around the bout list: title, day bar (2),
// date line, rule, table header, rule, kyujo, keys.
const chrome = 9

func (d *Day) scroll() {
	visible := max(d.height-chrome, 1)
	switch {
	case d.cursor < d.offset:
		d.offset = d.cursor
	case d.cursor >= d.offset+visible:
		d.offset = d.cursor - visible + 1
	}
	d.offset = min(max(d.offset, 0), max(len(d.rows())-visible, 0))
}

// View implements tui.Screen.
func (d Day) View(ctx *tui.Context, width, height int) string {
	t := ctx.Theme
	var b strings.Builder
	other := domain.DivJuryo + domain.DivMakuuchi - d.div
	title := t.Title.Render(fmt.Sprintf("%s Basho %d", d.id.Name(), d.id.Year)) + t.Dim.Render(" · "+venue(d)+" · ") + t.Bold.Render(d.div.String())
	b.WriteString(tui.Spread(title, t.Dim.Render("[d] "+other.String()), width) + "\n")
	b.WriteString(d.dayBar(ctx) + "\n")

	switch {
	case d.err != nil && d.data == nil:
		b.WriteString("\n" + t.Error.Render(" "+dayError(d.err)) + "\n")
		if h := tui.ErrorHint(d.err); h != "" {
			b.WriteString(t.Dim.Render(" "+h) + "\n")
		}
		return b.String()
	case d.data == nil:
		frames := spinner
		if ctx.Cfg.UI.ASCII {
			frames = asciiSpinner
		}
		label := " Loading…"
		if d.day > 0 {
			label = fmt.Sprintf(" Loading day %d…", d.day)
		}
		b.WriteString("\n " + t.Accent.Render(frames[d.frame%len(frames)]) + t.Dim.Render(label) + "\n")
		return b.String()
	}
	if d.recapOpen {
		b.WriteString(d.recapView(ctx, width, height-3))
		return b.String()
	}
	data := d.data
	date := fmt.Sprintf("Day %d · %s %s", data.Day, data.Date.Weekday().String()[:3], data.Date)
	b.WriteString(tui.Spread(t.Bold.Render(date), t.Dim.Render(fmt.Sprintf("%d bouts", len(data.Bouts))), width) + "\n")
	b.WriteString(t.Rule.Render(" "+strings.Repeat(t.Line, max(width-2, 1))) + "\n")
	if len(data.Bouts) == 0 {
		b.WriteString(t.Dim.Render(fmt.Sprintf(" No bouts published for day %d yet.", data.Day)) + "\n")
		return b.String()
	}
	cols := columns(t, d.rows(), tui.LayoutOf(width) == tui.Compact, d.winChance)
	var table strings.Builder
	table.WriteString(t.Header.Render(cols.header()) + "\n")
	rows := d.rows()
	visible := max(height-chrome, 1)
	for i := d.offset; i < min(d.offset+visible, len(rows)); i++ {
		if i == len(data.Bouts) {
			table.WriteString(t.Dim.Render(fmt.Sprintf(" %s%s Kettei-sen %s%s", t.Line, t.Line, t.Line, t.Line)) + "\n")
		}
		table.WriteString(cols.row(t, rows[i], i == d.cursor, d.isFresh(ctx, rows[i])) + "\n")
	}
	if tui.LayoutOf(width) == tui.Wide && d.race != nil && d.raceFor == (refreshKey{d.id, d.div, d.day}) {
		left := lipgloss.NewStyle().Width(width - panelWidth - 1).Render(strings.TrimSuffix(table.String(), "\n"))
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, racePanel(ctx, *d.race, visible+1)) + "\n")
	} else {
		b.WriteString(table.String())
	}
	b.WriteString(t.Rule.Render(" "+strings.Repeat(t.Line, max(width-2, 1))) + "\n")
	b.WriteString(kyujo(t, d, width) + "\n")
	if c := d.confirm; c != nil {
		b.WriteString(t.Gold.Render(fmt.Sprintf(" Reveal every result up to Day %d bout %d? [y/N]", c.Day, c.MatchNo)))
		return b.String()
	}
	hints := " [enter] VS · [s] standings · [r] recap · ←/→ day · j/k bout · d division · ? help"
	if tui.LayoutOf(width) == tui.Compact {
		hints = " enter VS · ←/→ day · j/k bout · d division"
	}
	b.WriteString(t.Dim.Render(hints))
	return b.String()
}

// dayError is the error screen's text; R retries a failed load.
func dayError(err error) string {
	if service.Unavailable(err) {
		return tui.ErrorText(err) + " R retries."
	}
	return tui.ErrorText(err)
}

func venue(d Day) string {
	if d.data != nil && d.data.Venue != "" {
		return d.data.Venue
	}
	return d.id.Venue()
}

// dayBar is the two lines of day numbers and status marks.
func (d Day) dayBar(ctx *tui.Context) string {
	t := ctx.Theme
	var nums, marks strings.Builder
	nums.WriteString(" ")
	marks.WriteString(" ")
	for n := 1; n <= domain.BashoDays; n++ {
		label := fmt.Sprint(n)
		cell := fmt.Sprintf(" %s ", label)
		if n == d.day {
			cell = t.Selected.Render(fmt.Sprintf("[%s]", label))
		} else if d.haveStatuses && d.statuses[n-1] == service.NotPublished {
			cell = t.Dim.Render(cell)
		}
		nums.WriteString(cell)
		mark := ""
		if d.haveStatuses {
			switch d.statuses[n-1] {
			case service.Complete:
				mark = t.Complete
			case service.InProgress:
				mark = t.Accent.Render(t.InProgress)
			case service.Scheduled:
				mark = t.Scheduled
			}
		}
		w := lipgloss.Width(label) + 2
		marks.WriteString(lipgloss.PlaceHorizontal(w, lipgloss.Center, mark))
	}
	return nums.String() + "\n" + t.Dim.Render(marks.String())
}

func kyujo(t theme.Theme, d Day, width int) string {
	if d.data == nil || len(d.data.Kyujo) == 0 {
		return ""
	}
	names := make([]string, len(d.data.Kyujo))
	for i, w := range d.data.Kyujo {
		names[i] = fmt.Sprintf("%s (%s)", t.Name(w.Shikona), w.Rank.Short())
	}
	return tui.Truncate(" Kyujo: "+strings.Join(names, ", "), width)
}

// Status implements tui.Screen.
func (d Day) Status(*tui.Context) tui.Status {
	left := fmt.Sprintf("%s %d · %s", d.id.Name(), d.id.Year, d.div)
	if d.day > 0 {
		left += fmt.Sprintf(" · Day %d", d.day)
	}
	return tui.Status{Left: left, Prov: d.prov, Loading: d.loading}
}

// Keys implements tui.Screen.
func (d Day) Keys() []key.Binding {
	return []key.Binding{keyPrev, keyNext, keyUp, keyDown, keyJump, keyG, keyDivision, keyWinChance, keyVS, keyStandings, keyRecap, keyRefresh, keyRevealHere}
}

// panelWidth is the side panel of the wide layout.
const panelWidth = 36

// racePanel is the yusho race after the day: the leaders and those one win
// behind (SPEC §8.4), as many as fit.
func racePanel(ctx *tui.Context, s service.Standings, height int) string {
	t := ctx.Theme
	head := fmt.Sprintf("Yusho race · after Day %d", s.Day)
	if s.Limited {
		head = "Yusho race · spoiler-free"
	}
	lines := []string{t.Title.Render(head)}
	band := derive.Band(-1)
	for _, r := range s.Rows {
		if r.Band > derive.OneBack || len(lines) >= height {
			break
		}
		if r.Band != band {
			band = r.Band
			if band == derive.OneBack {
				lines = append(lines, t.Dim.Render(t.Line+t.Line+" −1 "+t.Line+t.Line))
			}
		}
		lines = append(lines, fmt.Sprintf("%5s  %-4s %s", r.Record.String(), r.Rank.Short(), tui.Truncate(t.Name(r.Shikona), panelWidth-16)))
	}
	if s.YushoDecided {
		lines = append(lines, t.Gold.Render("✓ yusho"))
	}
	return lipgloss.NewStyle().Width(panelWidth).Height(height).PaddingLeft(1).
		BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(t.Colors.Subtle).
		Render(strings.Join(lines, "\n"))
}
