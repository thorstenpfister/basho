package datatool

import (
	"bytes"
	"context"
	"flag"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

var update = flag.Bool("update", false, "rebuild testdata/snapshot from the recorded fixtures")

// fixtureSnapshotDir is the three-tournament snapshot the app's tests load
// (SPEC §13): 196001, 202607 and 202609, with the days that are recorded.
const fixtureSnapshotDir = "../../testdata/snapshot"

var fixtureTournaments = []domain.BashoID{
	domain.FirstBasho, {Year: 2026, Month: time.July}, {Year: 2026, Month: time.September},
}

// firstIndexPage serves the recorded responses; only the first page of the
// wrestler index is recorded, so later pages are empty.
type firstIndexPage struct{ fixture.Getter }

func (g firstIndexPage) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	if strings.Contains(url, "/rikishis?") && !strings.HasSuffix(url, "skip=0") {
		return &httpx.Response{Status: 200, Body: []byte(`{"total":0,"records":[]}`)}, nil
	}
	return g.Getter.Get(ctx, url, v)
}

func buildFixtureSnapshot(t *testing.T, dir string) {
	t.Helper()
	g := firstIndexPage{fixture.Getter{Dir: "../../testdata/sumoapi", Base: sumoapi.DefaultBaseURL}}
	cfg := SnapshotConfig{Dir: dir, Only: fixtureTournaments, Partial: true, Today: today, Generator: "fixtures"}
	if _, err := UpdateSnapshot(t.Context(), sumoapi.New(g, sumoapi.DefaultBaseURL), cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
}

// TestFixtureSnapshot keeps testdata/snapshot in step with the builder and
// the recordings. Rebuild it with: go test ./internal/datatool -run FixtureSnapshot -update
func TestFixtureSnapshot(t *testing.T) {
	if *update {
		if err := os.RemoveAll(fixtureSnapshotDir); err != nil {
			t.Fatal(err)
		}
		buildFixtureSnapshot(t, fixtureSnapshotDir)
	}
	fresh := t.TempDir()
	buildFixtureSnapshot(t, fresh)

	want, err := snapshot.ReadManifest(os.DirFS(fresh))
	if err != nil {
		t.Fatal(err)
	}
	got, err := snapshot.ReadManifest(os.DirFS(fixtureSnapshotDir))
	if err != nil {
		t.Fatal(err)
	}
	got.GeneratedAt, want.GeneratedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("testdata/snapshot is out of date; rebuild with -update\n got: %+v\nwant: %+v", got, want)
	}
	for p := range want.Files {
		a, _ := os.ReadFile(filepath.Join(fresh, p))
		b, _ := os.ReadFile(filepath.Join(fixtureSnapshotDir, p))
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs from a fresh build; rebuild with -update", p)
		}
	}
	if !reflect.DeepEqual(want.Basho, fixtureTournaments) {
		t.Errorf("tournaments = %v", want.Basho)
	}
	problems, err := VerifySnapshot(os.DirFS(fixtureSnapshotDir))
	if err != nil {
		t.Fatal(err)
	}
	// Unrecorded days leave records without bouts, only the first page of the
	// wrestler index is recorded, and so is 202607's Makuuchi banzuke alone;
	// nothing else may be wrong.
	for _, p := range problems {
		if !strings.Contains(p, "without a bout") && !strings.HasSuffix(p, "not in the index") &&
			p != "202607: Juryo yusho winner 49 is not on its banzuke" {
			t.Errorf("verify: %s", p)
		}
	}
	if _, err := fs.Stat(os.DirFS(fixtureSnapshotDir), snapshot.RikishiPath); err != nil {
		t.Error(err)
	}
}
