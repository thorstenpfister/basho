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
	Cursor, Refresh, Stale, Spoiler, Live, LiveWait, Line          string
}

// Unicode is the default glyph set.
var Unicode = Glyphs{
	Win: "○", Loss: "●", FusenWin: "□", FusenLoss: "■", Absent: "–", Pending: "·", Draw: "△", Unknown: "?",
	Kinboshi: "★", Up: "▲", Down: "▼", None: "–", Divider: "│",
	Musubi: "結び", Kyujo: "休",
	Complete: "✓", InProgress: "●", Scheduled: "·",
	Cursor: "▶", Refresh: "↻", Stale: "⚠", Spoiler: "🙈", Live: "●", LiveWait: "◌", Line: "─",
}

// ASCII is the glyph set of --ascii and ui.ascii.
var ASCII = Glyphs{
	Win: "o", Loss: "x", FusenWin: "O", FusenLoss: "X", Absent: "-", Pending: ".", Draw: "d", Unknown: "?",
	Kinboshi: "*", Up: "+", Down: "-", None: "-", Divider: "|",
	Musubi: "musubi", Kyujo: "kyujo",
	Complete: "v", InProgress: "*", Scheduled: ".",
	Cursor: ">", Refresh: "~", Stale: "!", Spoiler: "[SF]", Live: "*", LiveWait: "o", Line: "-",
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

// light and dark are the palette's colours on each background. Text keeps
// a WCAG contrast of 4.5 on the common terminal backgrounds, rules 1.75
// (theme_test.go lists them, Ghostty's default among them).
var (
	light = Palette{
		Fg: lipgloss.Color("#1f2328"), Dim: lipgloss.Color("#636c76"), Accent: lipgloss.Color("#4b3fd1"),
		Gold: lipgloss.Color("#8a5c00"), Green: lipgloss.Color("#1a7a35"), Red: lipgloss.Color("#cf222e"),
		Subtle: lipgloss.Color("#afb8c1"),
	}
	dark = Palette{
		Fg: lipgloss.Color("#e6e6e6"), Dim: lipgloss.Color("#8b949e"), Accent: lipgloss.Color("#8c84ff"),
		Gold: lipgloss.Color("#e3b341"), Green: lipgloss.Color("#3fb950"), Red: lipgloss.Color("#ff6e64"),
		Subtle: lipgloss.Color("#545b64"),
	}
)

func palette(isDark bool) Palette {
	p := light
	if isDark {
		p = dark
	}
	p.Categories = [domain.Other + 1]color.Color{
		domain.Push:  p.Accent, // indigo
		domain.Belt:  p.Green,
		domain.Throw: p.Gold,
		domain.Pull:  p.Red,
		domain.Other: p.Dim, // grey
	}
	return p
}

// Theme is the TUI's styles and glyphs.
type Theme struct {
	Glyphs
	Colors Palette
	Dark   bool
	ASCII  bool // the ASCII glyph set: bars use #, the STYLE bar letters
	// Mono is set when the terminal shows no colours (NO_COLOR, --no-color):
	// whatever only colour told apart needs letters, like the STYLE bar.
	Mono bool
	// Names is how wrestlers' names show (ui.names).
	Names domain.NameMode

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

// WithNames sets how names show. NamesEn means no kanji at all, so the
// musubi and kyujo marks become words, as in the ASCII set.
func (t Theme) WithNames(m domain.NameMode) Theme {
	t.Names = m
	if m == domain.NamesEn {
		t.Musubi, t.Kyujo = ASCII.Musubi, ASCII.Kyujo
	}
	return t
}

// Rebuild returns the theme for another background, keeping the rest.
func (t Theme) Rebuild(dark bool) Theme {
	n := New(dark, t.ASCII).WithNames(t.Names)
	n.Mono = t.Mono
	return n
}

// Name is a wrestler's name to lead with (ui.names).
func (t Theme) Name(n domain.Name) string { return n.Show(t.Names) }

// AltName is the name beside it, or "".
func (t Theme) AltName(n domain.Name) string { return n.Alt(t.Names) }

// Kanji is a technique's kanji, or "" when names show no kanji.
func (t Theme) Kanji(kanji string) string {
	if t.Names == domain.NamesEn {
		return ""
	}
	return kanji
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
