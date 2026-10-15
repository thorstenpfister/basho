package datatool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/atomicfile"
	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cache"
	"github.com/thorstenpfister/basho/internal/cli"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/httpx"
	"github.com/thorstenpfister/basho/internal/source/osada"
	"github.com/thorstenpfister/basho/internal/source/sumoapi"
)

// NewRoot returns the basho-data root command with all subcommands.
func NewRoot(info buildinfo.Info) *cobra.Command {
	root := &cobra.Command{
		Use:   "basho-data",
		Short: "Build basho's data: snapshot, fixtures, live recordings, calibration, portraits",
		Args:  cobra.NoArgs,
	}
	cli.SetVersion(root, info)
	root.AddCommand(newRecordCmd(info), newFixturesCmd(info), newSnapshotCmd(info), newRatingCmd(info), newPortraitsCmd(info))
	return root
}

func newFixturesCmd(info buildinfo.Info) *cobra.Command {
	var out string
	record := &cobra.Command{
		Use:   "record",
		Short: fmt.Sprintf("Record the test fixtures (%d requests)", FixtureRequests()),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return RecordFixtures(cmd.Context(), newRecordClient(info), out, cmd.ErrOrStderr())
		},
	}
	record.Flags().StringVar(&out, "out", "testdata", "output directory")
	fixtures := &cobra.Command{Use: "fixtures", Short: "Test fixtures recorded from the APIs"}
	fixtures.AddCommand(record)
	return fixtures
}

func newRecordCmd(info buildinfo.Info) *cobra.Command {
	cfg := RecordConfig{SumoAPI: sumoapi.DefaultBaseURL, Osada: osada.DefaultBaseURL}
	var from, until, tournament string
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record raw API responses during the live window of tournament days",
		Long: "record saves the osada.us and sumo-api.com responses every --interval between\n" +
			"--from and --until (JST) on tournament days, as test fixtures (SPEC Appendix E).\n" +
			"On other days it exits at once, so it can be started every day by launchd.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			if cfg.From, err = parseClock(from); err != nil {
				return err
			}
			if cfg.Until, err = parseClock(until); err != nil {
				return err
			}
			if tournament != "" {
				if cfg.Tournament, err = domain.ParseBashoID(tournament); err != nil {
					return err
				}
			}
			// launchd stops jobs with SIGTERM: end between files, not mid-write.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			r := Recorder{Config: cfg, HTTP: newRecordClient(info), Log: cmd.ErrOrStderr()}
			if err := r.Run(ctx); err != nil && ctx.Err() == nil {
				return err
			}
			return nil // a stop signal is a normal end: exit 0, so launchd does not restart it
		},
	}
	f := cmd.Flags()
	f.StringVar(&cfg.Out, "out", "testdata/live", "output directory")
	f.DurationVar(&cfg.Interval, "interval", 10*time.Minute, "time between ticks (at least 2m)")
	f.StringVar(&from, "from", "14:00", "window start, JST")
	f.StringVar(&until, "until", "19:15", "window end, JST")
	f.StringVar(&tournament, "tournament", "", "tournament as YYYYMM (default: this month's, or the upcoming one in even months)")
	f.BoolVar(&cfg.Once, "once", false, "record one tick now, ignoring the window and tournament days (installation test)")
	return cmd
}

func newSnapshotCmd(info buildinfo.Info) *cobra.Command {
	cfg := SnapshotConfig{Generator: info.Version}
	var from, to, workCache string
	update := &cobra.Command{
		Use:   "update",
		Short: "Add finished tournaments missing from the snapshot (about 33 requests each)",
		Long: "update fetches every finished tournament between --from and --to that the manifest\n" +
			"does not list yet, then rewrites the wrestler index. Responses go through a work\n" +
			"cache, so an interrupted run resumes without fetching anything twice (SPEC §4.3).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var err error
			if cfg.From, err = optionalBashoID(from); err != nil {
				return err
			}
			if cfg.To, err = optionalBashoID(to); err != nil {
				return err
			}
			if cfg.Interval < time.Second {
				return fmt.Errorf("the interval must be at least 1s for sumo-api, not %s", cfg.Interval)
			}
			cfg.Today = domain.DateOf(time.Now())
			c, err := cache.Open(workCache, 4<<30)
			if err != nil {
				return err
			}
			g := &cache.Getter{Cache: c, Next: newClient(info, cfg.Interval, false), Policy: sumoapi.Freshness{}.Policy}
			_, err = UpdateSnapshot(cmd.Context(), sumoapi.New(g, sumoapi.DefaultBaseURL), cfg, cmd.ErrOrStderr())
			return err
		},
	}
	f := update.Flags()
	f.StringVar(&cfg.Dir, "dir", "data/snapshot", "snapshot directory")
	f.StringVar(&workCache, "cache", ".cache/basho-data", "work cache directory")
	f.StringVar(&from, "from", "", "first tournament (YYYYMM, default 196001)")
	f.StringVar(&to, "to", "", "last tournament (YYYYMM, default the newest finished)")
	f.DurationVar(&cfg.Interval, "interval", time.Second, "time between requests; at least 1s")
	f.BoolVar(&cfg.DryRun, "dry-run", false, "print the plan and an estimated duration without fetching")
	var dir string
	verify := &cobra.Command{
		Use:   "verify",
		Short: "Check checksums, schema, and bouts against banzuke records",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			problems, err := VerifySnapshot(os.DirFS(dir))
			if err != nil {
				return err
			}
			for _, p := range problems {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), p)
			}
			if len(problems) > 0 {
				return fmt.Errorf("%d problems in %s", len(problems), dir)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", dir)
			return err
		},
	}
	stats := &cobra.Command{
		Use:   "stats",
		Short: "Sizes and coverage of the snapshot",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return SnapshotStats(os.DirFS(dir), cmd.OutOrStdout())
		},
	}
	for _, c := range []*cobra.Command{verify, stats} {
		c.Flags().StringVar(&dir, "dir", "data/snapshot", "snapshot directory")
	}
	snap := &cobra.Command{Use: "snapshot", Short: "The embedded snapshot of finished tournaments"}
	snap.AddCommand(update, verify, stats)
	return snap
}

func optionalBashoID(s string) (domain.BashoID, error) {
	if s == "" {
		return domain.BashoID{}, nil
	}
	return domain.ParseBashoID(s)
}

func newRecordClient(info buildinfo.Info) *httpx.Client { return newClient(info, time.Second, true) }

// newClient returns the polite client for sumo-api at interval per request,
// and osada.us at its fixed 20 s if wanted (SPEC §7.1).
func newClient(info buildinfo.Info, interval time.Duration, withOsada bool) *httpx.Client {
	hosts := map[string]httpx.HostPolicy{host(sumoapi.DefaultBaseURL): {Interval: interval}}
	if withOsada {
		hosts[host(osada.DefaultBaseURL)] = httpx.HostPolicy{Interval: 20 * time.Second}
	}
	return httpx.New(httpx.Options{UserAgent: httpx.UserAgent(info.Version), Hosts: hosts})
}

func host(base string) string {
	u, err := url.Parse(base)
	if err != nil {
		panic(err) // the bases are constants
	}
	return u.Host
}

// parseClock parses "HH:MM" into a time of day.
func parseClock(s string) (time.Duration, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("time of day %q: want HH:MM", s)
	}
	return time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute, nil
}

func newRatingCmd(info buildinfo.Info) *cobra.Command {
	var dir, params, report string
	calibrate := &cobra.Command{
		Use:   "calibrate",
		Short: "Search the rating parameters and write params.json and the report (no network)",
		Long: "calibrate runs the Elo model over the snapshot for every parameter set of the grid\n" +
			"(SPEC Appendix F), chooses the best on the training years and judges it against the\n" +
			"rank and coin baselines on the evaluation years (V9).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := LoadHistory(os.DirFS(dir))
			if err != nil {
				return err
			}
			start := time.Now()
			c := Calibrate(h)
			f, err := json.MarshalIndent(c.ParamsFile(info.Version), "", "  ")
			if err != nil {
				return err
			}
			if err := atomicfile.Write(params, append(f, '\n')); err != nil {
				return err
			}
			var md bytes.Buffer
			if err := c.WriteReport(&md); err != nil {
				return err
			}
			if err := atomicfile.Write(report, md.Bytes()); err != nil {
				return err
			}
			best := c.Best()
			_, err = fmt.Fprintf(cmd.ErrOrStderr(), "%d parameter sets in %s; chosen %+v: eval log loss %.4f, rank baseline %.4f, shown %v\n",
				len(c.Results), time.Since(start).Round(time.Millisecond), best.Params, best.Eval.LogLoss, c.Baselines.Rank.Eval.LogLoss, c.Shown())
			return err
		},
	}
	f := calibrate.Flags()
	f.StringVar(&dir, "snapshot", "data/snapshot", "snapshot directory")
	f.StringVar(&params, "params", "data/rating/params.json", "parameters file to write")
	f.StringVar(&report, "report", "docs/rating.md", "report to write")
	rating := &cobra.Command{Use: "rating", Short: "The rating model (SPEC §6.8)"}
	rating.AddCommand(calibrate)
	return rating
}
