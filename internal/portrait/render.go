package portrait

import (
	"image"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Render draws img as half-block text (SPEC §8.9): each line holds two
// pixel rows, the upper as the foreground of ▀, the lower as its
// background; transparent pixels show the terminal. mirror flips it, so
// the West wrestler faces the East one. Colours are true colour; Bubble
// Tea's renderer brings them down to the terminal's profile (256 colours
// and fewer).
func Render(img image.Image, mirror bool) string {
	b := img.Bounds()
	at := func(x, y int) (color.Color, bool) {
		if y >= b.Max.Y {
			return nil, false
		}
		if mirror {
			x = b.Max.X - 1 - (x - b.Min.X)
		}
		c := img.At(x, y)
		_, _, _, a := c.RGBA()
		return c, a >= 0x8000
	}
	lines := make([]string, 0, (b.Dy()+1)/2)
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		var line strings.Builder
		for x := b.Min.X; x < b.Max.X; x++ {
			up, upOK := at(x, y)
			down, downOK := at(x, y+1)
			switch {
			case upOK && downOK:
				line.WriteString(lipgloss.NewStyle().Foreground(up).Background(down).Render("▀"))
			case upOK:
				line.WriteString(lipgloss.NewStyle().Foreground(up).Render("▀"))
			case downOK:
				line.WriteString(lipgloss.NewStyle().Foreground(down).Render("▄"))
			default:
				line.WriteByte(' ')
			}
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

// Silhouette is the stand-in for a wrestler without a photo: a head with a
// topknot over broad shoulders, in c on a transparent ground, size pixels
// square (SPEC §8.9).
func Silhouette(size int, c color.Color) *image.Paletted {
	img := image.NewPaletted(image.Rect(0, 0, size, size), color.Palette{color.Transparent, c})
	s := float64(size)
	inEllipse := func(x, y, cx, cy, rx, ry float64) bool {
		dx, dy := (x-cx)/rx, (y-cy)/ry
		return dx*dx+dy*dy <= 1
	}
	for py := range size {
		for px := range size {
			x, y := float64(px)+0.5, float64(py)+0.5
			head := inEllipse(x, y, s/2, 0.36*s, 0.17*s, 0.19*s)
			topknot := inEllipse(x, y, s/2, 0.14*s, 0.07*s, 0.05*s)
			body := y >= 0.56*s && inEllipse(x, y, s/2, 1.05*s, 0.5*s, 0.5*s)
			if head || topknot || body {
				img.SetColorIndex(px, py, 1)
			}
		}
	}
	return img
}
