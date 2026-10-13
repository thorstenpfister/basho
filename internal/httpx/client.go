package httpx

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

// maxBody caps a response body; the largest known response (osada.us
// torikumi.json) is under 1 MB.
const maxBody = 32 << 20

// HostPolicy is how politely one host is treated (SPEC §7.1).
type HostPolicy struct {
	// Interval is the least time between the starts of two requests.
	Interval time.Duration
}

// Options configures a Client.
type Options struct {
	UserAgent string        // see UserAgent
	Timeout   time.Duration // whole request including the body; zero means 15 s
	Hosts     map[string]HostPolicy
}

// Client is basho's polite HTTP client, a subset of SPEC §7.1. It is safe for
// concurrent use.
type Client struct {
	hc        *http.Client
	userAgent string
	limiters  map[string]*rate.Limiter // read-only after New
}

// New returns a Client.
func New(opts Options) *Client {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	limiters := map[string]*rate.Limiter{}
	for host, p := range opts.Hosts {
		limiters[host] = rate.NewLimiter(rate.Every(p.Interval), 1)
	}
	return &Client{
		hc:        &http.Client{Timeout: timeout, Transport: transport},
		userAgent: opts.UserAgent,
		limiters:  limiters,
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

// Response is an HTTP response with its body read.
type Response struct {
	Status     int
	Body       []byte
	Validators Validators // from this response, for the next conditional request
	Duration   time.Duration
}

// NotModified reports a 304 answer to a conditional request.
func (r *Response) NotModified() bool { return r.Status == http.StatusNotModified }

// OK reports a 200 answer.
func (r *Response) OK() bool { return r.Status == http.StatusOK }

// Get fetches rawURL after any wait the host's policy requires. Non-2xx
// statuses are responses, not errors; an error means no response was read.
func (c *Client) Get(ctx context.Context, rawURL string, v Validators) (*Response, error) {
	resp, err := c.get(ctx, rawURL, v)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", rawURL, err)
	}
	return resp, nil
}

func (c *Client) get(ctx context.Context, rawURL string, v Validators) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if l := c.limiters[u.Host]; l != nil {
		if err := l.Wait(ctx); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	if v.ETag != "" {
		req.Header.Set("If-None-Match", v.ETag)
	}
	if v.LastModified != "" {
		req.Header.Set("If-Modified-Since", v.LastModified)
	}

	start := time.Now()
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() // read-only body: a close error changes nothing
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	return &Response{
		Status: resp.StatusCode,
		Body:   body,
		Validators: Validators{
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
		},
		Duration: time.Since(start),
	}, nil
}
