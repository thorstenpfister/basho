// Command basho-data is the developer tool that builds basho's data: snapshot,
// fixtures, live recordings, rating calibration and portraits. It is never
// shipped.
package main

import (
	"os"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cli"
	"github.com/thorstenpfister/basho/internal/datatool"
)

func main() {
	os.Exit(cli.Execute(datatool.NewRoot(buildinfo.Get())))
}
