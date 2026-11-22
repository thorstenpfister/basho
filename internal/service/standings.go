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
	// Limited: spoiler-free mode cut the standings off at the horizon,
	// before the end of Day.
	Limited bool

	// Opponents names everyone a row's results meet, visitors from a lower
	// division included: the standings popup (SPEC §8.6).
	Opponents map[domain.RikishiID]domain.Name
}

// Standings returns the standings of div after day.
func (s *Service) Standings(ctx context.Context, id domain.BashoID, div domain.Division, day int) (Result[Standings], error) {
	d, err := s.loadDay(ctx, id, div, day)
	if err != nil {
		return Result[Standings]{}, err
	}
	c := limit(ctx, derive.AfterDay(id, day))
	out := Standings{Basho: id, Name: id.Name(), Division: div, Day: day, Standings: derive.StandingsAt(d.entries, d.days, c), Limited: c != derive.AfterDay(id, day)}
	if out.Opponents, err = s.opponents(id, d); err != nil {
		return Result[Standings]{}, err
	}
	w, ok := d.basho.Yusho[div]
	if !ok && day == domain.BashoDays {
		w, ok = playoffWinner(d.bouts)
	}
	if ok && day == domain.BashoDays && finalShown(ctx, id) {
		if e, ok := byID(d.entries)[w]; ok {
			out.Yusho = &Wrestler{RikishiID: e.RikishiID, Shikona: e.Shikona, Rank: e.Rank}
		}
	}
	return Result[Standings]{Value: out, Prov: d.prov}, nil
}

// playoffWinner is the winner of a day's last playoff bout, once every
// listed playoff bout is decided: the yusho before the source names it
// (sumo-api names it some 40 minutes later, osada not at all).
func playoffWinner(bouts []domain.Bout) (domain.RikishiID, bool) {
	var last *domain.Bout
	for i, b := range bouts {
		if !b.IsPlayoff {
			continue
		}
		if b.Winner == 0 {
			return 0, false
		}
		if last == nil || b.MatchNo > last.MatchNo {
			last = &bouts[i]
		}
	}
	if last == nil {
		return 0, false
	}
	w, _, _ := last.Result()
	return w.RikishiID, true
}

// Recap is the summary of one day (SPEC §8.4).
type Recap struct {
	Basho    domain.BashoID
	Name     string
	Division domain.Division
	derive.Recap
	Technique *Technique // the rarest technique's names; nil without one
	Limited   bool       // spoiler-free mode cut the day off at the horizon
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
	c := limit(ctx, derive.AfterDay(id, day))
	out := Recap{Limited: c != derive.AfterDay(id, day), Basho: id, Name: id.Name(), Division: div, Recap: derive.DayRecap(derive.StandingsAt(d.entries, d.days, c), div, d.bouts, c, freq, upset)}
	if out.Recap.Technique != nil {
		t := technique(out.Recap.Technique.Kimarite)
		out.Technique = &t
	}
	return Result[Recap]{Value: out, Prov: d.prov}, nil
}

// opponents names a division's opponents: the banzuke's names, then the
// names in the tournament's bouts, then today's wrestler index.
func (s *Service) opponents(id domain.BashoID, d dayData) (map[domain.RikishiID]domain.Name, error) {
	names := map[domain.RikishiID]domain.Name{}
	for _, e := range d.entries {
		names[e.RikishiID] = e.Shikona
	}
	var missing []domain.RikishiID
	for _, e := range d.entries {
		for _, r := range e.Results {
			if _, ok := names[r.OpponentID]; !ok && r.OpponentID != 0 {
				missing = append(missing, r.OpponentID)
			}
		}
	}
	if len(missing) == 0 {
		return names, nil
	}
	x, err := s.index()
	if err != nil {
		return nil, err
	}
	for _, b := range append(x.Tournament(id), d.bouts...) {
		for _, side := range []domain.BoutSide{b.East, b.West} {
			if _, ok := names[side.RikishiID]; !ok {
				names[side.RikishiID] = side.Shikona
			}
		}
	}
	ws, err := s.wrestlers()
	if err != nil {
		return nil, err
	}
	for _, m := range missing {
		if _, ok := names[m]; !ok {
			if w, ok := ws[m]; ok {
				names[m] = w.Name
			}
		}
	}
	return names, nil
}
