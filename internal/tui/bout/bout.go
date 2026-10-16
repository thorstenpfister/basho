package bout

import (
	"fmt"
	"math"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/components"
	"github.com/thorstenpfister/basho/internal/tui/wrestler"
)

// VS is the tale of the tape of one bout (SPEC §8.5).
type VS struct {
	id    domain.BashoID
	div   domain.Division
	day   int
	match int
	row   service.BoutRow   // what the day view knew: drawn until the tale is in
	date  domain.Date       // of the day: an undecided bout today waits for its result
	rows  []service.BoutRow // the day's bouts in order, for [ and ]
	index int

	tale    *service.Tale
	prov    *service.Provenance
	err     error
	loading bool
	req     int

	slide, slideVel float64 // the names' slide-in: 0 at the edges, 1 in place
	sliding         bool
	flash           bool // the result panel's reveal flash
	revealing       bool // space: flash once the revealed tale is in

	pics          pictures      // rendered portraits, shared by the copies
	history       bool          // v: the last 10 meetings
	moveset       bool          // m: the moveset overlay
	window        derive.Window // w: its sample window
	width, height int
}

var (
	keyHistory = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "head-to-head history"))
	keyEast    = key.NewBinding(key.WithKeys("1"), key.WithHelp("1/2", "East/West wrestler's career"))
	keyWest    = key.NewBinding(key.WithKeys("2"))
	keyMoveset = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "moveset panel"))
	keyWindow  = key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "moveset window"))
	keyPrev    = key.NewBinding(key.WithKeys("["), key.WithHelp("[", "previous bout"))
	keyNext    = key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next bout"))
	keyReveal  = key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "reveal the result (spoiler-free)"))
	keyBack    = key.NewBinding(key.WithKeys("q", "esc", "b"), key.WithHelp("q/esc", "back to the day")) // the root pops
)

// New returns the VS screen of bout index of a day's rows (regular bouts,
// then playoffs) dated date.
func New(id domain.BashoID, div domain.Division, day int, date domain.Date, rows []service.BoutRow, index int) VS {
	v := VS{id: id, div: div, rows: rows, pics: pictures{}}
	return v.show(day, date, rows, index)
}

// show switches to bout index of a day.
func (v VS) show(day int, date domain.Date, rows []service.BoutRow, index int) VS {
	v.day, v.date, v.rows, v.index = day, date, rows, index
	v.row = rows[index]
	v.match = v.row.MatchNo
	v.tale, v.prov, v.err = nil, nil, nil
	v.req++
	return v
}

// dayMsg is an adjacent day's bouts, for [ and ] across days.
type dayMsg struct {
	dir int
	day int
	res service.Result[service.Day]
	err error
}

// step moves to the previous or next bout in day order, into the adjacent
// day at the ends (SPEC §8.8).
func (v VS) step(ctx *tui.Context, dir int) (tui.Screen, tea.Cmd) {
	if i := v.index + dir; i >= 0 && i < len(v.rows) {
		v = v.show(v.day, v.date, v.rows, i)
		return v, v.fetch(ctx, true)
	}
	return v, v.loadDay(ctx, dir, v.day+dir)
}

func (v VS) loadDay(ctx *tui.Context, dir, day int) tea.Cmd {
	if day < 1 || day > domain.BashoDays {
		edge := "first"
		if dir > 0 {
			edge = "last"
		}
		return tui.Message(fmt.Sprintf("This is the %s bout of the tournament", edge))
	}
	svc, id, div, base := ctx.Svc, v.id, v.div, ctx.Data()
	return func() tea.Msg {
		res, err := svc.Day(service.CachedOnly(base), id, div, day)
		if err != nil && !service.NotFound(err) {
			res, err = svc.Day(base, id, div, day)
		}
		return dayMsg{dir, day, res, err}
	}
}

// adjacent takes the next day in the direction of travel; a day without
// bouts (a gap in the source) is skipped.
func (v VS) adjacent(ctx *tui.Context, msg dayMsg) (tui.Screen, tea.Cmd) {
	rows := append(append([]service.BoutRow{}, msg.res.Value.Bouts...), msg.res.Value.Playoffs...)
	switch {
	case msg.err != nil && !service.NotFound(msg.err):
		return v, tui.Message("Could not load day " + fmt.Sprint(msg.day))
	case len(rows) == 0:
		return v, v.loadDay(ctx, msg.dir, msg.day+msg.dir)
	}
	i := 0
	if msg.dir < 0 {
		i = len(rows) - 1
	}
	v = v.show(msg.day, msg.res.Value.Date, rows, i)
	return v, v.fetch(ctx, true)
}

type taleMsg struct {
	req    int
	cached bool
	res    service.Result[service.Tale]
	err    error
}

// Init implements tui.Screen.
func (v VS) Init(ctx *tui.Context) tea.Cmd {
	return v.fetch(ctx, true)
}

// Animation (SPEC §8.5): the names slide in on a spring of about 250 ms and
// VS brightens; a reveal flashes the result panel for 150 ms.
type (
	frameMsg struct{}
	// RevealMsg flashes the result panel: the result was just revealed
	// (spoiler-free mode, Phase 11).
	RevealMsg    struct{}
	flashDoneMsg struct{}
)

const (
	flashTime = 150 * time.Millisecond
	fps       = 60
)

// spring is critically damped: it settles to 1% in about 6.6/ω, so ω 26
// gives the ~250 ms slide of SPEC §8.5.
var spring = harmonica.NewSpring(harmonica.FPS(fps), 26, 1)

func frame(ctx *tui.Context) tea.Cmd {
	return ctx.After(time.Second/fps, func(time.Time) tea.Msg { return frameMsg{} })
}

// startSlide begins the slide-in, or places the names at once when
// animations are off.
func (v VS) startSlide(ctx *tui.Context) (VS, tea.Cmd) {
	if !ctx.Animate(v.width) {
		v.slide, v.sliding = 1, false
		return v, nil
	}
	v.slide, v.slideVel, v.sliding = 0, 0, true
	return v, frame(ctx)
}

// fetch loads the tale: the cache's copy first, like the day view.
func (v VS) fetch(ctx *tui.Context, cached bool) tea.Cmd {
	svc, id, div, day, match, req, base := ctx.Svc, v.id, v.div, v.day, v.match, v.req, ctx.Data()
	return func() tea.Msg {
		c := base
		if cached {
			c = service.CachedOnly(c)
		}
		res, err := svc.Bout(c, id, div, day, match)
		return taleMsg{req, cached, res, err}
	}
}

// Update implements tui.Screen.
func (v VS) Update(ctx *tui.Context, msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		first := v.width == 0
		v.width, v.height = msg.Width, msg.Height-1
		if first {
			return v.startSlide(ctx)
		}
	case frameMsg:
		if !v.sliding {
			return v, nil
		}
		v.slide, v.slideVel = spring.Update(v.slide, v.slideVel, 1)
		if math.Abs(1-v.slide) < 0.02 && math.Abs(v.slideVel) < 0.1 { // within a cell of place
			v.slide, v.sliding = 1, false
			return v, nil
		}
		return v, frame(ctx)
	case RevealMsg:
		if !ctx.Animate(v.width) {
			return v, nil
		}
		v.flash = true
		return v, ctx.After(flashTime, func(time.Time) tea.Msg { return flashDoneMsg{} })
	case flashDoneMsg:
		v.flash = false
	case taleMsg:
		if msg.req != v.req {
			return v, nil
		}
		if msg.err != nil {
			if msg.cached {
				v.loading = true
				return v, v.fetch(ctx, false)
			}
			v.loading = false
			if v.tale == nil {
				v.err = msg.err
			}
			return v, nil
		}
		v.tale, v.prov, v.err = &msg.res.Value, &msg.res.Prov, nil
		var flash tea.Cmd
		if v.revealing && !v.tale.Hidden {
			v.revealing = false
			flash = func() tea.Msg { return RevealMsg{} }
		}
		if msg.cached && msg.res.Prov.Source != service.SnapshotSource {
			v.loading = true
			return v, tea.Batch(flash, v.fetch(ctx, false))
		}
		v.loading = false
		return v, flash
	case tui.SpoilersMsg:
		v.req++
		v.loading = true
		return v, v.fetch(ctx, true)
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyReveal):
			return v.reveal(ctx)
		case key.Matches(msg, keyEast):
			return v, tui.Push(wrestler.New(v.row.East.RikishiID, v.id))
		case key.Matches(msg, keyWest):
			return v, tui.Push(wrestler.New(v.row.West.RikishiID, v.id))
		case key.Matches(msg, keyHistory):
			v.history = !v.history
		case key.Matches(msg, keyMoveset):
			v.moveset = !v.moveset
		case key.Matches(msg, keyWindow) && v.moveset:
			v.window = (v.window + 1) % 3
		case key.Matches(msg, keyPrev):
			return v.step(ctx, -1)
		case key.Matches(msg, keyNext):
			return v.step(ctx, 1)
		}
	case dayMsg:
		return v.adjacent(ctx, msg)
	}
	return v, nil
}

// reveal is space in spoiler-free mode: the horizon moves through this
// bout, every screen reloads, and the result flashes in (SPEC §9).
func (v VS) reveal(ctx *tui.Context) (tui.Screen, tea.Cmd) {
	switch {
	case !ctx.Hide || ctx.Spoilers == nil:
		return v, nil
	case v.tale == nil || !v.tale.Hidden:
		return v, nil
	}
	b := domain.Bout{Basho: v.id, Division: v.div, Day: v.day, MatchNo: v.match, IsPlayoff: v.tale.Playoff}
	if _, err := ctx.Spoilers.Reveal(b); err != nil {
		return v, tui.Message("Could not save the horizon: " + err.Error())
	}
	v.revealing = true
	return v, tui.Spoilers()
}

// QuitGoesBack implements tui.QuitBacker: q returns to the day view.
func (v VS) QuitGoesBack() bool { return true }

// layout is the measures of one render.
type layout struct {
	tui.Layout
	width   int // of the tape, without the wide history column and portraits
	history int // the wide layout's head-to-head column; 0 otherwise
	side    int // each value column
	middle  int // the label column
}

// historyWidth is the wide layout's head-to-head column at least; it takes
// the width the tape, capped at 110, leaves over.
const historyWidth = 42

func measure(width int) layout {
	l := layout{Layout: tui.LayoutOf(width), width: width}
	if l.Layout == tui.Wide {
		l.width = width - historyWidth - 1
	}
	l.width = min(l.width, 110)
	if l.Layout == tui.Wide {
		l.history = width - l.width - 1
	}
	l.middle = 14
	l.side = (l.width - l.middle - 2) / 2
	return l
}

// View implements tui.Screen.
func (v VS) View(ctx *tui.Context, width, height int) string {
	t := ctx.Theme
	l := measure(width)
	var b strings.Builder
	b.WriteString(" " + t.Title.Render(v.heading()) + "\n")
	if v.err != nil && v.tale == nil {
		b.WriteString("\n " + t.Error.Render(tui.ErrorText(v.err)) + "\n")
		if h := tui.ErrorHint(v.err); h != "" {
			b.WriteString(" " + t.Dim.Render(h) + "\n")
		}
		return b.String()
	}
	tape := v.tape(ctx, l)
	if v.moveset && v.tale != nil {
		tape = v.movesetPanel(ctx, l)
	}
	if l.Layout == tui.Wide && v.tale != nil {
		col := lipgloss.NewStyle().Width(l.history).PaddingLeft(1).
			BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(t.Colors.Subtle).
			Render(v.historyView(ctx, l.history-2, true))
		tape = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(l.width+1).Render(tape), col)
	}
	rest := tape + "\n" + v.resultPanel(ctx, l) + "\n" + t.Dim.Render(" [ ] prev/next · v head-to-head · m moveset · 1/2 career · esc back")
	// The portraits go above the names, in the rows the rest leaves free.
	if size := v.portraitSize(ctx, l, height-1-lipgloss.Height(rest)); size > 0 {
		b.WriteString(v.portraitRow(ctx, l, size) + "\n")
	}
	b.WriteString(rest)
	return b.String()
}

func (v VS) heading() string {
	s := fmt.Sprintf("%s %d · Day %d · Bout %d", v.id.Name(), v.id.Year, v.day, v.match)
	if v.tale != nil {
		s += fmt.Sprintf("/%d", v.tale.Of)
		switch {
		case v.tale.Playoff:
			s += " · KETTEI-SEN"
		case v.tale.Musubi:
			s += " · MUSUBI-NO-ICHIBAN"
		}
	} else if v.row.Musubi {
		s += " · MUSUBI-NO-ICHIBAN"
	}
	return s
}

// tape draws the mirrored columns (SPEC §8.5).
func (v VS) tape(ctx *tui.Context, l layout) string {
	t := ctx.Theme
	row := func(left, label, right string) string {
		return " " + components.StatRow(t, left, label, right, l.side, l.middle)
	}
	var lines []string
	east, west := v.row.East, v.row.West
	lines = append(lines, v.names(ctx, l, east, west))
	if v.tale == nil {
		// Progressive loading (SPEC §8.5): what the day view knew at once,
		// placeholders for the rest.
		wait := t.Dim.Render("…")
		if alt := [2]string{t.AltName(east.Shikona), t.AltName(west.Shikona)}; alt != [2]string{} {
			lines = append(lines, row(alt[0], "", alt[1]))
		}
		lines = append(lines, " "+t.Rule.Render(strings.Repeat(t.Line, l.width-2)))
		lines = append(lines, row(east.Rank.String(), "RANK", west.Rank.String()))
		for _, label := range []string{"HEYA", "FROM", "AGE"} {
			lines = append(lines, row(wait, label, wait))
		}
		lines = append(lines, row(record(east), "RECORD", record(west)))
		for _, label := range []string{"LAST 5", "HEIGHT", "WEIGHT", "KINBOSHI", "YUSHO"} {
			lines = append(lines, row(wait, label, wait))
		}
		lines = append(lines, " "+t.Rule.Render(strings.Repeat(t.Line, l.width-2)), " "+t.Dim.Render("H2H  …"))
		return strings.Join(lines, "\n")
	}
	e, w := v.tale.East, v.tale.West
	if alt := [2]string{t.AltName(e.Shikona), t.AltName(w.Shikona)}; alt != [2]string{} {
		lines = append(lines, row(alt[0], "", alt[1]))
	}
	if et, wt := e.Moveset().Type.String(), w.Moveset().Type.String(); et != "" || wt != "" {
		lines = append(lines, row(t.Dim.Render(et), "", t.Dim.Render(wt)))
	}
	lines = append(lines, " "+t.Rule.Render(strings.Repeat(t.Line, l.width-2)))
	lines = append(lines,
		row(e.Rank.String(), "RANK", w.Rank.String()),
		row(orUnknown(e.Heya, e.Profile), "HEYA", orUnknown(w.Heya, w.Profile)),
		row(orUnknown(e.Shusshin, e.Profile), "FROM", orUnknown(w.Shusshin, w.Profile)),
		row(age(e), "AGE", age(w)),
		row(taleRecord(e), "RECORD", taleRecord(w)),
		row(components.Pips(t, e.Last5, 5), "LAST 5", components.Pips(t, w.Last5, 5)),
	)
	lines = append(lines, v.measures(ctx, l, row)...)
	lines = append(lines,
		row(components.Stars(t, e.Kinboshi), "KINBOSHI", components.Stars(t, w.Kinboshi)),
		row(fmt.Sprint(e.Career.Yusho[domain.DivMakuuchi]), "YUSHO", fmt.Sprint(w.Career.Yusho[domain.DivMakuuchi])),
		row(styleBar(ctx, e.Moveset(), true, l), "STYLE", styleBar(ctx, w.Moveset(), false, l)),
		row(signature(ctx, e.Moveset()), "SIGNATURE", signature(ctx, w.Moveset())),
	)
	lines = append(lines, v.rating(ctx, l, row)...)
	if e.Career.Now != "" || w.Career.Now != "" {
		lines = append(lines, row(t.Dim.Render(now(e.Career.Now)), "", t.Dim.Render(now(w.Career.Now))))
	}
	lines = append(lines, " "+t.Rule.Render(strings.Repeat(t.Line, l.width-2)))
	lines = append(lines, v.headToHead(ctx, l)...)
	if v.history && l.Layout != tui.Wide {
		lines = append(lines, v.historyView(ctx, l.width-4, false))
	}
	return strings.Join(lines, "\n")
}

// measures are the HEIGHT and WEIGHT rows: values next to the label, bars
// outwards, scaled to the division (none in the compact layout).
func (v VS) measures(ctx *tui.Context, l layout, row func(string, string, string) string) []string {
	t := ctx.Theme
	e, w := v.tale.East, v.tale.West
	value := func(x float64, unit string, latest bool) string {
		if x == 0 {
			return "—"
		}
		s := fmt.Sprintf("%.0f%s", x, unit)
		if latest {
			s += t.Dim.Render("*")
		}
		return s
	}
	pair := func(ex, wx float64, unit, label string) string {
		return row(value(ex, unit, e.Career.Latest), label, value(wx, unit, w.Career.Latest))
	}
	return []string{
		pair(e.Career.Height, w.Career.Height, "cm", "HEIGHT"),
		pair(e.Career.Weight, w.Career.Weight, "kg", "WEIGHT"),
	}
}

// headToHead is the H2H line: wins on both sides with a split bar, and each
// side's top techniques.
func (v VS) headToHead(ctx *tui.Context, l layout) []string {
	t := ctx.Theme
	h := v.tale.HeadToHead
	total := h.A.Wins + h.B.Wins
	cells := max(l.width-14, 10)
	bar := t.Dim.Render(strings.Repeat("░", cells))
	if total > 0 {
		en := h.A.Wins * cells / total
		bar = t.Accent.Render(strings.Repeat("━", en)) + t.Dim.Render(strings.Repeat("░", cells-en))
	}
	lines := []string{fmt.Sprintf(" %s %3d %s %d", t.Dim.Render("H2H"), h.A.Wins, bar, h.B.Wins)}
	if total == 0 {
		return []string{" " + t.Dim.Render("H2H  first meeting")}
	}
	tops := components.StatRow(t, counts(h.A.Top), "", counts(h.B.Top), l.side, l.middle)
	return append(lines, " "+tops)
}

// historyView lists the last meetings (v): tournament, day, ranks, winner,
// technique. In the wide layout it is a side column and always shown.
func (v VS) historyView(ctx *tui.Context, width int, column bool) string {
	t := ctx.Theme
	h := v.tale.HeadToHead
	title := "Head-to-head"
	if !column {
		title = "Last meetings"
	}
	lines := []string{" " + t.Title.Render(title)}
	if len(h.Meetings) == 0 {
		return strings.Join(append(lines, " "+t.Dim.Render("none before this bout")), "\n")
	}
	n := len(h.Meetings)
	if !column && !v.history {
		n = min(n, 3)
	}
	for _, m := range h.Meetings[:n] {
		who := t.Name(v.tale.East.Shikona)
		if m.Winner == v.tale.West.RikishiID {
			who = t.Name(v.tale.West.Shikona)
		}
		line := fmt.Sprintf(" %s %2d %-4s %-4s %s %s", m.Basho, m.Day, m.RankA.Short(), m.RankB.Short(), t.Bold.Render(who), t.Dim.Render(string(m.Kimarite)))
		lines = append(lines, tui.Truncate(line, width))
	}
	return strings.Join(lines, "\n")
}

func record(s service.BoutSide) string {
	if s.Visitor {
		return "—"
	}
	return s.Before.String()
}

func taleRecord(s service.TaleSide) string {
	if s.Visitor {
		return "—"
	}
	return s.Record.String()
}

func orUnknown(s string, profile bool) string {
	switch {
	case !profile:
		return "profile unavailable"
	case s == "":
		return "—"
	default:
		return s
	}
}

func age(s service.TaleSide) string {
	if s.Career.Age == 0 {
		return "—"
	}
	return fmt.Sprint(s.Career.Age)
}

func now(name string) string {
	if name == "" {
		return ""
	}
	return "now: " + name
}

func counts(top []derive.KimariteCount) string {
	parts := make([]string, len(top))
	for i, k := range top {
		parts[i] = fmt.Sprintf("%s ×%d", k.Kimarite, k.Count)
	}
	return strings.Join(parts, " · ")
}

// Status implements tui.Screen.
func (v VS) Status(*tui.Context) tui.Status {
	return tui.Status{Left: fmt.Sprintf("%s %d · %s · Day %d · Bout %d", v.id.Name(), v.id.Year, v.div, v.day, v.match), Prov: v.prov, Loading: v.loading}
}

// Keys implements tui.Screen.
func (v VS) Keys() []key.Binding {
	return []key.Binding{keyPrev, keyNext, keyReveal, keyHistory, keyMoveset, keyWindow, keyEast, keyBack}
}

// resultPanel is the box under the tape (SPEC §8.5): the winner, the
// technique in romaji, kanji and English, the records after the bout and
// the KINBOSHI or FUSEN stamp; or the undecided state.
func (v VS) resultPanel(ctx *tui.Context, l layout) string {
	t := ctx.Theme
	var lines []string
	switch r := v.result(); {
	case r == nil && v.tale == nil && v.row.Winner == 0 && v.err == nil:
		lines = []string{t.Dim.Render("…")}
	case v.hidden():
		lines = []string{t.Dim.Render("Result hidden (spoiler-free)"), t.Accent.Render("[space] reveal")}
	case r == nil:
		lines = []string{t.Dim.Render(v.undecided(ctx))}
	default:
		e, w := t.Name(v.row.East.Shikona), t.Name(v.row.West.Shikona)
		winner := e
		if r.Winner == domain.West {
			winner = w
		}
		head := t.Bold.Render(strings.ToUpper(winner)) + " wins by " + t.Bold.Render(strings.ToUpper(string(r.Technique.Kimarite)))
		if k := t.Kanji(r.Technique.Kanji); k != "" {
			head += "  " + k
		}
		lines = append(lines, head)
		if r.Technique.English != "" {
			lines = append(lines, t.Dim.Render(r.Technique.English))
		}
		lines = append(lines, fmt.Sprintf("%s %s   ·   %s %s", e, recordAfter(r.East, v.row.East.Visitor), w, recordAfter(r.West, v.row.West.Visitor)))
		var stamps []string
		if r.Kinboshi {
			stamps = append(stamps, t.Gold.Render(t.Kinboshi+" KINBOSHI"))
		}
		if r.Fusen {
			stamps = append(stamps, t.Accent.Render("FUSEN (default)"))
		}
		if len(stamps) > 0 {
			lines = append(lines, strings.Join(stamps, "   "))
		}
	}
	inner := max(l.width-4, 10)
	body := make([]string, len(lines))
	for i, s := range lines {
		body[i] = lipgloss.PlaceHorizontal(inner, lipgloss.Center, s)
	}
	style := lipgloss.NewStyle()
	if v.flash {
		style = style.Reverse(true)
	}
	return lipgloss.NewStyle().MarginLeft(1).Border(lipgloss.RoundedBorder()).BorderForeground(t.Colors.Subtle).
		Render(style.Render(strings.Join(body, "\n")))
}

// hidden reports a result behind the spoiler horizon.
func (v VS) hidden() bool {
	if v.tale != nil {
		return v.tale.Hidden
	}
	return v.row.Hidden
}

// result is the decided bout from the tale, or from the day's row while the
// tale loads.
func (v VS) result() *service.TaleResult {
	if v.tale != nil {
		return v.tale.Result
	}
	if v.row.Winner == 0 {
		return nil
	}
	return &service.TaleResult{
		Winner: v.row.Winner, Technique: v.row.Kimarite, Kinboshi: v.row.Kinboshi, Fusen: v.row.Fusen,
		East: v.row.East.After, West: v.row.West.After,
	}
}

// undecided is the panel of a bout without a result: today's waits, later
// ones are scheduled.
func (v VS) undecided(ctx *tui.Context) string {
	if v.date == domain.DateOf(ctx.Clock.Now()) {
		return "Waiting for result…"
	}
	return fmt.Sprintf("Scheduled — Day %d", v.day)
}

func recordAfter(r derive.Record, visitor bool) string {
	if visitor {
		return ""
	}
	return r.String()
}

// notEnough is the STYLE and SIGNATURE rows below 10 wins (SPEC §8.5).
const notEnough = "— not enough data —"

// styleBar is the STYLE row's bar: 20 cells, 10 in the compact layout.
func styleBar(ctx *tui.Context, p derive.Profile, left bool, l layout) string {
	if !p.Enough() {
		return ctx.Theme.Dim.Render(notEnough)
	}
	cells := components.StyleCells
	if l.Layout == tui.Compact {
		cells /= 2
	}
	return components.StyleBar(ctx.Theme, p.WinShare, cells, left)
}

func signature(ctx *tui.Context, p derive.Profile) string {
	if !p.Enough() {
		return ctx.Theme.Dim.Render(notEnough)
	}
	return fmt.Sprintf("%s %.0f%%", p.Signature.Kimarite, 100*p.SignatureShare)
}

// movesetPanel is the m overlay: both technique profiles side by side in
// the chosen window, the matchup line and the window switch (SPEC §8.5).
func (v VS) movesetPanel(ctx *tui.Context, l layout) string {
	t := ctx.Theme
	col := max((l.width-4)/2, 30)
	side := func(s service.TaleSide) string {
		p := s.Movesets[v.window]
		lines := []string{
			t.Bold.Render(strings.ToUpper(t.Name(s.Shikona))) + t.Dim.Render(fmt.Sprintf("   %d W · %d L", p.Wins, p.Losses)),
			t.Dim.Render(orDash(p.Type.String())),
		}
		cells := min(20, col-16)
		for _, c := range components.Categories {
			n := int(p.WinShare[c]*float64(cells) + 0.5)
			bar := lipgloss.NewStyle().Foreground(t.Colors.Categories[c]).Render(strings.Repeat("█", n)) + t.Dim.Render(strings.Repeat("░", cells-n))
			if t.ASCII {
				bar = strings.Repeat("#", n) + strings.Repeat(".", cells-n)
			}
			lines = append(lines, fmt.Sprintf("%-6s %s %4.0f%%", strings.ToUpper(c.String()), bar, 100*p.WinShare[c]))
		}
		lines = append(lines,
			"Wins by   "+orDash(counts(p.TopWins)),
			"Loses to  "+orDash(counts(p.TopLosses)),
		)
		for i := range lines {
			lines[i] = tui.Truncate(lines[i], col)
		}
		return lipgloss.NewStyle().Width(col).Render(strings.Join(lines, "\n"))
	}
	var windows []string
	for w := derive.LastSix; w <= derive.ThisBasho; w++ {
		name := w.String()
		if w == v.window {
			name = t.Selected.Render(name)
		} else {
			name = t.Dim.Render(name)
		}
		windows = append(windows, name)
	}
	out := []string{
		" " + t.Title.Render("MOVESET · "+v.window.String()) + t.Dim.Render("   [m] close"),
		"",
		lipgloss.JoinHorizontal(lipgloss.Top, "  ", side(v.tale.East), "  ", side(v.tale.West)),
		" " + t.Rule.Render(strings.Repeat(t.Line, l.width-2)),
	}
	if m := v.tale.Matchups[v.window]; m != nil {
		att, def := t.Name(v.tale.East.Shikona), t.Name(v.tale.West.Shikona)
		if !m.AttackerIsA {
			att, def = def, att
		}
		cat := strings.ToUpper(m.Category.String())
		out = append(out, tui.Truncate(fmt.Sprintf("  Matchup: %s wins %.0f%% by %s; %s loses %.0f%% to %s", att, 100*m.WinShare, cat, def, 100*m.LossShare, cat), l.width))
	}
	out = append(out, "  Window: "+t.StatusKey.Render("[w]")+" "+strings.Join(windows, " ▸ "))
	return strings.Join(out, "\n")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// rating is the RATING and WIN CHANCE rows: ratings before the bout with the
// change since the tournament began, and the win chance drawn from the
// centre outwards. Hidden when the rating failed calibration (V9) or
// ui.win_chance is off.
func (v VS) rating(ctx *tui.Context, l layout, row func(string, string, string) string) []string {
	t := ctx.Theme
	e, w := v.tale.East.Rating, v.tale.West.Rating
	if !ctx.Cfg.UI.WinChance || e == nil || w == nil || v.tale.WinChance == nil {
		return nil
	}
	value := func(r *service.Rating) string {
		s := fmt.Sprintf("%.0f", r.Value)
		switch c := int(math.Round(r.Change)); {
		case c > 0:
			s += " " + t.Green.Render(fmt.Sprintf("%s%d", t.Up, c))
		case c < 0:
			s += " " + t.Red.Render(fmt.Sprintf("%s%d", t.Down, -c))
		}
		if r.Provisional {
			s += t.Dim.Render(" new")
		}
		return s
	}
	left, right := components.WinChance(t, *v.tale.WinChance, max(l.side-6, 4))
	return []string{
		row(value(e), "RATING", value(w)),
		row(left, "WIN CHANCE", right),
	}
}

// names is the top row: the names slide in from the edges while the slide
// runs, and VS goes from dim to bright.
func (v VS) names(ctx *tui.Context, l layout, east, west service.BoutSide) string {
	t := ctx.Theme
	e, w := strings.ToUpper(t.Name(east.Shikona)), strings.ToUpper(t.Name(west.Shikona))
	shift := 0
	if v.sliding {
		shift = int((1 - math.Min(math.Max(v.slide, 0), 1)) * float64(max(l.side-lipgloss.Width(e), 0)))
	}
	vs := t.Bold.Render("VS")
	if v.sliding && v.slide < 0.6 {
		vs = t.Dim.Render("VS")
	}
	left := t.Bold.Render(e) + strings.Repeat(" ", shift)
	right := strings.Repeat(" ", shift) + t.Bold.Render(w)
	return " " + lipgloss.PlaceHorizontal(l.side, lipgloss.Right, left) +
		lipgloss.PlaceHorizontal(l.middle, lipgloss.Center, vs) +
		lipgloss.PlaceHorizontal(l.side, lipgloss.Left, right)
}
