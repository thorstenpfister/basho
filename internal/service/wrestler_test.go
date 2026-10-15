package service

import (
	"testing"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// Phase 15 spot checks against the snapshot.
func TestWrestlerCareers(t *testing.T) {
	s := embedded()
	makuuchi := func(c Career) (r derive.Record, yusho int) {
		for _, b := range c.Tournaments {
			if b.Rank.Title.Division() != domain.DivMakuuchi {
				continue
			}
			r.Wins, r.Losses, r.Absences = r.Wins+b.Record.Wins, r.Losses+b.Record.Losses, r.Absences+b.Record.Absences
			if b.Yusho {
				yusho++
			}
		}
		return r, yusho
	}

	// Hakuho: his official Makuuchi record and 45 yusho, all in the source.
	res, err := s.Wrestler(t.Context(), 3081, aki2026)
	if err != nil {
		t.Fatal(err)
	}
	h := res.Value
	if r, y := makuuchi(h); r.String() != "1093-199-253" || y != 45 || h.Stats.Yusho[domain.DivMakuuchi] != 45 {
		t.Errorf("Hakuho: %s, %d yusho (stats %v)", r, y, h.Stats.Yusho)
	}
	if first, last := h.Tournaments[0], h.Tournaments[len(h.Tournaments)-1]; first.Basho.String() != "200401" || last.Basho.String() != "202109" || last.Rank.Short() != "Y1e" {
		t.Errorf("Hakuho runs %s %s to %s %s", first.Basho, first.Rank.Short(), last.Basho, last.Rank.Short())
	}
	if h.Shikona.En != "Hakuho" || !h.HasProfile || !h.RatingShown || h.Tournaments[len(h.Tournaments)-1].Rating < 1500 {
		t.Errorf("Hakuho: %+v", h.Wrestler)
	}

	// Taiho: 32 yusho. The source lacks two days of 1965 (SPEC §4.3), so his
	// record is a win and a loss short of the official 746-144.
	res, err = s.Wrestler(t.Context(), 1511, aki2026)
	if err != nil {
		t.Fatal(err)
	}
	if r, y := makuuchi(res.Value); r.Wins != 745 || r.Losses != 143 || y != 32 {
		t.Errorf("Taiho: %s, %d yusho", r, y)
	}

	// Onosato, seen from Aki 2026: its yusho, and nothing after it.
	res, err = s.Wrestler(t.Context(), 8850, aki2026)
	if err != nil {
		t.Fatal(err)
	}
	o := res.Value
	last := o.Tournaments[len(o.Tournaments)-1]
	if last.Basho != aki2026 || !last.Yusho || last.Partial || last.Record.String() != "12-3" || o.Rank.Short() != "Y1e" {
		t.Errorf("Onosato's Aki 2026: %+v", last)
	}
	// Seen from an earlier tournament: it ends there.
	res, err = s.Wrestler(t.Context(), 8850, domain.BashoID{Year: 2025, Month: 1})
	if err != nil {
		t.Fatal(err)
	}
	if l := res.Value.Tournaments[len(res.Value.Tournaments)-1]; l.Basho.String() != "202501" {
		t.Errorf("seen from 202501, the career ends at %s", l.Basho)
	}
	if _, err := s.Wrestler(t.Context(), 999999, aki2026); !NotFound(err) {
		t.Errorf("an unknown wrestler: %v", err)
	}
}

// In spoiler-free mode the career stops at the horizon: the tournament is
// cut there, its yusho unknown, and the rating frozen.
func TestWrestlerHorizon(t *testing.T) {
	s := embedded()
	ctx := Hiding(t.Context(), midDay13)
	res, err := s.Wrestler(ctx, 8850, aki2026)
	if err != nil {
		t.Fatal(err)
	}
	full, err := s.Wrestler(t.Context(), 8850, aki2026)
	if err != nil {
		t.Fatal(err)
	}
	hidden, all := res.Value.Tournaments[len(res.Value.Tournaments)-1], full.Value.Tournaments[len(full.Value.Tournaments)-1]
	if !hidden.Partial || hidden.Yusho || hidden.Record.Wins+hidden.Record.Losses+hidden.Record.Absences != 12 {
		t.Errorf("at the horizon: %+v", hidden)
	}
	if hidden.Rating == all.Rating || res.Value.Stats.Yusho[domain.DivMakuuchi] != full.Value.Stats.Yusho[domain.DivMakuuchi]-1 {
		t.Errorf("rating %v (full %v), yusho %v (full %v)", hidden.Rating, all.Rating, res.Value.Stats.Yusho, full.Value.Stats.Yusho)
	}
}

func TestFindWrestlers(t *testing.T) {
	s := embedded()
	for name, want := range map[string]int{"Hakuho": 1, "hakuho": 1, "大の里": 1, "3081": 1, "Takanohana": 2, "Nobody": 0, "99999999": 0} {
		got, err := s.FindWrestlers(name)
		if len(got) != want || (want == 0) != NotFound(err) {
			t.Errorf("%q: %d matches, want %d (%v)", name, len(got), want, err)
		}
	}
	got, _ := s.FindWrestlers("Takanohana")
	if len(got) == 2 && (got[0].First.Compare(got[1].First) >= 0 || got[1].Shikona.En != "Takanohana") {
		t.Errorf("Takanohana: %+v", got)
	}
}
