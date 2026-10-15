package portrait

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRender(t *testing.T) {
	red, blue := color.NRGBA{255, 0, 0, 255}, color.NRGBA{0, 0, 255, 255}
	img := image.NewNRGBA(image.Rect(0, 0, 3, 3)) // odd height: the last line has no lower row
	img.SetNRGBA(0, 0, red)
	img.SetNRGBA(0, 1, blue)
	img.SetNRGBA(1, 1, blue) // only the lower pixel
	img.SetNRGBA(0, 2, red)
	out := Render(img, false)
	if got := ansi.Strip(out); got != "▀▄ \n▀  " {
		t.Errorf("cells %q", got)
	}
	first := strings.Split(out, "\n")[0]
	if !strings.Contains(first, "38;2;255;0;0") || !strings.Contains(first, "48;2;0;0;255") {
		t.Errorf("colours %q", first)
	}
	if got := ansi.Strip(Render(img, true)); got != " ▄▀\n  ▀" {
		t.Errorf("mirrored %q", got)
	}
}

func TestSilhouette(t *testing.T) {
	for _, size := range Sizes {
		s := Silhouette(size, color.White)
		lines := strings.Split(ansi.Strip(Render(s, false)), "\n")
		if len(lines) != size/2 || ansi.StringWidth(lines[0]) != size {
			t.Fatalf("%d: %d lines of %d", size, len(lines), ansi.StringWidth(lines[0]))
		}
		if strings.TrimSpace(lines[0]) == "" && strings.TrimSpace(lines[1]) == "" {
			t.Errorf("%d: no topknot at the top:\n%s", size, strings.Join(lines, "\n"))
		}
		if s.ColorIndexAt(0, 0) != 0 || s.ColorIndexAt(size/2, size-1) != 1 || s.ColorIndexAt(size/2, size/3) != 1 {
			t.Errorf("%d: corners clear, head and body drawn", size)
		}
		for y := range size { // symmetric, so mirroring changes nothing
			for x := range size {
				if s.ColorIndexAt(x, y) != s.ColorIndexAt(size-1-x, y) {
					t.Fatalf("%d: not symmetric at %d,%d", size, x, y)
				}
			}
		}
	}
	t.Log("\n" + ansi.Strip(Render(Silhouette(20, color.White), false)))
}

// A 32 px portrait is 512 styled cells; the VS screen redraws it on every
// animation frame unless it keeps the text.
func BenchmarkRender(b *testing.B) {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.SetNRGBA(x, y, color.NRGBA{uint8(8 * x), uint8(8 * y), 128, 255})
		}
	}
	for b.Loop() {
		_ = Render(img, true)
	}
}
