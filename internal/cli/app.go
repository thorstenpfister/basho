package cli

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
)

// Deps are what the CLI needs from outside; tests replace them.
type Deps struct {
	// Open builds the service from the effective configuration;
	// service.Open in the binary.
	Open func(config.Config) (*service.Service, error)
	Env  config.Env       // nil: os.LookupEnv
	Now  func() time.Time // nil: time.Now
}

// app is one invocation's state: flags, configuration and the service.
type app struct {
	deps Deps

	// global flags (SPEC §11)
	json, noColor, ascii, debug, offline, noSnapshot bool
	source, configPath                               string

	cfg config.Config
	svc func() (*service.Service, error)
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
	f.BoolVar(&a.noSnapshot, "no-snapshot", false, "ignore the embedded snapshot (development)")
	_ = f.MarkHidden("no-snapshot")
}

// setup loads the configuration and prepares the service; the commands that
// need data run it first.
func (a *app) setup(cmd *cobra.Command) error {
	path, required := a.configPath, a.configPath != ""
	if !required {
		var err error
		if path, err = config.DefaultPath(a.deps.Env); err != nil {
			return err
		}
	}
	cfg, err := config.Load(path, required, a.deps.Env)
	if err != nil {
		return usage(err)
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
	cfg.NoSnapshot = a.noSnapshot
	if err := cfg.Validate(); err != nil {
		return usage(err)
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

// tournament parses a -t value against today (D18).
func (a *app) tournament(s string) (domain.BashoID, error) {
	id, err := domain.ParseTournament(s, domain.DateOf(a.deps.Now()))
	if err != nil {
		return domain.BashoID{}, usage(err)
	}
	return id, nil
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
