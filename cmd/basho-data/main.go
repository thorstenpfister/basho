// Command basho-data is the developer tool that builds basho's data: snapshot,
// fixtures, live recordings, rating calibration and portraits. It is never
// shipped.
package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cli"
)

func main() {
	root := &cobra.Command{
		Use:   "basho-data",
		Short: "Build basho's data: snapshot, fixtures, live recordings, calibration",
		Args:  cobra.NoArgs,
	}
	cli.SetVersion(root, buildinfo.Get())
	os.Exit(cli.Execute(root))
}
