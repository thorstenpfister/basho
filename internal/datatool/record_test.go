package datatool

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
)

// fakeAPI answers like sumo-api.com and osada.us; osada.us honours ETags.
// The recorder requests sequentially, so no locking is needed.
type fakeAPI struct {
	startDate string // returned by /api/basho/{id}
	fail      string // URL suffix that gets a transport error
	status503 string // URL suffix that gets HTTP 503
	calls     int
}

func (f *fakeAPI) Get(_ context.Context, url string, v httpx.Validators) (*httpx.Response, error) {
	f.calls++
	switch {
	case f.fail != "" && strings.HasSuffix(url, f.fail):
		return nil, errors.New("connection refused")
	case f.status503 != "" && strings.HasSuffix(url, f.status503):
		return &httpx.Response{Status: 503}, nil
	case strings.HasPrefix(url, "https://osada.test/"):
		if v.ETag == `"e1"` {
			return &httpx.Response{Status: 304, Validators: v}, nil
		}
		return &httpx.Response{Status: 200, Body: []byte(`{"osada":true}`), Validators: httpx.Validators{ETag: `"e1"`}}, nil
	case strings.HasSuffix(url, "/api/basho/200001"):
		return &httpx.Response{Status: 200, Body: []byte(`{"date":"200001","startDate":"` + f.startDate + `"}`)}, nil
	default:
		return &httpx.Response{Status: 200, Body: []byte(`{"torikumi":[]}`)}, nil
	}
}

// newTestRecorder records tournament 200001. synctest's clock starts at
// 2000-01-01 00:00 UTC, which is 09:00 JST on 1 January.
func newTestRecorder(t *testing.T, api *fakeAPI) *Recorder {
	return &Recorder{
		Config: RecordConfig{
			Out:        t.TempDir(),
			Interval:   10 * time.Minute,
			From:       14 * time.Hour,
			Until:      19*time.Hour + 15*time.Minute,
			Tournament: domain.BashoID{Year: 2000, Month: time.January},
			SumoAPI:    "https://sumo.test",
			Osada:      "https://osada.test",
		},
		HTTP: api,
		last: map[string]*httpx.Response{}, // as Run sets it up
	}
}

func ticks(out string) []string {
	dirs, _ := filepath.Glob(filepath.Join(out, "*", "*"))
	for i, d := range dirs {
		dirs[i], _ = filepath.Rel(out, d)
	}
	return dirs
}

func TestRecordsTheWholeWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{startDate: "2000-01-01T00:00:00Z"} // today is day 1
		r := newTestRecorder(t, api)
		if err := r.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		got := ticks(r.Config.Out)
		// 14:00 to 19:10 every 10 minutes; 19:20 would be past 19:15.
		if len(got) != 32 || got[0] != "20000101/1400" || got[31] != "20000101/1910" {
			t.Fatalf("ticks = %d, first %v, last %v", len(got), got[0], got[len(got)-1])
		}
		if want := 1 + 32*5; api.calls != want {
			t.Errorf("requests = %d, want %d (dates + 5 per tick)", api.calls, want)
		}
		if now := time.Now().In(domain.JST); now.Format("15:04") != "19:10" {
			t.Errorf("run ended at %s, want after the last tick", now.Format("15:04"))
		}

		if tmp, _ := filepath.Glob(filepath.Join(r.Config.Out, "*", "*", "*.tmp")); len(tmp) != 0 {
			t.Errorf("leftover temporary files: %v", tmp)
		}
		// Every tick is self-contained: a 304 repeats the last body.
		for _, tick := range []string{"1400", "1410"} {
			dir := filepath.Join(r.Config.Out, "20000101", tick)
			for _, name := range []string{
				"osada-torikumi.json", "osada-banzuke.json", "sumoapi-basho.json",
				"sumoapi-torikumi-makuuchi.json", "sumoapi-torikumi-juryo.json", "meta.json",
			} {
				if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
					t.Errorf("tick %s: %v", tick, err)
				}
			}
		}
		meta := readMeta(t, filepath.Join(r.Config.Out, "20000101", "1410"))
		if meta[0].Status != 304 {
			t.Errorf("second tick osada entry = %+v, want 304", meta[0])
		}
		if !strings.HasSuffix(meta[3].URL, "/torikumi/Makuuchi/1") {
			t.Errorf("makuuchi URL = %s, want day 1", meta[3].URL)
		}
	})
}

func TestSkipsNonTournamentDays(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{startDate: "2000-01-09T00:00:00Z"} // starts in 8 days
		r := newTestRecorder(t, api)
		if err := r.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if api.calls != 1 || len(ticks(r.Config.Out)) != 0 {
			t.Errorf("requests = %d, ticks = %d; want only the date lookup", api.calls, len(ticks(r.Config.Out)))
		}
	})
}

func TestOnceRecordsOneTickNow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{startDate: "2000-01-09T00:00:00Z"} // not a tournament day
		r := newTestRecorder(t, api)
		r.Config.Once = true
		if err := r.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		got := ticks(r.Config.Out)
		if len(got) != 1 || got[0] != "20000101/0900" {
			t.Fatalf("ticks = %v, want one at 09:00 JST", got)
		}
		// Before day 1 the bout lists of day 1 are requested.
		if meta := readMeta(t, filepath.Join(r.Config.Out, got[0])); !strings.HasSuffix(meta[3].URL, "/Makuuchi/1") {
			t.Errorf("makuuchi URL = %s", meta[3].URL)
		}
	})
}

func TestFailedRequestIsRecordedAndRunContinues(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{startDate: "2000-01-01T00:00:00Z", fail: "/Juryo/1"}
		r := newTestRecorder(t, api)
		r.Config.Until = 14*time.Hour + 10*time.Minute
		if err := r.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := ticks(r.Config.Out); len(got) != 2 {
			t.Fatalf("ticks = %v, want 2", got)
		}
		meta := readMeta(t, filepath.Join(r.Config.Out, "20000101", "1400"))
		if meta[4].Error == "" || meta[4].Status != 0 {
			t.Errorf("juryo entry = %+v, want the error recorded", meta[4])
		}
	})
}

func TestSlightlyLateStartKeepsTheTick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		time.Sleep(5*time.Hour + 3*time.Second) // 14:00:03 JST: just after the first slot
		api := &fakeAPI{startDate: "2000-01-01T00:00:00Z"}
		r := newTestRecorder(t, api)
		r.Config.Until = 14*time.Hour + 10*time.Minute
		if err := r.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := ticks(r.Config.Out); len(got) != 2 || got[0] != "20000101/1400" {
			t.Errorf("ticks = %v, want 1400 and 1410", got)
		}
	})
}

func TestErrorStatusCountsAsFailed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var log strings.Builder
		api := &fakeAPI{startDate: "2000-01-01T00:00:00Z", status503: "/Makuuchi/1"}
		r := newTestRecorder(t, api)
		r.Log = &log
		r.Config.Until = r.Config.From
		r.Config.From -= time.Minute // a window with exactly one tick slot at From+0
		if err := r.Run(t.Context()); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(log.String(), "5 requests, 1 failed") {
			t.Errorf("log = %q, want the 503 counted as failed", log.String())
		}
	})
}

func TestRejectsReversedWindow(t *testing.T) {
	r := newTestRecorder(t, &fakeAPI{})
	r.Config.From, r.Config.Until = r.Config.Until, r.Config.From
	if err := r.Run(t.Context()); err == nil {
		t.Error("Run accepted a window that ends before it starts")
	}
}

func TestStartDateFallsBackToEstimate(t *testing.T) {
	id := domain.BashoID{Year: 2000, Month: time.January}
	for name, api := range map[string]*fakeAPI{
		"unpublished (zero object)": {startDate: "0001-01-01T00:00:00Z"},
		"sumo-api unreachable":      {fail: "/api/basho/200001"},
	} {
		r := newTestRecorder(t, api)
		if got := r.startDate(t.Context(), id); got != id.EstimatedStart() {
			t.Errorf("%s: startDate = %s, want the estimate %s", name, got, id.EstimatedStart())
		}
	}
}

func TestRejectsImpoliteInterval(t *testing.T) {
	r := newTestRecorder(t, &fakeAPI{})
	r.Config.Interval = time.Minute
	if err := r.Run(t.Context()); err == nil {
		t.Error("Run accepted a 1m interval")
	}
}

func TestParseClock(t *testing.T) {
	if got, err := parseClock("19:15"); err != nil || got != 19*time.Hour+15*time.Minute {
		t.Errorf("parseClock = %v, %v", got, err)
	}
	if _, err := parseClock("7pm"); err == nil {
		t.Error("parseClock accepted 7pm")
	}
}

func readMeta(t *testing.T, dir string) []tickEntry {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta []tickEntry
	if err := json.Unmarshal(b, &meta); err != nil {
		t.Fatal(err)
	}
	return meta
}
