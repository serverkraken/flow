package worktime

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/tui/shell"
)

// adjustStartMsg is emitted by the "Startzeit anpassen" palette entry and
// handled in TodayRoute.Update to open the start-edit dialog for the running
// session.
type adjustStartMsg struct{}

// PaletteEntries implements shell.PaletteProvider: while a timer runs, the
// ":"-palette offers "Startzeit anpassen" to correct the running session's
// start time.
func (r *TodayRoute) PaletteEntries() []shell.PaletteEntry {
	if !r.st.Running || r.st.Active == nil {
		return nil
	}
	return []shell.PaletteEntry{{
		Label:  "Startzeit anpassen",
		Action: func() tea.Msg { return adjustStartMsg{} },
	}}
}
