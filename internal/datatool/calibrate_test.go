package datatool

import (
	"strings"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
)

// history: wrestler 1 beats 2 every day of three tournaments, then loses
// every day of a fourth; 3 and 4 trade wins.
func testHistory() History {
	y1, m1 := domain.Rank{Title: domain.Yokozuna, Number: 1, Side: domain.East}, domain.Rank{Title: domain.Maegashira, Number: 1, Side: domain.West}
	h := History{Names: map[domain.RikishiID]string{1: "One", 2: "Two", 3: "Three", 4: "Four"}}
	id := domain.BashoID{Year: 1960, Month: time.January}
	for i := range 4 {
		var bouts []domain.Bout
		for day := 1; day <= domain.BashoDays; day++ {
			won := domain.East
			if i == 3 {
				won = domain.West
			}
			bouts = append(bouts,
				domain.Bout{Basho: id, Division: domain.DivMakuuchi, Day: day, MatchNo: 1, East: domain.BoutSide{RikishiID: 1, Rank: y1}, West: domain.BoutSide{RikishiID: 2, Rank: m1}, Winner: won, Kimarite: "yorikiri"},
				domain.Bout{Basho: id, Division: domain.DivMakuuchi, Day: day, MatchNo: 2, East: domain.BoutSide{RikishiID: 3, Rank: m1}, West: domain.BoutSide{RikishiID: 4, Rank: m1}, Winner: domain.Side(1 + day%2), Kimarite: "oshidashi"})
		}
		h.Tournaments = append(h.Tournaments, bouts)
		h.Through = id
		id = id.Next()
	}
	return h
}

func TestPeakRatings(t *testing.T) {
	h := testHistory()
	peaks := PeakRatings(rating.Params{K: 10, KNew: 20, NewBouts: 15, WarmupScale: 1}, h, 2)
	if len(peaks) != 2 || peaks[0].ID != 1 || peaks[0].Name != "One" {
		t.Fatalf("peaks %+v", peaks)
	}
	// The peak is at the end of the third tournament, before the losses.
	if want := (domain.BashoID{Year: 1960, Month: time.May}); peaks[0].Basho != want {
		t.Errorf("peak in %v, want %v", peaks[0].Basho, want)
	}
	if peaks[0].Rating <= peaks[1].Rating {
		t.Errorf("not sorted: %+v", peaks)
	}
}

func TestWriteReport(t *testing.T) {
	h := testHistory()
	split := rating.Split{TrainFrom: h.Tournaments[0][0].Basho, EvalFrom: h.Tournaments[2][0].Basho, Through: h.Through}
	grid := []rating.Params{{K: 10, KNew: 10, NewBouts: 1, WarmupScale: 1}, {K: 20, KNew: 20, NewBouts: 1, WarmupScale: 1}}
	c := Calibration{Split: split, Results: rating.Calibrate(grid, h.Tournaments, split), Baselines: rating.FitBaselines(h.Tournaments, split)}
	c.Peaks = PeakRatings(c.Best().Params, h, 3)
	var sb strings.Builder
	if err := c.WriteReport(&sb); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Rating calibration", "**V9:**", "| Elo (chosen) |", "## Top 5 parameter sets", "| 1 | One |", "Coin flip | 0.6931"} {
		if !strings.Contains(sb.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, sb.String())
		}
	}
	f := c.ParamsFile("test")
	if f.Schema != rating.FileSchema || f.Params != c.Best().Params || f.Show != c.Shown() || f.Eval != c.Best().Eval {
		t.Errorf("params file %+v", f)
	}
}
