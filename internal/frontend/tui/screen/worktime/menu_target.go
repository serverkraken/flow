// Output-Target-Sub-Picker für das Worktime-Aktions-Menü. Tritt auf den
// Plan, sobald der User in der Aktions-Liste eine output-tragende Aktion
// (Brief / Export / Stats) wählt — drei feste Ziele Clipboard /
// tmux-Split / Datei. Hotkeys c · s · f, enter wählt cursor, esc geht
// in die Aktions-Liste zurück.
//
// Die Sub-Picker-Mechanik ist absichtlich klein gehalten: keine Filter-
// Eingabe, keine Sections — drei Optionen sind kein Picker-Bedarfsfall,
// das Layout folgt §Spacing-Skala mit zwei-Cell-Indent + monospace-Hot-
// key-Spalte rechts.

package worktime

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// outputTarget is the user's picked sink for the active action's
// rendered content. The numeric values are stable enough to use in a
// switch without naming churn.
type outputTarget int

const (
	// outputTargetSplit ist der Default: less -S für Text/CSV/JSON im
	// tmux-Split, Markdown-Inhalte laufen seit G3 durch das in-process
	// Brief-Overlay (siehe brief_view.go) statt durch einen externen
	// Pager.
	outputTargetSplit outputTarget = iota
	// outputTargetClipboard kopiert den Inhalt via pbcopy.
	outputTargetClipboard
	// outputTargetFile schreibt nach ~/Downloads/<basename>-<ts>.<ext>.
	outputTargetFile
)

// hotkey returns the c/s/f single-rune shortcut for direct selection.
func (t outputTarget) hotkey() string {
	switch t {
	case outputTargetSplit:
		return "s"
	case outputTargetClipboard:
		return "c"
	case outputTargetFile:
		return "f"
	}
	return ""
}

// label returns the German user-facing target name (Skill §German UI).
func (t outputTarget) label() string {
	switch t {
	case outputTargetSplit:
		return "tmux-Split"
	case outputTargetClipboard:
		return "Zwischenablage"
	case outputTargetFile:
		return "Datei in ~/Downloads"
	}
	return ""
}

// hint returns the right-aligned per-row meta text. For the split
// target it shows the planned viewer (the external pager command for
// CSV/JSON/Text, or the literal "integriert" for Markdown content that
// goes into the in-process overlay); for clipboard the underlying
// tool; for file the empty placeholder.
func (t outputTarget) hint(viewer string) string {
	switch t {
	case outputTargetSplit:
		if viewer == "" {
			return "integriert"
		}
		return viewer
	case outputTargetClipboard:
		return "pbcopy"
	}
	return ""
}

// targetPicker is the modal-state of the output-target sub-picker.
// It carries the cursor and the suggested viewer (for the split-row
// hint). The menu owns the lifetime — `targetPicker{}` resets to the
// default cursor (Split) on each new pending-action.
type targetPicker struct {
	cursor int
	viewer string
}

// newTargetPicker builds a fresh picker primed at outputTargetSplit
// with the given viewer hint. viewer is shown as the right-side meta
// of the split row.
func newTargetPicker(viewer string) targetPicker {
	return targetPicker{cursor: int(outputTargetSplit), viewer: viewer}
}

const targetCount = 3

// targetEvent is the result of one keystroke fed into the target
// picker. The menu inspects it and either closes (canceled), routes
// the picked target through dispatch (picked), or stays in target mode
// (still navigating).
type targetEvent struct {
	picked   bool
	canceled bool
	target   outputTarget
}

// handleKey routes a key into the picker. Returns the (possibly
// updated) picker plus an event describing what happened: nav-only,
// target picked, or cancel requested.
func (tp targetPicker) handleKey(msg tea.KeyPressMsg) (targetPicker, targetEvent) {
	switch msg.String() {
	case "esc":
		return tp, targetEvent{canceled: true}
	case "j", "down":
		tp.cursor = (tp.cursor + 1) % targetCount
	case "k", "up":
		tp.cursor = (tp.cursor + targetCount - 1) % targetCount
	case "g":
		tp.cursor = 0
	case "G":
		tp.cursor = targetCount - 1
	case "enter":
		return tp, targetEvent{picked: true, target: outputTarget(tp.cursor)}
	case "c":
		tp.cursor = int(outputTargetClipboard)
		return tp, targetEvent{picked: true, target: outputTargetClipboard}
	case "s":
		tp.cursor = int(outputTargetSplit)
		return tp, targetEvent{picked: true, target: outputTargetSplit}
	case "f":
		tp.cursor = int(outputTargetFile)
		return tp, targetEvent{picked: true, target: outputTargetFile}
	}
	return tp, targetEvent{}
}

// view renders the target picker body. parentLabel is the action the
// user came from (e.g. "Brief Wochenbericht") and gets shown as a
// purple-bold title — same Identity-mark as dialog titles in today.go
// and dayoffs.go.
func (tp targetPicker) view(parentLabel string, pal theme.Palette, inner int) string {
	rows := []string{
		theme.Highlight("  Aktion · "+parentLabel, pal),
		"",
		picker.SectionHeader("output-ziel", inner, pal),
	}
	// Underline zusätzlich zum Accent: unter NO_COLOR fällt der Foreground
	// weg und der Hotkey-Buchstabe wäre nur noch bold — Underline hält das
	// c/s/f auch ohne Farbe als Hotkey erkennbar (A11y: nie nur Farbe).
	hkStyle := lipgloss.NewStyle().Foreground(pal.Sem().Accent).Bold(true).Underline(true)
	for i := 0; i < targetCount; i++ {
		t := outputTarget(i)
		hint := t.hint(tp.viewer)
		hk := hkStyle.Render(t.hotkey())
		if hint != "" {
			hint = hint + "   " + hk
		} else {
			hint = hk
		}
		rows = append(rows, picker.Row(i == tp.cursor, t.label(), hint, inner, pal))
	}
	rows = append(rows, "", renderFooterHints(pal, []string{
		"c · s · f → direkt",
		"enter → ausführen",
		"esc → zurück",
	}, inner))
	return strings.Join(rows, "\n")
}
