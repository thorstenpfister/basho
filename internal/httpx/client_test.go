package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

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
	if first.Status != http.StatusOK || string(first.Body) != `{"ok":true}` || first.NotModified() {
		t.Errorf("first = %d %q", first.Status, first.Body)
	}
	want := Validators{ETag: `"v1"`, LastModified: "Tue, 13 Oct 2026 08:00:00 GMT"}
	if first.Validators != want {
		t.Errorf("Validators = %+v, want %+v", first.Validators, want)
	}

	second, err := c.Get(t.Context(), srv.URL, first.Validators)
	if err != nil {
		t.Fatal(err)
	}
	if !second.NotModified() || len(second.Body) != 0 {
		t.Errorf("second = %d %q, want 304 without body", second.Status, second.Body)
	}
	if got.Get("If-Modified-Since") != want.LastModified {
		t.Errorf("If-Modified-Since = %q", got.Get("If-Modified-Since"))
	}
}

func TestGetReturnsErrorStatusesAsResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	resp, err := New(Options{}).Get(t.Context(), srv.URL, Validators{})
	if err != nil || resp.Status != http.StatusTooManyRequests {
		t.Fatalf("Get = %v, %v; want a 429 response and no error", resp, err)
	}
}

func TestHostIntervalSpacesRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New(Options{Hosts: map[string]HostPolicy{"osada.us": {Interval: 20 * time.Second}}})
		start := time.Now()
		for i := range 3 {
			if err := c.limiters["osada.us"].Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
			if got, want := time.Since(start), time.Duration(i)*20*time.Second; got != want {
				t.Errorf("request %d started after %v, want %v", i, got, want)
			}
		}
		if c.limiters["www.sumo-api.com"] != nil {
			t.Error("a host without a policy got a limiter")
		}
	})
}

// Get stops at the limiter when the context cannot wait long enough: it fails
// at once, without touching the network (example.invalid never resolves).
func TestGetStopsAtLimiterWhenContextIsTooShort(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := New(Options{Hosts: map[string]HostPolicy{"example.invalid": {Interval: time.Minute}}})
		if err := c.limiters["example.invalid"].Wait(t.Context()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		start := time.Now()
		if _, err := c.Get(ctx, "https://example.invalid/", Validators{}); err == nil {
			t.Fatal("Get succeeded, want the limiter to refuse")
		}
		if waited := time.Since(start); waited != 0 {
			t.Errorf("Get waited %v, want an immediate refusal", waited)
		}
	})
}
