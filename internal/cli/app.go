package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/spoiler"
)

// Deps are what the CLI needs from outside; tests replace them.
type Deps struct {
	// Open builds the service from the effective configuration;
	// service.Open in the binary.
	Open func(config.Config) (*service.Service, error)
	Env  config.Env       // nil: os.LookupEnv
	Now  func() time.Time // nil: time.Now
	// TUI runs the interactive app (basho without a command); nil shows the
	// help instead.
	TUI func(cfg config.Config, svc *service.Service, noColor bool, st *spoiler.State) error
	// Sleep waits between `basho watch` polls; nil: a timer.
	Sleep func(ctx context.Context, d time.Duration) error
}

// app is one invocation's state: flags, configuration and the service.
type app struct {
	deps Deps

	// global flags (SPEC §11)
	json, noColor, ascii, debug, offline, noSnapshot bool
	source, configPath                               string
	noSpoilers                                       bool
	revealTo                                         string

	cfg      config.Config
	svc      func() (*service.Service, error)
	spoilers *spoiler.State // the reveal horizons, loaded in setup
	reveal   *spoiler.Horizon
	revealed bool // --reveal-to has been applied
}

func newApp(deps Deps) *app {
	if deps.Env == nil {
		deps.Env = os.LookupEnv
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &app{deps: deps}
}

// flags registers the global flags on root.
func (a *app) flags(root *cobra.Command) {
	f := root.PersistentFlags()
	f.BoolVar(&a.json, "json", false, "machine-readable output")
	f.StringVar(&a.source, "source", "", "force a data source: auto, sumoapi, osada")
	f.BoolVar(&a.noColor, "no-color", false, "plain output without colour")
	f.BoolVar(&a.ascii, "ascii", false, "ASCII instead of symbols")
	f.StringVar(&a.configPath, "config", "", "alternative config file")
	f.BoolVar(&a.debug, "debug", false, "write a debug log")
	f.BoolVar(&a.offline, "offline", false, "snapshot and cache only, never the network")
	f.BoolVar(&a.noSpoilers, "no-spoilers", false, "spoiler-free: stop at the stored reveal horizon")
	f.StringVar(&a.revealTo, "reveal-to", "", "set the tournament's reveal horizon: DAY or DAY:BOUT (Makuuchi); implies --no-spoilers")
	f.BoolVar(&a.noSnapshot, "no-snapshot", false, "ignore the embedded snapshot (development)")
	_ = f.MarkHidden("no-snapshot")
}

// configFile is the config file in use: --config, or the default path.
func (a *app) configFile() (string, error) {
	if a.configPath != "" {
		return a.configPath, nil
	}
	return config.DefaultPath(a.deps.Env)
}

// configHint says where settings come from after a config error.
func configHint(err error, path string) error {
	return fmt.Errorf("%w\nSettings come from %s and BASHO_* variables", err, path)
}

// setup loads the configuration and prepares the service; the commands that
// need data run it first.
func (a *app) setup(cmd *cobra.Command) error {
	path, err := a.configFile()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path, a.configPath != "", a.deps.Env)
	if err != nil {
		return usage(configHint(err, path))
	}
	f := cmd.Flags()
	if f.Changed("source") {
		cfg.Source = a.source
	}
	if f.Changed("offline") {
		cfg.Offline = a.offline
	}
	if f.Changed("debug") {
		cfg.Debug = a.debug
	}
	if f.Changed("ascii") {
		cfg.UI.ASCII = a.ascii
	}
	// NO_COLOR counts with any value (no-color.org); colorprofile only
	// takes the ones strconv.ParseBool reads, so NO_COLOR=yes slipped by.
	if v, _ := a.deps.Env("NO_COLOR"); v != "" {
		a.noColor = true
	}
	cfg.NoSnapshot = a.noSnapshot
	if err := cfg.Validate(); err != nil {
		return usage(configHint(fmt.Errorf("config: %w", err), path))
	}
	if a.revealTo != "" {
		h, err := parseHorizon(a.revealTo)
		if err != nil {
			return usage(err)
		}
		a.reveal, a.noSpoilers = &h, true
	}
	if a.noSpoilers {
		cfg.Spoilers.Mode = "hide"
	}
	dir, err := config.StateDir(a.deps.Env)
	if err != nil {
		return err
	}
	if a.spoilers, err = spoiler.Load(filepath.Join(dir, spoiler.FileName)); err != nil {
		return fmt.Errorf("spoiler state %w\nIt only keeps how far each tournament is revealed; delete it to start afresh", err)
	}
	a.cfg = cfg
	a.svc = sync.OnceValues(func() (*service.Service, error) {
		if a.deps.Open == nil {
			return nil, fmt.Errorf("no data layers")
		}
		return a.deps.Open(cfg)
	})
	return nil
}

// tournament parses a -t value against today (D18), and applies
// --reveal-to to it.
func (a *app) tournament(s string) (domain.BashoID, error) {
	id, err := domain.ParseTournament(s, domain.DateOf(a.deps.Now()))
	if err != nil {
		return domain.BashoID{}, usage(err)
	}
	return id, a.applyReveal(id)
}

// applyReveal sets the horizon of --reveal-to on tournament id, once.
func (a *app) applyReveal(id domain.BashoID) error {
	if a.reveal == nil || a.revealed {
		return nil
	}
	a.revealed = true
	return a.spoilers.Set(id, *a.reveal)
}

// current is the current tournament, for commands without -t.
func (a *app) current() domain.BashoID {
	id, _ := domain.ParseTournament("current", domain.DateOf(a.deps.Now()))
	return id
}

// dataCtx returns the context of the service calls: spoiler-free when
// spoilers.mode is hide, --no-spoilers or --reveal-to (SPEC §9).
func (a *app) dataCtx(cmd *cobra.Command) context.Context {
	if a.cfg.Spoilers.Mode == "hide" && a.spoilers != nil {
		return service.Hiding(cmd.Context(), a.spoilers)
	}
	return cmd.Context()
}

// parseHorizon reads --reveal-to: "13" (the end of day 13), "13:19"
// (through Makuuchi bout 19), "j13:5" (Juryo) or "0" (nothing).
func parseHorizon(s string) (spoiler.Horizon, error) {
	div := domain.DivMakuuchi
	if rest, ok := strings.CutPrefix(strings.ToLower(s), "j"); ok {
		div, s = domain.DivJuryo, rest
	}
	dayText, matchText, hasMatch := strings.Cut(s, ":")
	day, err := strconv.Atoi(dayText)
	if err != nil || day < 0 || day > domain.BashoDays {
		return spoiler.Horizon{}, fmt.Errorf("--reveal-to %q: want DAY or DAY:BOUT, e.g. 13 or 13:19", s)
	}
	h := spoiler.Horizon{Day: day}
	if hasMatch {
		match, err := strconv.Atoi(matchText)
		if err != nil || match < 1 || day == 0 {
			return spoiler.Horizon{}, fmt.Errorf("--reveal-to %q: want DAY:BOUT with a bout number from 1", s)
		}
		h.Division, h.MatchNo = div, match
	}
	return h, nil
}

// division parses --division, defaulting to ui.division.
func (a *app) division(s string) (domain.Division, error) {
	if s == "" {
		s = a.cfg.UI.Division
	}
	for _, d := range []domain.Division{domain.DivMakuuchi, domain.DivJuryo} {
		if strings.EqualFold(s, d.String()) {
			return d, nil
		}
	}
	return 0, usage(fmt.Errorf("division %q: want makuuchi or juryo", s))
}

// out returns stdout with colours downsampled for the terminal, or removed
// when piped or with --no-color (SPEC §11).
func (a *app) out(cmd *cobra.Command) *colorprofile.Writer {
	w := colorprofile.NewWriter(cmd.OutOrStdout(), os.Environ())
	if a.noColor {
		w.Profile = colorprofile.NoTTY
	}
	return w
}

// glyphs are the symbols of the text output; --ascii or ui.ascii swaps them.
type glyphs struct {
	win, loss, fusenWin, fusenLoss, absent, pending string
	star, up, down, none, divider, musubi, kyujo    string
}

var (
	unicodeGlyphs = glyphs{"○", "●", "□", "■", "–", "·", "★", "▲", "▼", "–", "│", "結び", "休"}
	asciiGlyphs   = glyphs{"o", "x", "O", "X", "-", ".", "*", "+", "-", "-", "|", "musubi", "kyujo"}
)

func (a *app) glyphs() glyphs {
	if a.cfg.UI.ASCII {
		return asciiGlyphs
	}
	return unicodeGlyphs
}
