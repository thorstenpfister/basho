package spoiler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

var aki = domain.BashoID{Year: 2026, Month: time.September}

func bout(div domain.Division, day, match int) domain.Bout {
	return domain.Bout{Basho: aki, Division: div, Day: day, MatchNo: match}
}

func TestRevealAndVisible(t *testing.T) {
	s, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if s.Visible(bout(domain.DivJuryo, 1, 1)) || s.Horizon(aki) != (Horizon{}) {
		t.Error("a new tournament shows results")
	}
	moved, err := s.Reveal(bout(domain.DivMakuuchi, 13, 5))
	if err != nil || !moved {
		t.Fatalf("reveal: %v %v", moved, err)
	}
	for _, tt := range []struct {
		b    domain.Bout
		want bool
	}{
		{bout(domain.DivMakuuchi, 13, 5), true},
		{bout(domain.DivMakuuchi, 13, 6), false},
		{bout(domain.DivJuryo, 13, 14), true}, // Juryo fights first
		{bout(domain.DivMakuuchi, 12, 20), true},
		{bout(domain.DivJuryo, 14, 1), false},
		{domain.Bout{Basho: aki.Next(), Day: 1, Division: domain.DivJuryo, MatchNo: 1}, false},
	} {
		if got := s.Visible(tt.b); got != tt.want {
			t.Errorf("%+v: visible %v", tt.b, got)
		}
	}
	// Revealing an earlier bout leaves the horizon where it is.
	if moved, _ := s.Reveal(bout(domain.DivMakuuchi, 13, 2)); moved || s.Horizon(aki).MatchNo != 5 {
		t.Error("the horizon moved back")
	}
	if got := s.Horizon(aki).String(); got != "Day 13 · Makuuchi bout 5" {
		t.Errorf("String %q", got)
	}
	// Set moves it anywhere.
	if err := s.Set(aki, Horizon{Day: 12}); err != nil || s.Visible(bout(domain.DivJuryo, 13, 1)) || !s.Visible(bout(domain.DivMakuuchi, 12, 20)) {
		t.Error("Set to the end of day 12")
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "basho", "state.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reveal(bout(domain.DivMakuuchi, 13, 19)); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(aki.Next(), Horizon{Day: 2}); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if h := again.Horizon(aki); h != (Horizon{Day: 13, Division: domain.DivMakuuchi, MatchNo: 19}) {
		t.Errorf("after reload: %+v", h)
	}
	if h := again.Horizon(aki.Next()); h != (Horizon{Day: 2}) {
		t.Errorf("after reload: %+v", h)
	}
	if err := os.WriteFile(path, []byte(`{"v":9}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("a future schema loaded")
	}
}

func TestHidden(t *testing.T) {
	s, _ := Load("")
	s.AddHidden(2)
	s.AddHidden(1)
	if s.Hidden() != 3 {
		t.Errorf("%d", s.Hidden())
	}
	s.ClearHidden()
	if s.Hidden() != 0 {
		t.Error("not cleared")
	}
}
