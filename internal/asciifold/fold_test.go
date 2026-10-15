package asciifold

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Every stand-in is one ASCII cell for a one-cell symbol, so a folded
// screen keeps its layout.
func TestFold(t *testing.T) {
	for i := 0; i < len(folds); i += 2 {
		from, to := folds[i], folds[i+1]
		if ansi.StringWidth(from) != 1 || len(to) != 1 || to[0] > 127 {
			t.Errorf("%q → %q", from, to)
		}
	}
	if got := Fold("Day 13 · Kotozakur… ✓ ≈Oho │ 結び"); got != "Day 13 . Kotozakur. v ~Oho | 結び" {
		t.Errorf("Fold: %q", got)
	}
}
