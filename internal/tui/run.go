package tui

import tea "charm.land/bubbletea/v2"

// Run starts the TUI over a starting stack and blocks until it quits.
func Run(ctx *Context, stack []Screen, today func() []Screen, opts ...tea.ProgramOption) error {
	_, err := tea.NewProgram(New(ctx, stack, today), opts...).Run()
	return err
}
