package day

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui"
)

// recapMsg answers loadRecap.
type recapMsg struct {
	key refreshKey
	res service.Result[service.Recap]
	err error
}

// loadRecap fetches the recap of the shown day while the overlay is open;
// the day is cached by then, so it costs no request.
func (d Day) loadRecap(ctx *tui.Context) tea.Cmd {
	k := refreshKey{d.id, d.div, d.day}
	if !d.recapOpen || d.day == 0 || d.recap != nil && d.recapFor == k {
		return nil
	}
	svc := ctx.Svc
	return func() tea.Msg {
		res, err := svc.Recap(service.CachedOnly(context.Background()), k.id, k.div, k.day)
		return recapMsg{k, res, err}
	}
}

// recapView is the r overlay: the "between rounds" summary of the day
// (SPEC §8.4).
func (d Day) recapView(ctx *tui.Context, width, height int) string {
	t := ctx.Theme
	k := refreshKey{d.id, d.div, d.day}
	boxWidth := min(width-4, 84)
	var body string
	switch {
	case d.recapErr != nil:
		body = t.Error.Render(tui.ErrorText(d.recapErr))
	case d.recap == nil || d.recapFor != k:
		body = t.Dim.Render("Loading the recap…")
	default:
		body = recapBody(ctx, *d.recap, boxWidth-4)
	}
	title := t.Title.Render(fmt.Sprintf("Day %d recap · %s", d.day, d.div))
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(t.Colors.Subtle).Padding(0, 1).Width(boxWidth).
		Render(title + "\n\n" + body + "\n\n" + t.Dim.Render("r/esc close · ←/→ day"))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// recapLabel is the width of the recap's labels.
const recapLabel = 12

func recapBody(ctx *tui.Context, r service.Recap, width int) string {
	t := ctx.Theme
	var lines []string
	add := func(label string, items []string, empty string, sep ...string) {
		packed := []string{t.Dim.Render(empty)}
		if len(items) > 0 {
			packed = pack(items, width-recapLabel, append(sep, ",")[0])
		}
		indent := strings.Repeat(" ", recapLabel)
		for i, l := range packed {
			if i == 0 {
				lines = append(lines, t.Header.Render(fmt.Sprintf("%-*s", recapLabel, label))+l)
			} else {
				lines = append(lines, indent+l)
			}
		}
	}
	add("Leaders", leaders(r.Leaders), "—")
	add("Kachi-koshi", standingNames(r.KachiKoshi, false), "nobody today")
	add("Make-koshi", standingNames(r.MakeKoshi, false), "nobody today")
	var kinboshi []string
	for _, b := range r.Kinboshi {
		kinboshi = append(kinboshi, t.Gold.Render(t.Kinboshi)+" "+over(b))
	}
	add("Kinboshi", kinboshi, "none")
	var upsets []string
	for _, b := range r.Upsets {
		upsets = append(upsets, over(b))
	}
	add("Upsets", upsets, "none")
	technique := []string{}
	if b, tech := r.Recap.Technique, r.Technique; b != nil && tech != nil {
		winner, _, _ := b.Result()
		name := string(tech.Kimarite)
		if tech.Kanji != "" {
			name += " " + tech.Kanji
		}
		if tech.English != "" {
			name += " (" + tech.English + ")"
		}
		technique = append(technique, t.Bold.Render(name)+" by "+winner.Shikona.En, times(r.TechniqueCount))
	}
	add("Technique", technique, "—", " ·")
	return strings.Join(lines, "\n")
}

// times says how rare the technique of the day is in earlier tournaments.
func times(n int) string {
	switch n {
	case 0:
		return "never seen before in the record"
	case 1:
		return "once before in the record"
	default:
		return fmt.Sprintf("%d times before in the record", n)
	}
}

func standingNames(rows []derive.Standing, withRecord bool) []string {
	names := make([]string, len(rows))
	for i, r := range rows {
		names[i] = fmt.Sprintf("%s (%s)", r.Shikona.En, r.Rank.Short())
		if withRecord {
			names[i] += " " + r.Record.String()
		}
	}
	return names
}

// maxLeaders is how many leaders the recap names; early in a tournament
// half the division may share the lead.
const maxLeaders = 6

func leaders(rows []derive.Standing) []string {
	if len(rows) <= maxLeaders {
		return standingNames(rows, true)
	}
	names := standingNames(rows[:maxLeaders], true)
	return append(names, fmt.Sprintf("and %d more", len(rows)-maxLeaders))
}

// pack fills lines of width with the items, each but the last followed by
// sep, breaking only between items; an item wider than a line is cut.
func pack(items []string, width int, sep string) []string {
	var lines []string
	cur := ""
	for i, item := range items {
		if i < len(items)-1 {
			item += sep
		}
		switch {
		case cur == "":
			cur = item
		case lipgloss.Width(cur)+1+lipgloss.Width(item) <= width:
			cur += " " + item
		default:
			lines = append(lines, tui.Truncate(cur, width))
			cur = item
		}
	}
	return append(lines, tui.Truncate(cur, width))
}

// over is "Winner (M9e) over Loser (Y1e) · technique".
func over(b domain.Bout) string {
	w, l, _ := b.Result()
	return fmt.Sprintf("%s (%s) over %s (%s) · %s", w.Shikona.En, w.Rank.Short(), l.Shikona.En, l.Rank.Short(), b.Kimarite)
}
