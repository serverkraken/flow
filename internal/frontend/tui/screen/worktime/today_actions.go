package worktime

// Heute action commands — async tea.Cmd wrappers around the Session-
// Writer / NoteOpener / LinkWriter ports. Split from today.go so the
// model/Update file stays focused on routing while the action surfaces
// (start/stop/pause, attached-note ops, delete) live next to each other.

import (
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
	"github.com/serverkraken/flow/internal/ports"
)

// editAttachedNoteCmd öffnet die erste angehängte Note im Editor via
// NoteOpener.Open (typischerweise tmux split + nvim). Der
// „keine angehängten Notes"-Branch liefert einen Info-Toast statt
// durch den Empty-ID-Guard zu fallen.
func (h heute) editAttachedNoteCmd() tea.Cmd {
	if len(h.attachedNotes) == 0 {
		return func() tea.Msg {
			return heuteActionDoneMsg{toast: "Keine Notiz angehängt — `n` hängt eine an", info: true}
		}
	}
	id := h.attachedNotes[0]
	opener := h.deps.NoteOpener
	return func() tea.Msg {
		if err := opener.Open(id); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("%s Note %s zum Bearbeiten geöffnet", glyphs.Done, id)}
	}
}

// detachAttachedNoteCmd entfernt die erste angehängte Note via
// LinkWriter.Remove. Kein Confirm-Dialog: die Operation ist reversibel
// (re-attach via `n` mit derselben ID) und der Store ist idempotent
// — Over-Remove eines bereits fehlenden Pairs ist als no-op dokumentiert.
// `D` (uppercase) ist absichtlich NICHT verwendet — bindet bereits
// delete-session in Welle B; `R` (uppercase Remove) landet in derselben
// destructive-uppercase Grammatik ohne D-Kollision.
func (h heute) detachAttachedNoteCmd() tea.Cmd {
	if len(h.attachedNotes) == 0 {
		return func() tea.Msg {
			return heuteActionDoneMsg{toast: "Keine Notiz angehängt", info: true}
		}
	}
	id := h.attachedNotes[0]
	date := h.deps.Clock.Now()
	writer := h.deps.LinkWriter
	mut := func() tea.Msg {
		if err := writer.Remove(date, id); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("%s Note %s entfernt", glyphs.Done, id)}
	}
	return tea.Batch(mut, emitWorktimeChanged(date))
}

// toggleStartStopCmd mappt den `s`-Key auf das simpelste sinnvolle
// Verhalten: start im Idle, resume im Pause, stop im Run. Der smart
// stop-choice prompt für sehr kurze Sessions ist deferred.
func (h heute) toggleStartStopCmd() tea.Cmd {
	sw := h.deps.SessionWriter
	clock := h.deps.Clock
	now := clock.Now()
	switch {
	case h.day.IsRunning():
		mut := func() tea.Msg {
			s, err := sw.Stop()
			if err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("%s Gestoppt — Session %s", glyphs.Stopped, formatDur(s.Elapsed))}
		}
		return tea.Batch(mut, emitWorktimeChanged(now))
	case h.day.IsPaused():
		mut := func() tea.Msg {
			if err := sw.Resume(); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: glyphs.Active + " Worktime fortgesetzt"}
		}
		return tea.Batch(mut, emitWorktimeChanged(now))
	default:
		mut := func() tea.Msg {
			if err := sw.Start(now); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: glyphs.Active + " Worktime gestartet — " + now.Format("15:04")}
		}
		return tea.Batch(mut, emitWorktimeChanged(now))
	}
}

func (h heute) pauseCmd() tea.Cmd {
	sw := h.deps.SessionWriter
	now := h.deps.Clock.Now()
	mut := func() tea.Msg {
		// New path: when ActiveSessions + UserID are wired and a session is
		// running, pause the ActiveSessions row. Falls through to the legacy
		// SessionWriter.Pause() path when not wired.
		if h.deps.ActiveSessions != nil && h.deps.UserID != "" && len(h.activeSessions) > 0 {
			target := h.activeSessions[0]
			sess, err := h.deps.ActiveSessions.Pause(h.deps.UserID, target.ProjectID)
			if errors.Is(err, ports.ErrActiveSessionNotFound) {
				return heuteActionDoneMsg{toast: "Pausiert — Session war bereits gestoppt", info: true}
			}
			if err != nil {
				return heuteActionDoneMsg{err: err}
			}
			elapsed := sess.Elapsed(now)
			return heuteActionDoneMsg{
				toast: fmt.Sprintf("%s Pausiert nach %dh %02dm", glyphs.Paused, int(elapsed.Hours()), int(elapsed.Minutes())%60),
			}
		}
		// Legacy path: SessionWriter.Pause stops the flockstate session.
		s, err := sw.Pause()
		if err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("%s Pausiert nach %s", glyphs.Paused, formatDur(s.Elapsed))}
	}
	return tea.Batch(mut, emitWorktimeChanged(now))
}

func (h heute) deleteCmd(date time.Time, idx int) tea.Cmd {
	sw := h.deps.SessionWriter
	mut := func() tea.Msg {
		if err := sw.Delete(date, idx); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("%s Session %d gelöscht", glyphs.Done, idx+1)}
	}
	return tea.Batch(mut, emitWorktimeChanged(date))
}
