package help

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/internal/portrait"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui/components"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

// Page is one page of the help overlay, wrapped to the width it was made
// for.
type Page struct {
	Title string
	Body  string
}

// Pages are the overlay's pages at a width: the keys (the screen's, then the
// global ones), the legend, the rating, the data sources and the known
// limitations. model is the rating's calibration, nil until it has loaded;
// photos is the number of portraits shipped.
func Pages(t theme.Theme, keys [][]key.Binding, model *service.RatingModel, photos, width int) []Page {
	return []Page{
		{"Keys", keysPage(keys, width)},
		{"Legend", legend(t, width)},
		{"Rating", ratingPage(t, model, width)},
		{"Data", dataPage(t, photos, width)},
		{"Limitations", limitations(width)},
	}
}

func keysPage(keys [][]key.Binding, width int) string {
	h := help.New()
	h.ShowAll = true
	h.SetWidth(width)
	return h.FullHelpView(keys)
}

func legend(t theme.Theme, width int) string {
	g := t.Glyphs
	var style []string
	marks := components.StyleMarks(t)
	for _, c := range components.Categories {
		style = append(style, lipgloss.NewStyle().Foreground(t.Colors.Categories[c]).Render(marks[c])+" "+c.String())
	}
	return defs(t, width, [][2]string{
		{"Results", fmt.Sprintf("%s win  %s loss  %s fusen win  %s fusen loss  %s absent  %s pending  %s draw  %s hidden (spoiler-free)",
			g.Win, g.Loss, g.FusenWin, g.FusenLoss, g.Absent, g.Pending, g.Draw, g.Unknown)},
		{"Records", "KK kachi-koshi (8 wins), MK make-koshi (8 losses and absences), " + g.Kyujo + " kyujo (absent)"},
		{"Kinboshi", g.Kinboshi + " a maegashira beat a yokozuna"},
		{"Ranks", "Y yokozuna, O ozeki, S sekiwake, K komusubi, M maegashira, J juryo, Ms makushita; e/w east/west"},
		{"Day bar", fmt.Sprintf("%s complete  %s in progress  %s scheduled", g.Complete, g.InProgress, g.Scheduled)},
		{"STYLE", strings.Join(style, "  ") + ": how a wrestler wins"},
		{"Names", "≈ transliterated from kana: a wrestler sumo-api does not know yet"},
		{"Body", "height and weight are the latest known; * marks them on other tournaments"},
		{"Status", fmt.Sprintf("%s refreshing  %s a stale copy (the source failed)  %s spoiler-free", g.Refresh, g.Stale, g.Spoiler)},
		{"LIVE", fmt.Sprintf("%s polling  %s waiting for the window  %s day complete; NEW: decided since the last poll", g.Live, g.LiveWait, g.Complete)},
	})
}

func ratingPage(t theme.Theme, m *service.RatingModel, width int) string {
	g := t.Glyphs
	paras := []string{
		"RATING is basho's own Elo-style estimate of strength, built from every sekitori bout since 1960, one after another. " +
			"A win against a stronger opponent gains more than one against a weaker. Newcomers start at the average of " +
			"established wrestlers of their rank; a wrestler back from a long absence is pulled towards the average of the rank " +
			"they return to. " + g.Up + "/" + g.Down + " is the change since the tournament began.",
		"WIN CHANCE turns the gap between two ratings into a probability: 100 points apart is 64%, 200 is 76%.",
		"Ratings only use the bouts before the one shown: the VS screen shows what both wrestlers brought into the bout, " +
			"and in spoiler-free mode they stop at the horizon.",
	}
	switch {
	case m == nil || m.Bouts == 0: // not loaded yet, or calibrated without metrics
	case !m.Shown:
		paras = append(paras, "In calibration the rating did not predict better than the higher rank, so basho hides it.")
	default:
		paras = append(paras, fmt.Sprintf("Tested on %d bouts from %d to %s %d that played no part in choosing its settings, "+
			"the favourite won %.0f%% of them; the higher rank won %.0f%%. Bouts are close: it is an estimate, not betting advice.",
			m.Bouts, m.EvalFrom.Year, m.Through.Name(), m.Through.Year, 100*m.Accuracy, 100*m.RankAccuracy))
	}
	paras = append(paras, t.Dim.Render("ui.win_chance = false hides both rows; p shows a win-chance column in the day view."))
	return paragraphs(width, paras...)
}

func dataPage(t theme.Theme, photos, width int) string {
	rows := make([][2]string, len(service.Credits))
	for i, c := range service.Credits {
		rows[i] = [2]string{c.Source, c.About}
	}
	if photos > 0 {
		rows = append(rows, [2]string{"Portraits", fmt.Sprintf("%d photos from Wikimedia Commons, %s, each under its own licence "+
			"(mostly CC BY-SA). basho credits --portraits lists every photo's author, licence and source. "+
			"To hide them: ui.portraits = false in the config, or --no-portraits.", photos, portrait.Changes)})
	}
	return defs(t, width, rows) + "\n\n" + paragraphs(width,
		service.CreditsNote,
		"The status bar names the source of what is on screen and when it was fetched.")
}

func limitations(width int) string {
	return bullets(width,
		"Heya and hometown are today's values: stable closures and moves don't show in past tournaments.",
		"Height and weight are the latest known, not those of the tournament shown (marked *).",
		"Only sekitori bouts count (Makuuchi and Juryo, from 1960). Records, head-to-head, movesets and ratings "+
			"leave out the lower divisions, so a wrestler's history starts at their first sekitori bout.",
		"Ratings start from nothing in 1960 and from an estimate by rank for each newcomer, so early values are rough.",
		"Live results come from osada.us, which copies the Japan Sumo Association's pages, and show minutes "+
			"after the bout: basho checks every few minutes.",
		"A wrestler new to sumo-api is named from osada.us's kana (≈) until sumo-api lists them.",
		"sumo-api has no bouts for three days: Haru 1965 day 7, Natsu 1965 day 6 and Kyushu 1978 day 5.",
	)
}

// defs is a definition list: terms in a column, their text wrapped beside.
func defs(t theme.Theme, width int, rows [][2]string) string {
	col := 0
	for _, r := range rows {
		col = max(col, ansi.StringWidth(r[0]))
	}
	var lines []string
	for _, r := range rows {
		for i, l := range strings.Split(wrap(r[1], max(width-col-2, 20)), "\n") {
			term := ""
			if i == 0 {
				term = r[0]
			}
			lines = append(lines, t.Bold.Render(pad(term, col))+"  "+l)
		}
	}
	return strings.Join(lines, "\n")
}

func paragraphs(width int, paras ...string) string {
	for i, p := range paras {
		paras[i] = wrap(p, width)
	}
	return strings.Join(paras, "\n\n")
}

func bullets(width int, items ...string) string {
	var lines []string
	for _, it := range items {
		for i, l := range strings.Split(wrap(it, max(width-2, 20)), "\n") {
			mark := "  "
			if i == 0 {
				mark = "- "
			}
			lines = append(lines, mark+l)
		}
	}
	return strings.Join(lines, "\n")
}

func pad(s string, w int) string { return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) }

// wrap wraps at spaces only: ansi.Wordwrap also breaks after any hyphen
// (head-|to-head, sumo-|api), so hyphens go in non-breaking for the wrap.
func wrap(s string, width int) string {
	return strings.ReplaceAll(ansi.Wordwrap(strings.ReplaceAll(s, "-", "\u2011"), width, ""), "\u2011", "-")
}
