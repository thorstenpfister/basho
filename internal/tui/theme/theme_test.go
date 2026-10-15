package theme

import (
	"image/color"
	"math"
	"reflect"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestGlyphSetsComplete(t *testing.T) {
	for name, g := range map[string]Glyphs{"unicode": Unicode, "ascii": ASCII} {
		v := reflect.ValueOf(g)
		for i := range v.NumField() {
			if v.Field(i).String() == "" {
				t.Errorf("%s: %s is empty", name, v.Type().Field(i).Name)
			}
		}
	}
	for i, r := range ASCII.Win + ASCII.Loss + ASCII.Spoiler + ASCII.Musubi {
		if r > 127 {
			t.Errorf("ASCII glyph %d is %q", i, r)
		}
	}
}

func TestOutcome(t *testing.T) {
	for o, want := range map[domain.Outcome]string{
		domain.Win: "○", domain.Loss: "●", domain.FusenWin: "□", domain.FusenLoss: "■",
		domain.Absent: "–", domain.Pending: "·", domain.Draw: "△", domain.Unknown: "?",
	} {
		if got := Unicode.Outcome(o); got != want {
			t.Errorf("%v: %q, want %q", o, got, want)
		}
	}
}

func TestNew(t *testing.T) {
	dark, light := New(true, false), New(false, true)
	if dark.Colors.Fg == light.Colors.Fg || dark.Colors.Accent == light.Colors.Accent {
		t.Error("the palette does not adapt to the background")
	}
	if light.Win != "o" || dark.Win != "○" {
		t.Error("glyph sets")
	}
	if got := dark.Record("8-7", true, false); got == "8-7" {
		t.Error("kachi-koshi not coloured")
	}
	if got := dark.Record("7-7", false, false); got != "7-7" {
		t.Error("a plain record coloured")
	}
}

// Text colours stay readable on the common terminal backgrounds (WCAG 4.5),
// rules and borders visible (1.75).
func TestContrast(t *testing.T) {
	backgrounds := map[bool]map[string]string{
		false: {"white": "#ffffff", "Solarized Light": "#fdf6e3", "Catppuccin Latte": "#eff1f5"},
		true:  {"black": "#000000", "VS Code": "#1e1e1e", "Ghostty": "#282c34", "Solarized Dark": "#002b36"},
	}
	for isDark, bgs := range backgrounds {
		p := palette(isDark)
		text := map[string]color.Color{"Fg": p.Fg, "Dim": p.Dim, "Accent": p.Accent, "Gold": p.Gold, "Green": p.Green, "Red": p.Red}
		for bg, hex := range bgs {
			for name, c := range text {
				if r := contrast(c, lipgloss.Color(hex)); r < 4.5 {
					t.Errorf("dark=%v: %s on %s: %.2f", isDark, name, bg, r)
				}
			}
			if r := contrast(p.Subtle, lipgloss.Color(hex)); r < 1.75 {
				t.Errorf("dark=%v: Subtle on %s: %.2f", isDark, bg, r)
			}
		}
	}
}

// contrast is the WCAG 2 contrast ratio.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		x := float64(v) / 0xffff
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}
