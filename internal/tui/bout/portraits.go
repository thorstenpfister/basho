package bout

import (
	"fmt"
	"image"

	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
	"github.com/thorstenpfister/basho/internal/tui"
)

// portraitSize is the size of the portraits above the names (SPEC §8.9):
// the largest of portrait.Sizes that fits its wrestler's column and, with
// the caption, the rows left; 0 for none. They show only when at least one
// wrestler has a photo: two silhouettes alone are just a distraction.
func (v VS) portraitSize(ctx *tui.Context, l layout, rows int) int {
	t := ctx.Theme
	if !ctx.Cfg.UI.Portraits || t.ASCII || t.Mono || ctx.Svc == nil || l.Layout == tui.Compact || !v.hasPhoto(ctx) {
		return 0
	}
	for _, size := range portrait.Sizes { // largest first
		if size <= l.side && size/2+1 <= rows {
			return size
		}
	}
	return 0
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

// portraitRow is the portraits above the names, each centred over its
// wrestler's column: East facing right, West mirrored, facing left.
func (v VS) portraitRow(ctx *tui.Context, l layout, size int) string {
	t := ctx.Theme
	side := func(id domain.RikishiID, mirror bool) string {
		p := v.portrait(ctx, id, size, mirror)
		return lipgloss.NewStyle().Width(l.side).Align(lipgloss.Center).Render(p.text + "\n" + t.Dim.Render(p.caption))
	}
	gap := lipgloss.NewStyle().Width(l.middle).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, " ", side(v.row.East.RikishiID, false), gap, side(v.row.West.RikishiID, true))
}

// hasPhoto reports whether either wrestler has a portrait.
func (v VS) hasPhoto(ctx *tui.Context) bool {
	return ctx.Svc != nil && (ctx.Svc.HasPortrait(v.row.East.RikishiID) || ctx.Svc.HasPortrait(v.row.West.RikishiID))
}
