package domain

import (
	"testing"
	"time"
)

func TestLiveWindowContains(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, time.November, 8, h, m, 0, 0, JST) }
	w := DefaultLiveWindow
	for _, c := range []struct {
		t        time.Time
		finalDay bool
		want     bool
	}{
		{at(13, 59), false, false},
		{at(14, 0), false, true},
		{at(18, 44), false, true},
		{at(18, 45), false, false},
		{at(19, 0), true, true},
		{at(19, 15), true, false},
		{time.Date(2026, time.November, 8, 5, 30, 0, 0, time.UTC), false, true}, // 14:30 JST
	} {
		if got := w.Contains(c.t, c.finalDay); got != c.want {
			t.Errorf("Contains(%v, final=%v) = %v, want %v", c.t, c.finalDay, got, c.want)
		}
	}
}
