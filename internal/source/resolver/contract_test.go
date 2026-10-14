package resolver

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Both adapters describe Aki 2026 alike (Phase 9 step 6): osada.us mapped
// through the JSA IDs gives sumo-api's banzuke, results and bouts. What
// differs by design: osada adds kana (and kanji in bouts) and the playoffs;
// it gives Makushita visitors no side.
func TestContract(t *testing.T) {
	s := &sources{}
	cur, err := s.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	m, err := newResolver(t, s, day13).mapper(t.Context(), cur)
	if err != nil {
		t.Fatal(err)
	}
	for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		sumo, err := s.sumo().Banzuke(t.Context(), aki2026, div)
		if err != nil {
			t.Fatal(err)
		}
		mapped := m.banzuke(div)
		if len(mapped) != len(sumo) {
			t.Errorf("%s: %d entries, sumo-api %d", div, len(mapped), len(sumo))
		}
		theirs := map[domain.RikishiID]domain.BanzukeEntry{}
		for _, e := range mapped {
			theirs[e.RikishiID] = e
		}
		for _, e := range sumo {
			o, ok := theirs[e.RikishiID]
			switch {
			case !ok:
				t.Errorf("%s: %s (%d) not mapped", div, e.Shikona.En, e.RikishiID)
			case o.Rank != e.Rank || o.Shikona.En != e.Shikona.En || o.Shikona.Jp != e.Shikona.Jp || o.Shikona.Approx:
				t.Errorf("%s: %+v %s; sumo-api %+v %s", div, o.Shikona, o.Rank, e.Shikona, e.Rank)
			case o.Results != e.Results:
				t.Errorf("%s %s: results\n%v\nsumo-api\n%v", div, e.Shikona.En, o.Results, e.Results)
			}
		}

		for _, day := range []int{1, 13, 15} {
			sumo, err := s.sumo().Torikumi(t.Context(), aki2026, div, day)
			if err != nil {
				t.Fatal(err)
			}
			mapped, err := m.torikumi(div, day)
			if err != nil {
				t.Fatal(err)
			}
			var regular []domain.Bout
			for _, b := range mapped {
				if !b.IsPlayoff {
					regular = append(regular, b)
				}
			}
			if len(regular) != len(sumo) {
				t.Errorf("%s day %d: %d bouts, sumo-api %d", div, day, len(regular), len(sumo))
				continue
			}
			for i, b := range sumo {
				if o := comparable(regular[i]); o != comparable(b) {
					t.Errorf("%s day %d:\n%+v\nsumo-api\n%+v", div, day, o, comparable(b))
				}
			}
		}
	}
}

// comparable keeps what both sources have: romaji names, and visitors'
// ranks without the side.
func comparable(b domain.Bout) domain.Bout {
	for _, s := range []*domain.BoutSide{&b.East, &b.West} {
		s.Shikona = domain.Name{En: s.Shikona.En}
		if s.Rank.Title == domain.Makushita {
			s.Rank.Side = 0
		}
	}
	return b
}
