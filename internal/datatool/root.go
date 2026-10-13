package datatool

import (
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/buildinfo"
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
		Short: "Build basho's data: snapshot, fixtures, live recordings, calibration",
		Args:  cobra.NoArgs,
	}
	cli.SetVersion(root, info)
	root.AddCommand(newRecordCmd(info), newFixturesCmd(info))
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

func newRecordClient(info buildinfo.Info) *httpx.Client {
	return httpx.New(httpx.Options{
		UserAgent: httpx.UserAgent(info.Version),
		Hosts: map[string]httpx.HostPolicy{ // SPEC §7.1
			host(sumoapi.DefaultBaseURL): {Interval: time.Second},
			host(osada.DefaultBaseURL):   {Interval: 20 * time.Second},
		},
	})
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
