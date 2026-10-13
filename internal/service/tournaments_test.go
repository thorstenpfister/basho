package service

import (
	"context"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/snapshot/snapshottest"
)

// remote counts the requests that get past the snapshot.
type remote struct{ calls int }

func (r *remote) Basho(_ context.Context, id domain.BashoID) (domain.Basho, error) {
	r.calls++
	return domain.Basho{ID: id}, nil
}

func (r *remote) Banzuke(context.Context, domain.BashoID, domain.Division) ([]domain.BanzukeEntry, error) {
	r.calls++
	return nil, nil
}

func (r *remote) Torikumi(context.Context, domain.BashoID, domain.Division, int) ([]domain.Bout, error) {
	r.calls++
	return nil, nil
}

func TestTournamentsLayerOrder(t *testing.T) {
	inSnapshot := domain.BashoID{Year: 2026, Month: time.July}
	fsys, err := snapshottest.MapFS([]snapshot.Tournament{{Basho: domain.Basho{ID: inSnapshot, Location: "snapshot"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := snapshot.Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	r := &remote{}
	ts := Tournaments{Snapshot: snap, Remote: r}
	ctx := t.Context()

	b, err := ts.Basho(ctx, inSnapshot)
	_, _ = ts.Banzuke(ctx, inSnapshot, domain.DivMakuuchi)
	_, _ = ts.Torikumi(ctx, inSnapshot, domain.DivMakuuchi, 1)
	if err != nil || b.Location != "snapshot" || r.calls != 0 {
		t.Errorf("a snapshot tournament: %+v, %v after %d remote calls; want none", b, err, r.calls)
	}

	newer := inSnapshot.Next()
	_, _ = ts.Basho(ctx, newer)
	_, _ = ts.Banzuke(ctx, newer, domain.DivMakuuchi)
	_, _ = ts.Torikumi(ctx, newer, domain.DivMakuuchi, 1)
	if r.calls != 3 {
		t.Errorf("a newer tournament: %d remote calls, want 3", r.calls)
	}

	r.calls = 0
	_, _ = Tournaments{Remote: r}.Basho(ctx, inSnapshot) // --no-snapshot
	if r.calls != 1 {
		t.Error("without a snapshot, the remote layer is not asked")
	}
}
