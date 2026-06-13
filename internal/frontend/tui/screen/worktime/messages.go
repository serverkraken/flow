package worktime

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// — project-picker messages (new ActiveSessions path) —

// pickerPickedMsg is emitted by the project_picker's onPick callback when the
// user selects an existing project. heute.Update handles it by dispatching
// activeSessionsStartCmd.
type pickerPickedMsg struct {
	projectID   string
	projectName string
}

// pickerCreateMsg is emitted by the project_picker's onCreate callback when
// the user confirms the sticky "+ Neues Projekt anlegen" entry. heute.Update
// handles it by dispatching projectsCreateThenStartCmd.
type pickerCreateMsg struct {
	name string
}

// pickerCancelMsg is emitted by the project_picker's onCancel value. heute
// closes the picker and restores normal focus.
type pickerCancelMsg struct{}

// ChangedMsg signals that day/session/dayoff data changed
// (created, edited, deleted). Emitted by any worktime sub-tab that
// commits a mutation; broadcast by the parent worktime Model to all
// sub-tabs so stale views reload.
//
// Date is the affected calendar day (or zero for global mutations
// like a yearly Feiertage-Sync). Sub-tabs decide whether they need
// a reload based on whether their visible date range intersects —
// today the four current sub-tabs always reload, but the contract
// stays date-scoped so a future tab covering a different window can
// no-op cheaply.
//
// Background: editing a session in History didn't update Heute or
// Woche until the next dayRefreshMsg tick — typing a tag in History,
// switching to Heute, the row still showed the old value. The msg
// closes that loop deterministically (mutation → emit → broadcast →
// per-tab reload).
type ChangedMsg struct {
	Date time.Time
}

// emitWorktimeChanged returns a tea.Cmd that fires ChangedMsg
// for date. Mutation closures bundle this via tea.Batch alongside their
// existing `*ActionDoneMsg`-returning closure so the parent Model can
// fan the change-signal out to all sub-tabs without coupling itself to
// action-done message shapes.
func emitWorktimeChanged(date time.Time) tea.Cmd {
	return func() tea.Msg { return ChangedMsg{Date: date} }
}
