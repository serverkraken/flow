package browse

// Status-Bar- und Footer-Render-Helper. Split aus model.go (Skill
// §No-Monoliths). Die Status-Bar trägt eigene Mode-/Path-/Meta-
// Buchhaltung mit elastischer Path-Truncation; renderFooter wechselt
// zwischen ModeSearch / ModeConfirmDelete / Help-View. Beide haben
// keine Verbindung zu Row-Render-Logic, gehören semantisch in ein
// eigenes File.

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	uistrings "github.com/serverkraken/flow/internal/frontend/tui/components/strings"
)

func (m Model) renderFooter() string {
	switch m.mode {
	case ModeSearch:
		return footerStyle.Render("tippen → filtern  ·  Enter → anwenden  ·  Esc → abbrechen")
	case ModeConfirmDelete:
		// Wording aus components/strings.HintConfirm — confirm-Modal-
		// Hint und Footer-Hint synchron, ein DE-Drift fällt sofort auf.
		return footerStyle.Render(uistrings.HintConfirm)
	}
	return m.helpUI.View(m.keys)
}

// renderStatusBar is the bar at the bottom of the frame. Left to right:
// transient-mode badge (only while searching or confirming a delete),
// current note path (cursor's ID, truncated to fit), and a meta tail
// with the index's age plus a `?` help hint. The bar paints
// pal.BgChip across its full inner width so it reads as a contiguous
// strip; the path is the elastic cell so the line never wraps onto a
// second row.
func (m Model) renderStatusBar() string {
	width := m.contentWidth()
	if width <= 0 {
		return ""
	}
	mode := m.statusBarMode()
	meta := m.statusBarMeta()

	// Reserve a single padding space between mode/path and path/meta.
	consumed := lipgloss.Width(mode) + lipgloss.Width(meta) + 2
	avail := width - consumed
	if avail < 5 {
		avail = 5
	}
	path := m.statusBarPath()
	if lipgloss.Width(path) > avail {
		path = truncateText(path, avail)
	}

	pathSegment := statusBarPathStyle.Render(" " + path)
	gap := width - lipgloss.Width(mode) - lipgloss.Width(pathSegment) - lipgloss.Width(meta)
	if gap < 0 {
		gap = 0
	}
	return mode + pathSegment + statusBarStyle.Render(strings.Repeat(" ", gap)) + meta
}

// statusBarMode returns the transient-mode badge — only Search and
// Delete-Confirm get one. Normal mode renders nothing so the bar starts
// with the path directly: there's no concept of a vim-style "NORMAL"
// state to communicate, and labelling one made the bar read as if there
// were modes the user could switch between.
func (m Model) statusBarMode() string {
	switch m.mode {
	case ModeSearch:
		return statusBarModeSearchStyle.Render("SUCHE")
	case ModeConfirmDelete:
		return statusBarModeDeleteStyle.Render("LÖSCHEN")
	}
	return ""
}

// statusBarPath returns the cursor's note ID (notebook-relative path,
// no .md suffix). Falls back to "—" when no entry is selected so the
// bar stays a stable shape across empty/loading states.
func (m Model) statusBarPath() string {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return "—"
	}
	return m.visible[m.cursor].ID.String()
}

// statusBarMeta builds the right-aligned tail. Today that's just the
// index age (when the IndexAgeFunc is wired and produced a non-zero
// time); a `? help` hint used to live here too but the help footer
// directly above the bar already lists `?`, so the second copy was just
// noise. Returns "" when there's nothing to show — keeps the bar
// flush-right without dangling padding.
func (m Model) statusBarMeta() string {
	if m.indexAge == nil {
		return ""
	}
	t := m.indexAge()
	if t.IsZero() {
		return ""
	}
	label := statusBarMetaStyle.Render("Index " + humanizeAge(time.Since(t)))
	return statusBarStyle.Render(" ") + label + statusBarStyle.Render(" ")
}

// humanizeAge renders a duration as a compact "5s" / "12m" / "3h" /
// "4d" string. Anything under one second collapses to "now".
func humanizeAge(d time.Duration) string {
	if d < time.Second {
		return "now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
