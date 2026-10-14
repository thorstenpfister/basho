package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Glyphs are the symbols the TUI draws (SPEC §8.1).
type Glyphs struct {
	Win, Loss, FusenWin, FusenLoss, Absent, Pending, Draw, Unknown string
	Kinboshi, Up, Down, None, Divider                              string
	Musubi, Kyujo                                                  string
	Complete, InProgress, Scheduled                                string // day bar
	Cursor, Refresh, Stale, Spoiler, Live, Line                    string
}

// Unicode is the default glyph set.
var Unicode = Glyphs{
	Win: "○", Loss: "●", FusenWin: "□", FusenLoss: "■", Absent: "–", Pending: "·", Draw: "△", Unknown: "?",
	Kinboshi: "★", Up: "▲", Down: "▼", None: "–", Divider: "│",
	Musubi: "結び", Kyujo: "休",
	Complete: "✓", InProgress: "●", Scheduled: "·",
	Cursor: "▶", Refresh: "↻", Stale: "⚠", Spoiler: "🙈", Live: "●", Line: "─",
}

// ASCII is the glyph set of --ascii and ui.ascii.
var ASCII = Glyphs{
	Win: "o", Loss: "x", FusenWin: "O", FusenLoss: "X", Absent: "-", Pending: ".", Draw: "d", Unknown: "?",
	Kinboshi: "*", Up: "+", Down: "-", None: "-", Divider: "|",
	Musubi: "musubi", Kyujo: "kyujo",
	Complete: "v", InProgress: "*", Scheduled: ".",
	Cursor: ">", Refresh: "~", Stale: "!", Spoiler: "[SF]", Live: "*", Line: "-",
}

// Outcome returns the glyph of a day result.
func (g Glyphs) Outcome(o domain.Outcome) string {
	switch o {
	case domain.Win:
		return g.Win
	case domain.Loss:
		return g.Loss
	case domain.FusenWin:
		return g.FusenWin
	case domain.FusenLoss:
		return g.FusenLoss
	case domain.Absent:
		return g.Absent
	case domain.Draw:
		return g.Draw
	case domain.Unknown:
		return g.Unknown
	default:
		return g.Pending
	}
}

// Palette is the colours of one background (SPEC §8.1): neutral foreground,
// dim secondary text, one accent (indigo), gold, green and red.
type Palette struct {
	Fg, Dim, Accent, Gold, Green, Red, Subtle color.Color
	// Categories colour the STYLE bar: push, belt, throw, pull, other
	// (SPEC §8.5), indexed by domain.Push … domain.Other.
	Categories [domain.Other + 1]color.Color
}

func palette(dark bool) Palette {
	ld := lipgloss.LightDark(dark)
	return Palette{
		Fg:     ld(lipgloss.Color("#1f2328"), lipgloss.Color("#e6e6e6")),
		Dim:    ld(lipgloss.Color("#6e7781"), lipgloss.Color("#8b949e")),
		Accent: ld(lipgloss.Color("#4b3fd1"), lipgloss.Color("#8c84ff")),
		Gold:   ld(lipgloss.Color("#9a6700"), lipgloss.Color("#e3b341")),
		Green:  ld(lipgloss.Color("#1a7f37"), lipgloss.Color("#3fb950")),
		Red:    ld(lipgloss.Color("#cf222e"), lipgloss.Color("#f85149")),
		Subtle: ld(lipgloss.Color("#d0d7de"), lipgloss.Color("#30363d")),
		Categories: [domain.Other + 1]color.Color{
			domain.Push:  ld(lipgloss.Color("#4b3fd1"), lipgloss.Color("#8c84ff")), // indigo
			domain.Belt:  ld(lipgloss.Color("#1a7f37"), lipgloss.Color("#3fb950")), // green
			domain.Throw: ld(lipgloss.Color("#9a6700"), lipgloss.Color("#e3b341")), // gold
			domain.Pull:  ld(lipgloss.Color("#cf222e"), lipgloss.Color("#f85149")), // red
			domain.Other: ld(lipgloss.Color("#6e7781"), lipgloss.Color("#8b949e")), // grey
		},
	}
}

// Theme is the TUI's styles and glyphs.
type Theme struct {
	Glyphs
	Colors Palette
	Dark   bool
	ASCII  bool // the ASCII glyph set: bars use #, the STYLE bar letters

	Text, Dim, Bold, Title, Accent, Gold, GoldDim, Green, Red lipgloss.Style
	Selected, Header, Rule, StatusBar, StatusKey              lipgloss.Style
	Error, Message                                            lipgloss.Style
}

// New returns the theme for a dark or light background.
func New(dark, ascii bool) Theme {
	p := palette(dark)
	g := Unicode
	if ascii {
		g = ASCII
	}
	base := lipgloss.NewStyle().Foreground(p.Fg)
	return Theme{
		Glyphs: g, Colors: p, Dark: dark, ASCII: ascii,
		Text:      base,
		Dim:       lipgloss.NewStyle().Foreground(p.Dim),
		Bold:      base.Bold(true),
		Title:     base.Bold(true),
		Accent:    lipgloss.NewStyle().Foreground(p.Accent),
		Gold:      lipgloss.NewStyle().Foreground(p.Gold).Bold(true),
		GoldDim:   lipgloss.NewStyle().Foreground(p.Gold).Faint(true),
		Green:     lipgloss.NewStyle().Foreground(p.Green),
		Red:       lipgloss.NewStyle().Foreground(p.Red),
		Selected:  lipgloss.NewStyle().Foreground(p.Accent).Bold(true),
		Header:    lipgloss.NewStyle().Foreground(p.Dim).Bold(true),
		Rule:      lipgloss.NewStyle().Foreground(p.Subtle),
		StatusBar: lipgloss.NewStyle().Foreground(p.Dim),
		StatusKey: lipgloss.NewStyle().Foreground(p.Accent),
		Error:     lipgloss.NewStyle().Foreground(p.Red).Bold(true),
		Message:   lipgloss.NewStyle().Foreground(p.Accent),
	}
}

// Record colours a record by kachi-koshi (green) or make-koshi (red).
func (t Theme) Record(text string, kachiKoshi, makeKoshi bool) string {
	switch {
	case kachiKoshi:
		return t.Green.Render(text)
	case makeKoshi:
		return t.Red.Render(text)
	default:
		return text
	}
}
