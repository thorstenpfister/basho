package live

import (
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
)

// Limits of SPEC §10.2.
const (
	MinInterval  = 2 * time.Minute  // the base interval is never shorter
	MaxInterval  = 10 * time.Minute // doubling and backoff stop here
	Jitter       = 15 * time.Second // ± around each interval
	FinalCheck   = 30 * time.Minute // after the day is complete
	firstBackoff = 2 * time.Minute
	// unchangedMax is how many polls without a change double the
	// interval: three, as Makuuchi results come about every 6 minutes
	// (Kyushu 2026), so two can miss one by a second.
	unchangedMax = 3
)

// State is what live mode is doing (SPEC §10.3).
type State int

const (
	Off         State = iota // turned off
	Waiting                  // a tournament day, before the window
	Polling                  // inside the window
	Backoff                  // the last poll failed
	Done                     // the day is complete, or its window is over
	Unavailable              // not a tournament day
)

var stateNames = [...]string{"off", "waiting", "polling", "backoff", "done", "unavailable"}

func (s State) String() string { return stateNames[s] }

// Config is the schedule's settings.
type Config struct {
	Interval time.Duration     // base interval; raised to MinInterval
	Window   domain.LiveWindow // zero: domain.DefaultLiveWindow
	// Rand returns a number in [0, 1) for the jitter; nil: no jitter.
	Rand func() float64
}

// Result is what a poll found.
type Result struct {
	UpdatedAt time.Time // when the results last changed (service.LivePoll.UpdatedAt)
	Complete  bool      // every bout of today in both divisions decided, no playoff pending
	Err       error     // the primary source failed (the fallback may still have answered)
	Fallback  string    // the source that answered instead, if any
}

// Scheduler decides when live mode polls (SPEC §10.2). It is pure: every
// method takes the time, so tests drive it with a fake clock. It is not safe
// for concurrent use; the TUI owns it.
type Scheduler struct {
	cfg   Config
	start domain.Date // day 1 of the current tournament; zero: none in progress
	on    bool

	day       domain.Date // the tournament day the fields below belong to
	next      time.Time   // the next poll; zero: poll at once
	interval  time.Duration
	updatedAt time.Time
	unchanged int
	errors    int
	lastErr   error
	fallback  string
	doneAt    time.Time // when the day was found complete
	finalDone bool      // the final check ran
}

// New returns a scheduler, off, for the tournament starting on start (zero
// when none is in progress).
func New(cfg Config, start domain.Date) *Scheduler {
	if cfg.Interval < MinInterval {
		cfg.Interval = MinInterval
	}
	if cfg.Window == (domain.LiveWindow{}) {
		cfg.Window = domain.DefaultLiveWindow
	}
	return &Scheduler{cfg: cfg, start: start, interval: cfg.Interval}
}

// On reports whether live mode is on.
func (s *Scheduler) On() bool { return s.on }

// SetOn turns live mode on or off. Turning it on polls at once inside the
// window.
func (s *Scheduler) SetOn(on bool) {
	s.on = on
	if on {
		s.next = time.Time{}
	}
}

// Reset is the manual refresh R: the next poll is due now, at the base
// interval (the caller keeps the 30 s cooldown).
func (s *Scheduler) Reset() {
	s.next, s.interval, s.unchanged = time.Time{}, s.cfg.Interval, 0
}

// DayNumber returns the tournament day of now, 0 outside the tournament.
func (s *Scheduler) DayNumber(now time.Time) int {
	if s.start.IsZero() {
		return 0
	}
	if n := domain.DateOf(now).DaysSince(s.start) + 1; n >= 1 && n <= domain.BashoDays {
		return n
	}
	return 0
}

// window returns today's window as times.
func (s *Scheduler) window(now time.Time) (open, close time.Time) {
	midnight := domain.DateOf(now).Time()
	end := s.cfg.Window.End
	if s.DayNumber(now) == domain.BashoDays {
		end = s.cfg.Window.FinalDayEnd
	}
	return midnight.Add(s.cfg.Window.Start), midnight.Add(end)
}

// InWindow reports whether now is inside a tournament day's window: where
// live.auto turns live mode on.
func (s *Scheduler) InWindow(now time.Time) bool {
	open, close := s.window(now)
	return s.DayNumber(now) > 0 && !now.Before(open) && now.Before(close)
}

// newDay forgets the previous day's polling.
func (s *Scheduler) newDay(now time.Time) {
	if d := domain.DateOf(now); d != s.day {
		*s = Scheduler{cfg: s.cfg, start: s.start, on: s.on, day: d, interval: s.cfg.Interval}
	}
}

// State returns the state as of now.
func (s *Scheduler) State(now time.Time) State {
	s.newDay(now)
	open, close := s.window(now)
	switch {
	case !s.on:
		return Off
	case s.DayNumber(now) == 0:
		return Unavailable
	case !s.doneAt.IsZero() || !now.Before(close):
		return Done
	case now.Before(open):
		return Waiting
	case s.lastErr != nil:
		return Backoff
	}
	return Polling
}

// Due reports whether a poll should run now.
func (s *Scheduler) Due(now time.Time) bool {
	switch s.State(now) {
	case Polling, Backoff:
		return !now.Before(s.next)
	case Done:
		return s.finalDue(now)
	}
	return false
}

func (s *Scheduler) finalDue(now time.Time) bool {
	return !s.doneAt.IsZero() && !s.finalDone && !now.Before(s.doneAt.Add(FinalCheck))
}

// Wake returns when the scheduler next needs a look: the next poll, the
// window opening, the final check, or tomorrow's window. Zero: never (off,
// or the tournament is over).
func (s *Scheduler) Wake(now time.Time) time.Time {
	open, _ := s.window(now)
	switch s.State(now) {
	case Off:
		return time.Time{}
	case Waiting:
		return open
	case Polling, Backoff:
		if s.next.Before(now) {
			return now
		}
		return s.next
	case Done:
		if !s.doneAt.IsZero() && !s.finalDone {
			return s.doneAt.Add(FinalCheck)
		}
	}
	return s.tomorrow(now)
}

// NextOpen returns the next window opening from now: today's, while it is
// still ahead, else the next tournament day's; zero after the tournament.
// live.auto turns live mode on then.
func (s *Scheduler) NextOpen(now time.Time) time.Time {
	if open, _ := s.window(now); s.DayNumber(now) > 0 && now.Before(open) {
		return open
	}
	return s.tomorrow(now)
}

// tomorrow is the next tournament day's window opening; zero after day 15.
func (s *Scheduler) tomorrow(now time.Time) time.Time {
	next := now.AddDate(0, 0, 1)
	if s.DayNumber(next) == 0 {
		return time.Time{}
	}
	open, _ := s.window(next)
	return open
}

// Polled records a poll's result at now and schedules the next one.
func (s *Scheduler) Polled(now time.Time, r Result) {
	s.newDay(now)
	if !s.doneAt.IsZero() { // the final check
		s.finalDone = true
		s.lastErr = r.Err
		return
	}
	s.fallback = r.Fallback
	if r.Err != nil {
		s.lastErr = r.Err
		s.errors++
		s.next = now.Add(min(firstBackoff<<(s.errors-1), MaxInterval))
		if !r.Complete {
			return
		}
	} else {
		s.lastErr, s.errors = nil, 0
	}
	if r.Complete {
		s.doneAt = now
		return
	}
	switch {
	case r.UpdatedAt.IsZero() || !r.UpdatedAt.Equal(s.updatedAt):
		s.updatedAt, s.unchanged, s.interval = r.UpdatedAt, 0, s.cfg.Interval
	default:
		s.unchanged++
		if s.unchanged >= unchangedMax {
			s.interval, s.unchanged = min(2*s.interval, MaxInterval), 0
		}
	}
	if r.Err == nil {
		s.next = now.Add(s.jitter(s.interval))
	}
}

func (s *Scheduler) jitter(d time.Duration) time.Duration {
	if s.cfg.Rand == nil {
		return d
	}
	return d + time.Duration((2*s.cfg.Rand()-1)*float64(Jitter))
}

// Status is what the status bar shows (SPEC §10.3).
type Status struct {
	State      State
	Next       time.Time // the next poll, window opening or final check
	UpdatedAt  time.Time // the source's last update stamp
	Err        error     // Backoff: the last failure
	Fallback   string    // the source that answered instead
	FinalCheck time.Time // Done: when the final check runs; zero once it ran
}

// Status returns the status as of now.
func (s *Scheduler) Status(now time.Time) Status {
	st := Status{State: s.State(now), Next: s.Wake(now), UpdatedAt: s.updatedAt, Err: s.lastErr, Fallback: s.fallback}
	if !s.doneAt.IsZero() && !s.finalDone {
		st.FinalCheck = s.doneAt.Add(FinalCheck)
	}
	return st
}
