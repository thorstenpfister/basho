// Package asciifold swaps the symbols basho draws for ASCII stand-ins of the
// same width, for --ascii in the TUI and the CLI.
package asciifold

import "strings"

// folds maps the symbols the glyph sets don't cover (separators, ellipses,
// arrows, bars, box borders) to ASCII of the same width, so --ascii can
// fold a finished screen without moving anything.
var folds = []string{
	// punctuation
	"·", ".", "—", "-", "–", "-", "−", "-", "…", ".", "×", "x", "≈", "~",
	// arrows, markers and the day bar
	"←", "<", "→", ">", "↑", "^", "↓", "v", "⇄", "/", "▶", ">", "▸", ">", "▾", "v",
	"✓", "v", "↻", "~", "⚠", "!", "◌", "o",
	// result glyphs, for text that bypasses the glyph set
	"○", "o", "●", "x", "□", "O", "■", "X", "△", "d", "★", "*", "▲", "+", "▼", "-",
	// bars, and the sparklines' heights as a ramp
	"█", "#", "━", "=", "░", ".", "▓", "#", "▒", "+", "▚", "x",
	"▁", "_", "▂", "_", "▃", ".", "▄", "-", "▅", "-", "▆", "=", "▇", "=",
	// lines and box borders (lipgloss NormalBorder and RoundedBorder)
	"─", "-", "│", "|", "┌", "+", "┐", "+", "└", "+", "┘", "+", "╭", "+", "╮", "+", "╰", "+", "╯", "+",
	"├", "+", "┤", "+", "┬", "+", "┴", "+", "┼", "+",
}

var fold = strings.NewReplacer(folds...)

// Fold replaces the symbols left in s by their ASCII stand-ins (SPEC §8.1,
// --ascii). Each stand-in is one cell, like the symbol, so layouts hold.
// Names and techniques in kanji are data and stay.
func Fold(s string) string { return fold.Replace(s) }
