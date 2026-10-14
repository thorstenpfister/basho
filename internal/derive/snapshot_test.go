package derive_test

import (
	"encoding/json"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/data"
	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
)

// Acceptance tests against the embedded snapshot (SPEC §17 Phase 4).

var aki2026 = domain.BashoID{Year: 2026, Month: time.September}

var index = sync.OnceValues(func() (*snapshot.Index, error) {
	s, err := data.Snapshot()
	if err != nil {
		return nil, err
	}
	return s.BuildIndex()
})

func embedded(t *testing.T) (*snapshot.Snapshot, *snapshot.Index) {
	t.Helper()
	s, err := data.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	x, err := index()
	if err != nil {
		t.Fatal(err)
	}
	return s, x
}

// Standings after day 15 match the source's own final totals, which
// sumo-api keeps apart from the day-by-day record.
func TestStandingsMatchFinalRecords(t *testing.T) {
	s, x := embedded(t)
	b, _ := x.Basho(aki2026)
	d := derive.NewDayBouts(aki2026, x.Tournament(aki2026))
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		raw, err := os.ReadFile("../../testdata/sumoapi/basho_202609_banzuke_" + div.String() + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var totals struct {
			East, West []struct {
				RikishiID              domain.RikishiID
				Wins, Losses, Absences int
			}
		}
		if err := json.Unmarshal(raw, &totals); err != nil {
			t.Fatal(err)
		}
		entries, err := s.Banzuke(aki2026, div)
		if err != nil {
			t.Fatal(err)
		}
		st := derive.StandingsAt(entries, d, derive.EndOf(aki2026))
		rows := map[domain.RikishiID]derive.Record{}
		for _, r := range st.Rows {
			rows[r.RikishiID] = r.Record
		}
		for _, w := range append(totals.East, totals.West...) {
			want := derive.Record{Wins: w.Wins, Losses: w.Losses, Absences: w.Absences}
			if got := rows[w.RikishiID]; got != want {
				t.Errorf("%s %d: %v, want %v", div, w.RikishiID, got, want)
			}
		}
		if len(rows) != len(totals.East)+len(totals.West) {
			t.Errorf("%s: %d rows, %d on the banzuke", div, len(rows), len(totals.East)+len(totals.West))
		}
		// Makuuchi was decided in regular bouts; Juryo went to a three-way
		// playoff at 11-4 (playoffs are not in sumo-api's bout lists).
		var leaders []domain.RikishiID
		for _, r := range st.Rows {
			if r.Band == derive.Leaders {
				leaders = append(leaders, r.RikishiID)
			}
		}
		switch {
		case div == domain.DivMakuuchi && (!st.YushoDecided || leaders[0] != b.Yusho[div]):
			t.Errorf("Makuuchi: leaders %v (decided %v), yusho %d", leaders, st.YushoDecided, b.Yusho[div])
		case div == domain.DivJuryo && (!st.Playoff || len(leaders) != 3 || !slices.Contains(leaders, b.Yusho[div])):
			t.Errorf("Juryo: leaders %v (playoff %v), yusho %d", leaders, st.Playoff, b.Yusho[div])
		}
	}
}

// Career kinboshi totals as published (Wikipedia, checked 2026-10-14).
func TestKinboshiTotals(t *testing.T) {
	_, x := embedded(t)
	for id, want := range map[domain.RikishiID]int{
		3833: 16, // Akinoshima, the record
		1014: 12, // Takamiyama
		3847: 12, // Tochinonada
		3857: 11, // Tosanoumi
	} {
		if got := derive.KinboshiOf(id, x.Rikishi(id), derive.NoCutoff); got.Count != want {
			t.Errorf("wrestler %d: %d kinboshi, want %d", id, got.Count, want)
		}
	}
}

// The fighter-type thresholds against the career profiles they were tuned
// on (SPEC §6.7).
func TestFighterTypes(t *testing.T) {
	_, x := embedded(t)
	for id, want := range map[domain.RikishiID]derive.FighterType{
		1:    derive.PusherThruster, // Takakeisho
		3142: derive.BeltFighter,    // Kotoshogiku
		1511: derive.BeltFighter,    // Taiho
		3081: derive.Thrower,        // Hakuho
		45:   derive.BeltFighter,    // Terunofuji
	} {
		if got := derive.ProfileOf(id, x.Rikishi(id), derive.NoCutoff, derive.Career).Type; got != want {
			t.Errorf("wrestler %d: %v, want %v", id, got, want)
		}
	}
}
