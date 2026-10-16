package wrestler

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/components"
)

// headerPortrait is the career page's portrait size: the larger one, as
// the page is about one wrestler.
const headerPortrait = 32

// View is one wrestler's career page.
type View struct {
	id      domain.RikishiID
	through domain.BashoID

	data    *service.Career
	prov    *service.Provenance
	err     error
	loading bool
	req     int

	cursor  int    // the table row, newest first
	offset  int    // the first table row shown
	picture string // the rendered portrait, "" for none
	height  int
}

var (
	keyUp    = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "newer tournament"))
	keyDown  = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "older tournament"))
	keyTop   = key.NewBinding(key.WithKeys("g"), key.WithHelp("g/G", "newest/oldest"))
	keyEnd   = key.NewBinding(key.WithKeys("G"))
	keyEnter = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "that tournament's day"))
)

// New returns wrestler id's career page as seen from tournament through.
func New(id domain.RikishiID, through domain.BashoID) View {
	return View{id: id, through: through, loading: true}
}

type careerMsg struct {
	req int
	res service.Result[service.Career]
	err error
}

// Init implements tui.Screen.
func (v View) Init(ctx *tui.Context) tea.Cmd { return v.fetch(ctx) }

func (v View) fetch(ctx *tui.Context) tea.Cmd {
	svc, id, through, req, base := ctx.Svc, v.id, v.through, v.req, ctx.Data()
	return func() tea.Msg {
		res, err := svc.Wrestler(base, id, through)
		return careerMsg{req: req, res: res, err: err}
	}
}

// Update implements tui.Screen.
func (v View) Update(ctx *tui.Context, msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.height = msg.Height - 1
		v.clamp()
	case careerMsg:
		if msg.req != v.req {
			return v, nil
		}
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
			return v, nil
		}
		v.data, v.prov, v.err = &msg.res.Value, &msg.res.Prov, nil
		v.picture = v.render(ctx)
		v.clamp()
	case tui.SpoilersMsg:
		v.req++
		v.loading = true
		return v, v.fetch(ctx)
	case tea.KeyPressMsg:
		return v.key(ctx, msg)
	}
	return v, nil
}

func (v View) key(ctx *tui.Context, msg tea.KeyPressMsg) (tui.Screen, tea.Cmd) {
	if v.data == nil {
		return v, nil
	}
	n := len(v.data.Tournaments)
	switch {
	case key.Matches(msg, keyUp):
		v.cursor--
	case key.Matches(msg, keyDown):
		v.cursor++
	case key.Matches(msg, keyTop):
		v.cursor = 0
	case key.Matches(msg, keyEnd):
		v.cursor = n - 1
	case key.Matches(msg, keyEnter):
		if ctx.OpenDay == nil || n == 0 {
			return v, nil
		}
		b := v.row(v.cursor)
		return v, tui.Push(ctx.OpenDay(b.Basho, b.Rank.Title.Division(), max(b.LastDay, 1)))
	}
	v.clamp()
	return v, nil
}

// row returns table row i: the tournaments newest first.
func (v View) row(i int) service.CareerBasho {
	t := v.data.Tournaments
	return t[len(t)-1-i]
}

// clamp keeps the cursor in the table and in view.
func (v *View) clamp() {
	if v.data == nil {
		return
	}
	v.cursor = min(max(v.cursor, 0), max(len(v.data.Tournaments)-1, 0))
	rows := v.tableRows()
	if v.cursor < v.offset {
		v.offset = v.cursor
	}
	if v.cursor >= v.offset+rows {
		v.offset = v.cursor - rows + 1
	}
}

// tableRows is how many table rows fit: the height less the header (the
// portrait or its text), two rules, the trends, the table's head and the
// hints.
func (v View) tableRows() int {
	header := headerLines
	if v.picture != "" {
		header = max(header, lipgloss.Height(v.picture))
	}
	trends := 1 // RANK
	if v.data != nil && v.data.RatingShown {
		trends++
	}
	return max(v.height-header-trends-4, 3)
}

// headerLines is the header text's height at most: the name and up to 13
// facts (RATING, SIGNATURE and the spoiler-free banner come and go).
const headerLines = 14

// render draws the portrait once: never in ASCII, without colours or for a
// wrestler without a photo.
func (v View) render(ctx *tui.Context) string {
	t := ctx.Theme
	if !ctx.Cfg.UI.Portraits || t.ASCII || t.Mono || ctx.Svc == nil {
		return ""
	}
	img, _, ok := ctx.Svc.Portrait(v.id, headerPortrait)
	if !ok {
		return ""
	}
	return portrait.Render(img, false)
}

// View implements tui.Screen.
func (v View) View(ctx *tui.Context, width, height int) string {
	t := ctx.Theme
	switch {
	case v.err != nil:
		s := "\n " + t.Error.Render(tui.ErrorText(v.err))
		if h := tui.ErrorHint(v.err); h != "" {
			s += "\n " + t.Dim.Render(h)
		}
		return s
	case v.data == nil:
		return "\n " + t.Dim.Render("Loading the career…")
	}
	compact := tui.LayoutOf(width) == tui.Compact
	var b strings.Builder
	header := v.header(ctx, width)
	if v.picture != "" && !compact {
		header = lipgloss.JoinHorizontal(lipgloss.Top, " ", v.picture, "  ", v.header(ctx, width-headerPortrait-3))
	}
	b.WriteString(header + "\n")
	b.WriteString(" " + t.Rule.Render(strings.Repeat(t.Line, width-2)) + "\n")
	for _, line := range v.trends(ctx, width) {
		b.WriteString(line + "\n")
	}
	b.WriteString(" " + t.Rule.Render(strings.Repeat(t.Line, width-2)) + "\n")
	b.WriteString(v.table(ctx, width))
	b.WriteString("\n" + t.Dim.Render(" j/k tournament · g/G newest/oldest · enter day · esc back"))
	return b.String()
}

// header is the name over the facts, one labelled row each, as on the VS
// screen: they fill the rows beside the portrait and read down the column.
func (v View) header(ctx *tui.Context, width int) string {
	t, c := ctx.Theme, v.data
	name := t.Bold.Render(strings.ToUpper(t.Name(c.Shikona)))
	if alt := t.AltName(c.Shikona); alt != "" {
		name += "  " + alt
	}
	lines := []string{" " + name}
	for _, f := range v.facts(ctx) {
		lines = append(lines, tui.Truncate(" "+t.Header.Render(fmt.Sprintf("%-10s", f[0]))+" "+f[1], width))
	}
	return strings.Join(lines, "\n")
}

// facts are the header's rows: label and value.
func (v View) facts(ctx *tui.Context) [][2]string {
	t, c := ctx.Theme, v.data
	dash := t.Dim.Render("—")
	or := func(s string) string {
		if s == "" {
			return dash
		}
		return s
	}
	measure := func(x float64, unit string) string {
		if x == 0 {
			return dash
		}
		s := fmt.Sprintf("%.0f%s", x, unit)
		if c.Stats.Latest {
			s += t.Dim.Render("*")
		}
		return s
	}
	age := dash
	if c.Stats.Age > 0 {
		age = fmt.Sprint(c.Stats.Age)
	}
	var yusho []string
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		if n := c.Stats.Yusho[div]; n > 0 {
			yusho = append(yusho, t.Gold.Render(fmt.Sprint(n))+" "+div.String())
		}
	}
	var prizes []string
	for _, prize := range slices.Sorted(maps.Keys(c.Stats.Sansho)) {
		prizes = append(prizes, fmt.Sprintf("%s %d", prize, c.Stats.Sansho[prize]))
	}
	kinboshi := dash
	if c.Kinboshi.Count > 0 {
		kinboshi = t.Gold.Render(strings.Repeat(t.Kinboshi, min(c.Kinboshi.Count, 10))) + fmt.Sprintf(" %d", c.Kinboshi.Count)
	}
	rows := [][2]string{
		{"RANK", c.Rank.String()},
		{"HEYA", or(c.Profile.Heya)},
		{"FROM", or(c.Profile.Shusshin)},
		{"AGE", age},
		{"HEIGHT", measure(c.Stats.Height, "cm")},
		{"WEIGHT", measure(c.Stats.Weight, "kg")},
		{"YUSHO", or(strings.Join(yusho, " · "))},
		{"PRIZES", or(strings.Join(prizes, " · "))},
		{"KINBOSHI", kinboshi},
	}
	if c.RatingShown && len(c.Tournaments) > 0 {
		last, peak := c.Tournaments[len(c.Tournaments)-1], c.Tournaments[0]
		for _, b := range c.Tournaments {
			if b.Rating > peak.Rating {
				peak = b
			}
		}
		rows = append(rows, [2]string{"RATING", fmt.Sprintf("%.0f", last.Rating) + t.Dim.Render(fmt.Sprintf("  peak %.0f, %s %d", peak.Rating, peak.Basho.Name(), peak.Basho.Year))})
	}
	if p := c.Movesets[derive.Career]; p.Enough() {
		rows = append(rows,
			[2]string{"STYLE", components.StyleBar(t, p.WinShare, components.StyleCells, false) + "  " + p.Type.String()},
			[2]string{"SIGNATURE", fmt.Sprintf("%s %.0f%%", p.Signature.Kimarite, 100*p.SignatureShare)})
	} else {
		rows = append(rows, [2]string{"STYLE", t.Dim.Render("— not enough data —")})
	}
	if ctx.Hide && len(c.Tournaments) > 0 && c.Tournaments[len(c.Tournaments)-1].Partial && ctx.Spoilers != nil {
		rows = append(rows, [2]string{"", t.Accent.Render(fmt.Sprintf("as of %s (spoiler-free)", ctx.Spoilers.Horizon(c.Through)))})
	}
	return rows
}

// sparks are the sparklines' eight heights.
var sparks = []string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// sparkline draws values, lowest ▁ to highest █.
func sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}
	lo, hi := slices.Min(values), slices.Max(values)
	var b strings.Builder
	for _, x := range values {
		i := len(sparks) - 1
		if hi > lo {
			i = int(math.Round((x - lo) / (hi - lo) * float64(len(sparks)-1)))
		}
		b.WriteString(sparks[i])
	}
	return b.String()
}

// trends are the rank and rating over the career, the most recent that fit.
func (v View) trends(ctx *tui.Context, width int) []string {
	t, c := ctx.Theme, v.data
	// The sparklines get what the label and the longest summary leave.
	const summary = 40
	cells := max(width-2-9-summary, 10)
	shown := c.Tournaments[max(len(c.Tournaments)-cells, 0):]
	label := func(s string) string { return " " + t.Header.Render(fmt.Sprintf("%-8s", s)) + " " }
	ranks, ratings := make([]float64, len(shown)), make([]float64, len(shown))
	for i, b := range shown {
		ranks[i] = -float64(derive.RankLevel(b.Rank)) // higher rank, taller
		ratings[i] = b.Rating
	}
	first, last := shown[0], shown[len(shown)-1]
	span := fmt.Sprintf("  %s → %s", first.Rank.Short(), last.Rank.Short())
	if len(shown) < len(c.Tournaments) {
		span += fmt.Sprintf(" · last %d of %d", len(shown), len(c.Tournaments))
	}
	lines := []string{label("RANK") + t.Accent.Render(sparkline(ranks)) + t.Dim.Render(span)}
	if c.RatingShown {
		lines = append(lines, label("RATING")+t.Accent.Render(sparkline(ratings))+t.Dim.Render(fmt.Sprintf("  %.0f → %.0f", first.Rating, last.Rating)))
	}
	for i := range lines {
		lines[i] = tui.Truncate(lines[i], width)
	}
	return lines
}

// table lists the tournaments, newest first, from offset.
func (v View) table(ctx *tui.Context, width int) string {
	t, c := ctx.Theme, v.data
	head := t.Header.Render(fmt.Sprintf("   %-11s %-5s %-8s %-6s", "Basho", "Rank", "Record", "Rating"))
	lines := []string{tui.Truncate(head, width)}
	rows := v.tableRows()
	for i := v.offset; i < min(v.offset+rows, len(c.Tournaments)); i++ {
		b := v.row(i)
		record := b.Record.String()
		if b.Partial {
			record += "…"
		}
		rating := ""
		if c.RatingShown {
			rating = fmt.Sprintf("%.0f", b.Rating)
		}
		var marks []string
		if b.Yusho {
			marks = append(marks, t.Gold.Render("YUSHO"))
		}
		marks = append(marks, b.Prizes...)
		if b.Shikona.En != c.Shikona.En {
			marks = append(marks, t.Dim.Render("as "+t.Name(b.Shikona)))
		}
		cursor := "  "
		if i == v.cursor {
			cursor = " " + t.Selected.Render(t.Cursor)
		}
		basho := fmt.Sprintf("%-6s %d", b.Basho.Name(), b.Basho.Year)
		line := fmt.Sprintf("%s %-11s %-5s %s %-6s %s", cursor, basho, b.Rank.Short(),
			tui.PadRight(t.Record(record, b.Record.KachiKoshi(), b.Record.MakeKoshi()), 8), rating, strings.Join(marks, " "))
		lines = append(lines, tui.Truncate(line, width))
	}
	return strings.Join(lines, "\n")
}

// Tournament implements tui.Tournamenter: the one the career is seen from.
func (v View) Tournament() domain.BashoID { return v.through }

// Status implements tui.Screen.
func (v View) Status(ctx *tui.Context) tui.Status {
	left := "Career"
	if v.data != nil {
		left = fmt.Sprintf("%s · career through %s %d", ctx.Theme.Name(v.data.Shikona), v.through.Name(), v.through.Year)
	}
	return tui.Status{Left: left, Prov: v.prov, Loading: v.loading}
}

// Keys implements tui.Screen.
func (v View) Keys() []key.Binding { return []key.Binding{keyUp, keyDown, keyTop, keyEnter} }
