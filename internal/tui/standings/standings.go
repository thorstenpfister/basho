package standings

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
)

// Tab is one of the two views of the standings.
type Tab int

const (
	Race Tab = iota // the yusho race
	Grid            // the hoshitori grid
)

// View is the standings view of one division after a day (SPEC §8.6).
type View struct {
	id  domain.BashoID
	div domain.Division
	day int
	tab Tab

	data    *service.Standings
	prov    *service.Provenance
	err     error
	loading bool
	req     int

	expanded bool // the race's tail
	cursor   int  // the grid row, in banzuke order
	offset   int  // the first line shown
	popup    bool // the selected wrestler's results
	width    int
	height   int
}

var (
	keyTab      = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "race ⇄ grid"))
	keyPrev     = key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "after the previous day"))
	keyNext     = key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "after the next day"))
	keyUp       = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown     = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	keyEnter    = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "expand the tail · wrestler's results"))
	keyDivision = key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "Makuuchi ⇄ Juryo"))
	keyClose    = key.NewBinding(key.WithKeys("esc", "enter", "q", "b"), key.WithHelp("esc", "close"))
)

// New returns the standings of div after day.
func New(id domain.BashoID, div domain.Division, day int) View {
	return View{id: id, div: div, day: max(day, 1), loading: true}
}

type standingsMsg struct {
	req    int
	day    int
	cached bool
	live   bool // after a live poll: the cache is fresh, no revalidation
	res    service.Result[service.Standings]
	err    error
}

// Init implements tui.Screen.
func (v View) Init(ctx *tui.Context) tea.Cmd {
	return v.fetch(ctx, true)
}

// fetch loads the standings: the cache's copy first, like the day view.
func (v View) fetch(ctx *tui.Context, cached bool) tea.Cmd {
	svc, id, div, day, req := ctx.Svc, v.id, v.div, v.day, v.req
	return func() tea.Msg {
		c := context.Background()
		if cached {
			c = service.CachedOnly(c)
		}
		res, err := svc.Standings(c, id, div, day)
		return standingsMsg{req: req, day: day, cached: cached, res: res, err: err}
	}
}

// load moves to another day or division; the old table stays until the new
// one comes.
func (v View) load(ctx *tui.Context) (View, tea.Cmd) {
	v.req++
	v.loading = true
	return v, v.fetch(ctx, true)
}

// Update implements tui.Screen.
func (v View) Update(ctx *tui.Context, msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width, v.height = msg.Width, msg.Height-1
	case standingsMsg:
		if msg.req != v.req {
			return v, nil
		}
		return v.answer(ctx, msg)
	case tui.LiveMsg:
		// Today's results move the standings from today on; the poll put
		// them in the cache.
		if msg.Poll.Basho != v.id || v.day < msg.Poll.Day {
			return v, nil
		}
		v.req++
		fetch := v.fetch(ctx, true)
		return v, func() tea.Msg {
			m := fetch().(standingsMsg)
			m.live = true
			return m
		}
	case tea.KeyPressMsg:
		if v.popup {
			if key.Matches(msg, keyClose) {
				v.popup = false
			}
			return v, nil
		}
		return v.key(ctx, msg)
	}
	return v, nil
}

func (v View) answer(ctx *tui.Context, msg standingsMsg) (tui.Screen, tea.Cmd) {
	if msg.err != nil {
		if msg.cached && !msg.live {
			return v, v.fetch(ctx, false)
		}
		v.loading = false
		if v.data != nil { // keep the last table; the day moves back
			v.day = v.data.Day
			return v, tui.Message(fmt.Sprintf("No standings after day %d: %s", msg.day, tui.ErrorText(msg.err)))
		}
		v.err = msg.err
		return v, nil
	}
	v.data, v.prov, v.err = &msg.res.Value, &msg.res.Prov, nil
	v.cursor = min(v.cursor, max(len(v.data.Rows)-1, 0))
	v.clamp(ctx)
	if msg.cached && !msg.live && msg.res.Prov.Source != service.SnapshotSource {
		return v, v.fetch(ctx, false)
	}
	v.loading = false
	return v, nil
}

func (v View) key(ctx *tui.Context, msg tea.KeyPressMsg) (tui.Screen, tea.Cmd) {
	switch {
	case key.Matches(msg, keyTab):
		v.tab = 1 - v.tab
		v.offset = 0
		v.clamp(ctx)
	case key.Matches(msg, keyPrev, keyNext):
		dir := 1
		if key.Matches(msg, keyPrev) {
			dir = -1
		}
		n := v.day + dir
		if n < 1 || n > domain.BashoDays {
			return v, tui.Message(fmt.Sprintf("Days run from 1 to %d", domain.BashoDays))
		}
		v.day = n
		return v.load(ctx)
	case key.Matches(msg, keyDivision):
		v.div = domain.DivJuryo + domain.DivMakuuchi - v.div
		v.data, v.err, v.cursor, v.offset, v.expanded = nil, nil, 0, 0, false
		return v.load(ctx)
	case key.Matches(msg, keyUp):
		v.move(ctx, -1)
	case key.Matches(msg, keyDown):
		v.move(ctx, 1)
	case key.Matches(msg, keyEnter):
		switch {
		case v.data == nil:
		case v.tab == Race:
			v.expanded = !v.expanded
			v.clamp(ctx)
		case len(v.data.Rows) > 0:
			v.popup = true
		}
	}
	return v, nil
}

// move is j/k: the grid's cursor, or the race's scroll position.
func (v *View) move(ctx *tui.Context, n int) {
	if v.data == nil {
		return
	}
	if v.tab == Grid {
		v.cursor = min(max(v.cursor+n, 0), max(len(v.data.Rows)-1, 0))
		visible := v.visible()
		switch {
		case v.cursor < v.offset:
			v.offset = v.cursor
		case v.cursor >= v.offset+visible:
			v.offset = v.cursor - visible + 1
		}
	} else {
		v.offset += n
	}
	v.clamp(ctx)
}

// clamp keeps the scroll position inside the content.
func (v *View) clamp(ctx *tui.Context) {
	total := 0
	switch {
	case v.data == nil:
	case v.tab == Grid:
		total = len(v.data.Rows)
	default:
		total = len(v.raceLines(ctx, v.width))
	}
	v.offset = min(max(v.offset, 0), max(total-v.visible(), 0))
}

// chrome is the lines around the table: title, header or blank, rule, rule,
// footer, keys.
const chrome = 6

func (v View) visible() int { return max(v.height-chrome, 1) }

// CapturesKeys implements tui.KeyCapturer: esc and q close the popup instead
// of leaving the screen.
func (v View) CapturesKeys() bool { return v.popup }

// QuitGoesBack implements tui.QuitBacker: q returns to the day view.
func (v View) QuitGoesBack() bool { return true }

// View implements tui.Screen.
func (v View) View(ctx *tui.Context, width, height int) string {
	t := ctx.Theme
	var b strings.Builder
	other := "[tab] grid"
	if v.tab == Grid {
		other = "[tab] race"
	}
	title := t.Title.Render(fmt.Sprintf("Standings after Day %d", v.day)) + t.Dim.Render(fmt.Sprintf(" · %s %d · ", v.id.Name(), v.id.Year)) + t.Bold.Render(v.div.String())
	b.WriteString(tui.Spread(title, t.Dim.Render(other), width) + "\n")
	switch {
	case v.err != nil:
		b.WriteString("\n" + t.Error.Render(" "+tui.ErrorText(v.err)) + "\n")
		return b.String()
	case v.data == nil:
		b.WriteString("\n" + t.Dim.Render(fmt.Sprintf(" Loading the standings after day %d…", v.day)) + "\n")
		return b.String()
	}
	s := *v.data
	var lines []string
	header := ""
	if v.tab == Grid {
		header, lines = v.grid(ctx, width)
	} else {
		lines = v.raceLines(ctx, width)
	}
	b.WriteString(t.Header.Render(header) + "\n")
	rule := t.Rule.Render(" " + strings.Repeat(t.Line, max(width-2, 1)))
	b.WriteString(rule + "\n")
	visible := max(height-chrome, 1)
	offset := min(v.offset, max(len(lines)-visible, 0))
	shown := lines[offset:min(offset+visible, len(lines))]
	body := strings.Join(shown, "\n")
	if v.popup && v.tab == Grid && v.cursor < len(s.Rows) {
		body = lipgloss.Place(width, visible, lipgloss.Center, lipgloss.Center, v.popupView(ctx, s.ByBanzuke()[v.cursor]))
		shown = strings.Split(body, "\n")
	}
	b.WriteString(body + "\n")
	b.WriteString(strings.Repeat("\n", max(visible-len(shown), 0)))
	b.WriteString(rule + "\n")
	b.WriteString(tui.Spread(fmt.Sprintf("Kachi-koshi: %d · Make-koshi: %d · Kyujo: %d", s.KachiKoshi, s.MakeKoshi, s.Kyujo), yusho(ctx, s), width) + "\n")
	hints := " tab race/grid · ←/→ day · enter expand · d division · esc back"
	if v.tab == Grid {
		hints = " tab race/grid · ←/→ day · j/k wrestler · enter results · d division · esc back"
	}
	b.WriteString(t.Dim.Render(tui.Truncate(hints, width)))
	return b.String()
}

// yusho is the footer's verdict.
func yusho(ctx *tui.Context, s service.Standings) string {
	t := ctx.Theme
	switch {
	case s.Yusho != nil && s.Playoff:
		return t.Gold.Render("Yusho: " + s.Yusho.Shikona.Romaji() + " (playoff)")
	case s.Yusho != nil:
		return t.Gold.Render("Yusho: " + s.Yusho.Shikona.Romaji())
	case s.YushoDecided:
		return t.Gold.Render("✓ yusho " + s.Rows[0].Shikona.Romaji())
	case s.Playoff:
		return t.Accent.Render("Playoff")
	default:
		return t.Dim.Render("Yusho decided: no")
	}
}

// marker is a row's KK/MK, "new" and 休 marks (SPEC §6.5).
func marker(ctx *tui.Context, r derive.Standing) string {
	t := ctx.Theme
	var m string
	switch {
	case r.Record.KachiKoshi():
		m = t.Green.Render("KK")
	case r.Record.MakeKoshi():
		m = t.Red.Render("MK")
	}
	if m != "" && r.NewToday {
		m = t.Bold.Render(m + " new")
	}
	if r.Kyujo {
		m = strings.TrimSpace(m + " " + t.Dim.Render(t.Kyujo))
	}
	return m
}

func record(ctx *tui.Context, r derive.Record) string {
	return ctx.Theme.Record(r.String(), r.KachiKoshi(), r.MakeKoshi())
}

// pips is a row's results up to the cutoff, one glyph each.
func pips(ctx *tui.Context, r derive.Standing, days int) string {
	var b strings.Builder
	for i := range days {
		b.WriteString(ctx.Theme.Outcome(r.Results[i].Outcome))
	}
	return b.String()
}

// raceLines is tab A: one line per record (wrapped when it holds more
// wrestlers than fit), the leaders and the two bands behind; the rest
// collapsed into one line until expanded.
func (v View) raceLines(ctx *tui.Context, width int) []string {
	s := *v.data
	entryWidth := maxName + 1 + 4 + 1 + s.Today + 10 // name, rank, pips, "KK new 休"
	perLine := max((width-9)/(entryWidth+2), 1)
	var lines []string
	for i := 0; i < len(s.Rows); {
		if s.Rows[i].Band == derive.Rest && (i == 0 || s.Rows[i-1].Band != derive.Rest) {
			lines = append(lines, v.tailLine(ctx, s.Rows[i:]))
			if !v.expanded {
				break
			}
		}
		j := i
		for j < len(s.Rows) && sameScore(s.Rows[j].Record, s.Rows[i].Record) {
			j++
		}
		group := s.Rows[i:j]
		i = j
		for k := 0; k < len(group); k += perLine {
			label := ""
			if k == 0 {
				label = score(ctx, group[0].Record)
			}
			var entries []string
			for _, r := range group[k:min(k+perLine, len(group))] {
				entries = append(entries, v.entry(ctx, r, entryWidth))
			}
			lines = append(lines, " "+lipgloss.PlaceHorizontal(6, lipgloss.Right, label)+"  "+strings.Join(entries, "  "))
		}
	}
	return lines
}

// tailLine is the collapsed rest: "─ out of contention ─ 8-5 and below ▸".
func (v View) tailLine(ctx *tui.Context, tail []derive.Standing) string {
	t := ctx.Theme
	label := "out of contention"
	for _, r := range tail {
		if r.InContention {
			label = "rest"
			break
		}
	}
	toggle := "▸ (enter to expand)"
	if v.expanded {
		toggle = "▾ (enter to collapse)"
	}
	return t.Dim.Render(fmt.Sprintf(" %s %s %s %s and below · %d %s", t.Line, label, t.Line, tail[0].Record.String(), len(tail), toggle))
}

// entry is one wrestler of a race line; wrestlers out of contention are dim.
func (v View) entry(ctx *tui.Context, r derive.Standing, width int) string {
	t := ctx.Theme
	name := fmt.Sprintf("%-*s %-4s", maxName, tui.Truncate(r.Shikona.Romaji(), maxName), r.Rank.Short())
	if !r.InContention {
		name = t.Dim.Render(name)
	} else if r.Band == derive.Leaders {
		name = t.Bold.Render(name)
	}
	text := name + " " + pips(ctx, r, v.data.Today)
	if m := marker(ctx, r); m != "" {
		text += " " + m
	}
	return text + strings.Repeat(" ", max(width-lipgloss.Width(text), 0))
}

// sameScore groups the race as it is sorted: by wins, then losses and
// absences together.
func sameScore(a, b derive.Record) bool {
	return a.Wins == b.Wins && a.Losses+a.Absences == b.Losses+b.Absences
}

// score is a race line's label: wins and losses with absences counted as
// losses (each wrestler's absences show as – in the pips).
func score(ctx *tui.Context, r derive.Record) string {
	r.Losses, r.Absences = r.Losses+r.Absences, 0
	return record(ctx, r)
}

const (
	maxName        = 14
	maxCompactName = 10
)

// grid is tab B: the header and one row per wrestler in banzuke order.
func (v View) grid(ctx *tui.Context, width int) (string, []string) {
	t := ctx.Theme
	s := *v.data
	compact := tui.LayoutOf(width) == tui.Compact
	cell, name := 3, maxName
	if compact {
		cell, name = 2, maxCompactName
	}
	var h strings.Builder
	fmt.Fprintf(&h, "  %-4s %-*s ", "", name, "")
	for d := 1; d <= domain.BashoDays; d++ {
		label := fmt.Sprint(d)
		if compact {
			label = fmt.Sprint(d % 10)
		}
		h.WriteString(lipgloss.PlaceHorizontal(cell, lipgloss.Center, label))
	}
	h.WriteString("  W-L-A")
	var lines []string
	for i, r := range s.ByBanzuke() {
		var row strings.Builder
		cursor := "  "
		rank, n := fmt.Sprintf("%-4s", r.Rank.Short()), fmt.Sprintf("%-*s", name, tui.Truncate(r.Shikona.Romaji(), name))
		if i == v.cursor {
			cursor = t.Selected.Render(t.Cursor) + " "
			rank, n = t.Selected.Render(rank), t.Selected.Render(n)
		}
		row.WriteString(cursor + rank + " " + n + " ")
		for d := 1; d <= domain.BashoDays; d++ {
			g := t.Outcome(r.Results[d-1].Outcome)
			switch {
			case d > s.Day:
				g = t.Dim.Render(t.Pending)
			case d == s.Day && !compact:
				g = t.Selected.Render("[" + g + "]")
			case d == s.Day:
				g = t.Selected.Render(g)
			}
			row.WriteString(lipgloss.PlaceHorizontal(cell, lipgloss.Center, g))
		}
		row.WriteString("  " + lipgloss.PlaceHorizontal(7, lipgloss.Right, record(ctx, r.Record)))
		if m := marker(ctx, r); m != "" {
			row.WriteString("  " + m)
		}
		lines = append(lines, row.String())
	}
	return h.String(), lines
}

// popupView is enter on a grid row: the wrestler's results, opponent and
// technique for each day.
func (v View) popupView(ctx *tui.Context, r derive.Standing) string {
	t := ctx.Theme
	s := *v.data
	ranks := map[domain.RikishiID]domain.Rank{}
	for _, row := range s.Rows {
		ranks[row.RikishiID] = row.Rank
	}
	lines := []string{t.Title.Render(fmt.Sprintf("%s · %s · %s", r.Shikona.Romaji(), r.Rank.Short(), r.Record.String())), ""}
	for d := 1; d <= domain.BashoDays; d++ {
		res := r.Results[d-1]
		var what string
		switch {
		case d > s.Day:
			what = t.Dim.Render("—")
		case res.Outcome == domain.Absent:
			what = t.Dim.Render("absent")
		case res.OpponentID == 0:
			what = t.Dim.Render("no bout")
		default:
			opp := s.Opponents[res.OpponentID].Romaji()
			if opp == "" {
				opp = fmt.Sprintf("#%d", res.OpponentID)
			}
			if rk, ok := ranks[res.OpponentID]; ok {
				opp += " (" + rk.Short() + ")"
			}
			what = fmt.Sprintf("%-22s %s", tui.Truncate(opp, 22), res.Kimarite)
		}
		glyph := t.Outcome(res.Outcome)
		if d > s.Day {
			glyph = t.Dim.Render(t.Pending)
		}
		lines = append(lines, fmt.Sprintf("Day %2d  %s  %s", d, glyph, what))
	}
	lines = append(lines, "", t.Dim.Render("esc close"))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Colors.Subtle).Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

// Status implements tui.Screen.
func (v View) Status(*tui.Context) tui.Status {
	return tui.Status{Left: fmt.Sprintf("%s %d · %s · Standings after Day %d", v.id.Name(), v.id.Year, v.div, v.day), Prov: v.prov, Loading: v.loading}
}

// Keys implements tui.Screen.
func (v View) Keys() []key.Binding {
	return []key.Binding{keyTab, keyPrev, keyNext, keyUp, keyDown, keyEnter, keyDivision}
}
