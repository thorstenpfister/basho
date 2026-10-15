// Package tuitest drives Bubble Tea models synchronously in tests.
package tuitest

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Driver runs a model: each message's commands run to completion, batches
// are expanded, and the resulting messages are fed back until none are left.
// The model's timers must be off (tui.NoTimers), or they would block.
type Driver struct {
	M tea.Model
}

// New returns a driver over m after its Init commands.
func New(m tea.Model, width, height int) *Driver {
	d := &Driver{M: m}
	d.Send(tea.WindowSizeMsg{Width: width, Height: height})
	d.run(m.Init())
	return d
}

// Send delivers messages in order.
func (d *Driver) Send(msgs ...tea.Msg) {
	for _, msg := range msgs {
		var cmd tea.Cmd
		d.M, cmd = d.M.Update(msg)
		d.run(cmd)
	}
}

func (d *Driver) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			d.run(c)
		}
	case tea.QuitMsg:
	default:
		d.Send(msg)
	}
}

// Keys presses keys by name: "a", "enter", "esc", "left", "ctrl+c", ….
func (d *Driver) Keys(keys ...string) {
	for _, k := range keys {
		d.Send(Key(k))
	}
}

// Key returns the key press for a name.
func Key(k string) tea.KeyPressMsg {
	special := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "left": tea.KeyLeft, "right": tea.KeyRight,
		"up": tea.KeyUp, "down": tea.KeyDown, "tab": tea.KeyTab, "space": tea.KeySpace,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
	}
	if c, ok := strings.CutPrefix(k, "ctrl+"); ok && len(c) == 1 {
		return tea.KeyPressMsg{Code: rune(c[0]), Mod: tea.ModCtrl}
	}
	if k == "shift+tab" {
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	}
	if c, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: c}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

// View is the model's view without styles.
func (d *Driver) View() string { return ansi.Strip(d.M.View().Content) }
