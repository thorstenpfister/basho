package resolver

import (
	"context"
	"fmt"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/idmap"
	"github.com/thorstenpfister/basho/internal/source"
)

// mapper turns osada.us's tournament into domain values with sumo-api IDs
// (SPEC §5.5).
type mapper struct {
	cur source.Current
	ids map[int]idmap.Identity // by JSA ID
	// bouts finds each wrestler's regular bout of a day, in any division:
	// Juryo wrestlers fight up in Makuuchi.
	bouts map[int]map[int]source.CurrentBout // day → JSA ID → bout
}

func (r *Resolver) mapper(ctx context.Context, cur source.Current) (*mapper, error) {
	if r.IDs == nil {
		return nil, fmt.Errorf("osada.us: no JSA ID mapping: %w", source.ErrNotFound)
	}
	ids, err := r.IDs()
	if err != nil {
		return nil, err
	}
	m := &mapper{cur: cur, ids: map[int]idmap.Identity{}, bouts: map[int]map[int]source.CurrentBout{}}
	jp := map[int]string{}
	for _, es := range cur.Banzuke {
		for _, e := range es {
			jp[e.JSAID] = e.Name.Jp
			m.ids[e.JSAID] = ids.Resolve(ctx, e.JSAID, e.Name.Jp, e.Name.Kana)
		}
	}
	for _, d := range cur.Days {
		byJSA := map[int]source.CurrentBout{}
		for _, bouts := range d.Bouts {
			for _, b := range bouts {
				for _, s := range []source.CurrentSide{b.East, b.West} {
					if _, ok := m.ids[s.JSAID]; !ok { // a visitor
						m.ids[s.JSAID] = ids.Resolve(ctx, s.JSAID, jp[s.JSAID], s.Kana)
					}
					if !b.IsPlayoff {
						byJSA[s.JSAID] = b
					}
				}
			}
		}
		m.bouts[d.Day] = byJSA
	}
	return m, nil
}

// today is the tournament's day number on now; 0 outside the 15 days.
func (m *mapper) today(now time.Time) int {
	if m.cur.Start.IsZero() {
		return 0
	}
	if d := domain.DateOf(now).DaysSince(m.cur.Start) + 1; d >= 1 && d <= domain.BashoDays {
		return d
	}
	return 0
}

// banzuke returns a division's entries, with each day's result from the
// bouts (fusen included) or, without a bout, osada's tally (absences).
func (m *mapper) banzuke(div domain.Division) []domain.BanzukeEntry {
	var out []domain.BanzukeEntry
	for _, e := range m.cur.Banzuke[div] {
		id := m.ids[e.JSAID]
		entry := domain.BanzukeEntry{RikishiID: id.ID, Shikona: id.Name, Rank: e.Rank}
		for i := range domain.BashoDays {
			entry.Results[i] = m.result(e, i+1)
		}
		out = append(out, entry)
	}
	return out
}

func (m *mapper) result(e source.CurrentEntry, day int) domain.DayResult {
	b, ok := m.bouts[day][e.JSAID]
	if !ok {
		o := e.Results[day-1]
		if o != domain.Absent && o != domain.Pending {
			o = domain.Unknown // a tally without a bout
		}
		return domain.DayResult{Outcome: o}
	}
	them, side := b.West, domain.East
	if b.West.JSAID == e.JSAID {
		them, side = b.East, domain.West
	}
	res := domain.DayResult{OpponentID: m.ids[them.JSAID].ID, Kimarite: b.Kimarite}
	fusen := b.Kimarite == "fusen"
	switch {
	case b.Winner == 0:
		res.Outcome = domain.Pending
	case b.Winner == side && fusen:
		res.Outcome = domain.FusenWin
	case b.Winner == side:
		res.Outcome = domain.Win
	case fusen:
		res.Outcome = domain.FusenLoss
	default:
		res.Outcome = domain.Loss
	}
	return res
}

// torikumi returns a division's bouts of a day, playoffs included.
func (m *mapper) torikumi(div domain.Division, day int) ([]domain.Bout, error) {
	for _, d := range m.cur.Days {
		if d.Day != day {
			continue
		}
		out := make([]domain.Bout, 0, len(d.Bouts[div]))
		for _, b := range d.Bouts[div] {
			out = append(out, domain.Bout{
				Basho: m.cur.Basho, Division: div, Day: day, MatchNo: b.MatchNo,
				East: m.side(b.East), West: m.side(b.West),
				Winner: b.Winner, Kimarite: b.Kimarite, IsPlayoff: b.IsPlayoff,
			})
		}
		if len(out) == 0 {
			break
		}
		return out, nil
	}
	return nil, fmt.Errorf("osada.us %s %s day %d: %w", m.cur.Basho, div, day, source.ErrNotYetAvailable)
}

func (m *mapper) side(s source.CurrentSide) domain.BoutSide {
	id := m.ids[s.JSAID]
	return domain.BoutSide{RikishiID: id.ID, Shikona: id.Name, Rank: s.Rank}
}
