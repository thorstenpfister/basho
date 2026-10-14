package derive

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

// fight returns a decided bout of wrestler 1 against opp; won is from 1's side.
func fight(basho domain.BashoID, day int, opp domain.RikishiID, won bool, k domain.Kimarite) *domain.Bout {
	b := bout(basho, domain.DivMakuuchi, day, 1)
	b.East.RikishiID, b.West.RikishiID, b.Kimarite = 1, opp, k
	b.Winner = domain.West
	if won {
		b.Winner = domain.East
	}
	return &b
}

func TestLastResults(t *testing.T) {
	career := []*domain.Bout{
		fight(nago, 13, 7, true, "yorikiri"),
		fight(nago, 14, 8, false, "oshidashi"),
		fight(nago, 15, 9, true, fusen),
		// Aki: absent on day 1 (no bout), west side on day 2.
		{
			Basho: aki, Division: domain.DivMakuuchi, Day: 2, MatchNo: 3,
			East: domain.BoutSide{RikishiID: 6}, West: domain.BoutSide{RikishiID: 1}, Winner: domain.West, Kimarite: "hatakikomi",
		},
		fight(aki, 3, 5, false, "uwatenage"),
		{Basho: aki, Division: domain.DivMakuuchi, Day: 4, East: domain.BoutSide{RikishiID: 1}, West: domain.BoutSide{RikishiID: 4}}, // undecided
	}
	got := LastResults(1, career, BeforeDay(aki, 5), 5)
	want := []Result{
		{Basho: nago, Day: 13, Opponent: 7, Won: true, Kimarite: "yorikiri"},
		{Basho: nago, Day: 14, Opponent: 8, Kimarite: "oshidashi"},
		{Basho: nago, Day: 15, Opponent: 9, Won: true, Fusen: true, Kimarite: fusen},
		{Basho: aki, Day: 2, Opponent: 6, Won: true, Kimarite: "hatakikomi"},
		{Basho: aki, Day: 3, Opponent: 5, Kimarite: "uwatenage"},
	}
	if got.New || len(got.Results) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got.Results[i] != want[i] {
			t.Errorf("result %d = %+v, want %+v", i, got.Results[i], want[i])
		}
	}

	// On day 2 of Aki: nothing from Aki yet, the three Nagoya bouts, "new".
	if got := LastResults(1, career, BeforeBout(*career[3]), 5); !got.New || len(got.Results) != 3 || got.Results[2].Basho != nago {
		t.Errorf("into day 2: %+v", got)
	}
	if got := LastResults(1, career, NoCutoff, 2); got.New || got.Results[0].Day != 2 || got.Results[1].Day != 3 {
		t.Errorf("last 2: %+v", got)
	}
	if got := LastResults(1, nil, NoCutoff, 5); !got.New || len(got.Results) != 0 {
		t.Errorf("no career: %+v", got)
	}
}
