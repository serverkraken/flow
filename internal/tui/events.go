package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/adapter/apiclient"
)

// errMsg carries an async error into Update.
type errMsg struct{ err error }

// eventMsg is one SSE client event delivered to a model.
type eventMsg struct{ ev apiclient.ClientEvent }

// eventsReadyMsg hands a model the live SSE channel after subscribe().
type eventsReadyMsg struct{ ch <-chan apiclient.ClientEvent }

// waitForEvent blocks on the SSE channel and re-delivers the next event.
func waitForEvent(ch <-chan apiclient.ClientEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg{ev: ev}
	}
}
