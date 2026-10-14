// Command basho is a terminal app for browsing grand sumo tournaments.
package main

import (
	"context"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cli"
	"github.com/thorstenpfister/basho/internal/clock"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
	"github.com/thorstenpfister/basho/internal/spoiler"
	"github.com/thorstenpfister/basho/internal/tui"
	"github.com/thorstenpfister/basho/internal/tui/day"
	"github.com/thorstenpfister/basho/internal/tui/picker"
	"github.com/thorstenpfister/basho/internal/tui/theme"
)

func main() {
	info := buildinfo.Get()
	deps := cli.Deps{
		Open: func(cfg config.Config) (*service.Service, error) {
			return service.Open(cfg, info.Version, os.LookupEnv)
		},
		TUI: runTUI,
	}
	os.Exit(cli.Execute(cli.NewRoot(info, deps)))
}

// runTUI starts the interactive app on the current tournament's latest day,
// with the picker underneath (SPEC §8.3).
func runTUI(cfg config.Config, svc *service.Service, noColor bool) error {
	dir, err := config.StateDir(os.LookupEnv)
	if err != nil {
		return err
	}
	st, err := spoiler.Load(filepath.Join(dir, spoiler.FileName))
	if err != nil {
		return err
	}
	ctx := &tui.Context{
		Svc: svc, Cfg: cfg, Theme: theme.New(cfg.UI.Theme != "light", cfg.UI.ASCII), Clock: clock.Real{},
		TTY:      term.IsTerminal(os.Stdout.Fd()),
		Spoilers: st, Hide: cfg.Spoilers.Mode == "hide",
	}
	open := func(id domain.BashoID) tui.Screen { return day.New(ctx, id, 0) }
	today := func() []tui.Screen {
		return []tui.Screen{picker.New(ctx, open), day.New(ctx, svc.CurrentBasho(), 0)}
	}
	var opts []tea.ProgramOption
	if noColor {
		opts = append(opts, tea.WithColorProfile(colorprofile.Ascii))
	}
	svc.StartBackground(context.Background())
	return tui.Run(ctx, today(), today, opts...)
}
