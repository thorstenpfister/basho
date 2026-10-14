package datatool

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

// fakeSource makes up a consistent tournament for any id: two wrestlers per
// division, who meet every day. It counts calls.
type fakeSource struct {
	calls   int
	unfinal map[domain.BashoID]bool // no yusho yet

	// Gaps like sumo-api's: no bouts and pending records on emptyDay; pending
	// records but a decided bout on blankDay.
	emptyDay, blankDay int
}

func (f *fakeSource) Basho(_ context.Context, id domain.BashoID) (domain.Basho, error) {
	f.calls++
	start := id.EstimatedStart()
	b := domain.Basho{ID: id, Start: start, End: start.AddDays(14), Yusho: map[domain.Division]domain.RikishiID{}}
	if !f.unfinal[id] {
		b.Yusho[domain.DivMakuuchi], _ = wrestlers(domain.DivMakuuchi)
	}
	return b, nil
}

func wrestlers(div domain.Division) (domain.RikishiID, domain.RikishiID) {
	return domain.RikishiID(div*10 + 1), domain.RikishiID(div*10 + 2)
}

func (f *fakeSource) Banzuke(_ context.Context, _ domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	f.calls++
	a, b := wrestlers(div)
	title := domain.Maegashira
	if div == domain.DivJuryo {
		title = domain.Juryo
	}
	ea := domain.BanzukeEntry{RikishiID: a, Shikona: domain.Name{En: "A"}, Rank: domain.Rank{Title: title, Number: 1, Side: domain.East}}
	eb := domain.BanzukeEntry{RikishiID: b, Shikona: domain.Name{En: "B"}, Rank: domain.Rank{Title: title, Number: 1, Side: domain.West}}
	for d := range domain.BashoDays {
		ea.Results[d] = domain.DayResult{Outcome: domain.Win, OpponentID: b, Kimarite: "yorikiri"}
		eb.Results[d] = domain.DayResult{Outcome: domain.Loss, OpponentID: a, Kimarite: "yorikiri"}
		switch d + 1 {
		case f.emptyDay:
			ea.Results[d], eb.Results[d] = domain.DayResult{}, domain.DayResult{}
		case f.blankDay:
			ea.Results[d], eb.Results[d] = domain.DayResult{OpponentID: b}, domain.DayResult{OpponentID: a}
		}
	}
	return []domain.BanzukeEntry{ea, eb}, nil
}

func (f *fakeSource) Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	entries, _ := f.Banzuke(ctx, id, div) // counts this call
	if day == f.emptyDay {
		return nil, fmt.Errorf("torikumi day %d: %w", day, source.ErrNotYetAvailable)
	}
	side := func(e domain.BanzukeEntry) domain.BoutSide {
		return domain.BoutSide{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank}
	}
	return []domain.Bout{{
		Basho: id, Division: div, Day: day, MatchNo: 1, East: side(entries[0]), West: side(entries[1]),
		Winner: domain.East, Kimarite: "yorikiri",
	}}, nil
}

func (f *fakeSource) RikishiIndex(context.Context) ([]domain.Rikishi, error) {
	f.calls += indexRequests
	return []domain.Rikishi{{ID: 11}, {ID: 12}, {ID: 21}, {ID: 22}, {ID: 99, Name: domain.Name{En: "Never sekitori"}}}, nil
}

var today = domain.NewDate(2026, time.October, 13)

func b(y int, m time.Month) domain.BashoID { return domain.BashoID{Year: y, Month: m} }

func TestPlanSnapshot(t *testing.T) {
	cfg := SnapshotConfig{From: b(2020, time.January), To: b(2020, time.September), Today: today}
	p := PlanSnapshot(snapshot.Manifest{Basho: []domain.BashoID{b(2020, time.March)}}, cfg)
	want := []domain.BashoID{b(2020, time.January), b(2020, time.July), b(2020, time.September)} // 202003 present, 202005 cancelled
	if !slices.Equal(p.Tournaments, want) || p.Requests != 3*33+indexRequests {
		t.Errorf("plan = %v, %d requests", p.Tournaments, p.Requests)
	}
	// Defaults: from 196001 to the newest finished tournament; Aki 2026 ended
	// 2026-09-27, Kyushu has not started.
	all := PlanSnapshot(snapshot.Manifest{}, SnapshotConfig{Today: today})
	if all.Tournaments[0] != domain.FirstBasho || all.Tournaments[len(all.Tournaments)-1] != b(2026, time.September) {
		t.Errorf("default range %v … %v", all.Tournaments[0], all.Tournaments[len(all.Tournaments)-1])
	}
	early := PlanSnapshot(snapshot.Manifest{}, SnapshotConfig{Today: domain.NewDate(2026, time.September, 28)}).Tournaments
	if last := early[len(early)-1]; last != b(2026, time.July) {
		t.Errorf("one day after Aki 2026 ended, the plan ends at %v", last)
	}
}

func TestUpdateSnapshot(t *testing.T) {
	dir := t.TempDir()
	src := &fakeSource{unfinal: map[domain.BashoID]bool{b(2026, time.September): true}}
	cfg := SnapshotConfig{Dir: dir, From: b(2026, time.May), Today: today, Generator: "test", Interval: time.Second}

	// Dry run: a plan, no requests, no files.
	var log strings.Builder
	if _, err := UpdateSnapshot(t.Context(), src, SnapshotConfig{Dir: dir, From: cfg.From, Today: today, DryRun: true, Interval: time.Second}, &log); err != nil {
		t.Fatal(err)
	}
	if src.calls != 0 || !strings.Contains(log.String(), "3 tournaments to add, up to 109 requests") {
		t.Errorf("dry run: %d calls, log %q", src.calls, log.String())
	}
	if _, err := os.Stat(filepath.Join(dir, snapshot.ManifestPath)); err == nil {
		t.Error("dry run wrote a manifest")
	}

	if _, err := UpdateSnapshot(t.Context(), src, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	// 202605 and 202607 in full, 202609 only its metadata (no yusho yet), the index.
	if want := 2*requestsPerTournament + 1 + indexRequests; src.calls != want {
		t.Errorf("first update: %d calls, want %d", src.calls, want)
	}
	m, err := snapshot.ReadManifest(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.Basho, []domain.BashoID{b(2026, time.May), b(2026, time.July)}) || len(m.Files) != 3 || m.Generator != "test" {
		t.Errorf("manifest = %+v", m)
	}
	for path, sum := range m.Files {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || snapshot.Checksum(data) != sum {
			t.Errorf("%s: checksum mismatch (%v)", path, err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, snapshot.RikishiPath))
	if index, err := snapshot.DecodeRikishi(data); err != nil || len(index) != 4 {
		t.Errorf("index = %v, %v; want the four sekitori only", index, err)
	}

	// Nothing new: only the still unfinished tournament's metadata is asked for.
	src.calls = 0
	if _, err := UpdateSnapshot(t.Context(), src, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	if src.calls != 1 {
		t.Errorf("second update: %d calls, want 1 (202609's metadata)", src.calls)
	}
	// A run interrupted after its last tournament but before the index:
	// the next run writes the index although nothing is new.
	m.IndexThrough = domain.BashoID{}
	if err := writeManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	src.calls = 0
	if _, err := UpdateSnapshot(t.Context(), src, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	if want := 1 + indexRequests; src.calls != want {
		t.Errorf("after an interrupted index: %d calls, want %d", src.calls, want)
	}
	// Once it is final, it is added and the index rewritten.
	src.calls, src.unfinal = 0, nil
	if _, err := UpdateSnapshot(t.Context(), src, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	if want := requestsPerTournament + indexRequests; src.calls != want {
		t.Errorf("third update: %d calls, want %d", src.calls, want)
	}
	if m, _ := snapshot.ReadManifest(os.DirFS(dir)); m.Newest() != b(2026, time.September) {
		t.Errorf("newest = %v", m.Newest())
	}
	// A tournament older than the newest, like one skipped before, rewrites
	// the index too.
	src.calls = 0
	if _, err := UpdateSnapshot(t.Context(), src, SnapshotConfig{Dir: dir, From: b(2026, time.March), Today: today}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if want := requestsPerTournament + indexRequests; src.calls != want {
		t.Errorf("filling a gap: %d calls, want %d", src.calls, want)
	}
}

func TestFetchTournamentSourceGaps(t *testing.T) {
	src := &fakeSource{emptyDay: 7, blankDay: 15}
	var log strings.Builder
	tour, err := fetchTournament(t.Context(), src, b(2026, time.July), today, false, &log)
	if err != nil {
		t.Fatal(err)
	}
	if p := verifyTournament(tour); len(p) != 0 {
		t.Errorf("after the repair: %v", p)
	}
	mak := tour.Banzuke[domain.DivMakuuchi]
	if r := mak[0].Results[14]; r != (domain.DayResult{Outcome: domain.Win, OpponentID: 12, Kimarite: "yorikiri"}) {
		t.Errorf("day 15 filled as %+v", r)
	}
	if mak[1].Results[14].Outcome != domain.Loss || mak[0].Results[6].Outcome != domain.Pending {
		t.Errorf("B's day 15 %v, A's day 7 %v", mak[1].Results[14].Outcome, mak[0].Results[6].Outcome)
	}
	if n := len(tour.Bouts); n != 2*(domain.BashoDays-1) {
		t.Errorf("%d bouts, want 28", n)
	}
	for _, want := range []string{
		"202607: Makuuchi day 7: no bouts in the source",
		"202607: Juryo day 7: no bouts in the source",
		"202607: day 15: A's pending record filled from the bout: win",
		"202607: day 15: B's pending record filled from the bout: loss",
	} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("log lacks %q:\n%s", want, log.String())
		}
	}

	// A pending day naming another opponent is not filled: verify reports it.
	tour.Banzuke[domain.DivJuryo][0].Results[14] = domain.DayResult{OpponentID: 99}
	if f := fillFromBouts(tour); len(f) != 0 {
		t.Errorf("filled %v", f)
	}
}

// failingSource stops after a number of tournaments, like an interrupted run.
type failingSource struct {
	fakeSource
	left int
}

func (f *failingSource) Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error) {
	if f.left == 0 {
		return domain.Basho{}, source.ErrUpstream
	}
	f.left--
	return f.fakeSource.Basho(ctx, id)
}

func TestUpdateSnapshotKeepsFinishedTournamentsOnFailure(t *testing.T) {
	dir := t.TempDir()
	cfg := SnapshotConfig{Dir: dir, From: b(2026, time.March), To: b(2026, time.July), Today: today}
	if _, err := UpdateSnapshot(t.Context(), &failingSource{left: 1}, cfg, io.Discard); err == nil {
		t.Fatal("no error from an interrupted run")
	}
	m, err := snapshot.ReadManifest(os.DirFS(dir))
	if err != nil || !slices.Equal(m.Basho, []domain.BashoID{b(2026, time.March)}) {
		t.Fatalf("after the failure: %v, %v; want 202603 kept", m.Basho, err)
	}
	plan := PlanSnapshot(m, cfg)
	if !slices.Equal(plan.Tournaments, []domain.BashoID{b(2026, time.May), b(2026, time.July)}) {
		t.Errorf("resume plan = %v", plan.Tournaments)
	}
}
