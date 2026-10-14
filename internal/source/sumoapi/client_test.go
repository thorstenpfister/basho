package sumoapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source"
)

// fakeAPI answers paths with canned responses; shapes follow SPEC Appendix A.1.
type fakeAPI map[string]*httpx.Response

func (f fakeAPI) Get(_ context.Context, url string, _ httpx.Validators) (*httpx.Response, error) {
	path := strings.TrimPrefix(url, testBase)
	if resp, ok := f[path]; ok {
		if resp == nil {
			return nil, errors.New("connection refused")
		}
		return resp, nil
	}
	return &httpx.Response{Status: 404}, nil
}

const testBase = "https://sumo.test"

func (f fakeAPI) client() *Client { return New(f, testBase) }

func ok(body string) *httpx.Response { return &httpx.Response{Status: 200, Body: []byte(body)} }

var aki = domain.BashoID{Year: 2026, Month: time.September}

func TestBasho(t *testing.T) {
	c := fakeAPI{
		"/api/basho/202609": ok(`{"date":"202609","startDate":"2026-09-13T00:00:00Z","endDate":"2026-09-27T00:00:00Z",
			"yusho":[{"type":"Makuuchi","rikishiId":8850,"shikonaEn":"Onosato"},{"type":"Juryo","rikishiId":66},{"type":"Sandanme","rikishiId":612}],
			"specialPrizes":[{"type":"Kanto-sho","rikishiId":615}]}`),
		"/api/basho/202611": ok(`{"date":"","startDate":"0001-01-01T00:00:00Z","endDate":"0001-01-01T00:00:00Z"}`),
	}.client()

	b, err := c.Basho(t.Context(), aki)
	if err != nil {
		t.Fatal(err)
	}
	if b.Start.String() != "2026-09-13" || b.End.String() != "2026-09-27" {
		t.Errorf("dates = %s – %s", b.Start, b.End)
	}
	if len(b.Yusho) != 2 || b.Yusho[domain.DivMakuuchi] != 8850 || b.Yusho[domain.DivJuryo] != 66 {
		t.Errorf("Yusho = %v, want Makuuchi and Juryo only", b.Yusho)
	}
	if len(b.Prizes) != 1 || b.Prizes[0] != (domain.Prize{Type: "Kanto-sho", RikishiID: 615}) {
		t.Errorf("Prizes = %v", b.Prizes)
	}

	if _, err := c.Basho(t.Context(), aki.Next()); !errors.Is(err, source.ErrNotYetAvailable) {
		t.Errorf("zero object: err = %v, want ErrNotYetAvailable", err)
	}
}

func TestBanzuke(t *testing.T) {
	c := fakeAPI{
		"/api/basho/202609/banzuke/Makuuchi": ok(`{"east":[{"side":"East","rikishiID":8850,"shikonaEn":"Onosato","shikonaJp":"大の里　泰輝","rankValue":101,"rank":"Yokozuna 1 East",
			"record":[{"result":"win","opponentID":9,"kimarite":"hatakikomi"},{"result":"loss","opponentID":10,"kimarite":"yorikiri"},
			{"result":"fusen win","opponentID":11,"kimarite":"fusen"},{"result":"fusen loss","opponentID":12,"kimarite":"fusen"},
			{"result":"absent","opponentID":0,"kimarite":""},{"result":"","opponentID":0,"kimarite":""},{"result":"hikiwake?","opponentID":13,"kimarite":"hikiwake"}]}],
			"west":[{"side":"West","rikishiID":19,"shikonaEn":"Hoshoryu","shikonaJp":"豊昇龍　智勝","rank":"Yokozuna 1 West","record":[]}]}`),
	}.client()

	got, err := c.Banzuke(t.Context(), aki, domain.DivMakuuchi)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].RikishiID != 19 || got[1].Rank != (domain.Rank{Title: domain.Yokozuna, Number: 1, Side: domain.West}) {
		t.Fatalf("entries = %+v", got)
	}
	e := got[0]
	if e.Shikona != (domain.Name{En: "Onosato", Jp: "大の里"}) {
		t.Errorf("Shikona = %+v", e.Shikona)
	}
	want := []domain.Outcome{domain.Win, domain.Loss, domain.FusenWin, domain.FusenLoss, domain.Absent, domain.Pending, domain.Unknown}
	for day, o := range want {
		if e.Results[day].Outcome != o {
			t.Errorf("day %d: Outcome = %s, want %s", day+1, e.Results[day].Outcome, o)
		}
	}
	if e.Results[0] != (domain.DayResult{Outcome: domain.Win, OpponentID: 9, Kimarite: "hatakikomi"}) {
		t.Errorf("day 1 = %+v", e.Results[0])
	}
	if e.Results[7].Outcome != domain.Pending {
		t.Error("days beyond the record must stay Pending")
	}
}

func TestTorikumi(t *testing.T) {
	c := fakeAPI{
		"/api/basho/202609/torikumi/Makuuchi/15": ok(`{"date":"202609","torikumi":[
			{"day":15,"matchNo":1,"eastId":49,"eastShikona":"Shonannoumi","eastRank":"Maegashira 17 East","westId":8865,"westShikona":"Kazuma","westRank":"Juryo 2 West","kimarite":"oshidashi","winnerId":8865},
			{"day":15,"matchNo":2,"eastId":1,"eastShikona":"A","eastRank":"Maegashira 16 West","westId":2,"westShikona":"B","westRank":"Maegashira 15 East","kimarite":"","winnerId":0}]}`),
		"/api/basho/202609/torikumi/Makuuchi/16": ok(`{"date":"202609","torikumi":[]}`),
	}.client()

	got, err := c.Torikumi(t.Context(), aki, domain.DivMakuuchi, 15)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("bouts = %d", len(got))
	}
	b := got[0]
	if b.Basho != aki || b.Division != domain.DivMakuuchi || b.Day != 15 || b.MatchNo != 1 ||
		b.Winner != domain.West || b.Kimarite != "oshidashi" || b.West.Rank.Title != domain.Juryo || b.East.Shikona.En != "Shonannoumi" {
		t.Errorf("bout 1 = %+v", b)
	}
	if got[1].Winner != 0 {
		t.Errorf("an undecided bout has winner %s", got[1].Winner)
	}
	if _, err := c.Torikumi(t.Context(), aki, domain.DivMakuuchi, 16); !errors.Is(err, source.ErrNotYetAvailable) {
		t.Errorf("day 16: err = %v, want ErrNotYetAvailable", err)
	}
}

func TestRikishiIndexPages(t *testing.T) {
	page := func(from, n, total int) *httpx.Response {
		var recs []string
		for i := from; i < from+n; i++ {
			recs = append(recs, fmt.Sprintf(`{"id":%d,"nskId":%d,"shikonaEn":"R%d","shikonaJp":"名%d　名","debut":"202305","birthDate":"2000-06-07T00:00:00Z"}`, i, 4000+i, i, i))
		}
		return ok(fmt.Sprintf(`{"limit":1000,"skip":%d,"total":%d,"records":[%s]}`, from, total, strings.Join(recs, ",")))
	}
	c := fakeAPI{
		"/api/rikishis?intai=true&limit=1000&skip=0":    page(1, 1000, 1500),
		"/api/rikishis?intai=true&limit=1000&skip=1000": page(1001, 500, 1500),
	}.client()
	got, err := c.RikishiIndex(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1500 {
		t.Fatalf("wrestlers = %d, want 1500 from two pages", len(got))
	}
	r := got[0]
	if r.ID != 1 || r.NSKID != 4001 || r.Name.Jp != "名1" || r.Debut != (domain.BashoID{Year: 2023, Month: time.May}) || r.Birth.String() != "2000-06-07" {
		t.Errorf("first = %+v", r)
	}
	active, err := fakeAPI{"/api/rikishis?limit=1000&skip=0": page(1, 601, 601)}.client().ActiveRikishi(t.Context())
	if err != nil || len(active) != 601 {
		t.Errorf("active: %d, %v; want 601 from one page without intai", len(active), err)
	}
}

func TestErrorMapping(t *testing.T) {
	c := fakeAPI{
		"/api/basho/196001": {Status: 429},
		"/api/basho/196003": {Status: 503},
		"/api/basho/196005": nil, // transport error
		"/api/basho/196007": ok(`{not json`),
	}.client()
	tests := map[string]error{
		"196001": source.ErrRateLimited,
		"196003": source.ErrUpstream,
		"196005": source.ErrUpstream,
		"196007": source.ErrUpstream,
		"196009": source.ErrNotFound, // fake answers 404
	}
	for id, want := range tests {
		b, _ := domain.ParseBashoID(id)
		if _, err := c.Basho(t.Context(), b); !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", id, err, want)
		}
	}
}

func TestMappingRejectsBadData(t *testing.T) {
	c := fakeAPI{
		"/api/basho/202609/banzuke/Juryo":    ok(`{"east":[{"rikishiID":1,"rank":"Shogun 1 East"}]}`),
		"/api/basho/202609/torikumi/Juryo/1": ok(`{"torikumi":[{"matchNo":1,"eastId":1,"eastRank":"Juryo 1 East","westId":2,"westRank":"Juryo 1 West","winnerId":3}]}`),
	}.client()
	if _, err := c.Banzuke(t.Context(), aki, domain.DivJuryo); !errors.Is(err, source.ErrBadData) {
		t.Errorf("unknown rank: err = %v", err)
	}
	if _, err := c.Torikumi(t.Context(), aki, domain.DivJuryo, 1); !errors.Is(err, source.ErrBadData) {
		t.Errorf("winner of neither side: err = %v", err)
	}
}
