package httpx

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ErrCircuitOpen is returned without a request while a host is paused: after
// repeated failures, or after a 429 until its Retry-After has passed.
var ErrCircuitOpen = errors.New("host paused after failures or rate limiting")

const (
	circuitThreshold = 3               // consecutive failed requests that open the circuit
	circuitPause     = 2 * time.Minute // how long it stays open
)

// HostPolicy is how politely one host is treated (SPEC §7.1).
type HostPolicy struct {
	Interval      time.Duration // average time between request starts; zero: no limit
	Burst         int           // requests allowed back to back before Interval applies; zero means 1
	MaxConcurrent int           // requests in flight at once; zero: no limit
}

// outcome is what a finished request tells the circuit breaker.
type outcome int

const (
	succeeded outcome = iota
	failed            // transport error or 5xx, after retries
	throttled         // 429
)

// host is the per-host state: rate, concurrency and the circuit breaker.
//
// The circuit is closed while openUntil is zero. A failure streak or a 429
// sets openUntil; until then every request is refused. After it, exactly one
// trial request is admitted (half-open), and only its outcome closes or
// reopens the circuit.
type host struct {
	limiter *rate.Limiter // nil: no rate limit
	slots   chan struct{} // nil: no concurrency limit

	mu        sync.Mutex
	failures  int // consecutive failed requests while closed
	openUntil time.Time
	trial     bool // the half-open trial is in flight
}

func newHost(p HostPolicy) *host {
	h := &host{}
	if p.Interval > 0 {
		h.limiter = rate.NewLimiter(rate.Every(p.Interval), max(p.Burst, 1))
	}
	if p.MaxConcurrent > 0 {
		h.slots = make(chan struct{}, p.MaxConcurrent)
	}
	return h
}

// admit decides whether a request may go out, and whether it is the trial.
func (h *host) admit() (trial bool, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.openUntil.IsZero():
		return false, nil
	case time.Now().Before(h.openUntil) || h.trial:
		return false, ErrCircuitOpen
	}
	h.trial = true
	return true, nil
}

// done records an admitted request's outcome. While the circuit is open or
// half-open, only the trial's outcome counts; requests admitted earlier do
// not close it. A 429 always pauses (or extends the pause).
func (h *host) done(trial bool, o outcome, pause time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if trial {
		h.trial = false
	}
	if o == throttled && pause > 0 {
		h.openUntil = later(h.openUntil, time.Now().Add(pause))
		return
	}
	if !trial && !h.openUntil.IsZero() {
		return
	}
	switch o {
	case succeeded:
		h.failures, h.openUntil = 0, time.Time{}
	default: // failed, or a 429 without a pause
		h.failures++
		if trial || h.failures >= circuitThreshold {
			h.openUntil = time.Now().Add(circuitPause)
		}
	}
}

// paused reports whether the host currently refuses requests.
func (h *host) paused() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return time.Now().Before(h.openUntil)
}

// acquire waits for a concurrency slot and the rate limit; release frees the slot.
func (h *host) acquire(ctx context.Context) error {
	if h.slots != nil {
		select {
		case h.slots <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if h.limiter != nil {
		if err := h.limiter.Wait(ctx); err != nil {
			h.release()
			return err
		}
	}
	return nil
}

func (h *host) release() {
	if h.slots != nil {
		<-h.slots
	}
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
