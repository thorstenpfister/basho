package derive

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// careerOf gives wrestler 1 one bout a day in each tournament from the
// techniques listed; a leading "-" marks a loss to that technique.
func careerOf(tournaments map[domain.BashoID][]string) []*domain.Bout {
	var career []*domain.Bout
	for _, id := range slices.SortedFunc(maps.Keys(tournaments), domain.BashoID.Compare) {
		for i, k := range tournaments[id] {
			won := k[0] != '-'
			if !won {
				k = k[1:]
			}
			career = append(career, fight(id, i+1, 2, won, domain.Kimarite(k)))
		}
	}
	return career
}

func repeat(n int, k ...string) []string {
	var out []string
	for range n {
		out = append(out, k...)
	}
	return out
}

func TestProfileOf(t *testing.T) {
	career := careerOf(map[domain.BashoID][]string{
		nago: append(repeat(4, "oshidashi", "tsukidashi"), "hatakikomi", "yorikiri", "fusen", "-yorikiri", "-uwatenage", "-yorikiri", "-hatakikomi"),
	})
	p := ProfileOf(1, career, NoCutoff, Career)
	if p.Wins != 10 || p.Losses != 4 || p.Tournaments != 1 {
		t.Fatalf("%d W %d L in %d tournaments; fusen must not count", p.Wins, p.Losses, p.Tournaments)
	}
	if p.WinShare[domain.Push] != 0.8 || p.WinShare[domain.Pull] != 0.1 || p.LossShare[domain.Belt] != 0.5 {
		t.Errorf("shares %v / %v", p.WinShare, p.LossShare)
	}
	if want := []KimariteCount{{"oshidashi", 4}, {"tsukidashi", 4}, {"hatakikomi", 1}}; !slices.Equal(p.TopWins, want) {
		t.Errorf("top wins %v", p.TopWins)
	}
	if p.TopLosses[0] != (KimariteCount{"yorikiri", 2}) || p.Signature.Kimarite != "oshidashi" || p.SignatureShare != 0.4 {
		t.Errorf("loses to %v, signature %v %.2f", p.TopLosses, p.Signature, p.SignatureShare)
	}
	if !p.Enough() || p.Type != PusherThruster {
		t.Errorf("type %v", p.Type)
	}
	if p := ProfileOf(1, career[:9], NoCutoff, Career); p.Enough() || p.Type != NoType {
		t.Errorf("9 wins: %v, enough %v", p.Type, p.Enough())
	}
}

func TestProfileWindows(t *testing.T) {
	bashos := map[domain.BashoID][]string{}
	id := domain.BashoID{Year: 2025, Month: time.January}
	for range 8 { // 202501 … 202603, five wins each
		bashos[id] = repeat(5, "yorikiri")
		id = id.Next()
	}
	bashos[aki] = []string{"oshidashi", "oshidashi"}
	career := careerOf(bashos)

	// Into Aki day 3: Aki plus 5 more is 6 tournaments with 27 wins.
	c := BeforeDay(aki, 3)
	if p := ProfileOf(1, career, c, LastSix); p.Tournaments != 6 || p.Wins != 27 {
		t.Errorf("last 6: %d tournaments, %d wins", p.Tournaments, p.Wins)
	}
	if p := ProfileOf(1, career, c, ThisBasho); p.Tournaments != 1 || p.Wins != 2 {
		t.Errorf("this basho: %d tournaments, %d wins", p.Tournaments, p.Wins)
	}
	if p := ProfileOf(1, career, c, Career); p.Tournaments != 9 || p.Wins != 42 {
		t.Errorf("career: %d tournaments, %d wins", p.Tournaments, p.Wins)
	}
	// Before Aki, with only two wins a tournament: widened to 10 tournaments
	// would be needed, the career has 8, so all of it.
	for id := range bashos {
		bashos[id] = []string{"yorikiri", "yorikiri", "-oshidashi"}
	}
	career = careerOf(bashos)
	if p := ProfileOf(1, career, BeforeDay(aki, 1), LastSix); p.Tournaments != 8 || p.Wins != 16 {
		t.Errorf("widened: %d tournaments, %d wins", p.Tournaments, p.Wins)
	}
	// Day 1 of a tournament: this basho is empty.
	if p := ProfileOf(1, career, BeforeDay(aki, 1), ThisBasho); p.Tournaments != 0 || p.Wins != 0 {
		t.Errorf("this basho on day 1: %+v", p)
	}
}

func TestFighterType(t *testing.T) {
	sh := func(push, belt, throw, pull float64) Shares {
		var s Shares
		s[domain.Push], s[domain.Belt], s[domain.Throw], s[domain.Pull] = push, belt, throw, pull
		s[domain.Other] = 1 - push - belt - throw - pull
		return s
	}
	for _, tt := range []struct {
		s    Shares
		want FighterType
	}{
		{sh(0.64, 0.03, 0.16, 0.17), PusherThruster}, // Takakeisho
		{sh(0.15, 0.65, 0.16, 0.04), BeltFighter},    // Kotoshogiku
		{sh(0.10, 0.46, 0.33, 0.11), BeltFighter},    // Taiho: throws, but under 35%
		{sh(0.19, 0.28, 0.44, 0.07), Thrower},        // Hoshoryu
		{sh(0.34, 0.12, 0.22, 0.30), CounterPuller},  // Tobizaru
		{sh(0.43, 0.16, 0.25, 0.16), AllRounder},     // Ura
	} {
		if got := fighterType(tt.s); got != tt.want {
			t.Errorf("%v: %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestMatchup(t *testing.T) {
	var pusher, belt Profile
	pusher.Wins, pusher.Losses = 10, 10
	pusher.WinShare[domain.Push], pusher.WinShare[domain.Belt] = 0.6, 0.4
	pusher.LossShare[domain.Belt] = 0.5
	belt.Wins, belt.Losses = 10, 10
	belt.WinShare[domain.Belt] = 0.7
	belt.LossShare[domain.Push] = 0.3
	// pusher → belt: 0.6 × 0.3 = 0.18; belt → pusher: 0.7 × 0.5 = 0.35.
	m, ok := MatchupOf(pusher, belt)
	if want := (Matchup{Category: domain.Belt, WinShare: 0.7, LossShare: 0.5}); !ok || m != want {
		t.Errorf("matchup %+v, want %+v", m, want)
	}
	belt.Losses = 0
	if m, ok := MatchupOf(belt, pusher); !ok || !m.AttackerIsA || m.Category != domain.Belt {
		t.Errorf("one direction only: %+v %v", m, ok)
	}
	if _, ok := MatchupOf(Profile{}, Profile{}); ok {
		t.Error("no samples, no matchup")
	}
}

func TestNames(t *testing.T) {
	if LastSix.String() != "last 6 basho" || ThisBasho.String() != "this basho" || Thrower.String() != "Thrower" || NoType.String() != "" {
		t.Error("window and fighter-type names")
	}
}
