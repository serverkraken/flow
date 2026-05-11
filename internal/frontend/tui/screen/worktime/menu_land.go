// Bundesland-Picker für die Land-Action. Listet die 16 Bundesländer
// (plus „DE" — bundesweit-only-Variante) und triggert nach Auswahl
// SyncGermanHolidays für das aktuelle Jahr.
//
// Persistenz nach worktime.conf ist Slice F1 (siehe Plan) — diese
// Iteration hält das gewählte Land nur für den nächsten Sync.

package worktime

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/serverkraken/flow/internal/frontend/tui/components/picker"
	"github.com/serverkraken/flow/internal/frontend/tui/theme"
)

// landEntry is one row in the land picker. code is the two-letter
// Bundesland-Code GermanHolidays accepts; label is the German
// Klartext — sortiert nach Bundesland-Name (außer „DE — bundesweit"
// als head).
type landEntry struct {
	code  string
	label string
}

// landEntries sind die 16 Bundesländer + bundesweit-only (DE) als
// erstes Element, damit der Cursor-Default „DE" auf alle gemeinsamen
// Feiertage reduziert.
var landEntries = []landEntry{
	{"DE", "Deutschland (bundesweit)"},
	{"BW", "Baden-Württemberg"},
	{"BY", "Bayern"},
	{"BE", "Berlin"},
	{"BB", "Brandenburg"},
	{"HB", "Bremen"},
	{"HH", "Hamburg"},
	{"HE", "Hessen"},
	{"MV", "Mecklenburg-Vorpommern"},
	{"NI", "Niedersachsen"},
	{"NW", "Nordrhein-Westfalen"},
	{"RP", "Rheinland-Pfalz"},
	{"SL", "Saarland"},
	{"SN", "Sachsen"},
	{"ST", "Sachsen-Anhalt"},
	{"SH", "Schleswig-Holstein"},
	{"TH", "Thüringen"},
}

// landPicker is the modal state of the Bundesland sub-picker.
// initialCode is the cursor's starting position — the user's
// last-known land (landOrDefault(deps.Land) at open-time) so a quick
// re-sync against the same state is one Enter away.
type landPicker struct {
	cursor int
}

// newLandPicker builds a picker with the cursor on currentCode (or
// the first entry when currentCode isn't in the list).
func newLandPicker(currentCode string) landPicker {
	idx := indexOfLand(currentCode)
	if idx < 0 {
		idx = 0
	}
	return landPicker{cursor: idx}
}

// indexOfLand returns the row index for code (case-insensitive) or
// -1 when it isn't in landEntries.
func indexOfLand(code string) int {
	want := strings.ToUpper(strings.TrimSpace(code))
	if want == "NRW" {
		want = "NW" // documented alias from CLI usage
	}
	for i, e := range landEntries {
		if e.code == want {
			return i
		}
	}
	return -1
}

// landEvent is the keystroke result. canceled rolls back to the action
// list; picked carries the chosen entry. Slice E doesn't expose a
// hotkey-per-Land — the list is too long for sensible single-rune
// shortcuts and the live filter (palette-style /) would be a Slice F
// follow-up.
type landEvent struct {
	picked   bool
	canceled bool
	entry    landEntry
}

// handleKey routes a key into the picker. Esc cancels; j/k/Up/Down
// navigate; g/G jump to first/last; Enter picks the focused row.
func (lp landPicker) handleKey(msg tea.KeyMsg) (landPicker, landEvent) {
	switch msg.String() {
	case "esc":
		return lp, landEvent{canceled: true}
	case "j", "down":
		lp.cursor = (lp.cursor + 1) % len(landEntries)
	case "k", "up":
		lp.cursor = (lp.cursor + len(landEntries) - 1) % len(landEntries)
	case "g":
		lp.cursor = 0
	case "G":
		lp.cursor = len(landEntries) - 1
	case "enter":
		return lp, landEvent{picked: true, entry: landEntries[lp.cursor]}
	}
	return lp, landEvent{}
}

// view renders the land list. parentLabel is the action that opened
// the picker (e.g. „Land für Feiertage") so the user always sees what
// they're picking for.
func (lp landPicker) view(parentLabel string, p theme.Palette, inner int) string {
	rows := []string{
		theme.Highlight("  Aktion · "+parentLabel, p),
		"",
		picker.SectionHeader(fmt.Sprintf("bundesland (%d)", len(landEntries)), inner, p),
	}
	for i, e := range landEntries {
		rows = append(rows, picker.Row(i == lp.cursor, e.label, e.code, inner, p))
	}
	rows = append(rows, "", renderFooterHints(p, []string{
		"j/k → bewegen",
		"enter → syncen",
		"esc → zurück",
	}, inner))
	return strings.Join(rows, "\n")
}

// landSyncCmd runs DayOffWriter.SyncGermanHolidays(year, code) for
// the current year and surfaces the (added, skipped) result in a
// toast. No persistence to worktime.conf — that's Slice F1.
func landSyncCmd(deps Deps, code string) tea.Cmd {
	return func() tea.Msg {
		if deps.DayOffWriter == nil {
			return menuActionDoneMsg{err: fmt.Errorf("dayoff writer nicht verdrahtet")}
		}
		if deps.Clock == nil {
			return menuActionDoneMsg{err: fmt.Errorf("clock nicht verdrahtet")}
		}
		year := deps.Clock.Now().Year()
		added, skipped, err := deps.DayOffWriter.SyncGermanHolidays(year, code, time.Local)
		if err != nil {
			return menuActionDoneMsg{err: fmt.Errorf("sync %s/%d: %w", code, year, err)}
		}
		return menuActionDoneMsg{
			toast: fmt.Sprintf("✓ %s/%d: %d Feiertag(e) ergänzt, %d übersprungen",
				code, year, added, skipped),
		}
	}
}
