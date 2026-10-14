package derive

import "github.com/thorstenpfister/basho/internal/domain"

// Window is the sample a technique profile is built from (SPEC §6.7).
type Window int

const (
	LastSix   Window = iota // the wrestler's last 6 tournaments, widened to 20 wins
	Career                  // every sekitori bout
	ThisBasho               // the cutoff's tournament only
)

func (w Window) String() string {
	return [...]string{"last 6 basho", "career", "this basho"}[w]
}

// FighterType is a wrestler's style, read from their winning techniques.
type FighterType int

const (
	NoType FighterType = iota // not enough wins to tell
	PusherThruster
	BeltFighter
	Thrower
	CounterPuller
	AllRounder
)

func (t FighterType) String() string {
	return [...]string{"", "Pusher-thruster", "Belt fighter", "Thrower", "Counter-puller", "All-rounder"}[t]
}

// Fighter-type thresholds on shares of wins, tuned against well-known
// wrestlers (SPEC §6.7).
const (
	pusherShare  = 0.50 // PUSH
	beltShare    = 0.55 // BELT + THROW
	throwerShare = 0.35 // THROW alone, among belt fighters
	pullerShare  = 0.25 // PULL

	enoughWins = 10 // below: "not enough data"
	widenWins  = 20 // the last-6 window widens until it holds this many wins
	lastN      = 6
)

// Shares holds a share per category, indexed by domain.Push … domain.Other.
type Shares [domain.Other + 1]float64

// Profile is a wrestler's technique profile before a cutoff. Fusen and draws
// are left out of every count.
type Profile struct {
	Window              Window
	Tournaments         int // in the sample, after widening
	Wins, Losses        int
	WinShare, LossShare Shares
	TopWins, TopLosses  []KimariteCount // 3 each; TopLosses are the techniques they lost to
	Signature           KimariteCount   // the most frequent winning technique
	SignatureShare      float64         // of all wins
	Type                FighterType     // NoType below enoughWins
}

// Enough reports whether the sample has enough wins to show style and
// signature.
func (p Profile) Enough() bool { return p.Wins >= enoughWins }

// ProfileOf builds id's profile from their chronological career before the
// cutoff.
func ProfileOf(id domain.RikishiID, career []*domain.Bout, c AsOf, w Window) Profile {
	p := Profile{Window: w}
	wins, losses := map[domain.Kimarite]int{}, map[domain.Kimarite]int{}
	var winCat, lossCat [domain.Other + 1]int
	career = before(career, c)
	var current domain.BashoID
	for i := len(career) - 1; i >= 0; i-- {
		b := career[i]
		if b.Basho != current {
			if w == ThisBasho && b.Basho != c.Basho || w == LastSix && p.Tournaments >= lastN && p.Wins >= widenWins {
				break
			}
			current = b.Basho
			p.Tournaments++
		}
		r, ok := resultOf(id, b)
		cat := r.Kimarite.Category()
		if !ok || cat == domain.NoTechnique {
			continue
		}
		if r.Won {
			p.Wins++
			wins[r.Kimarite]++
			winCat[cat]++
		} else {
			p.Losses++
			losses[r.Kimarite]++
			lossCat[cat]++
		}
	}
	for cat := domain.Push; cat <= domain.Other; cat++ {
		p.WinShare[cat] = share(winCat[cat], p.Wins)
		p.LossShare[cat] = share(lossCat[cat], p.Losses)
	}
	p.TopWins, p.TopLosses = top(wins, 3), top(losses, 3)
	if len(p.TopWins) > 0 {
		p.Signature = p.TopWins[0]
		p.SignatureShare = share(p.Signature.Count, p.Wins)
	}
	if p.Enough() {
		p.Type = fighterType(p.WinShare)
	}
	return p
}

// fighterType applies the first rule that matches (SPEC §6.7).
func fighterType(s Shares) FighterType {
	switch {
	case s[domain.Push] >= pusherShare:
		return PusherThruster
	case s[domain.Belt]+s[domain.Throw] >= beltShare && s[domain.Throw] >= throwerShare:
		return Thrower
	case s[domain.Belt]+s[domain.Throw] >= beltShare:
		return BeltFighter
	case s[domain.Pull] >= pullerShare:
		return CounterPuller
	default:
		return AllRounder
	}
}

// Matchup is the most striking style clash between two wrestlers: the
// attacker's main winning category against the defender's losses in it.
type Matchup struct {
	AttackerIsA bool
	Category    domain.KimariteCategory
	WinShare    float64 // of the attacker's wins
	LossShare   float64 // of the defender's losses
}

// MatchupOf compares both directions and keeps the larger product; false
// when neither has a sample.
func MatchupOf(a, b Profile) (Matchup, bool) {
	ab, okAB := direction(a, b)
	ba, okBA := direction(b, a)
	ab.AttackerIsA = true
	switch {
	case !okAB && !okBA:
		return Matchup{}, false
	case !okBA || okAB && ab.WinShare*ab.LossShare >= ba.WinShare*ba.LossShare:
		return ab, true
	default:
		return ba, true
	}
}

func direction(attacker, defender Profile) (Matchup, bool) {
	if attacker.Wins == 0 || defender.Losses == 0 {
		return Matchup{}, false
	}
	main := domain.Push
	for cat := domain.Push; cat <= domain.Other; cat++ {
		if attacker.WinShare[cat] > attacker.WinShare[main] {
			main = cat
		}
	}
	return Matchup{Category: main, WinShare: attacker.WinShare[main], LossShare: defender.LossShare[main]}, true
}

func share(n, of int) float64 {
	if of == 0 {
		return 0
	}
	return float64(n) / float64(of)
}
