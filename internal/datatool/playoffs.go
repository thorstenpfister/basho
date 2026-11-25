package datatool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

// PlayoffSource serves the current tournament; *osada.Client implements it.
// sumo-api's bout lists have no playoff bouts (SPEC §16 V2), osada's do, and
// osada keeps serving a tournament until the next banzuke is out, about two
// weeks after the tournament is final (SPEC §4.3).
type PlayoffSource interface {
	Current(ctx context.Context) (source.Current, error)
}

// playoffRequests is what one Current costs: osada's banzuke and bout list.
const playoffRequests = 2

// OnceCurrent asks Next once and answers every later call from that, so a
// run adding many tournaments costs osada two requests.
type OnceCurrent struct {
	Next PlayoffSource
	once sync.Once
	cur  source.Current
	err  error
}

// Current implements PlayoffSource.
func (o *OnceCurrent) Current(ctx context.Context) (source.Current, error) {
	o.once.Do(func() { o.cur, o.err = o.Next.Current(ctx) })
	return o.cur, o.err
}

// errOtherTournament: osada serves another tournament than the one asked for.
var errOtherTournament = errors.New("osada serves another tournament")

// addPlayoffs replaces t's playoff bouts with the decided ones osada lists
// on day 15 of t's tournament, and returns how many it added. A side is
// matched by its rank on t's banzuke (unique within a division); each must
// be among the division's leaders, so a wrong match cannot slip in. On any
// error t is left unchanged.
func addPlayoffs(ctx context.Context, src PlayoffSource, t *snapshot.Tournament) (int, error) {
	cur, err := src.Current(ctx)
	if err != nil {
		return 0, err
	}
	if cur.Basho != t.Basho.ID {
		return 0, fmt.Errorf("%w (%s)", errOtherTournament, cur.Basho)
	}
	var last source.CurrentDay
	for _, d := range cur.Days {
		if d.Day == domain.BashoDays {
			last = d
		}
	}
	var playoffs []domain.Bout
	for _, div := range snapshotDivisions {
		byRank := map[domain.Rank]domain.BanzukeEntry{}
		leaders := map[domain.RikishiID]bool{}
		top := -1
		for _, e := range t.Banzuke[div] {
			byRank[e.Rank] = e
			switch w := wins(e); {
			case w > top:
				top, leaders = w, map[domain.RikishiID]bool{e.RikishiID: true}
			case w == top:
				leaders[e.RikishiID] = true
			}
		}
		side := func(s source.CurrentSide) (domain.BoutSide, error) {
			e, ok := byRank[s.Rank]
			switch {
			case !ok:
				return domain.BoutSide{}, fmt.Errorf("%s playoff: no %s on the banzuke", div, s.Rank.Short())
			case !leaders[e.RikishiID]:
				return domain.BoutSide{}, fmt.Errorf("%s playoff: %s (%s) is not among the leaders", div, e.Shikona.En, s.Rank.Short())
			}
			return domain.BoutSide{RikishiID: e.RikishiID, Shikona: domain.Name{En: e.Shikona.En}, Rank: e.Rank}, nil
		}
		for _, b := range last.Bouts[div] {
			if !b.IsPlayoff {
				continue
			}
			if b.Winner == 0 {
				return 0, fmt.Errorf("%s playoff bout %d is not decided", div, b.MatchNo)
			}
			east, err := side(b.East)
			if err != nil {
				return 0, err
			}
			west, err := side(b.West)
			if err != nil {
				return 0, err
			}
			playoffs = append(playoffs, domain.Bout{
				Basho: t.Basho.ID, Division: div, Day: domain.BashoDays, MatchNo: b.MatchNo,
				East: east, West: west, Winner: b.Winner, Kimarite: b.Kimarite, IsPlayoff: true,
			})
		}
	}
	regular := t.Bouts[:0:0]
	for _, b := range t.Bouts {
		if !b.IsPlayoff {
			regular = append(regular, b)
		}
	}
	t.Bouts = append(regular, playoffs...)
	snapshot.SortBouts(t.Bouts)
	return len(playoffs), nil
}

// wins counts a banzuke record's wins, fusen included, as the standings do.
func wins(e domain.BanzukeEntry) int {
	n := 0
	for _, r := range e.Results {
		if r.Outcome.IsWin() {
			n++
		}
	}
	return n
}

// logPlayoffs runs addPlayoffs for the builder: a failure is logged, never
// fatal, as most tournaments have no playoff and the snapshot is complete
// without one (the standings name the yusho from the metadata). osada can
// only have the newest tournament of a run, so for older ones (a full
// build) its serving another one goes unsaid.
func logPlayoffs(ctx context.Context, src PlayoffSource, t *snapshot.Tournament, newest bool, log io.Writer) {
	n, err := addPlayoffs(ctx, src, t)
	switch {
	case errors.Is(err, errOtherTournament) && !newest:
	case err != nil:
		_, _ = fmt.Fprintf(log, "%s: no playoff bouts from osada: %v\n", t.Basho.ID, err)
	case n > 0:
		_, _ = fmt.Fprintf(log, "%s: %s from osada\n", t.Basho.ID, playoffCount(n))
	}
}

// RefreshPlayoffs adds osada's playoff bouts to tournament id, already in the
// snapshot in dir, and rewrites its file and the manifest. It is for a
// tournament added while osada was unreachable, or before the builder asked
// osada at all (Kyushu 2026).
func RefreshPlayoffs(ctx context.Context, src PlayoffSource, dir string, id domain.BashoID, generator string, log io.Writer) error {
	m, err := snapshot.ReadManifest(os.DirFS(dir))
	if err != nil {
		return err
	}
	if !m.Has(id) {
		return fmt.Errorf("%s is not in the snapshot", id)
	}
	t, err := snapshot.ReadTournament(os.DirFS(dir), id)
	if err != nil {
		return err
	}
	n, err := addPlayoffs(ctx, src, &t)
	if err != nil {
		return fmt.Errorf("%s: %w", id, err)
	}
	data, err := encodeChecked(t)
	if err != nil {
		return fmt.Errorf("%s: %w", id, err)
	}
	path := snapshot.TournamentPath(id)
	if snapshot.Checksum(data) == m.Files[path] {
		_, _ = fmt.Fprintf(log, "%s: %s, unchanged\n", id, playoffCount(n))
		return nil
	}
	if err := writeSnapshotFile(dir, path, data, &m); err != nil {
		return err
	}
	m.Generator, m.GeneratedAt = generator, time.Now().UTC().Truncate(time.Second)
	if err := writeManifest(dir, m); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(log, "%s: %s from osada, %d bytes\n", id, playoffCount(n), len(data))
	return nil
}

func playoffCount(n int) string {
	if n == 1 {
		return "1 playoff bout"
	}
	return fmt.Sprintf("%d playoff bouts", n)
}
