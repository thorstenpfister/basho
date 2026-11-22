package datatool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/fixture"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// Recording analysis (SPEC Appendix E, V1): when each result first appears
// in osada and in sumo-api, read through the production parsers.

// Analysis is the result of AnalyzeRecordings over a recording directory.
type Analysis struct {
	Dir      string
	Basho    domain.BashoID
	Interval time.Duration // the shortest gap between ticks: the resolution of every time below
	Days     []DayAnalysis
}

// DayAnalysis is one recorded day.
type DayAnalysis struct {
	Date                         domain.Date
	Day                          int
	Ticks                        []time.Time
	Requests, Failed             int
	OsadaRequests, NotMod        int         // osada's torikumi.json, and how many were 304
	SumoRequests, SumoValidators int         // sumo-api's, and how many came with an ETag or Last-Modified
	OsadaStamps                  []time.Time // distinct resultUpdatedAt values for this day's results
	Bouts                        []BoutTiming
	OsadaComplete                time.Time // first tick with every bout decided, playoffs included; zero: never
	SumoComplete                 time.Time
	ParseErrors                  []string
}

// BoutTiming is when one bout's result first showed in each source.
type BoutTiming struct {
	Division      domain.Division
	MatchNo       int
	Playoff       bool
	Osada, Sumo   time.Time // first tick decided; zero: never
	OsadaStamp    time.Time // osada's resultUpdatedAt at that tick: when osada published it
	Disagree      bool      // decided in both, with a different winner or kimarite at the end
	Changed       []string  // sources whose decided result changed later
	final         [2]result
	seen          [2]bool
	changedSource [2]bool
}

type result struct {
	winner   domain.Side
	kimarite domain.Kimarite
}

// Lead is how much earlier sumo-api had the result; negative when osada was
// first. ok is false unless both sources had it.
func (b BoutTiming) Lead() (time.Duration, bool) {
	if b.Osada.IsZero() || b.Sumo.IsZero() {
		return 0, false
	}
	return b.Osada.Sub(b.Sumo), true
}

const recBase = "http://recording"

// AnalyzeRecordings reads every tick under dir (<YYYYMMDD>/<HHMM>/).
func AnalyzeRecordings(ctx context.Context, dir string) (Analysis, error) {
	out := Analysis{Dir: dir}
	days, err := os.ReadDir(dir)
	if err != nil {
		return out, err
	}
	for _, d := range days {
		date, err := time.ParseInLocation("20060102", d.Name(), domain.JST)
		if !d.IsDir() || err != nil {
			continue // e.g. sumoapi/ with what the recorder does not save
		}
		da, err := analyzeDay(ctx, filepath.Join(dir, d.Name()), domain.DateOf(date), &out)
		if err != nil {
			return out, fmt.Errorf("%s: %w", d.Name(), err)
		}
		if len(da.Ticks) > 0 {
			out.Days = append(out.Days, da)
		}
	}
	if len(out.Days) == 0 {
		return out, fmt.Errorf("%s: no recorded days", dir)
	}
	return out, nil
}

func analyzeDay(ctx context.Context, dir string, date domain.Date, a *Analysis) (DayAnalysis, error) {
	da := DayAnalysis{Date: date}
	ticks, err := os.ReadDir(dir)
	if err != nil {
		return da, err
	}
	bouts := map[string]*BoutTiming{}
	var order []string
	stamps := map[time.Time]bool{}
	for _, t := range ticks {
		hm, err := time.Parse("1504", t.Name())
		if !t.IsDir() || err != nil {
			continue
		}
		at := date.Time().Add(time.Duration(hm.Hour())*time.Hour + time.Duration(hm.Minute())*time.Minute)
		if n := len(da.Ticks); n > 0 && (a.Interval == 0 || at.Sub(da.Ticks[n-1]) < a.Interval) {
			a.Interval = at.Sub(da.Ticks[n-1]) // the shortest gap: trimmed recordings skip ticks
		}
		da.Ticks = append(da.Ticks, at)
		tick := filepath.Join(dir, t.Name())
		if err := countRequests(tick, &da); err != nil {
			return da, err
		}
		// note records a source's view of a bout; it returns the bout when
		// this tick is the first to show it decided in that source.
		note := func(src int, div domain.Division, no int, playoff bool, r result) *BoutTiming {
			k := fmt.Sprintf("%s#%t#%03d", div, playoff, no)
			b := bouts[k]
			if b == nil {
				b = &BoutTiming{Division: div, MatchNo: no, Playoff: playoff}
				bouts[k], order = b, append(order, k)
			}
			if r.winner == 0 {
				return nil
			}
			if b.seen[src] && b.final[src] != r {
				b.changedSource[src] = true
			}
			b.seen[src], b.final[src] = true, r
			first := &b.Osada
			if src == 1 {
				first = &b.Sumo
			}
			if first.IsZero() {
				*first = at
				return b
			}
			return nil
		}

		// osada: today's results, if any.
		osDone := false
		cur, err := osada.New(fixture.Getter{Dir: filepath.Join(tick, "osada"), Base: recBase}, recBase).Current(ctx)
		if err != nil {
			da.ParseErrors = append(da.ParseErrors, t.Name()+" osada: "+err.Error())
		} else {
			if a.Basho.IsZero() {
				a.Basho = cur.Basho
			}
			for _, d := range cur.Days {
				if d.Date != date {
					continue
				}
				da.Day = d.Day
				total, decided := 0, 0
				for div, bs := range d.Bouts {
					for _, b := range bs {
						total++
						if b.Winner != 0 {
							decided++
						}
						if fresh := note(0, div, b.MatchNo, b.IsPlayoff, result{b.Winner, b.Kimarite}); fresh != nil {
							fresh.OsadaStamp = cur.ResultUpdatedAt
						}
					}
				}
				if decided > 0 && !cur.ResultUpdatedAt.IsZero() {
					stamps[cur.ResultUpdatedAt] = true
				}
				osDone = total > 0 && decided == total
			}
		}
		if osDone && da.OsadaComplete.IsZero() {
			da.OsadaComplete = at
		}

		// sumo-api: the day's two bout lists.
		if da.Day == 0 || a.Basho.IsZero() {
			continue // osada names the tournament and the day; without them there is nothing to ask
		}
		api := sumoapi.New(fixture.Getter{Dir: filepath.Join(tick, "sumoapi"), Base: recBase}, recBase)
		total, decided := 0, 0
		for _, div := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
			bs, err := api.Torikumi(ctx, a.Basho, div, da.Day)
			if err != nil {
				da.ParseErrors = append(da.ParseErrors, fmt.Sprintf("%s sumo-api %s: %v", t.Name(), div, err))
				total = -1
				break
			}
			for _, b := range bs {
				total++
				if b.Winner != 0 {
					decided++
				}
				note(1, div, b.MatchNo, b.IsPlayoff, result{b.Winner, b.Kimarite})
			}
		}
		if total > 0 && decided == total && da.SumoComplete.IsZero() {
			da.SumoComplete = at
		}
	}
	for _, k := range order {
		b := bouts[k]
		b.Disagree = b.seen[0] && b.seen[1] && b.final[0] != b.final[1]
		for i, name := range []string{"osada", "sumo-api"} {
			if b.changedSource[i] {
				b.Changed = append(b.Changed, name)
			}
		}
		da.Bouts = append(da.Bouts, *b)
	}
	sort.SliceStable(da.Bouts, func(i, j int) bool {
		x, y := da.Bouts[i], da.Bouts[j]
		if x.Division != y.Division {
			return x.Division < y.Division
		}
		if x.Playoff != y.Playoff {
			return !x.Playoff
		}
		return x.MatchNo < y.MatchNo
	})
	for s := range stamps {
		da.OsadaStamps = append(da.OsadaStamps, s)
	}
	slices.SortFunc(da.OsadaStamps, func(x, y time.Time) int { return x.Compare(y) })
	return da, nil
}

// countRequests adds a tick's meta.json to the day's request counts.
func countRequests(tick string, da *DayAnalysis) error {
	raw, err := os.ReadFile(filepath.Join(tick, "meta.json"))
	if err != nil {
		return err
	}
	var meta []tickEntry
	if err := json.Unmarshal(raw, &meta); err != nil {
		return fmt.Errorf("%s: %w", tick, err)
	}
	for _, e := range meta {
		da.Requests++
		if e.Error != "" || (e.Status != 200 && e.Status != 304) {
			da.Failed++
		}
		if strings.HasPrefix(e.Name, "sumoapi/") {
			da.SumoRequests++
			if e.ETag != "" || e.LastModified != "" {
				da.SumoValidators++
			}
		}
		if e.Name == "osada/torikumi.json" {
			da.OsadaRequests++
			if e.Status == 304 {
				da.NotMod++
			}
		}
	}
	return nil
}

// leadStats counts bouts by lead bucket: sumo-api first, same tick, osada first.
type leadStats struct {
	sumo, same, osada, n int
	leads                []time.Duration
}

func (s *leadStats) add(b BoutTiming) {
	l, ok := b.Lead()
	if !ok || b.Playoff {
		return
	}
	s.n++
	s.leads = append(s.leads, l)
	switch {
	case l > 0:
		s.sumo++
	case l < 0:
		s.osada++
	default:
		s.same++
	}
}

func (s leadStats) median() time.Duration {
	if len(s.leads) == 0 {
		return 0
	}
	l := slices.Clone(s.leads)
	slices.Sort(l)
	return l[len(l)/2]
}

func (s leadStats) mean() time.Duration {
	if len(s.leads) == 0 {
		return 0
	}
	var sum time.Duration
	for _, l := range s.leads {
		sum += l
	}
	return sum / time.Duration(len(s.leads))
}

func pct(n, of int) string {
	if of == 0 {
		return "–"
	}
	return fmt.Sprintf("%.0f%%", 100*float64(n)/float64(of))
}

func hhmm(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.In(domain.JST).Format("15:04")
}

func mins(d time.Duration) string {
	return fmt.Sprintf("%+.0f min", d.Minutes())
}

// WriteReport writes docs/v1-latency.md.
func (a Analysis) WriteReport(w io.Writer) error {
	var sb strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&sb, format, args...) }
	var all, juryo, maku leadStats
	ticks, reqs, failed, osReq, notMod, sumoReq, sumoVal := 0, 0, 0, 0, 0, 0, 0
	for _, d := range a.Days {
		ticks += len(d.Ticks)
		reqs, failed, osReq, notMod = reqs+d.Requests, failed+d.Failed, osReq+d.OsadaRequests, notMod+d.NotMod
		sumoReq, sumoVal = sumoReq+d.SumoRequests, sumoVal+d.SumoValidators
		for _, b := range d.Bouts {
			all.add(b)
			if b.Division == domain.DivJuryo {
				juryo.add(b)
			} else {
				maku.add(b)
			}
		}
	}
	p("# V1: osada.us and sumo-api.com latency\n\n")
	p("Generated by `basho-data record analyze` (SPEC Appendix E) from the %s recordings: ", a.Basho)
	p("%d days, %d ticks every %s, %d requests, %d failed. ", len(a.Days), ticks, a.Interval, reqs, failed)
	p("A result counts as shown at the first tick that has it decided, so every time is accurate to one tick (%s). ", a.Interval)
	p("A lead is how much earlier sumo-api had a result than osada; negative when osada was first. Playoff bouts are left out of the leads.\n\n")
	p("**V1:** of %d results, sumo-api had %s first, %s came in the same tick, osada had %s first. ", all.n, pct(all.sumo, all.n), pct(all.same, all.n), pct(all.osada, all.n))
	p("Median lead %s, mean %s.\n\n", mins(all.median()), mins(all.mean()))

	p("## By division\n\n| Division | Results | sumo-api first | Same tick | osada first | Median lead | Mean lead |\n|---|---|---|---|---|---|---|\n")
	for _, r := range []struct {
		name string
		s    leadStats
	}{{"Juryo", juryo}, {"Makuuchi", maku}, {"Both", all}} {
		p("| %s | %d | %s | %s | %s | %s | %s |\n", r.name, r.s.n, pct(r.s.sumo, r.s.n), pct(r.s.same, r.s.n), pct(r.s.osada, r.s.n), mins(r.s.median()), mins(r.s.mean()))
	}

	p("\n## By day\n\n")
	p("First result: the first tick with any of the day's results. Complete: the first tick with every listed bout decided (osada: listed playoffs included). ")
	p("osada's first publication is its `resultUpdatedAt` for that tick.\n\n")
	p("| Day | Date | Ticks | Failed | osada first published | osada first result | sumo-api first result | osada complete | sumo-api complete | sumo-api / same / osada first | Playoffs (osada) |\n")
	p("|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, d := range a.Days {
		var s leadStats
		var osFirst, sumoFirst time.Time
		playoffs := 0
		for _, b := range d.Bouts {
			s.add(b)
			if b.Playoff {
				playoffs++
			}
			if !b.Osada.IsZero() && (osFirst.IsZero() || b.Osada.Before(osFirst)) {
				osFirst = b.Osada
			}
			if !b.Sumo.IsZero() && (sumoFirst.IsZero() || b.Sumo.Before(sumoFirst)) {
				sumoFirst = b.Sumo
			}
		}
		firstStamp := time.Time{}
		if len(d.OsadaStamps) > 0 {
			firstStamp = d.OsadaStamps[0]
		}
		p("| %d | %s | %d | %d | %s | %s | %s | %s | %s | %d / %d / %d | %d |\n", d.Day, d.Date, len(d.Ticks), d.Failed,
			hhmm(firstStamp), hhmm(osFirst), hhmm(sumoFirst), hhmm(d.OsadaComplete), hhmm(d.SumoComplete), s.sumo, s.same, s.osada, playoffs)
	}

	p("\n## osada's publication\n\n")
	p("osada publishes in batches; each batch changes `resultUpdatedAt`. Gaps are between consecutive batches seen in the recordings, so batches closer together than one tick count as one.\n\n")
	var gaps []time.Duration
	stamps := 0
	for _, d := range a.Days {
		stamps += len(d.OsadaStamps)
		for i := 1; i < len(d.OsadaStamps); i++ {
			gaps = append(gaps, d.OsadaStamps[i].Sub(d.OsadaStamps[i-1]))
		}
	}
	slices.Sort(gaps)
	p("- %d batches seen, %.1f a day\n", stamps, float64(stamps)/float64(len(a.Days)))
	if len(gaps) > 0 {
		p("- gap between batches: median %s, shortest %s, longest %s\n", gaps[len(gaps)/2].Round(time.Minute), gaps[0].Round(time.Minute), gaps[len(gaps)-1].Round(time.Minute))
	}
	p("- `torikumi.json`: %d requests, %d of them 304 (%s)\n", osReq, notMod, pct(notMod, osReq))
	p("\nsumo-api: %d requests, %d of them with an ETag or Last-Modified to revalidate against", sumoReq, sumoVal)
	if sumoVal == 0 {
		p(", so every request is a full answer")
	}
	p(".\n")

	p("\n## Disagreements and corrections\n\n")
	var lines []string
	for _, d := range a.Days {
		for _, b := range d.Bouts {
			kind := "bout"
			if b.Playoff {
				kind = "playoff"
			}
			id := fmt.Sprintf("Day %d %s %s %d", d.Day, b.Division, kind, b.MatchNo)
			if b.Disagree {
				lines = append(lines, fmt.Sprintf("- %s: the sources disagree on the result", id))
			}
			if len(b.Changed) > 0 {
				lines = append(lines, fmt.Sprintf("- %s: a decided result changed later in %s", id, strings.Join(b.Changed, " and ")))
			}
			switch {
			case b.Playoff && b.Sumo.IsZero() && !b.Osada.IsZero():
				lines = append(lines, fmt.Sprintf("- %s: only osada lists it (V2: sumo-api has no playoff bouts)", id))
			case !b.Playoff && b.Osada.IsZero() != b.Sumo.IsZero():
				lines = append(lines, fmt.Sprintf("- %s: decided in one source only (osada %s, sumo-api %s)", id, hhmm(b.Osada), hhmm(b.Sumo)))
			}
		}
		for _, e := range d.ParseErrors {
			lines = append(lines, fmt.Sprintf("- Day %d, tick %s", d.Day, e))
		}
	}
	if len(lines) == 0 {
		p("None.\n")
	} else {
		p("%s\n", strings.Join(lines, "\n"))
	}
	_, err := io.WriteString(w, sb.String())
	return err
}
