package service

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/derive"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/rating"
	"github.com/thorstenpfister/basho/internal/snapshot"
	"github.com/thorstenpfister/basho/internal/source"
)

var (
	aki2026  = domain.BashoID{Year: 2026, Month: time.September}
	testFile = rating.File{Schema: rating.FileSchema, Params: rating.Params{K: 10, KNew: 24, NewBouts: 45, R: 0.3, WarmupScale: 0.5}, Show: true}
)

// fixtureService is a service over the fixture snapshot (196001, 202607,
// 202609; SPEC §13).
func fixtureService(t *testing.T, f rating.File) *Service {
	t.Helper()
	return New(Options{
		Snapshot: func() (*snapshot.Snapshot, error) { return snapshot.Open(os.DirFS("../../testdata/snapshot")) },
		Params:   func() (rating.File, error) { return f, nil },
		Clock:    clock.NewFake(time.Date(2026, time.October, 14, 12, 0, 0, 0, domain.JST)),
	})
}

func TestRatings(t *testing.T) {
	s := fixtureService(t, testFile)
	r, err := s.Ratings()
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Ratings(); again != r {
		t.Error("ratings built twice")
	}
	x, _ := s.index()
	var b domain.Bout
	for _, bt := range x.Tournament(aki2026) {
		if bt.Winner != 0 && bt.Kimarite.Category() != domain.NoTechnique {
			b = bt
			break
		}
	}
	// The win chance comes from both ratings before the bout.
	c := derive.BeforeBout(b)
	e, w := r.At(b.East.RikishiID, b.East.Rank, c), r.At(b.West.RikishiID, b.West.Rank, c)
	if got := r.WinChance(b); got != rating.WinChance(e.Value, w.Value) {
		t.Errorf("win chance %.4f", got)
	}
	// After the bout the winner gained what the loser lost.
	after := derive.ThroughBout(b)
	winner, loser := b.East, b.West
	if b.Winner == domain.West {
		winner, loser = loser, winner
	}
	gain := r.At(winner.RikishiID, winner.Rank, after).Value - r.At(winner.RikishiID, winner.Rank, c).Value
	if gain <= 0 {
		t.Errorf("the winner gained %.2f", gain)
	}
	if loss := r.At(loser.RikishiID, loser.Rank, c).Value - r.At(loser.RikishiID, loser.Rank, after).Value; loss <= 0 {
		t.Errorf("the loser lost %.2f", loss)
	}

	// Upsets: the side the model gave under 35% won.
	east := r.WinChance(b)
	won := east
	if b.Winner == domain.West {
		won = 1 - east
	}
	if r.IsUpset(&b) != (won < upsetChance) {
		t.Errorf("upset %v at %.2f", r.IsUpset(&b), won)
	}
	fusen := b
	fusen.Kimarite = "fusen"
	if r.IsUpset(&fusen) {
		t.Error("a fusen is no upset")
	}
}

func TestRatingTable(t *testing.T) {
	s := fixtureService(t, testFile)
	res, err := s.RatingTable(t.Context(), aki2026, 5)
	tab := res.Value
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Rows) != 5 || !tab.Shown || tab.Basho != aki2026 {
		t.Fatalf("table %+v", tab)
	}
	for i := 1; i < len(tab.Rows); i++ {
		if tab.Rows[i-1].Value < tab.Rows[i].Value {
			t.Errorf("not sorted: %+v", tab.Rows)
		}
	}
	allRes, _ := s.RatingTable(t.Context(), aki2026, 0)
	all := allRes.Value
	if len(all.Rows) != 42+28 {
		t.Errorf("all: %d rows, want both divisions (70)", len(all.Rows))
	}
	if _, err := s.RatingTable(t.Context(), domain.BashoID{Year: 2026, Month: time.November}, 5); !errors.Is(err, source.ErrNotYetAvailable) {
		t.Errorf("after the snapshot: %v", err)
	}
	hidden := fixtureService(t, rating.File{Params: testFile.Params})
	if res, _ := hidden.RatingTable(t.Context(), aki2026, 1); res.Value.Shown || res.Prov.Source != SnapshotSource {
		t.Error("V9 failed but shown")
	}
	if n, err := s.Newest(); err != nil || n != aki2026 {
		t.Errorf("newest %v, %v", n, err)
	}
}

func TestRatingsSetLive(t *testing.T) {
	s := fixtureService(t, testFile)
	r, _ := s.Ratings()
	x, _ := s.index()
	kyushu := domain.BashoID{Year: 2026, Month: time.November}
	b := x.Tournament(aki2026)[0]
	b.Basho, b.Day = kyushu, 1
	if err := r.Set(kyushu, []domain.Bout{b}); err != nil {
		t.Fatal(err)
	}
	if err := r.Set(aki2026, nil); err == nil {
		t.Error("an older tournament was accepted")
	}
}
