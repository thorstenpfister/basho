package cli

import (
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
		return run(cmd, svc, args)
	}
}

// emit writes a result as text.
func (a *app) emit(cmd *cobra.Command, text string) error {
	_, err := a.out(cmd).Write([]byte(text))
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
			res, err := svc.List(cmd.Context(), from, to)
			if err != nil {
				return err
			}
			return a.emit(cmd, renderList(res.Value, res.Prov))
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
			res, err := svc.Info(cmd.Context(), id)
			if err != nil {
				return err
			}
			return a.emit(cmd, renderInfo(res.Value, res.Prov))
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
			res, err := svc.Day(cmd.Context(), id, div, n)
			if err != nil {
				return err
			}
			return a.emit(cmd, renderDay(a.glyphs(), res.Value, res.Prov))
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
			res, err := svc.Bout(cmd.Context(), id, div, n, match)
			if err != nil {
				return err
			}
			return a.emit(cmd, renderTale(a.glyphs(), res.Value, res.Prov))
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
			res, err := svc.Standings(cmd.Context(), id, div, n)
			if err != nil {
				return err
			}
			return a.emit(cmd, renderStandings(a.glyphs(), res.Value, grid, res.Prov))
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
			res, err := svc.RatingTable(cmd.Context(), id, top)
			if err != nil {
				return err
			}
			return a.emit(cmd, renderRatings(a.glyphs(), res.Value, res.Prov))
		}),
	}
	tFlag(ratings)
	ratings.Flags().IntVar(&top, "top", 20, "number of wrestlers (0: all)")

	creditsCmd := &cobra.Command{
		Use: "credits", Short: "Data sources", Args: usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error { return a.emit(cmd, credits) },
	}
	return []*cobra.Command{list, info, day, bout, standings, ratings, creditsCmd}
}

// dayArg is the day given, or the default (SPEC §11).
func (a *app) dayArg(cmd *cobra.Command, svc *service.Service, args []string, id domain.BashoID) (int, error) {
	if len(args) == 1 {
		return number("day", args[0])
	}
	return svc.DefaultDay(cmd.Context(), id)
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
