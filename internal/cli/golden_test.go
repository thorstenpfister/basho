package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// Golden files lock the text layout and the JSON schema (SPEC §13, Phase 6
// AC) over the fixture snapshot, which does not change with snapshot
// updates. Refresh them with go test ./internal/cli -run Golden -update.
func TestGolden(t *testing.T) {
	deps := fixtureDeps(t)
	for _, tt := range []struct {
		name string
		args []string
	}{
		{"day", []string{"day", "13", "-t", "202609"}},
		{"bout", []string{"bout", "13", "19", "-t", "aki2026"}},
		{"standings_grid", []string{"standings", "--day", "15", "-t", "2026-09", "--grid"}},
		{"standings", []string{"standings", "--day", "15", "-t", "2026-09"}},
		{"juryo_day_ascii", []string{"day", "15", "-t", "202609", "--division", "juryo", "--ascii"}},
		{"info", []string{"info", "202609"}},
		{"list", []string{"list", "--from", "2026"}},
		{"ratings", []string{"ratings", "-t", "202609", "--top", "10"}},
	} {
		for _, format := range []string{"text", "json"} {
			t.Run(tt.name+"_"+format, func(t *testing.T) {
				args := tt.args
				if format == "json" {
					args = append(args[:len(args):len(args)], "--json")
				}
				out, code := run(t, deps, args...)
				if code != exitOK {
					t.Fatalf("basho %s: exit %d\n%s", strings.Join(args, " "), code, out)
				}
				golden.RequireEqual(t, out)
			})
		}
	}
}
