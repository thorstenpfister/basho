package domain

import (
	"testing"
	"time"
)

func TestParseTournament(t *testing.T) {
	today := NewDate(2026, time.October, 13) // between Aki and Kyushu
	tests := map[string]BashoID{
		"202609":      {2026, time.September},
		"2026-09":     {2026, time.September},
		"aki2026":     {2026, time.September},
		"Aki-2026":    {2026, time.September},
		"HATSU2027":   {2027, time.January},
		"kyushu-2026": {2026, time.November},
		"current":     {2026, time.September},
		"prev":        {2026, time.July},
		"next":        {2026, time.November},
		" Next ":      {2026, time.November},
	}
	for in, want := range tests {
		if got, err := ParseTournament(in, today); err != nil || got != want {
			t.Errorf("ParseTournament(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "202610", "2026-10", "aki26", "aki", "autumn2026", "195901", "kyushu-1959", "2026/09"} {
		if got, err := ParseTournament(bad, today); err == nil {
			t.Errorf("ParseTournament(%q) = %v, want an error", bad, got)
		}
	}
}
