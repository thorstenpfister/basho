package domain

import (
	"testing"
	"time"
)

func TestEstimatedStart(t *testing.T) {
	// Known start dates: 196001 and 202609 from sumo-api.com, 202611 published.
	known := map[BashoID]string{
		{1960, time.January}:   "1960-01-10",
		{2026, time.September}: "2026-09-13",
		{2026, time.November}:  "2026-11-08",
	}
	for id, want := range known {
		if got := id.EstimatedStart().String(); got != want {
			t.Errorf("%s: EstimatedStart = %s, want %s", id, got, want)
		}
	}
	// Always a Sunday between the 8th and the 14th.
	for id := FirstBasho; id.Year <= 2040; id = id.Next() {
		d := id.EstimatedStart()
		if d.Weekday() != time.Sunday || d.Day < 8 || d.Day > 14 || d.Month != id.Month {
			t.Fatalf("%s: EstimatedStart = %s (%s)", id, d, d.Weekday())
		}
	}
}

func TestDayDate(t *testing.T) {
	start := NewDate(2026, time.September, 13)
	if got := DayDate(start, 1); got != start {
		t.Errorf("day 1 = %s", got)
	}
	if got := DayDate(start, BashoDays).String(); got != "2026-09-27" {
		t.Errorf("senshuraku = %s, want 2026-09-27 (the API's end date)", got)
	}
}

func TestNamesAndPlaces(t *testing.T) {
	tests := []struct {
		id                BashoID
		name, city, venue string
	}{
		{BashoID{1960, time.January}, "Hatsu", "Tokyo", "Kuramae Kokugikan"},
		{BashoID{1984, time.September}, "Aki", "Tokyo", "Kuramae Kokugikan"},
		{BashoID{1985, time.January}, "Hatsu", "Tokyo", "Ryogoku Kokugikan"},
		{BashoID{2026, time.March}, "Haru", "Osaka", "Osaka Prefectural Gymnasium"},
		{BashoID{1964, time.July}, "Nagoya", "Nagoya", ""}, // before the certain era
		{BashoID{1965, time.July}, "Nagoya", "Nagoya", "Aichi Prefectural Gymnasium"},
		{BashoID{2025, time.July}, "Nagoya", "Nagoya", "IG Arena"},
		{BashoID{1980, time.November}, "Kyushu", "Fukuoka", ""},
		{BashoID{2026, time.November}, "Kyushu", "Fukuoka", "Fukuoka Kokusai Center"},
	}
	for _, tt := range tests {
		if tt.id.Name() != tt.name || tt.id.City() != tt.city || tt.id.Venue() != tt.venue {
			t.Errorf("%s: got %q/%q/%q, want %q/%q/%q", tt.id,
				tt.id.Name(), tt.id.City(), tt.id.Venue(), tt.name, tt.city, tt.venue)
		}
	}
}

func TestHeld(t *testing.T) {
	tests := map[BashoID]Held{
		{2011, time.March}:     NotHeld,
		{2020, time.May}:       NotHeld,
		{2011, time.May}:       HeldSpecial,
		{2020, time.March}:     HeldRegular, // behind closed doors, but official
		{2026, time.September}: HeldRegular,
	}
	for id, want := range tests {
		if got := id.Held(); got != want {
			t.Errorf("%s: Held = %d, want %d", id, got, want)
		}
	}
}
