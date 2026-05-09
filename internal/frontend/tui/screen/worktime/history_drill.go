package worktime

// History drill — Tag-Detail-View mit Sessions-Liste, Pause-Trennern,
// Drill-Edit/Add/Delete-Dialog (in history_edit.go), und der Footer-
// Rendering. Split aus history.go (Skill §No-Monoliths) damit die
// Drill-Surface in einem File zusammenhängend lesbar bleibt.

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/components/toast"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// — drill open + key dispatch —

func (h history) openDrill(date time.Time) (tea.Model, tea.Cmd) {
	h.dialog = historyDialogDrill
	h.drillDate = startOfDay(date)
	h.drillCur = 0
	h.drillSessions = nil
	h.drillErr = nil
	return h, h.drillLoadCmd(h.drillDate)
}

func (h history) handleDrillKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "b":
		h.dialog = historyDialogNone
		h.drillSessions = nil
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
		dur := lipgloss.NewStyle().Width(8).Render(formatDur(s.Elapsed))
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

// drillFormHint is the canonical key-hint shown while editing or adding
// a drill session. Both modes share the same form layout, so the hint
// is identical and lives here as a single literal.
const drillFormHint = "  Tab/↑↓ → Feld  ·  Enter → weiter / speichern  ·  Esc → abbrechen"

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
	// Empty-Day-Empty-State: kein Session-Eintrag → `a → neu` an die
	// Spitze des Hint-Stacks, weil "navigieren" auf einer leeren Liste
	// keinen Sinn ergibt.
	if len(h.drillSessions) == 0 {
		return stDim(h.pal, "  a → erste Session hinzufügen  ·  b/Esc → zurück")
	}
	hints := []string{"j/k → bewegen"}
	if h.drillOnSession() {
		hints = append(hints, "enter → bearbeiten", "D → löschen")
	}
	hints = append(hints, "a → neu", "b/Esc → zurück")
	if len(hints) > 4 {
		// 4-Hint-Limit (Skill §Hint format) — bei vorhandener Session
		// fällt der seltenere "neu" weg, sonst der "back".
		hints = hints[:4]
	}
	return stDim(h.pal, "  "+strings.Join(hints, "  ·  "))
}
