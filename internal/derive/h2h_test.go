package derive

import (
	"slices"
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

func TestHeadToHead(t *testing.T) {
	m1, y1 := rank(domain.Maegashira, 1), rank(domain.Yokozuna, 1)
	west := match(aki, domain.DivMakuuchi, 6, m1, y1, 2, true, "hatakikomi")
	west.East, west.West = west.West, west.East // 1 on the west side
	west.Winner = domain.West
	career := []*domain.Bout{
		match(nago, domain.DivMakuuchi, 1, m1, y1, 2, false, "yorikiri"),
		match(nago, domain.DivMakuuchi, 2, m1, y1, 3, true, "oshidashi"), // someone else
		match(nago, domain.DivMakuuchi, 9, m1, y1, 2, true, "oshidashi"),
		match(nago, domain.DivMakuuchi, 12, m1, y1, 2, true, fusen),
		match(aki, domain.DivMakuuchi, 3, m1, y1, 2, false, "yorikiri"),
		west,
		match(aki, domain.DivMakuuchi, 9, m1, y1, 2, true, "oshidashi"), // after the cutoff
	}
	h := HeadToHeadOf(1, 2, career, BeforeDay(aki, 9), 3)
	if h.A.Wins != 3 || h.B.Wins != 2 {
		t.Errorf("wins %d-%d, want 3-2 (fusen counts)", h.A.Wins, h.B.Wins)
	}
	if want := []KimariteCount{{"hatakikomi", 1}, {"oshidashi", 1}}; !slices.Equal(h.A.Top, want) {
		t.Errorf("A's top %v, want %v (fusen left out, ties by name)", h.A.Top, want)
	}
	if want := []KimariteCount{{"yorikiri", 2}}; !slices.Equal(h.B.Top, want) {
		t.Errorf("B's top %v, want %v", h.B.Top, want)
	}
	want := []Meeting{
		{Basho: aki, Day: 6, Winner: 1, Kimarite: "hatakikomi", RankA: m1, RankB: y1},
		{Basho: aki, Day: 3, Winner: 2, Kimarite: "yorikiri", RankA: m1, RankB: y1},
		{Basho: nago, Day: 12, Winner: 1, Kimarite: fusen, RankA: m1, RankB: y1},
	}
	if !slices.Equal(h.Meetings, want) {
		t.Errorf("meetings\n%+v\nwant\n%+v", h.Meetings, want)
	}
	if h := HeadToHeadOf(1, 9, career, NoCutoff, 3); h.A.Wins+h.B.Wins != 0 || h.Meetings != nil {
		t.Errorf("never met: %+v", h)
	}
}

func TestTop(t *testing.T) {
	got := top(map[domain.Kimarite]int{"yorikiri": 3, "oshidashi": 5, "hatakikomi": 3, "uwatenage": 1}, 3)
	if want := []KimariteCount{{"oshidashi", 5}, {"hatakikomi", 3}, {"yorikiri", 3}}; !slices.Equal(got, want) {
		t.Errorf("top = %v, want %v", got, want)
	}
	if got := top(nil, 2); len(got) != 0 {
		t.Errorf("top of nothing = %v", got)
	}
}
