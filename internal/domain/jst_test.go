package domain

import (
	"testing"
	"time"
	_ "time/tzdata" // the real Asia/Tokyo rules, even without a system database
)

func TestJSTMatchesAsiaTokyo(t *testing.T) {
	tokyo, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	// Twice a year from 1960 to 2040: catches any offset change or DST rule.
	for year := 1960; year <= 2040; year++ {
		for _, month := range []time.Month{time.January, time.July} {
			u := time.Date(year, month, 15, 12, 0, 0, 0, time.UTC)
			_, want := u.In(tokyo).Zone()
			if _, got := u.In(JST).Zone(); got != want {
				t.Fatalf("%s: JST offset %d, Asia/Tokyo %d", u.Format("2006-01"), got, want)
			}
		}
	}
}

func TestStartOfDay(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want string // JST date
	}{
		{"evening in Europe is already tomorrow in Japan", time.Date(2026, 11, 7, 16, 30, 0, 0, time.UTC), "2026-11-08"},
		{"just before JST midnight", time.Date(2026, 11, 8, 14, 59, 59, 0, time.UTC), "2026-11-08"},
		{"exactly JST midnight", time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC), "2026-11-09"},
		{"year boundary", time.Date(2026, 12, 31, 15, 0, 0, 0, time.UTC), "2027-01-01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StartOfDay(tt.t)
			if s := got.Format("2006-01-02"); s != tt.want {
				t.Errorf("StartOfDay = %s, want %s", s, tt.want)
			}
			if got.Location() != JST || TimeOfDay(got) != 0 {
				t.Errorf("StartOfDay = %v, want midnight JST", got)
			}
		})
	}
}

func TestTimeOfDay(t *testing.T) {
	// 05:30 UTC is 14:30 JST: inside the live window that opens at 14:00.
	got := TimeOfDay(time.Date(2026, 11, 8, 5, 30, 0, 0, time.UTC))
	if want := 14*time.Hour + 30*time.Minute; got != want {
		t.Errorf("TimeOfDay = %v, want %v", got, want)
	}
}
