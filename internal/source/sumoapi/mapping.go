package sumoapi

import (
	"fmt"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/source"
)

// divisions maps sumo-api's division names (as in yusho types and paths) to
// domain divisions.
var divisions = func() map[string]domain.Division {
	m := map[string]domain.Division{}
	for d := domain.DivMakuuchi; d <= domain.DivMakushita; d++ {
		m[d.String()] = d
	}
	return m
}()

func mapBasho(id domain.BashoID, dto bashoDTO) domain.Basho {
	b := domain.Basho{
		ID:       id,
		Start:    domain.DateOf(dto.StartDate),
		End:      domain.DateOf(dto.EndDate),
		Location: dto.Location,
		Yusho:    map[domain.Division]domain.RikishiID{},
	}
	for _, y := range dto.Yusho {
		if div, ok := divisions[y.Type]; ok {
			b.Yusho[div] = domain.RikishiID(y.RikishiID)
		}
	}
	for _, p := range dto.SpecialPrizes {
		b.Prizes = append(b.Prizes, domain.Prize{Type: p.Type, RikishiID: domain.RikishiID(p.RikishiID)})
	}
	return b
}

// outcomes maps sumo-api's record results; anything else is domain.Unknown.
var outcomes = map[string]domain.Outcome{
	"":           domain.Pending,
	"win":        domain.Win,
	"loss":       domain.Loss,
	"fusen win":  domain.FusenWin,
	"fusen loss": domain.FusenLoss,
	"absent":     domain.Absent,
	"draw":       domain.Draw,
}

func mapOutcome(result string) domain.Outcome {
	if o, ok := outcomes[result]; ok {
		return o
	}
	return domain.Unknown
}

func mapBanzuke(dto banzukeDTO) ([]domain.BanzukeEntry, error) {
	entries := make([]domain.BanzukeEntry, 0, len(dto.East)+len(dto.West))
	for _, e := range append(dto.East, dto.West...) {
		rank, err := domain.ParseRank(e.Rank)
		if err != nil {
			return nil, fmt.Errorf("banzuke entry %d: %w: %w", e.RikishiID, source.ErrBadData, err)
		}
		entry := domain.BanzukeEntry{
			RikishiID: domain.RikishiID(e.RikishiID),
			Shikona:   domain.Name{En: e.ShikonaEn, Jp: cleanShikona(e.ShikonaJp)},
			Rank:      rank,
		}
		for day, r := range e.Record[:min(len(e.Record), domain.BashoDays)] {
			res := domain.DayResult{
				Outcome:    mapOutcome(r.Result),
				OpponentID: domain.RikishiID(r.OpponentID),
				Kimarite:   domain.NormalizeKimarite(r.Kimarite),
			}
			if res.Outcome == domain.Absent {
				res.OpponentID = 0 // sumo-api writes a placeholder id (7402) on absent days
			}
			entry.Results[day] = res
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func mapTorikumi(id domain.BashoID, div domain.Division, dto torikumiDTO) ([]domain.Bout, error) {
	bouts := make([]domain.Bout, 0, len(dto.Torikumi))
	for _, t := range dto.Torikumi {
		east, err := boutSide(t.EastID, t.EastShikona, t.EastRank)
		if err != nil {
			return nil, err
		}
		west, err := boutSide(t.WestID, t.WestShikona, t.WestRank)
		if err != nil {
			return nil, err
		}
		b := domain.Bout{
			Basho: id, Division: div, Day: t.Day, MatchNo: t.MatchNo,
			East: east, West: west,
			Kimarite: domain.NormalizeKimarite(t.Kimarite),
		}
		switch t.WinnerID {
		case 0:
		case t.EastID:
			b.Winner = domain.East
		case t.WestID:
			b.Winner = domain.West
		default:
			return nil, fmt.Errorf("bout %d: winner %d fought neither side: %w", t.MatchNo, t.WinnerID, source.ErrBadData)
		}
		bouts = append(bouts, b)
	}
	return bouts, nil
}

func boutSide(id int, shikona, rank string) (domain.BoutSide, error) {
	r, err := domain.ParseRank(rank)
	if err != nil {
		return domain.BoutSide{}, fmt.Errorf("rikishi %d: %w: %w", id, source.ErrBadData, err)
	}
	return domain.BoutSide{RikishiID: domain.RikishiID(id), Shikona: domain.Name{En: shikona}, Rank: r}, nil
}

func mapRikishi(r rikishiDTO) domain.Rikishi {
	out := domain.Rikishi{
		ID:       domain.RikishiID(r.ID),
		NSKID:    r.NSKID,
		SumoDBID: r.SumoDBID,
		Name:     domain.Name{En: r.ShikonaEn, Jp: cleanShikona(r.ShikonaJp)},
		Heya:     r.Heya,
		Shusshin: r.Shusshin,
		Height:   r.Height,
		Weight:   r.Weight,
		Retired:  !r.Intai.IsZero(),
	}
	if !r.BirthDate.IsZero() {
		out.Birth = domain.DateOf(r.BirthDate)
	}
	if debut, err := domain.ParseBashoID(r.Debut); err == nil {
		out.Debut = debut
	}
	return out
}
