package derive

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Edge cases across the derived views (SPEC §17 Phase 4 step 10). Wrestler 1
// is the subject throughout.

// Absent days 4–8, back on day 9: the absences count apart, last 5 skips
// them, and kyujo ends with the return.
func TestEdgeAbsenceThenReturn(t *testing.T) {
	m5 := rank(domain.Maegashira, 5)
	e := entry(1, m5, "WLWAAAAAWWL....")
	var career []*domain.Bout
	var bouts []domain.Bout
	for day, letter := range "WLW.....WWL" {
		if letter == '.' {
			continue
		}
		b := fight(aki, day+1, domain.RikishiID(10+day), letter == 'W', "yorikiri")
		career = append(career, b)
		bouts = append(bouts, *b)
	}
	d := NewDayBouts(aki, bouts)

	for _, tt := range []struct {
		day   int
		rec   string
		kyujo bool
	}{
		{3, "2-1", false},
		{6, "2-1-3", true},
		{8, "2-1-5", true},
		{9, "3-1-5", false},
		{11, "4-2-5", false},
	} {
		c := AfterDay(aki, tt.day)
		if got := RecordAt(e, d, c).String(); got != tt.rec {
			t.Errorf("after day %d: %s, want %s", tt.day, got, tt.rec)
		}
		s := StandingsAt([]domain.BanzukeEntry{e}, d, c)
		if s.Rows[0].Kyujo != tt.kyujo || s.Kyujo != b2i(tt.kyujo) {
			t.Errorf("after day %d: kyujo %v, want %v", tt.day, s.Rows[0].Kyujo, tt.kyujo)
		}
	}
	// Into day 10: the last 5 bouts skip the absence (days 1, 2, 3, 9).
	last := LastResults(1, career, BeforeDay(aki, 10), 5)
	if len(last.Results) != 5-1 || !last.New || last.Results[3].Day != 9 || last.Results[2].Day != 3 {
		t.Errorf("last 5 across the absence: %+v", last)
	}
	// Absent again from day 12: 4-2-9, make-koshi by the absences.
	e = entry(1, m5, "WLWAAAAAWWLAAAA")
	if r := RecordAt(e, d, EndOf(aki)); !r.MakeKoshi() || r.String() != "4-2-9" {
		t.Errorf("final %v, make-koshi %v", r, r.MakeKoshi())
	}
}

// A fusen win over a Yokozuna: a win in the record and in the head-to-head,
// but no kinboshi, no scalp, no upset, and no technique in any profile.
func TestEdgeFusenAgainstYokozuna(t *testing.T) {
	m1, y1 := rank(domain.Maegashira, 1), rank(domain.Yokozuna, 1)
	b := match(aki, domain.DivMakuuchi, 5, m1, y1, 2, true, fusen)
	career := []*domain.Bout{b}
	e := entry(1, m1, "....F")
	d := NewDayBouts(aki, []domain.Bout{*b})

	if r := RecordAt(e, d, EndOf(aki)); r.Wins != 1 {
		t.Errorf("record %v", r)
	}
	if k := KinboshiOf(1, career, NoCutoff); k.Count != 0 {
		t.Errorf("kinboshi %+v", k)
	}
	if n := Scalps(1, career, EndOf(aki)); n != 0 || IsUpset(b) {
		t.Errorf("scalps %d, upset %v", n, IsUpset(b))
	}
	if h := HeadToHeadOf(1, 2, career, NoCutoff, 3); h.A.Wins != 1 || len(h.A.Top) != 0 {
		t.Errorf("head-to-head %+v", h)
	}
	if p := ProfileOf(1, career, NoCutoff, Career); p.Wins != 0 {
		t.Errorf("profile counts the fusen: %+v", p)
	}
	if last := LastResults(1, career, NoCutoff, 5); !last.Results[0].Fusen || !last.Results[0].Won {
		t.Errorf("last 5: %+v", last)
	}
}

// A Juryo wrestler fills a gap in Makuuchi: the bout belongs to their Juryo
// record and follows Makuuchi's order of play, and a win over a Yokozuna there
// is no kinboshi.
func TestEdgeJuryoInMakuuchi(t *testing.T) {
	j1, m17, y1 := rank(domain.Juryo, 1), rank(domain.Maegashira, 17), rank(domain.Yokozuna, 1)
	up := match(aki, domain.DivMakuuchi, 6, j1, m17, 2, true, "oshidashi") // match 1 of Makuuchi
	juryoLast := bout(aki, domain.DivJuryo, 6, 14)                         // Juryo's last bout, earlier that day
	juryoLast.East.RikishiID, juryoLast.West.RikishiID, juryoLast.Winner = 3, 4, domain.East
	d := NewDayBouts(aki, []domain.Bout{*up, juryoLast})
	e := entry(1, j1, ".....W")

	if r := RecordAt(e, d, ThroughBout(juryoLast)); r.Wins != 0 {
		t.Errorf("counted before the Makuuchi bout was fought: %v", r)
	}
	if r := RecordAt(e, d, ThroughBout(*up)); r.Wins != 1 {
		t.Errorf("not counted after it: %v", r)
	}
	s := StandingsAt([]domain.BanzukeEntry{e}, d, ThroughBout(*up))
	if s.Rows[0].Rank != j1 || s.Rows[0].Record.Wins != 1 {
		t.Errorf("Juryo standings row %+v", s.Rows[0])
	}
	if _, ok := IsKinboshi(match(aki, domain.DivMakuuchi, 7, j1, y1, 5, true, "yorikiri")); ok {
		t.Error("a Juryo wrestler's win over a Yokozuna is no kinboshi")
	}
}

// Fewer than 10 sekitori wins: style and signature give way to "not enough
// data", and a short career's last 5 is marked new.
func TestEdgeNewlyPromoted(t *testing.T) {
	career := careerOf(map[domain.BashoID][]string{aki: {"oshidashi", "-yorikiri", "oshidashi"}})
	if p := ProfileOf(1, career, NoCutoff, LastSix); p.Enough() || p.Type != NoType || p.Signature.Kimarite != "oshidashi" {
		t.Errorf("profile %+v", p)
	}
	if last := LastResults(1, career, NoCutoff, 5); !last.New || len(last.Results) != 3 {
		t.Errorf("last 5 %+v", last)
	}
}
