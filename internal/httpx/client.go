package httpx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/thorstenpfister/basho/internal/clock"
)

// maxBody caps a response body; the largest known response (osada.us
// torikumi.json) is under 1 MB.
const maxBody = 32 << 20

// retryDelays are the waits before the retries of a failed request (SPEC §7.1);
// each is jittered by ±20 %.
var retryDelays = []time.Duration{time.Second, 3 * time.Second}

const (
	defaultRetryAfter = time.Minute // a 429 without a usable Retry-After
	maxRetryAfter     = time.Hour   // longer requests are capped
)

// Options configures a Client.
type Options struct {
	UserAgent string        // see UserAgent
	Timeout   time.Duration // whole request including the body; zero means 15 s
	Hosts     map[string]HostPolicy
	Log       *slog.Logger      // debug log of every request; nil: none
	Transport http.RoundTripper // nil: a transport with a 5 s connect timeout
}

// Getter is what callers need from a Client; tests substitute a fake.
type Getter interface {
	Get(ctx context.Context, url string, v Validators) (*Response, error)
}

// Client is basho's polite HTTP client (SPEC §7.1). It is safe for
// concurrent use.
type Client struct {
	hc        *http.Client
	userAgent string
	policies  map[string]HostPolicy
	log       *slog.Logger
	group     singleflight.Group

	mu    sync.Mutex
	hosts map[string]*host
}

// New returns a Client.
func New(opts Options) *Client {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	transport := opts.Transport
	if transport == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
		transport = t
	}
	return &Client{
		hc:        &http.Client{Timeout: timeout, Transport: transport},
		userAgent: opts.UserAgent,
		policies:  opts.Hosts,
		log:       opts.Log,
		hosts:     map[string]*host{},
	}
}

// UserAgent returns basho's User-Agent for version (SPEC §7.1).
func UserAgent(version string) string {
	return "basho/" + version + " (+https://github.com/thorstenpfister/basho)"
}

// Validators make a request conditional (If-None-Match, If-Modified-Since).
type Validators struct {
	ETag         string
	LastModified string
}

// Response is an HTTP response with its body read. Callers get their own
// Response, but merged requests share Body: treat it as read-only.
type Response struct {
	Status     int
	Body       []byte
	Validators Validators // from this response, for the next conditional request
	Duration   time.Duration
	Stale      bool // served from a cache because the source could not be reached
}

// NotModified reports a 304 answer to a conditional request.
func (r *Response) NotModified() bool { return r.Status == http.StatusNotModified }

// OK reports a 200 answer.
func (r *Response) OK() bool { return r.Status == http.StatusOK }

// Get fetches rawURL politely: it waits for the host's rate and concurrency
// limits, retries transport errors and 5xx twice with jitter, pauses the host
// after repeated failures or a 429 (ErrCircuitOpen), and merges identical
// requests in flight. Non-2xx statuses are responses, not errors; an error
// means no response was read.
//
// The request itself runs detached from ctx, so one impatient caller neither
// cancels it for merged callers nor teaches the circuit breaker anything
// about the host; ctx only bounds how long this caller waits.
func (c *Client) Get(ctx context.Context, rawURL string, v Validators) (*Response, error) {
	key := rawURL + "\x00" + v.ETag + "\x00" + v.LastModified
	ch := c.group.DoChan(key, func() (any, error) {
		return c.get(context.WithoutCancel(ctx), rawURL, v)
	})
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("get %s: %w", rawURL, ctx.Err())
	case r := <-ch:
		if r.Err != nil {
			return nil, fmt.Errorf("get %s: %w", rawURL, r.Err)
		}
		resp := *r.Val.(*Response)
		return &resp, nil
	}
}

func (c *Client) get(ctx context.Context, rawURL string, v Validators) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	h := c.host(u.Host)
	trial, err := h.admit()
	if err != nil {
		return nil, err
	}
	var (
		resp       *Response
		retryAfter time.Duration
	)
	for attempt := 0; ; attempt++ {
		resp, retryAfter, err = c.attempt(ctx, h, rawURL, v)
		if !retryable(resp, err) || attempt == len(retryDelays) {
			break
		}
		// Back off, then retry only if the host has not been paused meanwhile
		// (a 429 or an opened circuit from another request).
		if clock.Sleep(ctx, jitter(retryDelays[attempt])) != nil || h.paused() {
			break
		}
	}
	switch {
	case resp != nil && resp.Status == http.StatusTooManyRequests:
		h.done(trial, throttled, retryAfter)
	case retryable(resp, err):
		h.done(trial, failed, 0)
	default:
		h.done(trial, succeeded, 0)
	}
	return resp, err
}

// attempt makes one request once the host's limits allow it, and returns the
// pause a 429 asked for.
func (c *Client) attempt(ctx context.Context, h *host, rawURL string, v Validators) (*Response, time.Duration, error) {
	if err := h.acquire(ctx); err != nil {
		return nil, 0, err
	}
	defer h.release()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	if v.ETag != "" {
		req.Header.Set("If-None-Match", v.ETag)
	}
	if v.LastModified != "" {
		req.Header.Set("If-Modified-Since", v.LastModified)
	}

	start := time.Now()
	resp, retryAfter, err := c.read(req)
	took := time.Since(start)
	if resp != nil {
		resp.Duration = took
	}
	if c.log != nil && c.log.Enabled(ctx, slog.LevelDebug) {
		attrs := []any{"url", rawURL, "duration", took}
		if err != nil {
			attrs = append(attrs, "error", err)
		} else {
			attrs = append(attrs, "status", resp.Status, "bytes", len(resp.Body))
		}
		c.log.DebugContext(ctx, "http", attrs...)
	}
	return resp, retryAfter, err
}

func (c *Client) read(req *http.Request) (*Response, time.Duration, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }() // read-only body: a close error changes nothing
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, 0, fmt.Errorf("read body: %w", err)
	}
	return &Response{
		Status: resp.StatusCode,
		Body:   body,
		Validators: Validators{
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
		},
	}, parseRetryAfter(resp.StatusCode, resp.Header.Get("Retry-After")), nil
}

func (c *Client) host(name string) *host {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.hosts[name]
	if !ok {
		h = newHost(c.policies[name])
		c.hosts[name] = h
	}
	return h
}

// retryable reports a transport failure or a server error. 4xx, including 429,
// is never retried.
func retryable(resp *Response, err error) bool {
	return err != nil || resp.Status >= 500
}

// parseRetryAfter reads a 429's Retry-After value (seconds or an HTTP date).
// Zero or a past date means no pause; a missing or unreadable value means
// defaultRetryAfter; anything longer than maxRetryAfter is capped.
func parseRetryAfter(status int, v string) time.Duration {
	if status != http.StatusTooManyRequests {
		return 0
	}
	d := defaultRetryAfter
	if secs, err := strconv.Atoi(v); err == nil {
		d = time.Duration(max(secs, 0)) * time.Second
	} else if t, err := http.ParseTime(v); err == nil {
		d = max(time.Until(t), 0)
	}
	return min(d, maxRetryAfter)
}

// jitter spreads d by ±20 %.
func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.8 + 0.4*rand.Float64()))
}
