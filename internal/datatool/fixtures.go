package datatool

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// fixtureSet is what the tests need (SPEC §13), recorded once.
var fixtureSet = []struct {
	dir   string // under testdata/
	base  string
	paths []string
}{
	{"sumoapi", sumoapi.DefaultBaseURL, []string{
		// 202609: a finished tournament, in full for the days the tests use.
		"/api/basho/202609",
		"/api/basho/202609/banzuke/Makuuchi",
		"/api/basho/202609/banzuke/Juryo",
		"/api/basho/202609/torikumi/Makuuchi/1",
		"/api/basho/202609/torikumi/Makuuchi/13",
		"/api/basho/202609/torikumi/Makuuchi/15",
		"/api/basho/202609/torikumi/Juryo/1",
		"/api/basho/202609/torikumi/Juryo/13",
		"/api/basho/202609/torikumi/Juryo/15",
		// 196001: the old era (no kanji for opponents, 18 Juryo ranks).
		"/api/basho/196001",
		"/api/basho/196001/banzuke/Makuuchi",
		"/api/basho/196001/banzuke/Juryo",
		"/api/basho/196001/torikumi/Makuuchi/1",
		"/api/basho/196001/torikumi/Makuuchi/15",
		"/api/basho/196001/torikumi/Juryo/15",
		// 202607 day 15: 20 Makuuchi bouts (a Juryo wrestler fills in); no playoff bouts (V2).
		"/api/basho/202607",
		"/api/basho/202607/banzuke/Makuuchi",
		"/api/basho/202607/torikumi/Makuuchi/15",
		"/api/basho/202607/torikumi/Juryo/15",
		// The zero object of an unpublished tournament.
		"/api/basho/202611",
		// One page of the wrestler index.
		"/api/rikishis?intai=true&limit=1000&skip=0",
	}},
	{"osada", osada.DefaultBaseURL, []string{
		"/api/v1/torikumi.json",
		"/api/v1/banzuke.json",
	}},
}

// FixtureRequests is the number of requests RecordFixtures makes.
func FixtureRequests() int {
	n := 0
	for _, s := range fixtureSet {
		n += len(s.paths)
	}
	return n
}

// RecordFixtures fetches fixtureSet into out/<source>/ with the file names of
// fixture.Name. Any failure stops the run: fixtures must be complete.
func RecordFixtures(ctx context.Context, http httpx.Getter, out string, log io.Writer) error {
	for _, s := range fixtureSet {
		dir := filepath.Join(out, s.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, p := range s.paths {
			resp, err := http.Get(ctx, s.base+p, httpx.Validators{})
			if err != nil {
				return err
			}
			if !resp.OK() {
				return fmt.Errorf("%s%s: HTTP %d", s.base, p, resp.Status)
			}
			name := filepath.Join(dir, fixture.Name(p))
			if err := writeFileAtomic(name, resp.Body); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(log, "%-60s %8d bytes\n", name, len(resp.Body))
		}
	}
	return nil
}
