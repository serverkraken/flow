package worktime

// Heute project-picker integration — openPickerCmd, handleSKey,
// activeSessionsStartCmd, projectsCreateThenStartCmd, activeSessionsListCmd.
//
// When deps.ActiveSessions and deps.UserID are both set (`s`-key) the screen
// opens project_picker instead of calling the legacy SessionWriter. The picker
// runs as a full-screen bubbletea overlay (pp field on heute) and emits
// pickerPickedMsg / pickerCreateMsg / pickerCancelMsg via its onPick /
// onCreate / onCancel callbacks.
//
// Legacy path: when deps.ActiveSessions == nil or deps.UserID == "", `s` falls
// through to toggleStartStopCmd in today_actions.go. All existing tests that
// use newRig (which never sets ActiveSessions/UserID) continue to exercise the
// legacy path — no change to their expectations.

import (
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/frontend/tui/components/project_picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/usecase"
)

// activeSessionsListMsg carries the result of an ActiveSessions.ListActive
// call. heute.Update stores the slice in h.activeSessions for use in the
// running-indicator header and to pre-filter the picker's item list.
type activeSessionsListMsg struct {
	sessions []domain.ActiveSession
	err      error
}

// handlePickerMsg dispatches the four picker-related message types so they
// can be grouped behind a single case in heute.Update (reducing its cyclomatic
// complexity back below the 20-branch limit).
func (h heute) handlePickerMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case pickerPickedMsg:
		h.pp = nil
		h.actionInFlight = true
		return h, h.activeSessionsStartCmd(msg.projectID, msg.projectName)
	case pickerCreateMsg:
		h.pp = nil
		h.actionInFlight = true
		return h, h.projectsCreateThenStartCmd(msg.name)
	case pickerCancelMsg:
		h.pp = nil
		return h, nil
	case activeSessionsListMsg:
		if msg.err == nil {
			h.activeSessions = msg.sessions
		}
		return h, nil
	}
	return h, nil
}

// handleSKey is the dispatcher for the `s` key in normal (no-dialog) mode.
// New path: when ActiveSessions + UserID are wired, open the project picker.
// Legacy path: call toggleStartStopCmd (existing behaviour, unchanged).
func (h heute) handleSKey() (tea.Model, tea.Cmd) {
	if h.deps.ActiveSessions != nil && h.deps.UserID != "" {
		return h.openProjectPicker()
	}
	return h, h.toggleStartStopCmd()
}

// openProjectPicker loads the project list, builds the picker, and sets h.pp.
// The picker is initialised with Size(h.width, h.height) immediately; a
// subsequent WindowSizeMsg will resize it if the terminal changes.
//
// Already-running projects are filtered out from the item list so the picker
// only shows projects that can actually be started. This is the simplest UX
// (no greyed-out rows needed) and matches the brief's "pass only not-yet-running
// ones" recommendation.
func (h heute) openProjectPicker() (tea.Model, tea.Cmd) {
	projects := h.deps.Projects
	userID := h.deps.UserID
	if projects == nil {
		// Projects dep missing — degrade to legacy.
		return h, h.toggleStartStopCmd()
	}

	// Build running-project ID set for pre-filter.
	runningIDs := map[string]bool{}
	for _, as := range h.activeSessions {
		runningIDs[as.ProjectID] = true
	}

	// Synchronous list (projects are small, typically < 50 rows; the
	// Projects.ListActive call hits a local SQLite store). If latency
	// becomes a concern a future task can add an async load here.
	items, err := projects.ListActive(userID)
	if err != nil {
		t := toast.NewDanger("Projekte konnten nicht geladen werden: "+err.Error(), h.pal)
		h.toast = &t
		return h, t.Init()
	}

	// Exclude already-running projects.
	filtered := items[:0:len(items)]
	for _, p := range items {
		if !runningIDs[p.ID] {
			filtered = append(filtered, p)
		}
	}

	pp := project_picker.New(
		filtered,
		h.pal,
		func(p domain.Project) tea.Msg { return pickerPickedMsg{projectID: p.ID, projectName: p.Name} },
		func(name string) tea.Msg { return pickerCreateMsg{name: name} },
		pickerCancelMsg{},
	)
	pp = pp.SetSize(h.width, h.height)
	cmd := pp.Init()
	h.pp = &pp
	return h, cmd
}

// activeSessionsStartCmd calls ActiveSessions.Start for the given project and
// returns a tea.Cmd that emits heuteActionDoneMsg. On success a ChangedMsg is
// also emitted so all sub-tabs reload.
func (h heute) activeSessionsStartCmd(projectID, projectName string) tea.Cmd {
	as := h.deps.ActiveSessions
	userID := h.deps.UserID
	now := h.deps.Clock.Now()
	mut := func() tea.Msg {
		// TUI picker does not yet prompt for tag/note; pass empty strings.
		// The Stop dialog (today_dialog_submit) can still attach tag/note.
		if _, err := as.Start(userID, projectID, "", ""); err != nil {
			// ErrActiveSessionExists is not a hard error — it just means the
			// user picked a project that started running on another device
			// between the picker opening and the Enter press. Surface it as a
			// warning toast and let the user pick again.
			if errors.Is(err, usecase.ErrActiveSessionExists) {
				return heuteActionDoneMsg{
					toast: glyphs.Active + " " + projectName + " läuft bereits",
					info:  true,
				}
			}
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{
			toast: fmt.Sprintf("%s %s gestartet — %s", glyphs.Active, projectName, now.Format("15:04")),
		}
	}
	return tea.Batch(mut, emitWorktimeChanged(now))
}

// projectsCreateThenStartCmd creates a new project via Projects.Create and
// then calls ActiveSessions.Start on the resulting ID. Both operations run in
// a single tea.Cmd closure so the caller sees one heuteActionDoneMsg at the
// end.
func (h heute) projectsCreateThenStartCmd(name string) tea.Cmd {
	projects := h.deps.Projects
	as := h.deps.ActiveSessions
	userID := h.deps.UserID
	now := h.deps.Clock.Now()
	mut := func() tea.Msg {
		created, err := projects.Create(userID, name)
		if err != nil {
			return heuteActionDoneMsg{err: fmt.Errorf("projekt anlegen: %w", err)}
		}
		if _, err := as.Start(userID, created.ID, "", ""); err != nil {
			if errors.Is(err, usecase.ErrActiveSessionExists) {
				return heuteActionDoneMsg{
					toast: glyphs.Active + " " + created.Name + " läuft bereits",
					info:  true,
				}
			}
			return heuteActionDoneMsg{err: fmt.Errorf("session starten: %w", err)}
		}
		return heuteActionDoneMsg{
			toast: fmt.Sprintf("%s %s angelegt und gestartet — %s", glyphs.Active, created.Name, now.Format("15:04")),
		}
	}
	return tea.Batch(mut, emitWorktimeChanged(now))
}

// activeSessionsListCmd loads the current active sessions from the store.
// The result is delivered as activeSessionsListMsg; heute.Update stores it
// in h.activeSessions for the running-indicator and picker pre-filter.
func (h heute) activeSessionsListCmd() tea.Cmd {
	as := h.deps.ActiveSessions
	userID := h.deps.UserID
	return func() tea.Msg {
		sessions, err := as.ListActive(userID)
		return activeSessionsListMsg{sessions: sessions, err: err}
	}
}
