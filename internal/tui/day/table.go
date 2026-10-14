package day

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

// cols are the column widths of a day's bout table, sized to its content;
// the compact layout (under 90 columns) caps names and techniques and drops
// the bout number.
type cols struct {
	name, rec, tech int
	compact         bool
	winChance       bool
}

const (
	maxName        = 14
	maxCompactName = 10
	maxCompactTech = 10
)

func columns(rows []service.BoutRow, compact, winChance bool) cols {
	c := cols{name: 4, rec: 3, tech: 6, compact: compact, winChance: winChance}
	for _, r := range rows {
		c.name = max(c.name, len(r.East.Shikona.Romaji()), len(r.West.Shikona.Romaji()))
		c.rec = max(c.rec, len(record(r.East)), len(record(r.West)))
		c.tech = max(c.tech, len(r.Kimarite.Kimarite))
	}
	c.name = min(c.name, maxName)
	if compact {
		c.name, c.tech = min(c.name, maxCompactName), min(c.tech, maxCompactTech)
	}
	return c
}

func (c cols) number(s string) string {
	if c.compact {
		return ""
	}
	return s + "  "
}

func (c cols) header() string {
	h := fmt.Sprintf("  %s%-4s %-*s %*s  %s %s %s  %-*s %-*s %-4s", c.number(fmt.Sprintf("%3s", "#")), "", c.name, "East", c.rec, "", " ", center("Result", c.tech), " ", c.rec, "", c.name, "West", "")
	return strings.TrimRight(h, " ")
}

// row draws one bout: the winner bold, the loser dim, records before the
// bout, the result glyphs next to the technique (SPEC §8.4).
func (c cols) row(t theme.Theme, r service.BoutRow, selected bool) string {
	decided := r.Winner != 0
	east, west := fit(r.East.Shikona.Romaji(), c.name), fit(r.West.Shikona.Romaji(), c.name)
	eastName := fmt.Sprintf("%-*s", c.name, east)
	westName := fmt.Sprintf("%-*s", c.name, west)
	switch r.Winner {
	case domain.East:
		eastName, westName = t.Bold.Render(eastName), t.Dim.Render(westName)
	case domain.West:
		eastName, westName = t.Dim.Render(eastName), t.Bold.Render(westName)
	}
	technique := center(fit(string(r.Kimarite.Kimarite), c.tech), c.tech)
	if !decided {
		technique = center("—", c.tech)
	}
	marker := "  "
	num := fmt.Sprintf("%3d", r.MatchNo)
	if selected {
		marker = t.Selected.Render(t.Cursor) + " "
		num = t.Selected.Render(num)
	}
	tag := ""
	if r.Musubi {
		tag = t.Dim.Render(t.Musubi)
	}
	if r.Kinboshi {
		tag = strings.TrimSpace(tag + " " + t.Gold.Render(t.Kinboshi))
	}
	if c.winChance && r.WinChance != nil {
		p, side := *r.WinChance, "E"
		if p < 0.5 {
			p, side = 1-p, "W"
		}
		tag = strings.TrimSpace(fmt.Sprintf("%3.0f%% %s  %s", 100*p, side, tag))
	}
	return strings.TrimRight(fmt.Sprintf("%s%s%-4s %s %*s  %s %s %s  %-*s %s %-4s %s",
		marker, c.number(num), r.East.Rank.Short(), eastName, c.rec, record(r.East),
		glyph(t, r.Winner == domain.East, decided, r.Fusen), technique, glyph(t, r.Winner == domain.West, decided, r.Fusen),
		c.rec, record(r.West), westName, r.West.Rank.Short(), tag), " ")
}

func record(s service.BoutSide) string {
	if s.Visitor {
		return "—"
	}
	return s.Before.String()
}

func glyph(t theme.Theme, won, decided, fusen bool) string {
	switch {
	case !decided:
		return t.Pending
	case won && fusen:
		return t.FusenWin
	case won:
		return t.Win
	case fusen:
		return t.FusenLoss
	default:
		return t.Loss
	}
}

func center(s string, w int) string { return lipgloss.PlaceHorizontal(w, lipgloss.Center, s) }

// fit shortens a name to w cells.
func fit(s string, w int) string {
	if len(s) <= w {
		return s
	}
	return s[:w-1] + "…"
}
