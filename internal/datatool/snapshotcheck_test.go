package datatool

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

func builtSnapshot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := SnapshotConfig{Dir: dir, From: b(2026, time.May), To: b(2026, time.July), Today: today}
	if _, err := UpdateSnapshot(t.Context(), &fakeSource{}, cfg, io.Discard); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestVerifySnapshot(t *testing.T) {
	dir := builtSnapshot(t)
	if problems, err := VerifySnapshot(os.DirFS(dir)); err != nil || len(problems) != 0 {
		t.Fatalf("a fresh snapshot: %v, %v", problems, err)
	}

	// A tampered file and a stray one.
	path := filepath.Join(dir, snapshot.TournamentPath(b(2026, time.July)))
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "basho", "stray.json.zst"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	problems, err := VerifySnapshot(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(problems, "\n")
	for _, want := range []string{"202607.json.zst: checksum does not match", "stray.json.zst: not in the manifest"} {
		if !strings.Contains(got, want) {
			t.Errorf("problems lack %q:\n%s", want, got)
		}
	}
}

func TestKnownIssues(t *testing.T) {
	dir := builtSnapshot(t)
	path := filepath.Join(dir, snapshot.TournamentPath(b(2026, time.July)))
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, 0), 0o644); err != nil {
		t.Fatal(err)
	}
	known := "# accepted for the test\nbasho/202607.json.zst: checksum does not match the manifest\n202607: zstd: unexpected EOF\n\n# fixed upstream\n202605: gone\n"
	if err := os.WriteFile(filepath.Join(dir, KnownIssuesPath), []byte(known), 0o644); err != nil {
		t.Fatal(err)
	}
	problems, err := VerifySnapshot(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"known-issues.txt: no longer occurs: 202605: gone"}; !slices.Equal(problems, want) {
		t.Errorf("problems = %q, want %q", problems, want)
	}
}

func TestVerifyTournament(t *testing.T) {
	src := &fakeSource{}
	tour, err := fetchTournament(t.Context(), src, b(2026, time.July), today, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if p := verifyTournament(tour); len(p) != 0 {
		t.Fatalf("consistent tournament: %v", p)
	}
	mak := tour.Banzuke[domain.DivMakuuchi]
	mak[0].Results[2].Outcome = domain.Loss       // the bout says a win
	mak[1].Results[4].OpponentID = 99             // the bout says the other wrestler
	tour.Bouts = tour.Bouts[1:]                   // day 1 Makuuchi bout gone: both records lack it
	tour.Basho.Yusho[domain.DivJuryo] = 12345     // not on the Juryo banzuke
	tour.Basho.Yusho[domain.DivMakushita] = 54321 // lower division: not checked
	want := []string{
		"Juryo yusho winner 12345 is not on its banzuke",
		"day 1: A's record says win without a bout",
		"day 1: B's record says loss without a bout",
		"day 3: A's record says loss, the bout a win",
		"day 5: B's record has opponent 99, the bout 11",
	}
	got := verifyTournament(tour)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSnapshotStats(t *testing.T) {
	dir := builtSnapshot(t)
	var out strings.Builder
	if err := SnapshotStats(os.DirFS(dir), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"202605", "202607", "2 tournaments (202605 to 202607), 60 bouts", "index: 4 wrestlers"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stats lack %q:\n%s", want, out.String())
		}
	}
}
