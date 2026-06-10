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
	"github.com/serverkraken/flow/internal/ports"
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
		// If a session is already running, decide between switch and no-op.
		if len(h.activeSessions) > 0 {
			current := h.activeSessions[0]
			if current.ProjectID == msg.projectID {
				// Same project — no-op. Show a gentle confirmation toast.
				return h, func() tea.Msg {
					return heuteActionDoneMsg{
						toast: glyphs.Active + " " + msg.projectName + " läuft bereits",
						info:  true,
					}
				}
			}
			// Different project — stop current, start new.
			h.actionInFlight = true
			return h, h.switchProjectCmd(current, msg.projectID, msg.projectName)
		}
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
// New path: when ActiveSessions + UserID are wired, `s` always opens the
// project picker regardless of whether a session is currently running.
// If the user then picks a different project the picker handler sequences a
// Stop+Start atomically (switchProjectCmd). Picking the same project is a
// no-op. This lets the user switch projects in a single gesture.
//
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
// The currently-running project (if any) is included in the list with a "▶ "
// prefix so the user can see what is running and understand what they are
// switching FROM. Picking the running project is a no-op (handled in
// handlePickerMsg). Picking a different project sequences Stop+Start atomically
// via switchProjectCmd.
func (h heute) openProjectPicker() (tea.Model, tea.Cmd) {
	projects := h.deps.Projects
	userID := h.deps.UserID
	if projects == nil {
		// Projects dep missing — degrade to legacy.
		return h, h.toggleStartStopCmd()
	}

	// Build running-project ID set for display annotation.
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

	// Annotate the running project with a "▶ " prefix so the user can
	// see at a glance which project is active. All projects remain
	// selectable; picking the running one produces a no-op toast.
	annotated := make([]domain.Project, len(items))
	copy(annotated, items)
	for i, p := range annotated {
		if runningIDs[p.ID] {
			annotated[i].Name = "▶ " + p.Name
		}
	}

	pp := project_picker.New(
		annotated,
		h.pal,
		func(p domain.Project) tea.Msg {
			// Strip the running-indicator prefix before forwarding the name so
			// toast messages and Start calls use the clean project name.
			name := p.Name
			if len(name) > 2 && name[:2] == "▶ " {
				name = name[2:]
			}
			return pickerPickedMsg{projectID: p.ID, projectName: name}
		},
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

// switchProjectCmd sequences ActiveSessions.Stop(current) then
// ActiveSessions.Start(newProjectID) in a single goroutine so the caller sees
// exactly one heuteActionDoneMsg at the end. The two calls are NOT separated
// by a model-update tick, preventing flicker or partial state.
//
// Error semantics:
//   - Stop returns ErrActiveSessionNotFound → treat as already stopped, proceed.
//   - Stop returns any other error → abort, emit error msg (do not start new).
//   - Start fails → emit error msg (stop already committed locally).
func (h heute) switchProjectCmd(current domain.ActiveSession, newProjectID, newProjectName string) tea.Cmd {
	as := h.deps.ActiveSessions
	userID := h.deps.UserID
	now := h.deps.Clock.Now()
	mut := func() tea.Msg {
		stopped, err := as.Stop(userID, current.ProjectID, "", "")
		if err != nil && !errors.Is(err, ports.ErrActiveSessionNotFound) {
			return heuteActionDoneMsg{err: err}
		}
		if _, err := as.Start(userID, newProjectID, "", ""); err != nil {
			if errors.Is(err, usecase.ErrActiveSessionExists) {
				return heuteActionDoneMsg{
					toast: glyphs.Active + " " + newProjectName + " läuft bereits",
					info:  true,
				}
			}
			return heuteActionDoneMsg{err: fmt.Errorf("session starten: %w", err)}
		}
		// Build toast. If the stop succeeded (not a NotFound no-op), include the
		// elapsed time of the old session for context.
		oldName := current.ProjectName
		if oldName == "" {
			oldName = current.ProjectID
		}
		if errors.Is(err, ports.ErrActiveSessionNotFound) {
			// Old session was already gone — just confirm the new one.
			return heuteActionDoneMsg{
				toast: fmt.Sprintf("%s Gewechselt zu '%s'", glyphs.Active, newProjectName),
			}
		}
		return heuteActionDoneMsg{
			toast: fmt.Sprintf("%s Wechsel: '%s' (%dh %02dm) → '%s'",
				glyphs.Active, oldName,
				int(stopped.Elapsed.Hours()), int(stopped.Elapsed.Minutes())%60,
				newProjectName),
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

// activeSessionsListCmd loads the current active sessions from the store and
// enriches each with its project name. The result is delivered as
// activeSessionsListMsg; heute.Update stores it in h.activeSessions for the
// running-indicator and picker pre-filter.
//
// Enrich step: when deps.Projects is set, a single ListActive call builds a
// name map and stamps ActiveSession.ProjectName. Nil-tolerant: when
// deps.Projects is nil (legacy tests, offline tools) sessions are returned
// with empty ProjectName and the indicator falls back to UUID-suffix display.
func (h heute) activeSessionsListCmd() tea.Cmd {
	as := h.deps.ActiveSessions
	projects := h.deps.Projects
	userID := h.deps.UserID
	return func() tea.Msg {
		sessions, err := as.ListActive(userID)
		if err != nil || projects == nil || len(sessions) == 0 {
			return activeSessionsListMsg{sessions: sessions, err: err}
		}
		// Build name map once; stamp each session.
		all, projErr := projects.ListActive(userID)
		if projErr == nil {
			names := make(map[string]string, len(all))
			for _, p := range all {
				names[p.ID] = p.Name
			}
			for i := range sessions {
				sessions[i].ProjectName = names[sessions[i].ProjectID]
			}
		}
		return activeSessionsListMsg{sessions: sessions, err: nil}
	}
}
