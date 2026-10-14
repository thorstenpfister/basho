package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/live"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/resolver"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// A simulated day 13 (SPEC §10, Phase 10 step 5): osada.us publishes on its
// 3-minute cron as the bouts end, Juryo from 14:30, Makuuchi from 16:00, the
// last bout at 17:54.

const cron = 3 * time.Minute

var simDay = time.Date(2026, time.September, 25, 0, 0, 0, 0, domain.JST)

// decidedAt is when a bout ends.
func decidedAt(div string, i int) time.Time {
	start := 14*time.Hour + 30*time.Minute
	if div == "makuuchi" {
		start = 16 * time.Hour
	}
	return simDay.Add(start + time.Duration(i+1)*6*time.Minute)
}

// publishedAt is when osada's cron first shows a bout's result.
func publishedAt(div string, i int) time.Time {
	at := decidedAt(div, i)
	return at.Truncate(cron).Add(cron)
}

// osadaDay serves the recorded torikumi.json with day 13's results as far
// as osada would have them at the clock's time.
type osadaDay struct {
	clk      *clock.Fake
	torikumi map[string]any
	banzuke  []byte

	mu                sync.Mutex
	requests, notMods int
}

func (o *osadaDay) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.requests++
	body := o.banzuke
	if r.URL.Path == osada.TorikumiPath {
		body = o.at(o.clk.Now())
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		o.notMods++
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

// at is torikumi.json as published by now: undecided bouts are 未定, and
// resultUpdatedAt is the cron run that last changed something.
func (o *osadaDay) at(now time.Time) []byte {
	raw, _ := json.Marshal(o.torikumi)
	var t map[string]any
	_ = json.Unmarshal(raw, &t)
	stamp := simDay.Add(13 * time.Hour)
	day := t["resultDays"].([]any)[0].(map[string]any)["data"].(map[string]any)
	for _, div := range []string{"makuuchi", "juryo"} {
		for i, m := range day[div].(map[string]any)["matches"].([]any) {
			match := m.(map[string]any)
			if p := publishedAt(div, i); p.After(now) {
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

func TestLiveDay(t *testing.T) {
	clk := clock.NewFake(simDay.Add(13*time.Hour + 50*time.Minute))
	o := &osadaDay{clk: clk}
	raw, err := os.ReadFile("../../testdata/osada/torikumi.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &o.torikumi); err != nil {
		t.Fatal(err)
	}
	// Day 13 alone keeps each poll's file small; the rest of the tournament
	// comes from sumo-api's banzuke.
	days := o.torikumi["resultDays"].([]any)
	o.torikumi["resultDays"], o.torikumi["scheduleDays"] = []any{days[12]}, []any{}
	if o.banzuke, err = os.ReadFile("../../testdata/osada/banzuke.json"); err != nil {
		t.Fatal(err)
	}
	osSrv, sumoSrv := httptest.NewServer(o), httptest.NewServer(&server{dir: "../../testdata/sumoapi"})
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

	// Deterministic jitter across the whole range.
	n := 0
	jitter := func() float64 { n++; return float64(n%10) / 10 }
	s := live.New(live.Config{Interval: 3 * time.Minute, Rand: jitter}, domain.NewDate(2026, time.September, 13))
	s.SetOn(true)

	seen := map[string]time.Time{} // division and match → first poll that showed it decided
	var polls []time.Time
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
		for div, d := range p.Days {
			for _, b := range d.Value.Bouts {
				if k := div.String() + "#" + itoa(b.MatchNo); b.Winner != 0 && seen[k].IsZero() {
					seen[k] = now
				}
			}
		}
		s.Polled(now, live.Result{UpdatedAt: p.UpdatedAt, Complete: p.Complete, Err: p.Err()})
	}

	last := polls[len(polls)-1]
	t.Logf("%d polls %s–%s, %d osada requests, %d of them 304", len(polls), polls[0].Format("15:04"), last.Format("15:04"), o.requests, o.notMods)
	if o.requests > 90 {
		t.Errorf("%d osada requests; the budget is 90 a day", o.requests)
	}
	if o.notMods*2 < o.requests {
		t.Errorf("only %d of %d requests were 304", o.notMods, o.requests)
	}
	// The last result is out at 17:57; complete at the next poll, the
	// final check 30 minutes later, then nothing.
	if st := s.State(last); st != live.Done || last.After(simDay.Add(18*time.Hour+40*time.Minute)) {
		t.Errorf("last poll %s in state %s", last.Format("15:04"), st)
	}
	// Every result shows within one poll interval of osada's update: the
	// base interval and jitter once results flow, the doubled interval at
	// most for the first ones after the quiet start.
	slow := 0
	for _, div := range []string{"makuuchi", "juryo"} {
		name := map[string]string{"makuuchi": "Makuuchi", "juryo": "Juryo"}[div]
		matches := o.torikumi["resultDays"].([]any)[0].(map[string]any)["data"].(map[string]any)[div].(map[string]any)["matches"].([]any)
		for i, m := range matches {
			no := int(m.(map[string]any)["boutNo"].(float64))
			at := seen[name+"#"+itoa(no)]
			lag := at.Sub(publishedAt(div, i))
			switch {
			case at.IsZero():
				t.Errorf("%s #%d never seen", name, no)
			case lag > live.MaxInterval:
				t.Errorf("%s #%d seen %s after osada had it", name, no, lag)
			case lag > 3*time.Minute+live.Jitter:
				slow++
			}
		}
	}
	if slow > 2 {
		t.Errorf("%d results took longer than one base interval", slow)
	}
}

func itoa(n int) string { return string(rune('0'+n/10)) + string(rune('0'+n%10)) }
