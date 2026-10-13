package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDateOf(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"evening in Europe is already tomorrow in Japan", time.Date(2026, 11, 7, 16, 30, 0, 0, time.UTC), "2026-11-08"},
		{"just before JST midnight", time.Date(2026, 11, 8, 14, 59, 59, 0, time.UTC), "2026-11-08"},
		{"exactly JST midnight", time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC), "2026-11-09"},
		{"year boundary", time.Date(2026, 12, 31, 15, 0, 0, 0, time.UTC), "2027-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DateOf(tt.t).String(); got != tt.want {
				t.Errorf("DateOf = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestDateArithmetic(t *testing.T) {
	d := NewDate(2026, time.December, 28)
	if got := d.AddDays(5).String(); got != "2027-01-02" {
		t.Errorf("AddDays(5) = %s", got)
	}
	if got := d.AddDays(-28).String(); got != "2026-11-30" {
		t.Errorf("AddDays(-28) = %s", got)
	}
	if got := NewDate(2026, time.October, 32).String(); got != "2026-11-01" {
		t.Errorf("NewDate normalisation = %s", got)
	}
	if got := NewDate(2026, time.November, 8).Weekday(); got != time.Sunday {
		t.Errorf("Weekday = %s, want Sunday", got)
	}
	if NewDate(2026, 11, 8).Compare(NewDate(2026, 11, 9)) != -1 || d.Compare(d) != 0 {
		t.Error("Compare")
	}
	// Comparable and usable as a map key: the reason Date exists.
	seen := map[Date]bool{NewDate(2026, 11, 8): true}
	if !seen[DateOf(time.Date(2026, 11, 8, 3, 0, 0, 0, time.UTC))] {
		t.Error("map lookup by an equal Date failed")
	}
	if got := NewDate(2026, 11, 8).Time(); got.Location() != JST || TimeOfDay(got) != 0 {
		t.Errorf("Time = %v, want midnight JST", got)
	}
}

func TestDateText(t *testing.T) {
	type doc struct {
		Start Date `json:"start"`
		End   Date `json:"end"`
	}
	in := doc{Start: NewDate(2026, 11, 8)}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"start":"2026-11-08","end":""}` {
		t.Errorf("marshal = %s", b)
	}
	var out doc
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round trip = %+v, want %+v", out, in)
	}
	if _, err := ParseDate("2026-13-01"); err == nil {
		t.Error("ParseDate accepted month 13")
	}
}
