package derive

import "github.com/thorstenpfister/basho/internal/domain"

// Recap is the "between rounds" summary of a day (SPEC §8.4).
type Recap struct {
	Day                   int
	Leaders               []Standing
	KachiKoshi, MakeKoshi []Standing    // reached today
	Kinboshi              []domain.Bout // earned today
	Upsets                []domain.Bout // see IsUpset
	Technique             *domain.Bout  // the day's rarest technique; nil without one
	TechniqueCount        int           // how often it occurs in the frequency table
}

// upsetGap is how many rank levels below the loser an upset's winner is at
// least; see rankLevel. Phase 5 replaces it with the win chance.
const upsetGap = 6

// DayRecap summarises day s.Today of division div at the cutoff s was
// computed for: bouts are the tournament's, freq counts each technique over
// the bout index.
func DayRecap(s Standings, div domain.Division, bouts []domain.Bout, c AsOf, freq KimariteFreq) Recap {
	r := Recap{Day: s.Today}
	for _, row := range s.Rows {
		if row.Band == Leaders && row.Record.Wins > 0 {
			r.Leaders = append(r.Leaders, row)
		}
		if row.NewToday && row.Record.KachiKoshi() {
			r.KachiKoshi = append(r.KachiKoshi, row)
		}
		if row.NewToday && row.Record.MakeKoshi() {
			r.MakeKoshi = append(r.MakeKoshi, row)
		}
	}
	for i := range bouts {
		b := &bouts[i]
		if b.Division != div || b.Day != s.Today || b.Winner == 0 || !c.Includes(*b) {
			continue
		}
		if _, ok := IsKinboshi(b); ok {
			r.Kinboshi = append(r.Kinboshi, *b)
		}
		if IsUpset(b) {
			r.Upsets = append(r.Upsets, *b)
		}
		if b.Kimarite.Category() == domain.NoTechnique {
			continue
		}
		if n := freq[b.Kimarite]; r.Technique == nil || n < r.TechniqueCount {
			r.Technique, r.TechniqueCount = b, n
		}
	}
	return r
}

// IsUpset reports a win by technique over a wrestler at least upsetGap rank
// levels higher.
func IsUpset(b *domain.Bout) bool {
	winner, loser, ok := sides(b)
	return ok && b.Kimarite.Category() != domain.NoTechnique && rankLevel(winner.Rank)-rankLevel(loser.Rank) >= upsetGap
}

// rankLevel places a rank on one scale, highest 0: each san'yaku title is
// three levels, each Maegashira or Juryo number one.
func rankLevel(r domain.Rank) int {
	switch r.Title {
	case domain.Yokozuna, domain.Ozeki, domain.Sekiwake, domain.Komusubi:
		return 3 * int(r.Title-domain.Yokozuna)
	case domain.Maegashira:
		return 9 + r.Number
	case domain.Juryo:
		return 9 + 17 + r.Number
	default:
		return 9 + 17 + 14 + r.Number
	}
}

// KimariteFreq counts how often each technique decided a bout.
type KimariteFreq map[domain.Kimarite]int

// Add counts the decided bouts.
func (f KimariteFreq) Add(bouts []domain.Bout) {
	for _, b := range bouts {
		if b.Winner != 0 {
			f[b.Kimarite]++
		}
	}
}
