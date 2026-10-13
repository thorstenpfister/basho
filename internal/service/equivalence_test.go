package service

import (
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// TestSnapshotMatchesAdapter checks the two paths to a finished tournament
// against each other (SPEC §13): the fixture snapshot, and the sumo-api
// adapter over the same recorded responses. Every recorded banzuke and day
// must come out identical.
func TestSnapshotMatchesAdapter(t *testing.T) {
	snap, err := snapshot.Open(os.DirFS("../../testdata/snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	viaSnapshot := Tournaments{Snapshot: snap}
	viaAdapter := Tournaments{Remote: sumoapi.New(fixture.Getter{Dir: "../../testdata/sumoapi", Base: sumoapi.DefaultBaseURL}, sumoapi.DefaultBaseURL)}
	ctx := t.Context()

	entries, err := os.ReadDir("../../testdata/sumoapi")
	if err != nil {
		t.Fatal(err)
	}
	compared := 0
	for _, e := range entries {
		// basho_YYYYMM.json, basho_YYYYMM_banzuke_DIV.json, basho_YYYYMM_torikumi_DIV_DAY.json
		parts := strings.Split(strings.TrimSuffix(e.Name(), ".json"), "_")
		if parts[0] != "basho" {
			continue
		}
		id, err := domain.ParseBashoID(parts[1])
		if err != nil || !snap.Has(id) {
			continue // recorded, but not a finished fixture tournament
		}
		var got, want any
		var gotErr, wantErr error
		switch len(parts) {
		case 2:
			got, gotErr = viaSnapshot.Basho(ctx, id)
			want, wantErr = viaAdapter.Basho(ctx, id)
		case 4:
			div, _ := domain.ParseDivision(parts[3])
			got, gotErr = viaSnapshot.Banzuke(ctx, id, div)
			want, wantErr = viaAdapter.Banzuke(ctx, id, div)
		case 5:
			div, _ := domain.ParseDivision(parts[3])
			day, _ := strconv.Atoi(parts[4])
			got, gotErr = viaSnapshot.Torikumi(ctx, id, div, day)
			want, wantErr = viaAdapter.Torikumi(ctx, id, div, day)
		default:
			t.Fatalf("unexpected fixture %s", e.Name())
		}
		if gotErr != nil || wantErr != nil {
			t.Errorf("%s: snapshot %v, adapter %v", e.Name(), gotErr, wantErr)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: the snapshot and the adapter differ", e.Name())
		}
		compared++
	}
	t.Logf("compared %d recordings", compared)
	if compared < 15 {
		t.Errorf("compared %d recordings; the fixtures should give at least 15", compared)
	}

	// An unrecorded day is empty in the snapshot and missing from the adapter.
	if bouts, err := viaSnapshot.Torikumi(ctx, domain.FirstBasho, domain.DivMakuuchi, 2); err != nil || len(bouts) != 0 {
		t.Errorf("unrecorded day from the snapshot: %d bouts, %v", len(bouts), err)
	}
	if _, err := viaAdapter.Torikumi(ctx, domain.FirstBasho, domain.DivMakuuchi, 2); !errors.Is(err, source.ErrNotFound) {
		t.Errorf("unrecorded day from the adapter: %v", err)
	}
}
