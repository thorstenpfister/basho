// Command basho is a terminal app for browsing grand sumo tournaments.
package main

import (
	"os"

	"github.com/thorstenpfister/basho/internal/buildinfo"
	"github.com/thorstenpfister/basho/internal/cli"
	"github.com/thorstenpfister/basho/internal/config"
	"github.com/thorstenpfister/basho/internal/service"
)

func main() {
	info := buildinfo.Get()
	deps := cli.Deps{Open: func(cfg config.Config) (*service.Service, error) {
		return service.Open(cfg, info.Version, os.LookupEnv)
	}}
	os.Exit(cli.Execute(cli.NewRoot(info, deps)))
}
