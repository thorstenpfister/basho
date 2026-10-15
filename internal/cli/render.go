package cli

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	ltable "charm.land/lipgloss/v2/table"
	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
)

// Styles of the text output. Lip Gloss renders full colour; the writer
// downsamples or strips it (SPEC §11).
var (
	bold   = lipgloss.NewStyle().Bold(true)
	dim    = lipgloss.NewStyle().Faint(true)
	title  = lipgloss.NewStyle().Bold(true)
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fa34d"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d1495b"))
	gold   = lipgloss.NewStyle().Foreground(lipgloss.Color("#d4a017"))
	header = lipgloss.NewStyle().Bold(true)
)

// table lays out rows in columns without borders; right lists the columns
// aligned right.
func table(headers []string, rows [][]string, right ...int) string {
	t := ltable.New()
	if slices.ContainsFunc(headers, func(h string) bool { return h != "" }) {
		t = t.Headers(headers...)
	}
	t = t.Rows(rows...).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderHeader(false).BorderRow(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			s := lipgloss.NewStyle()
			if col < len(headers)-1 {
				s = s.PaddingRight(2)
			}
			for _, r := range right {
				if r == col {
					s = s.Align(lipgloss.Right)
				}
			}
			if row == ltable.HeaderRow {
				s = s.Inherit(header)
			}
			return s
		})
	return trimLines(t.Render())
}

// trailingPad is the end of a line made of padding and style sequences,
// which Lip Gloss interleaves.
var trailingPad = regexp.MustCompile(`(?: |\x1b\[[0-9;]*m)+$`)

// trimLines removes the padding at the ends of lines, keeping the style
// sequences.
func trimLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = trailingPad.ReplaceAllStringFunc(l, func(tail string) string { return strings.ReplaceAll(tail, " ", "") })
	}
	return strings.Join(lines, "\n")
}

// provenance is the footer of data output.
func provenance(p service.Provenance) string {
	s := "source: " + p.Source
	if !p.FetchedAt.IsZero() {
		s += " (" + p.FetchedAt.Format("2006-01-02") + ")"
	}
	if p.Forced {
		s += " · forced"
	}
	if p.Stale {
		s += " · stale: the source could not be reached"
	}
	return dim.Render(s)
}

func outcome(g glyphs, o domain.Outcome) string {
	switch o {
	case domain.Win:
		return g.win
	case domain.Loss:
		return g.loss
	case domain.FusenWin:
		return g.fusenWin
	case domain.FusenLoss:
		return g.fusenLoss
	case domain.Absent:
		return g.absent
	case domain.Draw:
		return "△"
	case domain.Unknown:
		return "?"
	default:
		return g.pending
	}
}

// sideGlyph is a bout side's result mark.
func sideGlyph(g glyphs, won, decided, fusen bool) string {
	switch {
	case !decided:
		return g.pending
	case won && fusen:
		return g.fusenWin
	case won:
		return g.win
	case fusen:
		return g.fusenLoss
	default:
		return g.loss
	}
}

func change(g glyphs, d float64) string {
	switch n := math.Round(d); {
	case n > 0:
		return fmt.Sprintf("%s%.0f", g.up, n)
	case n < 0:
		return fmt.Sprintf("%s%.0f", g.down, -n)
	default:
		return g.none
	}
}

func stars(g glyphs, k derive.Kinboshi) string {
	switch {
	case k.Count == 0:
		return g.none
	case k.Count >= 5:
		return gold.Render(fmt.Sprintf("%s×%d", g.star, k.Count))
	default:
		return gold.Render(strings.Repeat(g.star, k.Count))
	}
}

func dayHeading(name string, id domain.BashoID, div domain.Division, day int, date domain.Date, venue string) string {
	return title.Render(fmt.Sprintf("%s %d · %s · Day %d · %s %s · %s", name, id.Year, div, day, date.Weekday().String()[:3], date, venue))
}

// renderDay is `basho day`.
func renderDay(g glyphs, d service.Day, p service.Provenance) string {
	var b strings.Builder
	b.WriteString(dayHeading(d.Name, d.Basho, d.Division, d.Day, d.Date, d.Venue) + "\n\n")
	rows := make([][]string, 0, len(d.Bouts)+len(d.Playoffs))
	add := func(r service.BoutRow) {
		decided := r.Winner != 0
		east, west := g.name(r.East.Shikona), g.name(r.West.Shikona)
		switch r.Winner {
		case domain.East:
			east, west = bold.Render(east), dim.Render(west)
		case domain.West:
			east, west = dim.Render(east), bold.Render(west)
		}
		tag := ""
		if r.Musubi {
			tag = g.musubi
		}
		if r.Kinboshi {
			tag = strings.TrimSpace(tag + " " + gold.Render(g.star+" kinboshi"))
		}
		technique := string(r.Kimarite.Kimarite)
		eg, wg := sideGlyph(g, r.Winner == domain.East, decided, r.Fusen), sideGlyph(g, r.Winner == domain.West, decided, r.Fusen)
		switch {
		case r.Hidden:
			technique, eg, wg = "?", "?", "?"
		case !decided:
			technique = g.none
		}
		rows = append(rows, []string{
			fmt.Sprint(r.MatchNo), r.East.Rank.Short(), east, record(r.East),
			eg, technique, wg,
			record(r.West), west, r.West.Rank.Short(), tag,
		})
	}
	for _, r := range d.Bouts {
		add(r)
	}
	b.WriteString(table([]string{"#", "", "East", "", "", "", "", "", "West", "", ""}, rows, 0, 2, 3))
	b.WriteString("\n")
	if len(d.Playoffs) > 0 {
		rows = rows[:0]
		for _, r := range d.Playoffs {
			add(r)
		}
		b.WriteString("\n── Kettei-sen ──\n" + table([]string{"#", "", "East", "", "", "", "", "", "West", "", ""}, rows, 0, 2, 3) + "\n")
	}
	if len(d.Kyujo) > 0 {
		names := make([]string, len(d.Kyujo))
		for i, w := range d.Kyujo {
			names[i] = fmt.Sprintf("%s (%s)", g.name(w.Shikona), w.Rank.Short())
		}
		b.WriteString("\nKyujo: " + strings.Join(names, ", ") + "\n")
	}
	if d.Status == service.NotPublished {
		b.WriteString("\nNo bouts published for this day yet.\n")
	}
	b.WriteString("\n" + provenance(p) + "\n")
	return b.String()
}

// record is a bout side's record before the bout; a visitor from a lower
// division has none here.
func record(s service.BoutSide) string {
	if s.Visitor {
		return "—"
	}
	return s.Before.String()
}

func taleRecord(t service.TaleSide) string {
	if t.Visitor {
		return "—"
	}
	return t.Record.String()
}

// renderTale is `basho bout`: the tale of the tape with both technique
// profiles (SPEC §6.7, §8.5).
func renderTale(g glyphs, t service.Tale, p service.Provenance) string {
	var b strings.Builder
	head := fmt.Sprintf("%s %d · Day %d · Bout %d/%d", t.Name, t.Basho.Year, t.Day, t.MatchNo, t.Of)
	switch {
	case t.Playoff:
		head += " · KETTEI-SEN"
	case t.Musubi:
		head += " · MUSUBI-NO-ICHIBAN"
	}
	b.WriteString(title.Render(head) + "\n\n")
	e, w := t.East, t.West
	rows := [][]string{{bold.Render(strings.ToUpper(g.name(e.Shikona))), "VS", bold.Render(strings.ToUpper(g.name(w.Shikona)))}}
	if ea, wa := g.alt(e.Shikona), g.alt(w.Shikona); ea != "" || wa != "" {
		rows = append(rows, []string{ea, "", wa})
	}
	rows = append(rows, [][]string{
		{dim.Render(fighter(e.Moveset())), "", dim.Render(fighter(w.Moveset()))},
		{e.Rank.String(), "RANK", w.Rank.String()},
		{orUnknown(e.Heya, e.Profile), "HEYA", orUnknown(w.Heya, w.Profile)},
		{orUnknown(e.Shusshin, e.Profile), "FROM", orUnknown(w.Shusshin, w.Profile)},
		{age(e), "AGE", age(w)},
		{taleRecord(e), "RECORD", taleRecord(w)},
		{last5(g, e.Last5), "LAST 5", last5(g, w.Last5)},
		{measure(e.Career.Height, "cm", e.Career.Latest), "HEIGHT", measure(w.Career.Height, "cm", w.Career.Latest)},
		{measure(e.Career.Weight, "kg", e.Career.Latest), "WEIGHT", measure(w.Career.Weight, "kg", w.Career.Latest)},
		{stars(g, e.Kinboshi), "KINBOSHI", stars(g, w.Kinboshi)},
		{fmt.Sprint(e.Career.Yusho[domain.DivMakuuchi]), "YUSHO", fmt.Sprint(w.Career.Yusho[domain.DivMakuuchi])},
		{signature(e.Moveset()), "SIGNATURE", signature(w.Moveset())},
	}...)
	if e.Rating != nil && w.Rating != nil && t.WinChance != nil {
		rows = append(rows,
			[]string{fmt.Sprintf("%.0f %s", e.Rating.Value, change(g, e.Rating.Change)), "RATING", fmt.Sprintf("%.0f %s", w.Rating.Value, change(g, w.Rating.Change))},
			[]string{fmt.Sprintf("%.0f%%", 100**t.WinChance), "WIN CHANCE", fmt.Sprintf("%.0f%%", 100*(1-*t.WinChance))},
		)
	}
	h := t.HeadToHead
	rows = append(rows, []string{fmt.Sprint(h.A.Wins), "H2H", fmt.Sprint(h.B.Wins)},
		[]string{counts(h.A.Top), "", counts(h.B.Top)})
	if e.Career.Now != "" || w.Career.Now != "" {
		rows = append(rows, []string{now(e.Career.Now), "", now(w.Career.Now)})
	}
	rows = slices.DeleteFunc(rows, func(r []string) bool { return r[0] == "" && r[1] == "" && r[2] == "" })
	b.WriteString(tape(rows) + "\n")

	if len(h.Meetings) > 0 {
		b.WriteString("\nLast meetings: ")
		var ms []string
		for _, m := range h.Meetings[:min(3, len(h.Meetings))] {
			who := g.name(e.Shikona)
			if m.Winner == w.RikishiID {
				who = g.name(w.Shikona)
			}
			ms = append(ms, fmt.Sprintf("%s day %d %s (%s)", m.Basho, m.Day, who, m.Kimarite))
		}
		b.WriteString(strings.Join(ms, " · ") + "\n")
	}

	b.WriteString("\n")
	if r := t.Result; t.Hidden {
		b.WriteString("Result hidden (spoiler-free)\n")
	} else if r == nil {
		fmt.Fprintf(&b, "Scheduled — Day %d\n", t.Day)
	} else {
		winner := g.name(e.Shikona)
		if r.Winner == domain.West {
			winner = g.name(w.Shikona)
		}
		line := strings.TrimRight(fmt.Sprintf("%s wins by %s  %s", bold.Render(strings.ToUpper(winner)), strings.ToUpper(string(r.Technique.Kimarite)), g.kanji(r.Technique.Kanji)), " ")
		if r.Technique.English != "" {
			line += " (" + r.Technique.English + ")"
		}
		b.WriteString(line + "\n")
		fmt.Fprintf(&b, "%s %s · %s %s\n", g.name(e.Shikona), r.East, g.name(w.Shikona), r.West)
		if r.Kinboshi {
			b.WriteString(gold.Render(g.star+" KINBOSHI") + "\n")
		}
		if r.Fusen {
			b.WriteString("FUSEN (default)\n")
		}
	}

	b.WriteString("\n" + title.Render("MOVESET · "+e.Moveset().Window.String()) + "\n")
	b.WriteString(moveset(g, e) + "\n" + moveset(g, w))
	if m := t.Matchup(); m != nil {
		att, def := g.name(e.Shikona), g.name(w.Shikona)
		if !m.AttackerIsA {
			att, def = def, att
		}
		cat := strings.ToUpper(m.Category.String())
		fmt.Fprintf(&b, "Matchup: %s wins %.0f%% by %s; %s loses %.0f%% to %s\n", att, 100*m.WinShare, cat, def, 100*m.LossShare, cat)
	}
	b.WriteString("\n" + provenance(p) + "\n")
	return b.String()
}

// tape lays out the mirrored columns: values face the labels in the middle.
func tape(rows [][]string) string {
	t := ltable.New().Rows(rows...).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderColumn(false).BorderHeader(false).BorderRow(false).
		StyleFunc(func(_, col int) lipgloss.Style {
			switch col {
			case 0:
				return lipgloss.NewStyle().Align(lipgloss.Right)
			case 1:
				return lipgloss.NewStyle().Align(lipgloss.Center).Padding(0, 3).Inherit(dim)
			default:
				return lipgloss.NewStyle()
			}
		})
	return trimLines(t.Render())
}

func fighter(p derive.Profile) string {
	if t := p.Type.String(); t != "" {
		return t
	}
	return "—"
}

func orUnknown(s string, profile bool) string {
	if !profile {
		return "profile unavailable"
	}
	if s == "" {
		return "—"
	}
	return s
}

func age(t service.TaleSide) string {
	if t.Career.Age == 0 {
		return "—"
	}
	return fmt.Sprint(t.Career.Age)
}

func measure(v float64, unit string, latest bool) string {
	if v == 0 {
		return "—"
	}
	s := fmt.Sprintf("%.0f%s", v, unit)
	if latest {
		s += dim.Render(" (latest)")
	}
	return s
}

func last5(g glyphs, l derive.LastN) string {
	var b strings.Builder
	for i, r := range l.Results {
		if i > 0 && l.Results[i-1].Basho != r.Basho {
			b.WriteString(g.divider)
		}
		b.WriteString(sideGlyph(g, r.Won, true, r.Fusen))
	}
	if l.New {
		b.WriteString(strings.Repeat(g.pending, 5-len(l.Results)) + dim.Render(" new"))
	}
	return b.String()
}

func signature(p derive.Profile) string {
	if !p.Enough() {
		return "— not enough data —"
	}
	return fmt.Sprintf("%s %.0f%%", p.Signature.Kimarite, 100*p.SignatureShare)
}

func counts(top []derive.KimariteCount) string {
	if len(top) == 0 {
		return "—"
	}
	parts := make([]string, len(top))
	for i, k := range top {
		parts[i] = fmt.Sprintf("%s ×%d", k.Kimarite, k.Count)
	}
	return strings.Join(parts, " · ")
}

func now(name string) string {
	if name == "" {
		return ""
	}
	return dim.Render("now: " + name)
}

var categories = []domain.KimariteCategory{domain.Push, domain.Belt, domain.Throw, domain.Pull, domain.Other}

func moveset(g glyphs, t service.TaleSide) string {
	p := t.Moveset()
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %d W · %d L  %s\n", bold.Render(g.name(t.Shikona)), p.Wins, p.Losses, dim.Render(fighter(p)))
	if p.Wins+p.Losses == 0 {
		return b.String()
	}
	var shares []string
	for _, c := range categories {
		shares = append(shares, fmt.Sprintf("%s %.0f%%", strings.ToUpper(c.String()), 100*p.WinShare[c]))
	}
	b.WriteString("    " + strings.Join(shares, " · ") + "\n")
	b.WriteString("    Wins by   " + counts(p.TopWins) + "\n")
	b.WriteString("    Loses to  " + counts(p.TopLosses) + "\n")
	return b.String()
}

// renderStandings is `basho standings`: the yusho race, or the hoshitori
// grid with --grid.
func renderStandings(g glyphs, s service.Standings, grid bool, p service.Provenance) string {
	var b strings.Builder
	b.WriteString(title.Render(fmt.Sprintf("Standings after Day %d · %s %d · %s", s.Day, s.Name, s.Basho.Year, s.Division)) + "\n\n")
	marker := func(r derive.Standing) string {
		var m string
		switch {
		case r.Record.KachiKoshi():
			m = green.Render("KK")
		case r.Record.MakeKoshi():
			m = red.Render("MK")
		}
		if m != "" && r.NewToday {
			m = bold.Render(m + " new")
		}
		if r.Kyujo {
			m = strings.TrimSpace(m + " " + g.kyujo)
		}
		return m
	}
	cells := func(r derive.Standing) string {
		var c []string
		for i, res := range r.Results {
			cell := outcome(g, res.Outcome)
			if i+1 > s.Day {
				cell = dim.Render(g.pending)
			}
			c = append(c, cell)
		}
		return strings.Join(c, " ")
	}
	if grid {
		var rows [][]string
		for _, r := range s.ByBanzuke() {
			rows = append(rows, []string{r.Rank.Short(), g.name(r.Shikona), cells(r), r.Record.String(), marker(r)})
		}
		b.WriteString(table([]string{"", "", days(), "W-L-A", ""}, rows, 3) + "\n")
	} else {
		bands := []string{"Leaders", "−1", "−2", "Rest"}
		var rows [][]string
		band := derive.Band(-1)
		for _, r := range s.Rows {
			label := ""
			if r.Band != band {
				band, label = r.Band, bands[r.Band]
			}
			rows = append(rows, []string{label, r.Record.String(), r.Rank.Short(), g.name(r.Shikona), cells(r), marker(r)})
		}
		b.WriteString(table([]string{"", "", "", "", days(), ""}, rows, 1) + "\n")
	}
	yusho := "no"
	switch {
	case s.Yusho != nil:
		yusho = g.name(s.Yusho.Shikona)
		if s.Playoff {
			yusho += " (playoff)"
		}
	case s.YushoDecided:
		yusho = "yes, " + g.name(s.Rows[0].Shikona)
	case s.Playoff:
		yusho = "playoff"
	}
	fmt.Fprintf(&b, "\nKachi-koshi: %d · Make-koshi: %d · Kyujo: %d · Yusho: %s\n", s.KachiKoshi, s.MakeKoshi, s.Kyujo, yusho)
	b.WriteString("\n" + provenance(p) + "\n")
	return b.String()
}

func days() string {
	var d []string
	for i := 1; i <= domain.BashoDays; i++ {
		d = append(d, fmt.Sprint(i%10))
	}
	return strings.Join(d, " ")
}

// renderList is `basho list`.
func renderList(g glyphs, es []service.ListEntry, p service.Provenance) string {
	rows := make([][]string, 0, len(es))
	for _, e := range es {
		yusho := ""
		if y := e.Yusho; y != nil {
			yusho = g.name(y.Shikona)
			if y.Record != nil {
				yusho += " (" + y.Record.String() + ")"
			}
		}
		rows = append(rows, []string{e.Basho.String(), fmt.Sprintf("%s %d", e.Name, e.Basho.Year), e.Venue, yusho, statusText(e.Status)})
	}
	return table([]string{"ID", "Tournament", "Venue", "Yusho", "Status"}, rows) + "\n\n" + provenance(p) + "\n"
}

func statusText(s service.BashoStatus) string { return strings.ReplaceAll(s.String(), "_", " ") }

// renderInfo is `basho info`.
func renderInfo(g glyphs, in service.Info, p service.Provenance) string {
	var b strings.Builder
	b.WriteString(title.Render(fmt.Sprintf("%s %d (%s)", in.Name, in.Basho.ID.Year, in.Basho.ID)) + "\n")
	fmt.Fprintf(&b, "Venue:   %s\nStatus:  %s\n", in.Venue, statusText(in.Status))
	if !in.Basho.Start.IsZero() {
		fmt.Fprintf(&b, "Dates:   %s – %s\n", in.Basho.Start, in.Basho.End)
	}
	if len(in.Yusho) > 0 {
		b.WriteString("\nYusho\n")
		var rows [][]string
		for _, y := range in.Yusho {
			rows = append(rows, []string{y.Division.String(), wrestlerName(g, y.Wrestler), recordText(y.Record)})
		}
		b.WriteString(table([]string{"", "", ""}, rows) + "\n")
	}
	if len(in.Prizes) > 0 {
		b.WriteString("\nSpecial prizes\n")
		var rows [][]string
		for _, pr := range in.Prizes {
			rows = append(rows, []string{pr.Prize, wrestlerName(g, pr.Wrestler), recordText(pr.Record)})
		}
		b.WriteString(table([]string{"", "", ""}, rows) + "\n")
	}
	b.WriteString("\n" + provenance(p) + "\n")
	return b.String()
}

func recordText(r *derive.Record) string {
	if r == nil {
		return ""
	}
	return r.String()
}

func wrestlerName(g glyphs, w service.Wrestler) string {
	s := g.name(w.Shikona)
	if s == "" {
		s = fmt.Sprintf("#%d", w.RikishiID)
	}
	if r := w.Rank.Short(); r != "" {
		s += " (" + r + ")"
	}
	return s
}

// renderRatings is `basho ratings`.
func renderRatings(g glyphs, t service.RatingTable, p service.Provenance) string {
	var b strings.Builder
	b.WriteString(title.Render(fmt.Sprintf("Ratings after %s %d (%s)", t.Basho.Name(), t.Basho.Year, t.Basho)) + "\n")
	if !t.Shown {
		b.WriteString(dim.Render("The rating did not beat the rank baseline in calibration; shown here only.") + "\n")
	}
	b.WriteString("\n")
	rows := make([][]string, 0, len(t.Rows))
	for i, r := range t.Rows {
		rows = append(rows, []string{fmt.Sprint(i + 1), r.Rank.Short(), g.name(r.Shikona), fmt.Sprintf("%.0f", r.Value), change(g, r.Change)})
	}
	b.WriteString(table([]string{"#", "Rank", "Wrestler", "Rating", "Change"}, rows, 0, 3, 4) + "\n")
	b.WriteString("\n" + dim.Render("An estimate from past results, not betting advice.") + "\n" + provenance(p) + "\n")
	return b.String()
}

// credits is `basho credits` (SPEC §4, §14.4): each source with its notes
// in a column beside it.
func credits() string {
	var b strings.Builder
	b.WriteString("basho uses data from:\n\n")
	for _, c := range service.Credits {
		for i, line := range strings.Split(wrap(c.About, 60), "\n") {
			name := ""
			if i == 0 {
				name = c.Source
			}
			fmt.Fprintf(&b, "  %-12s  %s\n", name, line)
		}
	}
	b.WriteString("\n" + wrap(service.CreditsNote, 74) + "\n")
	return b.String()
}

// wrap wraps at spaces only: ansi.Wordwrap also breaks after any hyphen
// (sumo-|api), so hyphens go in non-breaking for the wrap.
func wrap(s string, width int) string {
	return strings.ReplaceAll(ansi.Wordwrap(strings.ReplaceAll(s, "-", "\u2011"), width, ""), "\u2011", "-")
}
