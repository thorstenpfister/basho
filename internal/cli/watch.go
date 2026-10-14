package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/live"
	"github.com/thorstenpfister/basho/internal/service"
)

// watchCommand is `basho watch` (SPEC §10.4): live mode without the TUI.
// It prints one line per newly decided bout, as polls find them, and exits
// when the day is complete or its window has closed.
func (a *app) watchCommand() *cobra.Command {
	var division string
	cmd := &cobra.Command{
		Use: "watch", Short: "Print each result of today's bouts as it comes in", Args: usageArgs(cobra.NoArgs),
		RunE: a.data(func(cmd *cobra.Command, svc *service.Service, _ []string) error {
			divs := []domain.Division{domain.DivJuryo, domain.DivMakuuchi} // in the order they fight
			if division != "" {
				d, err := a.division(division)
				if err != nil {
					return err
				}
				divs = []domain.Division{d}
			}
			return a.watch(cmd, svc, divs)
		}),
	}
	cmd.Flags().StringVar(&division, "division", "", "makuuchi or juryo (default both)")
	return cmd
}

func (a *app) watch(cmd *cobra.Command, svc *service.Service, divs []domain.Division) error {
	ctx := cmd.Context()
	id, start, ok, err := svc.LiveTournament(ctx)
	if err != nil {
		return err
	}
	if !ok {
		today := domain.DateOf(a.deps.Now())
		next := domain.UpcomingTournament(today)
		if today.Compare(next.EstimatedStart()) >= 0 {
			next = next.Next()
		}
		return &exitError{code: exitNotFound, err: fmt.Errorf("no tournament day today; next basho %s, Day 1 %s", next.Name(), next.EstimatedStart())}
	}
	l := a.cfg.Live
	s := live.New(live.Config{
		Interval: time.Duration(l.Interval),
		Window: domain.LiveWindow{
			Start: time.Duration(l.WindowStart), End: time.Duration(l.WindowEnd),
			FinalDayEnd: time.Duration(l.WindowEnd) + 30*time.Minute,
		},
		Rand: rand.Float64,
	}, start)
	s.SetOn(true)
	w := watcher{out: cmd.OutOrStdout(), errs: cmd.ErrOrStderr(), json: a.json, divs: divs, seen: map[string]bool{}, g: a.glyphs()}
	for {
		now := a.deps.Now()
		switch st := s.State(now); {
		case st == live.Done && w.complete:
			return w.event(now, "complete", "Day complete")
		case st == live.Done:
			return w.event(now, "window_closed", "The live window has closed")
		case st == live.Unavailable:
			return w.event(now, "window_closed", "No tournament day today")
		}
		if !s.Due(now) {
			if err := a.sleep(ctx, s.Wake(now).Sub(now)); err != nil {
				return nil // interrupted: a normal end
			}
			continue
		}
		p, err := svc.LivePoll(ctx, id, s.DayNumber(now))
		r := live.Result{Err: err}
		if err == nil {
			if err := w.bouts(now, p); err != nil {
				return err
			}
			r = live.Result{UpdatedAt: p.UpdatedAt, Complete: p.Complete, Err: p.Err()}
			w.complete = p.Complete
			if p.Complete {
				s.Polled(now, r)
				return w.event(now, "complete", "Day complete")
			}
		}
		if r.Err != nil {
			_, _ = fmt.Fprintf(w.errs, "%s  %s\n", jstClock(now), r.Err) // and carry on
		}
		s.Polled(now, r)
	}
}

// sleep waits d or until ctx ends; tests replace it.
func (a *app) sleep(ctx context.Context, d time.Duration) error {
	if a.deps.Sleep != nil {
		return a.deps.Sleep(ctx, d)
	}
	t := time.NewTimer(max(d, 0))
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// watcher prints what polls find.
type watcher struct {
	out, errs io.Writer
	json      bool
	divs      []domain.Division
	seen      map[string]bool // division and match number of the bouts printed
	complete  bool
	g         glyphs
}

func jstClock(t time.Time) string { return t.In(domain.JST).Format("15:04 JST") }

// bouts prints the decided bouts not printed yet, in the order they fought.
func (w *watcher) bouts(now time.Time, p service.LivePoll) error {
	for _, div := range w.divs {
		day := p.Days[div].Value
		for i, r := range append(append([]service.BoutRow{}, day.Bouts...), day.Playoffs...) {
			playoff := i >= len(day.Bouts)
			k := fmt.Sprintf("%s#%d", div, r.MatchNo)
			if r.Winner == 0 || w.seen[k] {
				continue
			}
			w.seen[k] = true
			var err error
			if w.json {
				err = w.write(watchEvent{Event: "bout", Time: now, Basho: p.Basho, Day: p.Day, Division: lower(div.String()), Playoff: playoff, Bout: boutRowJSON(r)})
			} else {
				_, err = fmt.Fprintln(w.out, w.line(now, div, r, playoff))
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// line is "16:42 JST  Makuuchi #12  M3e Oho  ○ yorikiri ●  M4w Hiradoumi".
func (w *watcher) line(now time.Time, div domain.Division, r service.BoutRow, playoff bool) string {
	no := fmt.Sprintf("#%d", r.MatchNo)
	if playoff {
		no = "playoff " + no
	}
	eg := sideGlyph(w.g, r.Winner == domain.East, true, r.Fusen)
	wg := sideGlyph(w.g, r.Winner == domain.West, true, r.Fusen)
	tag := ""
	if r.Kinboshi {
		tag = "  " + w.g.star + " kinboshi"
	}
	return strings.TrimRight(fmt.Sprintf("%s  %-8s %-4s  %s %s  %s %s %s  %s %s%s", jstClock(now), div, no,
		r.East.Rank.Short(), r.East.Shikona.Romaji(), eg, r.Kimarite.Kimarite, wg, r.West.Rank.Short(), r.West.Shikona.Romaji(), tag), " ")
}

func (w *watcher) event(now time.Time, kind, text string) error {
	if w.json {
		return w.write(watchEvent{Event: kind, Time: now})
	}
	_, err := fmt.Fprintf(w.out, "%s  %s\n", jstClock(now), text)
	return err
}

// watchEvent is one line of `basho watch --json` (newline-delimited JSON).
type watchEvent struct {
	Event    string         `json:"event"` // bout, complete, window_closed
	Time     time.Time      `json:"time"`
	Basho    domain.BashoID `json:"basho,omitzero"`
	Day      int            `json:"day,omitempty"`
	Division string         `json:"division,omitempty"`
	Playoff  bool           `json:"playoff,omitempty"`
	Bout     *jsonBoutRow   `json:"bout,omitempty"`
}

func (w *watcher) write(e watchEvent) error {
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w.out, string(b))
	return err
}
