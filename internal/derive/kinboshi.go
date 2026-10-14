package derive

import "github.com/thorstenpfister/basho/internal/domain"

// Kinboshi counts a wrestler's kinboshi before a cutoff (SPEC §6.3).
type Kinboshi struct {
	Count     int // career
	ThisBasho int // of Count, in the cutoff's tournament: drawn in brighter gold
}

// IsKinboshi reports whether b is a kinboshi and returns its winner: a
// Maegashira beating a Yokozuna in a regular Makuuchi bout, by technique (not
// by default, nor a draw the source gives a winner).
func IsKinboshi(b *domain.Bout) (domain.RikishiID, bool) {
	winner, loser, ok := sides(b)
	if !ok || b.Division != domain.DivMakuuchi || b.IsPlayoff || b.Kimarite.Category() == domain.NoTechnique {
		return 0, false
	}
	return winner.RikishiID, winner.Rank.Title == domain.Maegashira && loser.Rank.Title == domain.Yokozuna
}

// KinboshiOf counts id's kinboshi in their chronological career before the
// cutoff.
func KinboshiOf(id domain.RikishiID, career []*domain.Bout, c AsOf) Kinboshi {
	var k Kinboshi
	for _, b := range before(career, c) {
		if w, ok := IsKinboshi(b); ok && w == id {
			k.Count++
			if b.Basho == c.Basho {
				k.ThisBasho++
			}
		}
	}
	return k
}

// Scalps counts id's wins over a Yokozuna or Ozeki in the cutoff's tournament
// before the cutoff (ThisBashoScalps, shown in the day recap): regular bouts
// won by technique.
func Scalps(id domain.RikishiID, career []*domain.Bout, c AsOf) int {
	n := 0
	for _, b := range before(career, c) {
		winner, loser, ok := sides(b)
		if ok && b.Basho == c.Basho && winner.RikishiID == id && !b.IsPlayoff && b.Kimarite.Category() != domain.NoTechnique &&
			(loser.Rank.Title == domain.Yokozuna || loser.Rank.Title == domain.Ozeki) {
			n++
		}
	}
	return n
}

// sides returns a decided bout's winner and loser.
func sides(b *domain.Bout) (winner, loser domain.BoutSide, ok bool) {
	switch b.Winner {
	case domain.East:
		return b.East, b.West, true
	case domain.West:
		return b.West, b.East, true
	}
	return domain.BoutSide{}, domain.BoutSide{}, false
}
