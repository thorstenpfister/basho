package datatool

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

// progress reports a long loop: on a terminal one line, rewritten for each
// item; otherwise a line every progressEvery items, so logs stay short.
type progress struct {
	w     io.Writer
	tty   bool
	label string
	total int
}

const progressEvery = 25

func newProgress(w io.Writer, label string, total int) *progress {
	f, ok := w.(*os.File)
	return &progress{w: w, tty: ok && term.IsTerminal(f.Fd()), label: label, total: total}
}

// step reports item i (from 1) of the total, named.
func (p *progress) step(i int, name string) {
	switch {
	case p.tty:
		_, _ = fmt.Fprintf(p.w, "\r\x1b[K  %s %d/%d  %s", p.label, i, p.total, name)
	case i%progressEvery == 0 || i == p.total:
		_, _ = fmt.Fprintf(p.w, "  %s %d/%d\n", p.label, i, p.total)
	}
}

// done ends the line on a terminal, before the next output.
func (p *progress) done() {
	if p.tty {
		_, _ = fmt.Fprint(p.w, "\r\x1b[K")
	}
}
