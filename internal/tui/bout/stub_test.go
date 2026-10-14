package bout

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/thorstenpfister/basho/internal/tui"
)

// tuiDayStub stands in for the day view under the VS screen.
type tuiDayStub struct{}

func (tuiDayStub) Init(*tui.Context) tea.Cmd                            { return nil }
func (s tuiDayStub) Update(*tui.Context, tea.Msg) (tui.Screen, tea.Cmd) { return s, nil }
func (tuiDayStub) View(*tui.Context, int, int) string                   { return "day" }
func (tuiDayStub) Status(*tui.Context) tui.Status                       { return tui.Status{} }
func (tuiDayStub) Keys() []key.Binding                                  { return nil }
