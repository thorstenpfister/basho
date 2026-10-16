package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/service"
)

// Execute runs root through fang and returns the process exit code (SPEC
// §11). fang prints errors itself.
func Execute(root *cobra.Command) int {
	return exitCode(fang.Execute(context.Background(), root, fang.WithoutVersion(), fang.WithErrorHandler(errorHandler)))
}

// errorHandler is fang's, without its case change and hard wrap: fang
// capitalises the first word, which turned a path at the start into
// /Private/Tmp/…, and wrapped paths at hyphens. A hint follows on its own
// line.
func errorHandler(w io.Writer, styles fang.Styles, err error) {
	styles.ErrorText = styles.ErrorText.UnsetTransform().UnsetWidth()
	fang.DefaultErrorHandler(w, styles, err)
}

// SetVersion makes --version print the same text as `version`. fang's own
// version output is turned off in Execute because it shortens the commit.
func SetVersion(root *cobra.Command, info buildinfo.Info, data string) {
	root.Version = info.Version
	root.SetVersionTemplate(versionText(root.Name(), info, data))
}

// NewRoot returns the basho root command with all subcommands.
func NewRoot(info buildinfo.Info, deps Deps) *cobra.Command {
	a := newApp(deps)
	root := &cobra.Command{
		Use:   "basho",
		Short: "Browse grand sumo tournaments in the terminal",
		Long: "basho shows the days and bouts of grand sumo tournaments since 1960:\n" +
			"results, standings and a fighting-game-style preview of every bout.",
		Args: cobra.NoArgs,
		// Having a run function also makes cobra reject unknown arguments.
		RunE: func(cmd *cobra.Command, _ []string) error {
			if deps.TUI == nil {
				return cmd.Help()
			}
			if err := a.setup(cmd); err != nil {
				return err
			}
			svc, err := a.svc()
			if err != nil {
				return err
			}
			if err := a.applyReveal(a.current()); err != nil {
				return err
			}
			return deps.TUI(a.cfg, svc, a.noColor, a.spoilers)
		},
	}
	data := bundleLine(deps.Bundle)
	SetVersion(root, info, data)
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), versionText(cmd.Root().Name(), info, data))
			return err
		},
	})
	root.AddCommand(a.commands()...)
	a.flags(root)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usage(err) })
	return root
}

func versionText(name string, info buildinfo.Info, data string) string {
	text := name + " " + info.Version + "\n"
	if info.Commit != "" {
		text += "  commit  " + info.Commit + "\n"
	}
	if info.Date != "" {
		text += "  date    " + info.Date + "\n"
	}
	if data != "" {
		text += "  data    " + data + "\n"
	}
	return text
}

// bundleLine is the version's data line: the snapshot's date and range,
// "2026-10-14 (196001–202609)"; empty without one.
func bundleLine(bundle func() (service.Bundle, error)) string {
	if bundle == nil {
		return ""
	}
	b, err := bundle()
	switch {
	case err != nil:
		return "unreadable: " + err.Error()
	case b.Newest.IsZero():
		return "none"
	}
	return fmt.Sprintf("%s (%s–%s)", b.Generated.Format("2006-01-02"), b.Oldest, b.Newest)
}
