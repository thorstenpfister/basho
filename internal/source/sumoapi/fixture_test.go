package sumoapi

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/source"
)

// These tests run the adapter against responses recorded from sumo-api.com
// (make fixtures, SPEC §13).

const fixtureDir = "../../../testdata/sumoapi"

func fixtureClient() *Client {
	return New(fixture.Getter{Dir: fixtureDir, Base: DefaultBaseURL}, DefaultBaseURL)
}

var (
	b196001 = domain.BashoID{Year: 1960, Month: time.January}
	b202607 = domain.BashoID{Year: 2026, Month: time.July}
	b202609 = domain.BashoID{Year: 2026, Month: time.September}
)

// TestBanzukeRecordsMatchAPITotals checks the outcome mapping against the
// totals sumo-api computes itself, for a modern and an old-era tournament.
func TestBanzukeRecordsMatchAPITotals(t *testing.T) {
	c := fixtureClient()
	for _, tt := range []struct {
		id      domain.BashoID
		div     domain.Division
		entries int
	}{
		{b202609, domain.DivMakuuchi, 42},
		{b202609, domain.DivJuryo, 28},
		{b196001, domain.DivMakuuchi, 43},
		{b196001, domain.DivJuryo, 36}, // 18 Juryo ranks per side in 1960
		{b202607, domain.DivMakuuchi, 42},
	} {
		got, err := c.Banzuke(t.Context(), tt.id, tt.div)
		if err != nil {
			t.Fatalf("%s %s: %v", tt.id, tt.div, err)
		}
		if len(got) != tt.entries {
			t.Errorf("%s %s: %d entries, want %d", tt.id, tt.div, len(got), tt.entries)
		}
		totals := apiTotals(t, tt.id, tt.div)
		for _, e := range got {
			var w, l, a int
			for _, r := range e.Results {
				switch {
				case r.Outcome.IsWin():
					w++
				case r.Outcome.IsLoss():
					l++
				case r.Outcome == domain.Absent:
					a++
				case r.Outcome != domain.Pending:
					t.Errorf("%s: %s has outcome %s", tt.id, e.Shikona.En, r.Outcome)
				}
			}
			if want := totals[e.RikishiID]; [3]int{w, l, a} != want {
				t.Errorf("%s %s: W-L-A %d-%d-%d, API says %v", tt.id, e.Shikona.En, w, l, a, want)
			}
			if e.Rank.Title.Division() != tt.div {
				t.Errorf("%s: %s ranked %s in %s", tt.id, e.Shikona.En, e.Rank, tt.div)
			}
		}
	}
}

// apiTotals reads sumo-api's own wins/losses/absences from the fixture.
func apiTotals(t *testing.T, id domain.BashoID, div domain.Division) map[domain.RikishiID][3]int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, fixture.Name("/api/basho/"+id.String()+"/banzuke/"+div.String())))
	if err != nil {
		t.Fatal(err)
	}
	type entry struct {
		RikishiID              int `json:"rikishiID"`
		Wins, Losses, Absences int
	}
	var dto struct{ East, West []entry }
	if err := json.Unmarshal(b, &dto); err != nil {
		t.Fatal(err)
	}
	m := map[domain.RikishiID][3]int{}
	for _, e := range append(dto.East, dto.West...) {
		m[domain.RikishiID(e.RikishiID)] = [3]int{e.Wins, e.Losses, e.Absences}
	}
	return m
}

func TestTorikumiFixtures(t *testing.T) {
	c := fixtureClient()
	for _, tt := range []struct {
		id    domain.BashoID
		div   domain.Division
		day   int
		bouts int
	}{
		{b202609, domain.DivMakuuchi, 1, 0},
		{b202609, domain.DivMakuuchi, 15, 19},
		{b202609, domain.DivJuryo, 15, 14}, // the Juryo playoff is not in sumo-api's list (V2)
		{b202607, domain.DivMakuuchi, 15, 20},
		{b196001, domain.DivMakuuchi, 15, 21},
	} {
		got, err := c.Torikumi(t.Context(), tt.id, tt.div, tt.day)
		if err != nil {
			t.Fatalf("%s %s day %d: %v", tt.id, tt.div, tt.day, err)
		}
		if tt.bouts != 0 && len(got) != tt.bouts {
			t.Errorf("%s %s day %d: %d bouts, want %d", tt.id, tt.div, tt.day, len(got), tt.bouts)
		}
		for i, b := range got {
			if b.MatchNo != i+1 || b.Winner == 0 {
				t.Errorf("%s day %d bout %d: MatchNo %d, Winner %v", tt.id, tt.day, i+1, b.MatchNo, b.Winner)
			}
			if _, ok := b.Kimarite.Info(); !ok {
				t.Errorf("%s day %d bout %d: unknown kimarite %q", tt.id, tt.day, b.MatchNo, b.Kimarite)
			}
		}
	}
}

func TestBashoFixtures(t *testing.T) {
	c := fixtureClient()
	b, err := c.Basho(t.Context(), b202609)
	if err != nil || b.Start.String() != "2026-09-13" || b.Yusho[domain.DivMakuuchi] != 8850 {
		t.Errorf("202609 = %+v, %v", b, err)
	}
	old, err := c.Basho(t.Context(), b196001)
	if err != nil || old.Location != "Tokyo, Kuramae Kokugikan" || old.Start.String() != "1960-01-10" {
		t.Errorf("196001 = %+v, %v", old, err)
	}
	if _, err := c.Basho(t.Context(), b202609.Next()); !errors.Is(err, source.ErrNotYetAvailable) {
		t.Errorf("202611 (recorded before its banzuke): err = %v, want ErrNotYetAvailable", err)
	}
}

func TestRikishiIndexFixture(t *testing.T) {
	c := fixtureClient()
	// Only the first page is recorded, so read it directly; RikishiIndex
	// itself would go on to page 2 (9132 wrestlers in total).
	var page rikishiPageDTO
	if err := c.getJSON(t.Context(), "/api/rikishis?intai=true&limit=1000&skip=0", &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1000 || page.Total <= 1000 {
		t.Errorf("page: %d records of %d", len(page.Records), page.Total)
	}
	withNSK := 0
	for _, dto := range page.Records {
		if mapRikishi(dto).NSKID != 0 {
			withNSK++
		}
	}
	if withNSK == 0 {
		t.Error("no wrestler has a JSA id: the osada.us mapping (SPEC §5.5) would fail")
	}
}
