package derive

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

func rank(title domain.RankTitle, n int) domain.Rank {
	return domain.Rank{Title: title, Number: n, Side: domain.East}
}

// match is a decided bout between 1 (east) and opp (west); won is from 1's side.
func match(basho domain.BashoID, div domain.Division, day int, mine, theirs domain.Rank, opp domain.RikishiID, won bool, k domain.Kimarite) *domain.Bout {
	b := fight(basho, day, opp, won, k)
	b.Division, b.East.Rank, b.West.Rank = div, mine, theirs
	return b
}

func TestIsKinboshi(t *testing.T) {
	m3, y1, o1, k1, j1 := rank(domain.Maegashira, 3), rank(domain.Yokozuna, 1), rank(domain.Ozeki, 1), rank(domain.Komusubi, 1), rank(domain.Juryo, 1)
	playoff := match(aki, domain.DivMakuuchi, 15, m3, y1, 2, true, "yorikiri")
	playoff.IsPlayoff = true
	tests := []struct {
		name string
		b    *domain.Bout
		want bool
	}{
		{"Maegashira beats Yokozuna", match(aki, domain.DivMakuuchi, 1, m3, y1, 2, true, "yorikiri"), true},
		{"Maegashira loses to Yokozuna", match(aki, domain.DivMakuuchi, 1, m3, y1, 2, false, "yorikiri"), false},
		{"fusen against a Yokozuna", match(aki, domain.DivMakuuchi, 1, m3, y1, 2, true, fusen), false},
		{"draw given a winner", match(aki, domain.DivMakuuchi, 1, m3, y1, 2, true, "hikiwake"), false},
		{"Ozeki is no Yokozuna", match(aki, domain.DivMakuuchi, 1, m3, o1, 2, true, "yorikiri"), false},
		{"Komusubi is no Maegashira", match(aki, domain.DivMakuuchi, 1, k1, y1, 2, true, "yorikiri"), false},
		{"Juryo wrestler in Makuuchi", match(aki, domain.DivMakuuchi, 1, j1, y1, 2, true, "yorikiri"), false},
		{"playoff", playoff, false},
		{"undecided", &domain.Bout{Division: domain.DivMakuuchi, East: domain.BoutSide{Rank: m3}, West: domain.BoutSide{Rank: y1}}, false},
	}
	for _, tt := range tests {
		if _, got := IsKinboshi(tt.b); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
	// From the west: wrestler 2, a Maegashira, beats 1, a Yokozuna.
	if w, ok := IsKinboshi(match(aki, domain.DivMakuuchi, 1, y1, m3, 2, false, "yorikiri")); !ok || w != 2 {
		t.Errorf("west Maegashira over east Yokozuna: %d, %v", w, ok)
	}
}

func TestKinboshiAndScalps(t *testing.T) {
	m3, m1, y1, o1 := rank(domain.Maegashira, 3), rank(domain.Maegashira, 1), rank(domain.Yokozuna, 1), rank(domain.Ozeki, 1)
	career := []*domain.Bout{
		match(nago, domain.DivMakuuchi, 2, m3, y1, 2, true, "oshidashi"), // kinboshi
		match(nago, domain.DivMakuuchi, 3, m3, o1, 3, true, "oshidashi"), // scalp, last tournament
		match(aki, domain.DivMakuuchi, 1, m1, y1, 2, true, "yorikiri"),   // kinboshi, scalp
		match(aki, domain.DivMakuuchi, 2, m1, o1, 3, true, fusen),        // neither
		match(aki, domain.DivMakuuchi, 3, m1, o1, 4, true, "hatakikomi"), // scalp
		match(aki, domain.DivMakuuchi, 4, m1, y1, 5, true, "yorikiri"),   // kinboshi, scalp
	}
	tests := []struct {
		name   string
		c      AsOf
		want   Kinboshi
		scalps int
	}{
		{"before Aki", BeforeDay(aki, 1), Kinboshi{Count: 1}, 0},
		{"into day 4", BeforeBout(*career[5]), Kinboshi{Count: 2, ThisBasho: 1}, 2},
		{"after day 4", AfterDay(aki, 4), Kinboshi{Count: 3, ThisBasho: 2}, 3},
	}
	for _, tt := range tests {
		if got := KinboshiOf(1, career, tt.c); got != tt.want {
			t.Errorf("%s: kinboshi %+v, want %+v", tt.name, got, tt.want)
		}
		if got := Scalps(1, career, tt.c); got != tt.scalps {
			t.Errorf("%s: scalps %d, want %d", tt.name, got, tt.scalps)
		}
	}
	if got := KinboshiOf(2, career, NoCutoff); got.Count != 0 {
		t.Errorf("the Yokozuna has %+v", got)
	}
}
