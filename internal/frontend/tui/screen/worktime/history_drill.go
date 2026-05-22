package worktime

// History drill — Tag-Detail-View mit Sessions-Liste, Pause-Trennern,
// Drill-Edit/Add/Delete-Dialog (in history_edit.go), und der Footer-
// Rendering. Split aus history.go (Skill §No-Monoliths) damit die
// Drill-Surface in einem File zusammenhängend lesbar bleibt.

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// — drill open + key dispatch —

func (h history) openDrill(date time.Time) (tea.Model, tea.Cmd) {
	h.dialog = historyDialogDrill
	h.drillDate = startOfDay(date)
	h.drillCur = 0
	h.drillSessions = nil
	h.drillAttached = nil
	h.drillNoteView = nil
	h.drillErr = nil
	return h, h.drillLoadCmd(h.drillDate)
}

func (h history) handleDrillKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b":
		h.dialog = historyDialogNone
		h.drillSessions = nil
		h.drillAttached = nil
		h.drillNoteView = nil
		h.drillToast = nil
		return h, nil
	case "j", "down":
		if n := len(h.drillSessions); n > 0 {
			h.drillCur = (h.drillCur + 1) % n
		}
	case "k", "up":
		if n := len(h.drillSessions); n > 0 {
			h.drillCur = (h.drillCur + n - 1) % n
		}
	case "g":
		h.drillCur = 0
	case "G":
		if n := len(h.drillSessions); n > 0 {
			h.drillCur = n - 1
		}
	case "enter":
		if h.drillOnSession() {
			return h.openDrillEdit()
		}
	case "a":
		return h.openDrillAdd()
	case "D":
		if h.drillOnSession() {
			return h.openDrillDelete()
		}
	case "n":
		// Note-Attach hängt an drillDate, nicht an einer Session-Zeile —
		// die LinkStore-Keyung ist tagesbasiert. Funktioniert auch bei
		// leerem Tag (es gibt dann nichts zu fokussieren, aber der User
		// kann trotzdem eine Note an den Tag knüpfen, z.B. um spätere
		// retrospektive Notizen einzuhängen).
		return h.openNoteAttachDialog()
	case "o":
		// Inline-Viewer der ersten angehängten Note. Analog zu Heute's
		// `o` (today_note_view.go). Bei leerer drillAttached liefert
		// openDrillNoteView einen Info-Toast statt eines stillen no-op.
		return h.openDrillNoteView()
	case "O":
		// Externer Editor (NoteOpener) auf die erste angehängte Note.
		// Analog Heute's `O` in today_actions.go.
		return h, h.editDrillNoteCmd()
	case "R":
		// Detach der ersten angehängten Note. Skill §Keybind grammar:
		// `R` (uppercase Remove) bleibt von der `D`-Session-Delete-
		// Kollision frei. Reversible Operation, kein Confirm-Dialog.
		return h, h.detachDrillNoteCmd()
	}
	return h, nil
}

// drillOnSession reports whether the drill cursor sits on a real
// session row. Without sessions, the edit/delete shortcuts are no-ops.
func (h history) drillOnSession() bool {
	return h.drillCur >= 0 && h.drillCur < len(h.drillSessions)
}

// — drill render —

func (h history) renderDrill() string {
	// NoteView überdeckt den Drill voll — markdown_overlay rendert sein
	// eigenes Chrome (Frame + Title + Status-Bar), kein zusätzlicher
	// Drill-Frame nötig. Schließen via ExitMsg im outer Update bringt
	// dialog zurück auf historyDialogDrill und die Liste taucht wieder
	// auf.
	if h.dialog == historyDialogDrillNoteView && h.drillNoteView != nil {
		return h.drillNoteView.View()
	}
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}
	rows := []string{theme.Heading("  Tag "+h.drillDate.Format("2006-01-02")+" ("+
		domain.WeekdayShortDe(h.drillDate.Weekday())+")", h.pal), ""}
	if h.drillErr != nil {
		rows = append(rows, stErr(h.pal, h.drillErr.Error()))
		rows = append(rows, "", stDim(h.pal, drillBackHint))
		return strings.Join(rows, "\n")
	}
	if len(h.drillSessions) == 0 {
		rows = append(rows, stDim(h.pal, "  keine Sessions an diesem Tag"))
		// Auch im leeren Tag darf eine Note dranhängen — die LinkStore-
		// Keyung ist tagesbasiert. Chip-Zeile direkt unter dem
		// Empty-State, damit User retrospektiv angehängte Notizen
		// sehen ohne Sessions zu brauchen.
		if chip := h.renderDrillAttachedNotes(); chip != "" {
			rows = append(rows, "", chip)
		}
		// Even an empty day allows manual entry — `a` adds the first
		// session. Without this hint the only visible action is "back",
		// which would force the user into Heute-just-to-add-an-old-row.
		if dialogRows := h.renderDrillDialog(inner); len(dialogRows) > 0 {
			rows = append(rows, "")
			rows = append(rows, dialogRows...)
		}
		rows = append(rows, "", h.renderDrillFooter())
		return strings.Join(rows, "\n")
	}
	target := h.deps.Stats.Targets.For(h.drillDate)
	var total time.Duration
	for _, s := range h.drillSessions {
		total += s.Elapsed
	}
	pct := 0
	if target > 0 {
		pct = int(total * 100 / target)
	}
	rows = append(rows, "  "+theme.Strong(formatDur(total), h.pal)+
		"  "+stDim(h.pal, fmt.Sprintf("/ %s  ·  %d%%", formatDur(target), pct)))
	rows = append(rows, "")
	rows = append(rows, picker.SectionHeader(
		fmt.Sprintf("sessions (%d)", len(h.drillSessions)), inner, h.pal,
	))
	prevStop := time.Time{}
	for i, s := range h.drillSessions {
		if !prevStop.IsZero() {
			pause := s.Start.Sub(prevStop)
			if pause > 0 {
				rows = append(rows, stDim(h.pal,
					fmt.Sprintf("       ─ %s Pause ─", formatDur(pause))))
			}
		}
		prevStop = s.Stop
		dur := durationWidth8Style.Render(formatDur(s.Elapsed))
		label := fmt.Sprintf("%s → %s   %s",
			s.Start.Format("15:04"), s.Stop.Format("15:04"), dur)
		hint := ""
		if s.Tag != "" {
			hint = "[" + s.Tag + "]"
		}
		rows = append(rows, picker.Row(i == h.drillCur, label, hint, inner, h.pal))
		if s.Note != "" {
			rows = append(rows, stDim(h.pal, "       "+s.Note))
		}
	}

	// Chip-Zeile mit angehängten Notizen — analog Heute's
	// renderAttachedNotes (today_render.go). Leeres drillAttached
	// rendert KEINE Zeile, damit der Layout-Schwerpunkt bei den
	// Sessions bleibt und der Drill nicht visuell hin- und herwackelt
	// wenn der Tag mal Notizen hat und mal nicht.
	if chip := h.renderDrillAttachedNotes(); chip != "" {
		rows = append(rows, "", chip)
	}

	if dialogRows := h.renderDrillDialog(inner); len(dialogRows) > 0 {
		rows = append(rows, "")
		rows = append(rows, dialogRows...)
	}
	// Toast-Slot via SlotRows — kollabiert auf nichts, wenn kein Toast
	// aktiv ist, statt eine reservierte Leerzeile zu zeigen.
	if h.dialog == historyDialogDrill {
		rows = append(rows, toast.SlotRows(h.drillToast, "  ")...)
	}
	if h.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+h.errMsg, h.pal))
	}
	rows = append(rows, "", h.renderDrillFooter())
	return strings.Join(rows, "\n")
}

// renderDrillAttachedNotes rendert die Chip-Zeile mit den angehängten
// Kompendium-Note-IDs (analog Heute's renderAttachedNotes in
// today_render.go). Leere Liste → leerer String, der Caller skipt
// die Zeile dann komplett.
func (h history) renderDrillAttachedNotes() string {
	if len(h.drillAttached) == 0 {
		return ""
	}
	label := theme.Highlight("●", h.pal)
	ids := stDim(h.pal, strings.Join(h.drillAttached, "  ·  "))
	hint := stDim(h.pal, "  ·  o/O → ansehen/bearbeiten  ·  R → entfernen")
	return "  " + label + "  " + ids + hint
}

// drillFormHint is the canonical key-hint shown while editing or adding
// a drill session. Wording kommt aus uistrings.HintFormNav (geteilt mit
// today_dialog/dayoffs); zwei Spaces vorgestellt für die Footer-Indent-
// Konvention der Drill-Surface.
const drillFormHint = "  " + uistrings.HintFormNav

// drillBackHint is the standalone "back" hint shown when the drill
// branch has nothing else to advertise (error path). The plain drill
// footer composes its own multi-hint strip; this constant is for the
// degenerate case where only "back" is meaningful.
const drillBackHint = "  b/Esc zurück"

// renderDrillFooter picks the hint line for the active drill dialog
// mode. Each mode advertises its own keys; the bare drill view
// promotes navigate / edit / add / delete to the user.
func (h history) renderDrillFooter() string {
	switch h.dialog {
	case historyDialogDrillEdit, historyDialogDrillAdd:
		return stDim(h.pal, drillFormHint)
	case historyDialogDrillDelete:
		// confirm.Model rendert bereits den eigenen y/Enter-Hint —
		// die Footer-Zeile bleibt hier auf dem Standard "zurück", damit
		// der globale Help-Button + Tab-Strip nicht aus dem Layout fällt.
		return stDim(h.pal, "  y/Enter → löschen  ·  n/Esc → abbrechen")
	}
	if h.dialog == historyDialogDrillNoteAttach {
		return stDim(h.pal, "  "+h.notePicker.HintLine())
	}
	// Empty-Day-Empty-State: kein Session-Eintrag → `a → neu` an die
	// Spitze des Hint-Stacks, weil "navigieren" auf einer leeren Liste
	// keinen Sinn ergibt. `n → Note` bleibt aber relevant, weil LinkStore
	// tagesbasiert ist (auch ohne Sessions).
	if len(h.drillSessions) == 0 {
		return stDim(h.pal, "  a → erste Session hinzufügen  ·  n → Note  ·  b/Esc → zurück")
	}
	hints := []string{"j/k → bewegen"}
	if h.drillOnSession() {
		hints = append(hints, "enter → bearbeiten", "D → löschen")
	}
	// `o → ansehen` nur wenn überhaupt was angehängt ist — sonst wäre
	// der Hint eine Lüge (openDrillNoteView gibt dann den "keine Note"-
	// Toast aus). `n → Note` ist immer relevant (Attach-Operation
	// funktioniert auch ohne vorherige Anhänge).
	if len(h.drillAttached) > 0 {
		hints = append(hints, "o → ansehen")
	}
	hints = append(hints, "n → Note", "a → neu", "b/Esc → zurück")
	if len(hints) > 4 {
		// 4-Hint-Limit (Skill §Hint format) — bei vorhandener Session
		// fällt "a → neu" als seltenster weg; `n → Note` und Esc bleiben
		// nach der Priorisierung der häufigeren retrospektiven Aktionen
		// (Note anhängen ist im History-Drill häufiger als manuelle
		// Neu-Eingabe, da letztere typischer aus Heute heraus geht).
		hints = hints[:4]
	}
	return stDim(h.pal, "  "+strings.Join(hints, "  ·  "))
}
