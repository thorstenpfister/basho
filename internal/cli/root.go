package cli

import (
	"context"
	"io"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/buildinfo"
)

// Execute runs root through fang and returns the process exit code (SPEC
// §11). fang prints errors itself.
func Execute(root *cobra.Command) int {
	return exitCode(fang.Execute(context.Background(), root, fang.WithoutVersion()))
}

// SetVersion makes --version print the same text as `version`. fang's own
// version output is turned off in Execute because it shortens the commit.
func SetVersion(root *cobra.Command, info buildinfo.Info) {
	root.Version = info.Version
	root.SetVersionTemplate(versionText(root.Name(), info))
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
	SetVersion(root, info)
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := io.WriteString(cmd.OutOrStdout(), versionText(cmd.Root().Name(), info))
			return err
		},
	})
	root.AddCommand(a.commands()...)
	a.flags(root)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usage(err) })
	return root
}

func versionText(name string, info buildinfo.Info) string {
	text := name + " " + info.Version + "\n"
	if info.Commit != "" {
		text += "  commit  " + info.Commit + "\n"
	}
	if info.Date != "" {
		text += "  date    " + info.Date + "\n"
	}
	return text
}
