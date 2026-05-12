package worktime

// Heute action commands — async tea.Cmd wrappers around the Session-
// Writer / NoteOpener / LinkWriter ports. Split from today.go so the
// model/Update file stays focused on routing while the action surfaces
// (start/stop/pause, attached-note ops, delete) live next to each other.

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/glyphs"
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
	return func() tea.Msg {
		if err := writer.Remove(date, id); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("%s Note %s entfernt", glyphs.Done, id)}
	}
}

// toggleStartStopCmd mappt den `s`-Key auf das simpelste sinnvolle
// Verhalten: start im Idle, resume im Pause, stop im Run. Der smart
// stop-choice prompt für sehr kurze Sessions ist deferred.
func (h heute) toggleStartStopCmd() tea.Cmd {
	sw := h.deps.SessionWriter
	clock := h.deps.Clock
	switch {
	case h.day.IsRunning():
		return func() tea.Msg {
			s, err := sw.Stop()
			if err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: fmt.Sprintf("%s Gestoppt — Session %s", glyphs.Stopped, formatDur(s.Elapsed))}
		}
	case h.day.IsPaused():
		return func() tea.Msg {
			if err := sw.Resume(); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: glyphs.Active + " Worktime fortgesetzt"}
		}
	default:
		return func() tea.Msg {
			now := clock.Now()
			if err := sw.Start(now); err != nil {
				return heuteActionDoneMsg{err: err}
			}
			return heuteActionDoneMsg{toast: glyphs.Active + " Worktime gestartet — " + now.Format("15:04")}
		}
	}
}

func (h heute) pauseCmd() tea.Cmd {
	sw := h.deps.SessionWriter
	return func() tea.Msg {
		s, err := sw.Pause()
		if err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("%s Pausiert nach %s", glyphs.Paused, formatDur(s.Elapsed))}
	}
}

func (h heute) deleteCmd(date time.Time, idx int) tea.Cmd {
	sw := h.deps.SessionWriter
	return func() tea.Msg {
		if err := sw.Delete(date, idx); err != nil {
			return heuteActionDoneMsg{err: err}
		}
		return heuteActionDoneMsg{toast: fmt.Sprintf("%s Session %d gelöscht", glyphs.Done, idx+1)}
	}
}
