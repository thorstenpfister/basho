// Command basho is a terminal app for browsing grand sumo tournaments.
package main

import (
	"os"

	"github.com/thorstenpfister/basho/data"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cli"
	"github.com/thorstenpfister/basho/internal/service"
)

func main() {
	svc := service.New(service.Options{Snapshot: data.Snapshot, Params: data.RatingParams})
	os.Exit(cli.Execute(cli.NewRoot(buildinfo.Get(), svc)))
}
