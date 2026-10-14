package bout

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/components"
)

// VS is the tale of the tape of one bout (SPEC §8.5).
type VS struct {
	id    domain.BashoID
	div   domain.Division
	day   int
	match int
	row   service.BoutRow // what the day view knew: drawn until the tale is in
	date  domain.Date     // of the day: an undecided bout today waits for its result

	tale    *service.Tale
	prov    *service.Provenance
	err     error
	loading bool
	req     int

	history       bool          // v: the last 10 meetings
	moveset       bool          // m: the moveset overlay
	window        derive.Window // w: its sample window
	width, height int
}

var (
	keyHistory = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "head-to-head history"))
	keyMoveset = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "moveset panel"))
	keyWindow  = key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "moveset window"))
	keyBack    = key.NewBinding(key.WithKeys("q", "esc", "b"), key.WithHelp("q/esc", "back to the day")) // the root pops
)

// New returns the VS screen of a bout; row is the day view's row of it and
// date the day's date.
func New(id domain.BashoID, div domain.Division, day int, date domain.Date, row service.BoutRow) VS {
	return VS{id: id, div: div, day: day, date: date, match: row.MatchNo, row: row}
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

// fetch loads the tale: the cache's copy first, like the day view.
func (v VS) fetch(ctx *tui.Context, cached bool) tea.Cmd {
	svc, id, div, day, match, req := ctx.Svc, v.id, v.div, v.day, v.match, v.req
	return func() tea.Msg {
		c := context.Background()
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
		v.width, v.height = msg.Width, msg.Height-1
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
		if msg.cached && msg.res.Prov.Source != service.SnapshotSource {
			v.loading = true
			return v, v.fetch(ctx, false)
		}
		v.loading = false
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyHistory):
			v.history = !v.history
		case key.Matches(msg, keyMoveset):
			v.moveset = !v.moveset
		case key.Matches(msg, keyWindow) && v.moveset:
			v.window = (v.window + 1) % 3
		}
	}
	return v, nil
}

// QuitGoesBack implements tui.QuitBacker: q returns to the day view.
func (v VS) QuitGoesBack() bool { return true }

// layout is the measures of one render.
type layout struct {
	tui.Layout
	width  int // of the tape, without the wide history column
	side   int // each value column
	middle int // the label column
}

// historyWidth is the wide layout's head-to-head column.
const historyWidth = 42

func measure(width int) layout {
	l := layout{Layout: tui.LayoutOf(width), width: width}
	if l.Layout == tui.Wide {
		l.width = width - historyWidth - 1
	}
	l.width = min(l.width, 110)
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
		b.WriteString("\n " + t.Error.Render(errorText(v.err)) + "\n")
		return b.String()
	}
	tape := v.tape(ctx, l)
	if v.moveset && v.tale != nil {
		tape = v.movesetPanel(ctx, l)
	}
	if l.Layout == tui.Wide && v.tale != nil {
		col := lipgloss.NewStyle().Width(historyWidth).PaddingLeft(1).
			BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(t.Colors.Subtle).
			Render(v.historyView(ctx, historyWidth-2, true))
		tape = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(l.width+1).Render(tape), col)
	}
	b.WriteString(tape + "\n")
	b.WriteString(v.resultPanel(ctx, l) + "\n")
	b.WriteString(t.Dim.Render(" [ ] prev/next · v head-to-head · m moveset · esc back"))
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
	lines = append(lines,
		row(t.Bold.Render(strings.ToUpper(east.Shikona.En)), "VS", t.Bold.Render(strings.ToUpper(west.Shikona.En))),
	)
	if v.tale == nil {
		// Progressive loading (SPEC §8.5): what the day view knew at once,
		// placeholders for the rest.
		wait := t.Dim.Render("…")
		lines = append(lines, row(east.Shikona.Jp, "", west.Shikona.Jp), " "+t.Rule.Render(strings.Repeat(t.Line, l.width-2)))
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
	if e.Shikona.Jp != "" || w.Shikona.Jp != "" {
		lines = append(lines, row(e.Shikona.Jp, "", w.Shikona.Jp))
	}
	lines = append(lines, row(t.Dim.Render(e.Moveset().Type.String()), "", t.Dim.Render(w.Moveset().Type.String())))
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
	e, w, sc := v.tale.East, v.tale.West, v.tale.Scale
	cells := l.side - 8
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
	pair := func(ex, wx, lo, hi float64, unit, label string) string {
		left, right := value(ex, unit, e.Career.Latest), value(wx, unit, w.Career.Latest)
		if l.Layout != tui.Compact && cells > 4 {
			left = components.Bar(t, ex, lo, hi, cells-lipgloss.Width(left), true) + " " + left
			right += " " + components.Bar(t, wx, lo, hi, cells-lipgloss.Width(right), false)
		}
		return row(left, label, right)
	}
	return []string{
		pair(e.Career.Height, w.Career.Height, sc.MinHeight, sc.MaxHeight, "cm", "HEIGHT"),
		pair(e.Career.Weight, w.Career.Weight, sc.MinWeight, sc.MaxWeight, "kg", "WEIGHT"),
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
		who := v.tale.East.Shikona.En
		if m.Winner == v.tale.West.RikishiID {
			who = v.tale.West.Shikona.En
		}
		line := fmt.Sprintf(" %s %2d %-4s %-4s %s %s", m.Basho, m.Day, m.RankA.Short(), m.RankB.Short(), t.Bold.Render(who), t.Dim.Render(string(m.Kimarite)))
		lines = append(lines, tui.Truncate(line, width))
	}
	return strings.Join(lines, "\n")
}

func errorText(err error) string {
	switch {
	case service.NotFound(err):
		return "Not available: " + err.Error()
	case service.Unavailable(err):
		return "Every source failed and nothing is cached."
	default:
		return err.Error()
	}
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
func (v VS) Keys() []key.Binding { return []key.Binding{keyHistory, keyMoveset, keyWindow, keyBack} }

// resultPanel is the box under the tape (SPEC §8.5): the winner, the
// technique in romaji, kanji and English, the records after the bout and
// the KINBOSHI or FUSEN stamp; or the undecided state.
func (v VS) resultPanel(ctx *tui.Context, l layout) string {
	t := ctx.Theme
	var lines []string
	switch r := v.result(); {
	case r == nil && v.tale == nil && v.row.Winner == 0 && v.err == nil:
		lines = []string{t.Dim.Render("…")}
	case r == nil:
		lines = []string{t.Dim.Render(v.undecided(ctx))}
	default:
		e, w := v.row.East.Shikona.En, v.row.West.Shikona.En
		winner := e
		if r.Winner == domain.West {
			winner = w
		}
		head := t.Bold.Render(strings.ToUpper(winner)) + " wins by " + t.Bold.Render(strings.ToUpper(string(r.Technique.Kimarite)))
		if r.Technique.Kanji != "" {
			head += "  " + r.Technique.Kanji
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
	return lipgloss.NewStyle().MarginLeft(1).Border(lipgloss.RoundedBorder()).BorderForeground(t.Colors.Subtle).
		Render(strings.Join(body, "\n"))
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

var categories = []domain.KimariteCategory{domain.Push, domain.Belt, domain.Throw, domain.Pull, domain.Other}

// movesetPanel is the m overlay: both technique profiles side by side in
// the chosen window, the matchup line and the window switch (SPEC §8.5).
func (v VS) movesetPanel(ctx *tui.Context, l layout) string {
	t := ctx.Theme
	col := max((l.width-4)/2, 30)
	side := func(s service.TaleSide) string {
		p := s.Movesets[v.window]
		lines := []string{
			t.Bold.Render(strings.ToUpper(s.Shikona.En)) + t.Dim.Render(fmt.Sprintf("   %d W · %d L", p.Wins, p.Losses)),
			t.Dim.Render(orDash(p.Type.String())),
		}
		cells := min(20, col-16)
		for _, c := range categories {
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
		att, def := v.tale.East.Shikona.En, v.tale.West.Shikona.En
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
