package datatool

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
	"github.com/thorstenpfister/basho/internal/source/osada"
)

var (
	kyushu2026 = domain.BashoID{Year: 2026, Month: time.November}
	aki2026    = domain.BashoID{Year: 2026, Month: time.September}
)

// recordedOsada serves an osada recording from dir.
func recordedOsada(dir string) *osada.Client {
	return osada.New(fixture.Getter{Dir: dir, Base: osada.DefaultBaseURL}, osada.DefaultBaseURL)
}

// kyushuFinal is osada at 19:10 on Kyushu 2026's day 15, after the playoff.
var kyushuFinal = filepath.Join("..", "..", "testdata", "live", "20261122", "1910", "osada")

// kyushuWithout is Kyushu 2026 from the committed snapshot as sumo-api has
// it: without playoff bouts, whether or not they are committed by now.
func kyushuWithout(t *testing.T) snapshot.Tournament {
	t.Helper()
	tour := readTournament(t, filepath.Join("..", "..", "data", "snapshot"), kyushu2026)
	tour.Bouts = slices.DeleteFunc(tour.Bouts, func(b domain.Bout) bool { return b.IsPlayoff })
	return tour
}

func readTournament(t *testing.T, dir string, id domain.BashoID) snapshot.Tournament {
	t.Helper()
	tour, err := snapshot.ReadTournament(os.DirFS(dir), id)
	if err != nil {
		t.Fatal(err)
	}
	return tour
}

func playoffBouts(t snapshot.Tournament) []domain.Bout {
	var out []domain.Bout
	for _, b := range t.Bouts {
		if b.IsPlayoff {
			out = append(out, b)
		}
	}
	return out
}

func TestAddPlayoffsKyushu(t *testing.T) {
	tour := kyushuWithout(t)
	before := verifyTournament(tour)
	regular := len(tour.Bouts)

	n, err := addPlayoffs(t.Context(), recordedOsada(kyushuFinal), &tour)
	if err != nil || n != 1 || len(tour.Bouts) != regular+1 {
		t.Fatalf("added %d, %v; %d bouts, want %d", n, err, len(tour.Bouts), regular+1)
	}
	p := playoffBouts(tour)[0]
	if p.Division != domain.DivMakuuchi || p.Day != 15 || p.East.RikishiID != 8850 || p.East.Shikona.En != "Onosato" ||
		p.West.Shikona.En != "Wakamotoharu" || p.Winner != domain.East || p.Kimarite != "watashikomi" {
		t.Errorf("playoff %+v", p)
	}
	// Sorted by day, division, playoff: after the day's last Makuuchi bout.
	i := slices.IndexFunc(tour.Bouts, func(b domain.Bout) bool { return b.IsPlayoff })
	if prev := tour.Bouts[i-1]; prev.Day != 15 || prev.Division != domain.DivMakuuchi || prev.IsPlayoff {
		t.Errorf("the playoff follows %+v", prev)
	}
	if i+1 < len(tour.Bouts) && tour.Bouts[i+1].Division == domain.DivMakuuchi && tour.Bouts[i+1].Day == 15 {
		t.Error("a regular Makuuchi bout of day 15 sorts after the playoff")
	}
	if _, err := encodeChecked(tour); err != nil {
		t.Error(err)
	}
	if after := verifyTournament(tour); !slices.Equal(after, before) {
		t.Errorf("verify: %v, before %v", after, before)
	}
	// Again: osada's playoffs replace the stored ones, nothing doubles.
	if n, err := addPlayoffs(t.Context(), recordedOsada(kyushuFinal), &tour); err != nil || n != 1 || len(tour.Bouts) != regular+1 {
		t.Errorf("twice: %d, %v, %d bouts", n, err, len(tour.Bouts))
	}
}

// Aki 2026's Juryo playoff: three wrestlers at 11-4, two bouts.
func TestAddPlayoffsJuryo(t *testing.T) {
	tour := readTournament(t, filepath.Join("..", "..", "testdata", "snapshot"), aki2026)
	n, err := addPlayoffs(t.Context(), recordedOsada(filepath.Join("..", "..", "testdata", "osada")), &tour)
	if err != nil || n != 2 {
		t.Fatalf("added %d, %v", n, err)
	}
	ps := playoffBouts(tour)
	if ps[0].Division != domain.DivJuryo || ps[0].MatchNo >= ps[1].MatchNo {
		t.Errorf("playoffs %+v", ps)
	}
	if w := ps[1]; w.Winner == 0 || (w.Winner == domain.East && w.East.RikishiID != 66) || (w.Winner == domain.West && w.West.RikishiID != 66) {
		t.Errorf("the last playoff bout's winner is not the yusho (Kitanowaka, 66): %+v", w)
	}
	if _, err := encodeChecked(tour); err != nil {
		t.Error(err)
	}
}

func TestAddPlayoffsRefuses(t *testing.T) {
	kyushu := kyushuWithout(t)
	regular := len(kyushu.Bouts)

	// osada has moved on to another tournament.
	if _, err := addPlayoffs(t.Context(), recordedOsada(filepath.Join("..", "..", "testdata", "osada")), &kyushu); !errors.Is(err, errOtherTournament) {
		t.Errorf("another tournament: %v", err)
	}
	// A side that is not among the leaders: a wrong rank match must not slip in.
	for i, e := range kyushu.Banzuke[domain.DivMakuuchi] {
		if e.RikishiID == 8850 {
			kyushu.Banzuke[domain.DivMakuuchi][i].Results[0].Outcome = domain.Loss
			kyushu.Banzuke[domain.DivMakuuchi][i].Results[1].Outcome = domain.Loss
		}
	}
	if _, err := addPlayoffs(t.Context(), recordedOsada(kyushuFinal), &kyushu); err == nil || !strings.Contains(err.Error(), "not among the leaders") {
		t.Errorf("not a leader: %v", err)
	}
	if len(kyushu.Bouts) != regular {
		t.Errorf("a refused tournament changed: %d bouts", len(kyushu.Bouts))
	}
	// osada unreachable.
	down := sourceFunc(func(context.Context) (source.Current, error) { return source.Current{}, source.ErrUpstream })
	if _, err := addPlayoffs(t.Context(), down, &kyushu); !errors.Is(err, source.ErrUpstream) {
		t.Errorf("unreachable: %v", err)
	}
}

type sourceFunc func(context.Context) (source.Current, error)

func (f sourceFunc) Current(ctx context.Context) (source.Current, error) { return f(ctx) }

func TestOnceCurrent(t *testing.T) {
	calls := 0
	o := &OnceCurrent{Next: sourceFunc(func(context.Context) (source.Current, error) {
		calls++
		return source.Current{Basho: kyushu2026}, nil
	})}
	for range 3 {
		if cur, err := o.Current(t.Context()); err != nil || cur.Basho != kyushu2026 {
			t.Fatal(cur.Basho, err)
		}
	}
	if calls != 1 {
		t.Errorf("%d calls", calls)
	}
}

func TestRefreshPlayoffs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join("..", "..", "data", "snapshot")
	m, err := snapshot.ReadManifest(os.DirFS(src))
	if err != nil {
		t.Fatal(err)
	}
	// A two-file snapshot: the manifest's checksums for the rest are kept.
	path := snapshot.TournamentPath(kyushu2026)
	data, err := snapshot.EncodeTournament(kyushuWithout(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshotFile(dir, path, data, &m); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(dir, m); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	if err := RefreshPlayoffs(t.Context(), recordedOsada(kyushuFinal), dir, kyushu2026, "test", &log); err != nil {
		t.Fatal(err)
	}
	after, err := snapshot.ReadManifest(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if after.Files[path] == m.Files[path] || after.Generator != "test" || !strings.Contains(log.String(), "1 playoff bout from osada") {
		t.Errorf("manifest %s / %s, log %q", after.Files[path], after.Generator, log.String())
	}
	if got := playoffBouts(readTournament(t, dir, kyushu2026)); len(got) != 1 {
		t.Errorf("%d playoff bouts written", len(got))
	}
	log.Reset()
	if err := RefreshPlayoffs(t.Context(), recordedOsada(kyushuFinal), dir, kyushu2026, "test", &log); err != nil || !strings.Contains(log.String(), "unchanged") {
		t.Errorf("second run: %v, %q", err, log.String())
	}
	if err := RefreshPlayoffs(t.Context(), recordedOsada(kyushuFinal), dir, aki2026, "test", io.Discard); err == nil {
		t.Error("a tournament not in the snapshot")
	}
}

func TestLogPlayoffs(t *testing.T) {
	aki := recordedOsada(filepath.Join("..", "..", "testdata", "osada")) // serves Aki 2026
	for _, tt := range []struct {
		newest bool
		want   string
	}{
		{false, ""},
		{true, "202611: no playoff bouts from osada: osada serves another tournament (202609)\n"},
	} {
		tour := kyushuWithout(t)
		var log strings.Builder
		logPlayoffs(t.Context(), aki, &tour, tt.newest, &log)
		if log.String() != tt.want {
			t.Errorf("newest %v: %q", tt.newest, log.String())
		}
	}
	tour := kyushuWithout(t)
	var log strings.Builder
	logPlayoffs(t.Context(), recordedOsada(kyushuFinal), &tour, true, &log)
	if log.String() != "202611: 1 playoff bout from osada\n" {
		t.Errorf("added: %q", log.String())
	}
}
