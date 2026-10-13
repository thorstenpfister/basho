package service

import (
	"context"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

// TournamentSource is a data layer behind the snapshot: in production the
// sumo-api adapter over the disk cache, which in turn sits over the network.
// The resolver's interfaces (SPEC §5.3, Phase 9) will replace it.
type TournamentSource interface {
	Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error)
	Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error)
	Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error)
}

// Tournaments asks the data layers in the order of SPEC §5.8: the embedded
// snapshot for every tournament it holds, which is never fetched or cached
// again, then Remote (disk cache, then network) for everything newer.
type Tournaments struct {
	Snapshot *snapshot.Snapshot // nil: --no-snapshot
	Remote   TournamentSource
}

func (t Tournaments) fromSnapshot(id domain.BashoID) bool {
	return t.Snapshot != nil && t.Snapshot.Has(id)
}

// Basho returns a tournament's metadata.
func (t Tournaments) Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error) {
	if t.fromSnapshot(id) {
		return t.Snapshot.Basho(id)
	}
	return t.Remote.Basho(ctx, id)
}

// Banzuke returns a division's banzuke.
func (t Tournaments) Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	if t.fromSnapshot(id) {
		return t.Snapshot.Banzuke(id, div)
	}
	return t.Remote.Banzuke(ctx, id, div)
}

// Torikumi returns a division's bouts of one day.
func (t Tournaments) Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	if t.fromSnapshot(id) {
		return t.Snapshot.Torikumi(id, div, day)
	}
	return t.Remote.Torikumi(ctx, id, div, day)
}
