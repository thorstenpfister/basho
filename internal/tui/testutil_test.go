package tui

import (
	"image/color"

	"github.com/charmbracelet/x/ansi"
)

var lipglossWhite color.Color = color.White

func stripANSI(s string) string { return ansi.Strip(s) }
