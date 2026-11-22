package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/resolver"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// Replays of the Kyushu 2026 recordings (SPEC Appendix E): testdata/live
// holds a few ticks of day 1, day 5 and day 15, each a folder
// <YYYYMMDD>/<HHMM>/ with the osada and sumo-api answers at that time.

// replay serves one source's files from the latest recorded tick at the
// clock's time, with ETags, so unchanged files are 304s as on the day.
type replay struct {
	clk  *clock.Fake
	day  string // testdata/live/YYYYMMDD
	sub  string // "osada" or "sumoapi"
	down bool

	mu                sync.Mutex
	requests, notMods int
}

func (r *replay) tick() string {
	ticks, _ := os.ReadDir(r.day) // HHMM folders, in order
	now := r.clk.Now().In(domain.JST).Format("1504")
	at := ""
	for _, t := range ticks {
		if t.Name() <= now {
			at = t.Name()
		}
	}
	return at
}

func (r *replay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests++
	t := r.tick()
	if r.down || t == "" {
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	name := fixture.Name(req.URL.Path)
	body, err := os.ReadFile(filepath.Join(r.day, t, r.sub, name))
	if errors.Is(err, fs.ErrNotExist) {
		// What the recorder did not save, like sumo-api's banzuke, which
		// the app has cached long before the day.
		body, err = os.ReadFile(filepath.Join(r.day, "..", r.sub, name))
	}
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	if req.Header.Get("If-None-Match") == etag {
		r.notMods++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

// replayStack is service.Open's stack over the recordings of one day.
func replayStack(t *testing.T, date string, start time.Time) (*Service, *replay, *replay, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(start)
	dir := filepath.Join("../../testdata/live", date)
	os, sumo := &replay{clk: clk, day: dir, sub: "osada"}, &replay{clk: clk, day: dir, sub: "sumoapi"}
	osSrv, sumoSrv := httptest.NewServer(os), httptest.NewServer(sumo)
	t.Cleanup(osSrv.Close)
	t.Cleanup(sumoSrv.Close)
	c, err := cache.Open(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	client := httpx.New(httpx.Options{UserAgent: "basho-test"})
	cached := func(name string, p cache.Policy) httpx.Getter {
		return resolver.Record(&cache.Getter{Cache: c, Next: client, Policy: p}, name, clk.Now)
	}
	res := &resolver.Resolver{
		Historical: sumoapi.New(cached(resolver.SumoAPI, sumoapi.Freshness{}.Policy), sumoSrv.URL),
		Current:    osada.New(cached(resolver.Osada, osada.Freshness{}.Policy), osSrv.URL), Now: clk.Now,
	}
	svc := New(Options{Remote: res, Wrestlers: liveWrestlers{}, Clock: clk})
	res.IDs = svc.IDs
	return svc, os, sumo, clk
}

func decided(rows []BoutRow) int {
	n := 0
	for _, r := range rows {
		if r.Winner != 0 {
			n++
		}
	}
	return n
}

// Each recorded tick, polled as live mode would: the decided bouts as osada
// had them, and completeness, which on day 15 waits for the playoff.
func TestLiveReplay(t *testing.T) {
	type tick struct {
		at              string
		juryo, makuuchi int // decided regular bouts
		playoffs        int // decided Makuuchi playoff bouts
		complete        bool
	}
	days := []struct {
		date        string
		day         int
		juryo, maku int // regular bouts
		ticks       []tick
	}{
		{"20261108", 1, 14, 21, []tick{
			{"1400", 0, 0, 0, false}, // osada has only the schedule until about 15:04
			{"1510", 8, 0, 0, false},
			{"1540", 14, 0, 0, false},
			{"1620", 14, 1, 0, false},
			{"1700", 14, 11, 0, false},
			{"1800", 14, 21, 0, true},
		}},
		{"20261112", 5, 13, 21, []tick{
			{"1400", 0, 0, 0, false},
			{"1510", 4, 0, 0, false},
			{"1550", 13, 0, 0, false},
			{"1700", 13, 11, 0, false},
			{"1750", 13, 20, 0, false},
			{"1810", 13, 21, 0, true},
		}},
		{"20261122", 15, 14, 20, []tick{
			{"1400", 0, 0, 0, false},
			{"1530", 14, 0, 0, false},
			{"1700", 14, 15, 0, false},
			{"1730", 14, 19, 0, false},
			// Every regular bout decided, Onosato and Wakamotoharu level
			// at the top and no playoff listed yet: not complete.
			{"1740", 14, 20, 0, false},
			{"1750", 14, 20, 1, true}, // the playoff, listed with its result
			{"1820", 14, 20, 1, true},
			// osada also lists the playoff in day 15's schedule, undecided;
			// the result still wins.
			{"1910", 14, 20, 1, true},
		}},
	}
	for _, d := range days {
		t.Run(d.date, func(t *testing.T) {
			date, _ := time.ParseInLocation("20060102", d.date, domain.JST)
			svc, _, _, clk := replayStack(t, d.date, date.Add(14*time.Hour))
			for _, tk := range d.ticks {
				hm, _ := time.Parse("1504", tk.at)
				setClock(clk, date.Add(time.Duration(hm.Hour())*time.Hour+time.Duration(hm.Minute())*time.Minute))
				p, err := svc.LivePoll(t.Context(), kyushu2026, d.day)
				if err != nil {
					t.Fatalf("%s: %v", tk.at, err)
				}
				if p.Err() != nil {
					t.Errorf("%s: %v", tk.at, p.Err())
				}
				j, m := p.Days[domain.DivJuryo].Value, p.Days[domain.DivMakuuchi].Value
				if len(j.Bouts) != d.juryo || len(m.Bouts) != d.maku {
					t.Errorf("%s: %d Juryo and %d Makuuchi bouts, want %d and %d", tk.at, len(j.Bouts), len(m.Bouts), d.juryo, d.maku)
				}
				got := tick{tk.at, decided(j.Bouts), decided(m.Bouts), decided(m.Playoffs), p.Complete}
				if got != tk {
					t.Errorf("got %+v, want %+v", got, tk)
				}
				if len(m.Playoffs) != tk.playoffs || len(j.Playoffs) != 0 {
					t.Errorf("%s: %d Makuuchi and %d Juryo playoff rows", tk.at, len(m.Playoffs), len(j.Playoffs))
				}
			}
		})
	}
}

// Day 15's playoff: pending once the regular bouts left a shared lead,
// then listed with its result, which names the yusho.
func TestLiveReplayPlayoff(t *testing.T) {
	date := time.Date(2026, time.November, 22, 0, 0, 0, 0, domain.JST)
	svc, _, _, clk := replayStack(t, "20261122", date.Add(17*time.Hour+40*time.Minute))
	if _, err := svc.LivePoll(t.Context(), kyushu2026, 15); err != nil {
		t.Fatal(err)
	}
	st, err := svc.Standings(t.Context(), kyushu2026, domain.DivMakuuchi, 15)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Value.Playoff || st.Value.Yusho != nil {
		t.Errorf("17:40: playoff %v, yusho %v; want a playoff to come", st.Value.Playoff, st.Value.Yusho)
	}

	setClock(clk, date.Add(17*time.Hour+50*time.Minute))
	p, err := svc.LivePoll(t.Context(), kyushu2026, 15)
	if err != nil {
		t.Fatal(err)
	}
	po := p.Days[domain.DivMakuuchi].Value.Playoffs
	if len(po) != 1 {
		t.Fatalf("%d playoff bouts", len(po))
	}
	b := po[0]
	if b.East.Shikona.En != "Onosato" || b.West.Shikona.En != "Wakamotoharu" || b.Winner != domain.East || b.Kimarite.Kimarite != "watashikomi" {
		t.Errorf("playoff: %s vs %s, winner %v by %q", b.East.Shikona.En, b.West.Shikona.En, b.Winner, b.Kimarite.Kimarite)
	}
	// The yusho from the playoff: sumo-api names it only from 18:20, and
	// live polls do not ask it again.
	st, err = svc.Standings(t.Context(), kyushu2026, domain.DivMakuuchi, 15)
	if err != nil {
		t.Fatal(err)
	}
	if st.Value.Yusho == nil || st.Value.Yusho.Shikona.En != "Onosato" {
		t.Errorf("17:50: yusho %v; want Onosato", st.Value.Yusho)
	}
}

// sumo-api never listed the playoff bout. When osada fails after the
// regular bouts, day 15 stays open rather than closing without it.
func TestLiveReplayPlayoffFallback(t *testing.T) {
	date := time.Date(2026, time.November, 22, 0, 0, 0, 0, domain.JST)
	svc, os, _, clk := replayStack(t, "20261122", date.Add(17*time.Hour+40*time.Minute))
	if _, err := svc.LivePoll(t.Context(), kyushu2026, 15); err != nil {
		t.Fatal(err)
	}
	os.down = true
	setClock(clk, date.Add(19*time.Hour+10*time.Minute))
	p, err := svc.LivePoll(t.Context(), kyushu2026, 15)
	if err != nil {
		t.Fatal(err)
	}
	m := p.Days[domain.DivMakuuchi].Value
	if p.Err() == nil || !strings.Contains(p.Prov.Source, resolver.SumoAPI) {
		t.Errorf("source %q, err %v; want sumo-api after osada failed", p.Prov.Source, p.Err())
	}
	if decided(m.Bouts) != 20 || len(m.Playoffs) != 0 || p.Complete {
		t.Errorf("%d decided, %d playoffs, complete %v; want 20, none, open", decided(m.Bouts), len(m.Playoffs), p.Complete)
	}
}

// setClock moves the fake clock forward to at.
func setClock(clk *clock.Fake, at time.Time) {
	if d := at.Sub(clk.Now()); d > 0 {
		clk.Advance(d)
	}
}

// A three-way playoff: the last bout's winner, once every listed bout is
// decided.
func TestPlayoffWinner(t *testing.T) {
	bout := func(no int, east, west domain.RikishiID, w domain.Side) domain.Bout {
		return domain.Bout{MatchNo: no, IsPlayoff: true, East: domain.BoutSide{RikishiID: east}, West: domain.BoutSide{RikishiID: west}, Winner: w}
	}
	regular := domain.Bout{MatchNo: 20, East: domain.BoutSide{RikishiID: 9}, Winner: domain.East}
	for _, c := range []struct {
		name  string
		bouts []domain.Bout
		want  domain.RikishiID
		ok    bool
	}{
		{"none", []domain.Bout{regular}, 0, false},
		{"one", []domain.Bout{regular, bout(21, 1, 2, domain.West)}, 2, true},
		{"three-way", []domain.Bout{regular, bout(23, 3, 1, domain.West), bout(21, 1, 2, domain.East), bout(22, 1, 3, domain.East)}, 1, true},
		{"last pending", []domain.Bout{regular, bout(21, 1, 2, domain.East), bout(22, 1, 3, 0)}, 0, false},
	} {
		if got, ok := playoffWinner(c.bouts); got != c.want || ok != c.ok {
			t.Errorf("%s: %d %v, want %d %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
