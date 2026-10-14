package resolver

import (
	"fmt"

	"github.com/thorstenpfister/basho/internal/domain"
)

// overlay lays osada's results over sumo-api's banzuke (SPEC §5.4 rule 2):
// sumo-api has the fusen and absence detail, osada the newer results. A day
// sumo-api has pending takes osada's result; when both have one and they
// disagree, osada wins for today and sumo-api for earlier days. It reports
// whether any of osada's results were used.
func (r *Resolver) overlay(sumo, osada []domain.BanzukeEntry, today int) ([]domain.BanzukeEntry, bool) {
	theirs := make(map[domain.RikishiID]domain.BanzukeEntry, len(osada))
	for _, e := range osada {
		theirs[e.RikishiID] = e
	}
	used := false
	out := make([]domain.BanzukeEntry, len(sumo))
	for i, e := range sumo {
		o, ok := theirs[e.RikishiID]
		for d := range domain.BashoDays {
			mine, other := e.Results[d], o.Results[d]
			switch {
			case !ok || !decided(other.Outcome) || mine == other:
			case !decided(mine.Outcome):
				e.Results[d], used = other, true
			default:
				r.disagree("banzuke", e.RikishiID, d+1, mine, other, d+1 == today)
				if d+1 == today {
					e.Results[d], used = other, true
				}
			}
		}
		out[i] = e
	}
	return out, used
}

func decided(o domain.Outcome) bool { return o != domain.Pending && o != domain.Unknown }

// mergeBouts starts from osada's bout list, which has the playoffs, and
// takes sumo-api's result for a bout osada has undecided, or on an earlier
// day when they disagree (rule 2). Each bout's source is returned by match
// number; bouts only sumo-api has are added.
func (r *Resolver) mergeBouts(osada, sumo []domain.Bout, today bool) ([]domain.Bout, map[int]string) {
	type pair struct{ a, b domain.RikishiID }
	key := func(b domain.Bout) pair {
		if b.East.RikishiID < b.West.RikishiID {
			return pair{b.East.RikishiID, b.West.RikishiID}
		}
		return pair{b.West.RikishiID, b.East.RikishiID}
	}
	theirs := map[pair]domain.Bout{}
	for _, b := range sumo {
		theirs[key(b)] = b
	}
	sources := map[int]string{}
	out := make([]domain.Bout, 0, len(osada))
	seen := map[pair]bool{}
	for _, b := range osada {
		sources[b.MatchNo] = Osada
		if b.IsPlayoff {
			out = append(out, b)
			continue
		}
		k := key(b)
		seen[k] = true
		s, ok := theirs[k]
		winner := winnerID(s)
		switch {
		case !ok || s.Winner == 0 || winnerID(b) == winner && b.Kimarite == s.Kimarite:
		case b.Winner == 0:
			b, sources[b.MatchNo] = withResult(b, winner, s.Kimarite), SumoAPI
		case !today:
			r.disagreeBout(b, s)
			b, sources[b.MatchNo] = withResult(b, winner, s.Kimarite), SumoAPI
		default:
			r.disagreeBout(b, s)
		}
		out = append(out, b)
	}
	for _, s := range sumo {
		if !seen[key(s)] {
			out = append(out, s)
			sources[s.MatchNo] = SumoAPI
		}
	}
	return out, sources
}

func winnerID(b domain.Bout) domain.RikishiID {
	w, _, _ := b.Result()
	return w.RikishiID
}

// withResult gives b the winner by ID, whichever side they are on in b.
func withResult(b domain.Bout, winner domain.RikishiID, k domain.Kimarite) domain.Bout {
	b.Winner, b.Kimarite = domain.East, k
	if b.West.RikishiID == winner {
		b.Winner = domain.West
	}
	return b
}

func (r *Resolver) disagree(what string, id domain.RikishiID, day int, sumo, osada domain.DayResult, today bool) {
	if r.Log != nil {
		r.Log.Debug("sources disagree", "data", what, "rikishi", id, "day", day, "sumo-api", sumo, "osada", osada, "osada wins", today)
	}
}

func (r *Resolver) disagreeBout(osada, sumo domain.Bout) {
	if r.Log != nil {
		r.Log.Debug("sources disagree", "data", "torikumi", "basho", osada.Basho, "day", osada.Day, "match", osada.MatchNo,
			"osada", fmt.Sprint(winnerID(osada), " ", osada.Kimarite), "sumo-api", fmt.Sprint(winnerID(sumo), " ", sumo.Kimarite))
	}
}
