// Command basho is a terminal app for browsing grand sumo tournaments.
package main

import (
	"os"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cli"
)

func main() {
	os.Exit(cli.Execute(cli.NewRoot(buildinfo.Get())))
}
