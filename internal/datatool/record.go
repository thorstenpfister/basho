package datatool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/thorstenpfister/basho/internal/atomicfile"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/httpx"
)

// RecordConfig configures a recording run (SPEC Appendix E).
type RecordConfig struct {
	Out        string         // root directory; ticks go to Out/YYYYMMDD/HHMM
	Interval   time.Duration  // between ticks
	From       time.Duration  // window start, time of day JST
	Until      time.Duration  // window end, time of day JST
	Tournament domain.BashoID // zero: domain.UpcomingTournament(today)
	Once       bool           // one tick now, ignoring the window and the tournament days
	SumoAPI    string         // base URL, e.g. https://www.sumo-api.com
	Osada      string         // base URL, e.g. https://osada.us
}

// MinRecordInterval keeps the recorder polite.
const MinRecordInterval = 2 * time.Minute

// Recorder saves raw API responses during the live window of tournament days.
type Recorder struct {
	Config RecordConfig
	HTTP   httpx.Getter
	Log    io.Writer

	last map[string]*httpx.Response // last 200 per URL: validators, and the body to repeat on 304
}

// Run records until the window closes. Outside tournament days it logs why
// and returns nil, so a scheduler can start it every day.
func (r *Recorder) Run(ctx context.Context) error {
	cfg := r.Config
	if !cfg.Once && cfg.Interval < MinRecordInterval {
		return fmt.Errorf("interval %v is below the polite minimum %v", cfg.Interval, MinRecordInterval)
	}
	if !cfg.Once && cfg.From >= cfg.Until {
		return fmt.Errorf("window %v–%v: the start must be before the end", cfg.From, cfg.Until)
	}
	r.last = map[string]*httpx.Response{}
	today := domain.DateOf(time.Now())
	id := cfg.Tournament
	if id.IsZero() {
		id = domain.UpcomingTournament(today)
	}
	day := today.DaysSince(r.startDate(ctx, id)) + 1

	if cfg.Once {
		return r.tick(ctx, id, min(max(day, 1), domain.BashoDays), time.Now())
	}
	if day < 1 || day > domain.BashoDays {
		r.logf("%s: %s is not a tournament day", id, today)
		return nil
	}
	open, closeAt := today.Time().Add(cfg.From), today.Time().Add(cfg.Until)
	r.logf("%s day %d: recording %s–%s JST every %v", id, day,
		open.In(domain.JST).Format("15:04"), closeAt.In(domain.JST).Format("15:04"), cfg.Interval)
	// A tick runs if it is less than half an interval late: a start a few
	// seconds after a slot, a slow request or a reboot does not lose it, and the
	// wall clock is checked again after each wait (a sleeping machine wakes late).
	grace := cfg.Interval / 2
	for next := open; !next.After(closeAt); next = next.Add(cfg.Interval) {
		if time.Since(next) > grace {
			continue
		}
		if err := clock.Sleep(ctx, time.Until(next)); err != nil {
			return err
		}
		if late := time.Since(next); late > grace {
			r.logf("skipping the %s tick: %v late", next.In(domain.JST).Format("15:04"), late.Round(time.Second))
			continue
		}
		if err := r.tick(ctx, id, day, next); err != nil {
			return err
		}
	}
	r.logf("%s day %d: window closed", id, day)
	return nil
}

// startDate returns day 1 of id from sumo-api. It falls back to the estimate
// when the banzuke is not published yet or sumo-api cannot be reached, so one
// failed request does not cost a recording day. It decodes the response itself
// rather than using sumoapi.Client.Basho because it keeps the raw response for
// conditional requests.
func (r *Recorder) startDate(ctx context.Context, id domain.BashoID) domain.Date {
	url := r.Config.SumoAPI + "/api/basho/" + id.String()
	resp, err := r.HTTP.Get(ctx, url, httpx.Validators{})
	reason := "not published yet"
	switch {
	case err != nil:
		reason = err.Error()
	case !resp.OK():
		reason = fmt.Sprintf("HTTP %d", resp.Status)
	default:
		r.last[url] = resp // the first tick can then get a 304
		var b struct {
			StartDate time.Time `json:"startDate"` // zero object: 0001-01-01
		}
		if err := json.Unmarshal(resp.Body, &b); err != nil {
			reason = err.Error()
		} else if b.StartDate.Year() > 1 {
			return domain.DateOf(b.StartDate)
		}
	}
	r.logf("%s: no start date from sumo-api (%s), using the estimate %s", id, reason, id.EstimatedStart())
	return id.EstimatedStart()
}

// tickEntry is one request of a tick, as written to meta.json.
type tickEntry struct {
	Name         string    `json:"name"`
	URL          string    `json:"url"`
	FetchedAt    time.Time `json:"fetchedAt"`
	Status       int       `json:"status,omitempty"` // 304: the body file repeats the last 200
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"lastModified,omitempty"`
	Bytes        int       `json:"bytes,omitempty"`
	DurationMS   int64     `json:"durationMs,omitempty"`
	Error        string    `json:"error,omitempty"`
}

// tick fetches every source once and writes the bodies and meta.json. Each
// tick directory is self-contained, so recordings can be trimmed to a few
// ticks. Failed requests are recorded, not fatal; only a failing disk is.
func (r *Recorder) tick(ctx context.Context, id domain.BashoID, day int, at time.Time) error {
	dir := filepath.Join(r.Config.Out, filepath.FromSlash(at.In(domain.JST).Format("20060102/1504")))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Files go to <tick>/<source>/ under fixture.Name, the layout of testdata/,
	// so a tick can be replayed with fixture.Getter.
	sources := []struct{ dir, base, path string }{
		{"osada", r.Config.Osada, "/api/v1/torikumi.json"},
		{"osada", r.Config.Osada, "/api/v1/banzuke.json"},
		// Per tick, not once: yusho and prizes appear here on day 15.
		{"sumoapi", r.Config.SumoAPI, "/api/basho/" + id.String()},
		{"sumoapi", r.Config.SumoAPI, fmt.Sprintf("/api/basho/%s/torikumi/Makuuchi/%d", id, day)},
		{"sumoapi", r.Config.SumoAPI, fmt.Sprintf("/api/basho/%s/torikumi/Juryo/%d", id, day)},
	}
	var meta []tickEntry
	failed := 0
	for _, s := range sources {
		url := s.base + s.path
		name := filepath.Join(s.dir, fixture.Name(s.path))
		e := tickEntry{Name: filepath.ToSlash(name), URL: url, FetchedAt: time.Now().UTC()}
		var validators httpx.Validators
		if prev := r.last[url]; prev != nil {
			validators = prev.Validators
		}
		resp, err := r.HTTP.Get(ctx, url, validators)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			e.Error = err.Error()
			failed++
		default:
			e.Status, e.ETag, e.LastModified = resp.Status, resp.Validators.ETag, resp.Validators.LastModified
			e.Bytes, e.DurationMS = len(resp.Body), resp.Duration.Milliseconds()
			if resp.OK() {
				r.last[url] = resp
			} else if !resp.NotModified() {
				failed++
			}
			if body := r.last[url]; body != nil && (resp.OK() || resp.NotModified()) {
				if err := os.MkdirAll(filepath.Join(dir, s.dir), 0o755); err != nil {
					return err
				}
				if err := atomicfile.Write(filepath.Join(dir, name), body.Body); err != nil {
					return err
				}
			}
		}
		meta = append(meta, e)
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicfile.Write(filepath.Join(dir, "meta.json"), b); err != nil {
		return err
	}
	r.logf("tick %s: %d requests, %d failed", dir, len(sources), failed)
	return nil
}

func (r *Recorder) logf(format string, args ...any) {
	if r.Log != nil {
		_, _ = fmt.Fprintf(r.Log, time.Now().In(domain.JST).Format("2006-01-02 15:04:05 JST ")+format+"\n", args...)
	}
}
