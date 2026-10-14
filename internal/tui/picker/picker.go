package picker

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui"
)

// banzukeLead is how long before day 1 the banzuke is published (SPEC
// Appendix C): the upcoming row shows the estimated date.
const banzukeLead = 13

// Picker lists every tournament from the upcoming one back to 1960 (SPEC
// §8.3). It is built from the calendar at once; yusho winners come from the
// snapshot and the cache in the background, without requests.
type Picker struct {
	open func(domain.BashoID) tui.Screen

	rows      []row
	shown     []int // indices of rows matching the filter
	cursor    int   // index into shown
	offset    int   // first visible line
	filter    textinput.Model
	filtering bool

	loading bool
	prov    *service.Provenance
	height  int // of the screen, for scrolling
}

type row struct {
	id     domain.BashoID
	status service.BashoStatus
	yusho  *service.Winner
}

var (
	keyUp     = key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up"))
	keyDown   = key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down"))
	keyPgUp   = key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up"))
	keyPgDown = key.NewBinding(key.WithKeys("pgdown", "space"), key.WithHelp("pgdown", "page down"))
	keyTop    = key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top"))
	keyBottom = key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom"))
	keyOpen   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	keyFilter = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter (year, name)"))
	keyClear  = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear filter"))
)

// New returns the picker. open builds the screen for a chosen tournament.
func New(ctx *tui.Context, open func(domain.BashoID) tui.Screen) Picker {
	today := domain.DateOf(ctx.Clock.Now())
	p := Picker{open: open, filter: textinput.New(), loading: true}
	p.filter.Prompt = "/"
	p.filter.Placeholder = "year or name"
	for id := domain.UpcomingTournament(today); id.Compare(domain.FirstBasho) >= 0; id = id.Prev() {
		p.rows = append(p.rows, row{id: id, status: service.CalendarStatus(id, today)})
	}
	p.applyFilter()
	return p
}

type listMsg struct {
	res service.Result[[]service.ListEntry]
	err error
}

// Init loads the winners: snapshot and cache only, so browsing never makes a
// request.
func (p Picker) Init(ctx *tui.Context) tea.Cmd {
	svc, base := ctx.Svc, ctx.Data()
	return func() tea.Msg {
		res, err := svc.List(service.CachedOnly(base), 0, 0)
		return listMsg{res, err}
	}
}

// Update implements tui.Screen.
func (p Picker) Update(ctx *tui.Context, msg tea.Msg) (tui.Screen, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.SpoilersMsg:
		for i := range p.rows {
			p.rows[i].yusho = nil
		}
		return p, p.Init(ctx)
	case listMsg:
		p.loading = false
		if msg.err == nil {
			p.prov = &msg.res.Prov
			byID := map[domain.BashoID]service.ListEntry{}
			for _, e := range msg.res.Value {
				byID[e.Basho] = e
			}
			for i, r := range p.rows {
				if e, ok := byID[r.id]; ok {
					p.rows[i].yusho = e.Yusho
					if e.Yusho != nil || e.InSnapshot {
						p.rows[i].status = e.Status
					}
				}
			}
		}
		return p, nil
	case tea.WindowSizeMsg:
		p.height = msg.Height - 1 // the status bar
	case tea.KeyPressMsg:
		var cmd tea.Cmd
		if p.filtering {
			p, cmd = p.filterKey(msg)
		} else {
			p, cmd = p.key(msg)
		}
		lines, cursor := p.lines()
		p.offset = scroll(p.offset, cursor, p.visible(), len(lines))
		return p, cmd
	}
	return p, nil
}

// line is one line of the list: a row, or a year separator.
type line struct {
	row  int // index into shown; -1 for a separator
	year int
}

// lines lays out the shown rows with a separator before each new year, and
// returns the cursor's line.
func (p Picker) lines() ([]line, int) {
	var lines []line
	cursor := 0
	for i, ri := range p.shown {
		y := p.rows[ri].id.Year
		if i > 0 && p.rows[p.shown[i-1]].id.Year != y {
			lines = append(lines, line{row: -1, year: y})
		}
		if i == p.cursor {
			cursor = len(lines)
		}
		lines = append(lines, line{row: i})
	}
	return lines, cursor
}

// visible is the number of list lines: the height less title, filter line
// and footer.
func (p Picker) visible() int { return max(p.height-3, 1) }

func (p Picker) filterKey(msg tea.KeyPressMsg) (Picker, tea.Cmd) {
	switch msg.String() {
	case "enter":
		p.filtering = false
		p.filter.Blur()
		return p, nil
	case "esc":
		p.filtering = false
		p.filter.Blur()
		p.filter.SetValue("")
		p.applyFilter()
		return p, nil
	}
	var cmd tea.Cmd
	p.filter, cmd = p.filter.Update(msg)
	p.applyFilter()
	return p, cmd
}

func (p Picker) key(msg tea.KeyPressMsg) (Picker, tea.Cmd) {
	const page = 10
	switch {
	case key.Matches(msg, keyUp):
		p.move(-1)
	case key.Matches(msg, keyDown):
		p.move(1)
	case key.Matches(msg, keyPgUp):
		p.move(-page)
	case key.Matches(msg, keyPgDown):
		p.move(page)
	case key.Matches(msg, keyTop):
		p.cursor = 0
	case key.Matches(msg, keyBottom):
		p.cursor = max(len(p.shown)-1, 0)
	case key.Matches(msg, keyFilter):
		p.filtering = true
		return p, p.filter.Focus()
	case key.Matches(msg, keyClear) && p.filter.Value() != "":
		p.filter.SetValue("")
		p.applyFilter()
	case key.Matches(msg, keyOpen):
		if len(p.shown) == 0 {
			return p, nil
		}
		r := p.rows[p.shown[p.cursor]]
		if r.status == service.Cancelled {
			return p, tui.Message(fmt.Sprintf("%s %d was cancelled", r.id.Name(), r.id.Year))
		}
		return p, tui.Push(p.open(r.id))
	}
	return p, nil
}

func (p *Picker) move(n int) {
	p.cursor = min(max(p.cursor+n, 0), max(len(p.shown)-1, 0))
}

// applyFilter keeps the rows matching every word of the filter: a year
// prefix, a tournament name, or the month number.
func (p *Picker) applyFilter() {
	words := strings.Fields(strings.ToLower(p.filter.Value()))
	p.shown = p.shown[:0]
	for i, r := range p.rows {
		if matches(r.id, words) {
			p.shown = append(p.shown, i)
		}
	}
	p.cursor = min(p.cursor, max(len(p.shown)-1, 0))
}

func matches(id domain.BashoID, words []string) bool {
	year, name, month := fmt.Sprint(id.Year), strings.ToLower(id.Name()), fmt.Sprintf("%02d", int(id.Month))
	for _, w := range words {
		if !strings.HasPrefix(year, w) && !strings.HasPrefix(name, w) && w != month {
			return false
		}
	}
	return true
}

// CapturesKeys implements tui.KeyCapturer: typing into the filter.
func (p Picker) CapturesKeys() bool { return p.filtering }

// Selected returns the tournament under the cursor.
func (p Picker) Selected() (domain.BashoID, bool) {
	if len(p.shown) == 0 {
		return domain.BashoID{}, false
	}
	return p.rows[p.shown[p.cursor]].id, true
}

// View implements tui.Screen.
func (p Picker) View(ctx *tui.Context, width, height int) string {
	t := ctx.Theme
	var b strings.Builder
	title := t.Title.Render("BASHO") + t.Dim.Render(" "+t.Line+" select tournament")
	hint := t.Dim.Render("[/] filter")
	b.WriteString(pad(title, hint, width) + "\n")
	if p.filtering || p.filter.Value() != "" {
		b.WriteString(" " + p.filter.View() + "\n")
	} else {
		b.WriteString("\n")
	}

	lines, _ := p.lines()
	visible := max(height-3, 1) // title, filter line, footer
	offset := min(p.offset, max(len(lines)-visible, 0))
	for _, l := range lines[offset:min(offset+visible, len(lines))] {
		if l.row < 0 {
			b.WriteString(t.Rule.Render(fmt.Sprintf("   %s%s %d %s%s", t.Line, t.Line, l.year, t.Line, t.Line)) + "\n")
			continue
		}
		b.WriteString(p.rowView(ctx, l.row) + "\n")
	}
	if len(p.shown) == 0 {
		b.WriteString(t.Dim.Render("   no tournament matches") + "\n")
	}
	for range visible - min(visible, max(len(lines), 1)) {
		b.WriteString("\n")
	}
	b.WriteString(t.Dim.Render(" enter open · / filter (year, name) · g/G top/bottom · q quit"))
	return b.String()
}

func (p Picker) rowView(ctx *tui.Context, i int) string {
	t := ctx.Theme
	r := p.rows[p.shown[i]]
	var note string
	switch r.status {
	case service.Cancelled:
		note = t.Dim.Render("cancelled")
	case service.Upcoming:
		note = t.Dim.Render("upcoming · banzuke " + r.id.EstimatedStart().AddDays(-banzukeLead).String())
	case service.InProgressBasho:
		note = t.Accent.Render("in progress")
	case service.Finished:
		if ctx.Hide && r.yusho == nil {
			note = t.Dim.Render("Yusho hidden (spoiler-free)")
		}
		fallthrough
	default:
		if y := r.yusho; y != nil {
			note = "Yusho: " + t.Gold.Render(y.Shikona.Romaji())
			if y.Record != nil {
				note += " (" + y.Record.String() + ")"
			}
		}
	}
	text := fmt.Sprintf("%d  %-7s %02d  %s", r.id.Year, r.id.Name(), int(r.id.Month), note)
	if i == p.cursor {
		return t.Selected.Render(" "+t.Cursor+" ") + t.Bold.Render(text)
	}
	return "   " + text
}

// scroll keeps the cursor line inside the window.
func scroll(offset, cursor, visible, total int) int {
	switch {
	case cursor < offset:
		offset = cursor
	case cursor >= offset+visible:
		offset = cursor - visible + 1
	}
	return min(max(offset, 0), max(total-visible, 0))
}

func pad(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 1
	return " " + left + strings.Repeat(" ", max(gap, 1)) + right
}

// Status implements tui.Screen.
func (p Picker) Status(*tui.Context) tui.Status {
	return tui.Status{Left: fmt.Sprintf("Basho picker · %d tournaments", len(p.shown)), Prov: p.prov, Loading: p.loading}
}

// Keys implements tui.Screen.
func (p Picker) Keys() []key.Binding {
	return []key.Binding{keyUp, keyDown, keyPgUp, keyPgDown, keyTop, keyBottom, keyOpen, keyFilter, keyClear}
}
