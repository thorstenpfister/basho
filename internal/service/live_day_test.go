package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/live"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/resolver"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// A simulated day 13 (SPEC §10, Phase 10 steps 4 and 5), timed like
// Kyushu 2026 (docs/v1-latency.md): the bouts end every 6 minutes, Juryo
// from 14:30, Makuuchi from 16:00, the last at 17:54. sumo-api shows each
// result a minute after the bout; osada publishes in batches every 13
// minutes from 15:04.

var simDay = time.Date(2026, time.September, 25, 0, 0, 0, 0, domain.JST)

const (
	sumoDelay  = time.Minute
	osadaBatch = 13 * time.Minute
)

var osadaFirst = simDay.Add(15*time.Hour + 4*time.Minute)

// decidedAt is when a bout ends; i is its index in the division.
func decidedAt(div string, i int) time.Time {
	start := 14*time.Hour + 30*time.Minute
	if div == "makuuchi" {
		start = 16 * time.Hour
	}
	return simDay.Add(start + time.Duration(i+1)*6*time.Minute)
}

// sumoAt is when sumo-api first shows a bout's result.
func sumoAt(div string, i int) time.Time { return decidedAt(div, i).Add(sumoDelay) }

// osadaAt is the osada batch that first shows a bout's result.
func osadaAt(div string, i int) time.Time {
	at := decidedAt(div, i)
	if !at.After(osadaFirst) {
		return osadaFirst
	}
	n := (at.Sub(osadaFirst) + osadaBatch - 1) / osadaBatch
	return osadaFirst.Add(n * osadaBatch)
}

// simServer serves a source's files with ETags, rewritten for the clock's
// time, and counts the requests.
type simServer struct {
	clk  *clock.Fake
	file func(path string, now time.Time) []byte

	mu                sync.Mutex
	requests, notMods int
	paths             map[string]int
}

func (s *simServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	if s.paths == nil {
		s.paths = map[string]int{}
	}
	s.paths[r.URL.Path]++
	body := s.file(r.URL.Path, s.clk.Now())
	if body == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		s.notMods++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

// osadaFile is torikumi.json with day 13's results as far as osada has
// published them; resultUpdatedAt is the batch that last changed something.
func osadaFile(torikumi map[string]any, banzuke []byte) func(string, time.Time) []byte {
	return func(path string, now time.Time) []byte {
		if path != osada.TorikumiPath {
			return banzuke
		}
		raw, _ := json.Marshal(torikumi)
		var t map[string]any
		_ = json.Unmarshal(raw, &t)
		stamp := simDay.Add(13 * time.Hour)
		day := t["resultDays"].([]any)[0].(map[string]any)["data"].(map[string]any)
		for _, div := range []string{"makuuchi", "juryo"} {
			for i, m := range day[div].(map[string]any)["matches"].([]any) {
				match := m.(map[string]any)
				if p := osadaAt(div, i); p.After(now) {
					match["kimarite"], match["winner"] = "未定", nil
				} else if p.After(stamp) {
					stamp = p
				}
			}
		}
		t["resultUpdatedAt"] = stamp.Format(time.RFC3339)
		out, _ := json.Marshal(t)
		return out
	}
}

// sumoFile serves the recorded files, with day 13's bout lists as far as
// sumo-api has the results.
func sumoFile(path string, now time.Time) []byte {
	body, err := os.ReadFile(filepath.Join("../../testdata/sumoapi", fixture.Name(path)))
	if err != nil {
		return nil
	}
	div := ""
	switch {
	case strings.HasSuffix(path, "/torikumi/Makuuchi/13"):
		div = "makuuchi"
	case strings.HasSuffix(path, "/torikumi/Juryo/13"):
		div = "juryo"
	default:
		return body
	}
	var t map[string]any
	_ = json.Unmarshal(body, &t)
	for _, m := range t["torikumi"].([]any) {
		b := m.(map[string]any)
		if sumoAt(div, int(b["matchNo"].(float64))-1).After(now) {
			b["winnerId"], b["winnerEn"], b["winnerJp"], b["kimarite"] = 0, "", "", ""
		}
	}
	out, _ := json.Marshal(t)
	return out
}

func TestLiveDay(t *testing.T) {
	clk := clock.NewFake(simDay.Add(13*time.Hour + 50*time.Minute))
	var torikumi map[string]any
	raw, err := os.ReadFile("../../testdata/osada/torikumi.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &torikumi); err != nil {
		t.Fatal(err)
	}
	// Day 13 alone keeps each poll's file small; the rest of the tournament
	// comes from sumo-api's banzuke.
	days := torikumi["resultDays"].([]any)
	torikumi["resultDays"], torikumi["scheduleDays"] = []any{days[12]}, []any{}
	banzuke, err := os.ReadFile("../../testdata/osada/banzuke.json")
	if err != nil {
		t.Fatal(err)
	}
	o := &simServer{clk: clk, file: osadaFile(torikumi, banzuke)}
	sumo := &simServer{clk: clk, file: sumoFile}
	osSrv, sumoSrv := httptest.NewServer(o), httptest.NewServer(sumo)
	t.Cleanup(osSrv.Close)
	t.Cleanup(sumoSrv.Close)
	c, err := cache.Open(t.TempDir(), 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	client := httpx.New(httpx.Options{UserAgent: "basho-test"})
	cached := func(name string, p cache.Policy) httpx.Getter {
		return resolver.Record(&cache.Getter{Cache: c, Next: client, Policy: p, Now: clk.Now}, name, clk.Now)
	}
	res := &resolver.Resolver{
		Historical: sumoapi.New(cached(resolver.SumoAPI, sumoapi.Freshness{}.Policy), sumoSrv.URL),
		Current:    osada.New(cached(resolver.Osada, osada.Freshness{}.Policy), osSrv.URL), Now: clk.Now,
	}
	svc := New(Options{Remote: res, Wrestlers: liveWrestlers{}, Clock: clk})
	res.IDs = svc.IDs

	// Deterministic jitter across the whole range.
	n := 0
	jitter := func() float64 { n++; return float64(n%10) / 10 }
	s := live.New(live.Config{Interval: 3 * time.Minute, Rand: jitter}, domain.NewDate(2026, time.September, 13))
	s.SetOn(true)

	seen := map[string]time.Time{} // division and match → first poll that showed it decided
	var polls []time.Time
	osadaPolls := 0
	for range 1000 {
		now := clk.Now()
		wake := s.Wake(now)
		if wake.IsZero() || wake.After(simDay.Add(20*time.Hour)) {
			break
		}
		if wake.After(now) {
			clk.Advance(wake.Sub(now))
			now = wake
		}
		if !s.Due(now) {
			clk.Advance(time.Second)
			continue
		}
		p, err := svc.LivePoll(t.Context(), aki2026, 13)
		if err != nil {
			t.Fatal(err)
		}
		polls = append(polls, now)
		if p.Osada {
			osadaPolls++
		}
		for div, d := range p.Days {
			for _, b := range d.Value.Bouts {
				if k := div.String() + "#" + strconv.Itoa(b.MatchNo); b.Winner != 0 && seen[k].IsZero() {
					seen[k] = now
				}
			}
		}
		s.Polled(now, live.Result{UpdatedAt: p.UpdatedAt, Complete: p.Complete, Err: p.Err()})
	}

	last := polls[len(polls)-1]
	t.Logf("%d polls %s–%s; sumo-api %d requests; osada %d requests (%d polls), %d of them 304",
		len(polls), polls[0].Format("15:04"), last.Format("15:04"), sumo.requests, o.requests, osadaPolls, o.notMods)
	// Budget (SPEC §7.3): sumo-api's two bout lists per poll, plus the
	// metadata and banzuke once; osada every 15 minutes, its banzuke once.
	if lists := sumo.paths["/api/basho/202609/torikumi/Makuuchi/13"] + sumo.paths["/api/basho/202609/torikumi/Juryo/13"]; lists != 2*len(polls) || sumo.requests > 2*len(polls)+3 {
		t.Errorf("sumo-api: %d requests, %d of them bout lists, for %d polls", sumo.requests, lists, len(polls))
	}
	// osada's banzuke keeps its 30-minute TTL and comes along with a
	// revalidation once expired.
	span := last.Sub(polls[0])
	if lists, max := o.paths[osada.TorikumiPath], 2+int(span/OsadaEvery); lists > max || o.requests-lists > 1+int(span/(30*time.Minute)) {
		t.Errorf("osada: %d requests, %d of them torikumi.json; want %d torikumi.json at most and the banzuke every 30 minutes", o.requests, lists, max)
	}
	if sumo.requests+o.requests > 200 {
		t.Errorf("%d requests; the budget is 200 a day", sumo.requests+o.requests)
	}
	// The last result is on sumo-api at 17:55; complete at the next poll,
	// the final check 30 minutes later, then nothing.
	if st := s.State(last); st != live.Done || last.After(simDay.Add(18*time.Hour+40*time.Minute)) {
		t.Errorf("last poll %s in state %s", last.Format("15:04"), st)
	}
	// Every result shows within one poll interval of sumo-api having it:
	// the base interval and jitter once results flow, the doubled interval
	// at most for the first ones after the quiet start.
	slow := 0
	for _, div := range []string{"makuuchi", "juryo"} {
		name := map[string]string{"makuuchi": "Makuuchi", "juryo": "Juryo"}[div]
		matches := torikumi["resultDays"].([]any)[0].(map[string]any)["data"].(map[string]any)[div].(map[string]any)["matches"].([]any)
		for i, m := range matches {
			no := int(m.(map[string]any)["boutNo"].(float64))
			at := seen[name+"#"+strconv.Itoa(no)]
			lag := at.Sub(sumoAt(div, i))
			switch {
			case at.IsZero():
				t.Errorf("%s #%d never seen", name, no)
			case lag > live.MaxInterval:
				t.Errorf("%s #%d seen %s after sumo-api had it", name, no, lag)
			case lag > 3*time.Minute+live.Jitter:
				slow++
				t.Logf("slow: %s #%d on sumo-api %s, seen %s", name, no, sumoAt(div, i).Format("15:04"), at.Format("15:04:05"))
			}
			if !at.IsZero() && at.Before(sumoAt(div, i)) && at.Before(osadaAt(div, i)) {
				t.Errorf("%s #%d seen before either source had it", name, no)
			}
		}
	}
	if slow > 2 {
		t.Errorf("%d results took longer than one base interval", slow)
	}
}
