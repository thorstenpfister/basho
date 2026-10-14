package derive

import (
	"cmp"
	"maps"
	"slices"

	"github.com/thorstenpfister/basho/internal/domain"
)

// HeadToHead is two wrestlers' meetings before a cutoff (SPEC §6.4).
type HeadToHead struct {
	A, B     H2HSide
	Meetings []Meeting // the latest, newest first
}

// H2HSide is one wrestler's part of a head-to-head.
type H2HSide struct {
	Wins int             // fusen included, as in the official record
	Top  []KimariteCount // most frequent winning techniques, fusen and draws left out
}

// Meeting is one bout between the two.
type Meeting struct {
	Basho        domain.BashoID
	Day          int
	Winner       domain.RikishiID
	Kimarite     domain.Kimarite
	RankA, RankB domain.Rank
	Playoff      bool
}

// KimariteCount is a technique and how often it occurred.
type KimariteCount struct {
	Kimarite domain.Kimarite
	Count    int
}

// HeadToHeadOf compares a with b from a's chronological career before the
// cutoff: wins, each side's top 2 techniques and the last n meetings.
func HeadToHeadOf(a, b domain.RikishiID, careerA []*domain.Bout, c AsOf, n int) HeadToHead {
	var h HeadToHead
	techniques := [2]map[domain.Kimarite]int{{}, {}}
	career := before(careerA, c)
	for i := len(career) - 1; i >= 0; i-- {
		bt := career[i]
		winner, _, ok := bt.Result()
		if !ok || bt.East.RikishiID != b && bt.West.RikishiID != b {
			continue
		}
		side, s := &h.A, 0
		if winner.RikishiID == b {
			side, s = &h.B, 1
		}
		side.Wins++
		if bt.Kimarite.Category() != domain.NoTechnique {
			techniques[s][bt.Kimarite]++
		}
		if len(h.Meetings) < n {
			m := Meeting{Basho: bt.Basho, Day: bt.Day, Winner: winner.RikishiID, Kimarite: bt.Kimarite, RankA: bt.East.Rank, RankB: bt.West.Rank, Playoff: bt.IsPlayoff}
			if bt.East.RikishiID == b {
				m.RankA, m.RankB = m.RankB, m.RankA
			}
			h.Meetings = append(h.Meetings, m)
		}
	}
	h.A.Top, h.B.Top = top(techniques[0], 2), top(techniques[1], 2)
	return h
}

// top returns the n most frequent techniques, ties by name.
func top(counts map[domain.Kimarite]int, n int) []KimariteCount {
	out := make([]KimariteCount, 0, len(counts))
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		out = append(out, KimariteCount{k, counts[k]})
	}
	slices.SortStableFunc(out, func(x, y KimariteCount) int { return cmp.Compare(y.Count, x.Count) })
	return out[:min(n, len(out))]
}
