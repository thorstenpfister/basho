package service

import (
	"context"
	"fmt"

	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

// Tale is the tale of the tape for one bout (SPEC §8.5, `basho bout`).
type Tale struct {
	Basho      domain.BashoID
	Name       string
	Division   domain.Division
	Day        int
	MatchNo    int
	Of         int  // regular bouts that day
	Musubi     bool // the last bout of the day
	Playoff    bool
	East, West TaleSide
	HeadToHead derive.HeadToHead  // A is east; up to MaxMeetings, newest first
	Matchups   [3]*derive.Matchup // by derive.Window; nil without samples
	WinChance  *float64           // the east side's chance; nil when the rating is hidden
	Scale      Scale              // for the height and weight bars
	Result     *TaleResult        // nil while undecided, or Hidden
	Hidden     bool               // after the spoiler horizon, decided or not
}

// TaleSide is one wrestler's column.
type TaleSide struct {
	Wrestler
	Heya, Shusshin string
	Career         derive.CareerStats
	Profile        bool          // false: no profile (height, weight, heya and age are empty)
	Record         derive.Record // before the bout
	Visitor        bool          // from a lower division: no record here
	Last5          derive.LastN
	Kinboshi       derive.Kinboshi
	Movesets       [3]derive.Profile // by derive.Window: last 6 basho, career, this basho
	Rating         *Rating           // nil when the rating is hidden
}

// TaleResult is the decided bout.
type TaleResult struct {
	Winner      domain.Side
	Technique   Technique
	Kinboshi    bool
	Fusen       bool
	East, West  derive.Record // after the bout
	WinnerUpset bool          // the winner had under 35%
}

// Scale is the range of heights and weights in the bout's division, so the
// bars show differences (SPEC §8.5).
type Scale struct {
	MinHeight, MaxHeight, MinWeight, MaxWeight float64
}

// MaxMeetings is how many head-to-head meetings a tale carries: 3 on the
// VS screen, up to 10 expanded (SPEC §8.5).
const MaxMeetings = 10

// Moveset returns a side's profile in the last-6-basho window.
func (t TaleSide) Moveset() derive.Profile { return t.Movesets[derive.LastSix] }

// Matchup returns the matchup in the last-6-basho window.
func (t Tale) Matchup() *derive.Matchup { return t.Matchups[derive.LastSix] }

// Bout returns the tale of the tape of bout matchNo, as of just before it:
// nothing on it reveals the bout's own result except Result. In spoiler-free
// mode it is as of the horizon when that is earlier, and a result after the
// horizon is Hidden.
func (s *Service) Bout(ctx context.Context, id domain.BashoID, div domain.Division, day, matchNo int) (Result[Tale], error) {
	d, err := s.loadDay(ctx, id, div, day)
	if err != nil {
		return Result[Tale]{}, err
	}
	var b *domain.Bout
	of, last := 0, 0
	for i := range d.bouts {
		if !d.bouts[i].IsPlayoff {
			of++
			last = max(last, d.bouts[i].MatchNo)
		}
		if d.bouts[i].MatchNo == matchNo && b == nil {
			b = &d.bouts[i]
		}
	}
	if b == nil {
		return Result[Tale]{}, fmt.Errorf("%s %s day %d: no bout %d: %w", id, div, day, matchNo, source.ErrNotFound)
	}
	x, err := s.index()
	if err != nil {
		return Result[Tale]{}, err
	}
	r, err := s.ratings()
	if err != nil {
		return Result[Tale]{}, err
	}
	profiles, err := s.wrestlers()
	if err != nil {
		return Result[Tale]{}, err
	}
	tournaments := bashoList(x)
	entries := byID(d.entries)
	c := limit(ctx, derive.BeforeBout(*b))
	current := s.CurrentBasho()

	side := func(bs domain.BoutSide) TaleSide {
		p, ok := profiles[bs.RikishiID]
		career := x.Rikishi(bs.RikishiID)
		t := TaleSide{
			Wrestler: Wrestler{RikishiID: bs.RikishiID, Shikona: bs.Shikona, Rank: bs.Rank},
			Heya:     p.Heya, Shusshin: p.Shusshin, Profile: ok,
			Career:   derive.CareerAt(p, bs.Shikona, d.basho, tournaments, c, current),
			Last5:    derive.LastResults(bs.RikishiID, career, c, 5),
			Kinboshi: derive.KinboshiOf(bs.RikishiID, career, c),
			Movesets: [3]derive.Profile{
				derive.ProfileOf(bs.RikishiID, career, c, derive.LastSix),
				derive.ProfileOf(bs.RikishiID, career, c, derive.Career),
				derive.ProfileOf(bs.RikishiID, career, c, derive.ThisBasho),
			},
			Visitor: true,
		}
		if e, ok := entries[bs.RikishiID]; ok {
			t.Visitor = false
			t.Record = derive.RecordAt(e, d.days, c)
			t.Shikona = e.Shikona // bout sides may carry romaji only (SPEC Appendix D)
		}
		if r.Shown() {
			rt := r.At(bs.RikishiID, bs.Rank, c)
			t.Rating = &rt
		}
		return t
	}
	tale := Tale{
		Basho: id, Name: id.Name(), Division: div, Day: day, MatchNo: matchNo, Of: of,
		Musubi: !b.IsPlayoff && matchNo == last && div == domain.DivMakuuchi, Playoff: b.IsPlayoff,
		East: side(b.East), West: side(b.West),
		HeadToHead: derive.HeadToHeadOf(b.East.RikishiID, b.West.RikishiID, x.Rikishi(b.East.RikishiID), c, MaxMeetings),
		Scale:      scale(d.entries, profiles),
	}
	for w := range tale.Matchups {
		if m, ok := derive.MatchupOf(tale.East.Movesets[w], tale.West.Movesets[w]); ok {
			tale.Matchups[w] = &m
		}
	}
	if r.Shown() {
		p := r.WinChanceAt(*b, c)
		tale.WinChance = &p
	}
	tale.Hidden = !visible(ctx, *b)
	if b.Winner != 0 && !tale.Hidden {
		res := &TaleResult{Winner: b.Winner, Technique: technique(b.Kimarite), Fusen: b.Kimarite == "fusen"}
		_, res.Kinboshi = derive.IsKinboshi(b)
		if e, ok := entries[b.East.RikishiID]; ok {
			res.East = derive.RecordAt(e, d.days, derive.ThroughBout(*b))
		}
		if e, ok := entries[b.West.RikishiID]; ok {
			res.West = derive.RecordAt(e, d.days, derive.ThroughBout(*b))
		}
		res.WinnerUpset = r.Shown() && r.IsUpset(b)
		tale.Result = res
	}
	return Result[Tale]{Value: tale, Prov: d.prov}, nil
}

// bashoList returns the metadata of every tournament in the bout index, for
// yusho and sansho counts.
func bashoList(x *snapshot.Index) []domain.Basho {
	ids := x.Tournaments()
	out := make([]domain.Basho, 0, len(ids))
	for _, id := range ids {
		if b, ok := x.Basho(id); ok {
			out = append(out, b)
		}
	}
	return out
}

func scale(entries []domain.BanzukeEntry, profiles map[domain.RikishiID]domain.Rikishi) Scale {
	var sc Scale
	for _, e := range entries {
		p, ok := profiles[e.RikishiID]
		if !ok || p.Height == 0 || p.Weight == 0 {
			continue
		}
		if sc.MaxHeight == 0 {
			sc = Scale{p.Height, p.Height, p.Weight, p.Weight}
			continue
		}
		sc.MinHeight, sc.MaxHeight = min(sc.MinHeight, p.Height), max(sc.MaxHeight, p.Height)
		sc.MinWeight, sc.MaxWeight = min(sc.MinWeight, p.Weight), max(sc.MaxWeight, p.Weight)
	}
	return sc
}
