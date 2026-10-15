package bout

import (
	"fmt"
	"image"

	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
	"github.com/thorstenpfister/basho/internal/tui"
)

// screenPortrait is the VS screen's portrait size: 20 px keeps the faces
// from pulling the eye off the comparison (32 px is for the wrestler screen).
const screenPortrait = 20

// portraitSize is the size of the portraits above the names (SPEC §8.9):
// screenPortrait when it fits its wrestler's column and, with the caption,
// the rows left; 0 for none. They show only when at least one wrestler has
// a photo: two silhouettes alone are just a distraction.
func (v VS) portraitSize(ctx *tui.Context, l layout, rows int) int {
	t := ctx.Theme
	// Only once the tale is in: while it loads the screen is a row or two
	// shorter, and a portrait that fits then would vanish when it arrives.
	if !ctx.Cfg.UI.Portraits || t.ASCII || t.Mono || ctx.Svc == nil || l.Layout == tui.Compact || v.tale == nil || !v.hasPhoto(ctx) {
		return 0
	}
	if screenPortrait > l.side || screenPortrait/2+1 > rows {
		return 0
	}
	return screenPortrait
}

// pictures keeps each rendered portrait: the text never changes for a bout,
// and the VS screen redraws on every animation frame.
type pictures map[pictureKey]picture

type pictureKey struct {
	id     domain.RikishiID
	size   int
	mirror bool
}

type picture struct {
	text    string
	caption string
}

// portrait returns a wrestler's portrait as text with its caption, the
// photo's year; a silhouette without a photo.
func (v VS) portrait(ctx *tui.Context, id domain.RikishiID, size int, mirror bool) picture {
	k := pictureKey{id, size, mirror}
	if p, ok := v.pics[k]; ok {
		return p
	}
	var p picture
	if img, credit, ok := ctx.Svc.Portrait(id, size); ok {
		p.text = portrait.Render(img, mirror)
		if credit.Year > 0 {
			p.caption = fmt.Sprintf("photo %d", credit.Year)
		}
	} else {
		var img image.Image = portrait.Silhouette(size, ctx.Theme.Colors.Subtle)
		p.text = portrait.Render(img, mirror)
	}
	if v.pics != nil {
		v.pics[k] = p
	}
	return p
}

// portraitRow is the portraits above the names, each lined up with its
// wrestler's values next to the labels, as the columns are: East's to the
// right, facing right, West's to the left, mirrored, facing left.
func (v VS) portraitRow(ctx *tui.Context, l layout, size int) string {
	t := ctx.Theme
	side := func(id domain.RikishiID, mirror bool, align lipgloss.Position) string {
		p := v.portrait(ctx, id, size, mirror)
		caption := lipgloss.NewStyle().Width(size).Align(align).Render(t.Dim.Render(p.caption))
		block := lipgloss.JoinVertical(align, p.text, caption)
		return lipgloss.NewStyle().Width(l.side).Align(align).Render(block)
	}
	gap := lipgloss.NewStyle().Width(l.middle).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, " ", side(v.row.East.RikishiID, false, lipgloss.Right), gap, side(v.row.West.RikishiID, true, lipgloss.Left))
}

// hasPhoto reports whether either wrestler has a portrait.
func (v VS) hasPhoto(ctx *tui.Context) bool {
	return ctx.Svc != nil && (ctx.Svc.HasPortrait(v.row.East.RikishiID) || ctx.Svc.HasPortrait(v.row.West.RikishiID))
}
