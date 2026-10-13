package sumoapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
)

// countingTransport serves recorded responses and counts the requests that
// reach the network (SPEC §7.3).
type countingTransport struct {
	mu    sync.Mutex
	paths []string
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	path := req.URL.Path
	if req.URL.RawQuery != "" {
		path += "?" + req.URL.RawQuery
	}
	t.mu.Lock()
	t.paths = append(t.paths, path)
	t.mu.Unlock()
	resp := &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: http.Header{}, Request: req}
	recorded := fixture.Getter{Dir: fixtureDir, Base: DefaultBaseURL}
	if r, err := recorded.Get(req.Context(), DefaultBaseURL+path, httpx.Validators{}); err == nil && r.OK() {
		resp.StatusCode, resp.Body = http.StatusOK, io.NopCloser(bytes.NewReader(r.Body))
	}
	return resp, nil
}

// take returns the requests since the last call.
func (t *countingTransport) take() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.paths
	t.paths = nil
	return p
}

// stack builds the production chain: polite client → cache → adapter.
func stack(t *testing.T, dir string, offline bool) (*Client, *countingTransport) {
	t.Helper()
	tr := &countingTransport{}
	hc := httpx.New(httpx.Options{UserAgent: "basho-test", Transport: tr})
	c, err := cache.Open(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := &cache.Getter{Cache: c, Next: hc, Policy: Freshness{}.Policy, Offline: offline}
	return New(g, DefaultBaseURL), tr
}

// TestRequestBudget checks the sumo-api rows of SPEC §7.3 for a finished
// tournament that is not in a snapshot (Aki 2026, recorded after it ended).
func TestRequestBudget(t *testing.T) {
	dir := t.TempDir()
	api, tr := stack(t, dir, false)
	ctx := t.Context()

	step := func(name string, want int, actions ...func() error) {
		t.Helper()
		for _, a := range actions {
			if err := a(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if got := tr.take(); len(got) != want {
			t.Errorf("%s: %d requests %v, want %d", name, len(got), got, want)
		}
	}
	basho := func() error { _, err := api.Basho(ctx, b202609); return err }
	banzuke := func(div domain.Division) func() error {
		return func() error { _, err := api.Banzuke(ctx, b202609, div); return err }
	}
	torikumi := func(div domain.Division, day int) func() error {
		return func() error { _, err := api.Torikumi(ctx, b202609, div, day); return err }
	}
	open := []func() error{basho, banzuke(domain.DivMakuuchi), torikumi(domain.DivMakuuchi, 1)}

	step("open, day 1, one division (cold)", 3, open...)
	step("open again (warm)", 0, open...)
	step("move to another day (cold)", 1, torikumi(domain.DivMakuuchi, 13))
	step("move back and forth (warm)", 0, torikumi(domain.DivMakuuchi, 1), torikumi(domain.DivMakuuchi, 13))
	step("toggle division (cold)", 2, banzuke(domain.DivJuryo), torikumi(domain.DivJuryo, 13))
	step("toggle division (warm)", 0, banzuke(domain.DivJuryo), torikumi(domain.DivJuryo, 13))

	// A second run on the same cache: finished data stays fresh, so nothing
	// is fetched again, and --offline works for everything already viewed.
	for _, offline := range []bool{false, true} {
		again, tr2 := stack(t, dir, offline)
		api = again
		viewed := append(open, torikumi(domain.DivMakuuchi, 13), banzuke(domain.DivJuryo), torikumi(domain.DivJuryo, 13))
		for _, a := range viewed {
			if err := a(); err != nil {
				t.Fatalf("second run (offline=%v): %v", offline, err)
			}
		}
		if got := tr2.take(); len(got) != 0 {
			t.Errorf("second run (offline=%v): requests %v, want none", offline, got)
		}
		if offline {
			if err := torikumi(domain.DivMakuuchi, 15)(); !errors.Is(err, cache.ErrOffline) {
				t.Errorf("offline, never viewed: err = %v, want ErrOffline", err)
			}
		}
	}
}
