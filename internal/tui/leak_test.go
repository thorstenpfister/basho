package tui_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/spoiler"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/day"
	"github.com/thorstenpfister/basho/internal/tui/picker"
	"github.com/thorstenpfister/basho/internal/tui/theme"
	"github.com/thorstenpfister/basho/internal/tui/tuitest"
)

// Phase 11 step 6: no spoiler leaks through any view. Every screen is drawn
// in spoiler-free mode over two worlds that agree up to the horizon and
// disagree after it: the real Aki 2026, and one where every later result
// went the other way, by a rare technique, with another yusho winner. Any
// difference on screen is a later result leaking.

// history is the embedded snapshot without Aki 2026, which the worlds serve
// as a tournament after the snapshot (so it goes into the bout index and
// the ratings day by day, as live data does).
func history(t *testing.T) (fs.FS, *snapshot.Snapshot) {
	t.Helper()
	dir, err := data.SnapshotFS()
	if err != nil {
		t.Fatal(err)
	}
	full, err := snapshot.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := full.Manifest()
	m.Basho = slices.DeleteFunc(slices.Clone(m.Basho), func(id domain.BashoID) bool { return id == aki2026 })
	files := map[string]string{}
	for k, v := range m.Files {
		if k != snapshot.TournamentPath(aki2026) {
			files[k] = v
		}
	}
	m.Files = files
	out := fstest.MapFS{}
	for path := range files {
		b, err := fs.ReadFile(dir, path)
		if err != nil {
			t.Fatal(err)
		}
		out[path] = &fstest.MapFile{Data: b}
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	out[snapshot.ManifestPath] = &fstest.MapFile{Data: b}
	return out, full
}

// world serves Aki 2026 from the full snapshot; with flip set,
// every result after h went the other way.
type world struct {
	full  *snapshot.Snapshot
	h     *derive.AsOf // the horizon the flipped world agrees up to
	flip  bool
	bouts map[dayOf]domain.Bout // each wrestler's regular bout of a day
}

type dayOf struct {
	r   domain.RikishiID
	day int
}

func newWorld(full *snapshot.Snapshot, h *derive.AsOf, flip bool) world {
	w := world{full: full, h: h, flip: flip, bouts: map[dayOf]domain.Bout{}}
	for day := 1; day <= domain.BashoDays; day++ {
		for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
			bouts, _ := full.Torikumi(aki2026, div, day)
			for _, b := range bouts {
				if !b.IsPlayoff {
					w.bouts[dayOf{b.East.RikishiID, day}] = b
					w.bouts[dayOf{b.West.RikishiID, day}] = b
				}
			}
		}
	}
	return w
}

func (w world) Basho(_ context.Context, id domain.BashoID) (domain.Basho, error) {
	b, err := w.full.Basho(id)
	if err == nil && w.flip && derive.EndOf(id).After(*w.h) {
		b.Yusho = map[domain.Division]domain.RikishiID{domain.DivMakuuchi: 7, domain.DivJuryo: 16} // Kirishima, Nishikigi
		b.Prizes = []domain.Prize{{Type: "Shukun-sho", RikishiID: 7}}
	}
	return b, err
}

func (w world) Torikumi(_ context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	bouts, err := w.full.Torikumi(id, div, day)
	if err != nil || !w.flip {
		return bouts, err
	}
	out := slices.Clone(bouts)
	for i := range out {
		w.flipBout(&out[i])
	}
	return out, nil
}

func (w world) flipBout(b *domain.Bout) {
	if b.Winner == 0 || w.h.Includes(*b) {
		return
	}
	b.Winner = domain.East + domain.West - b.Winner
	if b.Kimarite != "fusen" {
		b.Kimarite = "tokkurinage" // two-handed head twist down: rare
	}
}

func (w world) Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	es, err := w.full.Banzuke(id, div)
	if err != nil || !w.flip {
		return es, err
	}
	out := slices.Clone(es)
	for i := range out {
		for d := range domain.BashoDays {
			b, ok := w.boutOf(id, out[i].RikishiID, d+1)
			if !ok || b.Winner == 0 || w.h.Includes(b) {
				continue
			}
			r := &out[i].Results[d]
			switch r.Outcome {
			case domain.Win:
				r.Outcome = domain.Loss
			case domain.Loss:
				r.Outcome = domain.Win
			case domain.FusenWin:
				r.Outcome = domain.FusenLoss
			case domain.FusenLoss:
				r.Outcome = domain.FusenWin
			}
			if r.Kimarite != "fusen" {
				r.Kimarite = "tokkurinage"
			}
		}
	}
	return out, nil
}

// boutOf finds a wrestler's regular bout of a day, in either division.
func (w world) boutOf(_ domain.BashoID, r domain.RikishiID, day int) (domain.Bout, bool) {
	b, ok := w.bouts[dayOf{r, day}]
	return b, ok
}

// leakWorld is one world's app in spoiler-free mode at horizon h.
type leakWorld struct {
	ctx *tui.Context
	st  *spoiler.State
	h   *derive.AsOf
}

// moveTo puts the world's horizon, and for a flipped world the point after
// which results differ, at h; every day is loaded again.
func (w leakWorld) moveTo(t *testing.T, h spoiler.Horizon) {
	t.Helper()
	if err := w.st.Set(aki2026, h); err != nil {
		t.Fatal(err)
	}
	*w.h = h.AsOf(aki2026)
	for n := 1; n <= domain.BashoDays; n++ {
		for _, div := range []domain.Division{domain.DivJuryo, domain.DivMakuuchi} {
			if _, err := w.ctx.Svc.Day(t.Context(), aki2026, div, n); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// leakApp is the app over one world in spoiler-free mode at horizon h.
func leakApp(t *testing.T, h spoiler.Horizon, flip bool) (*tui.Context, *spoiler.State) {
	w := newLeakWorld(t, h, flip)
	return w.ctx, w.st
}

func newLeakWorld(t *testing.T, h spoiler.Horizon, flip bool) leakWorld {
	t.Helper()
	fsys, full := history(t)
	st, _ := spoiler.Load("")
	cut := h.AsOf(aki2026)
	svc := service.New(service.Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(fsys) },
		Params: func() (rating.File, error) {
			f, err := data.RatingParams()
			f.Show = true // whatever calibration said: the rating must not leak either
			return f, err
		},
		Remote: newWorld(full, &cut, flip),
		Clock:  clock.NewFake(testNow),
	})
	ctx := &tui.Context{Svc: svc, Cfg: config.Default(), Theme: theme.New(true, false), Clock: clock.NewFake(testNow), Tick: tui.NoTimers, Spoilers: st, Hide: true}
	w := leakWorld{ctx: ctx, st: st, h: &cut}
	// Every day in the bout index and the ratings first, so later results
	// are there to leak through last 5, head-to-head and ratings.
	w.moveTo(t, h)
	return w
}

// screens walks one world through every view and returns what it showed.
func screens(t *testing.T, ctx *tui.Context, days []int) map[string]string {
	t.Helper()
	out := map[string]string{}
	open := func(id domain.BashoID) tui.Screen { return day.New(ctx, id, 0) }
	for _, div := range []string{"makuuchi", "juryo"} {
		for _, n := range days {
			d := tuitest.New(tui.New(ctx, []tui.Screen{picker.New(ctx, open), day.New(ctx, aki2026, n)}, nil), 140, 44)
			if div == "juryo" {
				d.Keys("d")
			}
			name := fmt.Sprintf("%s day %d", div, n)
			out[name] = d.View()
			d.Keys("r")
			out[name+" recap"] = d.View()
			d.Keys("r", "s")
			out[name+" standings"] = d.View()
			d.Keys("enter")
			out[name+" standings expanded"] = d.View()
			d.Keys("tab")
			out[name+" grid"] = d.View()
			for _, j := range []int{0, 3, 8} {
				d.Keys(slices.Repeat([]string{"j"}, j)...)
				d.Keys("enter")
				out[fmt.Sprintf("%s popup %d", name, j)] = d.View()
				d.Keys("esc")
			}
			d.Keys("i") // the career of the grid's wrestler
			out[name+" grid career"] = d.View()
			d.Keys("esc")
			d.Keys("esc") // the day view
			rows := 20
			for i := range rows {
				d.Keys("enter")
				out[fmt.Sprintf("%s VS %d", name, i+1)] = d.View()
				d.Keys("m")
				out[fmt.Sprintf("%s VS %d moveset", name, i+1)] = d.View()
				d.Keys("m", "v")
				out[fmt.Sprintf("%s VS %d history", name, i+1)] = d.View()
				d.Keys("v")
				if i == 0 || i == 9 || i == rows-2 { // careers: both sides of three bouts
					for _, side := range []string{"1", "2"} {
						d.Keys(side)
						out[fmt.Sprintf("%s VS %d career %s", name, i+1, side)] = d.View()
						d.Keys("esc")
					}
				}
				d.Keys("esc", "j")
			}
			d.Keys("esc")
			out[name+" picker"] = d.View()
		}
	}
	return out
}

func compareWorlds(t *testing.T, h spoiler.Horizon, days []int) {
	t.Helper()
	real, _ := leakApp(t, h, false)
	flipped, _ := leakApp(t, h, true)
	a, b := screens(t, real, days), screens(t, flipped, days)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("%d and %d screens", len(a), len(b))
	}
	leaks := 0
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for k := range a {
			if !yield(k) {
				return
			}
		}
	}) {
		if a[name] != b[name] {
			leaks++
			if leaks <= 3 {
				t.Errorf("%s leaks a later result:\n%s", name, diffLines(a[name], b[name]))
			}
		}
	}
	if leaks > 3 {
		t.Errorf("… %d screens leak in all", leaks)
	}
}

func diffLines(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	var out []string
	for i := range max(len(al), len(bl)) {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			out = append(out, "real:    "+x, "flipped: "+y)
		}
	}
	return strings.Join(out, "\n")
}

// A horizon in the middle of a day: after Makuuchi bout 9 of day 13.
func TestNoLeaks(t *testing.T) {
	if testing.Short() || raceDetector {
		t.Skip("walks every screen twice; make test-leaks runs it")
	}
	compareWorlds(t, spoiler.Horizon{Day: 13, Division: domain.DivMakuuchi, MatchNo: 9}, []int{12, 13, 14, 15})
}

// The two worlds really differ: without spoiler-free mode they do.
func TestLeakWorldsDiffer(t *testing.T) {
	h := spoiler.Horizon{Day: 13, Division: domain.DivMakuuchi, MatchNo: 9}
	real, _ := leakApp(t, h, false)
	flipped, _ := leakApp(t, h, true)
	real.Hide, flipped.Hide = false, false
	view := func(ctx *tui.Context) string {
		return tuitest.New(tui.New(ctx, []tui.Screen{day.New(ctx, aki2026, 13)}, nil), 140, 44).View()
	}
	if view(real) == view(flipped) {
		t.Fatal("the flipped world shows the same day 13")
	}
}

// Phase 11 AC: stepping through day 13 bout by bout with ] and space, no
// screen shows a later result than the one just revealed, ratings included.
func TestWalkDay13(t *testing.T) {
	if testing.Short() || raceDetector {
		t.Skip("rebuilds the flipped world at every step; make test-leaks runs it")
	}
	real := newLeakWorld(t, spoiler.Horizon{Day: 12}, false)
	flipped := newLeakWorld(t, spoiler.Horizon{Day: 12}, true)
	d := tuitest.New(tui.New(real.ctx, []tui.Screen{day.New(real.ctx, aki2026, 13)}, nil), 140, 44)
	d.Keys("enter") // bout 1
	for k := 1; k <= 19; k++ {
		h := real.st.Horizon(aki2026)
		flipped.moveTo(t, h)
		f := tuitest.New(tui.New(flipped.ctx, []tui.Screen{day.New(flipped.ctx, aki2026, 13)}, nil), 140, 44)
		f.Keys(slices.Repeat([]string{"j"}, k-1)...)
		f.Keys("enter")
		if a, b := d.View(), f.View(); a != b {
			t.Fatalf("bout %d before space, horizon %s:\n%s", k, h, diffLines(a, b))
		}
		if !strings.Contains(d.View(), "Result hidden") {
			t.Fatalf("bout %d shown before space:\n%s", k, d.View())
		}
		d.Keys("space")
		if v := d.View(); !strings.Contains(v, " wins by ") {
			t.Fatalf("bout %d after space:\n%s", k, v)
		}
		if k == 6 || k == 14 { // every screen, half-way through the day
			flipped.moveTo(t, real.st.Horizon(aki2026))
			a, b := screens(t, real.ctx, []int{13, 14}), screens(t, flipped.ctx, []int{13, 14})
			for name := range a {
				if a[name] != b[name] {
					t.Errorf("after bout %d, %s:\n%s", k, name, diffLines(a[name], b[name]))
				}
			}
		}
		d.Keys("]")
	}
	if h := real.st.Horizon(aki2026); h.Day != 13 || h.MatchNo != 19 {
		t.Errorf("horizon %s after the walk", h)
	}
}
