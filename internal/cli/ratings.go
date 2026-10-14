package cli

import (
	"fmt"
	"math"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/thorstenpfister/basho/internal/domain"
	"github.com/thorstenpfister/basho/internal/service"
)

func newRatingsCmd(svc *service.Service) *cobra.Command {
	var tournament string
	var top int
	cmd := &cobra.Command{
		Use:   "ratings",
		Short: "Top-rated wrestlers as of a tournament",
		Long: "ratings lists the highest Elo ratings among a tournament's Makuuchi and Juryo\n" +
			"wrestlers after it, with the change over the tournament (SPEC §6.8). The rating\n" +
			"is an estimate from past results, not betting advice.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id, err := domain.ParseTournament(tournament, domain.DateOf(time.Now()))
			if err != nil {
				return err
			}
			res, err := svc.RatingTable(cmd.Context(), id, top)
			if err != nil {
				return err
			}
			t := res.Value
			w := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(w, "Ratings after %s %d (%s)\n", id.Name(), id.Year, id); err != nil {
				return err
			}
			if !t.Shown {
				if _, err := fmt.Fprintln(w, "(the rating did not beat the rank baseline in calibration; shown here only)"); err != nil {
					return err
				}
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "#\tRank\tWrestler\tRating\tChange")
			for i, r := range t.Rows {
				_, _ = fmt.Fprintf(tw, "%d\t%s\t%s\t%.0f\t%s\n", i+1, r.Rank.Short(), r.Shikona.En, r.Value, change(r.Change))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&tournament, "tournament", "t", "current", "tournament: 202609, 2026-09, aki2026, current, prev, next")
	cmd.Flags().IntVar(&top, "top", 20, "number of wrestlers (0: all)")
	return cmd
}

// change prints a rating change as ▲12, ▼8 or – (SPEC §8.5).
func change(d float64) string {
	switch n := math.Round(d); {
	case n > 0:
		return fmt.Sprintf("▲%.0f", n)
	case n < 0:
		return fmt.Sprintf("▼%.0f", -n)
	default:
		return "–"
	}
}
