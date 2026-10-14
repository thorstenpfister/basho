package derive

import (
	"slices"

	"github.com/thorstenpfister/basho/internal/domain"
)

// fusen is the kimarite of a bout won by default.
const fusen domain.Kimarite = "fusen"

// Result is one of a wrestler's bouts, from their side.
type Result struct {
	Basho    domain.BashoID // the UI draws a divider where it changes
	Day      int
	Opponent domain.RikishiID
	Won      bool
	Fusen    bool // shown as □/■
	Kimarite domain.Kimarite
}

// LastN is a wrestler's latest results before a cutoff (SPEC §6.2).
type LastN struct {
	Results []Result // oldest first
	New     bool     // fewer bouts than asked for: padded with · and labelled "new"
}

// LastResults returns id's last n decided bouts before the cutoff from their
// chronological career (sekitori bouts only, §5.8), across tournaments.
// Absences have no bout and so are skipped.
func LastResults(id domain.RikishiID, career []*domain.Bout, c AsOf, n int) LastN {
	career = before(career, c)
	var out []Result
	for i := len(career) - 1; i >= 0 && len(out) < n; i-- {
		if r, ok := resultOf(id, career[i]); ok {
			out = append(out, r)
		}
	}
	slices.Reverse(out)
	return LastN{Results: out, New: len(out) < n}
}

// resultOf returns b from id's side, if it is decided.
func resultOf(id domain.RikishiID, b *domain.Bout) (Result, bool) {
	if b.Winner == 0 {
		return Result{}, false
	}
	east := b.East.RikishiID == id
	them := b.East.RikishiID
	if east {
		them = b.West.RikishiID
	}
	return Result{
		Basho: b.Basho, Day: b.Day, Opponent: them, Won: (b.Winner == domain.East) == east,
		Fusen: b.Kimarite == fusen, Kimarite: b.Kimarite,
	}, true
}
