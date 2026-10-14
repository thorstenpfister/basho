package derive

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestIsUpset(t *testing.T) {
	y1, o1, s1, k1 := rank(domain.Yokozuna, 1), rank(domain.Ozeki, 1), rank(domain.Sekiwake, 1), rank(domain.Komusubi, 1)
	m1, m5, m10, j1 := rank(domain.Maegashira, 1), rank(domain.Maegashira, 5), rank(domain.Maegashira, 10), rank(domain.Juryo, 1)
	tests := []struct {
		name          string
		winner, loser domain.Rank
		k             domain.Kimarite
		want          bool
	}{
		{"M1 over Yokozuna", m1, y1, "yorikiri", true},
		{"Sekiwake over Yokozuna", s1, y1, "yorikiri", true},
		{"Komusubi over Ozeki", k1, o1, "yorikiri", true},
		{"Ozeki over Yokozuna", o1, y1, "yorikiri", false},
		{"M5 over M1", m5, m1, "yorikiri", false},
		{"M10 over M1", m10, m1, "yorikiri", true},
		{"Juryo visitor over M10", j1, m10, "yorikiri", true},
		{"by default", m1, y1, fusen, false},
		{"the favourite wins", y1, m1, "yorikiri", false},
	}
	for _, tt := range tests {
		b := match(aki, domain.DivMakuuchi, 1, tt.winner, tt.loser, 2, true, tt.k)
		if got := IsUpset(b); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestDayRecap(t *testing.T) {
	ranks := map[domain.RikishiID]domain.Rank{1: rank(domain.Yokozuna, 1), 2: rank(domain.Maegashira, 1), 3: rank(domain.Maegashira, 2), 4: rank(domain.Maegashira, 3)}
	entries := []domain.BanzukeEntry{
		entry(1, ranks[1], "WWWWWWWWL"),
		entry(2, ranks[2], "WWWWWWWLW"), // beats the Yokozuna on day 9: kinboshi, 8 wins
		entry(3, ranks[3], "LLLLLLLWL"), // make-koshi on day 9
		entry(4, ranks[4], "WWWWWWWWW"),
	}
	day := func(d, n int, east, west domain.RikishiID, k domain.Kimarite) domain.Bout {
		b := bout(aki, domain.DivMakuuchi, d, n)
		b.East = domain.BoutSide{RikishiID: east, Rank: ranks[east]}
		b.West = domain.BoutSide{RikishiID: west, Rank: ranks[west]}
		b.Winner, b.Kimarite = domain.West, k
		return b
	}
	bouts := []domain.Bout{
		day(8, 1, 3, 2, "yorikiri"),  // day 8: 3 (east) beats 2, set below
		day(9, 1, 3, 4, "oshidashi"), // 4 wins
		day(9, 2, 1, 2, "ipponzeoi"), // 2 beats the Yokozuna by a rare throw
		{Basho: aki, Division: domain.DivJuryo, Day: 9, MatchNo: 1, Winner: domain.East, Kimarite: "zubuneri"},
	}
	bouts[0].Winner = domain.East
	d := NewDayBouts(aki, bouts)
	c := AfterDay(aki, 9)
	s := StandingsAt(entries, d, c)
	freq := KimariteFreq{}
	freq.Add([]domain.Bout{{Winner: domain.East, Kimarite: "oshidashi"}, {Winner: domain.East, Kimarite: "oshidashi"}, {Winner: domain.East, Kimarite: "ipponzeoi"}, {Kimarite: "ipponzeoi"}})
	if freq["oshidashi"] != 2 || freq["ipponzeoi"] != 1 {
		t.Fatalf("freq = %v", freq)
	}

	r := DayRecap(s, domain.DivMakuuchi, bouts, c, freq)
	if r.Day != 9 || len(r.Leaders) != 1 || r.Leaders[0].RikishiID != 4 {
		t.Errorf("day %d, leaders %+v", r.Day, r.Leaders)
	}
	if len(r.KachiKoshi) != 1 || r.KachiKoshi[0].RikishiID != 2 || len(r.MakeKoshi) != 1 || r.MakeKoshi[0].RikishiID != 3 {
		t.Errorf("KK %+v MK %+v", r.KachiKoshi, r.MakeKoshi)
	}
	if len(r.Kinboshi) != 1 || r.Kinboshi[0].MatchNo != 2 || len(r.Upsets) != 1 {
		t.Errorf("kinboshi %+v upsets %+v", r.Kinboshi, r.Upsets)
	}
	if r.Technique == nil || r.Technique.Kimarite != "ipponzeoi" || r.TechniqueCount != 1 {
		t.Errorf("technique %+v ×%d; Juryo's must not count", r.Technique, r.TechniqueCount)
	}

	// Through match 1 of day 9 only: the kinboshi has not happened yet.
	c = ThroughBout(bouts[1])
	r = DayRecap(StandingsAt(entries, d, c), domain.DivMakuuchi, bouts, c, freq)
	if len(r.Kinboshi) != 0 || r.Technique.Kimarite != "oshidashi" {
		t.Errorf("spoiler-safe recap: %+v", r)
	}
}
