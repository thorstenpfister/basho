package derive

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

// division is a small Makuuchi: records as letters, and the bouts they imply
// for the days given (opponents do not matter here, so each wrestler fights
// a ghost).
func division(days int, recs map[domain.RikishiID]string, ranks map[domain.RikishiID]domain.Rank) ([]domain.BanzukeEntry, DayBouts) {
	var entries []domain.BanzukeEntry
	var bouts []domain.Bout
	for id, rec := range recs {
		entries = append(entries, entry(id, ranks[id], rec))
		for day := 1; day <= days; day++ {
			switch rec[day-1] {
			case 'W', 'L', 'F', 'f':
				b := bout(aki, domain.DivMakuuchi, day, int(id))
				b.East.RikishiID, b.West.RikishiID = id, 1000+id
				bouts = append(bouts, b)
			}
		}
	}
	return entries, NewDayBouts(aki, bouts)
}

func TestStandings(t *testing.T) {
	ranks := map[domain.RikishiID]domain.Rank{
		1: rank(domain.Yokozuna, 1), 2: rank(domain.Ozeki, 1), 3: rank(domain.Sekiwake, 1),
		4: rank(domain.Maegashira, 1), 5: rank(domain.Maegashira, 2), 6: rank(domain.Maegashira, 3),
	}
	entries, d := division(15, map[domain.RikishiID]string{
		1: "WWWWWWWWWWWWWLW", // 14-1
		2: "WWWWWWWWWWWWLLW", // 13-2
		3: "WWWWWWWWWWWLLLL", // 11-4
		4: "LLLLLLLWWWWWWWW", // 8-7, kachi-koshi on day 15
		5: "WWWWWWWAAAAAAAA", // 7-0-8, make-koshi on day 15
		6: "WWWWWLLLLLLLWWF", // 8-7 with a fusen win on day 15
	}, ranks)

	s := StandingsAt(entries, d, AfterDay(aki, 12))
	order := []domain.RikishiID{1, 2, 3, 5, 4, 6} // 4 and 6 are both 5-7: rank decides
	for i, id := range order {
		if s.Rows[i].RikishiID != id {
			t.Fatalf("after day 12, row %d is %d; want order %v", i, s.Rows[i].RikishiID, order)
		}
	}
	if s.Today != 12 || s.Rows[0].Record.String() != "12-0" || s.Rows[0].Remaining != 3 {
		t.Errorf("after day 12: today %d, leader %v with %d left", s.Today, s.Rows[0].Record, s.Rows[0].Remaining)
	}
	// Yokozuna and Ozeki are both 12-0.
	if s.Rows[1].Band != Leaders || s.Rows[2].Band != OneBack || s.Rows[3].Band != Rest {
		t.Errorf("bands %v %v %v", s.Rows[1].Band, s.Rows[2].Band, s.Rows[3].Band)
	}
	if s.YushoDecided || s.Playoff {
		t.Error("after day 12 nothing is decided")
	}
	if !s.Rows[2].InContention || s.Rows[3].InContention {
		t.Errorf("contention: 11-1 %v, 7-0-5 %v", s.Rows[2].InContention, s.Rows[3].InContention)
	}

	s = StandingsAt(entries, d, AfterDay(aki, 14))
	if s.Rows[0].RikishiID != 1 || s.Rows[0].Record.String() != "13-1" || s.YushoDecided {
		t.Errorf("after day 14: %v %v decided %v; the Ozeki can still tie", s.Rows[0].RikishiID, s.Rows[0].Record, s.YushoDecided)
	}

	s = StandingsAt(entries, d, EndOf(aki))
	if !s.YushoDecided || s.Playoff {
		t.Errorf("final: decided %v, playoff %v", s.YushoDecided, s.Playoff)
	}
	if s.KachiKoshi != 5 || s.MakeKoshi != 1 || s.Kyujo != 1 {
		t.Errorf("final: KK %d, MK %d, kyujo %d", s.KachiKoshi, s.MakeKoshi, s.Kyujo)
	}
	for _, row := range s.Rows {
		wantNew := row.RikishiID == 4 || row.RikishiID == 5 || row.RikishiID == 6
		if row.NewToday != wantNew {
			t.Errorf("final: %d new today %v, want %v", row.RikishiID, row.NewToday, wantNew)
		}
		if row.Kyujo != (row.RikishiID == 5) {
			t.Errorf("final: %d kyujo %v", row.RikishiID, row.Kyujo)
		}
	}
	grid := s.ByBanzuke()
	for i, row := range grid {
		if row.RikishiID != domain.RikishiID(i+1) {
			t.Errorf("grid row %d is %d", i, row.RikishiID)
		}
	}
}

func TestStandingsDecidedEarlyAndPlayoff(t *testing.T) {
	ranks := map[domain.RikishiID]domain.Rank{1: rank(domain.Yokozuna, 1), 2: rank(domain.Ozeki, 1), 3: rank(domain.Sekiwake, 1)}
	entries, d := division(15, map[domain.RikishiID]string{
		1: "WWWWWWWWWWWWWWW",
		2: "WWWWWWWWWWLLLLL",
		3: "LLLLLLLLLLLLLLL",
	}, ranks)
	// 13-0 against at most 10-3 + 2: decided on day 13.
	if s := StandingsAt(entries, d, AfterDay(aki, 13)); !s.YushoDecided {
		t.Error("13-0 against 10-3 is decided")
	}
	// 12-0 against 10-2 + 3, which could tie.
	if s := StandingsAt(entries, d, AfterDay(aki, 12)); s.YushoDecided {
		t.Error("12-0 against 10-2 is not decided")
	}

	entries, d = division(15, map[domain.RikishiID]string{
		1: "WWWWWWWWWWWWLLW",
		2: "WWWWWWWWWWWWLWL",
		3: "LLLLLLLLLLLLLLL",
	}, ranks)
	if s := StandingsAt(entries, d, EndOf(aki)); !s.Playoff || s.YushoDecided {
		t.Errorf("13-2 and 13-2: playoff %v, decided %v", s.Playoff, s.YushoDecided)
	}
	if s := StandingsAt(entries, d, AfterDay(aki, 14)); s.Playoff {
		t.Error("a playoff before the last day")
	}
}

func TestStandingsMidDay(t *testing.T) {
	ranks := map[domain.RikishiID]domain.Rank{1: rank(domain.Yokozuna, 1), 2: rank(domain.Maegashira, 5)}
	entries, d := division(15, map[domain.RikishiID]string{
		1: "WWWWWWWWWWWWWWW", // match 1 each day
		2: "WWWWWWWLLLLLLLL", // match 2
	}, ranks)
	// Through match 1 of day 8: the Yokozuna's day 8 counts, the other's not.
	s := StandingsAt(entries, d, ThroughBout(*d.Bout(1, 8)))
	if s.Today != 8 {
		t.Errorf("today %d", s.Today)
	}
	for _, row := range s.Rows {
		switch row.RikishiID {
		case 1:
			if row.Record.Wins != 8 || !row.NewToday || row.Results[7].Outcome != domain.Win {
				t.Errorf("Yokozuna %+v", row)
			}
		case 2:
			if row.Record.String() != "7-0" || row.Remaining != 8 || row.Results[7].Outcome != domain.Pending {
				t.Errorf("Maegashira %+v", row)
			}
		}
	}
	if s := StandingsAt(nil, d, NoCutoff); len(s.Rows) != 0 || s.YushoDecided {
		t.Errorf("empty division: %+v", s)
	}
}
