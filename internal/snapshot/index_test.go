package snapshot

import (
	"fmt"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// synthetic makes a tournament of realistic size: 42 Makuuchi and 28 Juryo
// wrestlers, everyone fighting every day.
func synthetic(id domain.BashoID) Tournament {
	t := Tournament{Basho: domain.Basho{ID: id, Yusho: map[domain.Division]domain.RikishiID{}}, Banzuke: map[domain.Division][]domain.BanzukeEntry{}}
	for _, d := range []struct {
		div   domain.Division
		title domain.RankTitle
		n     int
		first domain.RikishiID
	}{{domain.DivMakuuchi, domain.Maegashira, 42, 1000}, {domain.DivJuryo, domain.Juryo, 28, 2000}} {
		for i := range d.n {
			t.Banzuke[d.div] = append(t.Banzuke[d.div], domain.BanzukeEntry{
				RikishiID: d.first + domain.RikishiID(i), Shikona: domain.Name{En: fmt.Sprint("R", d.first+domain.RikishiID(i))},
				Rank: domain.Rank{Title: d.title, Number: i/2 + 1, Side: domain.Side(i%2 + 1)},
			})
		}
		for day := 1; day <= domain.BashoDays; day++ {
			for m := range d.n / 2 {
				e, w := t.Banzuke[d.div][(m+day)%d.n], t.Banzuke[d.div][(d.n-1-m+day)%d.n]
				t.Bouts = append(t.Bouts, domain.Bout{
					Basho: id, Division: d.div, Day: day, MatchNo: m + 1,
					East:   domain.BoutSide{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank},
					West:   domain.BoutSide{RikishiID: w.RikishiID, Shikona: w.Shikona, Rank: w.Rank},
					Winner: domain.Side(m%2 + 1), Kimarite: "yorikiri",
				})
			}
		}
	}
	return t
}

func TestIndex(t *testing.T) {
	a, b := domain.BashoID{Year: 2026, Month: time.May}, domain.BashoID{Year: 2026, Month: time.July}
	x := NewIndex()
	tb := synthetic(b)
	tb.Basho.Yusho[domain.DivMakuuchi] = 1000
	x.Set(tb.Basho, tb.Bouts) // out of order on purpose
	ta := synthetic(a)
	x.Set(ta.Basho, ta.Bouts)
	if !slices.Equal(x.Tournaments(), []domain.BashoID{a, b}) {
		t.Errorf("Tournaments = %v", x.Tournaments())
	}
	if m, ok := x.Basho(b); !ok || m.Yusho[domain.DivMakuuchi] != 1000 {
		t.Errorf("Basho(202607) = %+v, %v", m, ok)
	}
	career := x.Rikishi(1000)
	if len(career) != 2*domain.BashoDays || career[0].Basho != a || career[0].Day != 1 || career[len(career)-1].Basho != b {
		t.Fatalf("career: %d bouts, first %v day %d", len(career), career[0].Basho, career[0].Day)
	}

	// The spoiler-safe view: bouts before a day.
	for _, c := range []struct {
		basho domain.BashoID
		day   int
		want  int
	}{{a, 1, 0}, {a, 2, 1}, {b, 1, domain.BashoDays}, {b, 14, domain.BashoDays + 13}, {b.Next(), 1, 2 * domain.BashoDays}} {
		if got := len(x.RikishiBefore(1000, c.basho, c.day)); got != c.want {
			t.Errorf("before %v day %d: %d bouts, want %d", c.basho, c.day, got, c.want)
		}
	}

	// Live results replace a tournament's bouts; earlier readers keep theirs.
	held := x.Rikishi(1000)
	live := slices.Clone(tb.Bouts[:21])
	live[0].Winner = 0
	x.Set(tb.Basho, live)
	if got := len(x.Rikishi(1000)); got != domain.BashoDays+1 {
		t.Errorf("after replacing 202607 with day 1 only: %d bouts, want %d", got, domain.BashoDays+1)
	}
	if got := x.Tournament(b); len(got) != 21 || got[0].Winner != 0 {
		t.Errorf("Tournament(202607) = %d bouts", len(got))
	}
	if len(held) != 2*domain.BashoDays {
		t.Error("Set changed a slice a reader already held")
	}
}

// fullSnapshot is about the size of the real one: 400 tournaments.
func fullSnapshot(tb testing.TB) fstest.MapFS {
	tb.Helper()
	fsys := fstest.MapFS{}
	m := Manifest{Schema: Schema, Files: map[string]string{}}
	id := domain.FirstBasho
	for range 400 {
		data, err := EncodeTournament(synthetic(id))
		if err != nil {
			tb.Fatal(err)
		}
		fsys[TournamentPath(id)] = &fstest.MapFile{Data: data}
		m.Basho = append(m.Basho, id)
		id = id.Next()
	}
	man, _ := EncodeManifest(m)
	fsys[ManifestPath] = &fstest.MapFile{Data: man}
	return fsys
}

func TestBuildIndex(t *testing.T) {
	s, err := Open(fullSnapshot(t))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	x, err := s.BuildIndex()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("index over 400 synthetic tournaments: %v", time.Since(start)) // V8, budget ~200 ms
	if n := len(x.Tournaments()); n != 400 {
		t.Errorf("%d tournaments", n)
	}
	if n := len(x.Rikishi(1000)); n != 400*domain.BashoDays {
		t.Errorf("wrestler 1000: %d bouts, want %d", n, 400*domain.BashoDays)
	}
}

func BenchmarkSnapshotIndex(b *testing.B) {
	fsys := fullSnapshot(b)
	for b.Loop() {
		s, err := Open(fsys)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := s.BuildIndex(); err != nil {
			b.Fatal(err)
		}
	}
}
