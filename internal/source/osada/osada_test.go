package osada

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source"
)

const testBase = "https://osada.test"

func fixtureCurrent(t *testing.T) source.Current {
	t.Helper()
	cur, err := New(fixture.Getter{Dir: "../../../testdata/osada", Base: testBase}, testBase).Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return cur
}

func entry(t *testing.T, cur source.Current, div domain.Division, jsa int) source.CurrentEntry {
	t.Helper()
	for _, e := range cur.Banzuke[div] {
		if e.JSAID == jsa {
			return e
		}
	}
	t.Fatalf("no %d in %s", jsa, div)
	return source.CurrentEntry{}
}

func TestCurrent(t *testing.T) {
	cur := fixtureCurrent(t)
	if cur.Basho.String() != "202609" || cur.Start != domain.NewDate(2026, time.September, 13) {
		t.Errorf("tournament %s from %s", cur.Basho, cur.Start)
	}
	if want := time.Date(2026, time.September, 27, 17, 27, 38, 0, domain.JST); !cur.UpdatedAt.Equal(want) || !cur.ResultUpdatedAt.Equal(want) {
		t.Errorf("updated %s, results %s", cur.UpdatedAt, cur.ResultUpdatedAt)
	}
	if n, m := len(cur.Banzuke[domain.DivMakuuchi]), len(cur.Banzuke[domain.DivJuryo]); n != 42 || m != 28 {
		t.Errorf("banzuke %d + %d", n, m)
	}
	if len(cur.Days) != domain.BashoDays {
		t.Fatalf("%d days", len(cur.Days))
	}
	for i, d := range cur.Days {
		if d.Day != i+1 || d.Date != cur.Start.AddDays(i) {
			t.Errorf("day %d on %s", d.Day, d.Date)
		}
	}
}

func TestBanzukeEntries(t *testing.T) {
	cur := fixtureCurrent(t)
	onosato := entry(t, cur, domain.DivMakuuchi, 4227)
	if onosato.Name != (domain.Name{Jp: "大の里", Kana: "おおのさと"}) || onosato.Rank.Short() != "Y1e" {
		t.Errorf("Onosato: %+v", onosato)
	}
	if onosato.Results[3] != domain.Loss || onosato.Results[14] != domain.Win {
		t.Errorf("Onosato's results: %v", onosato.Results)
	}
	// The second Ozeki East is O2e; osada writes the title without a number.
	ranks := map[string]bool{}
	for _, e := range cur.Banzuke[domain.DivMakuuchi] {
		ranks[e.Rank.Short()] = true
	}
	for _, want := range []string{"O1e", "O2e", "O1w", "S1e", "K1w", "M1e", "M17e", "Y1w"} {
		if !ranks[want] {
			t.Errorf("no %s", want)
		}
	}
	if ranks["M17w"] {
		t.Error("M17w is empty this tournament")
	}
	// "draw" is an absence: Hoshoryu missed the tournament, Kotoshoho its
	// last three days.
	hoshoryu := entry(t, cur, domain.DivMakuuchi, 3842)
	for d, r := range hoshoryu.Results {
		if r != domain.Absent {
			t.Errorf("Hoshoryu day %d: %v", d+1, r)
		}
	}
	kotoshoho := entry(t, cur, domain.DivMakuuchi, 3840)
	if kotoshoho.Results[11] != domain.Loss || kotoshoho.Results[12] != domain.Absent {
		t.Errorf("Kotoshoho: %v", kotoshoho.Results)
	}
	if kotoshoho.Rank.Short() != "M1e" {
		t.Errorf("Kotoshoho %s", kotoshoho.Rank.Short())
	}
}

func TestBouts(t *testing.T) {
	cur := fixtureCurrent(t)
	d1 := cur.Days[0].Bouts[domain.DivMakuuchi]
	if len(d1) != 20 {
		t.Fatalf("day 1: %d bouts", len(d1))
	}
	// Day 12: Kotoshoho's fusen loss to Fujiryoga.
	var fusen *source.CurrentBout
	for i, b := range cur.Days[11].Bouts[domain.DivMakuuchi] {
		if b.East.JSAID == 3840 {
			fusen = &cur.Days[11].Bouts[domain.DivMakuuchi][i]
		}
	}
	if fusen == nil || fusen.Winner != domain.West || fusen.Kimarite != "fusen" || fusen.West.JSAID != 4336 || fusen.East.Kana != "ことしょうほう" {
		t.Errorf("day 12 fusen: %+v", fusen)
	}
	// Day 15 Juryo: 14 bouts and the two playoff bouts, decided.
	j15 := cur.Days[14].Bouts[domain.DivJuryo]
	var playoffs []source.CurrentBout
	for _, b := range j15 {
		if b.IsPlayoff {
			playoffs = append(playoffs, b)
		}
	}
	if len(j15) != 16 || len(playoffs) != 2 || playoffs[0].Winner != domain.East || playoffs[1].Kimarite != "hatakikomi" {
		t.Errorf("day 15 Juryo: %d bouts, playoffs %+v", len(j15), playoffs)
	}
	// Ranks come from the banzuke with the side; Makushita visitors keep
	// theirs from the bout list, without one.
	var visitors int
	for _, d := range cur.Days {
		for _, b := range d.Bouts[domain.DivJuryo] {
			for _, s := range []source.CurrentSide{b.East, b.West} {
				switch {
				case s.Rank.Title == domain.Makushita:
					visitors++
					if s.Rank.Side != 0 {
						t.Errorf("visitor %d with a side", s.JSAID)
					}
				case s.Rank.Side == 0:
					t.Errorf("day %d: %d (%s) without a side", d.Day, s.JSAID, s.Kana)
				}
			}
		}
	}
	if visitors == 0 {
		t.Error("no Makushita visitors")
	}
	if a := cur.Days[14].Absentees[domain.DivMakuuchi]; len(a) != 5 || a[0] != 3265 {
		t.Errorf("day 15 absentees %v", a)
	}
}

// A day still to come is the schedule: bouts without winners.
func TestSchedule(t *testing.T) {
	b := banzukeDTO{BashoName: "九月場所", Year: "令和八年", Makuuchi: []banzukeRow{{Title: "横綱", East: []rikishiDTO{{ID: 1, Name: "甲"}}, West: []rikishiDTO{{ID: 2, Name: "乙"}}}}}
	match := matchDTO{BoutNo: 1, EastProfileURL: "https://x/profile/1/", WestProfileURL: "https://x/profile/2/", Kimarite: "未定"}
	day := func(n int, kimarite string, winner *string) dayDTO {
		m := match
		m.Kimarite, m.Winner = kimarite, winner
		d := dayDTO{Day: n, ISODate: domain.NewDate(2026, time.September, 12+n).String(), PathDate: "202609" + "1" + string(rune('2'+n)), Status: "published"}
		d.Data = &struct {
			Makuuchi *divisionDTO `json:"makuuchi"`
			Juryo    *divisionDTO `json:"juryo"`
		}{Makuuchi: &divisionDTO{Matches: []matchDTO{m}}}
		return d
	}
	east := "east"
	t1 := torikumiDTO{
		ResultDays:   []dayDTO{day(1, "寄り切り", &east)},
		ScheduleDays: []dayDTO{day(1, "未定", nil), day(2, "未定", nil), {Day: 3, Status: "unpublished"}},
	}
	cur, err := mapCurrent(b, t1)
	if err != nil {
		t.Fatal(err)
	}
	if len(cur.Days) != 2 {
		t.Fatalf("%d days", len(cur.Days))
	}
	if got := cur.Days[0].Bouts[domain.DivMakuuchi][0]; got.Winner != domain.East || got.Kimarite != "yorikiri" {
		t.Errorf("day 1 took the schedule: %+v", got)
	}
	if got := cur.Days[1].Bouts[domain.DivMakuuchi][0]; got.Winner != 0 || got.Kimarite != "" {
		t.Errorf("day 2: %+v", got)
	}
	// A winner without a technique is still undecided.
	t1.ResultDays = []dayDTO{day(1, "未定", &east)}
	if cur, _ := mapCurrent(b, t1); cur.Days[0].Bouts[domain.DivMakuuchi][0].Winner != 0 {
		t.Error("decided without a technique")
	}
}

// Between tournaments the banzuke can be the next tournament's while the
// bout list is still the last one's: no days then.
func TestNextBanzuke(t *testing.T) {
	cur := fixtureCurrent(t)
	g := fixture.Getter{Dir: "../../../testdata/osada", Base: testBase}
	b, err := New(g, testBase).bodies(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	b.banzuke.BashoName = "十一月場所"
	next, err := mapCurrent(b.banzuke, b.torikumi)
	if err != nil {
		t.Fatal(err)
	}
	if next.Basho.String() != "202611" || len(next.Days) != 0 || !next.Start.IsZero() {
		t.Errorf("next: %s, %d days, start %s", next.Basho, len(next.Days), next.Start)
	}
	if len(next.Banzuke[domain.DivMakuuchi]) != len(cur.Banzuke[domain.DivMakuuchi]) {
		t.Error("lost the banzuke")
	}
}

type fakeGetter struct {
	status int
	body   string
	err    error
}

func (f fakeGetter) Get(context.Context, string, httpx.Validators) (*httpx.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &httpx.Response{Status: f.status, Body: []byte(f.body)}, nil
}

func TestErrors(t *testing.T) {
	for _, tt := range []struct {
		get  fakeGetter
		want error
	}{
		{fakeGetter{err: errors.New("dial")}, source.ErrUpstream},
		{fakeGetter{status: 500}, source.ErrUpstream},
		{fakeGetter{status: 404}, source.ErrNotFound},
		{fakeGetter{status: 429}, source.ErrRateLimited},
		{fakeGetter{status: 200, body: "{"}, source.ErrUpstream},
		{fakeGetter{status: 200, body: `{"bashoName":"秋場所","year":"令和八年"}`}, source.ErrBadData},
		{fakeGetter{status: 200, body: `{"bashoName":"九月場所","year":"令和八年"}`}, source.ErrNotYetAvailable},
	} {
		if _, err := New(tt.get, testBase).Current(t.Context()); !errors.Is(err, tt.want) {
			t.Errorf("%+v: %v; want %v", tt.get, err, tt.want)
		}
	}
}

func TestJSAID(t *testing.T) {
	for url, want := range map[string]int{
		"https://www.sumo.or.jp/ResultRikishiData/profile/3553/": 3553,
		"https://www.sumo.or.jp/ResultRikishiData/profile/4227":  4227,
	} {
		if got, err := JSAID(url); err != nil || got != want {
			t.Errorf("JSAID(%q) = %d, %v", url, got, err)
		}
	}
	if _, err := JSAID("https://www.sumo.or.jp/"); err == nil {
		t.Error("no error")
	}
}
