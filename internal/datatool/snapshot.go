package datatool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"time"

	"github.com/thorstenpfister/basho/internal/atomicfile"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// SnapshotSource is what the builder reads; *sumoapi.Client implements it.
type SnapshotSource interface {
	Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error)
	Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error)
	Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error)
	RikishiIndex(ctx context.Context) ([]domain.Rikishi, error)
}

// snapshotDivisions are the divisions a snapshot holds (SPEC §4.3).
var snapshotDivisions = []domain.Division{domain.DivMakuuchi, domain.DivJuryo}

// Requests per tournament: metadata, two banzuke, 15 days of two divisions.
// The index is an estimate: about 9,100 wrestlers in pages of PageSize.
const (
	requestsPerTournament = 1 + 2 + 2*domain.BashoDays
	indexRequests         = 9200/sumoapi.PageSize + 1
)

// SnapshotConfig configures a snapshot update.
type SnapshotConfig struct {
	Dir       string         // data/snapshot
	From, To  domain.BashoID // zero From: 196001; zero To: the newest finished tournament
	Today     domain.Date
	Generator string        // basho-data version, for the manifest
	Interval  time.Duration // time between requests, for the dry-run estimate
	DryRun    bool

	// For the fixture snapshot built from recorded responses (SPEC §13):
	Only    []domain.BashoID // just these tournaments instead of From–To
	Partial bool             // skip banzuke and bout lists that are not recorded
}

// Plan lists the tournaments an update would fetch.
type Plan struct {
	Tournaments []domain.BashoID
	Requests    int
}

// PlanSnapshot works out which tournaments are missing from the manifest. It
// makes no requests: finished means estimated end + 2 days, checked against
// the real dates and yusho when the tournament is fetched.
func PlanSnapshot(m snapshot.Manifest, cfg SnapshotConfig) Plan {
	from, to := cfg.From, cfg.To
	if from.IsZero() || from.Compare(domain.FirstBasho) < 0 {
		from = domain.FirstBasho
	}
	if to.IsZero() {
		to = domain.MostRecentTournament(cfg.Today)
	}
	candidates := cfg.Only
	if candidates == nil {
		for id := from; id.Compare(to) <= 0; id = id.Next() {
			candidates = append(candidates, id)
		}
	}
	var p Plan
	for _, id := range candidates {
		if m.Has(id) || id.Held() == domain.NotHeld || cfg.Today.DaysSince(id.EstimatedEnd()) < domain.FinalMargin {
			continue
		}
		p.Tournaments = append(p.Tournaments, id)
	}
	p.Requests = len(p.Tournaments) * requestsPerTournament
	if len(p.Tournaments) > 0 {
		p.Requests += indexRequests
	}
	return p
}

// UpdateSnapshot adds the finished tournaments missing from the snapshot in
// cfg.Dir, then rewrites the wrestler index and the manifest. Without new
// tournaments it makes no requests, except one metadata request for a
// tournament past its estimated end that is not final yet. The manifest is
// written after every tournament, so an interrupted run keeps what it
// finished; the caller's work cache makes the rest cheap to redo.
func UpdateSnapshot(ctx context.Context, src SnapshotSource, cfg SnapshotConfig, log io.Writer) (Plan, error) {
	m, err := snapshot.ReadManifest(os.DirFS(cfg.Dir))
	if err != nil {
		return Plan{}, err
	}
	plan := PlanSnapshot(m, cfg)
	_, _ = fmt.Fprintf(log, "snapshot: %d tournaments to add, up to %d requests, about %s at %s per request\n",
		len(plan.Tournaments), plan.Requests, (time.Duration(plan.Requests) * cfg.Interval).Round(time.Minute), cfg.Interval)
	if cfg.DryRun {
		for _, id := range plan.Tournaments {
			_, _ = fmt.Fprintln(log, "  ", id)
		}
		return plan, nil
	}

	m.Schema, m.Generator, m.Source = snapshot.Schema, cfg.Generator, "sumo-api.com"
	if m.Files == nil {
		m.Files = map[string]string{}
	}
	for i, id := range plan.Tournaments {
		t, err := fetchTournament(ctx, src, id, cfg.Today, cfg.Partial, log)
		switch {
		case errors.Is(err, errNotFinal), errors.Is(err, source.ErrNotYetAvailable):
			_, _ = fmt.Fprintf(log, "%s: skipped (%v)\n", id, err)
			continue
		case err != nil:
			return plan, fmt.Errorf("%s: %w", id, err)
		}
		// Oddities in the source are logged, not fatal: verify decides,
		// with known-issues.txt for the accepted ones.
		for _, p := range verifyTournament(t) {
			_, _ = fmt.Fprintf(log, "%s: %s\n", id, p)
		}
		data, err := encodeChecked(t)
		if err != nil {
			return plan, fmt.Errorf("%s: %w", id, err)
		}
		if err := writeSnapshotFile(cfg.Dir, snapshot.TournamentPath(id), data, &m); err != nil {
			return plan, err
		}
		m.Basho = append(m.Basho, id)
		slices.SortFunc(m.Basho, domain.BashoID.Compare)
		m.IndexThrough = domain.BashoID{} // stale until rewritten, even for a tournament older than the newest
		m.GeneratedAt = time.Now().UTC().Truncate(time.Second)
		if err := writeManifest(cfg.Dir, m); err != nil {
			return plan, err
		}
		_, _ = fmt.Fprintf(log, "%s: %d bouts, %d bytes (%d/%d)\n", id, len(t.Bouts), len(data), i+1, len(plan.Tournaments))
	}
	if m.IndexThrough == m.Newest() { // also catches a run interrupted before the index or filling a gap
		return plan, nil
	}
	return plan, updateIndex(ctx, src, cfg.Dir, &m, log)
}

var errNotFinal = errors.New("not final yet")

// fetchTournament reads one tournament and checks that it is final: end date
// + 2 days passed and a yusho winner present (SPEC §7.2). Being final, an
// empty bout list is a gap in the source, not a day still to come, and is
// logged; so are records repaired by fillFromBouts.
func fetchTournament(ctx context.Context, src SnapshotSource, id domain.BashoID, today domain.Date, partial bool, log io.Writer) (snapshot.Tournament, error) {
	b, err := src.Basho(ctx, id)
	if err != nil {
		return snapshot.Tournament{}, err
	}
	if !domain.IsFinal(b.End, len(b.Yusho) > 0, today) {
		return snapshot.Tournament{}, errNotFinal
	}
	t := snapshot.Tournament{Basho: b, Banzuke: map[domain.Division][]domain.BanzukeEntry{}}
	unrecorded := func(err error) bool { return partial && errors.Is(err, source.ErrNotFound) }
	for _, div := range snapshotDivisions {
		banzuke, err := src.Banzuke(ctx, id, div)
		switch {
		case unrecorded(err):
		case err != nil:
			return snapshot.Tournament{}, err
		default:
			t.Banzuke[div] = banzuke
		}
		for day := 1; day <= domain.BashoDays; day++ {
			bouts, err := src.Torikumi(ctx, id, div, day)
			switch {
			case unrecorded(err):
			case errors.Is(err, source.ErrNotYetAvailable):
				_, _ = fmt.Fprintf(log, "%s: %s day %d: no bouts in the source\n", id, div, day)
			case err != nil:
				return snapshot.Tournament{}, err
			}
			t.Bouts = append(t.Bouts, bouts...)
		}
	}
	snapshot.SortBouts(t.Bouts)
	for _, f := range fillFromBouts(t) {
		_, _ = fmt.Fprintf(log, "%s: %s\n", id, f)
	}
	return t, nil
}

// fillFromBouts gives a record's pending day the result of that day's
// decided bout, as sumo-api sometimes misses one in a finished tournament
// (202507 Juryo day 15). A record naming another opponent is left for verify
// to report. It changes t's banzuke and describes each repair.
func fillFromBouts(t snapshot.Tournament) []string {
	records := recordsByID(t)
	var filled []string
	fill := func(b domain.Bout, me, them domain.BoutSide, won bool) {
		e, ok := records[me.RikishiID]
		if !ok {
			return
		}
		if r := e.Results[b.Day-1]; r.Outcome != domain.Pending || r.OpponentID != 0 && r.OpponentID != them.RikishiID {
			return
		}
		o := domain.Loss
		switch {
		case won && b.Kimarite == "fusen":
			o = domain.FusenWin
		case won:
			o = domain.Win
		case b.Kimarite == "fusen":
			o = domain.FusenLoss
		}
		e.Results[b.Day-1] = domain.DayResult{Outcome: o, OpponentID: them.RikishiID, Kimarite: b.Kimarite}
		filled = append(filled, fmt.Sprintf("day %d: %s's pending record filled from the bout: %s", b.Day, e.Shikona.En, o))
	}
	for _, b := range t.Bouts {
		if !b.IsPlayoff && b.Winner != 0 {
			fill(b, b.East, b.West, b.Winner == domain.East)
			fill(b, b.West, b.East, b.Winner == domain.West)
		}
	}
	return filled
}

// recordsByID points at t's banzuke entries, so changes go into t.
func recordsByID(t snapshot.Tournament) map[domain.RikishiID]*domain.BanzukeEntry {
	records := map[domain.RikishiID]*domain.BanzukeEntry{}
	for _, entries := range t.Banzuke {
		for i := range entries {
			records[entries[i].RikishiID] = &entries[i]
		}
	}
	return records
}

// encodeChecked encodes t and makes sure decoding gives it back, so a format
// bug can never reach a committed file.
func encodeChecked(t snapshot.Tournament) ([]byte, error) {
	data, err := snapshot.EncodeTournament(t)
	if err != nil {
		return nil, err
	}
	back, err := snapshot.DecodeTournament(data)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(back, t) {
		return nil, errors.New("encoding does not round-trip")
	}
	return data, nil
}

// updateIndex rewrites the wrestler index with everyone who appears in the
// snapshot's banzuke or bouts.
func updateIndex(ctx context.Context, src SnapshotSource, dir string, m *snapshot.Manifest, log io.Writer) error {
	seen := map[domain.RikishiID]bool{}
	for _, id := range m.Basho {
		t, err := snapshot.ReadTournament(os.DirFS(dir), id)
		if err != nil {
			return err
		}
		for _, r := range t.RikishiIDs() {
			seen[r] = true
		}
	}
	all, err := src.RikishiIndex(ctx)
	if err != nil {
		return fmt.Errorf("wrestler index: %w", err)
	}
	var index []domain.Rikishi
	for _, r := range all {
		if seen[r.ID] {
			index = append(index, r)
		}
	}
	data, err := snapshot.EncodeRikishi(index)
	if err != nil {
		return err
	}
	if err := writeSnapshotFile(dir, snapshot.RikishiPath, data, m); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(log, "wrestler index: %d of %d wrestlers, %d bytes\n", len(index), len(all), len(data))
	m.IndexThrough = m.Newest()
	return writeManifest(dir, *m)
}

func writeManifest(dir string, m snapshot.Manifest) error {
	data, err := snapshot.EncodeManifest(m)
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(dir, snapshot.ManifestPath), data)
}

// writeSnapshotFile writes one file and records its checksum in m.
func writeSnapshotFile(dir, path string, data []byte, m *snapshot.Manifest) error {
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	if err := atomicfile.Write(full, data); err != nil {
		return err
	}
	m.Files[path] = snapshot.Checksum(data)
	return nil
}
