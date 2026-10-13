package httpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// fakeTransport answers from a script of statuses (0 means a transport error),
// repeating the last one, and counts the requests. It needs no network, so it
// runs inside synctest bubbles.
type fakeTransport struct {
	mu         sync.Mutex
	script     []int
	header     http.Header
	delay      time.Duration // simulated server time per request
	calls      int
	startTimes []time.Time
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	status := f.script[min(f.calls, len(f.script)-1)]
	f.calls++
	f.startTimes = append(f.startTimes, time.Now())
	f.mu.Unlock()

	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	if status == 0 {
		return nil, errors.New("connection reset")
	}
	h := f.header
	if h == nil {
		h = http.Header{}
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader("ok"))}, nil
}

func (f *fakeTransport) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func newTestClient(f *fakeTransport, hosts map[string]HostPolicy) *Client {
	return New(Options{Transport: f, Hosts: hosts})
}

const testURL = "https://api.test/x"

func TestGetSendsUserAgentAndValidators(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Tue, 13 Oct 2026 08:00:00 GMT")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(Options{UserAgent: UserAgent("v1.2.3")})
	first, err := c.Get(t.Context(), srv.URL, Validators{})
	if err != nil {
		t.Fatal(err)
	}
	if ua := got.Get("User-Agent"); ua != "basho/v1.2.3 (+https://github.com/thorstenpfister/basho)" {
		t.Errorf("User-Agent = %q", ua)
	}
	want := Validators{ETag: `"v1"`, LastModified: "Tue, 13 Oct 2026 08:00:00 GMT"}
	if !first.OK() || string(first.Body) != `{"ok":true}` || first.Validators != want {
		t.Errorf("first = %d %q %+v", first.Status, first.Body, first.Validators)
	}
	second, err := c.Get(t.Context(), srv.URL, first.Validators)
	if err != nil || !second.NotModified() || got.Get("If-Modified-Since") != want.LastModified {
		t.Errorf("second = %v, %v", second, err)
	}
}

func TestRetriesServerErrorsWithJitter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{503, 0, 200}}
		start := time.Now()
		resp, err := newTestClient(f, nil).Get(t.Context(), testURL, Validators{})
		if err != nil || !resp.OK() || f.count() != 3 {
			t.Fatalf("Get = %v, %v after %d requests; want 200 after 3", resp, err, f.count())
		}
		// Waits of 1 s and 3 s, each ±20 %.
		if took := time.Since(start); took < 3200*time.Millisecond || took > 4800*time.Millisecond {
			t.Errorf("retries took %v, want 3.2–4.8 s", took)
		}
	})
}

func TestNoRetryForClientErrors(t *testing.T) {
	for _, status := range []int{304, 404, 429} {
		synctest.Test(t, func(t *testing.T) {
			f := &fakeTransport{script: []int{status}}
			resp, err := newTestClient(f, nil).Get(t.Context(), testURL, Validators{})
			if err != nil || resp.Status != status || f.count() != 1 {
				t.Errorf("%d: Get = %v, %v after %d requests; want one try", status, resp, err, f.count())
			}
		})
	}
}

func TestGivesUpAfterTwoRetries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{0}}
		if _, err := newTestClient(f, nil).Get(t.Context(), testURL, Validators{}); err == nil || f.count() != 3 {
			t.Errorf("Get = %v after %d requests; want an error after 3", err, f.count())
		}
	})
}

func TestCircuitBreaker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{500}}
		c := newTestClient(f, nil)
		for range circuitThreshold {
			if resp, err := c.Get(t.Context(), testURL, Validators{}); err != nil || resp.Status != 500 {
				t.Fatalf("Get = %v, %v", resp, err)
			}
		}
		calls := f.count()
		if _, err := c.Get(t.Context(), testURL, Validators{}); !errors.Is(err, ErrCircuitOpen) || f.count() != calls {
			t.Fatalf("after %d failures: err = %v, requests %d → %d; want ErrCircuitOpen without a request", circuitThreshold, err, calls, f.count())
		}

		// After the pause one trial goes out; it fails, so the circuit reopens.
		time.Sleep(circuitPause)
		if _, err := c.Get(t.Context(), testURL, Validators{}); err != nil {
			t.Fatalf("trial: %v", err)
		}
		if _, err := c.Get(t.Context(), testURL, Validators{}); !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("after a failed trial: err = %v, want ErrCircuitOpen", err)
		}

		// A successful trial closes it.
		time.Sleep(circuitPause)
		f.mu.Lock()
		f.script = []int{200}
		f.mu.Unlock()
		for range 2 {
			if resp, err := c.Get(t.Context(), testURL, Validators{}); err != nil || !resp.OK() {
				t.Fatalf("after recovery: %v, %v", resp, err)
			}
		}
	})
}

func TestAbandonedTrialIsResolvedByItsOutcome(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{500}}
		c := newTestClient(f, nil)
		for range circuitThreshold {
			_, _ = c.Get(t.Context(), testURL, Validators{})
		}
		time.Sleep(circuitPause)
		// The trial's caller gives up at once, but the trial still runs and fails,
		// so the circuit must not close.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := c.Get(ctx, testURL, Validators{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("abandoned trial: err = %v", err)
		}
		synctest.Wait()
		time.Sleep(5 * time.Second) // let the detached trial finish its retries
		if _, err := c.Get(t.Context(), testURL, Validators{}); !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("after the failed trial: err = %v, want ErrCircuitOpen", err)
		}
	})
}

func TestMergedCallersGetTheirOwnResponse(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{200}, delay: time.Second}
		c := newTestClient(f, nil)
		var a, b *Response
		var wg sync.WaitGroup
		wg.Go(func() { a, _ = c.Get(t.Context(), testURL, Validators{}) })
		wg.Go(func() { b, _ = c.Get(t.Context(), testURL, Validators{}) })
		wg.Wait()
		if a == nil || b == nil || a == b || f.count() != 1 {
			t.Fatalf("a=%p b=%p after %d requests; want two responses from one request", a, b, f.count())
		}
		a.Status = 0
		if b.Status != 200 {
			t.Error("changing one caller's Response changed the other's")
		}
	})
}

func TestRetriesStopWhenTheHostIsPaused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{500}}
		c := newTestClient(f, nil)
		h := c.host("api.test")
		go func() { // another request gets a 429 during the first back-off
			time.Sleep(500 * time.Millisecond)
			h.done(false, throttled, time.Minute)
		}()
		_, _ = c.Get(t.Context(), testURL, Validators{})
		if f.count() != 1 {
			t.Errorf("requests = %d, want no retry into a pause", f.count())
		}
	})
}

func TestTooManyRequestsPausesTheHost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{429, 200}, header: http.Header{"Retry-After": {"30"}}}
		c := newTestClient(f, nil)
		if resp, _ := c.Get(t.Context(), testURL, Validators{}); resp.Status != 429 {
			t.Fatalf("first = %v", resp)
		}
		time.Sleep(29 * time.Second)
		if _, err := c.Get(t.Context(), testURL, Validators{}); !errors.Is(err, ErrCircuitOpen) {
			t.Errorf("within Retry-After: err = %v, want ErrCircuitOpen", err)
		}
		time.Sleep(2 * time.Second)
		if resp, err := c.Get(t.Context(), testURL, Validators{}); err != nil || !resp.OK() {
			t.Errorf("after Retry-After: %v, %v", resp, err)
		}
	})
}

func TestBurstAndInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{200}}
		c := newTestClient(f, map[string]HostPolicy{"api.test": {Interval: time.Second, Burst: 3}})
		start := time.Now()
		for range 5 {
			if _, err := c.Get(t.Context(), testURL+"?"+time.Now().String(), Validators{}); err != nil {
				t.Fatal(err)
			}
		}
		var offsets []time.Duration
		for _, s := range f.startTimes {
			offsets = append(offsets, s.Sub(start))
		}
		want := []time.Duration{0, 0, 0, time.Second, 2 * time.Second}
		for i := range want {
			if offsets[i] != want[i] {
				t.Errorf("request starts = %v, want %v", offsets, want)
				break
			}
		}
	})
}

func TestMaxConcurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{200}, delay: 5 * time.Second}
		c := newTestClient(f, map[string]HostPolicy{"api.test": {MaxConcurrent: 2}})
		var wg sync.WaitGroup
		for i := range 5 {
			wg.Go(func() {
				_, _ = c.Get(t.Context(), testURL+"?"+string(rune('a'+i)), Validators{})
			})
		}
		wg.Wait()
		// Two at a time, 5 s each: starts at 0, 0, 5, 5 and 10 s.
		counts := map[time.Duration]int{}
		for _, at := range f.startTimes {
			counts[at.Sub(f.startTimes[0])]++
		}
		if counts[0] != 2 || counts[5*time.Second] != 2 || counts[10*time.Second] != 1 {
			t.Errorf("request starts = %v, want two at a time", counts)
		}
	})
}

func TestIdenticalRequestsAreMerged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{200}, delay: time.Second}
		c := newTestClient(f, nil)
		var wg sync.WaitGroup
		for range 4 {
			wg.Go(func() {
				if resp, err := c.Get(t.Context(), testURL, Validators{}); err != nil || !resp.OK() {
					t.Errorf("Get = %v, %v", resp, err)
				}
			})
		}
		wg.Wait()
		if f.count() != 1 {
			t.Errorf("requests = %d, want 1 for 4 identical concurrent Gets", f.count())
		}
	})
}

func TestHostIntervalHonoursContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &fakeTransport{script: []int{200}}
		c := newTestClient(f, map[string]HostPolicy{"api.test": {Interval: time.Minute}})
		if _, err := c.Get(t.Context(), testURL, Validators{}); err != nil {
			t.Fatal(err)
		}
		// Callers that give up waiting for the limiter get their own deadline
		// error, and do not count against the host, however many there are.
		for i := range circuitThreshold + 1 {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			_, err := c.Get(ctx, testURL+"?"+string(rune('a'+i)), Validators{})
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("impatient Get %d = %v, want DeadlineExceeded", i, err)
			}
		}
		if _, err := c.Get(t.Context(), testURL+"?z", Validators{}); err != nil {
			t.Errorf("after impatient callers: %v, want the host still open", err)
		}
	})
}

func TestDebugLog(t *testing.T) {
	var buf bytes.Buffer
	f := &fakeTransport{script: []int{200}}
	c := New(Options{Transport: f, Log: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if _, err := c.Get(t.Context(), testURL, Validators{}); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "url="+testURL) || !strings.Contains(out, "status=200") {
		t.Errorf("log = %q", out)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if got := parseRetryAfter(http.StatusTooManyRequests, "120"); got != 2*time.Minute {
		t.Errorf("seconds: %v", got)
	}
	if got := parseRetryAfter(http.StatusTooManyRequests, "soon"); got != defaultRetryAfter {
		t.Errorf("garbage: %v", got)
	}
	if got := parseRetryAfter(http.StatusServiceUnavailable, "5"); got != 0 {
		t.Errorf("non-429: %v", got)
	}
	if got := parseRetryAfter(http.StatusTooManyRequests, "0"); got != 0 {
		t.Errorf("Retry-After 0 = %v, want no pause", got)
	}
	if got := parseRetryAfter(http.StatusTooManyRequests, "86400"); got != maxRetryAfter {
		t.Errorf("a day = %v, want the cap %v", got, maxRetryAfter)
	}
}

func TestUserAgent(t *testing.T) {
	if got := UserAgent("v1"); got != "basho/v1 (+https://github.com/thorstenpfister/basho)" {
		t.Errorf("UserAgent = %q", got)
	}
}
