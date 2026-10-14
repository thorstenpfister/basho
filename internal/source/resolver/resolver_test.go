package resolver

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/idmap"
	"github.com/thorstenpfister/basho/internal/source"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

var (
	aki2026  = domain.BashoID{Year: 2026, Month: time.September}
	nagoya   = domain.BashoID{Year: 2026, Month: time.July}
	day13    = time.Date(2026, time.September, 25, 16, 0, 0, 0, domain.JST) // during day 13
	testBase = "https://fixture.test"
)

// sources are the recorded Aki 2026 files of both sources, with switches
// to fail or change them.
type sources struct {
	sumoFail, osadaFail error
	sumoStale           bool
	sumoCalls           []string // "live" or "cached-only", per Torikumi call
	osadaCalls          int
	torikumi            func([]domain.Bout) []domain.Bout // changes sumo-api's bouts
	banzuke             func([]domain.BanzukeEntry) []domain.BanzukeEntry
	current             func(*source.Current) // changes osada's tournament
}

func (s *sources) sumo() *sumoapi.Client {
	g := staler{fixture.Getter{Dir: "../../../testdata/sumoapi", Base: testBase}, &s.sumoStale}
	return sumoapi.New(Record(g, SumoAPI, time.Now), testBase)
}

// staler marks every response stale when told to.
type staler struct {
	g     httpx.Getter
	stale *bool
}

func (g staler) Get(ctx context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	resp, err := g.g.Get(ctx, url, v)
	if err == nil && *g.stale {
		resp.Stale, resp.Cached = true, true
	}
	return resp, err
}

func (s *sources) Basho(ctx context.Context, id domain.BashoID) (domain.Basho, error) {
	if s.sumoFail != nil {
		return domain.Basho{}, s.sumoFail
	}
	return s.sumo().Basho(ctx, id)
}

func (s *sources) Banzuke(ctx context.Context, id domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	if s.sumoFail != nil {
		return nil, s.sumoFail
	}
	es, err := s.sumo().Banzuke(ctx, id, div)
	if err == nil && s.banzuke != nil {
		es = s.banzuke(es)
	}
	return es, err
}

func (s *sources) Torikumi(ctx context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	call := "live"
	if cache.IsCachedOnly(ctx) {
		call = "cached-only"
	}
	s.sumoCalls = append(s.sumoCalls, call)
	if s.sumoFail != nil {
		return nil, s.sumoFail
	}
	bouts, err := s.sumo().Torikumi(ctx, id, div, day)
	if err == nil && s.torikumi != nil {
		bouts = s.torikumi(bouts)
	}
	return bouts, err
}

func (s *sources) Current(ctx context.Context) (source.Current, error) {
	s.osadaCalls++
	if s.osadaFail != nil {
		return source.Current{}, s.osadaFail
	}
	cur, err := osada.New(fixture.Getter{Dir: "../../../testdata/osada", Base: testBase}, testBase).Current(ctx)
	if err == nil && s.current != nil {
		s.current(&cur)
	}
	return cur, err
}

func ids(t *testing.T) func() (*idmap.Map, error) {
	t.Helper()
	sn, err := data.Snapshot() // the fixture snapshot's index lacks today's wrestlers
	if err != nil {
		t.Fatal(err)
	}
	rs, err := sn.Rikishi()
	if err != nil {
		t.Fatal(err)
	}
	m := idmap.New(rs)
	return func() (*idmap.Map, error) { return m, nil }
}

func newResolver(t *testing.T, s *sources, now time.Time) *Resolver {
	return &Resolver{Historical: s, Current: s, IDs: ids(t), Now: func() time.Time { return now }}
}

func call[T any](t *testing.T, fn func(context.Context) (T, error)) (T, *Track) {
	t.Helper()
	ctx, tr := Tracking(t.Context())
	v, err := fn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return v, tr
}

func bout(bouts []domain.Bout, match int) domain.Bout {
	for _, b := range bouts {
		if b.MatchNo == match {
			return b
		}
	}
	return domain.Bout{}
}

func TestIsCurrent(t *testing.T) {
	for _, tt := range []struct {
		today domain.Date
		id    domain.BashoID
		want  bool
	}{
		{domain.NewDate(2026, time.September, 25), aki2026, true},
		{domain.NewDate(2026, time.August, 31), aki2026, true},     // the banzuke is out
		{domain.NewDate(2026, time.August, 20), aki2026, false},    // not yet
		{domain.NewDate(2026, time.September, 28), aki2026, true},  // the day after
		{domain.NewDate(2026, time.September, 29), aki2026, false}, // final
		{domain.NewDate(2026, time.September, 25), nagoya, false},
	} {
		if got := IsCurrent(tt.id, tt.today); got != tt.want {
			t.Errorf("%s on %s: %v", tt.id, tt.today, got)
		}
	}
}

// A finished tournament goes to sumo-api alone; osada is not asked.
func TestFinishedTournament(t *testing.T) {
	s := &sources{}
	r := newResolver(t, s, day13)
	bouts, tr := call(t, func(ctx context.Context) ([]domain.Bout, error) {
		return r.Torikumi(ctx, nagoya, domain.DivMakuuchi, 15)
	})
	if len(bouts) == 0 || s.osadaCalls != 0 || !slices.Equal(tr.Answered(), []string{SumoAPI}) {
		t.Errorf("%d bouts, %d osada calls, answered %v", len(bouts), s.osadaCalls, tr.Answered())
	}
}

// The current tournament's day comes from osada, with English names through
// the ID mapping; sumo-api's copy is only consulted from the cache.
func TestTorikumiFromOsada(t *testing.T) {
	s := &sources{}
	r := newResolver(t, s, day13)
	bouts, tr := call(t, func(ctx context.Context) ([]domain.Bout, error) {
		return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
	})
	last := bout(bouts, 19)
	if last.East.RikishiID != 8850 || last.East.Shikona.En != "Onosato" || last.West.Shikona.En != "Kirishima" || last.Winner != domain.West || last.Kimarite != "shitatenage" {
		t.Errorf("bout 19: %+v", last)
	}
	if last.East.Rank.Short() != "Y1e" || last.Basho != aki2026 || last.Day != 13 || last.Division != domain.DivMakuuchi {
		t.Errorf("bout 19: %+v", last)
	}
	if !slices.Equal(tr.Answered(), []string{Osada}) || !slices.Equal(s.sumoCalls, []string{"cached-only"}) {
		t.Errorf("answered %v, sumo-api calls %v", tr.Answered(), s.sumoCalls)
	}
	if want := time.Date(2026, time.September, 27, 17, 27, 38, 0, domain.JST); !tr.UpdatedAt().Equal(want) {
		t.Errorf("updated %s", tr.UpdatedAt())
	}
	// Day 15 has the Juryo playoffs, which sumo-api lacks.
	j15, _ := call(t, func(ctx context.Context) ([]domain.Bout, error) { return r.Torikumi(ctx, aki2026, domain.DivJuryo, 15) })
	if p := bout(j15, 16); !p.IsPlayoff || p.Winner != domain.East {
		t.Errorf("playoff: %+v", p)
	}
}

func TestFallback(t *testing.T) {
	for _, err := range []error{source.ErrUpstream, source.ErrRateLimited, httpx.ErrCircuitOpen, source.ErrNotYetAvailable} {
		s := &sources{osadaFail: err}
		r := newResolver(t, s, day13)
		bouts, tr := call(t, func(ctx context.Context) ([]domain.Bout, error) {
			return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
		})
		if len(bouts) != 19 || !slices.Equal(tr.Answered(), []string{SumoAPI}) {
			t.Errorf("osada %v: %d bouts from %v", err, len(bouts), tr.Answered())
		}
	}
	// No fallback for data that cannot be mapped.
	s := &sources{osadaFail: source.ErrBadData}
	if _, err := newResolver(t, s, day13).Torikumi(t.Context(), aki2026, domain.DivMakuuchi, 13); !errors.Is(err, source.ErrBadData) {
		t.Errorf("bad data: %v", err)
	}
	// Both down: the first source's error.
	s = &sources{osadaFail: source.ErrUpstream, sumoFail: source.ErrRateLimited}
	if _, err := newResolver(t, s, day13).Torikumi(t.Context(), aki2026, domain.DivMakuuchi, 13); !errors.Is(err, source.ErrUpstream) {
		t.Errorf("both down: %v", err)
	}
}

// osada.us serving the previous tournament is behind: sumo-api answers.
func TestOsadaOnAnotherTournament(t *testing.T) {
	s := &sources{current: func(c *source.Current) { c.Basho = nagoya }}
	r := newResolver(t, s, day13)
	_, tr := call(t, func(ctx context.Context) ([]domain.Bout, error) {
		return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
	})
	if !slices.Equal(tr.Answered(), []string{SumoAPI}) {
		t.Errorf("answered %v", tr.Answered())
	}
}

// sumo-api disabled: the day, the banzuke and the dates from osada alone.
func TestSumoAPIDown(t *testing.T) {
	s := &sources{sumoFail: source.ErrUpstream}
	r := newResolver(t, s, day13)
	b, tr := call(t, func(ctx context.Context) (domain.Basho, error) { return r.Basho(ctx, aki2026) })
	if b.Start != domain.NewDate(2026, time.September, 13) || b.End != domain.NewDate(2026, time.September, 27) || !slices.Equal(tr.Answered(), []string{Osada}) {
		t.Errorf("basho %+v from %v", b, tr.Answered())
	}
	es, tr := call(t, func(ctx context.Context) ([]domain.BanzukeEntry, error) {
		return r.Banzuke(ctx, aki2026, domain.DivMakuuchi)
	})
	if len(es) != 42 || !slices.Equal(tr.Answered(), []string{Osada}) {
		t.Fatalf("%d entries from %v", len(es), tr.Answered())
	}
	byID := map[domain.RikishiID]domain.BanzukeEntry{}
	for _, e := range es {
		byID[e.RikishiID] = e
	}
	onosato := byID[8850]
	if onosato.Shikona.En != "Onosato" || onosato.Shikona.Jp != "大の里" || onosato.Rank.Short() != "Y1e" {
		t.Errorf("Onosato: %+v", onosato)
	}
	if r := onosato.Results[12]; r.Outcome != domain.Loss || r.OpponentID != 7 || r.Kimarite != "shitatenage" {
		t.Errorf("Onosato day 13: %+v", r)
	}
	// Kotoshoho (M1e): a fusen loss on day 12, absent after; Hoshoryu
	// absent throughout, without opponents.
	var kotoshoho domain.BanzukeEntry
	for _, e := range es {
		if e.Rank.Short() == "M1e" {
			kotoshoho = e
		}
	}
	if kotoshoho.Results[11].Outcome != domain.FusenLoss || kotoshoho.Results[11].Kimarite != "fusen" || kotoshoho.Results[12] != (domain.DayResult{Outcome: domain.Absent}) {
		t.Errorf("Kotoshoho: %+v", kotoshoho.Results[11:13])
	}
}

// Rule 2: osada's result fills a bout sumo-api has undecided; on an
// earlier day a disagreement goes to sumo-api, on today to osada.
func TestMergeBouts(t *testing.T) {
	undecide := func(match int) func(*source.Current) {
		return func(c *source.Current) {
			for i := range c.Days[12].Bouts[domain.DivMakuuchi] {
				b := &c.Days[12].Bouts[domain.DivMakuuchi][i]
				if b.MatchNo == match {
					b.Winner, b.Kimarite = 0, ""
				}
			}
		}
	}
	s := &sources{current: undecide(19)}
	r := newResolver(t, s, day13)
	bouts, tr := call(t, func(ctx context.Context) ([]domain.Bout, error) {
		return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
	})
	if b := bout(bouts, 19); b.Winner != domain.West || b.Kimarite != "shitatenage" {
		t.Errorf("bout 19 not filled from sumo-api: %+v", b)
	}
	if src := tr.BoutSources(); src[19] != SumoAPI || src[18] != Osada || !slices.Equal(tr.Answered(), []string{Osada, SumoAPI}) {
		t.Errorf("sources %v, answered %v", src, tr.Answered())
	}

	flip := func(bouts []domain.Bout) []domain.Bout {
		for i := range bouts {
			if bouts[i].MatchNo == 19 {
				bouts[i].Winner, bouts[i].Kimarite = domain.East, "yorikiri"
			}
		}
		return bouts
	}
	for _, tt := range []struct {
		name   string
		now    time.Time
		winner domain.Side
	}{
		{"today: osada", day13, domain.West},
		{"an earlier day: sumo-api", day13.AddDate(0, 0, 1), domain.East},
	} {
		s := &sources{torikumi: flip}
		r := newResolver(t, s, tt.now)
		bouts, _ := call(t, func(ctx context.Context) ([]domain.Bout, error) {
			return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
		})
		if b := bout(bouts, 19); b.Winner != tt.winner {
			t.Errorf("%s: winner %v", tt.name, b.Winner)
		}
	}
}

// Rule 2 for the banzuke: sumo-api's entries, with osada's results where
// sumo-api is still pending, and osada's on today when they disagree.
func TestOverlayBanzuke(t *testing.T) {
	pending := func(days ...int) func([]domain.BanzukeEntry) []domain.BanzukeEntry {
		return func(es []domain.BanzukeEntry) []domain.BanzukeEntry {
			for i := range es {
				for _, d := range days {
					es[i].Results[d-1] = domain.DayResult{}
				}
			}
			return es
		}
	}
	s := &sources{banzuke: pending(13)}
	r := newResolver(t, s, day13)
	es, tr := call(t, func(ctx context.Context) ([]domain.BanzukeEntry, error) {
		return r.Banzuke(ctx, aki2026, domain.DivMakuuchi)
	})
	if got := es[0].Results[12]; es[0].RikishiID != 8850 || got.Outcome != domain.Loss || got.OpponentID != 7 {
		t.Errorf("Onosato day 13: %+v", got)
	}
	if !slices.Equal(tr.Answered(), []string{SumoAPI, Osada}) {
		t.Errorf("answered %v", tr.Answered())
	}
	// Nothing pending: osada adds nothing, and is not named.
	s = &sources{}
	_, tr = call(t, func(ctx context.Context) ([]domain.BanzukeEntry, error) {
		return newResolver(t, s, day13).Banzuke(ctx, aki2026, domain.DivMakuuchi)
	})
	if !slices.Equal(tr.Answered(), []string{SumoAPI}) {
		t.Errorf("answered %v", tr.Answered())
	}
	// A disagreement on an earlier day keeps sumo-api's.
	s = &sources{banzuke: func(es []domain.BanzukeEntry) []domain.BanzukeEntry {
		es[0].Results[0] = domain.DayResult{Outcome: domain.Loss, OpponentID: 1}
		return es
	}}
	es, _ = call(t, func(ctx context.Context) ([]domain.BanzukeEntry, error) {
		return newResolver(t, s, day13).Banzuke(ctx, aki2026, domain.DivMakuuchi)
	})
	if es[0].Results[0].Outcome != domain.Loss {
		t.Error("an earlier day took osada's result")
	}
}

// Rule 4 with rule 1: a stale primary answer gives way to a fresh
// secondary, and is served when the secondary fails too.
func TestStale(t *testing.T) {
	s := &sources{sumoStale: true}
	r := newResolver(t, s, day13)
	_, tr := call(t, func(ctx context.Context) (domain.Basho, error) { return r.Basho(ctx, aki2026) })
	if _, _, stale, _ := tr.Responses(tr.Answered()...); !slices.Equal(tr.Answered(), []string{Osada}) || stale {
		t.Errorf("stale sumo-api: answered %v, stale %v", tr.Answered(), stale)
	}
	s.osadaFail = source.ErrUpstream
	b, tr := call(t, func(ctx context.Context) (domain.Basho, error) { return r.Basho(ctx, aki2026) })
	if _, _, stale, _ := tr.Responses(tr.Answered()...); !slices.Equal(tr.Answered(), []string{SumoAPI}) || !stale || b.Start.IsZero() {
		t.Errorf("both down: answered %v, stale %v", tr.Answered(), stale)
	}
}

func TestForced(t *testing.T) {
	s := &sources{}
	r := newResolver(t, s, day13)
	r.Mode = ForceOsada
	_, tr := call(t, func(ctx context.Context) ([]domain.Bout, error) {
		return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
	})
	if !tr.Forced() || len(s.sumoCalls) != 0 {
		t.Errorf("forced osada: forced %v, sumo-api calls %v", tr.Forced(), s.sumoCalls)
	}
	if _, err := r.Torikumi(t.Context(), nagoya, domain.DivMakuuchi, 15); err == nil {
		t.Error("forced osada served a finished tournament")
	}
	r.Mode = ForceSumoAPI
	s.osadaCalls = 0
	_, tr = call(t, func(ctx context.Context) ([]domain.Bout, error) {
		return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
	})
	if !tr.Forced() || s.osadaCalls != 0 || !slices.Equal(tr.Answered(), []string{SumoAPI}) {
		t.Errorf("forced sumo-api: forced %v, osada calls %d, answered %v", tr.Forced(), s.osadaCalls, tr.Answered())
	}
	// Osada disabled: the current tournament from sumo-api.
	r = newResolver(t, &sources{}, day13)
	r.Current = nil
	_, tr = call(t, func(ctx context.Context) ([]domain.Bout, error) {
		return r.Torikumi(ctx, aki2026, domain.DivMakuuchi, 13)
	})
	if tr.Forced() || !slices.Equal(tr.Answered(), []string{SumoAPI}) {
		t.Errorf("osada disabled: %v", tr.Answered())
	}
}

func TestParseMode(t *testing.T) {
	for in, want := range map[string]Mode{"": Auto, "auto": Auto, "sumoapi": ForceSumoAPI, "osada": ForceOsada} {
		if got, err := ParseMode(in); err != nil || got != want {
			t.Errorf("%q: %v, %v", in, got, err)
		}
	}
	if _, err := ParseMode("jsa"); err == nil {
		t.Error("no error")
	}
}
