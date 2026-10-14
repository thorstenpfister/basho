package service

import (
	"context"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// Horizons give each tournament's spoiler horizon as a cutoff;
// *spoiler.State implements it.
type Horizons interface {
	Cutoff(id domain.BashoID) derive.AsOf
}

type hidingKey struct{}

// Hiding marks ctx for spoiler-free mode (SPEC §9): every answer stops at
// the tournament's horizon. Results after it come back undecided with
// Hidden set, records and ratings freeze there, and the yusho stays unknown
// until the horizon has passed the whole tournament.
func Hiding(ctx context.Context, h Horizons) context.Context {
	return context.WithValue(ctx, hidingKey{}, h)
}

// horizon returns tournament id's horizon when ctx hides spoilers.
func horizon(ctx context.Context, id domain.BashoID) (derive.AsOf, bool) {
	h, ok := ctx.Value(hidingKey{}).(Horizons)
	if !ok || h == nil {
		return derive.NoCutoff, false
	}
	return h.Cutoff(id), true
}

// limit cuts c off at its tournament's horizon as well.
func limit(ctx context.Context, c derive.AsOf) derive.AsOf {
	h, _ := horizon(ctx, c.Basho)
	return derive.Earlier(c, h)
}

// visible reports whether a bout's result may be shown.
func visible(ctx context.Context, b domain.Bout) bool {
	h, _ := horizon(ctx, b.Basho)
	return h.Includes(b)
}

// finalShown reports whether a tournament's final results (yusho, prizes)
// may be shown: the horizon has passed it.
func finalShown(ctx context.Context, id domain.BashoID) bool {
	h, _ := horizon(ctx, id)
	return !derive.EndOf(id).After(h)
}
