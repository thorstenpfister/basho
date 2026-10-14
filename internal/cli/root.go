package cli

import (
	"context"
	"io"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/service"
)

// Execute runs root through fang and returns the process exit code. fang
// prints errors itself.
func Execute(root *cobra.Command) int {
	if err := fang.Execute(context.Background(), root, fang.WithoutVersion()); err != nil {
		return 1
	}
	return 0
}

// SetVersion makes --version print the same text as `version`. fang's own
// version output is turned off in Execute because it shortens the commit.
func SetVersion(root *cobra.Command, info buildinfo.Info) {
	root.Version = info.Version
	root.SetVersionTemplate(versionText(root.Name(), info))
}

// NewRoot returns the basho root command with all subcommands, over svc.
func NewRoot(info buildinfo.Info, svc *service.Service) *cobra.Command {
	root := &cobra.Command{
		Use:   "basho",
		Short: "Browse grand sumo tournaments in the terminal",
		Long: "basho shows the days and bouts of grand sumo tournaments since 1960:\n" +
			"results, standings and a fighting-game-style preview of every bout.",
		Args: cobra.NoArgs,
		// Starts the TUI from Phase 7. Until then it shows the help; having a
		// run function also makes cobra reject unknown arguments.
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
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
	}, newRatingsCmd(svc))
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
