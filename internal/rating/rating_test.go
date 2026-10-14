package rating

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

var (
	t1 = domain.BashoID{Year: 2010, Month: time.November}
	t2 = domain.BashoID{Year: 2011, Month: time.January}
	t3 = domain.BashoID{Year: 2011, Month: time.May} // 2011-03 was cancelled
	t4 = domain.BashoID{Year: 2011, Month: time.July}
)

var params = Params{K: 20, KNew: 40, NewBouts: 2, R: 0.5, WarmupScale: 1}

func rank(title domain.RankTitle, n int) domain.Rank {
	return domain.Rank{Title: title, Number: n, Side: domain.East}
}

// bout: east beats west unless eastWins is false.
func bout(basho domain.BashoID, day int, east, west domain.RikishiID, er, wr domain.Rank, eastWins bool) domain.Bout {
	b := domain.Bout{
		Basho: basho, Division: domain.DivMakuuchi, Day: day, MatchNo: 1,
		East: domain.BoutSide{RikishiID: east, Rank: er}, West: domain.BoutSide{RikishiID: west, Rank: wr},
		Winner: domain.West, Kimarite: "yorikiri",
	}
	if eastWins {
		b.Winner = domain.East
	}
	return b
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestWinChance(t *testing.T) {
	if !near(WinChance(1500, 1500), 0.5) || !near(WinChance(1900, 1500), 10.0/11) || !near(WinChance(1500, 1900)+WinChance(1900, 1500), 1) {
		t.Error("Elo expectation")
	}
}

func TestBands(t *testing.T) {
	for _, tt := range []struct {
		r    domain.Rank
		want Band
	}{
		{rank(domain.Yokozuna, 1), BandY},
		{rank(domain.Komusubi, 2), BandK},
		{rank(domain.Maegashira, 5), BandM1},
		{rank(domain.Maegashira, 6), BandM6},
		{rank(domain.Maegashira, 10), BandM6},
		{rank(domain.Maegashira, 17), BandM11},
		{rank(domain.Juryo, 7), BandJ1},
		{rank(domain.Juryo, 8), BandJ8},
		{rank(domain.Makushita, 3), BandMs},
	} {
		if got := BandOf(tt.r); got != tt.want {
			t.Errorf("%v: band %d, want %d", tt.r, got, tt.want)
		}
	}
}

func TestUpdates(t *testing.T) {
	y1, m1 := rank(domain.Yokozuna, 1), rank(domain.Maegashira, 1)
	r := New(params)
	fusen := bout(t1, 1, 1, 2, y1, m1, true)
	fusen.Kimarite = "fusen"
	draw := bout(t1, 1, 3, 4, y1, m1, true)
	draw.Kimarite = "hikiwake"
	undecided := bout(t1, 4, 1, 2, y1, m1, true)
	undecided.Winner = 0
	if err := r.Set(t1, []domain.Bout{
		bout(t1, 2, 1, 2, y1, m1, false), // the Maegashira beats the Yokozuna
		bout(t1, 3, 1, 2, y1, m1, false),
		bout(t1, 5, 1, 2, y1, m1, true), // 1's third bout: K, not K_new
		fusen, draw, undecided,
	}); err != nil {
		t.Fatal(err)
	}
	// Debut ratings from the warm-up table: Y 1750, M1–5 1575.
	ry, rm := 1750.0, 1575.0
	for _, eastWins := range []bool{false, false} {
		p := WinChance(ry, rm)
		ry, rm = ry+40*(b2f(eastWins)-p), rm+40*(p-b2f(eastWins))
	}
	p := WinChance(ry, rm)
	ry, rm = ry+20*(1-p), rm+20*(p-1)

	got, prov := r.Before(1, y1, derive.NoCutoff)
	if prov || !near(got, ry) {
		t.Errorf("Yokozuna %.6f, want %.6f", got, ry)
	}
	if got, _ := r.Before(2, m1, derive.NoCutoff); !near(got, rm) {
		t.Errorf("Maegashira %.6f, want %.6f", got, rm)
	}
	if got, prov := r.Before(3, y1, derive.NoCutoff); !prov || got != 1750 {
		t.Errorf("only a draw: %.1f provisional %v; want the start rating", got, prov)
	}
	// As of before day 3: one bout played.
	p = WinChance(1750, 1575)
	if got, _ := r.Before(1, y1, derive.BeforeDay(t1, 3)); !near(got, 1750-40*p) {
		t.Errorf("before day 3: %.6f", got)
	}
	if got, prov := r.Before(1, y1, derive.BeforeDay(t1, 1)); !prov || got != 1750 {
		t.Errorf("before the debut: %.1f %v", got, prov)
	}
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func TestStartAndPull(t *testing.T) {
	j10, j9 := rank(domain.Juryo, 10), rank(domain.Juryo, 9)
	r := New(params)
	// t1: twelve J8–14 wrestlers debut in pairs; the winners end above 1450.
	var bouts []domain.Bout
	for i := domain.RikishiID(1); i <= 12; i += 2 {
		bouts = append(bouts, bout(t1, 1, i, i+1, j10, j9, true))
	}
	if err := r.Set(t1, bouts); err != nil {
		t.Fatal(err)
	}
	// t2: they fight again, now established: their ratings form the band
	// average, which 1450 ± the same amounts averages back to 1450.
	bouts = bouts[:0]
	for i := domain.RikishiID(1); i <= 12; i += 2 {
		bouts = append(bouts, bout(t2, 1, i, i+1, j10, j9, true))
	}
	if err := r.Set(t2, bouts); err != nil {
		t.Fatal(err)
	}
	if got := r.start(t3, BandJ8); !near(got, 1450) {
		t.Errorf("band average %.6f, want 1450", got)
	}
	if got := r.start(t3, BandY); got != 1750 {
		t.Errorf("an empty band uses the table: %.1f", got)
	}

	// Wrestler 1 skips t3 (held; the cancelled 2011-03 does not count) and
	// returns in t4: pulled halfway (R 0.5) towards the band average.
	after2, _ := r.Before(1, j10, derive.EndOf(t2))
	want := 1450 + (after2-1450)*0.5
	if got, _ := r.Before(1, j10, derive.BeforeDay(t4, 1)); !near(got, want) {
		t.Errorf("before the return %.6f, want %.6f", got, want)
	}
	if err := r.Set(t3, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.Set(t4, []domain.Bout{bout(t4, 1, 1, 13, j10, j9, true)}); err != nil {
		t.Fatal(err)
	}
	p := WinChance(want, 1450) // 13 debuts at the band average
	if got, _ := r.Before(1, j10, derive.NoCutoff); !near(got, want+20*(1-p)) {
		t.Errorf("after the return %.6f, want %.6f", got, want+20*(1-p))
	}
}

func TestSetReplacesTheNewest(t *testing.T) {
	y1, m1 := rank(domain.Yokozuna, 1), rank(domain.Maegashira, 1)
	day1 := []domain.Bout{bout(t2, 1, 1, 2, y1, m1, true)}
	full := append(day1, bout(t2, 2, 1, 3, y1, m1, false))

	live := New(params)
	fresh := New(params)
	for _, r := range []*Ratings{live, fresh} {
		if err := r.Set(t1, []domain.Bout{bout(t1, 1, 1, 2, y1, m1, true)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := live.Set(t2, day1); err != nil {
		t.Fatal(err)
	}
	if err := live.Set(t2, full); err != nil { // day 2 came in
		t.Fatal(err)
	}
	if err := fresh.Set(t2, full); err != nil {
		t.Fatal(err)
	}
	for _, id := range []domain.RikishiID{1, 2, 3} {
		a, _ := live.Before(id, m1, derive.NoCutoff)
		b, _ := fresh.Before(id, m1, derive.NoCutoff)
		if a != b {
			t.Errorf("wrestler %d: %.6f after replacing, %.6f fresh", id, a, b)
		}
	}
	if len(live.wrestlers[1].history) != 3 {
		t.Errorf("history %d points, want 3", len(live.wrestlers[1].history))
	}
	if err := live.Set(t1, nil); !errors.Is(err, ErrOutOfOrder) {
		t.Errorf("older tournament: %v", err)
	}
}

func TestReplay(t *testing.T) {
	y1, m1 := rank(domain.Yokozuna, 1), rank(domain.Maegashira, 1)
	ts := [][]domain.Bout{
		{bout(t1, 2, 1, 2, y1, m1, false), bout(t1, 1, 1, 2, y1, m1, true)}, // out of order on purpose
		{bout(t2, 1, 1, 2, y1, m1, true)},
	}
	var seen []float64
	Replay(params, ts, func(b domain.Bout, east float64) { seen = append(seen, east) })

	r := New(params)
	for _, bouts := range ts {
		if err := r.Set(bouts[0].Basho, bouts); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("observed %d bouts", len(seen))
	}
	for i, b := range []domain.Bout{ts[0][1], ts[0][0], ts[1][0]} {
		a, _ := r.Before(1, y1, derive.BeforeBout(b))
		m, _ := r.Before(2, m1, derive.BeforeBout(b))
		if !near(seen[i], WinChance(a, m)) {
			t.Errorf("bout %d: replay %.6f, as-of %.6f", i, seen[i], WinChance(a, m))
		}
	}
}
