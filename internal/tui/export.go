package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const dayFmtTUI = "2006-01-02"

// exportPresetRange maps a preset name + "now" to an inclusive [from,to] date
// range as yyyy-mm-dd strings (in now's location):
//   - "monat":   first of current month → today
//   - "kw":      Monday of current week → today
//   - "letzter": first → last day of the previous month
//   - anything else (incl. "custom"): today → today (caller overrides for custom)
func exportPresetRange(preset string, now time.Time) (string, string) {
	y, mo, d := now.Date()
	loc := now.Location()
	today := time.Date(y, mo, d, 0, 0, 0, 0, loc)
	switch preset {
	case "monat":
		from := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		return from.Format(dayFmtTUI), today.Format(dayFmtTUI)
	case "kw":
		// Monday=0 offset: Go Sunday=0..Saturday=6 → days since Monday.
		off := (int(today.Weekday()) + 6) % 7
		from := today.AddDate(0, 0, -off)
		return from.Format(dayFmtTUI), today.Format(dayFmtTUI)
	case "letzter":
		firstThis := time.Date(y, mo, 1, 0, 0, 0, 0, loc)
		lastPrev := firstThis.AddDate(0, 0, -1)
		firstPrev := time.Date(lastPrev.Year(), lastPrev.Month(), 1, 0, 0, 0, 0, loc)
		return firstPrev.Format(dayFmtTUI), lastPrev.Format(dayFmtTUI)
	default:
		return today.Format(dayFmtTUI), today.Format(dayFmtTUI)
	}
}

// defaultExportPath builds the suggested target path. format is also the ext.
func defaultExportPath(from, to, format string) string {
	return "~/Downloads/flow-export-" + from + "_" + to + "." + format
}

// expandHome resolves a leading "~/" against the user's home directory.
func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if len(path) >= 2 && path[:2] == "~/" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// cycleFormat steps through csv → json → md (dir +1) or reverse (dir -1).
func cycleFormat(f string, dir int) string {
	order := []string{"csv", "json", "md"}
	return cycle(order, f, dir)
}

// cyclePreset steps through kw → monat → letzter → custom.
func cyclePreset(p string, dir int) string {
	order := []string{"kw", "monat", "letzter", "custom"}
	return cycle(order, p, dir)
}

// cycle returns the element dir steps from cur within order (wrapping). If cur
// is not a member of order, idx is treated as 0 (callers always pass a member).
func cycle(order []string, cur string, dir int) string {
	idx := 0
	for i, v := range order {
		if v == cur {
			idx = i
			break
		}
	}
	n := (idx + dir + len(order)) % len(order)
	return order[n]
}

// openExport initialises the export overlay state with sensible defaults
// (current month, markdown) relative to m.now.
func (m Model) openExport() Model {
	m.showExport = true
	m.expPreset = "monat"
	m.expFormat = "md"
	from, to := exportPresetRange("monat", m.now)
	m.expFrom, m.expTo = from, to
	m.expPath = defaultExportPath(from, to, "md")
	m.expPathEdited = false
	m.expFocus = 0
	m.expStatus = ""
	return m
}

// exportView renders the export overlay.
func (m Model) exportView() tea.View {
	var b strings.Builder
	b.WriteString(styleHeader.Render("flow · Export") + "\n\n")
	field := func(idx int, label, val string) {
		cursor := "  "
		render := val
		if m.expFocus == idx {
			cursor = styleSel.Render("▸") + " "
			render = styleSel.Render(val)
		}
		fmt.Fprintf(&b, "%s%-8s %s\n", cursor, label, render)
	}
	field(0, "Range", m.expPreset)
	field(1, "von", m.expFrom)
	field(2, "bis", m.expTo)
	field(3, "Format", m.expFormat)
	field(4, "Pfad", m.expPath)
	b.WriteString("\n")
	if m.expStatus != "" {
		b.WriteString(styleMuted.Render(m.expStatus) + "\n\n")
	}
	b.WriteString(styleMuted.Render("tab Feld · ←/→ wählen · enter export · esc back") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// handleExportKey is fleshed out in Task 3.
func (m Model) handleExportKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m, nil
}
