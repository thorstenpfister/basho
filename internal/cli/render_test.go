package cli

import "testing"

func TestTrimLines(t *testing.T) {
	in := "\x1b[1ma\x1b[m  \x1b[1m\x1b[m \nx  y  \nz"
	if got, want := trimLines(in), "\x1b[1ma\x1b[m\x1b[1m\x1b[m\nx  y\nz"; got != want {
		t.Errorf("trimLines = %q, want %q", got, want)
	}
}
