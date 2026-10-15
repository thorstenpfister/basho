package help

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

func TestPages(t *testing.T) {
	m := &service.RatingModel{
		Shown: true, EvalFrom: domain.BashoID{Year: 2015, Month: 1}, Through: domain.BashoID{Year: 2026, Month: 9},
		Bouts: 34759, Accuracy: 0.562, RankAccuracy: 0.540,
	}
	for _, width := range []int{56, 76, 116} {
		pages := Pages(theme.New(true, false), nil, m, width)
		var titles []string
		for _, p := range pages {
			titles = append(titles, p.Title)
			for i, l := range strings.Split(p.Body, "\n") {
				if w := ansi.StringWidth(l); w > width {
					t.Errorf("%s at %d: line %d is %d wide: %q", p.Title, width, i, w, ansi.Strip(l))
				}
			}
		}
		if got := strings.Join(titles, " "); got != "Keys Legend Rating Data Limitations" {
			t.Errorf("pages %q", got)
		}
		rating := strings.Join(strings.Fields(ansi.Strip(pages[2].Body)), " ")
		if !strings.Contains(rating, "Tested on 34759 bouts from 2015 to Aki 2026") || !strings.Contains(rating, "the favourite won 56% of them; the higher rank won 54%") {
			t.Errorf("rating page at %d:\n%s", width, pages[2].Body)
		}
	}
	m.Shown = false
	if body := Pages(theme.New(true, false), nil, m, 76)[2].Body; !strings.Contains(body, "basho hides it") {
		t.Errorf("a hidden rating:\n%s", body)
	}
}

func TestWrap(t *testing.T) {
	if got := wrap("leave out head-to-head and sumo-api", 15); got != "leave out\nhead-to-head\nand sumo-api" {
		t.Errorf("%q", got)
	}
}
