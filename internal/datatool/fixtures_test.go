package datatool

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/thorstenpfister/basho/internal/fixture"
)

func TestRecordFixturesWritesEveryRequest(t *testing.T) {
	out := t.TempDir()
	api := &fakeAPI{startDate: "2026-09-13T00:00:00Z"}
	if err := RecordFixtures(t.Context(), api, out, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, s := range fixtureSet {
		for _, p := range s.paths {
			if _, err := os.Stat(filepath.Join(out, s.dir, fixture.Name(p))); err != nil {
				t.Error(err)
			}
		}
	}
}

func TestRecordFixturesStopsOnErrorStatus(t *testing.T) {
	api := &fakeAPI{status503: "/api/basho/196001"}
	if err := RecordFixtures(t.Context(), api, t.TempDir(), io.Discard); err == nil {
		t.Error("a 503 must stop the recording")
	}
}
