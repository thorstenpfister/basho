package datatool

import (
	"bytes"
	"testing"
)

// Piped output (a log, make's output) gets a line every 25 items and the last.
func TestProgressPiped(t *testing.T) {
	var b bytes.Buffer
	p := newProgress(&b, "thumbnails", 60)
	for i := 1; i <= 60; i++ {
		p.step(i, "x")
	}
	p.done()
	if got := b.String(); got != "  thumbnails 25/60\n  thumbnails 50/60\n  thumbnails 60/60\n" {
		t.Errorf("%q", got)
	}
}
