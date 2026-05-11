package worktime

// Heute dialog rendering — renderDialog malt die fünf Dialog-Modi
// (Tag, Notiz, NoteAttach, Edit, Delete, Help) auf einem gemeinsamen
// Header-Strip. Aufmach-/Key-/Submit-Pfade liegen in today_dialog_open.go,
// today_dialog_keys.go bzw. today_dialog_submit.go (Skill §No-Monoliths).
// helpSectionsHeute + renderHelpRows bleiben hier, weil renderDialog
// im Help-Zweig direkt darauf zugreift.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/serverkraken/flow/internal/frontend/tui/components/help"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// renderDialog rendert die fünf Dialog-Modi (Tag, Notiz, NoteAttach,
// Edit, Delete, Help). Title sitzt am oberen Rand des Dialogs — der
// User soll beim Öffnen sofort wissen *wo* er ist, nicht erst zur
// letzten Zeile scrollen.
//
// Skill §Component vocabulary: Dialog-Title bekommt purple-bold (wie
// titlebox/help-Header) statt dim, sonst ist er im Body nicht mehr von
// dem Hint-String unterscheidbar. Hint-Format folgt §Hint format mit
// `key → action  ·  …`-Separatoren.
//
// Delete-Modus delegiert komplett an confirm.Model, das selbst
// Yellow-Question, Detail-Zeile und kanonisches y/Enter-→-ja-Hint mitbringt.
func (h heute) renderDialog() string {
	inner := h.width - 4
	if inner <= 0 {
		inner = 80
	}

	var rows []string
	var title, hint string

	switch h.dialog {
	case heuteDialogTag:
		title = "Tag setzen"
		hint = uistrings.HintInputSave
		rows = append(rows, picker.SectionHeader("tag", inner, h.pal), "  "+h.input.View())

	case heuteDialogNote:
		title = "Session-Notiz"
		hint = uistrings.HintInputSave
		rows = append(rows, picker.SectionHeader("notiz", inner, h.pal), "  "+h.input.View())

	case heuteDialogNoteAttach:
		title = "Kompendium-Note anhängen"
		// Hint hängt davon ab, ob ein Picker verfügbar ist — sonst
		// liest der User von der nicht-existenten Up/Down-Funktion ab.
		if len(h.noteSuggestions) > 0 {
			hint = "↑/↓ → wählen  ·  tippen → filter  ·  Enter → anhängen  ·  Esc → abbrechen"
		} else {
			hint = "Enter → anhängen  ·  Esc → abbrechen"
		}
		rows = append(rows, picker.SectionHeader("note id", inner, h.pal), "  "+h.input.View())
		rows = append(rows, h.renderNoteSuggestions(inner)...)
		if len(h.attachedNotes) > 0 {
			rows = append(rows, "", stDim(h.pal,
				"  bereits angehängt:  "+strings.Join(h.attachedNotes, "  ·  ")))
		}

	case heuteDialogEdit:
		title = "Session bearbeiten"
		hint = uistrings.HintFormNav
		if h.editIdx >= 0 && h.editIdx < len(h.day.Sessions) {
			s := h.day.Sessions[h.editIdx]
			rows = append(rows, stDim(h.pal, fmt.Sprintf("  Session %d:  %s → %s",
				h.editIdx+1, s.Start.Format("15:04"), s.Stop.Format("15:04"))), "")
		}
		labels := []string{"Start", "Stop", "Tag", "Notiz"}
		for i, ti := range h.form {
			rows = append(rows, picker.SectionHeader(labels[i], inner, h.pal))
			if i == h.formCur {
				rows = append(rows, "  "+ti.View())
			} else {
				v := ti.Value()
				if v == "" {
					v = stDim(h.pal, ti.Placeholder)
				}
				rows = append(rows, "    "+v)
			}
		}

	case heuteDialogDelete:
		title = "Session löschen"
		// confirm.Model rendert bereits seinen eigenen y/Enter-→-ja-Hint;
		// hier nur der gemeinsame Title-Strip, kein doppelter Hint nötig.
		hint = ""
		if h.confirmModel != nil {
			rows = append(rows, "  "+h.confirmModel.View())
		}

	case heuteDialogHelp:
		title = "Heute · Hilfe"
		hint = "beliebige Taste schließt"
		rows = append(rows, h.renderHelpRows(inner)...)
	}

	if h.errMsg != "" {
		rows = append(rows, "", theme.Err("  "+h.errMsg, h.pal))
	}
	// Title + Hint auf eigenen Zeilen: bei schmalen Sidekick-Panes (~70 Cols)
	// schluckte titlebox.Truncate vorher den Hint, weil er auf der gleichen
	// Zeile wie der Title hing und mit ` · `-Separator angehängt wurde.
	header := []string{"  " + theme.Highlight(title, h.pal)}
	if hint != "" {
		header = append(header, "  "+theme.Dim(hint, h.pal))
	}
	header = append(header, "")
	return strings.Join(append(header, rows...), "\n")
}

// helpSectionsHeute is the canonical Heute key-binding inventory. Both
// the standalone `?`-overlay (heute.renderHelpRows) and the sidekick's
// aggregated `?`-overlay (sidekick.renderHelp via worktime.HelpSections)
// read from this single source so the two surfaces cannot drift.
func helpSectionsHeute() []help.Section {
	return []help.Section{
		{Title: "Worktime — Heute · Cursor & Action", Keys: [][2]string{
			{"j/k · g/G", "bewegen · oben/unten"},
			{"s", "starten / stoppen / fortsetzen"},
			{"p", "pause (im laufenden Zustand)"},
		}},
		{Title: "Worktime — Heute · Session-Edit (auf fokussierter Zeile)", Keys: [][2]string{
			{"E / Enter", "Session bearbeiten"},
			{"D", "Session löschen (y/Enter bestätigt)"},
			{"t", "Session-Tag setzen"},
			{"N", "Session-Notiz setzen (großgeschrieben)"},
		}},
		{Title: "Worktime — Heute · Kompendium (für heute)", Keys: [][2]string{
			{"n", "Note anhängen (ID eingeben)"},
			{"o", "erste Note inline ansehen (integrierter Markdown-Viewer)"},
			{"O", "erste Note im Editor öffnen"},
			{"R", "erste Note entfernen"},
		}},
	}
}

// renderHelpRows enumerates Heute's keybinds for the standalone `?`
// overlay. Reads from helpSectionsHeute so the standalone overlay and
// the sidekick aggregator stay in lockstep.
func (h heute) renderHelpRows(inner int) []string {
	sections := helpSectionsHeute()
	rows := []string{}
	for i, sec := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		// Strip the "Worktime — Heute · " prefix in standalone mode —
		// the parent context is implicit when the user opened heute's
		// own help overlay, and the prefix wastes horizontal real
		// estate inside the cramped dialog frame.
		title := strings.TrimPrefix(sec.Title, "Worktime — Heute · ")
		rows = append(rows, picker.SectionHeader(title, inner, h.pal))
		for _, kv := range sec.Keys {
			keyCell := lipgloss.NewStyle().Width(theme.KeyHintWidth).Render(theme.Highlight(kv[0], h.pal))
			rows = append(rows, "  "+keyCell+stDim(h.pal, kv[1]))
		}
	}
	return rows
}
