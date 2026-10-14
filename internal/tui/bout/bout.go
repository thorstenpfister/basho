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

	tale    *service.Tale
	prov    *service.Provenance
	err     error
	loading bool
	req     int

	history       bool // v: the last 10 meetings
	width, height int
}

var (
	keyHistory = key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "head-to-head history"))
	keyBack    = key.NewBinding(key.WithKeys("q", "esc", "b"), key.WithHelp("q/esc", "back to the day")) // the root pops
)

// New returns the VS screen of a bout; row is the day view's row of it.
func New(id domain.BashoID, div domain.Division, day int, row service.BoutRow) VS {
	return VS{id: id, div: div, day: day, match: row.MatchNo, row: row}
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
		if key.Matches(msg, keyHistory) {
			v.history = !v.history
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
	if l.Layout == tui.Wide && v.tale != nil {
		col := lipgloss.NewStyle().Width(historyWidth).PaddingLeft(1).
			BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(t.Colors.Subtle).
			Render(v.historyView(ctx, historyWidth-2, true))
		tape = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(l.width+1).Render(tape), col)
	}
	b.WriteString(tape + "\n")
	b.WriteString(t.Dim.Render(" [ ] prev/next · v head-to-head · esc back"))
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
		lines = append(lines, row("", "", ""), " "+t.Rule.Render(strings.Repeat(t.Line, l.width-2)))
		lines = append(lines, row(east.Rank.String(), "RANK", west.Rank.String()))
		lines = append(lines, row(record(east), "RECORD", record(west)))
		lines = append(lines, "", " "+t.Dim.Render("  Loading the tale of the tape…"))
		return strings.Join(lines, "\n")
	}
	e, w := v.tale.East, v.tale.West
	if e.Shikona.Jp != "" || w.Shikona.Jp != "" {
		lines = append(lines, row(e.Shikona.Jp, "", w.Shikona.Jp))
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
func (v VS) Keys() []key.Binding { return []key.Binding{keyHistory, keyBack} }
