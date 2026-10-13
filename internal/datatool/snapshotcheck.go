package datatool

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

// KnownIssuesPath lists problems in the source data that verify accepts:
// one exact problem line per entry, each after a "#" comment saying why.
const KnownIssuesPath = "known-issues.txt"

// VerifySnapshot checks the snapshot in fsys (SPEC §4.3) and returns every
// problem found, minus the known issues (an entry that no longer occurs is a
// problem too): checksums and schema, every bout appearing in both
// wrestlers' banzuke records with matching outcomes, every recorded result
// having its bout, yusho winners on the banzuke, and the wrestler index
// covering everyone.
func VerifySnapshot(fsys fs.FS) ([]string, error) {
	m, err := snapshot.ReadManifest(fsys)
	if err != nil {
		return nil, err
	}
	var problems []string
	report := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	for _, p := range slices.Sorted(maps.Keys(m.Files)) {
		data, err := fs.ReadFile(fsys, p)
		switch {
		case err != nil:
			report("%s: %v", p, err)
		case snapshot.Checksum(data) != m.Files[p]:
			report("%s: checksum does not match the manifest", p)
		}
	}
	onDisk, err := fs.Glob(fsys, path.Join(path.Dir(snapshot.TournamentPath(domain.FirstBasho)), "*"))
	if err != nil {
		return nil, err
	}
	for _, p := range onDisk {
		if _, ok := m.Files[p]; !ok {
			report("%s: not in the manifest", p)
		}
	}

	ids := map[domain.RikishiID]bool{}
	for _, id := range m.Basho {
		if _, ok := m.Files[snapshot.TournamentPath(id)]; !ok {
			report("%s: listed without a file", id)
			continue
		}
		t, err := snapshot.ReadTournament(fsys, id)
		if err != nil {
			report("%v", err)
			continue
		}
		for _, p := range verifyTournament(t) {
			report("%s: %s", id, p)
		}
		for _, r := range t.RikishiIDs() {
			ids[r] = true
		}
	}

	if len(m.Basho) > 0 {
		data, err := fs.ReadFile(fsys, snapshot.RikishiPath)
		if err != nil {
			return problems, err
		}
		index, err := snapshot.DecodeRikishi(data)
		if err != nil {
			report("%s: %v", snapshot.RikishiPath, err)
		}
		for _, r := range index {
			delete(ids, r.ID)
		}
		for _, id := range slices.Sorted(maps.Keys(ids)) {
			report("wrestler %d: not in the index", id)
		}
	}
	return withoutKnown(fsys, problems)
}

// withoutKnown removes the known issues from problems and reports entries
// that no longer occur, so the list cannot go stale.
func withoutKnown(fsys fs.FS, problems []string) ([]string, error) {
	data, err := fs.ReadFile(fsys, KnownIssuesPath)
	if errors.Is(err, fs.ErrNotExist) {
		return problems, nil
	}
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for line := range strings.Lines(string(data)) {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
			known[line] = false
		}
	}
	var out []string
	for _, p := range problems {
		if _, ok := known[p]; ok {
			known[p] = true
			continue
		}
		out = append(out, p)
	}
	for _, line := range slices.Sorted(maps.Keys(known)) {
		if !known[line] {
			out = append(out, KnownIssuesPath+": no longer occurs: "+line)
		}
	}
	return out, nil
}

// verifyTournament checks one tournament's bouts against its banzuke
// records. The builder runs it on fetched data, where every check is
// meaningful; on a decoded snapshot the opponent check only re-finds the
// stored exceptions, but outcomes are stored apart from bouts.
func verifyTournament(t snapshot.Tournament) []string {
	var problems []string
	report := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	records := map[domain.RikishiID]*domain.BanzukeEntry{}
	for _, entries := range t.Banzuke {
		for i := range entries {
			records[entries[i].RikishiID] = &entries[i]
		}
	}
	check := func(b domain.Bout, me, them domain.BoutSide, won bool) {
		e, ok := records[me.RikishiID]
		if !ok {
			return // a visitor from Makushita has no record here
		}
		r, want := e.Results[b.Day-1], "loss"
		if won {
			want = "win"
		}
		switch {
		case r.OpponentID != them.RikishiID:
			report("day %d: %s's record has opponent %d, the bout %d", b.Day, e.Shikona.En, r.OpponentID, them.RikishiID)
		case b.Winner == 0:
			report("day %d: %s vs %s has no winner", b.Day, b.East.Shikona.En, b.West.Shikona.En)
		case won && !r.Outcome.IsWin(), !won && !r.Outcome.IsLoss():
			report("day %d: %s's record says %s, the bout a %s", b.Day, e.Shikona.En, r.Outcome, want)
		}
	}
	for _, b := range t.Bouts {
		if !b.IsPlayoff {
			check(b, b.East, b.West, b.Winner == domain.East)
			check(b, b.West, b.East, b.Winner == domain.West)
		}
	}
	days := snapshot.BoutsByDay(t.Bouts)
	for id, e := range records {
		for d, r := range e.Results {
			if _, ok := days.Bout(id, d+1); !ok && (r.Outcome.IsWin() || r.Outcome.IsLoss()) {
				report("day %d: %s's record says %s without a bout", d+1, e.Shikona.En, r.Outcome)
			}
		}
	}
	for div, winner := range t.Basho.Yusho {
		if div != domain.DivMakuuchi && div != domain.DivJuryo {
			continue // lower divisions' winners are not in the snapshot
		}
		if !slices.ContainsFunc(t.Banzuke[div], func(e domain.BanzukeEntry) bool { return e.RikishiID == winner }) {
			report("%s yusho winner %d is not on its banzuke", div, winner)
		}
	}
	slices.Sort(problems)
	return problems
}

// SnapshotStats writes sizes and coverage of the snapshot in fsys.
func SnapshotStats(fsys fs.FS, w io.Writer) error {
	m, err := snapshot.ReadManifest(fsys)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	_, _ = fmt.Fprintln(tw, "tournament\tbouts\tMakuuchi\tJuryo\tbytes\t")
	var bouts, size int
	for _, id := range m.Basho {
		data, err := fs.ReadFile(fsys, snapshot.TournamentPath(id))
		if err != nil {
			return err
		}
		t, err := snapshot.DecodeTournament(data)
		if err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
		bouts += len(t.Bouts)
		size += len(data)
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t\n", id, len(t.Bouts), len(t.Banzuke[domain.DivMakuuchi]), len(t.Banzuke[domain.DivJuryo]), len(data))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if len(m.Basho) == 0 {
		_, err := fmt.Fprintln(w, "\nempty snapshot: run make snapshot")
		return err
	}
	var missing []domain.BashoID
	for id := m.Oldest(); id.Compare(m.Newest()) <= 0; id = id.Next() {
		if !m.Has(id) && id.Held() != domain.NotHeld {
			missing = append(missing, id)
		}
	}
	index, _ := fs.ReadFile(fsys, snapshot.RikishiPath)
	var wrestlers int
	if rs, err := snapshot.DecodeRikishi(index); err == nil {
		wrestlers = len(rs)
	}
	_, err = fmt.Fprintf(w, "\n%d tournaments (%v to %v), %d bouts, %.2f MB; index: %d wrestlers, %.2f MB; generated %s by %s\n",
		len(m.Basho), m.Oldest(), m.Newest(), bouts, float64(size)/1e6, wrestlers, float64(len(index))/1e6,
		m.GeneratedAt.Format("2006-01-02"), m.Generator)
	if err == nil && len(missing) > 0 {
		_, err = fmt.Fprintf(w, "missing in between: %v\n", missing)
	}
	return err
}
