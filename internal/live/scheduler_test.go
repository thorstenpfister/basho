package live

import (
	"errors"
	"testing"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

var kyushu = domain.NewDate(2026, time.November, 8) // day 1

func at(day, h, m int) time.Time {
	return kyushu.AddDays(day - 1).Time().Add(time.Duration(h)*time.Hour + time.Duration(m)*time.Minute)
}

func on() *Scheduler {
	s := New(Config{Interval: 3 * time.Minute}, kyushu)
	s.SetOn(true)
	return s
}

func TestStates(t *testing.T) {
	s := on()
	for _, tt := range []struct {
		now  time.Time
		want State
	}{
		{at(1, 10, 0), Waiting},
		{at(1, 14, 0), Polling},
		{at(1, 18, 44), Polling},
		{at(1, 18, 45), Done},
		{at(15, 19, 0), Polling}, // day 15 runs until 19:15
		{at(15, 19, 15), Done},
		{at(0, 15, 0), Unavailable},
		{at(16, 15, 0), Unavailable},
	} {
		if got := s.State(tt.now); got != tt.want {
			t.Errorf("%s: %s; want %s", tt.now.Format("01-02 15:04"), got, tt.want)
		}
	}
	if got := New(Config{}, kyushu).State(at(1, 15, 0)); got != Off {
		t.Errorf("not turned on: %s", got)
	}
	if got := New(Config{}, domain.Date{}).DayNumber(at(1, 15, 0)); got != 0 {
		t.Errorf("no tournament: day %d", got)
	}
	if !s.InWindow(at(3, 14, 30)) || s.InWindow(at(3, 13, 59)) || s.InWindow(at(16, 15, 0)) {
		t.Error("InWindow")
	}
}

func TestWake(t *testing.T) {
	s := on()
	if got := s.Wake(at(2, 9, 0)); !got.Equal(at(2, 14, 0)) {
		t.Errorf("before the window: %s", got)
	}
	if got := s.Wake(at(2, 19, 0)); !got.Equal(at(3, 14, 0)) {
		t.Errorf("after the window: %s", got)
	}
	if got := s.Wake(at(15, 20, 0)); !got.IsZero() {
		t.Errorf("after day 15: %s", got)
	}
	if !s.Due(at(2, 14, 0)) {
		t.Error("not due when the window opens")
	}
}

func TestInterval(t *testing.T) {
	s := on()
	now := at(1, 15, 0)
	stamp := now
	s.Polled(now, Result{UpdatedAt: stamp})
	if got := s.Wake(now); !got.Equal(now.Add(3 * time.Minute)) {
		t.Fatalf("next %s", got)
	}
	if s.Due(now.Add(2 * time.Minute)) {
		t.Error("due early")
	}
	// The same stamp twice: the interval doubles, up to 10 minutes.
	var gaps []time.Duration
	for range 6 {
		now = s.Wake(now)
		if !s.Due(now) {
			t.Fatal("not due at its wake time")
		}
		s.Polled(now, Result{UpdatedAt: stamp})
		gaps = append(gaps, s.Wake(now).Sub(now))
	}
	want := []time.Duration{3, 6, 6, 10, 10, 10}
	for i, g := range gaps {
		if g != want[i]*time.Minute {
			t.Errorf("gaps %v; want %v minutes", gaps, want)
			break
		}
	}
	// A new stamp goes back to the base interval.
	now = s.Wake(now)
	s.Polled(now, Result{UpdatedAt: stamp.Add(time.Hour)})
	if got := s.Wake(now).Sub(now); got != 3*time.Minute {
		t.Errorf("after a change: %s", got)
	}
	// The minimum.
	if s := New(Config{Interval: time.Minute}, kyushu); s.cfg.Interval != MinInterval {
		t.Errorf("interval %s", s.cfg.Interval)
	}
}

func TestJitter(t *testing.T) {
	for _, r := range []float64{0, 0.5, 0.999} {
		s := New(Config{Interval: 3 * time.Minute, Rand: func() float64 { return r }}, kyushu)
		s.SetOn(true)
		now := at(1, 15, 0)
		s.Polled(now, Result{UpdatedAt: now})
		if gap := s.Wake(now).Sub(now); gap < 3*time.Minute-Jitter || gap > 3*time.Minute+Jitter {
			t.Errorf("rand %v: %s", r, gap)
		}
	}
}

func TestBackoff(t *testing.T) {
	s := on()
	now := at(1, 15, 0)
	var gaps []time.Duration
	for range 5 {
		s.Polled(now, Result{Err: errors.New("down"), Fallback: "sumo-api"})
		if st := s.Status(now); st.State != Backoff || st.Fallback != "sumo-api" || st.Err == nil {
			t.Fatalf("status %+v", st)
		}
		next := s.Wake(now)
		gaps = append(gaps, next.Sub(now))
		now = next
	}
	want := []time.Duration{2, 4, 8, 10, 10}
	for i, g := range gaps {
		if g != want[i]*time.Minute {
			t.Errorf("backoff %v; want %v minutes", gaps, want)
			break
		}
	}
	s.Polled(now, Result{UpdatedAt: now})
	if st := s.Status(now); st.State != Polling || st.Err != nil {
		t.Errorf("after recovery: %+v", st)
	}
}

func TestDoneAndFinalCheck(t *testing.T) {
	s := on()
	now := at(4, 18, 2)
	s.Polled(now, Result{UpdatedAt: now, Complete: true})
	st := s.Status(now)
	if st.State != Done || !st.FinalCheck.Equal(at(4, 18, 32)) || !st.Next.Equal(at(4, 18, 32)) {
		t.Fatalf("status %+v", st)
	}
	if s.Due(at(4, 18, 20)) || !s.Due(at(4, 18, 32)) {
		t.Error("final check timing")
	}
	s.Polled(at(4, 18, 32), Result{UpdatedAt: now, Complete: true})
	if s.Due(at(4, 18, 40)) || !s.Wake(at(4, 18, 40)).Equal(at(5, 14, 0)) {
		t.Error("polled after the final check")
	}
	// The next day starts afresh.
	if st := s.Status(at(5, 14, 0)); st.State != Polling || !st.FinalCheck.IsZero() || !s.Due(at(5, 14, 0)) {
		t.Errorf("next day: %+v", st)
	}
}

func TestToggleAndReset(t *testing.T) {
	s := on()
	now := at(1, 15, 0)
	s.Polled(now, Result{UpdatedAt: now})
	s.SetOn(false)
	if s.Due(now.Add(5*time.Minute)) || !s.Wake(now).IsZero() {
		t.Error("off but due")
	}
	s.SetOn(true)
	if !s.Due(now.Add(time.Second)) {
		t.Error("turning on does not poll at once")
	}
	s.Polled(now, Result{UpdatedAt: now})
	s.Reset()
	if !s.Due(now.Add(time.Second)) {
		t.Error("R does not poll at once")
	}
}
