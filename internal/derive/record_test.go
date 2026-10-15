package derive

import (
	"fmt"
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

// results turns a record string into banzuke results, one letter per day as
// in the snapshot (SPEC Appendix D): W L F f A D, "." pending.
func results(rec string) [domain.BashoDays]domain.DayResult {
	letters := map[rune]domain.Outcome{
		'.': domain.Pending, 'W': domain.Win, 'L': domain.Loss, 'F': domain.FusenWin,
		'f': domain.FusenLoss, 'A': domain.Absent, 'D': domain.Draw, 'U': domain.Unknown,
	}
	var out [domain.BashoDays]domain.DayResult
	for i, r := range rec {
		out[i].Outcome = letters[r]
	}
	return out
}

func entry(id domain.RikishiID, rank domain.Rank, rec string) domain.BanzukeEntry {
	return domain.BanzukeEntry{RikishiID: id, Shikona: domain.Name{En: fmt.Sprintf("R%d", id)}, Rank: rank, Results: results(rec)}
}

func TestRecordString(t *testing.T) {
	for _, tt := range []struct {
		r    Record
		want string
	}{
		{Record{}, "0-0"},
		{Record{Wins: 5, Losses: 7}, "5-7"},
		{Record{Wins: 5, Losses: 7, Absences: 3}, "5-7-3"},
		{Record{Wins: 7, Losses: 7, Draws: 1}, "7-7 1d"},
	} {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("%+v = %q, want %q", tt.r, got, tt.want)
		}
	}
	if !(Record{Wins: 8}).KachiKoshi() || (Record{Wins: 7}).KachiKoshi() {
		t.Error("kachi-koshi is 8 wins")
	}
	if !(Record{Losses: 3, Absences: 5}).MakeKoshi() || (Record{Losses: 7}).MakeKoshi() {
		t.Error("make-koshi is 8 losses and absences")
	}
}

func TestRecordAt(t *testing.T) {
	// Wrestler 1 fights wrestler 2 every day; day 6 is a fusen win, 7 and 8
	// absences, day 10 has no bout (pending) and day 15 adds a playoff.
	m1 := domain.Rank{Title: domain.Maegashira, Number: 1, Side: domain.East}
	e := entry(1, m1, "WLWWWFAAW.LWWWW")
	var bouts []domain.Bout
	for day := 1; day <= domain.BashoDays; day++ {
		if o := e.Results[day-1].Outcome; o == domain.Absent || o == domain.Pending {
			continue
		}
		b := bout(aki, domain.DivMakuuchi, day, 5)
		b.East.RikishiID, b.West.RikishiID = 1, 2
		bouts = append(bouts, b)
	}
	playoff := bout(aki, domain.DivMakuuchi, 15, 1)
	playoff.East.RikishiID, playoff.West.RikishiID, playoff.IsPlayoff = 1, 3, true
	bouts = append(bouts, playoff)
	d := NewDayBouts(aki, bouts)
	day := func(n int) domain.Bout { return *d.Bout(1, n) }

	tests := []struct {
		name string
		c    AsOf
		want string
	}{
		{"before the tournament", BeforeDay(aki, 1), "0-0"},
		{"into day 2", BeforeBout(day(2)), "1-0"},
		{"after day 2", ThroughBout(day(2)), "1-1"},
		{"fusen counts as a win", AfterDay(aki, 6), "5-1"},
		{"absences apart", AfterDay(aki, 8), "5-1-2"},
		{"absent day counts once the day started", BeforeBout(bout(aki, domain.DivMakuuchi, 8, 1)), "5-1-2"},
		{"no bout on day 10", AfterDay(aki, 10), "6-1-2"},
		{"into day 15", BeforeBout(day(15)), "9-2-2"},
		{"into the playoff: day 15 included", BeforeBout(playoff), "10-2-2"},
		{"the playoff does not count", EndOf(aki), "10-2-2"},
		{"an earlier tournament's cutoff", EndOf(nago), "0-0"},
		{"everything", NoCutoff, "10-2-2"},
	}
	for _, tt := range tests {
		if got := RecordAt(e, d, tt.c).String(); got != tt.want {
			t.Errorf("%s: %s, want %s", tt.name, got, tt.want)
		}
	}
	if d.Bout(3, 15) != nil {
		t.Error("playoffs are not day bouts")
	}
}

// RecordThrough counts the days a cutoff includes: a bout's day by the bout,
// a day without one (absent) by the day.
func TestRecordThrough(t *testing.T) {
	id := domain.BashoID{Year: 2026, Month: 9}
	var outcomes [domain.BashoDays]domain.Outcome
	outcomes[0], outcomes[1], outcomes[2] = domain.Win, domain.Loss, domain.Absent
	bouts := map[int]*domain.Bout{
		1: {Basho: id, Day: 1, Division: domain.DivMakuuchi, MatchNo: 3},
		2: {Basho: id, Day: 2, Division: domain.DivMakuuchi, MatchNo: 7},
	}
	boutOf := func(day int) *domain.Bout { return bouts[day] }
	for _, tt := range []struct {
		c    AsOf
		want string
	}{
		{BeforeDay(id, 1), "0-0"},
		{AsOf{Basho: id, Day: 2, Division: domain.DivMakuuchi, MatchNo: 5, Inclusive: true}, "1-0"}, // before day 2's bout
		{AfterDay(id, 2), "1-1"},
		{AfterDay(id, 3), "1-1-1"},
		{EndOf(id), "1-1-1"},
	} {
		if got := RecordThrough(id, outcomes, boutOf, tt.c).String(); got != tt.want {
			t.Errorf("%+v: %s, want %s", tt.c, got, tt.want)
		}
	}
}
