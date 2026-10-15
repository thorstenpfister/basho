package components

import (
	"fmt"
	"math"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

// StatRow lays out one row of the tale of the tape: the left value
// right-aligned towards the centre label, the right one left-aligned, so both
// face the middle (SPEC §8.5).
func StatRow(t theme.Theme, left, label, right string, side, middle int) string {
	l := lipgloss.PlaceHorizontal(side, lipgloss.Right, clip(left, side))
	m := lipgloss.PlaceHorizontal(middle, lipgloss.Center, t.Dim.Render(label))
	r := lipgloss.PlaceHorizontal(side, lipgloss.Left, clip(right, side))
	return l + m + r
}

func clip(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// Pips draws the last results, oldest first, with a divider where the
// tournament changes and · padding for a new wrestler (SPEC §6.2).
func Pips(t theme.Theme, last derive.LastN, n int) string {
	var b strings.Builder
	for i, r := range last.Results {
		if i > 0 && last.Results[i-1].Basho != r.Basho {
			b.WriteString(t.Dim.Render(t.Divider))
		}
		var g string
		switch {
		case r.Won && r.Fusen:
			g = t.FusenWin
		case r.Won:
			g = t.Green.Render(t.Win)
		case r.Fusen:
			g = t.FusenLoss
		default:
			g = t.Red.Render(t.Loss)
		}
		b.WriteString(g)
	}
	if last.New {
		b.WriteString(t.Dim.Render(strings.Repeat(t.Pending, max(n-len(last.Results), 0))))
	}
	return b.String()
}

// starsCompact is the count from which stars become ★×12.
const starsCompact = 5

// Stars draws the kinboshi: ★ each, ★×12 from five; those of this tournament
// in brighter gold (SPEC §6.3).
func Stars(t theme.Theme, k derive.Kinboshi) string {
	switch {
	case k.Count == 0:
		return t.Dim.Render(t.None)
	case k.Count >= starsCompact:
		s := fmt.Sprintf("%s×%d", t.Kinboshi, k.Count)
		if k.ThisBasho > 0 {
			return t.Gold.Render(s)
		}
		return t.GoldDim.Render(s)
	default:
		earlier := k.Count - k.ThisBasho
		return t.GoldDim.Render(strings.Repeat(t.Kinboshi, earlier)) + t.Gold.Render(strings.Repeat(t.Kinboshi, k.ThisBasho))
	}
}

// WinChance draws the mirrored win-chance bars from the centre outwards: the
// favourite's is longer (SPEC §8.5). east is the east side's chance.
func WinChance(t theme.Theme, east float64, cells int) (left, right string) {
	line := "━"
	if t.ASCII {
		line = "="
	}
	en := int(math.Round(east * float64(cells)))
	wn := int(math.Round((1 - east) * float64(cells)))
	style := func(won bool) lipgloss.Style {
		if won {
			return t.Accent
		}
		return t.Dim
	}
	left = fmt.Sprintf("%s %2.0f%%", style(east >= 0.5).Render(strings.Repeat(line, en)), 100*east)
	right = fmt.Sprintf("%2.0f%% %s", 100*(1-east), style(east < 0.5).Render(strings.Repeat(line, wn)))
	return left, right
}

// StyleCells is the width of the STYLE bar.
const StyleCells = 20

// Categories is the STYLE bar's order from the centre outwards.
var Categories = []domain.KimariteCategory{domain.Push, domain.Belt, domain.Throw, domain.Pull, domain.Other}

// StyleMarks are the STYLE bar's cells: a texture per category beside its
// colour, so the categories never differ by colour alone (red and green
// are belt and pull); letters without colours or in ASCII.
func StyleMarks(t theme.Theme) map[domain.KimariteCategory]string {
	if t.ASCII || t.Mono {
		return map[domain.KimariteCategory]string{domain.Push: "P", domain.Belt: "B", domain.Throw: "T", domain.Pull: "L", domain.Other: "O"}
	}
	return map[domain.KimariteCategory]string{domain.Push: "█", domain.Belt: "▓", domain.Throw: "▒", domain.Pull: "░", domain.Other: "▚"}
}

// StyleBar draws the win shares as a stacked bar of cells, one colour and
// texture per category in the order PUSH, BELT, THROW, PULL, OTHER; reversed on the left
// side so that PUSH sits next to the centre label on both sides (SPEC §8.5).
func StyleBar(t theme.Theme, s derive.Shares, cells int, left bool) string {
	n := apportion(s, cells)
	marks := StyleMarks(t)
	var parts []string
	for _, c := range Categories {
		if n[c] == 0 {
			continue
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(t.Colors.Categories[c]).Render(strings.Repeat(marks[c], n[c])))
	}
	if left {
		for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
			parts[i], parts[j] = parts[j], parts[i]
		}
	}
	return strings.Join(parts, "")
}

// apportion splits cells by share with the largest remainders, so the parts
// add up exactly.
func apportion(s derive.Shares, cells int) [domain.Other + 1]int {
	var n [domain.Other + 1]int
	var total float64
	for _, c := range Categories {
		total += s[c]
	}
	if total == 0 {
		return n
	}
	used := 0
	rem := map[domain.KimariteCategory]float64{}
	for _, c := range Categories {
		exact := s[c] / total * float64(cells)
		n[c] = int(exact)
		used += n[c]
		rem[c] = exact - float64(n[c])
	}
	for used < cells {
		best := Categories[0]
		for _, c := range Categories {
			if rem[c] > rem[best] {
				best = c
			}
		}
		n[best]++
		rem[best] = -1
		used++
	}
	return n
}
