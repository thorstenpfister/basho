package bout

import (
	"fmt"
	"image"

	"charm.land/lipgloss/v2"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/portrait"
	"github.com/thorstenpfister/basho/internal/tui"
)

// minTape is the narrowest the tale of the tape gets beside portraits:
// hometowns and the bars need it.
const minTape = 80

// screenRows is what the VS screen needs besides the portrait's rows:
// heading, result panel, hints and the tape below a portrait.
const screenRows = 14

// portraitSize is the portraits' size for the width left beside the
// history column and the screen's height: the largest of portrait.Sizes
// that leaves the tape minTape columns and fits its rows (half its pixels,
// and the caption) on the screen; 0 for none (SPEC §8.9).
func portraitSize(ctx *tui.Context, avail, height int) int {
	t := ctx.Theme
	if !ctx.Cfg.UI.Portraits || t.ASCII || t.Mono || ctx.Svc == nil {
		return 0
	}
	for _, size := range portrait.Sizes { // largest first
		if avail >= 2*(size+1)+minTape && height >= size/2+1+screenRows {
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

// withPortraits sets the portraits beside the tape: East on the left
// facing right, West on the right, mirrored, facing left.
func (v VS) withPortraits(ctx *tui.Context, tape string, size int) string {
	t := ctx.Theme
	side := func(id domain.RikishiID, mirror bool) string {
		p := v.portrait(ctx, id, size, mirror)
		return lipgloss.JoinVertical(lipgloss.Center, p.text, t.Dim.Render(lipgloss.PlaceHorizontal(size, lipgloss.Center, p.caption)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, " ", side(v.row.East.RikishiID, false), tape, side(v.row.West.RikishiID, true))
}
