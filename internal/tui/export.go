package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type exportDoneMsg struct{ path string }
type exportErrMsg struct{ err error }

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
		st := styleMuted
		switch {
		case strings.HasPrefix(m.expStatus, "✓"):
			st = styleOk
		case strings.HasPrefix(m.expStatus, "Fehler") || strings.HasPrefix(m.expStatus, "Ungültiges") || strings.HasPrefix(m.expStatus, "bis muss"):
			st = styleErr
		}
		b.WriteString(st.Render(m.expStatus) + "\n\n")
	}
	b.WriteString(styleMuted.Render("tab Feld · ←/→ wählen · enter export · esc back") + "\n")
	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// handleExportKey handles keys within the export overlay (Esc is handled by the
// caller). Tab/Shift-Tab move focus; ←/→ cycle choice fields (preset/format);
// typing edits text fields (von/bis/path); Enter triggers the export.
func (m Model) handleExportKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case k.Code == tea.KeyTab:
		if k.Mod == tea.ModShift {
			m.expFocus = (m.expFocus + 4) % 5
		} else {
			m.expFocus = (m.expFocus + 1) % 5
		}
		return m, nil
	case k.Code == tea.KeyEnter:
		return m.submitExport()
	case k.Code == tea.KeyLeft || k.Code == tea.KeyRight:
		dir := 1
		if k.Code == tea.KeyLeft {
			dir = -1
		}
		return m.cycleField(dir), nil
	case k.Code == tea.KeyBackspace:
		return m.editField(func(s string) string {
			if r := []rune(s); len(r) > 0 {
				return string(r[:len(r)-1])
			}
			return s
		}), nil
	case k.Text != "":
		t := k.Text
		return m.editField(func(s string) string { return s + t }), nil
	}
	return m, nil
}

// cycleField advances the focused choice field (preset/format) and recomputes
// dependent state. No-op on text fields.
func (m Model) cycleField(dir int) Model {
	switch m.expFocus {
	case 0: // preset
		m.expPreset = cyclePreset(m.expPreset, dir)
		if m.expPreset != "custom" {
			m.expFrom, m.expTo = exportPresetRange(m.expPreset, m.now)
		}
		refreshDefaultPath(&m)
	case 3: // format
		m.expFormat = cycleFormat(m.expFormat, dir)
		refreshDefaultPath(&m)
	}
	return m
}

// editField applies fn to the focused text field (von/bis/path). Editing a date
// switches the preset to "custom"; editing the path marks it user-owned.
func (m Model) editField(fn func(string) string) Model {
	switch m.expFocus {
	case 1:
		m.expFrom = fn(m.expFrom)
		m.expPreset = "custom"
		refreshDefaultPath(&m)
	case 2:
		m.expTo = fn(m.expTo)
		m.expPreset = "custom"
		refreshDefaultPath(&m)
	case 4:
		m.expPath = fn(m.expPath)
		m.expPathEdited = true
	}
	return m
}

// refreshDefaultPath recomputes the suggested path from from/to/format, unless
// the user has manually edited the path.
func refreshDefaultPath(m *Model) {
	if !m.expPathEdited {
		m.expPath = defaultExportPath(m.expFrom, m.expTo, m.expFormat)
	}
}

// submitExport validates the date range and, if valid, dispatches exportCmd.
// Invalid dates or to<from set an inline status and dispatch nothing.
func (m Model) submitExport() (tea.Model, tea.Cmd) {
	from, errF := time.Parse(dayFmtTUI, m.expFrom)
	to, errT := time.Parse(dayFmtTUI, m.expTo)
	if errF != nil || errT != nil {
		m.expStatus = "Ungültiges Datum (yyyy-mm-dd erwartet)"
		return m, nil
	}
	if to.Before(from) {
		m.expStatus = "bis muss >= von sein"
		return m, nil
	}
	if m.client == nil {
		m.expStatus = "Fehler: kein Server verbunden"
		return m, nil
	}
	m.expStatus = "exportiere…"
	return m, m.exportCmd()
}

// exportCmd fetches the export from the server and writes it to the resolved
// path, returning exportDoneMsg{path} or exportErrMsg{err}.
func (m Model) exportCmd() tea.Cmd {
	from, to, format, path := m.expFrom, m.expTo, m.expFormat, expandHome(m.expPath)
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		b, err := client.Export(ctx, from, to, format, "")
		if err != nil {
			return exportErrMsg{err}
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return exportErrMsg{err}
		}
		return exportDoneMsg{path: path}
	}
}
