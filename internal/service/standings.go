package service

import (
	"context"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
)

// Standings is a division's table after a day (SPEC §6.5, `basho standings`).
type Standings struct {
	Basho    domain.BashoID
	Name     string
	Division domain.Division
	Day      int
	derive.Standings
	Yusho *Wrestler // the tournament's winner once it is final, also after a playoff
}

// Standings returns the standings of div after day.
func (s *Service) Standings(ctx context.Context, id domain.BashoID, div domain.Division, day int) (Result[Standings], error) {
	d, err := s.loadDay(ctx, id, div, day)
	if err != nil {
		return Result[Standings]{}, err
	}
	c := derive.AfterDay(id, day)
	out := Standings{Basho: id, Name: id.Name(), Division: div, Day: day, Standings: derive.StandingsAt(d.entries, d.days, c)}
	if w, ok := d.basho.Yusho[div]; ok && day == domain.BashoDays {
		if e, ok := byID(d.entries)[w]; ok {
			out.Yusho = &Wrestler{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank}
		}
	}
	return Result[Standings]{Value: out, Prov: d.prov}, nil
}

// Recap is the summary of one day (SPEC §8.4).
type Recap struct {
	Basho    domain.BashoID
	Name     string
	Division domain.Division
	derive.Recap
	Technique *Technique // the rarest technique's names; nil without one
}

// Recap returns the day recap of div for day. Upsets come from the win
// chance when the rating is shown, otherwise from the rank gap.
func (s *Service) Recap(ctx context.Context, id domain.BashoID, div domain.Division, day int) (Result[Recap], error) {
	d, err := s.loadDay(ctx, id, div, day)
	if err != nil {
		return Result[Recap]{}, err
	}
	r, err := s.ratings()
	if err != nil {
		return Result[Recap]{}, err
	}
	x, err := s.index()
	if err != nil {
		return Result[Recap]{}, err
	}
	freq := derive.KimariteFreq{}
	for _, t := range x.Tournaments() {
		if t.Compare(id) < 0 { // spoiler-safe: earlier tournaments only
			freq.Add(x.Tournament(t))
		}
	}
	var upset func(*domain.Bout) bool
	if r.Shown() {
		upset = r.IsUpset
	}
	c := derive.AfterDay(id, day)
	out := Recap{Basho: id, Name: id.Name(), Division: div, Recap: derive.DayRecap(derive.StandingsAt(d.entries, d.days, c), div, d.bouts, c, freq, upset)}
	if out.Recap.Technique != nil {
		t := technique(out.Recap.Technique.Kimarite)
		out.Technique = &t
	}
	return Result[Recap]{Value: out, Prov: d.prov}, nil
}
