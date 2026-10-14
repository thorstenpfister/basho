package cli

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
)

// data wraps a command that reads data: configuration and service first.
func (a *app) data(run func(cmd *cobra.Command, svc *service.Service, args []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		if err := a.setup(cmd); err != nil {
			return err
		}
		svc, err := a.svc()
		if err != nil {
			return err
		}
		return a.explain(run(cmd, svc, args))
	}
}

// explain leads a failure to reach the data with a sentence; fang prints the
// error with its first word capitalised.
func (a *app) explain(err error) error {
	if !service.Unavailable(err) {
		return err
	}
	if a.cfg.Offline {
		return fmt.Errorf("not in the snapshot or the cache, and --offline is set: %w", err)
	}
	return fmt.Errorf("every source failed and nothing is cached: %w", err)
}

// emit writes a result: the JSON envelope with --json, otherwise text.
func (a *app) emit(cmd *cobra.Command, kind string, p service.Provenance, text func() string, data func() any) error {
	if a.json {
		return writeJSON(cmd.OutOrStdout(), kind, p, data())
	}
	_, err := a.out(cmd).Write([]byte(text()))
	return err
}

func (a *app) commands() []*cobra.Command {
	var tournament, division string
	tFlag := func(c *cobra.Command) {
		c.Flags().StringVarP(&tournament, "tournament", "t", "current", "tournament: 202609, 2026-09, aki2026, current, prev, next")
	}
	dFlag := func(c *cobra.Command) {
		c.Flags().StringVar(&division, "division", "", "makuuchi or juryo (default ui.division)")
	}

	var from, to int
	list := &cobra.Command{
		Use: "list", Short: "Tournaments, newest first", Args: usageArgs(cobra.NoArgs),
		RunE: a.data(func(cmd *cobra.Command, svc *service.Service, _ []string) error {
			res, err := svc.List(a.dataCtx(cmd), from, to)
			if err != nil {
				return err
			}
			return a.emit(cmd, "list", res.Prov, func() string { return renderList(res.Value, res.Prov) }, func() any { return listJSON(res.Value) })
		}),
	}
	list.Flags().IntVar(&from, "from", 0, "first year")
	list.Flags().IntVar(&to, "to", 0, "last year")

	info := &cobra.Command{
		Use: "info [YYYYMM]", Short: "A tournament's metadata, yusho and special prizes", Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: a.data(func(cmd *cobra.Command, svc *service.Service, args []string) error {
			arg := "current"
			if len(args) == 1 {
				arg = args[0]
			}
			id, err := a.tournament(arg)
			if err != nil {
				return err
			}
			res, err := svc.Info(a.dataCtx(cmd), id)
			if err != nil {
				return err
			}
			return a.emit(cmd, "info", res.Prov, func() string { return renderInfo(res.Value, res.Prov) }, func() any { return infoJSON(res.Value) })
		}),
	}

	day := &cobra.Command{
		Use: "day [N]", Short: "A day's bouts", Args: usageArgs(cobra.MaximumNArgs(1)),
		RunE: a.data(func(cmd *cobra.Command, svc *service.Service, args []string) error {
			id, err := a.tournament(tournament)
			if err != nil {
				return err
			}
			div, err := a.division(division)
			if err != nil {
				return err
			}
			n, err := a.dayArg(cmd, svc, args, id)
			if err != nil {
				return err
			}
			res, err := svc.Day(a.dataCtx(cmd), id, div, n)
			if err != nil {
				return err
			}
			return a.emit(cmd, "day", res.Prov, func() string { return renderDay(a.glyphs(), res.Value, res.Prov) }, func() any { return dayJSON(res.Value) })
		}),
	}
	tFlag(day)
	dFlag(day)

	bout := &cobra.Command{
		Use: "bout <day> <matchNo>", Short: "The tale of the tape of one bout", Args: usageArgs(cobra.ExactArgs(2)),
		RunE: a.data(func(cmd *cobra.Command, svc *service.Service, args []string) error {
			id, err := a.tournament(tournament)
			if err != nil {
				return err
			}
			div, err := a.division(division)
			if err != nil {
				return err
			}
			n, err := number("day", args[0])
			if err != nil {
				return err
			}
			match, err := number("match number", args[1])
			if err != nil {
				return err
			}
			res, err := svc.Bout(a.dataCtx(cmd), id, div, n, match)
			if err != nil {
				return err
			}
			return a.emit(cmd, "bout", res.Prov, func() string { return renderTale(a.glyphs(), res.Value, res.Prov) }, func() any { return taleJSON(res.Value) })
		}),
	}
	tFlag(bout)
	dFlag(bout)

	var standingsDay int
	var grid bool
	standings := &cobra.Command{
		Use: "standings", Short: "The standings after a day", Args: usageArgs(cobra.NoArgs),
		RunE: a.data(func(cmd *cobra.Command, svc *service.Service, _ []string) error {
			id, err := a.tournament(tournament)
			if err != nil {
				return err
			}
			div, err := a.division(division)
			if err != nil {
				return err
			}
			var args []string
			if cmd.Flags().Changed("day") {
				args = []string{strconv.Itoa(standingsDay)}
			}
			n, err := a.dayArg(cmd, svc, args, id)
			if err != nil {
				return err
			}
			res, err := svc.Standings(a.dataCtx(cmd), id, div, n)
			if err != nil {
				return err
			}
			return a.emit(cmd, "standings", res.Prov, func() string { return renderStandings(a.glyphs(), res.Value, grid, res.Prov) }, func() any { return standingsJSON(res.Value) })
		}),
	}
	tFlag(standings)
	dFlag(standings)
	standings.Flags().IntVar(&standingsDay, "day", 0, "after day N (default: today, or the last day)")
	standings.Flags().BoolVar(&grid, "grid", false, "the hoshitori grid in banzuke order")

	var top int
	ratings := &cobra.Command{
		Use:   "ratings",
		Short: "Top-rated wrestlers as of a tournament",
		Long: "ratings lists the highest Elo ratings among a tournament's Makuuchi and Juryo\n" +
			"wrestlers after it, with the change over the tournament (SPEC §6.8). The rating\n" +
			"is an estimate from past results, not betting advice.",
		Args: usageArgs(cobra.NoArgs),
		RunE: a.data(func(cmd *cobra.Command, svc *service.Service, _ []string) error {
			id, err := a.tournament(tournament)
			if err != nil {
				return err
			}
			res, err := svc.RatingTable(a.dataCtx(cmd), id, top)
			if err != nil {
				return err
			}
			return a.emit(cmd, "ratings", res.Prov, func() string { return renderRatings(a.glyphs(), res.Value, res.Prov) }, func() any { return ratingsJSON(res.Value) })
		}),
	}
	tFlag(ratings)
	ratings.Flags().IntVar(&top, "top", 20, "number of wrestlers (0: all)")

	creditsCmd := &cobra.Command{
		Use: "credits", Short: "Data sources", Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := service.Provenance{Source: "basho"}
			return a.emit(cmd, "credits", p, func() string { return credits }, func() any { return map[string]string{"text": credits} })
		},
	}
	return []*cobra.Command{list, info, day, bout, standings, a.watchCommand(), ratings, creditsCmd}
}

// dayArg is the day given, or the default (SPEC §11).
func (a *app) dayArg(cmd *cobra.Command, svc *service.Service, args []string, id domain.BashoID) (int, error) {
	if len(args) == 1 {
		return number("day", args[0])
	}
	return svc.DefaultDay(a.dataCtx(cmd), id)
}

func number(what, s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, usage(err)
	}
	return n, nil
}

// usageArgs makes argument errors usage errors: exit code 2.
func usageArgs(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return usage(err)
		}
		return nil
	}
}
