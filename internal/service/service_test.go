package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source"
)

var (
	kyushu2026 = domain.BashoID{Year: 2026, Month: time.November}
	testNow    = time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)
)

// afterSnapshot is the first tournament the embedded snapshot doesn't hold,
// so the tests that need one survive each data release.
var afterSnapshot = sync.OnceValue(func() domain.BashoID {
	b, err := Bundled()
	if err != nil {
		panic(err)
	}
	return b.Newest.Next()
})

// embedded is a service over the embedded snapshot with no remote layers;
// it is built once, as the bout index and ratings take a moment.
var embedded = sync.OnceValue(func() *Service {
	return New(Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
})

func TestDay(t *testing.T) {
	res, err := embedded().Day(t.Context(), aki2026, domain.DivMakuuchi, 13)
	if err != nil {
		t.Fatal(err)
	}
	d := res.Value
	if res.Prov.Source != SnapshotSource || d.Status != Complete || len(d.Bouts) != 19 || d.Date != domain.NewDate(2026, time.September, 25) {
		t.Fatalf("day 13: %s, %v, %d bouts on %v", res.Prov.Source, d.Status, len(d.Bouts), d.Date)
	}
	last := d.Bouts[18]
	if !last.Musubi || last.East.Shikona.En != "Onosato" || last.East.Before.String() != "11-1" || last.East.After.String() != "11-2" ||
		last.Winner != domain.West || last.Kimarite.Kanji != "下手投げ" || last.WinChance == nil {
		t.Errorf("musubi %+v", last)
	}
	if d.Bouts[0].Musubi {
		t.Error("bout 1 marked musubi")
	}
	kyujo := map[string]bool{}
	for _, w := range d.Kyujo {
		kyujo[w.Shikona.En] = true
	}
	if !kyujo["Hoshoryu"] {
		t.Errorf("kyujo %v lacks Hoshoryu", kyujo)
	}
	if _, err := embedded().Day(t.Context(), aki2026, domain.DivMakuuchi, 16); !errors.Is(err, source.ErrNotFound) {
		t.Errorf("day 16: %v", err)
	}
}

func TestBout(t *testing.T) {
	res, err := embedded().Bout(t.Context(), aki2026, domain.DivMakuuchi, 13, 19)
	if err != nil {
		t.Fatal(err)
	}
	b := res.Value
	e, w := b.East, b.West
	if !b.Musubi || b.Of != 19 || e.Shikona.En != "Onosato" || e.Record.String() != "11-1" || w.RikishiID != 7 {
		t.Fatalf("tale %+v", b)
	}
	if !e.Profile || e.Heya == "" || e.Career.Age == 0 || e.Career.Height == 0 || e.Career.Yusho[domain.DivMakuuchi] == 0 {
		t.Errorf("Onosato's profile %+v", e)
	}
	if len(e.Last5.Results) != 5 || e.Moveset().Wins == 0 || e.Rating == nil || b.WinChance == nil {
		t.Errorf("Onosato's form %+v", e)
	}
	// Nothing before the result may count the bout itself.
	if h := b.HeadToHead; h.A.Wins+h.B.Wins == 0 || len(h.Meetings) == 0 || h.Meetings[0].Basho == aki2026 && h.Meetings[0].Day == 13 {
		t.Errorf("head-to-head %+v", h)
	}
	if last := e.Last5.Results[4]; last.Basho == aki2026 && last.Day >= 13 {
		t.Errorf("last 5 includes the bout: %+v", last)
	}
	if r := b.Result; r == nil || r.Winner != domain.West || r.East.String() != "11-2" || r.Technique.Kimarite != "shitatenage" {
		t.Errorf("result %+v", b.Result)
	}
	if _, err := embedded().Bout(t.Context(), aki2026, domain.DivMakuuchi, 13, 99); !errors.Is(err, source.ErrNotFound) {
		t.Errorf("bout 99: %v", err)
	}
}

func TestStandingsAndRecap(t *testing.T) {
	res, err := embedded().Standings(t.Context(), aki2026, domain.DivMakuuchi, 15)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Value
	if s.Rows[0].Shikona.En != "Onosato" || s.Rows[0].Record.String() != "12-3" || !s.YushoDecided || s.Yusho == nil || s.Yusho.RikishiID != 8850 {
		t.Errorf("standings top %+v, decided %v, yusho %+v", s.Rows[0], s.YushoDecided, s.Yusho)
	}
	if res, _ := embedded().Standings(t.Context(), aki2026, domain.DivMakuuchi, 13); res.Value.Yusho != nil {
		t.Error("the yusho shown after day 13")
	}
	rec, err := embedded().Recap(t.Context(), aki2026, domain.DivMakuuchi, 13)
	if err != nil {
		t.Fatal(err)
	}
	if r := rec.Value; r.Day != 13 || len(r.Leaders) == 0 || r.Technique == nil || r.Technique.English == "" {
		t.Errorf("recap %+v", r)
	}
}

func TestInfoAndList(t *testing.T) {
	res, err := embedded().Info(t.Context(), aki2026)
	if err != nil {
		t.Fatal(err)
	}
	in := res.Value
	// Makushita's winner (Inami) is named: he reached Juryo later, so the
	// snapshot's index has him.
	if in.Status != Finished || in.Name != "Aki" || len(in.Yusho) != 3 || in.Yusho[0].Division != domain.DivMakuuchi || in.Yusho[0].Shikona.En != "Onosato" || in.Yusho[2].Shikona.En != "Inami" || len(in.Prizes) != 2 {
		t.Errorf("info %+v", in)
	}
	cancelled, _ := embedded().Info(t.Context(), domain.BashoID{Year: 2020, Month: time.May})
	if cancelled.Value.Status != Cancelled {
		t.Errorf("202005: %v", cancelled.Value.Status)
	}

	list, err := embedded().List(t.Context(), 2020, 2020)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Value) != 6 || list.Value[0].Basho != (domain.BashoID{Year: 2020, Month: time.November}) || list.Value[3].Status != Cancelled {
		t.Errorf("2020: %+v", list.Value)
	}
	if y := list.Value[0].Yusho; y == nil || y.Shikona.En == "" || !list.Value[0].InSnapshot {
		t.Errorf("202011 %+v", list.Value[0])
	}
	all, _ := embedded().List(t.Context(), 0, 0)
	if first := all.Value[0]; first.Basho != aki2026 { // Kyushu 2026 has not started on 2026-10-14
		t.Errorf("newest %v", first.Basho)
	}
}

func TestDefaultDay(t *testing.T) {
	fake := &fakeRemote{start: domain.NewDate(2026, time.November, 8)}
	for _, tt := range []struct {
		now  time.Time
		want int
	}{
		{time.Date(2026, time.November, 1, 12, 0, 0, 0, domain.JST), 1},
		{time.Date(2026, time.November, 12, 12, 0, 0, 0, domain.JST), 5},
		{time.Date(2026, time.November, 30, 12, 0, 0, 0, domain.JST), 15},
	} {
		s := New(Options{Remote: fake, Clock: clock.NewFake(tt.now)})
		if got, err := s.DefaultDay(t.Context(), kyushu2026); err != nil || got != tt.want {
			t.Errorf("on %v: day %d, %v; want %d", tt.now, got, err, tt.want)
		}
	}
	if got, _ := embedded().DefaultDay(t.Context(), aki2026); got != 15 {
		t.Errorf("a finished tournament: day %d", got)
	}
}

// fakeRemote serves a made-up Kyushu 2026: two Makuuchi wrestlers who meet
// every day, and the East one wins.
type fakeRemote struct {
	start domain.Date
	calls int
}

var (
	fakeY1 = domain.Rank{Title: domain.Yokozuna, Number: 1, Side: domain.East}
	fakeM1 = domain.Rank{Title: domain.Maegashira, Number: 1, Side: domain.West}
)

func (f *fakeRemote) Basho(_ context.Context, id domain.BashoID) (domain.Basho, error) {
	f.calls++
	return domain.Basho{ID: id, Start: f.start, End: f.start.AddDays(14)}, nil
}

func (f *fakeRemote) Banzuke(_ context.Context, _ domain.BashoID, div domain.Division) ([]domain.BanzukeEntry, error) {
	f.calls++
	if div != domain.DivMakuuchi {
		return nil, nil
	}
	a := domain.BanzukeEntry{RikishiID: 8850, Shikona: domain.Name{En: "Onosato"}, Rank: fakeY1}
	b := domain.BanzukeEntry{RikishiID: 7, Shikona: domain.Name{En: "Kirishima"}, Rank: fakeM1}
	a.Results[0] = domain.DayResult{Outcome: domain.Win, OpponentID: 7, Kimarite: "yorikiri"}
	b.Results[0] = domain.DayResult{Outcome: domain.Loss, OpponentID: 8850, Kimarite: "yorikiri"}
	return []domain.BanzukeEntry{a, b}, nil
}

func (f *fakeRemote) Torikumi(_ context.Context, id domain.BashoID, div domain.Division, day int) ([]domain.Bout, error) {
	f.calls++
	if day > 1 || div != domain.DivMakuuchi {
		return nil, source.ErrNotYetAvailable
	}
	return []domain.Bout{{
		Basho: id, Division: div, Day: 1, MatchNo: 1, Winner: domain.East, Kimarite: "yorikiri",
		East: domain.BoutSide{RikishiID: 8850, Shikona: domain.Name{En: "Onosato"}, Rank: fakeY1},
		West: domain.BoutSide{RikishiID: 7, Shikona: domain.Name{En: "Kirishima"}, Rank: fakeM1},
	}}, nil
}

func TestLaterTournament(t *testing.T) {
	later := afterSnapshot()
	fake := &fakeRemote{start: later.EstimatedStart()}
	s := New(Options{Snapshot: data.Snapshot, Params: data.RatingParams, Remote: fake, Clock: clock.NewFake(testNow)})
	res, err := s.Day(t.Context(), later, domain.DivMakuuchi, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Prov.Source != "sumo-api" || fake.calls != 3 || len(res.Value.Bouts) != 1 {
		t.Errorf("day 1: %+v after %d calls", res.Prov, fake.calls)
	}
	// The day went into the bout index and the ratings.
	x, _ := s.index()
	if len(x.Tournament(later)) != 1 {
		t.Error("not in the bout index")
	}
	r, _ := s.Ratings()
	before := r.At(8850, fakeY1, derive.BeforeDay(later, 1)).Value
	if after := r.At(8850, fakeY1, derive.NoCutoff).Value; after <= before {
		t.Errorf("rating %.1f → %.1f after a win", before, after)
	}
	// Onosato's last 5 crosses into the snapshot.
	tale, err := s.Bout(t.Context(), later, domain.DivMakuuchi, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if l := tale.Value.East.Last5.Results; len(l) != 5 || l[4].Basho.Compare(later) >= 0 {
		t.Errorf("last 5 %+v", l)
	}
	if _, err := s.Day(t.Context(), later, domain.DivMakuuchi, 2); !errors.Is(err, source.ErrNotYetAvailable) {
		t.Errorf("day 2: %v", err)
	}

	snapOnly := New(Options{Snapshot: data.Snapshot, Params: data.RatingParams, Clock: clock.NewFake(testNow)})
	if _, err := snapOnly.Day(t.Context(), later, domain.DivMakuuchi, 1); !errors.Is(err, source.ErrNotYetAvailable) {
		t.Errorf("without a remote: %v", err)
	}
	noSnap := New(Options{Remote: fake, Clock: clock.NewFake(testNow)})
	if res, err := noSnap.Day(t.Context(), aki2026, domain.DivMakuuchi, 1); err != nil || res.Prov.Source == SnapshotSource {
		t.Errorf("--no-snapshot: %+v, %v", res.Prov, err)
	}
}

type fakeGetter struct{ resp httpx.Response }

func (g fakeGetter) Get(context.Context, string, httpx.Validators) (*httpx.Response, error) {
	r := g.resp
	return &r, nil
}

func TestProvenance(t *testing.T) {
	old := testNow.Add(-2 * time.Hour)
	for _, tt := range []struct {
		name string
		resp httpx.Response
		want Provenance
	}{
		{"live", httpx.Response{Status: 200}, Provenance{Source: "sumo-api", FetchedAt: testNow}},
		{"cached", httpx.Response{Status: 200, Cached: true, FetchedAt: old}, Provenance{Source: "cache:sumo-api", FetchedAt: old}},
		{"stale", httpx.Response{Status: 200, Cached: true, Stale: true, FetchedAt: old}, Provenance{Source: "cache:sumo-api", FetchedAt: old, Stale: true}},
	} {
		g := Recording(fakeGetter{tt.resp}, func() time.Time { return testNow })
		p, _, err := tracked(t.Context(), "sumo-api", testNow, func(ctx context.Context) error {
			_, err := g.Get(ctx, "u", httpx.Validators{})
			return err
		})
		if err != nil || p != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, p, tt.want)
		}
	}
	snap := Provenance{Source: SnapshotSource, FetchedAt: old}
	cached := Provenance{Source: "cache:sumo-api", FetchedAt: testNow, Stale: true}
	if got := merge(snap, cached); got.Source != "cache:sumo-api" || !got.FetchedAt.Equal(old) || !got.Stale {
		t.Errorf("merge %+v", got)
	}
	if got := merge(Provenance{}, snap); got != snap {
		t.Errorf("merge with nothing %+v", got)
	}
	// Two sources as lively are both named; forcing spreads.
	for _, tt := range []struct{ a, b, want string }{
		{"sumo-api", "osada", "sumo-api + osada"},
		{"osada + sumo-api", "sumo-api", "osada + sumo-api"},
		{"cache:sumo-api", "cache:osada", "cache:sumo-api + osada"},
		{"cache:osada", "sumo-api", "sumo-api"},
	} {
		got := merge(Provenance{Source: tt.a, Forced: true}, Provenance{Source: tt.b})
		if got.Source != tt.want || !got.Forced {
			t.Errorf("merge %q, %q: %+v", tt.a, tt.b, got)
		}
	}
}

func TestCurrentBasho(t *testing.T) {
	if got := embedded().CurrentBasho(); got != aki2026 {
		t.Errorf("current on 2026-10-14: %v", got)
	}
}

type fakeWrestlers struct{ calls int }

func (f *fakeWrestlers) ActiveRikishi(context.Context) ([]domain.Rikishi, error) {
	f.calls++
	return []domain.Rikishi{{ID: 8850, NSKID: 4227, Name: domain.Name{En: "Onosato"}, Heya: "New heya", Height: 193}}, nil
}

func TestRefreshWrestlers(t *testing.T) {
	sn, err := data.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	built := sn.Manifest().GeneratedAt
	for _, tt := range []struct {
		name    string
		now     time.Time
		refresh bool
	}{
		{"index fresh", built.Add(6 * 24 * time.Hour), false},
		{"index older than 7 days", built.Add(8 * 24 * time.Hour), true},
	} {
		w := &fakeWrestlers{}
		s := New(Options{Snapshot: data.Snapshot, Params: data.RatingParams, Wrestlers: w, Clock: clock.NewFake(tt.now)})
		got, err := s.RefreshWrestlers(t.Context())
		if err != nil || got != tt.refresh || w.calls != b2i(tt.refresh) {
			t.Errorf("%s: refreshed %v after %d requests, %v", tt.name, got, w.calls, err)
		}
		ws, _ := s.wrestlers()
		if heya := ws[8850].Heya; (heya == "New heya") != tt.refresh {
			t.Errorf("%s: Onosato's heya %q", tt.name, heya)
		}
		if len(ws) < 900 {
			t.Errorf("%s: %d wrestlers; the overlay must keep the snapshot's", tt.name, len(ws))
		}
		ids, _ := s.IDs()
		if id, ok := ids.ByJSA(4227); tt.refresh && (!ok || id != 8850) {
			t.Errorf("%s: JSA 4227 → %d, %v", tt.name, id, ok)
		}
	}
	if got, _ := embedded().RefreshWrestlers(t.Context()); got {
		t.Error("refreshed without a source")
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestErrorClasses(t *testing.T) {
	for _, tt := range []struct {
		err                   error
		notFound, unavailable bool
	}{
		{source.ErrNotFound, true, false},
		{fmt.Errorf("day 3: %w", source.ErrNotYetAvailable), true, false},
		{fmt.Errorf("x: %w", source.ErrUpstream), false, true},
		{source.ErrRateLimited, false, true},
		{fmt.Errorf("u: %w: %w", source.ErrUpstream, cache.ErrOffline), false, true},
		{httpx.ErrCircuitOpen, false, true},
		{errors.New("bug"), false, false},
	} {
		if NotFound(tt.err) != tt.notFound || Unavailable(tt.err) != tt.unavailable {
			t.Errorf("%v: not found %v, unavailable %v", tt.err, NotFound(tt.err), Unavailable(tt.err))
		}
	}
}

func TestDayStatuses(t *testing.T) {
	res, err := embedded().DayStatuses(t.Context(), aki2026, domain.DivMakuuchi)
	if err != nil {
		t.Fatal(err)
	}
	for i, st := range res.Value {
		if st != Complete {
			t.Errorf("Aki day %d: %v", i+1, st)
		}
	}
	gap, _ := embedded().DayStatuses(t.Context(), domain.BashoID{Year: 1965, Month: time.March}, domain.DivMakuuchi)
	if gap.Value[6] != NotPublished || gap.Value[5] != Complete {
		t.Errorf("196503 (no bouts on day 7 in the source): %v", gap.Value)
	}
	fake := &fakeRemote{start: domain.NewDate(2026, time.November, 8)}
	s := New(Options{Remote: fake, Clock: clock.NewFake(time.Date(2026, time.November, 12, 16, 0, 0, 0, domain.JST))})
	live, err := s.DayStatuses(t.Context(), kyushu2026, domain.DivMakuuchi)
	if err != nil {
		t.Fatal(err)
	}
	want := [domain.BashoDays]DayStatus{Complete, Complete, Complete, Complete, InProgress, Scheduled}
	if live.Value != want || fake.calls != 1 {
		t.Errorf("day 5 of Kyushu: %v after %d calls", live.Value, fake.calls)
	}
}

// An unknown JSA ID refreshes the active wrestlers, revalidated past the
// cache, at most once an hour (SPEC §5.5).
func TestUnknownJSAID(t *testing.T) {
	w := &revalidated{}
	clk := clock.NewFake(time.Date(2026, time.November, 8, 15, 0, 0, 0, domain.JST))
	s := New(Options{Snapshot: data.Snapshot, Params: data.RatingParams, Wrestlers: w, Clock: clk})
	ids, err := s.IDs()
	if err != nil {
		t.Fatal(err)
	}
	if got := ids.Resolve(t.Context(), 4227, "", "おおのさと"); got.ID != 8850 || w.calls != 0 {
		t.Errorf("Onosato: %+v after %d requests", got, w.calls)
	}
	got := ids.Resolve(t.Context(), 9999, "新人", "しんじん")
	if got.ID != 9100 || got.Temporary || w.calls != 1 || !w.revalidate {
		t.Errorf("newcomer: %+v after %d requests (revalidated %v)", got, w.calls, w.revalidate)
	}
	if ws, _ := s.wrestlers(); ws[9100].Heya != "Newheya" {
		t.Error("the refresh did not reach the profiles")
	}
	ids.Resolve(t.Context(), 9998, "", "べつ")
	clk.Advance(30 * time.Minute)
	ids.Resolve(t.Context(), 9997, "", "べつ")
	if w.calls != 1 {
		t.Errorf("%d requests within the hour", w.calls)
	}
}

type revalidated struct {
	calls      int
	revalidate bool
}

func (f *revalidated) ActiveRikishi(ctx context.Context) ([]domain.Rikishi, error) {
	f.calls++
	f.revalidate = cache.Revalidating(ctx)
	return []domain.Rikishi{{ID: 9100, NSKID: 9999, Name: domain.Name{En: "Shinjin"}, Heya: "Newheya"}}, nil
}

// The help screen's rating numbers come from params.json without computing
// a single rating.
func TestRatingModel(t *testing.T) {
	m, err := embedded().RatingModel()
	if err != nil {
		t.Fatal(err)
	}
	if !m.Shown || m.Bouts < 30000 || m.EvalFrom.Year != 2015 || m.Accuracy <= m.RankAccuracy || m.LogLoss >= m.RankLogLoss {
		t.Errorf("%+v", m)
	}
}
