package derive

import "github.com/thorstenpfister/basho/internal/domain"

// CareerStats is a wrestler's tale-of-the-tape facts as of a tournament
// (SPEC §6.6).
type CareerStats struct {
	Age            int // at the tournament's start; 0 when the birth date is unknown
	Height, Weight float64
	Latest         bool                    // height and weight are the latest known, not the tournament's own
	Yusho          map[domain.Division]int // Makuuchi and Juryo
	Sansho         map[string]int          // by prize, e.g. "Kanto-sho"
	Now            string                  // today's shikona, when it differs from the tournament's
}

// CareerAt returns r's statistics as of tournament b, with shikona the name
// they fought under there. Yusho and sansho count from tournaments that ended
// before the cutoff, so the current one only counts once it is over. current
// is the tournament in progress or just ended: only for it are height and
// weight not labelled "latest".
func CareerAt(r domain.Rikishi, shikona domain.Name, b domain.Basho, tournaments []domain.Basho, c AsOf, current domain.BashoID) CareerStats {
	st := CareerStats{
		Age: age(r.Birth, b.Start), Height: r.Height, Weight: r.Weight, Latest: b.ID != current,
		Yusho: map[domain.Division]int{}, Sansho: map[string]int{},
	}
	if r.Name.En != "" && r.Name.En != shikona.En {
		st.Now = r.Name.En
	}
	for _, t := range tournaments {
		if !c.over(t.ID) {
			continue
		}
		for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
			if t.Yusho[div] == r.ID {
				st.Yusho[div]++
			}
		}
		for _, p := range t.Prizes {
			if p.RikishiID == r.ID {
				st.Sansho[p.Type]++
			}
		}
	}
	return st
}

// over reports whether all of tournament basho, playoffs included, lies
// before the cutoff.
func (c AsOf) over(basho domain.BashoID) bool {
	n := basho.Compare(c.Basho)
	return n < 0 || n == 0 && c.Day >= domain.BashoDays && c.MatchNo == 0 && c.Inclusive
}

// age returns whole years from birth to on; 0 when birth is unknown.
func age(birth, on domain.Date) int {
	if birth.IsZero() || on.IsZero() {
		return 0
	}
	years := on.Year - birth.Year
	if on.Month < birth.Month || on.Month == birth.Month && on.Day < birth.Day {
		years--
	}
	return years
}
